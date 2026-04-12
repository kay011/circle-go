package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"circle-go/internal/llm"
	"circle-go/internal/logging"
)

// ReflectionResult 反思结果
type ReflectionResult struct {
	IsEffective          bool     `json:"is_effective"`           // 行动是否有效
	Score                int      `json:"score"`                  // 效果评分 (1-10)
	Issues               []string `json:"issues,omitempty"`       // 发现的问题
	Suggestions          []string `json:"suggestions,omitempty"`  // 改进建议
	ShouldRetry          bool     `json:"should_retry"`           // 是否应该重试
	ShouldAdjustStrategy bool     `json:"should_adjust_strategy"` // 是否应该调整策略
	Reasoning            string   `json:"reasoning"`              // 推理过程
}

// Reflector 自我反思器
type Reflector struct {
	llm llm.LLM
}

// NewReflector 创建反思器
func NewReflector(llmClient llm.LLM) *Reflector {
	return &Reflector{
		llm: llmClient,
	}
}

// ReflectOnAction 对已执行的动作进行反思
func (r *Reflector) ReflectOnAction(ctx context.Context, goal, action, result string) (*ReflectionResult, error) {
	logger := logging.NewLogger(logging.INFO, "Agent.Reflection")
	logger.Info("开始反思", map[string]interface{}{
		"goal":   goal,
		"action": action,
	})

	// 构建反思提示
	systemPrompt := `你是一个专业的AI助手评估专家。请评估AI助手执行的行动是否有效地推进了目标。

评估维度：
1. 相关性：行动是否与目标相关
2. 有效性：行动结果是否有助于达成目标
3. 完整性：结果是否充分回答了问题
4. 准确性：结果是否准确可靠

评分标准：
- 9-10分：完美，完全达成目标
- 7-8分：良好，基本达成目标
- 5-6分：一般，部分达成目标
- 3-4分：较差，几乎没有帮助
- 1-2分：无效，完全偏离目标

请返回JSON格式的评估结果：
{
  "is_effective": true/false,
  "score": 1-10,
  "issues": ["问题1", "问题2"],
  "suggestions": ["建议1", "建议2"],
  "should_retry": true/false,
  "should_adjust_strategy": true/false,
  "reasoning": "详细的推理过程"
}`

	userPrompt := fmt.Sprintf(`请评估以下行动的效果：

目标：%s

执行的行动：%s

行动结果：%s

请给出客观评估。`, goal, action, result)

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := r.llm.Chat(ctx, messages)
	if err != nil {
		logger.Warn("反思调用LLM失败", map[string]interface{}{
			"error": err.Error(),
		})
		// 返回默认评估
		return r.defaultReflection(), nil
	}

	// 解析反思结果
	reflection, err := parseReflection(response)
	if err != nil {
		logger.Warn("解析反思结果失败", map[string]interface{}{
			"error":    err.Error(),
			"response": response,
		})
		return r.defaultReflection(), nil
	}

	logger.Info("反思完成", map[string]interface{}{
		"score":        reflection.Score,
		"is_effective": reflection.IsEffective,
		"should_retry": reflection.ShouldRetry,
	})

	return reflection, nil
}

// ReflectOnPlan 对整个任务计划进行反思
func (r *Reflector) ReflectOnPlan(ctx context.Context, goal string, completedTasks []CompletedTask) (*ReflectionResult, error) {
	logger := logging.NewLogger(logging.INFO, "Agent.PlanReflection")
	logger.Info("开始计划反思", map[string]interface{}{
		"goal":        goal,
		"tasks_count": len(completedTasks),
	})

	// 构建任务历史
	var taskHistory strings.Builder
	for i, task := range completedTasks {
		taskHistory.WriteString(fmt.Sprintf("%d. %s\n", i+1, task.Description))
		taskHistory.WriteString(fmt.Sprintf("   状态: %s\n", task.Status))
		if task.Result != "" {
			taskHistory.WriteString(fmt.Sprintf("   结果: %s\n", truncateString(task.Result, 200)))
		}
		if task.Error != "" {
			taskHistory.WriteString(fmt.Sprintf("   错误: %s\n", task.Error))
		}
		taskHistory.WriteString("\n")
	}

	systemPrompt := `你是一个任务规划评估专家。请评估已完成的任务序列是否有效地推进了总体目标。

评估要点：
1. 任务分解是否合理
2. 执行顺序是否最优
3. 是否有遗漏的关键步骤
4. 是否需要调整后续策略

请返回JSON格式的评估结果。`

	userPrompt := fmt.Sprintf(`请评估以下任务计划的执行效果：

总体目标：%s

已完成的任务：
%s

请评估并给出改进建议。`, goal, taskHistory.String())

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	response, err := r.llm.Chat(ctx, messages)
	if err != nil {
		logger.Warn("计划反思调用LLM失败", map[string]interface{}{
			"error": err.Error(),
		})
		return r.defaultReflection(), nil
	}

	reflection, err := parseReflection(response)
	if err != nil {
		logger.Warn("解析计划反思结果失败", map[string]interface{}{
			"error": err.Error(),
		})
		return r.defaultReflection(), nil
	}

	logger.Info("计划反思完成", map[string]interface{}{
		"score":       reflection.Score,
		"suggestions": len(reflection.Suggestions),
	})

	return reflection, nil
}

