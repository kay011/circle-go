package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"circle-go/internal/llm"
	"circle-go/internal/logging"
)

type ToolRouterConfig struct {
	Enabled             bool
	MinConfidence       float64
	Timeout             time.Duration
	ErrorRerouteEnabled bool
	ErrorRerouteTimeout time.Duration
}

type ToolRouter struct {
	llm    llm.LLM
	config ToolRouterConfig
}

type aiToolRoutingDecision struct {
	Tool       string                 `json:"tool"`
	Arguments  map[string]interface{} `json:"arguments"`
	Confidence float64                `json:"confidence"`
	Reason     string                 `json:"reason"`
}

func NewToolRouter(model llm.LLM, config ToolRouterConfig) *ToolRouter {
	cfg := config
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.68
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.ErrorRerouteTimeout <= 0 {
		cfg.ErrorRerouteTimeout = 6 * time.Second
	}
	return &ToolRouter{llm: model, config: cfg}
}

func (r *ToolRouter) Route(ctx context.Context, userInput string, current *llm.FunctionCall, availableTools []llm.Tool) *llm.FunctionCall {
	if current == nil || r.llm == nil || !r.config.Enabled {
		return current
	}
	toolNames := make([]string, 0, len(availableTools))
	for _, t := range availableTools {
		toolNames = append(toolNames, t.Name)
	}
	routerPrompt := fmt.Sprintf(
		`你是 Agent 工具路由器。目标：根据用户意图判断当前工具是否匹配，并在必要时纠偏。
只返回 JSON，不要解释。

可选工具: %s
当前工具: %s
当前参数: %s
用户输入: %s

返回格式:
{
  "tool": "工具名，必须在可选工具中；若保持不变则返回当前工具",
  "arguments": { "纠偏后的参数对象；若保持不变可返回当前参数" },
  "confidence": 0.0,
  "reason": "一句话原因"
}`,
		strings.Join(toolNames, ", "),
		current.Name,
		mustJSON(current.Arguments),
		userInput,
	)

	routeCtx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	raw, err := r.llm.Chat(routeCtx, []llm.Message{
		{Role: "system", Content: "你是严格输出 JSON 的工具路由器。"},
		{Role: "user", Content: routerPrompt},
	})
	if err != nil {
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "error"})
		return current
	}

	decision, ok := parseRoutingDecision(raw)
	if !ok {
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "invalid"})
		return current
	}
	if !toolExists(availableTools, decision.Tool) {
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "unknown_tool"})
		return current
	}
	if decision.Confidence < r.config.MinConfidence {
		logging.ObserveMetricWithLabels("tool_router_confidence", map[string]string{"decision": "below_threshold"}, decision.Confidence)
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "keep"})
		return current
	}
	logging.ObserveMetricWithLabels("tool_router_confidence", map[string]string{"decision": "switch"}, decision.Confidence)
	if decision.Tool == current.Name {
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "keep"})
	} else {
		logging.IncrMetricWithLabels("tool_router_total", map[string]string{"decision": "switch"})
	}
	newArgs := decision.Arguments
	if len(newArgs) == 0 {
		newArgs = current.Arguments
	}
	return &llm.FunctionCall{Name: decision.Tool, Arguments: newArgs}
}

func (r *ToolRouter) RerouteAfterError(ctx context.Context, userInput string, attempted *llm.FunctionCall, toolErr error, availableTools []llm.Tool) *llm.FunctionCall {
	if attempted == nil || toolErr == nil || r.llm == nil || !r.config.ErrorRerouteEnabled {
		return nil
	}
	names := make([]string, 0, len(availableTools))
	for _, t := range availableTools {
		if t.Name == attempted.Name {
			continue
		}
		names = append(names, t.Name)
	}
	if len(names) == 0 {
		return nil
	}
	prompt := fmt.Sprintf(
		`当前工具执行失败，请基于错误自动纠偏，选择一个替代工具（不能是 %s）。
只返回 JSON。
候选工具: %s
用户输入: %s
失败错误: %s
格式:
{"tool":"替代工具","arguments":{},"confidence":0.0,"reason":"一句话原因"}`,
		attempted.Name,
		strings.Join(names, ", "),
		userInput,
		toolErr.Error(),
	)
	routeCtx, cancel := context.WithTimeout(ctx, r.config.ErrorRerouteTimeout)
	defer cancel()
	raw, err := r.llm.Chat(routeCtx, []llm.Message{
		{Role: "system", Content: "你是工具纠偏路由器，只输出 JSON。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		logging.IncrMetricWithLabels("tool_reroute_total", map[string]string{"decision": "error"})
		return nil
	}
	decision, ok := parseRoutingDecision(raw)
	if !ok || decision.Tool == attempted.Name {
		logging.IncrMetricWithLabels("tool_reroute_total", map[string]string{"decision": "keep"})
		return nil
	}
	if !toolExists(availableTools, decision.Tool) {
		logging.IncrMetricWithLabels("tool_reroute_total", map[string]string{"decision": "unknown_tool"})
		return nil
	}
	if decision.Confidence < r.config.MinConfidence {
		logging.ObserveMetricWithLabels("tool_router_confidence", map[string]string{"decision": "reroute_below_threshold"}, decision.Confidence)
		return nil
	}
	logging.ObserveMetricWithLabels("tool_router_confidence", map[string]string{"decision": "reroute_switch"}, decision.Confidence)
	logging.IncrMetricWithLabels("tool_reroute_total", map[string]string{"decision": "switch"})
	return &llm.FunctionCall{Name: decision.Tool, Arguments: decision.Arguments}
}

func parseRoutingDecision(raw string) (aiToolRoutingDecision, bool) {
	block := extractJSONBlock(raw)
	var decision aiToolRoutingDecision
	if err := json.Unmarshal([]byte(block), &decision); err != nil {
		return aiToolRoutingDecision{}, false
	}
	decision.Tool = strings.TrimSpace(decision.Tool)
	if decision.Tool == "" {
		return aiToolRoutingDecision{}, false
	}
	decision.Confidence = parseConfidence(decision.Confidence)
	return decision, true
}

func extractJSONBlock(text string) string {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```json")
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSuffix(raw, "```")
		raw = strings.TrimSpace(raw)
	}
	return raw
}

func parseConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func parseConfidenceText(s string) float64 {
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return parseConfidence(n)
}

