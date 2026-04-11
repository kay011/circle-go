package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"circle-go/internal/llm"
	"circle-go/internal/logging"
	"circle-go/internal/memory"
	"circle-go/internal/tools"
)

// ToolCallRequest 工具调用请求
type ToolCallRequest struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
	Reasoning string                 `json:"reasoning"`
}

// ToolCallResponse 工具调用响应
type ToolCallResponse struct {
	Approved bool `json:"approved"`
}

// ToolCall 工具调用记录
type ToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// Agent 智能体
type Agent struct {
	llm            llm.LLM
	toolManager    *tools.ToolManager
	memoryManager  *memory.MemoryManager
	maxSteps       int
	toolCallHistory []ToolCall
	humanInTheLoop bool
	llmTools       []llm.Tool // 预先转换好的 LLM 工具列表
}

// NewAgent 创建Agent实例
func NewAgent(llm llm.LLM, toolManager *tools.ToolManager, maxSteps int, humanInTheLoop bool) *Agent {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	// 预先转换工具列表为 LLM 工具格式
	llmTools := prepareLLMTools(toolManager)

	return &Agent{
		llm:         llm,
		toolManager: toolManager,
		maxSteps:    maxSteps,
		toolCallHistory: []ToolCall{},
		humanInTheLoop: humanInTheLoop,
		llmTools:    llmTools,
	}
}

// prepareLLMTools 准备 LLM 工具列表
func prepareLLMTools(toolManager *tools.ToolManager) []llm.Tool {
	toolsList := toolManager.List()
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
	return llmTools
}

// SetMemoryManager 设置记忆管理器
func (a *Agent) SetMemoryManager(memoryManager *memory.MemoryManager) {
	a.memoryManager = memoryManager
}

// SetHumanInTheLoop 设置是否启用 human-in-the-loop
func (a *Agent) SetHumanInTheLoop(humanInTheLoop bool) {
	a.humanInTheLoop = humanInTheLoop
}

// isToolCallDuplicate 检查工具调用是否重复
func (a *Agent) isToolCallDuplicate(toolCall ToolCall) bool {
	for _, previousCall := range a.toolCallHistory {
		if previousCall.Name == toolCall.Name {
			// 检查参数是否相同
			if len(previousCall.Arguments) == len(toolCall.Arguments) {
				allMatch := true
				for key, value := range previousCall.Arguments {
					if toolCall.Arguments[key] != value {
						allMatch = false
						break
					}
				}
				if allMatch {
					return true
				}
			}
		}
	}
	return false
}

// isSimpleQuery 检查是否是简单查询
func isSimpleQuery(query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	simpleQueries := []string{
		"你好", "您好", "hi", "hello", "hey",
		"再见", "bye", "goodbye",
		"谢谢", "thanks", "thank you",
		"你是谁", "你是什么", "who are you", "what are you",
		"你来自哪里", "where are you from",
		"今天是星期几", "今天几号", "现在几点",
	}
	for _, q := range simpleQueries {
		if strings.Contains(query, q) {
			return true
		}
	}
	return false
}

// getQuickResponse 获取快速响应
func getQuickResponse(query string) string {
	query = strings.ToLower(strings.TrimSpace(query))
	
	// 问候语
	if strings.Contains(query, "你好") || strings.Contains(query, "您好") || strings.Contains(query, "hi") || strings.Contains(query, "hello") || strings.Contains(query, "hey") {
		return "你好！我是您的AI助手，有什么我可以帮您的吗？"
	}
	
	// 告别语
	if strings.Contains(query, "再见") || strings.Contains(query, "bye") || strings.Contains(query, "goodbye") {
		return "再见！祝您有愉快的一天！"
	}
	
	// 感谢语
	if strings.Contains(query, "谢谢") || strings.Contains(query, "thanks") || strings.Contains(query, "thank you") {
		return "不客气！很高兴能帮到您。"
	}
	
	// 身份问题
	if strings.Contains(query, "你是谁") || strings.Contains(query, "你是什么") || strings.Contains(query, "who are you") || strings.Contains(query, "what are you") {
		return "我是一个AI助手，能够通过思考和使用工具来解决您的问题。"
	}
	
	// 来源问题
	if strings.Contains(query, "你来自哪里") || strings.Contains(query, "where are you from") {
		return "我是由开发者创建的AI助手，随时为您服务。"
	}
	
	// 时间问题
	if strings.Contains(query, "今天是星期几") {
		weekdays := []string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}
		return fmt.Sprintf("今天是%s。", weekdays[time.Now().Weekday()])
	}
	
	if strings.Contains(query, "今天几号") {
		return fmt.Sprintf("今天是%s。", time.Now().Format("2006年1月2日"))
	}
	
	if strings.Contains(query, "现在几点") {
		return fmt.Sprintf("现在是%s。", time.Now().Format("15:04:05"))
	}
	
	return ""
}

