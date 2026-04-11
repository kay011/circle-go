package agent

import (
	"context"
	"fmt"

	"circle-go/internal/llm"
	"circle-go/internal/memory"
	"circle-go/internal/tools"
)

// Agent AI智能体
type Agent struct {
	llm          llm.LLM
	toolManager  *tools.ToolManager
	memoryManager *memory.MemoryManager
	maxSteps     int
}

// NewAgent 创建Agent实例
func NewAgent(llm llm.LLM, toolManager *tools.ToolManager, maxSteps int) *Agent {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	return &Agent{
		llm:         llm,
		toolManager: toolManager,
		maxSteps:    maxSteps,
	}
}

// SetMemoryManager 设置记忆管理器
func (a *Agent) SetMemoryManager(memoryManager *memory.MemoryManager) {
	a.memoryManager = memoryManager
}

// Run 运行Agent
func (a *Agent) Run(ctx context.Context, sessionID, userInput string) (string, error) {
	// 初始化对话历史
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "你是一个AI助手，能够通过思考和使用工具来解决问题。请按照ReAct模式思考：先分析问题，然后决定是否需要使用工具，最后给出答案。",
		},
	}

	// 添加记忆摘要（如果有）
	if a.memoryManager != nil {
		summary := a.memoryManager.SummarizeMemory(sessionID)
		if summary != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: summary,
			})
		}
	}

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userInput,
	})

	// ReAct循环
	for step := 0; step < a.maxSteps; step++ {
		// 准备工具列表
		toolsList := a.toolManager.List()
		llmTools := make([]llm.Tool, len(toolsList))
		for i, tool := range toolsList {
			params := tool.Parameters()
			// 转换参数类型
			llmParams := make(map[string]llm.Property)
			for k, v := range params {
				llmParams[k] = llm.Property{
					Type:        v.Type,
					Description: v.Description,
				}
			}
			llmTools[i] = llm.Tool{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters: llm.ToolParameters{
					Type:       "object",
					Properties: llmParams,
					Required:   tool.Required(),
				},
			}
		}

		// 发送请求
		response, functionCall, err := a.llm.FunctionCall(ctx, messages, llmTools)
		if err != nil {
			return "", fmt.Errorf("failed to call LLM: %w", err)
		}

		if functionCall != nil {
			// 执行工具
			toolResult, err := a.toolManager.Run(ctx, functionCall.Name, functionCall.Arguments)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}

			// 将工具执行结果添加到对话历史
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("我需要使用%s工具来解决这个问题。", functionCall.Name),
			})
			messages = append(messages, llm.Message{
				Role:    "tool",
				Content: toolResult,
			})

			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, functionCall.Name, toolResult, 3)
			}
		} else {
			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
				a.memoryManager.AddLongTermMemory(sessionID, "ai_response", response, 2)
			}

			// 直接返回答案
			return response, nil
		}
	}

	return "处理超时，请尝试简化问题或提供更多信息。", nil
}

// RunStream 流式运行Agent
func (a *Agent) RunStream(ctx context.Context, sessionID, userInput string, callback func(chunk string) error) error {
	// 初始化对话历史
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "你是一个AI助手，能够通过思考和使用工具来解决问题。请按照ReAct模式思考：先分析问题，然后决定是否需要使用工具，最后给出答案。",
		},
	}

	// 添加记忆摘要（如果有）
	if a.memoryManager != nil {
		summary := a.memoryManager.SummarizeMemory(sessionID)
		if summary != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: summary,
			})
		}
	}

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userInput,
	})

	// ReAct循环
	for step := 0; step < a.maxSteps; step++ {
		// 准备工具列表
		toolsList := a.toolManager.List()
		llmTools := make([]llm.Tool, len(toolsList))
		for i, tool := range toolsList {
			params := tool.Parameters()
			// 转换参数类型
			llmParams := make(map[string]llm.Property)
			for k, v := range params {
				llmParams[k] = llm.Property{
					Type:        v.Type,
					Description: v.Description,
				}
			}
			llmTools[i] = llm.Tool{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters: llm.ToolParameters{
					Type:       "object",
					Properties: llmParams,
					Required:   tool.Required(),
				},
			}
		}

		// 发送请求
		response, functionCall, err := a.llm.FunctionCall(ctx, messages, llmTools)
		if err != nil {
			return fmt.Errorf("failed to call LLM: %w", err)
		}

		if functionCall != nil {
			// 执行工具
			toolResult, err := a.toolManager.Run(ctx, functionCall.Name, functionCall.Arguments)
			if err != nil {
				toolResult = fmt.Sprintf("Error: %v", err)
			}

			// 将工具执行结果添加到对话历史
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("我需要使用%s工具来解决这个问题。", functionCall.Name),
			})
			messages = append(messages, llm.Message{
				Role:    "tool",
				Content: toolResult,
			})

			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, functionCall.Name, toolResult, 3)
			}

			// 通知用户正在使用工具
			if err := callback(fmt.Sprintf("正在使用 %s 工具...\n", functionCall.Name)); err != nil {
				return err
			}
		} else {
			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
				a.memoryManager.AddLongTermMemory(sessionID, "ai_response", response, 2)
			}

			// 直接返回答案
			return callback(response)
		}
	}

	if err := callback("处理超时，请尝试简化问题或提供更多信息。"); err != nil {
		return err
	}

	return nil
}
