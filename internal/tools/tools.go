package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"circle-go/internal/logging"

	"github.com/PuerkitoBio/goquery"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]Property
	Required() []string
	Run(ctx context.Context, args map[string]interface{}) (string, error)
}

// Property 属性定义
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolManager 工具管理器
type ToolManager struct {
	tools map[string]Tool
}

// NewToolManager 创建工具管理器
func NewToolManager() *ToolManager {
	return &ToolManager{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (tm *ToolManager) Register(tool Tool) {
	tm.tools[tool.Name()] = tool
}

// Get 获取工具
func (tm *ToolManager) Get(name string) Tool {
	return tm.tools[name]
}

// List 列出所有工具
func (tm *ToolManager) List() []Tool {
	tools := make([]Tool, 0, len(tm.tools))
	for _, tool := range tm.tools {
		tools = append(tools, tool)
	}
	return tools
}

// Run 运行工具
func (tm *ToolManager) Run(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	tool, ok := tm.tools[name]
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	// 验证参数
	required := tool.Required()
	for _, req := range required {
		if _, exists := args[req]; !exists {
			return "", fmt.Errorf("missing required parameter: %s", req)
		}
	}

	return tool.Run(ctx, args)
}

// ToLLMTools 转换为LLM工具格式
func (tm *ToolManager) ToLLMTools() []struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
} {
	llmTools := make([]struct {
		Name        string                 `json:"name"`
		Description string                 `json:"description"`
		Parameters  map[string]interface{} `json:"parameters"`
	}, 0, len(tm.tools))

	for _, tool := range tm.tools {
		parameters := map[string]interface{}{
			"type":       "object",
			"properties": tool.Parameters(),
		}

		required := tool.Required()
		if len(required) > 0 {
			parameters["required"] = required
		}

		llmTools = append(llmTools, struct {
			Name        string                 `json:"name"`
			Description string                 `json:"description"`
			Parameters  map[string]interface{} `json:"parameters"`
		}{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  parameters,
		})
	}

	return llmTools
}

// 内置工具：计算器
func NewCalculatorTool() Tool {
	return &calculatorTool{}
}

type calculatorTool struct{}

func (t *calculatorTool) Name() string {
	return "calculator"
}

func (t *calculatorTool) Description() string {
	return "执行数学计算"
}

func (t *calculatorTool) Parameters() map[string]Property {
	return map[string]Property{
		"expression": {
			Type:        "string",
			Description: "数学表达式，例如：2 + 3 * 4",
		},
	}
}

func (t *calculatorTool) Required() []string {
	return []string{"expression"}
}

func (t *calculatorTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	expression, ok := args["expression"].(string)
	if !ok {
		return "", fmt.Errorf("invalid expression type")
	}

	// 简单的表达式计算（实际应用中应使用更安全的计算库）
	// 这里仅作为示例
	result := evaluateExpression(expression)
	return fmt.Sprintf("计算结果: %v", result), nil
}

// 简单的表达式计算函数（支持运算符优先级）
func evaluateExpression(expr string) interface{} {
	// 这里实现支持运算符优先级的表达式计算
	expr = strings.TrimSpace(expr)

	// 移除所有空格
	expr = strings.ReplaceAll(expr, " ", "")

	if len(expr) == 0 {
		return "错误：空表达式"
	}

	// 处理括号
	for strings.Contains(expr, "(") {
		// 找到最内层的括号
		end := strings.Index(expr, ")")
		if end == -1 {
			return "错误：括号不匹配"
		}

		// 从后向前找对应的左括号
		start := strings.LastIndex(expr[:end], "(")
		if start == -1 {
			return "错误：括号不匹配"
		}

		// 计算括号内的表达式
		innerExpr := expr[start+1 : end]
		innerResult := evaluateExpression(innerExpr)

		// 检查结果是否为错误
		if err, ok := innerResult.(string); ok && strings.Contains(err, "错误") {
			return innerResult
		}

		// 将括号替换为计算结果
		var resultStr string
		if num, ok := innerResult.(float64); ok {
			resultStr = fmt.Sprintf("%g", num)
		} else {
			resultStr = fmt.Sprintf("%v", innerResult)
		}

		expr = expr[:start] + resultStr + expr[end+1:]
	}

	// 使用递归下降解析器处理运算符优先级
	pos := 0
	result, err := parseAddSub(expr, &pos)
	if err != nil {
		return err
	}

	// 检查是否还有未处理的字符
	if pos < len(expr) {
		return "错误：无效的表达式"
	}

	return result
}

// parseAddSub 处理加减法（最低优先级）
func parseAddSub(expr string, pos *int) (float64, error) {
	left, err := parseMulDiv(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) && (expr[*pos] == '+' || expr[*pos] == '-') {
		op := expr[*pos]
		*pos++

		right, err := parseMulDiv(expr, pos)
		if err != nil {
			return 0, err
		}

		if op == '+' {
			left = left + right
		} else {
			left = left - right
		}
	}

	return left, nil
}

// parseMulDiv 处理乘除法（较高优先级）
func parseMulDiv(expr string, pos *int) (float64, error) {
	left, err := parseNumber(expr, pos)
	if err != nil {
		return 0, err
	}

	for *pos < len(expr) && (expr[*pos] == '*' || expr[*pos] == '/') {
		op := expr[*pos]
		*pos++

		right, err := parseNumber(expr, pos)
		if err != nil {
			return 0, err
		}

		if op == '*' {
			left = left * right
		} else {
			if right == 0 {
				return 0, fmt.Errorf("错误：除数不能为零")
			}
			left = left / right
		}
	}

	return left, nil
}

// parseNumber 解析数字
func parseNumber(expr string, pos *int) (float64, error) {
	start := *pos

	// 跳过负号（一元运算符）
	if *pos < len(expr) && expr[*pos] == '-' {
		*pos++
	}

	// 解析数字部分
	for *pos < len(expr) && isDigitOrDot(expr[*pos]) {
		*pos++
	}

	if start == *pos {
		return 0, fmt.Errorf("错误：期望数字，但找到了 '%s'", expr[*pos:])
	}

	numStr := expr[start:*pos]
	var num float64
	if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
		return 0, fmt.Errorf("错误：无效的数字 '%s'", numStr)
	}

	return num, nil
}

