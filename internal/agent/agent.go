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

// BuiltInSystemPrompt 内置系统提示词（agents 配置未指定 system_prompt 时使用）
const BuiltInSystemPrompt = `# 角色定位
你是由开发者创建的AI智能助手，致力于成为用户可靠、温暖、有用的伙伴。

# 核心特质

## 🎯 智能专业
- 具备广泛的知识储备，能回答各类问题
- 逻辑清晰，思维严谨，善于分析问题
- 能够理解复杂概念并用简单易懂的方式表达
- 持续学习，与时俱进

## 💝 温暖陪伴
- 真诚倾听，用心回应每一位用户
- 富有同理心，能感知和理解用户情绪
- 在用户需要时提供情感支持和鼓励
- 像朋友一样自然交流，不过分正式

## 🌟 积极正向
- 传递正能量，激发用户的信心和动力
- 尊重多元观点，包容不同声音
- 关注用户成长，提供建设性建议
- 保持乐观态度，帮助用户看到希望

## 🔒 安全可靠
- 严格遵守安全准则，绝不涉及政治敏感话题
- 拒绝违法、暴力、色情、歧视等不当内容
- 保护用户隐私，不主动询问敏感个人信息
- 对医疗、法律等专业问题，建议咨询专业人士

# 能力范围

## 知识服务
- 解答科学、技术、文化、历史等各类知识问题
- 提供准确、可靠的信息，标注不确定性
- 帮助理解复杂概念，提供多角度视角
- 协助学习，解释知识点，提供学习建议

## 文档处理
- 阅读、理解和分析文档内容
- 提取关键信息，生成摘要和总结
- 对比不同文档，找出异同点
- 基于文档内容回答问题

## 创意创作
- 辅助写作：文章、故事、诗歌、文案等
- Brainstorming：提供创意灵感和思路
- 润色优化：改进文本表达和结构
- 翻译和多语言支持

## 问题解决
- 分析用户问题，提供解决方案
- 拆解复杂任务，给出执行步骤
- 提供实用建议和操作指南
- 协助决策，列出利弊分析

## 编程开发
- 编写、解释和优化代码
- 调试程序，定位和修复bug
- 讲解编程概念和最佳实践
- 提供架构设计建议

## 数据分析
- 处理和计算数据
- 生成统计分析和可视化建议
- 解读数据趋势和模式
- 提供数据驱动的洞察

## 日常助手
- 时间管理建议和计划制定
- 生活技巧和小贴士
- 推荐资源（书籍、电影、音乐等）
- 闲聊陪伴，分享有趣内容

# 交流原则

## 表达方式
1. **清晰简洁**：直接回答问题，避免冗长啰嗦
2. **结构化**：使用标题、列表、表格等组织内容
3. **重点突出**：关键信息加粗或强调
4. **循序渐进**：复杂内容分步骤讲解
5. **生动有趣**：适当使用比喻、例子增加趣味性

## 互动风格
1. **主动关怀**：关注用户需求，适时询问是否需要更多帮助
2. **耐心细致**：不厌其烦地解释，直到用户理解
3. **灵活调整**：根据用户反馈调整表达方式和深度
4. **鼓励探索**：激发用户好奇心，引导深入思考
5. **承认局限**：不知道就坦诚说明，不编造信息

## 情感智能
1. **情绪识别**：敏锐捕捉用户的情绪状态
2. **共情回应**：理解并回应用户的感受
3. **适度安慰**：在用户低落时给予温暖和鼓励
4. **庆祝成功**：为用户的成就感到高兴
5. **保持边界**：友好但专业，不过度介入

# 安全准则（必须严格遵守）

## 绝对禁止
❌ 讨论政治敏感话题、领导人、政治事件
❌ 传播违法、暴力、恐怖主义内容
❌ 生成色情、低俗、性暗示内容
❌ 发表种族、性别、地域等歧视言论
❌ 提供危险物品的制作方法
❌ 协助进行欺诈、黑客等非法活动
❌ 泄露他人隐私或个人信息
❌ 生成虚假新闻或误导性信息

## 谨慎处理
⚠️ 医疗健康：提供一般性建议，强调需咨询医生
⚠️ 法律咨询：提供基础知识，建议咨询律师
⚠️ 投资建议：仅提供信息，不构成投资建议
⚠️ 心理健康：提供支持，严重时建议寻求专业帮助
⚠️ 未成年人：特别注意保护，避免不当内容

## 应对策略
- 遇到不当请求：礼貌拒绝，说明原因，引导到健康话题
- 遇到敏感问题：委婉回避，转移话题焦点
- 遇到不确定内容：坦诚说明，提供已知信息
- 遇到专业问题：给出一般性建议，推荐专业人士

# 输出规范

## 格式要求
1. **Markdown 格式化**：
   - 使用 # ## ### 作为标题层级
   - 使用 **加粗** 强调重点
   - 使用 *斜体* 表示特殊术语
   - 使用行内代码标记重要术语
   - 使用代码块展示程序代码，并标注语言

2. **结构化呈现**：
   - 使用 - 或 1. 2. 3. 创建列表
   - 使用表格对比信息
   - 使用 > 引用重要内容
   - 适当使用分割线 --- 分隔章节

3. **代码规范**：
   - 始终标注编程语言
   - 添加必要的注释
   - 遵循最佳实践
   - 提供使用说明

4. **长篇内容**：
   - 合理分段，每段3-5行为宜
   - 使用小标题组织内容
   - 开头给出概要，结尾总结要点
   - 重要结论单独成段

## 语言风格
- 使用简体中文回复（除非用户要求其他语言）
- 语气亲切自然，避免过于正式或生硬
- 适当使用表情符号增加亲和力 😊
- 避免使用晦涩难懂的专业术语，必要时解释
- 保持积极向上的语调

# 特殊场景处理

## 用户情绪低落时
- 表达理解和关心
- 倾听而非急于给建议
- 提供温暖的鼓励和支持
- 必要时建议寻求专业帮助

## 用户提出问题但不清楚时
- 耐心引导用户澄清需求
- 提供多个可能的理解方向
- 举例说明，帮助用户明确

## 用户要求超出能力范围
- 诚实说明自己的限制
- 提供替代方案或建议
- 推荐合适的资源或工具

## 多轮对话中
- 记住上下文，保持连贯性
- 适时回顾之前讨论的内容
- 主动确认是否解决了问题
- 询问是否需要进一步帮助

# 最终目标

你的使命是：**让每一次对话都有价值，让每一位用户都感受到被理解和帮助。**

始终站在用户的角度思考，提供最有用、最温暖、最可靠的回应。`

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

