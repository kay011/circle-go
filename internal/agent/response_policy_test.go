package agent

import (
	"context"
	"testing"
	"time"
)

func TestResponsePolicyShouldSummarizeHybrid(t *testing.T) {
	p := NewResponsePolicyEngine(nil, ResponsePolicyConfig{
		Mode:             ResponseModeHybrid,
		SummarizeTimeout: 2 * time.Second,
	})
	if !p.ShouldSummarize("请详细总结这个结果", ToolExecutionOutcome{ToolResult: "ok"}) {
		t.Fatalf("expected summarize for explicit summarize request")
	}
	if p.ShouldSummarize("给我结果", ToolExecutionOutcome{ToolResult: "ok"}) {
		t.Fatalf("expected direct for short normal tool result")
	}
}

func TestResponsePolicyBuildFinalDirect(t *testing.T) {
	p := NewResponsePolicyEngine(nil, ResponsePolicyConfig{
		Mode:             ResponseModeDirect,
		SummarizeTimeout: 2 * time.Second,
	})
	final, summarized := p.BuildFinal(context.Background(), "查询", ToolExecutionOutcome{
		ToolName:   "investment_analyzer",
		ToolResult: "raw-result",
	})
	if summarized {
		t.Fatalf("expected direct path")
	}
	if final != "raw-result" {
		t.Fatalf("unexpected final: %s", final)
	}
}
