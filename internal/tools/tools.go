package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func (t *webSearchTool) Run(ctx context.Context, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("invalid query type")
	}

	numResults := 3
	if numResultsVal, exists := args["num_results"]; exists {
		if nr, ok := numResultsVal.(float64); ok {
			numResults = int(nr)
		}
	}

	// 模拟搜索结果（实际应用中应调用真实的搜索API）
	results := []string{
		fmt.Sprintf("搜索结果1: 关于'%s'的信息...", query),
		fmt.Sprintf("搜索结果2: 更多关于'%s'的内容...", query),
		fmt.Sprintf("搜索结果3: '%s'的相关资源...", query),
	}

	if numResults > len(results) {
		numResults = len(results)
	}

	resultStr := strings.Join(results[:numResults], "\n")
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
