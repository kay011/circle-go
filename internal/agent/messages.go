package agent

import (
	"circle-go/internal/llm"
	"circle-go/internal/logging"
)

// buildBaseMessages 构造 system + 记忆增强 + 短期历史/摘要 + 本轮 user（若需要）
func (a *Agent) buildBaseMessages(sessionID, userInput string, logger *logging.Logger) []llm.Message {
	messages := []llm.Message{
		{Role: "system", Content: a.systemPrompt},
	}

	if a.memoryManager != nil {
		if aug := a.memoryManager.SystemAugmentation(sessionID, userInput); aug != "" {
			messages = append(messages, llm.Message{Role: "system", Content: aug})
		}
	}

	if a.memoryManager != nil {
		session := a.memoryManager.GetSession(sessionID)
		if session != nil && len(session.ShortTerm) > 0 {
			for _, msg := range session.ShortTerm {
				messages = append(messages, llm.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
			if logger != nil {
				logger.Info("加载对话历史", map[string]interface{}{
					"session_id":    sessionID,
					"history_count": len(session.ShortTerm),
				})
			}
		} else {
			summary := a.memoryManager.SummarizeMemory(sessionID)
			if summary != "" {
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: summary,
				})
			}
		}
	}

	if a.shouldAppendUserMessage(sessionID, userInput) {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: userInput,
		})
	}

	return messages
}