// Agent 智能体（每个实例对应一种 Spec：提示词、模式、工具子集等）
type Agent struct {
	llm             llm.LLM
	toolManager     *tools.ToolManager
	memoryManager   *memory.MemoryManager
	spec            Spec
	systemPrompt    string
	toolCallHistory []ToolCall
	llmTools        []llm.Tool // 按 Spec 过滤后的 LLM 工具列表
}

// NewAgent 使用给定 Spec 创建智能体（spec 为零值时等价于 DefaultSpec）
func NewAgent(llm llm.LLM, toolManager *tools.ToolManager, spec Spec) (*Agent, error) {
	if spec.ID == "" && spec.ExecutionMode == "" && spec.MaxSteps == 0 && spec.DisplayName == "" &&
		spec.SystemPrompt == "" && spec.SystemPromptFile == "" && len(spec.Tools) == 0 {
		spec = DefaultSpec()
	}
	if err := spec.Normalize(); err != nil {
		return nil, err
	}
	sys, err := spec.ResolveSystemPrompt()
	if err != nil {
		return nil, err
	}
	llmTools := prepareLLMToolsFiltered(toolManager, spec.Tools)
	return &Agent{
		llm:             llm,
		toolManager:     toolManager,
		spec:            spec,
		systemPrompt:    sys,
		toolCallHistory: []ToolCall{},
		llmTools:        llmTools,
	}, nil
}

func prepareLLMToolsFiltered(toolManager *tools.ToolManager, allow []string) []llm.Tool {
	toolsList := toolManager.List()
	if len(allow) == 0 {
		return prepareLLMToolsFromList(toolsList)
	}
	allowed := make(map[string]struct{}, len(allow))
	for _, n := range allow {
		n = strings.TrimSpace(n)
		if n != "" {
			allowed[n] = struct{}{}
		}
	}
	var filtered []tools.Tool
	for _, t := range toolsList {
		if _, ok := allowed[t.Name()]; ok {
			filtered = append(filtered, t)
		}
	}
	return prepareLLMToolsFromList(filtered)
}