// isDigitOrDot 检查字符是否是数字或小数点
func isDigitOrDot(c byte) bool {
	return (c >= '0' && c <= '9') || c == '.'
}

// 内置工具：网络搜索（DuckDuckGo HTML/Lite + SearxNG 聚合，均无 API Key）
// searxInstances 为 SearxNG 根 URL 列表（如 https://searx.be）；为空时使用内置公共实例（可能随时间失效，建议在 config 中自行维护）。
func NewWebSearchTool(searxInstances []string) Tool {
	bases := searxInstances
	if len(bases) == 0 {
		bases = defaultSearxBases()
	}
	return &webSearchTool{searxBases: bases}
}

type webSearchTool struct {
	searxBases []string
}

// defaultSearxBases 公共 SearxNG 实例（零 Key）；实例可用性变化快，生产环境请在 config.search.searx_instances 中覆盖。
func defaultSearxBases() []string {
	return []string{
		"https://searx.tiekoetter.com",
		"https://searx.be",
		"https://paulgo.io",
		"https://search.mdosch.de",
	}
}

func (t *webSearchTool) Name() string {
	return "web_search"
}

func (t *webSearchTool) Description() string {
	return "当用户需要查询实时信息、新闻、天气或事实性知识时使用此工具。"
}

func (t *webSearchTool) Parameters() map[string]Property {
	return map[string]Property{
		"query": {
			Type:        "string",
			Description: "搜索关键词",
		},
	}
}

func (t *webSearchTool) Required() []string {
	return []string{"query"}
}

