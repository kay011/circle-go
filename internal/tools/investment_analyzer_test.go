package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestInvestmentAnalyzerTool_Metadata(t *testing.T) {
	tool := NewInvestmentAnalyzerTool()
	if tool.Name() != "investment_analyzer" {
		t.Fatalf("unexpected tool name: %s", tool.Name())
	}
	if len(tool.Required()) != 1 || tool.Required()[0] != "name_or_code" {
		t.Fatalf("unexpected required args: %#v", tool.Required())
	}
}

func TestInvestmentAnalyzerTool_EmptyInput(t *testing.T) {
	tool := NewInvestmentAnalyzerTool()
	_, err := tool.Run(context.Background(), map[string]interface{}{"name_or_code": ""})
	if err == nil {
		t.Fatal("expected error for empty name_or_code")
	}
}

func TestInvestmentAnalyzerTool_InvalidAssetType(t *testing.T) {
	tool := NewInvestmentAnalyzerTool()
	_, err := tool.Run(context.Background(), map[string]interface{}{
		"name_or_code": "600519",
		"asset_type":   "crypto",
	})
	if err == nil {
		t.Fatal("expected error for invalid asset_type")
	}
}

func TestInvestmentHelpers(t *testing.T) {
	if got := inferSecID("600519", "1"); got != "1.600519" {
		t.Fatalf("inferSecID mismatch: %s", got)
	}
	if got := inferSecID("000001", "0"); got != "0.000001" {
		t.Fatalf("inferSecID mismatch: %s", got)
	}
	if got := parseNumeric("12.34%"); got != 12.34 {
		t.Fatalf("parseNumber mismatch: %v", got)
	}
	if got := clamp(120, 0, 100); got != 100 {
		t.Fatalf("clamp mismatch: %v", got)
	}
	if verdict := investmentVerdict(80); verdict != "较高" {
		t.Fatalf("investmentVerdict mismatch: %s", verdict)
	}
	if s := investmentSummary(50, "fund"); !strings.Contains(s, "观望") {
		t.Fatalf("unexpected summary: %s", s)
	}
	if k := buildAnalysisCacheKey("600519", "stock"); k != "stock|600519" {
		t.Fatalf("unexpected cache key: %s", k)
	}
	fundJSON := `jsonpgz({"fundcode":"161725","name":"招商中证白酒指数(LOF)A","jzrq":"2026-04-23","dwjz":"0.6372","gsz":"0.6397","gszzl":"0.39","gztime":"2026-04-24 15:00"});`
	p, err := parseFundGZPayload(fundJSON)
	if err != nil {
		t.Fatalf("parseFundGZPayload error: %v", err)
	}
	if p.FundCode != "161725" || p.Name == "" {
		t.Fatalf("unexpected fund payload: %#v", p)
	}
	if got := peScore(16.76); got < 70 {
		t.Fatalf("unexpected pe score: %v", got)
	}
}

func TestInvestmentAnalyzerCache(t *testing.T) {
	tool := NewInvestmentAnalyzerTool().(*investmentAnalyzerTool)
	key := "stock|600519"
	tool.setCached(key, "report")

	got, ok := tool.getCached(key)
	if !ok || got != "report" {
		t.Fatalf("expected cache hit, got ok=%v val=%q", ok, got)
	}

	tool.mu.Lock()
	tool.cache[key] = cachedAnalysisResult{report: "expired", expiresAt: time.Now().Add(-time.Second)}
	tool.mu.Unlock()

	_, ok = tool.getCached(key)
	if ok {
		t.Fatal("expected cache miss for expired item")
	}
}