// streamResponse 流式输出响应
func streamResponse(response string, callback func(chunk string) error) error {
	// 一次性发送完整响应
	if err := callback(response); err != nil {
		return err
	}
	return nil
}

// Run 运行Agent
func (a *Agent) Run(ctx context.Context, sessionID, userInput string) (string, error) {
	// 重置工具调用历史
	a.toolCallHistory = []ToolCall{}

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "Agent")
	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id": sessionID,
		"user_input": userInput,
	})

	// 增加 Agent 调用计数
	logging.IncrMetric("agent_calls_total")
	startTime := time.Now()

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
			logger.Info("添加记忆摘要", map[string]interface{}{
				"session_id": sessionID,
				"summary_length": len(summary),
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
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step": step,
			"max_steps": a.maxSteps,
			"session_id": sessionID,
		})

		// 发送请求
		logger.Info("调用LLM", map[string]interface{}{
			"message_count": len(messages),
			"tool_count": len(a.llmTools),
			"session_id": sessionID,
		})

		response, functionCall, err := a.llm.FunctionCall(ctx, messages, a.llmTools)
		if err != nil {
			// 处理 LLM 调用错误，返回友好的错误信息
			logging.IncrMetric("agent_calls_errors")
			logger.Error("LLM调用失败", map[string]interface{}{
				"error": err.Error(),
				"session_id": sessionID,
				"processing_time": time.Since(startTime).Milliseconds(),
			})
			return fmt.Sprintf("抱歉，我无法处理您的请求。错误信息: %v", err), nil
		}

		if functionCall != nil {
			// 检查工具调用是否重复
			currentToolCall := ToolCall{
				Name:      functionCall.Name,
				Arguments: functionCall.Arguments,
			}
			if a.isToolCallDuplicate(currentToolCall) {
				// 检测到工具调用循环，返回错误信息
				logging.IncrMetric("agent_calls_errors")
				logging.IncrMetric("tool_calls_duplicate")
				logger.Error("检测到工具调用循环", map[string]interface{}{
					"tool_name": functionCall.Name,
					"arguments": functionCall.Arguments,
					"session_id": sessionID,
					"processing_time": time.Since(startTime).Milliseconds(),
				})
				return "检测到工具调用循环，请尝试简化问题或提供更多信息。", nil
			}

			// 添加到工具调用历史
			a.toolCallHistory = append(a.toolCallHistory, currentToolCall)

			// 执行工具
			logging.IncrMetric("tool_calls_total")
			toolStartTime := time.Now()
			logger.Info("执行工具", map[string]interface{}{
				"tool_name": functionCall.Name,
				"arguments": functionCall.Arguments,
				"session_id": sessionID,
			})

			toolResult, err := a.toolManager.Run(ctx, functionCall.Name, functionCall.Arguments)
			toolProcessingTime := time.Since(toolStartTime).Milliseconds()
			if err != nil {
				logging.IncrMetric("tool_calls_errors")
				logger.Error("工具执行失败", map[string]interface{}{
					"tool_name": functionCall.Name,
					"error": err.Error(),
					"session_id": sessionID,
					"processing_time": toolProcessingTime,
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name": functionCall.Name,
					"result_length": len(toolResult),
					"session_id": sessionID,
					"processing_time": toolProcessingTime,
				})
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
				logger.Info("添加长期记忆", map[string]interface{}{
					"session_id": sessionID,
					"memory_key": functionCall.Name,
				})
			}
		} else {
			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
				a.memoryManager.AddLongTermMemory(sessionID, "ai_response", response, 2)
				logger.Info("添加长期记忆", map[string]interface{}{
					"session_id": sessionID,
					"memory_type": "user_query_and_response",
				})
			}

			// 直接返回答案
			processingTime := time.Since(startTime).Milliseconds()
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
				"session_id": sessionID,
				"processing_time": processingTime,
			})
			return response, nil
		}
	}

	processingTime := time.Since(startTime).Milliseconds()
	logger.Info("处理超时", map[string]interface{}{
		"session_id": sessionID,
		"processing_time": processingTime,
	})
	return "处理超时，请尝试简化问题或提供更多信息。", nil
}

