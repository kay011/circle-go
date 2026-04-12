package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"circle-go/internal/llm"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "in_progress"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskSkipped    TaskStatus = "skipped"
)

// TaskPriority 任务优先级
type TaskPriority int

const (
	PriorityLow      TaskPriority = 1
	PriorityMedium   TaskPriority = 2
	PriorityHigh     TaskPriority = 3
	PriorityCritical TaskPriority = 4
)

// SubTask 子任务
type SubTask struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description"`
	Status       TaskStatus             `json:"status"`
	Priority     TaskPriority           `json:"priority"`
	Dependencies []string               `json:"dependencies,omitempty"` // 依赖的任务ID
	ToolName     string                 `json:"tool_name,omitempty"`    // 需要使用的工具
	Arguments    map[string]interface{} `json:"arguments,omitempty"`
	Result       string                 `json:"result,omitempty"`
	Error        string                 `json:"error,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
}

// TaskPlan 任务计划
type TaskPlan struct {
	Goal      string     `json:"goal"`
	Tasks     []SubTask  `json:"tasks"`
	Status    TaskStatus `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Planner 任务规划器
type Planner struct {
	llm llm.LLM
}

// NewPlanner 创建任务规划器
func NewPlanner(llmClient llm.LLM) *Planner {
	return &Planner{
		llm: llmClient,
	}
}

// DecomposeGoal 将复杂目标分解为子任务
func (p *Planner) DecomposeGoal(ctx context.Context, goal string) (*TaskPlan, error) {
	// 构建提示词，让 LLM 分解任务
	systemPrompt := `你是一个专业的任务规划助手。请将用户的复杂目标分解为可执行的子任务序列。

要求：
1. 每个子任务应该是原子性的、可执行的
2. 明确任务之间的依赖关系
3. 为每个任务分配合适的优先级
4. 如果需要使用工具，指定工具名称和参数
5. 返回 JSON 格式的任务计划

可用的工具：
- calculator: 数学计算
- web_search: 网络搜索（参数：query）
- file_operation: 文件读写（参数：operation, file_path, content）

返回格式示例：
{
  "goal": "用户目标",
  "tasks": [
    {
      "id": "task_1",
      "description": "任务描述",
      "priority": 2,
      "dependencies": [],
      "tool_name": "web_search",
      "arguments": {"query": "搜索关键词"}
    }
  ]
}`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("请分解以下目标：%s", goal)},
	}

	response, err := p.llm.Chat(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to decompose goal: %w", err)
	}

	// 解析 JSON 响应
	plan, err := parseTaskPlan(response, goal)
	if err != nil {
		// 如果解析失败，创建一个简单的单任务计划
		plan = createSimplePlan(goal)
	}

	return plan, nil
}

// parseTaskPlan 解析任务计划
func parseTaskPlan(response, goal string) (*TaskPlan, error) {
	// 清理可能的代码块标记
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// 提取 JSON 部分
	startIdx := strings.Index(response, "{")
	endIdx := strings.LastIndex(response, "}")
	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	jsonStr := response[startIdx : endIdx+1]

	var plan TaskPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("failed to parse task plan: %w", err)
	}

	// 设置默认值
	plan.Goal = goal
	plan.Status = TaskPending
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()

	for i := range plan.Tasks {
		if plan.Tasks[i].Status == "" {
			plan.Tasks[i].Status = TaskPending
		}
		if plan.Tasks[i].Priority == 0 {
			plan.Tasks[i].Priority = PriorityMedium
		}
		plan.Tasks[i].CreatedAt = time.Now()
		if plan.Tasks[i].Dependencies == nil {
			plan.Tasks[i].Dependencies = []string{}
		}
	}

	return &plan, nil
}

// createSimplePlan 创建简单的单任务计划（fallback）
func createSimplePlan(goal string) *TaskPlan {
	now := time.Now()
	return &TaskPlan{
		Goal:      goal,
		Status:    TaskPending,
		CreatedAt: now,
		UpdatedAt: now,
		Tasks: []SubTask{
			{
				ID:          "task_1",
				Description: goal,
				Status:      TaskPending,
				Priority:    PriorityMedium,
				CreatedAt:   now,
			},
		},
	}
}

// GetNextTask 获取下一个可执行的任务
func (p *Planner) GetNextTask(plan *TaskPlan) *SubTask {
	for i := range plan.Tasks {
		task := &plan.Tasks[i]

		// 跳过已完成或失败的任务
		if task.Status != TaskPending {
			continue
		}

		// 检查依赖是否都已完成
		if p.areDependenciesMet(task, plan) {
			return task
		}
	}
	return nil
}