// ShouldContinueExecution 根据反思结果判断是否继续执行
func (r *Reflector) ShouldContinueExecution(reflection *ReflectionResult, consecutiveFailures int) bool {
	// 如果连续失败次数过多，停止执行
	if consecutiveFailures >= 3 {
		return false
	}

	// 如果评分很低且不建议重试，停止执行
	if reflection.Score <= 2 && !reflection.ShouldRetry {
		return false
	}

	// 其他情况继续执行
	return true
}

// GetImprovementSuggestions 获取改进建议（人类可读格式）
func (r *Reflector) GetImprovementSuggestions(reflection *ReflectionResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 评估评分: %d/10\n", reflection.Score))
	sb.WriteString(fmt.Sprintf("✅ 行动有效: %v\n\n", reflection.IsEffective))

	if len(reflection.Issues) > 0 {
		sb.WriteString("⚠️ 发现的问题:\n")
		for i, issue := range reflection.Issues {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, issue))
		}
		sb.WriteString("\n")
	}

	if len(reflection.Suggestions) > 0 {
		sb.WriteString("💡 改进建议:\n")
		for i, suggestion := range reflection.Suggestions {
			sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, suggestion))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("🔄 推理过程:\n%s\n", reflection.Reasoning))

	return sb.String()
}

// CompletedTask 已完成的任务
type CompletedTask struct {
	Description string
	Status      string
	Result      string
	Error       string
}

// parseReflection 解析反思结果
func parseReflection(response string) (*ReflectionResult, error) {
	// 清理可能的代码块标记
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// 提取 JSON 部分
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("no valid JSON found")
	}

	jsonStr := response[startIdx : endIdx+1]

	var result ReflectionResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse reflection: %w", err)
	}

	// 验证字段
	if result.Score < 1 || result.Score > 10 {
		result.Score = 5 // 默认中等评分
	}

	if result.Reasoning == "" {
		result.Reasoning = "No reasoning provided"
	}

	return &result, nil
}

// defaultReflection 返回默认的反思结果（当LLM调用失败时）
func (r *Reflector) defaultReflection() *ReflectionResult {
	return &ReflectionResult{
		IsEffective:          true,
		Score:                5,
		Issues:               []string{"Unable to evaluate due to LLM error"},
		Suggestions:          []string{"Proceed with caution"},
		ShouldRetry:          false,
		ShouldAdjustStrategy: false,
		Reasoning:            "Default reflection due to LLM error",
	}
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ReflectAndAdjust 反思并调整策略（便捷方法）
func (a *Agent) ReflectAndAdjust(ctx context.Context, goal, action, result string) (*ReflectionResult, error) {
	if a.reflector == nil {
		a.reflector = NewReflector(a.llm)
	}

	reflection, err := a.reflector.ReflectOnAction(ctx, goal, action, result)
	if err != nil {
		return nil, err
	}

	// 如果需要调整策略，记录日志
	if reflection.ShouldAdjustStrategy {
		logger := logging.NewLogger(logging.INFO, "Agent.Strategy")
		logger.Info("建议调整策略", map[string]interface{}{
			"goal":        goal,
			"score":       reflection.Score,
			"suggestions": reflection.Suggestions,
		})
	}

	return reflection, nil
}
