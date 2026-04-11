package tools

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"circle-go/internal/logging"
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

// 内置工具：网络搜索
func NewWebSearchTool(baiduAPIKey, baiduAPIURL string) Tool {
	return &webSearchTool{
		baiduAPIKey: baiduAPIKey,
		baiduAPIURL: baiduAPIURL,
	}
}

type webSearchTool struct {
	baiduAPIKey string
	baiduAPIURL string
}

func (t *webSearchTool) Name() string {
	return "web_search"
}

func (t *webSearchTool) Description() string {
	return "搜索网络信息"
}

func (t *webSearchTool) Parameters() map[string]Property {
	return map[string]Property{
		"query": {
			Type:        "string",
			Description: "搜索查询词",
		},
		"num_results": {
			Type:        "integer",
			Description: "返回结果数量，默认为3",
		},
	}
}

func (t *webSearchTool) Required() []string {
	return []string{"query"}
}

// 百度搜索 API 请求结构
type baiduRequest struct {
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	Edition            string `json:"edition,omitempty"`
	SearchSource       string `json:"search_source,omitempty"`
	ResourceTypeFilter []struct {
		Type string `json:"type"`
		TopK int    `json:"top_k"`
	} `json:"resource_type_filter,omitempty"`
}

// 百度搜索 API 响应结构
type baiduResponse struct {
	Result struct {
		Items []struct {
			Title    string `json:"title"`
			Url      string `json:"url"`
			Abstract string `json:"abstract"`
			PageTime string `json:"page_time,omitempty"`
		} `json:"items"`
	} `json:"result"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (t *webSearchTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	// 初始化日志记录器
	logger := logging.NewLogger(logging.INFO, "WebSearchTool")

	query, ok := args["query"].(string)
	if !ok {
		logger.Error("无效的查询类型", map[string]interface{}{
			"args": args,
		})
		return "", fmt.Errorf("invalid query type")
	}

	numResults := 3
	if numResultsVal, exists := args["num_results"]; exists {
		if nr, ok := numResultsVal.(float64); ok {
			numResults = int(nr)
		}
	}

	logger.Info("开始搜索", map[string]interface{}{
		"query":       query,
		"num_results": numResults,
	})

	// 总是使用本地模拟数据
	logger.Info("使用本地模拟数据进行搜索", map[string]interface{}{
		"query": query,
	})
	return t.getMockSearchResults(query, numResults, logger), nil
}

// 获取模拟搜索结果
func (t *webSearchTool) getMockSearchResults(query string, numResults int, logger *logging.Logger) string {
	// 转换查询词为小写，方便匹配
	lowerQuery := strings.ToLower(query)

	// 匹配常见查询
	if strings.Contains(lowerQuery, "go语言") || strings.Contains(lowerQuery, "golang") {
		// Go 语言相关搜索结果
		return `搜索结果:
摘要: Go 是一种开源的编程语言，它能让构造简单、可靠且高效的软件变得容易。
来源: https://golang.org/

结果 1: Go 语言的最新稳定版本是 Go 1.22，于 2024 年 2 月发布。
链接: https://golang.org/doc/devel/release.html

结果 2: Go 语言由 Google 开发，于 2009 年首次发布。
链接: https://golang.org/doc/faq#history

结果 3: Go 语言的主要特点包括：静态类型、垃圾回收、并发支持、简洁的语法等。
链接: https://golang.org/doc/effective_go.html`
	} else if strings.Contains(lowerQuery, "docker") {
		// Docker 相关搜索结果
		return `搜索结果:
摘要: Docker 是一个开源的容器化平台，用于构建、部署和运行应用程序。
来源: https://www.docker.com/

结果 1: Docker 允许开发者将应用程序及其依赖项打包到一个轻量级、可移植的容器中。
链接: https://www.docker.com/what-docker

结果 2: Docker 的最新稳定版本是 Docker Desktop 4.26，于 2024 年 1 月发布。
链接: https://docs.docker.com/desktop/release-notes/

结果 3: Docker 容器是轻量级的，因为它们共享主机的操作系统内核，而不需要运行完整的操作系统。
链接: https://www.docker.com/resources/what-container`
	} else if strings.Contains(lowerQuery, "人工智能") || strings.Contains(lowerQuery, "ai") {
		// 人工智能相关搜索结果
		return `搜索结果:
摘要: 人工智能（AI）是计算机科学的一个分支，旨在创建能够模拟人类智能的系统。
来源: https://en.wikipedia.org/wiki/Artificial_intelligence

结果 1: 人工智能的主要领域包括机器学习、深度学习、自然语言处理、计算机视觉等。
链接: https://www.ibm.com/topics/artificial-intelligence

结果 2: 2024 年，人工智能技术继续快速发展，特别是在大语言模型和生成式 AI 领域。
链接: https://www.gartner.com/en/newsroom/press-releases/2024-01-16-gartner-top-strategic-technology-trends-for-2024-include-ai-security-and-industry-cloud-platforms

结果 3: 人工智能在医疗、金融、交通、教育等领域都有广泛的应用。
链接: https://www.mckinsey.com/capabilities/mckinsey-digital/our-insights/ai-by-industry`
	} else {
		// 通用搜索结果
		results := fmt.Sprintf("搜索结果:\n摘要: 关于 '%s' 的搜索结果。\n\n", query)
		for i := 1; i <= numResults; i++ {
			results += fmt.Sprintf("结果 %d: 这是关于 '%s' 的搜索结果 %d。\n", i, query, i)
			results += fmt.Sprintf("链接: https://example.com/search?q=%s&result=%d\n\n", url.QueryEscape(query), i)
		}
		return results
	}
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
