package tools

import (
	"context"
	"fmt"
	"time"
)

// AuditStatus 表示工具调用的审计状态。
type AuditStatus string

const (
	AuditStatusStarted AuditStatus = "started"
	AuditStatusSuccess AuditStatus = "success"
	AuditStatusFailed  AuditStatus = "failed"
)

// AuditEvent 记录工具执行的关键审计信息。
type AuditEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	ToolName  string                 `json:"tool_name"`
	Status    AuditStatus            `json:"status"`
	Duration  time.Duration          `json:"duration"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// AuditSink 为工具执行提供审计事件出口。
type AuditSink func(event AuditEvent)

// ToolPolicy 定义单个工具的策略。
type ToolPolicy struct {
	Timeout time.Duration
}

// ToolGateway 统一处理工具执行前后的治理逻辑。
type ToolGateway struct {
	manager       *ToolManager
	defaultPolicy ToolPolicy
	perToolPolicy map[string]ToolPolicy
	auditSink     AuditSink
}

// NewToolGateway 创建工具网关。
func NewToolGateway(manager *ToolManager, defaultTimeout time.Duration, sink AuditSink) *ToolGateway {
	if defaultTimeout <= 0 {
		defaultTimeout = 20 * time.Second
	}
	return &ToolGateway{
		manager:       manager,
		defaultPolicy: ToolPolicy{Timeout: defaultTimeout},
		perToolPolicy: make(map[string]ToolPolicy),
		auditSink:     sink,
	}
}

// SetPolicy 设置指定工具策略。
func (g *ToolGateway) SetPolicy(toolName string, policy ToolPolicy) {
	if toolName == "" {
		return
	}
	if policy.Timeout <= 0 {
		policy.Timeout = g.defaultPolicy.Timeout
	}
	g.perToolPolicy[toolName] = policy
}

// Execute 执行工具（含 schema 校验、超时控制、审计事件）。
func (g *ToolGateway) Execute(ctx context.Context, toolName string, args map[string]interface{}) (string, error) {
	start := time.Now()
	tool := g.manager.Get(toolName)
	if tool == nil {
		err := fmt.Errorf("tool not found: %s", toolName)
		g.emit(AuditEvent{
			Timestamp: start,
			ToolName:  toolName,
			Status:    AuditStatusFailed,
			Duration:  0,
			Arguments: args,
			Error:     err.Error(),
		})
		return "", err
	}

	if args == nil {
		args = map[string]interface{}{}
	}

	if err := validateArgs(tool, args); err != nil {
		g.emit(AuditEvent{
			Timestamp: start,
			ToolName:  toolName,
			Status:    AuditStatusFailed,
			Duration:  0,
			Arguments: args,
			Error:     err.Error(),
		})
		return "", err
	}

	g.emit(AuditEvent{
		Timestamp: start,
		ToolName:  toolName,
		Status:    AuditStatusStarted,
		Arguments: args,
	})

	policy := g.defaultPolicy
	if p, ok := g.perToolPolicy[toolName]; ok {
		policy = p
	}

	execCtx := ctx
	cancel := func() {}
	if policy.Timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, policy.Timeout)
	}
	defer cancel()

	result, err := tool.Run(execCtx, args)
	duration := time.Since(start)
	if err != nil {
		g.emit(AuditEvent{
			Timestamp: start,
			ToolName:  toolName,
			Status:    AuditStatusFailed,
			Duration:  duration,
			Arguments: args,
			Error:     err.Error(),
		})
		return "", err
	}

	g.emit(AuditEvent{
		Timestamp: start,
		ToolName:  toolName,
		Status:    AuditStatusSuccess,
		Duration:  duration,
		Arguments: args,
	})
	return result, nil
}

func (g *ToolGateway) emit(event AuditEvent) {
	if g.auditSink != nil {
		g.auditSink(event)
	}
}

func validateArgs(tool Tool, args map[string]interface{}) error {
	required := tool.Required()
	for _, req := range required {
		if _, exists := args[req]; !exists {
			return fmt.Errorf("missing required parameter: %s", req)
		}
	}

	properties := tool.Parameters()
	for key, value := range args {
		prop, exists := properties[key]
		if !exists {
			continue
		}
		if !matchesType(prop.Type, value) {
			return fmt.Errorf("invalid parameter type for %s: expected %s", key, prop.Type)
		}
	}
	return nil
}

func matchesType(expected string, value interface{}) bool {
	switch expected {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64:
			return true
		default:
			return false
		}
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	default:
		// 未知类型默认放行，避免破坏老工具。
		return true
	}
}