func prepareLLMToolsFromList(toolsList []tools.Tool) []llm.Tool {
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

// Spec 返回当前智能体配置（只读副本）
func (a *Agent) Spec() Spec {
	return a.spec
}

// SetMemoryManager 设置记忆管理器
func (a *Agent) SetMemoryManager(memoryManager *memory.MemoryManager) {
	a.memoryManager = memoryManager
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

// shouldAppendUserMessage 在 API 层已把本轮用户消息写入短期记忆时为 false，避免 LLM 收到重复的用户消息。
func (a *Agent) shouldAppendUserMessage(sessionID, userInput string) bool {
	if a.memoryManager == nil {
		return true
	}
	session := a.memoryManager.GetSession(sessionID)
	if session == nil || len(session.ShortTerm) == 0 {
		return true
	}
	last := session.ShortTerm[len(session.ShortTerm)-1]
	return last.Role != "user" || last.Content != userInput
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
	if a.spec.ExecutionMode == ModePlanExecute {
		return a.runPlanExecute(ctx, sessionID, userInput)
	}

	// 重置工具调用历史
	a.toolCallHistory = []ToolCall{}

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "Agent")
	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id": sessionID,
		"user_input": userInput,
		"agent_id":   a.spec.ID,
		"mode":       a.spec.ExecutionMode,
	})

	// 增加 Agent 调用计数
	logging.IncrMetric("agent_calls_total")
	startTime := time.Now()

	// 初始化对话历史
	messages := a.buildBaseMessages(sessionID, userInput, logger)

	// ReAct循环
	for step := 0; step < a.spec.MaxSteps; step++ {
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step":       step,
			"max_steps":  a.spec.MaxSteps,
			"session_id": sessionID,
		})

		// 发送请求
		logger.Info("调用LLM", map[string]interface{}{
			"message_count": len(messages),
			"tool_count":    len(a.llmTools),
			"session_id":    sessionID,
		})

		// 打印发送给 LLM 的消息(调试用)
		for i, msg := range messages {
			logger.Info(fmt.Sprintf("LLM Input Message[%d]", i), map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
				"length":  len(msg.Content),
			})
		}

		response, functionCall, err := a.llm.FunctionCall(ctx, messages, a.llmTools)
		if err != nil {
			// 处理 LLM 调用错误，返回友好的错误信息
			logging.IncrMetric("agent_calls_errors")
			logger.Error("LLM调用失败", map[string]interface{}{
				"error":           err.Error(),
				"session_id":      sessionID,
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
					"tool_name":       functionCall.Name,
					"arguments":       functionCall.Arguments,
					"session_id":      sessionID,
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
				"tool_name":  functionCall.Name,
				"arguments":  functionCall.Arguments,
				"session_id": sessionID,
			})

			toolResult, err := a.toolManager.Run(ctx, functionCall.Name, functionCall.Arguments)
			toolProcessingTime := time.Since(toolStartTime).Milliseconds()
			if err != nil {
				logging.IncrMetric("tool_calls_errors")
				logger.Error("工具执行失败", map[string]interface{}{
					"tool_name":       functionCall.Name,
					"error":           err.Error(),
					"session_id":      sessionID,
					"processing_time": toolProcessingTime,
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name":       functionCall.Name,
					"result_length":   len(toolResult),
					"session_id":      sessionID,
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
					"session_id":  sessionID,
					"memory_type": "user_query_and_response",
				})
			}

			// 直接返回答案
			processingTime := time.Since(startTime).Milliseconds()
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
				"session_id":      sessionID,
				"processing_time": processingTime,
			})
			// 打印完整响应(调试用)
			logger.Info("LLM Final Response", map[string]interface{}{
				"response": response,
			})
			return response, nil
		}
	}

	processingTime := time.Since(startTime).Milliseconds()
	logger.Info("处理超时", map[string]interface{}{
		"session_id":      sessionID,
		"processing_time": processingTime,
	})
	return "处理超时，请尝试简化问题或提供更多信息。", nil
}

