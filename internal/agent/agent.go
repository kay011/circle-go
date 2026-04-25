package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"circle-go/internal/llm"
	"circle-go/internal/logging"
	"circle-go/internal/memory"
	"circle-go/internal/planner"
	agentruntime "circle-go/internal/runtime"
	"circle-go/internal/tools"
)

// SystemPrompt 系统提示词
const SystemPrompt = `# 角色定位
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

// Agent 智能体（支持任务规划和自我反思）
type Agent struct {
	llm                 llm.LLM
	toolManager         *tools.ToolManager
	toolGateway         *tools.ToolGateway
	policyEngine        tools.PolicyEngine
	memoryManager       *memory.MemoryManager
	taskPlanner         *planner.Planner
	reflector           *Reflector
	maxSteps            int
	maxToolHistory      int // 最大工具调用历史记录数
	runtimeMaxToolCalls int
	runtimeMaxDuration  time.Duration
	toolCallHistory     []ToolCall
	humanInTheLoop      bool
	llmTools            []llm.Tool // 预先转换好的 LLM 工具列表
	pendingToolCalls    map[string]pendingToolCall
	pendingMu           sync.Mutex
	approvalTimeout     time.Duration
}

type pendingToolCall struct {
	sessionID string
	response  chan ToolCallResponse
}

// NewAgent 创建Agent实例
func NewAgent(llm llm.LLM, toolManager *tools.ToolManager, maxSteps int, humanInTheLoop bool) *Agent {
	if maxSteps <= 0 {
		maxSteps = 5
	}

	// 预先转换工具列表为 LLM 工具格式
	llmTools := prepareLLMTools(toolManager)
	toolGateway := tools.NewToolGateway(toolManager, 20*time.Second, nil)
	policyEngine := tools.NewDefaultPolicyEngine(nil)

	// 创建任务规划器
	taskPlanner := planner.NewPlanner(llm)

	// 创建反思器
	reflector := NewReflector(llm)

	return &Agent{
		llm:                 llm,
		toolManager:         toolManager,
		toolGateway:         toolGateway,
		policyEngine:        policyEngine,
		taskPlanner:         taskPlanner,
		reflector:           reflector,
		maxSteps:            maxSteps,
		maxToolHistory:      20,              // 最多保留20条工具调用历史
		runtimeMaxToolCalls: 20,              // 运行时工具调用预算
		runtimeMaxDuration:  2 * time.Minute, // 运行时总时长预算
		toolCallHistory:     []ToolCall{},
		humanInTheLoop:      humanInTheLoop,
		llmTools:            llmTools,
		pendingToolCalls:    make(map[string]pendingToolCall),
		approvalTimeout:     2 * time.Minute,
	}
}

// GetPlanner 获取任务规划器
func (a *Agent) GetPlanner() *planner.Planner {
	return a.taskPlanner
}

// GetReflector 获取反思器
func (a *Agent) GetReflector() *Reflector {
	return a.reflector
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

// SetRuntimeLimits 设置运行时预算（<=0 的值会被忽略）。
func (a *Agent) SetRuntimeLimits(maxSteps, maxToolCalls int, maxDuration time.Duration) {
	if maxSteps > 0 {
		a.maxSteps = maxSteps
	}
	if maxToolCalls > 0 {
		a.runtimeMaxToolCalls = maxToolCalls
	}
	if maxDuration > 0 {
		a.runtimeMaxDuration = maxDuration
	}
}

// SetToolGateway 设置自定义工具网关（用于注入审计/策略）。
func (a *Agent) SetToolGateway(gateway *tools.ToolGateway) {
	if gateway != nil {
		a.toolGateway = gateway
	}
}

// SetPolicyEngine 设置策略引擎。
func (a *Agent) SetPolicyEngine(engine tools.PolicyEngine) {
	if engine != nil {
		a.policyEngine = engine
	}
}

// ResolveToolCallApproval 处理外部的工具审批结果。
func (a *Agent) ResolveToolCallApproval(sessionID, toolCallID string, approved bool) error {
	a.pendingMu.Lock()
	pending, exists := a.pendingToolCalls[toolCallID]
	if !exists {
		a.pendingMu.Unlock()
		return errors.New("tool call not found or already resolved")
	}
	if pending.sessionID != sessionID {
		a.pendingMu.Unlock()
		return errors.New("session mismatch for tool call")
	}
	delete(a.pendingToolCalls, toolCallID)
	a.pendingMu.Unlock()

	pending.response <- ToolCallResponse{Approved: approved}
	return nil
}

func (a *Agent) createToolCallRequest(name string, arguments map[string]interface{}, reasoning string) ToolCallRequest {
	return ToolCallRequest{
		ID:        fmt.Sprintf("toolcall_%d", time.Now().UnixNano()),
		Name:      name,
		Arguments: arguments,
		Reasoning: reasoning,
	}
}

func (a *Agent) waitForToolApproval(ctx context.Context, sessionID string, req ToolCallRequest) (bool, error) {
	respCh := make(chan ToolCallResponse, 1)

	a.pendingMu.Lock()
	a.pendingToolCalls[req.ID] = pendingToolCall{
		sessionID: sessionID,
		response:  respCh,
	}
	a.pendingMu.Unlock()

	timeout := a.approvalTimeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		return resp.Approved, nil
	case <-ctx.Done():
		a.pendingMu.Lock()
		delete(a.pendingToolCalls, req.ID)
		a.pendingMu.Unlock()
		return false, ctx.Err()
	case <-timer.C:
		a.pendingMu.Lock()
		delete(a.pendingToolCalls, req.ID)
		a.pendingMu.Unlock()
		return false, errors.New("tool approval timeout")
	}
}

func (a *Agent) newRunContext(sessionID string) *agentruntime.RunContext {
	return agentruntime.NewRunContext(
		sessionID,
		agentruntime.WithMaxSteps(a.maxSteps),
		agentruntime.WithMaxToolCalls(a.runtimeMaxToolCalls),
		agentruntime.WithMaxDuration(a.runtimeMaxDuration),
	)
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
	// 重置工具调用历史
	a.toolCallHistory = []ToolCall{}

	runCtx := a.newRunContext(sessionID)
	runCtx.Enter(agentruntime.StateInit)

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "Agent")
	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id": sessionID,
		"user_input": userInput,
		"trace_id":   runCtx.TraceID,
	})

	// 增加 Agent 调用计数
	logging.IncrMetric("agent_calls_total")
	startTime := time.Now()

	// 初始化对话历史
	messages := []llm.Message{
		{
			Role:    "system",
			Content: SystemPrompt,
		},
	}

	runCtx.Enter(agentruntime.StatePlan)

	runCtx.Enter(agentruntime.StatePlan)

	// 添加完整的对话历史（如果有）
	if a.memoryManager != nil {
		session := a.memoryManager.GetSession(sessionID)
		if session != nil && len(session.ShortTerm) > 0 {
			// 添加短期记忆中的历史消息
			for _, msg := range session.ShortTerm {
				messages = append(messages, llm.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
			logger.Info("加载对话历史", map[string]interface{}{
				"session_id":    sessionID,
				"history_count": len(session.ShortTerm),
			})
		} else {
			// 如果没有短期记忆，尝试使用记忆摘要
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

	// ReAct循环
	for step := 0; ; step++ {
		if err := runCtx.ValidateBudget(); err != nil {
			logging.IncrMetric("agent_calls_errors")
			runCtx.Enter(agentruntime.StateFailed)
			logger.Warn("运行时预算超限", map[string]interface{}{
				"session_id":   sessionID,
				"trace_id":     runCtx.TraceID,
				"step":         runCtx.Stats.Steps,
				"tool_calls":   runCtx.Stats.ToolCalls,
				"elapsed_ms":   runCtx.Elapsed().Milliseconds(),
				"budget_cause": err.Error(),
			})
			return "处理超时，请尝试简化问题或提供更多信息。", nil
		}

		runCtx.IncStep()
		runCtx.Enter(agentruntime.StateExecute)
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step":       step,
			"max_steps":  a.maxSteps,
			"session_id": sessionID,
			"trace_id":   runCtx.TraceID,
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
				"trace_id":        runCtx.TraceID,
				"processing_time": time.Since(startTime).Milliseconds(),
			})
			runCtx.Enter(agentruntime.StateFailed)
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
					"trace_id":        runCtx.TraceID,
					"processing_time": time.Since(startTime).Milliseconds(),
				})
				runCtx.Enter(agentruntime.StateFailed)
				return "检测到工具调用循环，请尝试简化问题或提供更多信息。", nil
			}

			// 添加到工具调用历史，并限制历史记录数量
			a.toolCallHistory = append(a.toolCallHistory, currentToolCall)
			if len(a.toolCallHistory) > a.maxToolHistory {
				a.toolCallHistory = a.toolCallHistory[len(a.toolCallHistory)-a.maxToolHistory:]
			}

			// 策略评估：决定 allow / require_approval / deny
			policyResult := tools.PolicyResult{Decision: tools.PolicyAllow}
			if a.policyEngine != nil {
				policyResult = a.policyEngine.Evaluate(ctx, functionCall.Name, functionCall.Arguments)
			}
			if policyResult.Decision == tools.PolicyDeny {
				logging.IncrMetric("tool_calls_denied")
				logger.Warn("工具调用被策略拒绝", map[string]interface{}{
					"tool_name": functionCall.Name,
					"reason":    policyResult.Reason,
					"trace_id":  runCtx.TraceID,
				})
				return fmt.Sprintf("工具调用已被策略拒绝：%s", policyResult.Reason), nil
			}
			requireApproval := policyResult.Decision == tools.PolicyRequireApproval
			if requireApproval {
				return fmt.Sprintf("工具调用需要人工审批，请使用流式接口继续：%s", functionCall.Name), nil
			}

			// 执行工具
			logging.IncrMetric("tool_calls_total")
			if runCtx.Limits.MaxToolCalls > 0 && runCtx.Stats.ToolCalls >= runCtx.Limits.MaxToolCalls {
				logging.IncrMetric("agent_calls_errors")
				runCtx.Enter(agentruntime.StateFailed)
				logger.Warn("工具执行前预算超限", map[string]interface{}{
					"session_id":   sessionID,
					"trace_id":     runCtx.TraceID,
					"tool_name":    functionCall.Name,
					"step":         runCtx.Stats.Steps,
					"tool_calls":   runCtx.Stats.ToolCalls,
					"elapsed_ms":   runCtx.Elapsed().Milliseconds(),
					"budget_cause": agentruntime.ErrToolBudgetExceeded.Error(),
				})
				return "处理超时，请尝试简化问题或提供更多信息。", nil
			}
			runCtx.IncToolCall()

			toolStartTime := time.Now()
			logger.Info("执行工具", map[string]interface{}{
				"tool_name":  functionCall.Name,
				"arguments":  functionCall.Arguments,
				"session_id": sessionID,
				"trace_id":   runCtx.TraceID,
			})

			toolResult, err := a.toolGateway.Execute(ctx, functionCall.Name, functionCall.Arguments)
			toolProcessingTime := time.Since(toolStartTime).Milliseconds()
			if err != nil {
				logging.IncrMetric("tool_calls_errors")
				logger.Error("工具执行失败", map[string]interface{}{
					"tool_name":       functionCall.Name,
					"error":           err.Error(),
					"session_id":      sessionID,
					"trace_id":        runCtx.TraceID,
					"processing_time": toolProcessingTime,
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name":       functionCall.Name,
					"result_length":   len(toolResult),
					"session_id":      sessionID,
					"trace_id":        runCtx.TraceID,
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
			runCtx.Enter(agentruntime.StateVerify)
			runCtx.Enter(agentruntime.StateFinalize)
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
				"session_id":      sessionID,
				"trace_id":        runCtx.TraceID,
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
		"trace_id":        runCtx.TraceID,
		"processing_time": processingTime,
	})
	runCtx.Enter(agentruntime.StateFailed)
	return "处理超时，请尝试简化问题或提供更多信息。", nil
}

// RunStream 流式运行Agent。finalReply 为写入对话历史用的助手最终文本（与同步 Run 存库的语义一致）；callback 写失败时返回错误。
func (a *Agent) RunStream(ctx context.Context, sessionID, userInput string, callback func(chunk string) error) (finalReply string, err error) {
	// 开始时间
	startTime := time.Now()

	// 重置工具调用历史
	a.toolCallHistory = []ToolCall{}

	runCtx := a.newRunContext(sessionID)
	runCtx.Enter(agentruntime.StateInit)

	// 初始化对话历史
	messages := []llm.Message{
		{
			Role:    "system",
			Content: SystemPrompt,
		},
	}

	// 添加完整的对话历史（如果有）
	memoryStartTime := time.Now()
	if a.memoryManager != nil {
		session := a.memoryManager.GetSession(sessionID)
		if session != nil && len(session.ShortTerm) > 0 {
			// 添加短期记忆中的历史消息
			for _, msg := range session.ShortTerm {
				messages = append(messages, llm.Message{
					Role:    msg.Role,
					Content: msg.Content,
				})
			}
		} else {
			// 如果没有短期记忆，尝试使用记忆摘要
			summary := a.memoryManager.SummarizeMemory(sessionID)
			if summary != "" {
				messages = append(messages, llm.Message{
					Role:    "system",
					Content: summary,
				})
			}
		}
	}
	memoryDuration := time.Since(memoryStartTime)

	if a.shouldAppendUserMessage(sessionID, userInput) {
		messages = append(messages, llm.Message{
			Role:    "user",
			Content: userInput,
		})
	}

	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "Agent")
	logger.Info("开始处理用户请求", map[string]interface{}{
		"session_id":  sessionID,
		"user_input":  userInput,
		"trace_id":    runCtx.TraceID,
		"init_time":   time.Since(startTime).Milliseconds(),
		"memory_time": memoryDuration.Milliseconds(),
	})

	// 记录加载的历史消息数量
	if a.memoryManager != nil {
		session := a.memoryManager.GetSession(sessionID)
		if session != nil && len(session.ShortTerm) > 0 {
			logger.Info("加载对话历史", map[string]interface{}{
				"session_id":    sessionID,
				"history_count": len(session.ShortTerm),
			})
		}
	}

	// ReAct循环
	for step := 0; ; step++ {
		if err := runCtx.ValidateBudget(); err != nil {
			runCtx.Enter(agentruntime.StateFailed)
			logger.Warn("运行时预算超限", map[string]interface{}{
				"trace_id":     runCtx.TraceID,
				"session_id":   sessionID,
				"step":         runCtx.Stats.Steps,
				"tool_calls":   runCtx.Stats.ToolCalls,
				"elapsed_ms":   runCtx.Elapsed().Milliseconds(),
				"budget_cause": err.Error(),
			})
			msg := "处理超时，请尝试简化问题或提供更多信息。"
			if cerr := callback(msg); cerr != nil {
				return "", cerr
			}
			return msg, nil
		}

		runCtx.IncStep()
		runCtx.Enter(agentruntime.StateExecute)
		stepStartTime := time.Now()
		logger.Info("执行ReAct步骤", map[string]interface{}{
			"step":      step,
			"max_steps": a.maxSteps,
			"trace_id":  runCtx.TraceID,
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
				"trace_id": runCtx.TraceID,
			})
			runCtx.Enter(agentruntime.StateFailed)
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
					"trace_id":  runCtx.TraceID,
				})
				runCtx.Enter(agentruntime.StateFailed)
				msg := "检测到工具调用循环，请尝试简化问题或提供更多信息。"
				if cerr := callback(msg); cerr != nil {
					return "", cerr
				}
				return msg, nil
			}

			// 添加到工具调用历史，并限制历史记录数量
			a.toolCallHistory = append(a.toolCallHistory, currentToolCall)
			if len(a.toolCallHistory) > a.maxToolHistory {
				a.toolCallHistory = a.toolCallHistory[len(a.toolCallHistory)-a.maxToolHistory:]
			}

			// 策略评估：决定 allow / require_approval / deny
			policyResult := tools.PolicyResult{Decision: tools.PolicyAllow}
			if a.policyEngine != nil {
				policyResult = a.policyEngine.Evaluate(ctx, functionCall.Name, functionCall.Arguments)
			}
			if policyResult.Decision == tools.PolicyDeny {
				logger.Warn("工具调用被策略拒绝", map[string]interface{}{
					"tool_name": functionCall.Name,
					"reason":    policyResult.Reason,
					"trace_id":  runCtx.TraceID,
				})
				msg := fmt.Sprintf("工具调用已被策略拒绝：%s", policyResult.Reason)
				if cerr := callback(msg); cerr != nil {
					return "", cerr
				}
				return msg, nil
			}
			requireApproval := policyResult.Decision == tools.PolicyRequireApproval

			// 检查是否启用了 human-in-the-loop
			logger.Info("检查 human-in-the-loop 设置", map[string]interface{}{
				"human_in_the_loop": a.humanInTheLoop,
				"require_approval":  requireApproval,
			})
			if requireApproval {
				if !a.humanInTheLoop {
					msg := fmt.Sprintf("工具调用需要人工审批但当前未启用审批：%s", functionCall.Name)
					if cerr := callback(msg); cerr != nil {
						return "", cerr
					}
					return msg, nil
				}
				toolCallRequest := a.createToolCallRequest(functionCall.Name, functionCall.Arguments, response)

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

				approved, waitErr := a.waitForToolApproval(ctx, sessionID, toolCallRequest)
				if waitErr != nil {
					logger.Warn("等待工具审批失败", map[string]interface{}{
						"tool_name":    functionCall.Name,
						"tool_call_id": toolCallRequest.ID,
						"error":        waitErr.Error(),
						"trace_id":     runCtx.TraceID,
					})
					msg := "工具调用审批超时或会话已取消，请重试。"
					if cerr := callback(msg); cerr != nil {
						return "", cerr
					}
					return msg, nil
				}
				if !approved {
					logger.Info("工具调用被拒绝", map[string]interface{}{
						"tool_name":    functionCall.Name,
						"tool_call_id": toolCallRequest.ID,
						"trace_id":     runCtx.TraceID,
					})
					msg := fmt.Sprintf("工具调用已取消：%s", functionCall.Name)
					if cerr := callback(msg); cerr != nil {
						return "", cerr
					}
					return msg, nil
				}
			}

			// 通知用户正在使用工具
			toolStatusMsg := fmt.Sprintf("[STATUS] 正在使用 %s 工具...", functionCall.Name)
			if cerr := callback(toolStatusMsg); cerr != nil {
				return "", cerr
			}

			// 执行工具
			if runCtx.Limits.MaxToolCalls > 0 && runCtx.Stats.ToolCalls >= runCtx.Limits.MaxToolCalls {
				runCtx.Enter(agentruntime.StateFailed)
				logger.Warn("工具执行前预算超限", map[string]interface{}{
					"trace_id":     runCtx.TraceID,
					"session_id":   sessionID,
					"tool_name":    functionCall.Name,
					"step":         runCtx.Stats.Steps,
					"tool_calls":   runCtx.Stats.ToolCalls,
					"elapsed_ms":   runCtx.Elapsed().Milliseconds(),
					"budget_cause": agentruntime.ErrToolBudgetExceeded.Error(),
				})
				msg := "处理超时，请尝试简化问题或提供更多信息。"
				if cerr := callback(msg); cerr != nil {
					return "", cerr
				}
				return msg, nil
			}
			runCtx.IncToolCall()

			toolStartTime := time.Now()
			logger.Info("执行工具", map[string]interface{}{
				"tool_name": functionCall.Name,
				"arguments": functionCall.Arguments,
				"trace_id":  runCtx.TraceID,
			})

			toolResult, err := a.toolGateway.Execute(ctx, functionCall.Name, functionCall.Arguments)
			toolDuration := time.Since(toolStartTime)
			if err != nil {
				logger.Error("工具执行失败", map[string]interface{}{
					"tool_name": functionCall.Name,
					"error":     err.Error(),
					"duration":  toolDuration.Milliseconds(),
					"trace_id":  runCtx.TraceID,
				})
				toolResult = fmt.Sprintf("Error: %v", err)
			} else {
				logger.Info("工具执行成功", map[string]interface{}{
					"tool_name":     functionCall.Name,
					"result_length": len(toolResult),
					"duration":      toolDuration.Milliseconds(),
					"trace_id":      runCtx.TraceID,
				})
			}

			// 通知用户工具执行结果
			toolResultMsg := fmt.Sprintf("[RESULT] %s", toolResult)
			if cerr := callback(toolResultMsg); cerr != nil {
				return "", cerr
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

			// 记录步骤耗时
			stepDuration := time.Since(stepStartTime)
			logger.Info("ReAct步骤完成", map[string]interface{}{
				"step":     step,
				"duration": stepDuration.Milliseconds(),
				"trace_id": runCtx.TraceID,
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
				"trace_id": runCtx.TraceID,
			})
			runCtx.Enter(agentruntime.StateVerify)
			runCtx.Enter(agentruntime.StateFinalize)
			logger.Info("返回LLM响应", map[string]interface{}{
				"response_length": len(response),
				"trace_id":        runCtx.TraceID,
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
		"trace_id":       runCtx.TraceID,
	})

	msg := "处理超时，请尝试简化问题或提供更多信息。"
	runCtx.Enter(agentruntime.StateFailed)
	if cerr := callback(msg); cerr != nil {
		return "", cerr
	}
	return msg, nil
}

// RunWithPlanning 使用任务规划运行Agent（适合复杂任务）
func (a *Agent) RunWithPlanning(ctx context.Context, sessionID, userInput string) (string, error) {
	logger := logging.NewLogger(logging.INFO, "Agent.Planner")
	logger.Info("开始任务规划", map[string]interface{}{
		"session_id": sessionID,
		"user_input": userInput,
	})

	// 1. 分解任务
	plan, err := a.taskPlanner.DecomposeGoal(ctx, userInput)
	if err != nil {
		logger.Warn("任务分解失败", map[string]interface{}{
			"error": err.Error(),
		})
		// fallback 到普通模式
		return a.Run(ctx, sessionID, userInput)
	}

	logger.Info("任务分解完成", map[string]interface{}{
		"task_count": len(plan.Tasks),
	})

	// 2. 显示任务计划
	planDisplay := a.taskPlanner.FormatPlanForDisplay(plan)
	logger.Info("任务计划", map[string]interface{}{
		"plan": planDisplay,
	})

	// 3. 执行任务
	var results []string
	for {
		// 获取下一个可执行任务
		task := a.taskPlanner.GetNextTask(plan)
		if task == nil {
			break // 没有更多任务
		}

		// 更新任务状态为进行中
		a.taskPlanner.UpdateTaskStatus(plan, task.ID, planner.TaskInProgress, "", "")

		logger.Info("执行任务", map[string]interface{}{
			"task_id":     task.ID,
			"description": task.Description,
		})

		// 构建任务提示
		taskPrompt := fmt.Sprintf("当前任务：%s\n目标：%s", task.Description, plan.Goal)
		if len(results) > 0 {
			taskPrompt += fmt.Sprintf("\n之前任务的结果：\n%s", strings.Join(results, "\n"))
		}

		// 执行任务
		result, err := a.Run(ctx, sessionID, taskPrompt)
		if err != nil {
			a.taskPlanner.UpdateTaskStatus(plan, task.ID, planner.TaskFailed, "", err.Error())
			logger.Warn("任务执行失败", map[string]interface{}{
				"task_id": task.ID,
				"error":   err.Error(),
			})
			continue
		}

		// 🔄 自我反思：评估任务执行效果
		if a.reflector != nil {
			reflection, reflectErr := a.reflector.ReflectOnAction(ctx, plan.Goal, task.Description, result)
			if reflectErr == nil {
				logger.Info("任务反思", map[string]interface{}{
					"task_id":      task.ID,
					"score":        reflection.Score,
					"is_effective": reflection.IsEffective,
				})

				// 如果效果很差，记录建议
				if reflection.Score <= 4 && len(reflection.Suggestions) > 0 {
					logger.Info("改进建议", map[string]interface{}{
						"task_id":     task.ID,
						"suggestions": reflection.Suggestions,
					})
				}
			}
		}

		// 更新任务状态为完成
		a.taskPlanner.UpdateTaskStatus(plan, task.ID, planner.TaskCompleted, result, "")
		results = append(results, fmt.Sprintf("%s: %s", task.Description, result))

		logger.Info("任务完成", map[string]interface{}{
			"task_id": task.ID,
		})
	}

	// 4. 🔄 整体反思：评估整个任务计划
	var reflectionSummary string
	if a.reflector != nil && len(plan.Tasks) > 0 {
		completedTasks := make([]CompletedTask, 0)
		for _, task := range plan.Tasks {
			completedTasks = append(completedTasks, CompletedTask{
				Description: task.Description,
				Status:      string(task.Status),
				Result:      task.Result,
				Error:       task.Error,
			})
		}

		planReflection, err := a.reflector.ReflectOnPlan(ctx, plan.Goal, completedTasks)
		if err == nil {
			reflectionSummary = "\n\n📊 整体评估:\n" + a.reflector.GetImprovementSuggestions(planReflection)
			logger.Info("计划整体反思", map[string]interface{}{
				"score": planReflection.Score,
			})
		}
	}

	// 5. 汇总结果
	completed, total, percentage := a.taskPlanner.GetProgress(plan)
	summary := fmt.Sprintf("✅ 任务完成！进度：%d/%d (%.0f%%)\n\n", completed, total, percentage)
	summary += a.taskPlanner.FormatPlanForDisplay(plan)
	summary += reflectionSummary

	logger.Info("所有任务完成", map[string]interface{}{
		"completed":  completed,
		"total":      total,
		"percentage": percentage,
	})

	return summary, nil
}
