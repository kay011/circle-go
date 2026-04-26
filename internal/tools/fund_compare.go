package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const fundCompareCacheTTL = 15 * time.Minute

var jsVarPattern = regexp.MustCompile(`(?s)\b([A-Za-z0-9_]+)\s*=\s*(.*?);`)

// NewFundCompareTool 对比多个基金的关键指标并给出量化排序。
func NewFundCompareTool() Tool {
	return &fundCompareTool{
		client: &http.Client{Timeout: 20 * time.Second},
		cache:  make(map[string]cachedFundCompare),
	}
}

type fundCompareTool struct {
	client *http.Client
	mu     sync.RWMutex
	cache  map[string]cachedFundCompare
}

type cachedFundCompare struct {
	report    string
	expiresAt time.Time
}

type fundCompareItem struct {
	Code          string
	Name          string
	Type          string
	EstablishDate string
	Return1Y      float64
	Return3Y      float64
	Return5Y      float64
	HasReturn1Y   bool
	HasReturn3Y   bool
	HasReturn5Y   bool
	RankInType    string
	Sharpe        float64
	MaxDrawdown   float64
	ScaleBillion  float64
	Score         float64
}

func (t *fundCompareTool) Name() string {
	return "fund_compare"
}

func (t *fundCompareTool) Description() string {
	return "对多个基金进行量化对比（成立时间、近1年收益率、夏普比率、最大回撤、规模、类型）并输出排序建议。"
}

func (t *fundCompareTool) Metadata() ToolMetadata {
	return ToolMetadata{
		ID:          "investment.fund_compare",
		Version:     "1.0.0",
		IntentTags:  []string{"基金对比", "夏普比率", "最大回撤", "年化收益", "规模", "排名"},
		RiskLevel:   "medium",
		Owner:       "investment-pack",
		DisplayName: "基金对比",
	}
}

func (t *fundCompareTool) Parameters() map[string]Property {
	return map[string]Property{
		"funds": {
			Type:        "string",
			Description: "待对比基金，使用逗号分隔（2-5个），如：易方达蓝筹,招商中证白酒,161725",
		},
	}
}

func (t *fundCompareTool) Required() []string {
	return []string{"funds"}
}

func (t *fundCompareTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	rawFunds := strings.TrimSpace(fmt.Sprint(args["funds"]))
	if rawFunds == "" {
		return "", fmt.Errorf("funds 不能为空")
	}
	names := splitFundCandidates(rawFunds)
	if len(names) < 2 {
		return "", fmt.Errorf("请至少提供 2 个基金进行对比")
	}
	if len(names) > 5 {
		return "", fmt.Errorf("单次最多对比 5 个基金")
	}

	cacheKey := "fund_compare|" + strings.ToLower(strings.Join(names, ","))
	if report, ok := t.getCached(cacheKey); ok {
		return report, nil
	}

	items := make([]fundCompareItem, 0, len(names))
	for _, nameOrCode := range names {
		code, fallbackName, err := t.resolveFundCode(ctx, nameOrCode)
		if err != nil {
			return "", fmt.Errorf("解析基金 %q 失败: %w", nameOrCode, err)
		}
		item, err := t.fetchFundCompareItem(ctx, code, fallbackName)
		if err != nil {
			return "", fmt.Errorf("获取基金 %s(%s) 指标失败: %w", fallbackName, code, err)
		}
		items = append(items, item)
	}

	for i := range items {
		items[i].Score = scoreFundCompareItem(items[i])
	}
	sortFundCompareItems(items)

	report := buildFundCompareReport(items)
	t.setCached(cacheKey, report)
	return report, nil
}

