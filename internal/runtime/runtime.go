package runtime

import (
	"errors"
	"fmt"
	"time"
)

// State 表示 Agent 在一次执行中的阶段。
type State string

const (
	StateInit     State = "init"
	StatePlan     State = "plan"
	StateExecute  State = "execute"
	StateVerify   State = "verify"
	StateFinalize State = "finalize"
	StateFailed   State = "failed"
)

var (
	// ErrStepBudgetExceeded 表示超过最大步骤预算。
	ErrStepBudgetExceeded = errors.New("step budget exceeded")
	// ErrToolBudgetExceeded 表示超过最大工具调用预算。
	ErrToolBudgetExceeded = errors.New("tool call budget exceeded")
	// ErrDurationBudgetExceeded 表示超过最大耗时预算。
	ErrDurationBudgetExceeded = errors.New("duration budget exceeded")
)

// Limits 定义单次运行的预算限制。
type Limits struct {
	MaxSteps     int
	MaxToolCalls int
	MaxDuration  time.Duration
}

// Stats 记录一次运行中的实际使用情况。
type Stats struct {
	Steps     int
	ToolCalls int
	StartedAt time.Time
}

// RunContext 统一保存单次执行的上下文、预算和状态轨迹。
type RunContext struct {
	SessionID string
	TraceID   string
	Limits    Limits
	Stats     Stats
	History   []State
}

// Option 用于配置 RunContext。
type Option func(*RunContext)

// WithTraceID 设置 trace id。
func WithTraceID(traceID string) Option {
	return func(rc *RunContext) {
		if traceID != "" {
			rc.TraceID = traceID
		}
	}
}

// WithMaxSteps 设置步骤预算。
func WithMaxSteps(maxSteps int) Option {
	return func(rc *RunContext) {
		if maxSteps > 0 {
			rc.Limits.MaxSteps = maxSteps
		}
	}
}

// WithMaxToolCalls 设置工具调用预算。
func WithMaxToolCalls(maxToolCalls int) Option {
	return func(rc *RunContext) {
		if maxToolCalls > 0 {
			rc.Limits.MaxToolCalls = maxToolCalls
		}
	}
}

// WithMaxDuration 设置时长预算。
func WithMaxDuration(maxDuration time.Duration) Option {
	return func(rc *RunContext) {
		if maxDuration > 0 {
			rc.Limits.MaxDuration = maxDuration
		}
	}
}

// NewRunContext 创建运行时上下文并填充默认预算。
func NewRunContext(sessionID string, opts ...Option) *RunContext {
	now := time.Now()
	rc := &RunContext{
		SessionID: sessionID,
		TraceID:   fmt.Sprintf("run_%d", now.UnixNano()),
		Limits: Limits{
			MaxSteps:     5,
			MaxToolCalls: 20,
			MaxDuration:  2 * time.Minute,
		},
		Stats: Stats{
			StartedAt: now,
		},
		History: make([]State, 0, 8),
	}

	for _, opt := range opts {
		opt(rc)
	}

	return rc
}

// Enter 记录状态流转。
func (rc *RunContext) Enter(state State) {
	rc.History = append(rc.History, state)
}

// IncStep 增加步骤计数。
func (rc *RunContext) IncStep() {
	rc.Stats.Steps++
}

// IncToolCall 增加工具调用计数。
func (rc *RunContext) IncToolCall() {
	rc.Stats.ToolCalls++
}

// Elapsed 返回已耗时长。
func (rc *RunContext) Elapsed() time.Duration {
	return time.Since(rc.Stats.StartedAt)
}

// ValidateBudget 检查预算是否超限。
func (rc *RunContext) ValidateBudget() error {
	if rc.Limits.MaxSteps > 0 && rc.Stats.Steps >= rc.Limits.MaxSteps {
		return ErrStepBudgetExceeded
	}
	if rc.Limits.MaxToolCalls > 0 && rc.Stats.ToolCalls >= rc.Limits.MaxToolCalls {
		return ErrToolBudgetExceeded
	}
	if rc.Limits.MaxDuration > 0 && rc.Elapsed() >= rc.Limits.MaxDuration {
		return ErrDurationBudgetExceeded
	}
	return nil
}