func (t *webSearchTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	logger := logging.NewLogger(logging.INFO, "WebSearchTool")

	query := strings.TrimSpace(fmt.Sprint(args["query"]))
	if query == "" {
		return "", fmt.Errorf("missing or empty query")
	}

	logger.Info("开始网络搜索", map[string]interface{}{"query": query})

	client := &http.Client{Timeout: 25 * time.Second}

	var results []searchHit

	r1, err := duckduckgoHTMLSearch(ctx, client, query)
	if err != nil {
		logger.Warn("DuckDuckGo HTML 搜索失败", map[string]interface{}{"error": err.Error()})
	} else {
		results = r1
	}
	if len(results) == 0 {
		r2, err2 := duckduckgoLiteSearch(ctx, client, query)
		if err2 != nil {
			logger.Warn("DuckDuckGo Lite 搜索失败", map[string]interface{}{"error": err2.Error()})
		} else {
			results = r2
		}
	}
	if len(results) == 0 {
		r3 := searxSearchAggregated(ctx, client, query, t.searxBases, logger)
		results = r3
	}

	if len(results) == 0 {
		return fmt.Sprintf("未找到与 \"%s\" 相关的网页摘要（可能被目标站反爬或暂时无结果）。可换个关键词重试。", query), nil
	}

	const max = 5
	if len(results) > max {
		results = results[:max]
	}
	var lines []string
	for _, r := range results {
		lines = append(lines, fmt.Sprintf("- **%s**: %s\n  [链接](%s)", r.title, r.snippet, r.link))
	}
	return fmt.Sprintf("关于 \"%s\" 的搜索结果:\n\n%s", query, strings.Join(lines, "\n\n")), nil
}

type searchHit struct {
	title, link, snippet string
}

func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7")
}

func duckduckgoHTMLSearch(ctx context.Context, client *http.Client, query string) ([]searchHit, error) {
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	req.Header.Set("Referer", "https://html.duckduckgo.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var out []searchHit
	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		if len(out) >= 8 {
			return
		}
		if s.HasClass("result--ad") {
			return
		}
		a := s.Find("a.result__a").First()
		title := strings.TrimSpace(a.Text())
		link, _ := a.Attr("href")
		snippet := strings.TrimSpace(s.Find("a.result__snippet").Text())
		if snippet == "" {
			snippet = strings.TrimSpace(s.Find(".result__snippet").Text())
		}
		if title != "" && link != "" {
			out = append(out, searchHit{title: title, link: link, snippet: snippet})
		}
	})
	return out, nil
}

func duckduckgoLiteSearch(ctx context.Context, client *http.Client, query string) ([]searchHit, error) {
	u := "https://lite.duckduckgo.com/lite/?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	req.Header.Set("Referer", "https://lite.duckduckgo.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("lite status %d: %s", resp.StatusCode, string(b))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var out []searchHit
	doc.Find("tr.result").Each(func(i int, s *goquery.Selection) {
		if len(out) >= 8 {
			return
		}
		title := strings.TrimSpace(s.Find("a.result-link").Text())
		link, _ := s.Find("a.result-link").Attr("href")
		snippet := strings.TrimSpace(s.Find("td.result-snippet").Text())
		if title != "" && link != "" {
			out = append(out, searchHit{title: title, link: link, snippet: snippet})
		}
	})
	return out, nil
}

// searxSearchAggregated 依次尝试多个 SearxNG 实例：优先 format=json，无结果或失败再解析 HTML。
func searxSearchAggregated(ctx context.Context, client *http.Client, query string, bases []string, logger *logging.Logger) []searchHit {
	for _, raw := range bases {
		base := strings.TrimRight(strings.TrimSpace(raw), "/")
		if base == "" {
			continue
		}
		subCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		hits, err := searxSearchJSON(subCtx, client, base, query)
		cancel()
		if err != nil || len(hits) == 0 {
			if err != nil {
				logger.Warn("Searx JSON 不可用，尝试 HTML", map[string]interface{}{"base": base, "error": err.Error()})
			}
			subCtx2, cancel2 := context.WithTimeout(ctx, 12*time.Second)
			h2, err2 := searxSearchHTML(subCtx2, client, base, query)
			cancel2()
			if err2 != nil {
				logger.Warn("Searx HTML 失败", map[string]interface{}{"base": base, "error": err2.Error()})
				continue
			}
			hits = h2
		}
		if len(hits) > 0 {
			logger.Info("SearxNG 搜索成功", map[string]interface{}{"base": base, "count": len(hits)})
			return hits
		}
	}
	return nil
}

