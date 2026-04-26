package tools

import (
	"bytes"
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

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const eastmoneySuggestToken = "D43BF722C8E33BDC906FB84D85E326E8"
const investmentAnalyzerCacheTTL = 10 * time.Minute

var codePattern = regexp.MustCompile(`^\d{6}$`)

// NewInvestmentAnalyzerTool 提供股票/基金基础信息与量化评分。
func NewInvestmentAnalyzerTool() Tool {
	return &investmentAnalyzerTool{
		client: &http.Client{Timeout: 20 * time.Second},
		cache:  make(map[string]cachedAnalysisResult),
	}
}

type investmentAnalyzerTool struct {
	client *http.Client
	mu     sync.RWMutex
	cache  map[string]cachedAnalysisResult
}

type cachedAnalysisResult struct {
	report    string
	expiresAt time.Time
}

type stockQuoteData struct {
	Code        string
	Name        string
	Latest      float64
	ChangePct   float64
	Amplitude   float64
	PETTM       float64
	PEAvailable bool
}

func (t *investmentAnalyzerTool) Name() string {
	return "investment_analyzer"
}

func (t *investmentAnalyzerTool) Description() string {
	return "根据股票或基金名称/代码获取公开市场信息，并给出量化投资价值评分（仅供研究，不构成投资建议）。"
}

func (t *investmentAnalyzerTool) Metadata() ToolMetadata {
	return ToolMetadata{
		ID:          "investment.analyzer",
		Version:     "1.0.0",
		IntentTags:  []string{"股票", "基金", "投资", "估值", "pe", "量化评分"},
		RiskLevel:   "medium",
		Owner:       "investment-pack",
		DisplayName: "投资分析",
	}
}

func (t *investmentAnalyzerTool) Parameters() map[string]Property {
	return map[string]Property{
		"name_or_code": {
			Type:        "string",
			Description: "股票或基金名称/代码，例如：贵州茅台、600519、易方达蓝筹、161725",
		},
		"asset_type": {
			Type:        "string",
			Description: "可选：auto|stock|fund，默认 auto",
		},
	}
}

func (t *investmentAnalyzerTool) Required() []string {
	return []string{"name_or_code"}
}

func (t *investmentAnalyzerTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	query := strings.TrimSpace(fmt.Sprint(args["name_or_code"]))
	if query == "" {
		return "", fmt.Errorf("name_or_code 不能为空")
	}
	assetType := strings.ToLower(strings.TrimSpace(fmt.Sprint(args["asset_type"])))
	if assetType == "" || assetType == "<nil>" {
		assetType = "auto"
	}
	if assetType != "auto" && assetType != "stock" && assetType != "fund" {
		return "", fmt.Errorf("asset_type 仅支持 auto、stock、fund")
	}

	cacheKey := buildAnalysisCacheKey(query, assetType)
	if report, ok := t.getCached(cacheKey); ok {
		return report, nil
	}

	report, err := t.runAnalyze(ctx, query, assetType)
	if err != nil {
		return "", err
	}
	t.setCached(cacheKey, report)
	return report, nil
}

func (t *investmentAnalyzerTool) runAnalyze(ctx context.Context, query, assetType string) (string, error) {
	if assetType == "auto" {
		// auto 模式优先基金，失败后自动回退股票。
		resolvedFund, errFund := t.resolveTarget(ctx, query, "fund")
		if errFund == nil {
			if report, err := t.analyzeFund(ctx, resolvedFund.Code, resolvedFund.Name); err == nil {
				return report, nil
			}
		}

		resolvedStock, errStock := t.resolveTarget(ctx, query, "stock")
		if errStock == nil {
			return t.analyzeStock(ctx, resolvedStock.Code, resolvedStock.Name, resolvedStock.Market)
		}
		if errFund != nil {
			return "", errFund
		}
		return "", errStock
	}

	resolved, err := t.resolveTarget(ctx, query, assetType)
	if err != nil {
		return "", err
	}
	if resolved.AssetType == "fund" {
		return t.analyzeFund(ctx, resolved.Code, resolved.Name)
	}
	return t.analyzeStock(ctx, resolved.Code, resolved.Name, resolved.Market)
}

func buildAnalysisCacheKey(query, assetType string) string {
	return strings.ToLower(strings.TrimSpace(assetType)) + "|" + strings.ToLower(strings.TrimSpace(query))
}

func (t *investmentAnalyzerTool) getCached(key string) (string, bool) {
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

func (t *investmentAnalyzerTool) setCached(key, report string) {
	t.mu.Lock()
	t.cache[key] = cachedAnalysisResult{
		report:    report,
		expiresAt: time.Now().Add(investmentAnalyzerCacheTTL),
	}
	t.mu.Unlock()
}

type resolvedTarget struct {
	AssetType string
	Code      string
	Name      string
	Market    string
}

func (t *investmentAnalyzerTool) resolveTarget(ctx context.Context, query, assetType string) (resolvedTarget, error) {
	if codePattern.MatchString(query) {
		if assetType == "fund" {
			return resolvedTarget{AssetType: "fund", Code: query, Name: query}, nil
		}
		if assetType == "stock" {
			return resolvedTarget{AssetType: "stock", Code: query, Name: query, Market: inferStockMarket(query)}, nil
		}
		// auto 模式优先基金（基金代码与股票代码均是 6 位，基金数据更稳定）
		return resolvedTarget{AssetType: "fund", Code: query, Name: query}, nil
	}

	if assetType == "fund" || assetType == "auto" {
		if rt, err := t.resolveByEastmoneySuggest(ctx, query, "8"); err == nil {
			rt.AssetType = "fund"
			return rt, nil
		}
	}
	if assetType == "stock" || assetType == "auto" {
		if rt, err := t.resolveByEastmoneySuggest(ctx, query, "14"); err == nil {
			rt.AssetType = "stock"
			rt.Market = inferStockMarket(rt.Code)
			return rt, nil
		}
	}

	return resolvedTarget{}, fmt.Errorf("无法解析名称“%s”，请尝试直接输入 6 位代码", query)
}

func (t *investmentAnalyzerTool) resolveByEastmoneySuggest(ctx context.Context, query, typ string) (resolvedTarget, error) {
	u := "https://searchapi.eastmoney.com/api/suggest/get?input=" + url.QueryEscape(query) +
		"&type=" + url.QueryEscape(typ) + "&token=" + eastmoneySuggestToken
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return resolvedTarget{}, err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return resolvedTarget{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resolvedTarget{}, fmt.Errorf("名称解析失败，状态码: %d", resp.StatusCode)
	}

	var payload struct {
		QuotationCodeTable struct {
			Data []struct {
				Code   string `json:"Code"`
				Name   string `json:"Name"`
				MktNum string `json:"MktNum"`
			} `json:"Data"`
		} `json:"QuotationCodeTable"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return resolvedTarget{}, err
	}
	if len(payload.QuotationCodeTable.Data) == 0 {
		return resolvedTarget{}, fmt.Errorf("no match")
	}
	first := payload.QuotationCodeTable.Data[0]
	return resolvedTarget{
		Code:   strings.TrimSpace(first.Code),
		Name:   strings.TrimSpace(first.Name),
		Market: strings.TrimSpace(first.MktNum),
	}, nil
}

func (t *investmentAnalyzerTool) analyzeFund(ctx context.Context, code, fallbackName string) (string, error) {
	u := "https://fundgz.1234567.com.cn/js/" + url.QueryEscape(code) + ".js"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "application/javascript, text/javascript, */*")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("基金数据获取失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("基金数据接口异常，状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	fund, err := parseFundGZPayload(string(body))
	if err != nil {
		return "", err
	}

	name := strings.TrimSpace(fund.Name)
	if name == "" {
		name = fallbackName
	}
	estChange := parseNumeric(fund.EstimateChangePercent)
	netWorth := parseNumeric(fund.NetWorth)
	estimateWorth := parseNumeric(fund.EstimateWorth)
	var premiumPct float64
	if netWorth > 0 {
		premiumPct = (estimateWorth - netWorth) / netWorth * 100
	}

	// 基于可获取的实时估值数据做稳健评分（MVP）
	returnScore := clamp((estChange+2.5)/5*100, 0, 100)
	riskScore := clamp(100-math.Abs(estChange)*20, 0, 100)
	costScore := clamp(100-math.Abs(premiumPct)*25, 0, 100)
	total := weightedScore(
		component{"收益动量", returnScore, 0.45},
		component{"波动稳定", riskScore, 0.35},
		component{"估值偏离", costScore, 0.20},
	)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("资产类型: 基金\n名称: %s\n代码: %s\n", name, nonEmpty(fund.FundCode, code)))
	sb.WriteString(fmt.Sprintf("单位净值(上日): %s (日期: %s)\n", nonEmpty(fund.NetWorth, "-"), nonEmpty(fund.NetWorthDate, "-")))
	sb.WriteString(fmt.Sprintf("实时估值: %s (时间: %s)\n", nonEmpty(fund.EstimateWorth, "-"), nonEmpty(fund.EstimateTime, "-")))
	sb.WriteString("收益指标:\n")
	sb.WriteString(fmt.Sprintf("- 实时涨跌幅: %s%%\n- 估值偏离净值: %.2f%%\n", nonEmpty(fund.EstimateChangePercent, "-"), premiumPct))
	sb.WriteString("量化评分(0-100):\n")
	sb.WriteString(fmt.Sprintf("- 收益动量: %.1f\n- 波动稳定: %.1f\n- 估值偏离: %.1f\n", returnScore, riskScore, costScore))
	sb.WriteString(fmt.Sprintf("综合评分: %.1f (%s)\n", total, investmentVerdict(total)))
	sb.WriteString(fmt.Sprintf("结论: %s\n", investmentSummary(total, "fund")))
	sb.WriteString("免责声明: 以上分析基于公开行情与简化模型，仅供研究学习，不构成投资建议。")
	return sb.String(), nil
}

func (t *investmentAnalyzerTool) analyzeStock(ctx context.Context, code, fallbackName, market string) (string, error) {
	secid := inferSecID(code, market)
	quote, err := t.fetchStockQuote(ctx, secid, code, fallbackName)
	if err != nil {
		return "", err
	}

	trendScore := clamp((quote.ChangePct+10)/20*100, 0, 100)
	stabilityScore := clamp(100-quote.Amplitude*8, 0, 100)
	valuationScore := 50.0
	if quote.PEAvailable {
		valuationScore = peScore(quote.PETTM)
	}
	total := weightedScore(
		component{"趋势动量", trendScore, 0.4},
		component{"波动稳定", stabilityScore, 0.3},
		component{"估值水平", valuationScore, 0.3},
	)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("资产类型: 股票\n名称: %s\n代码: %s\n", nonEmpty(quote.Name, fallbackName), nonEmpty(quote.Code, code)))
	sb.WriteString(fmt.Sprintf("最新价: %.2f\n", quote.Latest))
	sb.WriteString(fmt.Sprintf("当日涨跌幅: %.2f%%\n振幅: %.2f%%\n", quote.ChangePct, quote.Amplitude))
	if quote.PEAvailable {
		sb.WriteString(fmt.Sprintf("PE(TTM): %.2f\n", quote.PETTM))
	} else {
		sb.WriteString("PE(TTM): 暂不可用\n")
	}
	sb.WriteString("量化评分(0-100):\n")
	sb.WriteString(fmt.Sprintf("- 趋势动量: %.1f\n- 波动稳定: %.1f\n- 估值水平: %.1f\n", trendScore, stabilityScore, valuationScore))
	sb.WriteString(fmt.Sprintf("综合评分: %.1f (%s)\n", total, investmentVerdict(total)))
	sb.WriteString(fmt.Sprintf("结论: %s\n", investmentSummary(total, "stock")))
	sb.WriteString("免责声明: 以上分析基于公开行情与简化模型，仅供研究学习，不构成投资建议。")
	return sb.String(), nil
}

func (t *investmentAnalyzerTool) fetchStockQuote(ctx context.Context, secid, code, fallbackName string) (stockQuoteData, error) {
	body, err := t.fetchStockQuoteWithRetry(ctx, secid)
	if err == nil {
		var payload struct {
			Data map[string]interface{} `json:"data"`
		}
		if json.Unmarshal(body, &payload) == nil && len(payload.Data) > 0 {
			peRaw := toFloat(payload.Data["f162"])
			pe := peRaw / 100.0
			return stockQuoteData{
				Code:        nonEmpty(toString(payload.Data["f57"]), code),
				Name:        nonEmpty(toString(payload.Data["f58"]), fallbackName),
				Latest:      toFloat(payload.Data["f43"]) / 100.0,
				ChangePct:   toFloat(payload.Data["f170"]) / 100.0,
				Amplitude:   toFloat(payload.Data["f171"]) / 100.0,
				PETTM:       pe,
				PEAvailable: pe > 0 && pe < 300,
			}, nil
		}
	}

	// 备用源：腾讯行情接口（主源 5xx 时兜底）
	q, ferr := t.fetchStockQuoteFromTencent(ctx, code)
	if ferr != nil {
		if err != nil {
			return stockQuoteData{}, fmt.Errorf("%v；备用源也失败: %w", err, ferr)
		}
		return stockQuoteData{}, ferr
	}
	if strings.TrimSpace(q.Name) == "" {
		q.Name = fallbackName
	}
	return q, nil
}

func (t *investmentAnalyzerTool) fetchStockQuoteWithRetry(ctx context.Context, secid string) ([]byte, error) {
	u := "https://push2.eastmoney.com/api/qt/stock/get?secid=" + url.QueryEscape(secid) +
		"&fields=f57,f58,f43,f170,f171,f162"
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
		req.Header.Set("Accept", "application/json")

		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("股票数据获取失败: %w", err)
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("股票数据读取失败: %w", readErr)
			} else if resp.StatusCode == http.StatusOK {
				return body, nil
			} else {
				lastErr = fmt.Errorf("股票数据接口异常，状态码: %d", resp.StatusCode)
				if resp.StatusCode < 500 || resp.StatusCode >= 600 {
					// 非 5xx 不重试，直接返回。
					return nil, lastErr
				}
			}
		}

		if attempt < 3 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("股票数据临时不可用（已重试3次）: %w", lastErr)
}

func (t *investmentAnalyzerTool) fetchStockQuoteFromTencent(ctx context.Context, code string) (stockQuoteData, error) {
	symbol := "sz" + code
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") {
		symbol = "sh" + code
	}
	u := "https://qt.gtimg.cn/q=" + url.QueryEscape(symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return stockQuoteData{}, err
	}
	req.Header.Set("User-Agent", "Circle-Go-Agent/1.0")
	req.Header.Set("Accept", "text/plain, */*")

	resp, err := t.client.Do(req)
	if err != nil {
		return stockQuoteData{}, fmt.Errorf("腾讯行情接口请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return stockQuoteData{}, fmt.Errorf("腾讯行情接口异常，状态码: %d", resp.StatusCode)
	}
	rawBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return stockQuoteData{}, err
	}
	raw, err := decodeGBK(rawBytes)
	if err != nil {
		raw = string(rawBytes)
	}
	start := strings.Index(raw, "\"")
	end := strings.LastIndex(raw, "\"")
	if start < 0 || end <= start {
		return stockQuoteData{}, fmt.Errorf("腾讯行情返回格式异常")
	}
	parts := strings.Split(raw[start+1:end], "~")
	if len(parts) < 45 {
		return stockQuoteData{}, fmt.Errorf("腾讯行情字段不足")
	}
	name := strings.TrimSpace(parts[1])
	latest := parseNumeric(parts[3])
	changePct := parseNumeric(parts[32])
	prevClose := parseNumeric(parts[4])
	high := parseNumeric(parts[33])
	low := parseNumeric(parts[34])
	amplitude := 0.0
	if prevClose > 0 && high > 0 && low > 0 && high >= low {
		amplitude = (high - low) / prevClose * 100
	}
	if amplitude < 0 || amplitude > 30 {
		amplitude = 0
	}
	pe := parseNumeric(parts[39])
	return stockQuoteData{
		Code:        strings.TrimSpace(parts[2]),
		Name:        name,
		Latest:      latest,
		ChangePct:   changePct,
		Amplitude:   amplitude,
		PETTM:       pe,
		PEAvailable: pe > 0 && pe < 300,
	}, nil
}

func decodeGBK(data []byte) (string, error) {
	reader := transform.NewReader(bytes.NewReader(data), simplifiedchinese.GBK.NewDecoder())
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

type component struct {
	Name   string
	Score  float64
	Weight float64
}

func weightedScore(cs ...component) float64 {
	var s float64
	for _, c := range cs {
		s += c.Score * c.Weight
	}
	return s
}

func investmentVerdict(score float64) string {
	switch {
	case score >= 75:
		return "较高"
	case score >= 60:
		return "中等"
	default:
		return "偏低"
	}
}

func investmentSummary(score float64, assetType string) string {
	switch {
	case score >= 75:
		return "短中期量化指标较优，可继续结合行业/基本面做二次验证。"
	case score >= 60:
		return "指标中性，建议观察估值与趋势确认信号后再决策。"
	default:
		if assetType == "fund" {
			return "当前收益风险比不理想，建议先观望或对比同类基金。"
		}
		return "当前动量与估值组合一般，建议降低仓位或继续等待更好入场点。"
	}
}

func peScore(pe float64) float64 {
	if pe <= 0 {
		return 40
	}
	if pe <= 15 {
		return 90
	}
	if pe <= 25 {
		return 75
	}
	if pe <= 40 {
		return 55
	}
	if pe <= 80 {
		return 35
	}
	return 20
}

func inferSecID(code, market string) string {
	m := strings.TrimSpace(market)
	if m == "1" || strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") {
		return "1." + code
	}
	return "0." + code
}

func inferStockMarket(code string) string {
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") {
		return "1"
	}
	return "0"
}

func parseNumeric(v string) float64 {
	v = strings.TrimSpace(strings.TrimSuffix(v, "%"))
	if v == "" || v == "--" || v == "null" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		return parseNumeric(n)
	default:
		return 0
	}
}

func toString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

type fundGZPayload struct {
	FundCode              string `json:"fundcode"`
	Name                  string `json:"name"`
	NetWorthDate          string `json:"jzrq"`
	NetWorth              string `json:"dwjz"`
	EstimateWorth         string `json:"gsz"`
	EstimateChangePercent string `json:"gszzl"`
	EstimateTime          string `json:"gztime"`
}

func parseFundGZPayload(raw string) (fundGZPayload, error) {
	s := strings.TrimSpace(raw)
	start := strings.Index(s, "(")
	end := strings.LastIndex(s, ")")
	if start < 0 || end <= start {
		return fundGZPayload{}, fmt.Errorf("基金数据格式异常")
	}
	jsonPart := strings.TrimSpace(s[start+1 : end])
	var payload fundGZPayload
	if err := json.Unmarshal([]byte(jsonPart), &payload); err != nil {
		return fundGZPayload{}, fmt.Errorf("基金数据解析失败: %w", err)
	}
	if strings.TrimSpace(payload.FundCode) == "" {
		return fundGZPayload{}, fmt.Errorf("基金数据为空")
	}
	return payload, nil
}
