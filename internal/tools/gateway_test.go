package tools

import (
	"context"
	"testing"
	"time"
)

type gatewayMockTool struct {
	name     string
	params   map[string]Property
	required []string
	runFn    func(ctx context.Context, args map[string]interface{}) (string, error)
}

func (t *gatewayMockTool) Name() string { return t.name }
func (t *gatewayMockTool) Description() string {
	return "mock"
}
func (t *gatewayMockTool) Parameters() map[string]Property { return t.params }
func (t *gatewayMockTool) Required() []string              { return t.required }
func (t *gatewayMockTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	return t.runFn(ctx, args)
}

func TestToolGateway_ValidateSchemaAndExecute(t *testing.T) {
	manager := NewToolManager()
	manager.Register(&gatewayMockTool{
		name: "echo",
		params: map[string]Property{
			"text": {Type: "string", Description: "text"},
		},
		required: []string{"text"},
		runFn: func(ctx context.Context, args map[string]interface{}) (string, error) {
			return args["text"].(string), nil
		},
	})

	gateway := NewToolGateway(manager, 5*time.Second, nil)

	if _, err := gateway.Execute(context.Background(), "echo", map[string]interface{}{}); err == nil {
		t.Fatalf("expected required param error")
	}
	if _, err := gateway.Execute(context.Background(), "echo", map[string]interface{}{"text": 1}); err == nil {
		t.Fatalf("expected type validation error")
	}
	result, err := gateway.Execute(context.Background(), "echo", map[string]interface{}{"text": "ok"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ok" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestToolGateway_Timeout(t *testing.T) {
	manager := NewToolManager()
	manager.Register(&gatewayMockTool{
		name:     "slow",
		params:   map[string]Property{},
		required: []string{},
		runFn: func(ctx context.Context, args map[string]interface{}) (string, error) {
			select {
			case <-time.After(100 * time.Millisecond):
				return "late", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	})

	gateway := NewToolGateway(manager, 10*time.Millisecond, nil)
	_, err := gateway.Execute(context.Background(), "slow", map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestToolGateway_AuditEvents(t *testing.T) {
	manager := NewToolManager()
	manager.Register(&gatewayMockTool{
		name:     "ok",
		params:   map[string]Property{},
		required: []string{},
		runFn: func(ctx context.Context, args map[string]interface{}) (string, error) {
			return "done", nil
		},
	})

	events := make([]AuditEvent, 0, 2)
	gateway := NewToolGateway(manager, time.Second, func(event AuditEvent) {
		events = append(events, event)
	})

	_, err := gateway.Execute(context.Background(), "ok", map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) < 2 {
		t.Fatalf("expected at least 2 audit events, got %d", len(events))
	}
	if events[0].Status != AuditStatusStarted {
		t.Fatalf("expected first event started, got %s", events[0].Status)
	}
	if events[len(events)-1].Status != AuditStatusSuccess {
		t.Fatalf("expected last event success, got %s", events[len(events)-1].Status)
	}
}