func normalizeSearxURL(base, link string) string {
	link = strings.TrimSpace(link)
	if link == "" {
		return ""
	}
	if strings.HasPrefix(link, "http://") || strings.HasPrefix(link, "https://") {
		return link
	}
	if strings.HasPrefix(link, "//") {
		return "https:" + link
	}
	if strings.HasPrefix(link, "/") {
		return base + link
	}
	return link
}

func searxSearchJSON(ctx context.Context, client *http.Client, base, query string) ([]searchHit, error) {
	u := base + "/search?q=" + url.QueryEscape(query) + "&format=json&language=auto"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", base+"/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	var payload struct {
		Results []struct {
			URL     string `json:"url"`
			Title   string `json:"title"`
			Content string `json:"content"`
		} `json:"results"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 3<<20))
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}

	var out []searchHit
	for _, r := range payload.Results {
		if len(out) >= 10 {
			break
		}
		title := strings.TrimSpace(r.Title)
		link := normalizeSearxURL(base, strings.TrimSpace(r.URL))
		if title == "" || link == "" {
			continue
		}
		out = append(out, searchHit{
			title:   title,
			link:    link,
			snippet: strings.TrimSpace(r.Content),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty json results")
	}
	return out, nil
}

func searxSearchHTML(ctx context.Context, client *http.Client, base, query string) ([]searchHit, error) {
	u := base + "/search?q=" + url.QueryEscape(query) + "&language=auto"
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	setBrowserHeaders(req)
	req.Header.Set("Referer", base+"/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var out []searchHit
	// 常见主题：article.result（SearxNG）、div.result
	doc.Find("article.result").Each(func(i int, s *goquery.Selection) {
		if len(out) >= 10 {
			return
		}
		a := s.Find("h3 a, h4 a, .result_header a").First()
		if a.Length() == 0 {
			a = s.Find("a").First()
		}
		title := strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		link := normalizeSearxURL(base, href)
		snippet := strings.TrimSpace(s.Find("p.content, .content").First().Text())
		if title != "" && link != "" {
			out = append(out, searchHit{title: title, link: link, snippet: snippet})
		}
	})
	if len(out) > 0 {
		return out, nil
	}

	doc.Find("div.result").Each(func(i int, s *goquery.Selection) {
		if len(out) >= 10 {
			return
		}
		if s.HasClass("result--ad") {
			return
		}
		a := s.Find(".result_header a, h3 a, h4 a").First()
		title := strings.TrimSpace(a.Text())
		href, _ := a.Attr("href")
		link := normalizeSearxURL(base, href)
		snippet := strings.TrimSpace(s.Find(".content").First().Text())
		if title != "" && link != "" {
			out = append(out, searchHit{title: title, link: link, snippet: snippet})
		}
	})
	if len(out) == 0 {
		return nil, fmt.Errorf("empty html results")
	}
	return out, nil
}

// 内置工具：文件操作
func NewFileTool() Tool {
	return &fileTool{}
}

type fileTool struct{}

func (t *fileTool) Name() string {
	return "file_operation"
}

func (t *fileTool) Description() string {
	return "读写文件操作"
}

func (t *fileTool) Parameters() map[string]Property {
	return map[string]Property{
		"operation": {
			Type:        "string",
			Description: "操作类型：read 或 write",
		},
		"file_path": {
			Type:        "string",
			Description: "文件路径",
		},
		"content": {
			Type:        "string",
			Description: "写入文件的内容（仅在operation为write时需要）",
		},
	}
}

func (t *fileTool) Required() []string {
	return []string{"operation", "file_path"}
}

func (t *fileTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	operation, ok := args["operation"].(string)
	if !ok {
		return "", fmt.Errorf("invalid operation type")
	}

	filePath, ok := args["file_path"].(string)
	if !ok {
		return "", fmt.Errorf("invalid file_path type")
	}

	switch operation {
	case "read":
		// 读取文件
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
		return fmt.Sprintf("文件内容:\n%s", string(data)), nil

	case "write":
		// 写入文件
		content, ok := args["content"].(string)
		if !ok {
			return "", fmt.Errorf("invalid content type")
		}

		// 确保目录存在
		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}

		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return "", fmt.Errorf("failed to write file: %w", err)
		}

		return "文件写入成功", nil

	default:
		return "", fmt.Errorf("unsupported operation: %s", operation)
	}
}