// areDependenciesMet 检查任务的依赖是否都已完成
func (p *Planner) areDependenciesMet(task *SubTask, plan *TaskPlan) bool {
	for _, depID := range task.Dependencies {
		depFound := false
		for _, t := range plan.Tasks {
			if t.ID == depID {
				depFound = true
				if t.Status != TaskCompleted {
					return false
				}
				break
			}
		}
		if !depFound {
			return false
		}
	}
	return true
}

// UpdateTaskStatus 更新任务状态
func (p *Planner) UpdateTaskStatus(plan *TaskPlan, taskID string, status TaskStatus, result, errMsg string) {
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			plan.Tasks[i].Status = status
			plan.Tasks[i].Result = result
			plan.Tasks[i].Error = errMsg

			if status == TaskCompleted || status == TaskFailed {
				now := time.Now()
				plan.Tasks[i].CompletedAt = &now
			}
			break
		}
	}

	plan.UpdatedAt = time.Now()
	p.updatePlanStatus(plan)
}

// updatePlanStatus 更新整体计划状态
func (p *Planner) updatePlanStatus(plan *TaskPlan) {
	allCompleted := true
	hasInProgress := false
	hasFailed := false

	for _, task := range plan.Tasks {
		switch task.Status {
		case TaskPending, TaskInProgress:
			allCompleted = false
			if task.Status == TaskInProgress {
				hasInProgress = true
			}
		case TaskFailed:
			allCompleted = false
			hasFailed = true
		}
	}

	if allCompleted {
		plan.Status = TaskCompleted
	} else if hasInProgress {
		plan.Status = TaskInProgress
	} else if hasFailed {
		plan.Status = TaskFailed
	} else {
		plan.Status = TaskInProgress
	}
}

// GetProgress 获取任务进度
func (p *Planner) GetProgress(plan *TaskPlan) (completed, total int, percentage float64) {
	total = len(plan.Tasks)
	for _, task := range plan.Tasks {
		if task.Status == TaskCompleted {
			completed++
		}
	}

	if total > 0 {
		percentage = float64(completed) / float64(total) * 100
	}

	return completed, total, percentage
}

// FormatPlanForDisplay 格式化任务计划用于显示
func (p *Planner) FormatPlanForDisplay(plan *TaskPlan) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 任务计划: %s\n", plan.Goal))
	sb.WriteString(fmt.Sprintf("状态: %s | 进度: ", plan.Status))

	completed, total, percentage := p.GetProgress(plan)
	sb.WriteString(fmt.Sprintf("%d/%d (%.0f%%)\n\n", completed, total, percentage))

	for i, task := range plan.Tasks {
		statusIcon := p.getStatusIcon(task.Status)
		priorityLabel := p.getPriorityLabel(task.Priority)

		sb.WriteString(fmt.Sprintf("%d. %s [%s] %s\n", i+1, statusIcon, priorityLabel, task.Description))

		if task.ToolName != "" {
			sb.WriteString(fmt.Sprintf("   工具: %s\n", task.ToolName))
		}

		if task.Result != "" {
			sb.WriteString(fmt.Sprintf("   结果: %s\n", task.Result))
		}

		if task.Error != "" {
			sb.WriteString(fmt.Sprintf("   错误: %s\n", task.Error))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func (p *Planner) getStatusIcon(status TaskStatus) string {
	switch status {
	case TaskCompleted:
		return "✅"
	case TaskInProgress:
		return "🔄"
	case TaskFailed:
		return "❌"
	case TaskSkipped:
		return "⏭️"
	default:
		return "⏳"
	}
}

func (p *Planner) getPriorityLabel(priority TaskPriority) string {
	switch priority {
	case PriorityCritical:
		return "CRITICAL"
	case PriorityHigh:
		return "HIGH"
	case PriorityMedium:
		return "MEDIUM"
	case PriorityLow:
		return "LOW"
	default:
		return "MEDIUM"
	}
}

// ReprioritizeTasks 根据执行情况重新排序任务
func (p *Planner) ReprioritizeTasks(plan *TaskPlan) {
	// 简单的重新排序逻辑：将高优先级任务前置
	tasks := plan.Tasks

	// 按优先级排序（降序）
	for i := 0; i < len(tasks)-1; i++ {
		for j := i + 1; j < len(tasks); j++ {
			if tasks[i].Priority < tasks[j].Priority &&
				tasks[i].Status == TaskPending &&
				tasks[j].Status == TaskPending {
				tasks[i], tasks[j] = tasks[j], tasks[i]
			}
		}
	}

	plan.Tasks = tasks
	plan.UpdatedAt = time.Now()
}
