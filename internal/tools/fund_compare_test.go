package tools

import (
	"strings"
	"testing"
)

func TestSplitFundCandidates(t *testing.T) {
	got := splitFundCandidates("161725，易方达蓝筹; 招商中证白酒\n景顺长城")
	if len(got) != 4 {
		t.Fatalf("expected 4 items, got %d", len(got))
	}
}

func TestParseFundCompareFromJS(t *testing.T) {
	raw := `
var fS_name = "测试基金";
var fS_code = "123456";
var ftype = "混合型";
var ESTABDATE = 1585699200000;
var syl_1n = "12.34";
var syl_3y = "28.80";
var syl_5y = "66.10";
var Data_performanceEvaluation = {"sharp":"1.21","maxDown":"18.7"};
var Data_fluctuationScale = {"series":[{"y":52.3}]};
var Data_rateInSimilarType = {"series":[["2024-12-31",12,180]]};
`
	item, err := parseFundCompareFromJS(raw, "123456", "fallback")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Name != "测试基金" {
		t.Fatalf("unexpected name: %s", item.Name)
	}
	if item.Type != "混合型" {
		t.Fatalf("unexpected type: %s", item.Type)
	}
	if item.Return1Y != 12.34 {
		t.Fatalf("unexpected 1Y return: %v", item.Return1Y)
	}
	if !item.HasReturn1Y || !item.HasReturn3Y || !item.HasReturn5Y {
		t.Fatalf("expected all return windows to be present")
	}
	if item.Return3Y != 28.80 {
		t.Fatalf("unexpected 3Y return: %v", item.Return3Y)
	}
	if item.Return5Y != 66.10 {
		t.Fatalf("unexpected 5Y return: %v", item.Return5Y)
	}
	if item.RankInType != "12/180" {
		t.Fatalf("unexpected rank: %s", item.RankInType)
	}
	if item.Sharpe != 1.21 {
		t.Fatalf("unexpected sharpe: %v", item.Sharpe)
	}
	if item.MaxDrawdown != 18.7 {
		t.Fatalf("unexpected max drawdown: %v", item.MaxDrawdown)
	}
	if item.ScaleBillion != 52.3 {
		t.Fatalf("unexpected scale: %v", item.ScaleBillion)
	}
}

func TestBuildFundCompareReport(t *testing.T) {
	items := []fundCompareItem{
		{Name: "A", Code: "1", Type: "混合", EstablishDate: "2020-01-01", Return1Y: 10, Return3Y: 22, Return5Y: 40, RankInType: "12/180", Sharpe: 1.1, MaxDrawdown: 15, ScaleBillion: 20, Score: 80},
		{Name: "B", Code: "2", Type: "股票", EstablishDate: "2018-01-01", Return1Y: 8, Return3Y: 15, Return5Y: 33, RankInType: "45/180", Sharpe: 0.9, MaxDrawdown: 22, ScaleBillion: 50, Score: 70},
	}
	report := buildFundCompareReport(items)
	if report == "" {
		t.Fatal("expected non-empty report")
	}
	if !(strings.Contains(report, "基金对比结果") && strings.Contains(report, "综合评分最高")) {
		t.Fatalf("report missing key sections: %s", report)
	}
}

func TestFormatPercentOrNA(t *testing.T) {
	if got := formatPercentOrNA(12.3, true); got != "12.30%" {
		t.Fatalf("unexpected percent format: %s", got)
	}
	if got := formatPercentOrNA(0, false); got != "N/A" {
		t.Fatalf("expected N/A, got %s", got)
	}
}
