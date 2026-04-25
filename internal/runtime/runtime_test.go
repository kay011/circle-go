package runtime

import (
	"errors"
	"testing"
	"time"
)

func TestRunContext_DefaultsAndOptions(t *testing.T) {
	rc := NewRunContext(
		"s1",
		WithTraceID("trace-x"),
		WithMaxSteps(9),
		WithMaxToolCalls(7),
		WithMaxDuration(3*time.Second),
	)

	if rc.SessionID != "s1" {
		t.Fatalf("unexpected session id: %s", rc.SessionID)
	}
	if rc.TraceID != "trace-x" {
		t.Fatalf("unexpected trace id: %s", rc.TraceID)
	}
	if rc.Limits.MaxSteps != 9 || rc.Limits.MaxToolCalls != 7 || rc.Limits.MaxDuration != 3*time.Second {
		t.Fatalf("unexpected limits: %+v", rc.Limits)
	}
}

func TestRunContext_ValidateBudget(t *testing.T) {
	rc := NewRunContext(
		"s1",
		WithMaxSteps(2),
		WithMaxToolCalls(1),
		WithMaxDuration(5*time.Second),
	)

	if err := rc.ValidateBudget(); err != nil {
		t.Fatalf("unexpected initial error: %v", err)
	}

	rc.IncStep()
	if err := rc.ValidateBudget(); err != nil {
		t.Fatalf("unexpected error after one step: %v", err)
	}

	rc.IncStep()
	if !errors.Is(rc.ValidateBudget(), ErrStepBudgetExceeded) {
		t.Fatalf("expected step budget exceeded")
	}
}

func TestRunContext_ValidateToolBudgetAndDuration(t *testing.T) {
	rc := NewRunContext(
		"s1",
		WithMaxSteps(10),
		WithMaxToolCalls(1),
		WithMaxDuration(20*time.Millisecond),
	)

	rc.IncToolCall()
	if !errors.Is(rc.ValidateBudget(), ErrToolBudgetExceeded) {
		t.Fatalf("expected tool call budget exceeded")
	}

	rc = NewRunContext(
		"s2",
		WithMaxDuration(1*time.Millisecond),
	)
	time.Sleep(2 * time.Millisecond)
	if !errors.Is(rc.ValidateBudget(), ErrDurationBudgetExceeded) {
		t.Fatalf("expected duration budget exceeded")
	}
}
