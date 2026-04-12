package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"circle-go/internal/llm"
	"circle-go/internal/logging"
)

const plannerSystemExtra = `你是任务规划器。根据用户问题与上下文，仅输出一个 JSON 对象（不要 Markdown 代码围栏以外的解释文字）。
JSON 格式严格如下：
{"steps":[{"type":"tool","tool":"工具名称","input":{参数对象}},{"type":"reason","detail":"推理说明"}]}
规则：
- type 只能是 "tool"、"reason" 之一。
- tool 步骤的 tool 必须是当前可用工具之一；input 为该工具的参数对象。
- 先规划再执行：步骤数建议 1～8，复杂任务可略多。
- 若无需工具即可回答，输出单个 {"type":"reason","detail":"可直接回答的要点"} 即可。`

type planStep struct {
	Type   string                 `json:"type"`
	Tool   string                 `json:"tool"`
	Input  map[string]interface{} `json:"input"`
	Detail string                 `json:"detail"`
}

type planDoc struct {
	Steps []planStep `json:"steps"`
}

func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```JSON")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func (a *Agent) runPlanExecute(ctx context.Context, sessionID, userInput string) (string, error) {
	logger := logging.NewLogger(logging.INFO, "Agent")
	logging.IncrMetric("agent_calls_total")
	logging.IncrMetric("agent_plan_execute_calls_total")
	start := time.Now()

	base := a.buildBaseMessages(sessionID, userInput, logger)
	planMsgs := append([]llm.Message{
		{Role: "system", Content: a.systemPrompt + "\n\n" + plannerSystemExtra + "\n\n可用工具名称: " + toolNames(a.llmTools)},
	}, dropLeadingSystem(base)...)

	planText, err := a.llm.Chat(ctx, planMsgs)
	if err != nil {
		return fmt.Sprintf("规划阶段失败: %v", err), nil
	}

	steps, perr := parsePlan(stripJSONFences(planText))
	if perr != nil {
		logger.Warn("规划 JSON 解析失败，退回单次对话", map[string]interface{}{"error": perr.Error()})
		return a.fallbackAnswer(ctx, base, logger)
	}

	var obs strings.Builder
	obs.WriteString("【规划】\n")
	obs.WriteString(planText)
	obs.WriteString("\n\n【执行记录】\n")

	for i, st := range steps {
		switch strings.ToLower(strings.TrimSpace(st.Type)) {
		case "tool":
			name := strings.TrimSpace(st.Tool)
			if name == "" {
				continue
			}
			res, err := a.toolManager.Run(ctx, name, st.Input)
			if err != nil {
				res = fmt.Sprintf("Error: %v", err)
			}
			line := fmt.Sprintf("步骤%d 工具 %s: %s\n", i+1, name, res)
			obs.WriteString(line)
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "plan_tool:"+name, res, 3)
			}
		case "reason":
			if strings.TrimSpace(st.Detail) != "" {
				obs.WriteString(fmt.Sprintf("步骤%d 推理: %s\n", i+1, st.Detail))
			}
		}
	}

	finalMsgs := append([]llm.Message{
		{Role: "system", Content: a.systemPrompt + "\n\n你是执行总结助手。根据用户问题与下方「规划与工具执行记录」生成最终回答，使用用户语言，结构清晰。"},
	}, dropLeadingSystem(base)...)
	finalMsgs = append(finalMsgs, llm.Message{
		Role:    "user",
		Content: obs.String(),
	})

	out, err := a.llm.Chat(ctx, finalMsgs)
	if err != nil {
		return fmt.Sprintf("总结阶段失败: %v", err), nil
	}
	if a.memoryManager != nil {
		a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
		a.memoryManager.AddLongTermMemory(sessionID, "ai_response", out, 2)
	}
	logger.Info("plan_execute 完成", map[string]interface{}{
		"session_id": sessionID,
		"agent_id":   a.spec.ID,
		"ms":         time.Since(start).Milliseconds(),
	})
	return out, nil
}

func (a *Agent) runPlanExecuteStream(ctx context.Context, sessionID, userInput string, callback func(chunk string) error) (string, error) {
	logger := logging.NewLogger(logging.INFO, "Agent")
	logging.IncrMetric("agent_calls_total")
	logging.IncrMetric("agent_plan_execute_calls_total")
	start := time.Now()

	base := a.buildBaseMessages(sessionID, userInput, logger)
	planMsgs := append([]llm.Message{
		{Role: "system", Content: a.systemPrompt + "\n\n" + plannerSystemExtra + "\n\n可用工具名称: " + toolNames(a.llmTools)},
	}, dropLeadingSystem(base)...)

	planText, err := a.llm.Chat(ctx, planMsgs)
	if err != nil {
		msg := fmt.Sprintf("规划阶段失败: %v", err)
		_ = callback(msg)
		return msg, nil
	}
	if err := callback("【规划】\n" + planText + "\n\n"); err != nil {
		return "", err
	}

	steps, perr := parsePlan(stripJSONFences(planText))
	if perr != nil {
		logger.Warn("规划 JSON 解析失败，退回流式单次对话", map[string]interface{}{"error": perr.Error()})
		return a.fallbackAnswerStream(ctx, base, callback)
	}

	var obs strings.Builder
	for i, st := range steps {
		switch strings.ToLower(strings.TrimSpace(st.Type)) {
		case "tool":
			name := strings.TrimSpace(st.Tool)
			if name == "" {
				continue
			}
			if err := callback(fmt.Sprintf("【执行】步骤%d 工具 %s ...\n", i+1, name)); err != nil {
				return "", err
			}
			res, err := a.toolManager.Run(ctx, name, st.Input)
			if err != nil {
				res = fmt.Sprintf("Error: %v", err)
			}
			if err := callback(res + "\n\n"); err != nil {
				return "", err
			}
			fmt.Fprintf(&obs, "步骤%d 工具 %s: %s\n", i+1, name, res)
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "plan_tool:"+name, res, 3)
			}
		case "reason":
			if strings.TrimSpace(st.Detail) != "" {
				line := fmt.Sprintf("步骤%d 推理: %s\n", i+1, st.Detail)
				_ = callback(line)
				obs.WriteString(line)
			}
		}
	}

	finalMsgs := append([]llm.Message{
		{Role: "system", Content: a.systemPrompt + "\n\n你是执行总结助手。根据用户问题与下方记录生成最终回答。"},
	}, dropLeadingSystem(base)...)
	finalMsgs = append(finalMsgs, llm.Message{
		Role:    "user",
		Content: "【规划】\n" + planText + "\n【执行记录】\n" + obs.String(),
	})

	var acc strings.Builder
	err = a.llm.ChatStream(ctx, finalMsgs, func(chunk string) error {
		acc.WriteString(chunk)
		return callback(chunk)
	})
	if err != nil {
		return "", err
	}
	out := acc.String()
	if a.memoryManager != nil {
		a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
		a.memoryManager.AddLongTermMemory(sessionID, "ai_response", out, 2)
	}
	logger.Info("plan_execute_stream 完成", map[string]interface{}{
		"session_id": sessionID,
		"agent_id":   a.spec.ID,
		"ms":         time.Since(start).Milliseconds(),
	})
	return out, nil
}

func toolNames(tools []llm.Tool) string {
	var b strings.Builder
	for i, t := range tools {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(t.Name)
	}
	return b.String()
}

// dropLeadingSystem 去掉首条 system（规划/总结阶段会单独加 system）
func dropLeadingSystem(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return nil
	}
	if msgs[0].Role == "system" {
		out := make([]llm.Message, 0, len(msgs)-1)
		out = append(out, msgs[1:]...)
		return out
	}
	return msgs
}

func parsePlan(s string) ([]planStep, error) {
	var doc planDoc
	if err := json.Unmarshal([]byte(s), &doc); err != nil {
		return nil, err
	}
	if len(doc.Steps) == 0 {
		return nil, fmt.Errorf("empty steps")
	}
	return doc.Steps, nil
}

func (a *Agent) fallbackAnswer(ctx context.Context, base []llm.Message, logger *logging.Logger) (string, error) {
	out, err := a.llm.Chat(ctx, base)
	if err != nil {
		return "", err
	}
	logger.Info("plan_execute 使用 fallback Chat", nil)
	return out, nil
}

func (a *Agent) fallbackAnswerStream(ctx context.Context, base []llm.Message, callback func(chunk string) error) (string, error) {
	var acc strings.Builder
	err := a.llm.ChatStream(ctx, base, func(chunk string) error {
		acc.WriteString(chunk)
		return callback(chunk)
	})
	return acc.String(), err
}
