package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"circle-go/internal/llm"
)

type ResponseMode string

const (
	ResponseModeDirect    ResponseMode = "direct"
	ResponseModeSummarize ResponseMode = "summarize"
	ResponseModeHybrid    ResponseMode = "hybrid"
)

type ResponsePolicyConfig struct {
	Mode                 ResponseMode
	SummarizeTimeout     time.Duration
	SummarizeOnToolError bool
}

type ToolExecutionOutcome struct {
	ToolName    string
	ToolArgs    map[string]interface{}
	ToolResult  string
	ToolErr     error
	FromReroute bool
}

type ResponsePolicyEngine struct {
	llm    llm.LLM
	config ResponsePolicyConfig
}

func NewResponsePolicyEngine(model llm.LLM, config ResponsePolicyConfig) *ResponsePolicyEngine {
	cfg := config
	if cfg.Mode == "" {
		cfg.Mode = ResponseModeHybrid
	}
	if cfg.SummarizeTimeout <= 0 {
		cfg.SummarizeTimeout = 12 * time.Second
	}
	return &ResponsePolicyEngine{llm: model, config: cfg}
}

func (e *ResponsePolicyEngine) ShouldSummarize(userInput string, outcome ToolExecutionOutcome) bool {
	switch e.config.Mode {
	case ResponseModeDirect:
		return false
	case ResponseModeSummarize:
		return true
	default:
		text := strings.ToLower(strings.TrimSpace(userInput))
		if strings.Contains(text, "详细") || strings.Contains(text, "总结") || strings.Contains(text, "解释") {
			return true
		}
		if outcome.ToolErr != nil {
			return e.config.SummarizeOnToolError
		}
		if len(outcome.ToolResult) > 900 {
			return true
		}
		return false
	}
}

func (e *ResponsePolicyEngine) BuildFinal(ctx context.Context, userInput string, outcome ToolExecutionOutcome) (string, bool) {
	// returns (finalText, summarized)
	if !e.ShouldSummarize(userInput, outcome) || e.llm == nil {
		if outcome.ToolErr != nil {
			return fmt.Sprintf("工具 %s 执行失败：%v", outcome.ToolName, outcome.ToolErr), false
		}
		return outcome.ToolResult, false
	}
	sumCtx, cancel := context.WithTimeout(ctx, e.config.SummarizeTimeout)
	defer cancel()

	errorText := ""
	if outcome.ToolErr != nil {
		errorText = outcome.ToolErr.Error()
	}
	prompt := fmt.Sprintf(
		`请基于以下工具执行结果生成面向用户的最终回复，保持准确且简洁。
用户输入: %s
工具: %s
参数: %s
工具结果: %s
工具错误: %s

要求:
1) 若工具结果包含明确结论，优先保留结论。
2) 若结果里存在明显异常字段，提示用户谨慎解读。
3) 不要虚构数据。`,
		userInput,
		outcome.ToolName,
		mustJSON(outcome.ToolArgs),
		outcome.ToolResult,
		errorText,
	)
	text, err := e.llm.Chat(sumCtx, []llm.Message{
		{Role: "system", Content: "你是结果整合器。"},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		if outcome.ToolErr != nil {
			return fmt.Sprintf("工具 %s 执行失败：%v", outcome.ToolName, outcome.ToolErr), false
		}
		return outcome.ToolResult, false
	}
	return text, true
}