func splitFundCandidates(raw string) []string {
	replacer := strings.NewReplacer("，", ",", "；", ",", ";", ",", "、", ",", "\n", ",")
	parts := strings.Split(replacer.Replace(raw), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (t *fundCompareTool) resolveFundCode(ctx context.Context, query string) (string, string, error) {
	query = strings.TrimSpace(query)
	if codePattern.MatchString(query) {
		return query, query, nil
	}
	u := "https://searchapi.eastmoney.com/api/suggest/get?input=" + url.QueryEscape(query) +
		"&type=8&token=" + eastmoneySuggestToken
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("名称解析失败，状态码: %d", resp.StatusCode)
	}

	var payload struct {
		QuotationCodeTable struct {
			Data []struct {
				Code string `json:"Code"`
				Name string `json:"Name"`
			} `json:"Data"`
		} `json:"QuotationCodeTable"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", "", err
	}
	if len(payload.QuotationCodeTable.Data) == 0 {
		return t.resolveFundCodeByFundSearch(ctx, query)
	}
	code := strings.TrimSpace(payload.QuotationCodeTable.Data[0].Code)
	name := strings.TrimSpace(payload.QuotationCodeTable.Data[0].Name)
	if code == "" {
		return t.resolveFundCodeByFundSearch(ctx, query)
	}
	return code, nonEmpty(name, query), nil
}

func (t *fundCompareTool) resolveFundCodeByFundSearch(ctx context.Context, query string) (string, string, error) {
	u := "https://fundsuggest.eastmoney.com/FundSearch/api/FundSearchAPI.ashx?m=1&key=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("fund search状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	jsonBody := strings.TrimSpace(string(body))
	if strings.HasPrefix(jsonBody, "jsonpgz(") {
		jsonBody = strings.TrimPrefix(jsonBody, "jsonpgz(")
		jsonBody = strings.TrimSuffix(jsonBody, ");")
	}
	var payload struct {
		Datas []struct {
			Code string `json:"CODE"`
			Name string `json:"NAME"`
		} `json:"Datas"`
	}
	if err := json.Unmarshal([]byte(jsonBody), &payload); err != nil {
		return "", "", err
	}
	if len(payload.Datas) == 0 {
		return "", "", fmt.Errorf("未匹配到基金代码")
	}

	q := strings.ReplaceAll(strings.ToLower(query), " ", "")
	bestCode := ""
	bestName := ""
	for _, d := range payload.Datas {
		code := strings.TrimSpace(d.Code)
		name := strings.TrimSpace(d.Name)
		if code == "" {
			continue
		}
		normName := strings.ReplaceAll(strings.ToLower(name), " ", "")
		if q != "" && strings.Contains(normName, q) {
			return code, nonEmpty(name, query), nil
		}
		if bestCode == "" {
			bestCode = code
			bestName = name
		}
	}
	if bestCode == "" {
		return "", "", fmt.Errorf("未匹配到基金代码")
	}
	return bestCode, nonEmpty(bestName, query), nil
}

func (t *fundCompareTool) fetchFundCompareItem(ctx context.Context, code, fallbackName string) (fundCompareItem, error) {
	u := fmt.Sprintf("https://fund.eastmoney.com/pingzhongdata/%s.js?v=%d", url.QueryEscape(code), time.Now().Unix())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fundCompareItem{}, err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/javascript, text/javascript, */*")

	resp, err := t.client.Do(req)
	if err != nil {
		return fundCompareItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fundCompareItem{}, fmt.Errorf("状态码: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fundCompareItem{}, err
	}
	return parseFundCompareFromJS(string(body), code, fallbackName)
}

func parseFundCompareFromJS(raw, code, fallbackName string) (fundCompareItem, error) {
	vars := parseJSVariables(raw)
	name := trimQuote(vars["fS_name"])
	if name == "" {
		name = fallbackName
	}
	ftype := trimQuote(vars["ftype"])
	establish := parseEstablishDate(vars)
	r1, ok1 := parseOptionalFloatMaybePercent(vars["syl_1n"])
	r3, ok3 := parseOptionalFloatMaybePercent(vars["syl_3y"])
	r5, ok5 := parseOptionalFloatMaybePercent(vars["syl_5y"])

	sharpe, maxDD := parsePerformanceEvaluation(vars["Data_performanceEvaluation"])
	scale := parseLatestFundScale(vars["Data_fluctuationScale"])
	rank := parseRateRankInType(vars["Data_rateInSimilarType"])

	return fundCompareItem{
		Code:          code,
		Name:          name,
		Type:          nonEmpty(ftype, "-"),
		EstablishDate: nonEmpty(establish, "-"),
		Return1Y:      r1,
		Return3Y:      r3,
		Return5Y:      r5,
		HasReturn1Y:   ok1,
		HasReturn3Y:   ok3,
		HasReturn5Y:   ok5,
		RankInType:    rank,
		Sharpe:        sharpe,
		MaxDrawdown:   maxDD,
		ScaleBillion:  scale,
	}, nil
}

func parseJSVariables(raw string) map[string]string {
	out := make(map[string]string)
	matches := jsVarPattern.FindAllStringSubmatch(raw, -1)
	for _, m := range matches {
		if len(m) == 3 {
			out[m[1]] = strings.TrimSpace(m[2])
		}
	}
	return out
}

func parseEstablishDate(vars map[string]string) string {
	if v := strings.TrimSpace(vars["ESTABDATE"]); v != "" {
		v = strings.Trim(v, "\"'")
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil && ts > 0 {
			return time.UnixMilli(ts).Format("2006-01-02")
		}
	}
	if v := trimQuote(vars["fS_estabDate"]); v != "" {
		return v
	}
	return ""
}

func parsePerformanceEvaluation(raw string) (sharpe, maxDrawdown float64) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "undefined" {
		return 0, 0
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 0, 0
	}
	// 字段在不同版本里命名可能不同，做多键兼容
	sharpe = firstFloat(payload, "sharp", "sharpRatio", "sharpe")
	maxDrawdown = firstFloat(payload, "maxDown", "maxRetracement", "maxDd")
	return sharpe, maxDrawdown
}

func parseLatestFundScale(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "undefined" {
		return 0
	}
	var payload struct {
		Series interface{} `json:"series"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 0
	}

	switch s := payload.Series.(type) {
	case []interface{}:
		if len(s) == 0 {
			return 0
		}
		last := s[len(s)-1]
		switch v := last.(type) {
		case float64:
			return v
		case map[string]interface{}:
			return firstFloat(v, "y", "value", "data")
		}
	}
	return 0
}

func parseRateRankInType(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "undefined" {
		return "-"
	}
	var payload struct {
		Series interface{} `json:"series"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "-"
	}
	if arr, ok := payload.Series.([]interface{}); ok && len(arr) > 0 {
		last := arr[len(arr)-1]
		if pair, ok := last.([]interface{}); ok && len(pair) >= 3 {
			rank := parseAnyFloat(pair[1])
			total := parseAnyFloat(pair[2])
			if rank > 0 && total > 0 {
				return fmt.Sprintf("%.0f/%.0f", rank, total)
			}
		}
	}
	return "-"
}

func parseAnyFloat(v interface{}) float64 {
	switch vv := v.(type) {
	case float64:
		return vv
	case string:
		return parseFloatMaybePercent(vv)
	default:
		return 0
	}
}

func firstFloat(m map[string]interface{}, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch vv := v.(type) {
			case float64:
				return vv
			case string:
				return parseFloatMaybePercent(vv)
			}
		}
	}
	return 0
}

func parseFloatMaybePercent(s string) float64 {
	s = strings.TrimSpace(strings.Trim(s, "\"'"))
	s = strings.TrimSuffix(s, "%")
	if s == "" || s == "--" || s == "null" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseOptionalFloatMaybePercent(s string) (float64, bool) {
	s = strings.TrimSpace(strings.Trim(s, "\"'"))
	s = strings.TrimSuffix(s, "%")
	if s == "" || s == "--" || s == "null" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func trimQuote(s string) string {
	return strings.Trim(strings.TrimSpace(s), "\"'")
}

func scoreFundCompareItem(item fundCompareItem) float64 {
	r1 := clamp((item.Return1Y+10)/40*100, 0, 100)
	r3 := clamp((item.Return3Y+20)/80*100, 0, 100)
	r5 := clamp((item.Return5Y+30)/120*100, 0, 100)
	var annualScore float64
	var annualWeight float64
	if item.HasReturn1Y {
		annualScore += r1 * 0.5
		annualWeight += 0.5
	}
	if item.HasReturn3Y {
		annualScore += r3 * 0.3
		annualWeight += 0.3
	}
	if item.HasReturn5Y {
		annualScore += r5 * 0.2
		annualWeight += 0.2
	}
	if annualWeight > 0 {
		annualScore /= annualWeight
	} else {
		annualScore = 40
	}
	sharpeScore := clamp((item.Sharpe+0.2)/1.8*100, 0, 100)
	drawdownScore := clamp(100-item.MaxDrawdown*2.5, 0, 100)
	scaleScore := clamp(math.Log10(item.ScaleBillion+1)/3*100, 0, 100)
	return weightedScore(
		component{Name: "收益能力", Score: annualScore, Weight: 0.35},
		component{Name: "夏普比率", Score: sharpeScore, Weight: 0.30},
		component{Name: "最大回撤", Score: drawdownScore, Weight: 0.20},
		component{Name: "基金规模", Score: scaleScore, Weight: 0.15},
	)
}

func sortFundCompareItems(items []fundCompareItem) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Score > items[i].Score {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

func buildFundCompareReport(items []fundCompareItem) string {
	var sb strings.Builder
	sb.WriteString("基金对比结果（按综合评分降序）\n")
	sb.WriteString("| 基金 | 代码 | 类型 | 成立时间 | 1Y | 3Y | 5Y | 同类排名 | 夏普比率 | 最大回撤 | 规模(亿) | 评分 |\n")
	sb.WriteString("|---|---|---|---|---:|---:|---:|---|---:|---:|---:|---:|\n")
	for _, it := range items {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | %s | %.2f | %.2f%% | %.2f | %.1f |\n",
			nonEmpty(it.Name, "-"), nonEmpty(it.Code, "-"), nonEmpty(it.Type, "-"), nonEmpty(it.EstablishDate, "-"),
			formatPercentOrNA(it.Return1Y, it.HasReturn1Y),
			formatPercentOrNA(it.Return3Y, it.HasReturn3Y),
			formatPercentOrNA(it.Return5Y, it.HasReturn5Y),
			it.RankInType, it.Sharpe, it.MaxDrawdown, it.ScaleBillion, it.Score))
	}
	sb.WriteString("\n结论建议:\n")
	if len(items) > 0 {
		best := items[0]
		sb.WriteString(fmt.Sprintf("- 综合评分最高：%s（%s），可作为优先关注标的。\n", best.Name, best.Code))
	}
	sb.WriteString("- 建议结合投资期限、风格偏好与行业暴露做二次筛选。\n")
	sb.WriteString("免责声明: 指标来自公开数据与简化量化模型，仅供研究学习，不构成投资建议。")
	return sb.String()
}

func formatPercentOrNA(v float64, ok bool) string {
	if !ok {
		return "N/A"
	}
	return fmt.Sprintf("%.2f%%", v)
}

func (t *fundCompareTool) getCached(key string) (string, bool) {
	t.mu.RLock()
	entry, ok := t.cache[key]
	t.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		t.mu.Lock()
		delete(t.cache, key)
		t.mu.Unlock()
		return "", false
	}
	return entry.report, true
}

func (t *fundCompareTool) setCached(key, report string) {
	t.mu.Lock()
	t.cache[key] = cachedFundCompare{
		report:    report,
		expiresAt: time.Now().Add(fundCompareCacheTTL),
	}
	t.mu.Unlock()
}
