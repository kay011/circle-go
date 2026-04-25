package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"circle-go/internal/llm"
	"circle-go/internal/tools"
)

type mockLLM struct {
	function func(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error)
}

func (m *mockLLM) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (m *mockLLM) ChatStream(ctx context.Context, messages []llm.Message, callback func(chunk string) error) error {
	return nil
}

func (m *mockLLM) FunctionCall(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error) {
	if m.function != nil {
		return m.function(ctx, messages, tools)
	}
	return "ok", nil, nil
}

type noopTool struct{}

func (t *noopTool) Name() string { return "noop" }
func (t *noopTool) Description() string {
	return "noop"
}
func (t *noopTool) Parameters() map[string]tools.Property {
	return map[string]tools.Property{}
}
func (t *noopTool) Required() []string { return []string{} }
func (t *noopTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	return "ok", nil
}

func newTestAgent(m llm.LLM, maxSteps int) *Agent {
	tm := tools.NewToolManager()
	tm.Register(&noopTool{})
	return NewAgent(m, tm, maxSteps, false)
}

type mockPolicyEngine struct {
	result tools.PolicyResult
}

func (m *mockPolicyEngine) Evaluate(ctx context.Context, toolName string, args map[string]interface{}) tools.PolicyResult {
	return m.result
}

func TestRun_StopsWhenStepBudgetExceeded(t *testing.T) {
	m := &mockLLM{
		function: func(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error) {
			return "", &llm.FunctionCall{Name: "noop", Arguments: map[string]interface{}{}}, nil
		},
	}
	a := newTestAgent(m, 1)
	a.SetRuntimeLimits(1, 10, 30*time.Second)

	resp, err := a.Run(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "处理超时，请尝试简化问题或提供更多信息。" {
		t.Fatalf("unexpected response: %s", resp)
	}
}

func TestRun_StopsWhenToolBudgetExceeded(t *testing.T) {
	m := &mockLLM{
		function: func(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error) {
			return "", &llm.FunctionCall{Name: "noop", Arguments: map[string]interface{}{}}, nil
		},
	}
	a := newTestAgent(m, 5)
	a.SetRuntimeLimits(5, 1, 30*time.Second)

	resp, err := a.Run(context.Background(), "s1", "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp != "处理超时，请尝试简化问题或提供更多信息。" {
		t.Fatalf("unexpected response: %s", resp)
	}
}

func TestRunStream_HumanApprovalApproved(t *testing.T) {
	callCount := 0
	m := &mockLLM{
		function: func(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error) {
			callCount++
			if callCount == 1 {
				return "need tool", &llm.FunctionCall{Name: "noop", Arguments: map[string]interface{}{}}, nil
			}
			return "final answer", nil, nil
		},
	}

	a := newTestAgent(m, 5)
	a.SetRuntimeLimits(5, 5, 30*time.Second)
	a.SetHumanInTheLoop(true)
	a.SetPolicyEngine(&mockPolicyEngine{
		result: tools.PolicyResult{Decision: tools.PolicyRequireApproval, Reason: "test approval"},
	})

	final, err := a.RunStream(context.Background(), "s1", "hello", func(chunk string) error {
		if strings.Contains(chunk, `"tool_call"`) {
			var payload map[string]ToolCallRequest
			if uErr := json.Unmarshal([]byte(chunk), &payload); uErr != nil {
				return uErr
			}
			req := payload["tool_call"]
			go func() {
				time.Sleep(20 * time.Millisecond)
				_ = a.ResolveToolCallApproval("s1", req.ID, req.ApprovalToken, true)
			}()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if final != "final answer" {
		t.Fatalf("unexpected final: %s", final)
	}
}

func TestRunStream_HumanApprovalRejected(t *testing.T) {
	m := &mockLLM{
		function: func(ctx context.Context, messages []llm.Message, tools []llm.Tool) (string, *llm.FunctionCall, error) {
			return "need tool", &llm.FunctionCall{Name: "noop", Arguments: map[string]interface{}{}}, nil
		},
	}

	a := newTestAgent(m, 5)
	a.SetRuntimeLimits(5, 5, 30*time.Second)
	a.SetHumanInTheLoop(true)
	a.SetPolicyEngine(&mockPolicyEngine{
		result: tools.PolicyResult{Decision: tools.PolicyRequireApproval, Reason: "test approval"},
	})

	final, err := a.RunStream(context.Background(), "s1", "hello", func(chunk string) error {
		if strings.Contains(chunk, `"tool_call"`) {
			var payload map[string]ToolCallRequest
			if uErr := json.Unmarshal([]byte(chunk), &payload); uErr != nil {
				return uErr
			}
			req := payload["tool_call"]
			go func() {
				time.Sleep(20 * time.Millisecond)
				_ = a.ResolveToolCallApproval("s1", req.ID, req.ApprovalToken, false)
			}()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(final, "工具调用已取消") {
		t.Fatalf("expected cancel message, got %s", final)
	}
}