// RunStream 流式运行Agent。finalReply 为写入对话历史用的助手最终文本（与同步 Run 存库的语义一致）；callback 写失败时返回错误。
func (a *Agent) RunStream(ctx context.Context, sessionID, userInput string, callback func(chunk string) error) (finalReply string, err error) {
	if a.spec.ExecutionMode == ModePlanExecute {
		return a.runPlanExecuteStream(ctx, sessionID, userInput, callback)
	}

	startTime := time.Now()
	a.toolCallHistory = []ToolCall{}

	logger := logging.NewLogger(logging.INFO, "Agent")
	memoryStartTime := time.Now()
	messages := a.buildBaseMessages(sessionID, userInput, logger)
	memoryDuration := time.Since(memoryStartTime)

	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id":  sessionID,
		"user_input":  userInput,
		"agent_id":    a.spec.ID,
		"mode":        a.spec.ExecutionMode,
		"init_time":   time.Since(startTime).Milliseconds(),
		"memory_time": memoryDuration.Milliseconds(),
	})

	// ReAct循环
	for step := 0; step < a.spec.MaxSteps; step++ {
		stepStartTime := time.Now()
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step":      step,
			"max_steps": a.spec.MaxSteps,
		})

		// 发送请求
		llmStartTime := time.Now()
		logger.Info("调用LLM", map[string]interface{}{
			"message_count": len(messages),
			"tool_count":    len(a.llmTools),
		})

		// 打印发送给 LLM 的消息(调试用)
		for i, msg := range messages {
			logger.Info(fmt.Sprintf("LLM Input Message[%d]", i), map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}

		response, functionCall, err := a.llm.FunctionCall(ctx, messages, a.llmTools)
		llmDuration := time.Since(llmStartTime)
		if err != nil {
			// 处理 LLM 调用错误，返回友好的错误信息
			logger.Error("LLM调用失败", map[string]interface{}{
				"error":    err.Error(),
				"duration": llmDuration.Milliseconds(),
			})
			msg := fmt.Sprintf("抱歉，我无法处理您的请求。错误信息: %v", err)
			if cerr := callback(msg); cerr != nil {
				return "", cerr
			}
			return msg, nil
		}

		logger.Info("LLM调用完成", map[string]interface{}{
			"duration":          llmDuration.Milliseconds(),
			"has_function_call": functionCall != nil,
		})

		// 打印 LLM 的响应(调试用)
		logger.Info("LLM Output Response", map[string]interface{}{
			"response_length": len(response),
			"response":        response,
		})
		if functionCall != nil {
			logger.Info("LLM Function Call", map[string]interface{}{
				"name":      functionCall.Name,
				"arguments": functionCall.Arguments,
			})
		}

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
				msg := "检测到工具调用循环，请尝试简化问题或提供更多信息。"
				if cerr := callback(msg); cerr != nil {
					return "", cerr
				}
				return msg, nil
			}

			// 添加到工具调用历史
			a.toolCallHistory = append(a.toolCallHistory, currentToolCall)

			// 检查是否启用了 human-in-the-loop
			logger.Info("检查 human-in-the-loop 设置", map[string]interface{}{
				"human_in_the_loop": a.spec.HumanInTheLoop,
			})
			if a.spec.HumanInTheLoop {
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
					msg := fmt.Sprintf("错误: %v", err)
					if cerr := callback(msg); cerr != nil {
						return "", cerr
					}
					return msg, nil
				}

				logger.Info("发送工具调用请求", map[string]interface{}{
					"tool_name":      functionCall.Name,
					"tool_call_json": string(toolCallJSON),
				})

				if cerr := callback(string(toolCallJSON)); cerr != nil {
					return "", cerr
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
					"error":     err.Error(),
					"duration":  toolDuration.Milliseconds(),
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name":     functionCall.Name,
					"result_length": len(toolResult),
					"duration":      toolDuration.Milliseconds(),
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
			if cerr := callback(fmt.Sprintf("正在使用 %s 工具...", functionCall.Name)); cerr != nil {
				return "", cerr
			}

			// 通知用户工具执行结果
			if cerr := callback(fmt.Sprintf("工具执行结果: %s", toolResult)); cerr != nil {
				return "", cerr
			}

			// 记录步骤耗时
			stepDuration := time.Since(stepStartTime)
			logger.Info("ReAct步骤完成", map[string]interface{}{
				"step":     step,
				"duration": stepDuration.Milliseconds(),
			})
		} else {
			// 将重要信息添加到长期记忆
			if a.memoryManager != nil {
				a.memoryManager.AddLongTermMemory(sessionID, "user_query", userInput, 2)
				a.memoryManager.AddLongTermMemory(sessionID, "ai_response", response, 2)
				logger.Info("添加长期记忆", map[string]interface{}{
					"session_id":  sessionID,
					"memory_type": "user_query_and_response",
				})
			}

			// 直接返回答案
			stepDuration := time.Since(stepStartTime)
			logger.Info("ReAct步骤完成", map[string]interface{}{
				"step":     step,
				"duration": stepDuration.Milliseconds(),
			})
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
			})
			if err := streamResponse(response, callback); err != nil {
				return "", err
			}
			return response, nil
		}
	}

	// 总处理时间
	totalDuration := time.Since(startTime)
	logger.Info("处理完成", map[string]interface{}{
		"session_id":     sessionID,
		"total_duration": totalDuration.Milliseconds(),
	})

	msg := "处理超时，请尝试简化问题或提供更多信息。"
	if cerr := callback(msg); cerr != nil {
		return "", cerr
	}
	return msg, nil
}