// RunStream 流式运行Agent
func (a *Agent) RunStream(ctx context.Context, sessionID, userInput string, callback func(chunk string) error) error {
	// 开始时间
	startTime := time.Now()

	// 重置工具调用历史
	a.toolCallHistory = []ToolCall{}

	// 初始化对话历史
	messages := []llm.Message{
		{
			Role:    "system",
			Content: "你是一个AI助手，能够通过思考和使用工具来解决问题。请按照ReAct模式思考：先分析问题，然后决定是否需要使用工具，最后给出答案。",
		},
	}

	// 添加记忆摘要（如果有）
	memoryStartTime := time.Now()
	if a.memoryManager != nil {
		summary := a.memoryManager.SummarizeMemory(sessionID)
		if summary != "" {
			messages = append(messages, llm.Message{
				Role:    "system",
				Content: summary,
			})
		}
	}
	memoryDuration := time.Since(memoryStartTime)

	// 添加用户输入
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: userInput,
	})

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "Agent")
	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id": sessionID,
		"user_input": userInput,
		"init_time": time.Since(startTime).Milliseconds(),
		"memory_time": memoryDuration.Milliseconds(),
	})

	// 检查是否是简单问题，直接使用快速响应
	if isSimpleQuery(userInput) {
		logger.Info("使用快速响应", map[string]interface{}{
			"user_input": userInput,
		})
		response := getQuickResponse(userInput)
		if response != "" {
			logger.Info("返回快速响应", map[string]interface{}{
				"response": response,
				"duration": time.Since(startTime).Milliseconds(),
			})
			// 流式输出响应
			return streamResponse(response, callback)
		}
	}

	// ReAct循环
	for step := 0; step < a.maxSteps; step++ {
		stepStartTime := time.Now()
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step": step,
			"max_steps": a.maxSteps,
		})

		// 发送请求
		llmStartTime := time.Now()
		logger.Info("调用LLM", map[string]interface{}{
			"message_count": len(messages),
			"tool_count": len(a.llmTools),
		})

		response, functionCall, err := a.llm.FunctionCall(ctx, messages, a.llmTools)
		llmDuration := time.Since(llmStartTime)
		if err != nil {
			// 处理 LLM 调用错误，返回友好的错误信息
			logger.Error("LLM调用失败", map[string]interface{}{
				"error": err.Error(),
				"duration": llmDuration.Milliseconds(),
			})
			if err := callback(fmt.Sprintf("抱歉，我无法处理您的请求。错误信息: %v", err)); err != nil {
				return err
			}
			return nil
		}

		logger.Info("LLM调用完成", map[string]interface{}{
			"duration": llmDuration.Milliseconds(),
			"has_function_call": functionCall != nil,
		})

		if functionCall != nil {
			// 检查工具调用是否重复
			currentToolCall := ToolCall{
				Name:      functionCall.Name,
				Arguments: functionCall.Arguments,
			}
			if a.isToolCallDuplicate(currentToolCall) {
				// 检测到工具调用循环，返回错误信息
				logger.Error("检测到工具调用循环", map[string]interface{}{
					"tool_name": functionCall.Name,
					"arguments": functionCall.Arguments,
				})
				if err := callback("检测到工具调用循环，请尝试简化问题或提供更多信息。"); err != nil {
					return err
				}
				return nil
			}

			// 添加到工具调用历史
			a.toolCallHistory = append(a.toolCallHistory, currentToolCall)

			// 检查是否启用了 human-in-the-loop
			logger.Info("检查 human-in-the-loop 设置", map[string]interface{}{
				"human_in_the_loop": a.humanInTheLoop,
			})
			if a.humanInTheLoop {
				// 生成工具调用请求
				toolCallRequest := ToolCallRequest{
					ID:        fmt.Sprintf("toolcall_%d", time.Now().UnixNano()),
					Name:      functionCall.Name,
					Arguments: functionCall.Arguments,
					Reasoning: response, // 使用 LLM 的响应作为调用原因
				}

				// 发送工具调用请求给前端
				toolCallJSON, err := json.Marshal(map[string]interface{}{
					"tool_call": toolCallRequest,
				})
				if err != nil {
					logger.Error("序列化工具调用请求失败", map[string]interface{}{
						"error": err.Error(),
					})
					if err := callback(fmt.Sprintf("错误: %v", err)); err != nil {
						return err
					}
					return nil
				}

				logger.Info("发送工具调用请求", map[string]interface{}{
					"tool_name": functionCall.Name,
					"tool_call_json": string(toolCallJSON),
				})

				if err := callback(string(toolCallJSON)); err != nil {
					return err
				}

				// 这里应该等待用户确认，实际实现需要更复杂的逻辑
				// 目前我们假设用户总是批准工具调用
			}

			// 执行工具
			toolStartTime := time.Now()
			logger.Info("执行工具", map[string]interface{}{
				"tool_name": functionCall.Name,
				"arguments": functionCall.Arguments,
			})

			toolResult, err := a.toolManager.Run(ctx, functionCall.Name, functionCall.Arguments)
			toolDuration := time.Since(toolStartTime)
			if err != nil {
				logger.Error("工具执行失败", map[string]interface{}{
					"tool_name": functionCall.Name,
					"error": err.Error(),
					"duration": toolDuration.Milliseconds(),
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name": functionCall.Name,
					"result_length": len(toolResult),
					"duration": toolDuration.Milliseconds(),
				})
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
				logger.Info("添加长期记忆", map[string]interface{}{
					"session_id": sessionID,
					"memory_key": functionCall.Name,
				})
			}

			// 通知用户正在使用工具
			if err := callback(fmt.Sprintf("正在使用 %s 工具...", functionCall.Name)); err != nil {
				return err
			}

			// 通知用户工具执行结果
			if err := callback(fmt.Sprintf("工具执行结果: %s", toolResult)); err != nil {
				return err
			}

			// 记录步骤耗时
			stepDuration := time.Since(stepStartTime)
			logger.Info("ReAct步骤完成", map[string]interface{}{
				"step": step,
				"duration": stepDuration.Milliseconds(),
			})
		} else {
			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
				a.memoryManager.AddLongTermMemory(sessionID, "ai_response", response, 2)
				logger.Info("添加长期记忆", map[string]interface{}{
					"session_id": sessionID,
					"memory_type": "user_query_and_response",
				})
			}

			// 直接返回答案
			stepDuration := time.Since(stepStartTime)
			logger.Info("ReAct步骤完成", map[string]interface{}{
				"step": step,
				"duration": stepDuration.Milliseconds(),
			})
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
			})
			// 流式输出响应
			return streamResponse(response, callback)
		}
	}

	// 总处理时间
	totalDuration := time.Since(startTime)
	logger.Info("处理完成", map[string]interface{}{
		"session_id": sessionID,
		"total_duration": totalDuration.Milliseconds(),
	})

	if err := callback("处理超时，请尝试简化问题或提供更多信息。"); err != nil {
		return err
	}

	return nil
}
