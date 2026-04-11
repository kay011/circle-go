package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

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

// 简单的表达式计算函数
func evaluateExpression(expr string) interface{} {
	// 这里仅实现简单的加减乘除
	expr = strings.TrimSpace(expr)
	
	// 简单处理：只支持两个操作数的表达式
	if strings.Contains(expr, "+") {
		parts := strings.Split(expr, "+")
		if len(parts) == 2 {
			var a, b float64
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &a)
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &b)
			return a + b
		}
	}

	if strings.Contains(expr, "-") {
		parts := strings.Split(expr, "-")
		if len(parts) == 2 {
			var a, b float64
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &a)
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &b)
			return a - b
		}
	}

	if strings.Contains(expr, "*") {
		parts := strings.Split(expr, "*")
		if len(parts) == 2 {
			var a, b float64
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &a)
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &b)
			return a * b
		}
	}

	if strings.Contains(expr, "/") {
		parts := strings.Split(expr, "/")
		if len(parts) == 2 {
			var a, b float64
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &a)
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &b)
			if b != 0 {
				return a / b
			}
			return "错误：除数不能为零"
		}
	}

	// 尝试直接解析为数字
	var num float64
	if _, err := fmt.Sscanf(expr, "%f", &num); err == nil {
		return num
	}

	return "无法计算的表达式"
}

// 内置工具：网络搜索
func NewWebSearchTool() Tool {
	return &webSearchTool{}
}

type webSearchTool struct{}

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

// DuckDuckGo API 响应结构
type ddgResponse struct {
	Abstract       string `json:"Abstract"`
	AbstractText   string `json:"AbstractText"`
	AbstractSource string `json:"AbstractSource"`
	AbstractURL    string `json:"AbstractURL"`
	Heading        string `json:"Heading"`
	Results        []struct {
		Result  string `json:"Result"`
		FirstURL string `json:"FirstURL"`
		Text    string `json:"Text"`
	} `json:"Results"`
	RelatedTopics []struct {
		Result  string `json:"Result"`
		FirstURL string `json:"FirstURL"`
		Text    string `json:"Text"`
	} `json:"RelatedTopics"`
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
		"query": query,
		"num_results": numResults,
	})

	// 构建 DuckDuckGo API URL
	apiURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&pretty=1", query)
	logger.Info("构建API URL", map[string]interface{}{
		"url": apiURL,
	})

	// 创建 HTTP 客户端
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 发送请求
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		logger.Error("创建请求失败", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	logger.Info("发送请求", map[string]interface{}{
		"method": req.Method,
		"url": req.URL.String(),
	})

	resp, err := client.Do(req)
	if err != nil {
		logger.Error("发送请求失败", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		logger.Error("API返回非OK状态", map[string]interface{}{
			"status_code": resp.StatusCode,
			"status": resp.Status,
		})
		return "", fmt.Errorf("API returned non-OK status: %s", resp.Status)
	}

	logger.Info("收到响应", map[string]interface{}{
		"status_code": resp.StatusCode,
	})

	// 解析响应
	var ddgResp ddgResponse
	if err := json.NewDecoder(resp.Body).Decode(&ddgResp); err != nil {
		logger.Error("解析响应失败", map[string]interface{}{
			"error": err.Error(),
		})
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Info("解析响应成功", map[string]interface{}{
		"has_abstract": ddgResp.Abstract != "",
		"result_count": len(ddgResp.Results),
		"related_topics_count": len(ddgResp.RelatedTopics),
	})

	// 构建搜索结果
	var results []string

	// 添加摘要信息（如果有）
	if ddgResp.Abstract != "" {
		results = append(results, fmt.Sprintf("摘要: %s", ddgResp.Abstract))
		if ddgResp.AbstractURL != "" {
			results = append(results, fmt.Sprintf("来源: %s", ddgResp.AbstractURL))
		}
		results = append(results, "")
	}

	// 添加搜索结果
	for i, result := range ddgResp.Results {
		if i >= numResults {
			break
		}
		results = append(results, fmt.Sprintf("结果 %d: %s", i+1, result.Text))
		if result.FirstURL != "" {
			results = append(results, fmt.Sprintf("链接: %s", result.FirstURL))
		}
		results = append(results, "")
	}

	// 如果结果不足，添加相关主题
	if len(results) == 0 || len(ddgResp.Results) < numResults {
		for i, topic := range ddgResp.RelatedTopics {
			if i >= numResults-len(ddgResp.Results) {
				break
			}
			results = append(results, fmt.Sprintf("相关主题 %d: %s", i+1, topic.Text))
			if topic.FirstURL != "" {
				results = append(results, fmt.Sprintf("链接: %s", topic.FirstURL))
			}
			results = append(results, "")
		}
	}

	// 如果没有结果
	if len(results) == 0 {
		logger.Info("没有找到搜索结果", map[string]interface{}{
			"query": query,
		})
		return fmt.Sprintf("没有找到关于 '%s' 的搜索结果。", query), nil
	}

	// 格式化结果
	resultStr := strings.Join(results, "\n")
	logger.Info("搜索完成", map[string]interface{}{
		"result_length": len(resultStr),
	})
	return fmt.Sprintf("搜索结果:\n%s", resultStr), nil
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
