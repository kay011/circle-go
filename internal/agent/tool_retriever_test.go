package agent

import (
	"testing"

	"circle-go/internal/llm"
)

func TestToolRetrieverSelectTopK(t *testing.T) {
	tools := []llm.Tool{
		{Name: "calculator", Description: "执行数学计算"},
		{Name: "web_search", Description: "查询实时信息和新闻"},
		{Name: "investment_analyzer", Description: "股票 基金 投资 分析"},
	}
	r := NewToolRetriever(tools, ToolRetrievalConfig{
		Enabled:       true,
		TopK:          2,
		MinScore:      1,
		FallbackToAll: true,
	})
	got := r.Select("帮我分析下贵州茅台股票")
	if len(got) == 0 {
		t.Fatalf("expected non-empty selected tools")
	}
	if got[0].Name != "investment_analyzer" {
		t.Fatalf("expected investment_analyzer first, got %s", got[0].Name)
	}
	if len(got) > 2 {
		t.Fatalf("expected at most 2 tools, got %d", len(got))
	}
}

func TestToolRetrieverFallbackToAll(t *testing.T) {
	tools := []llm.Tool{
		{Name: "calculator", Description: "执行数学计算"},
		{Name: "web_search", Description: "查询实时信息和新闻"},
	}
	r := NewToolRetriever(tools, ToolRetrievalConfig{
		Enabled:       true,
		TopK:          2,
		MinScore:      3,
		FallbackToAll: true,
	})
	got := r.Select("xyz-no-match")
	if len(got) != len(tools) {
		t.Fatalf("expected fallback all tools, got %d", len(got))
	}
}
