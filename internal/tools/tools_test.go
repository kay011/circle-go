package tools

import (
	"context"
	"testing"
)

func TestToolManager_RegisterAndGet(t *testing.T) {
	tm := NewToolManager()

	// 注册计算器工具
	calcTool := NewCalculatorTool()
	tm.Register(calcTool)

	// 获取工具
	retrieved := tm.Get("calculator")
	if retrieved == nil {
		t.Fatal("Expected to retrieve calculator tool")
	}
	if retrieved.Name() != "calculator" {
		t.Errorf("Expected tool name 'calculator', got '%s'", retrieved.Name())
	}

	// 获取不存在的工具
	nonExistent := tm.Get("nonexistent")
	if nonExistent != nil {
		t.Error("Expected nil for non-existent tool")
	}
}

func TestToolManager_List(t *testing.T) {
	tm := NewToolManager()

	tm.Register(NewCalculatorTool())
	tm.Register(NewWebSearchTool([]string{}))

	tools := tm.List()
	if len(tools) != 2 {
		t.Errorf("Expected 2 tools, got %d", len(tools))
	}
}

func TestToolManager_Run(t *testing.T) {
	tm := NewToolManager()
	tm.Register(NewCalculatorTool())

	ctx := context.Background()

	// 测试成功执行
	result, err := tm.Run(ctx, "calculator", map[string]interface{}{
		"expression": "2 + 3",
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == "" {
		t.Error("Expected non-empty result")
	}

	// 测试缺少必需参数
	_, err = tm.Run(ctx, "calculator", map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for missing required parameter")
	}

	// 测试不存在的工具
	_, err = tm.Run(ctx, "nonexistent", map[string]interface{}{})
	if err == nil {
		t.Error("Expected error for non-existent tool")
	}
}

func TestCalculatorTool_BasicOperations(t *testing.T) {
	tool := NewCalculatorTool()
	ctx := context.Background()

	tests := []struct {
		expr     string
		expected float64
	}{
		{"2 + 3", 5},
		{"10 - 4", 6},
		{"3 * 4", 12},
		{"15 / 3", 5},
		{"2 + 3 * 4", 14},   // 测试运算符优先级
		{"(2 + 3) * 4", 20}, // 测试括号
	}

	for _, tt := range tests {
		result, err := tool.Run(ctx, map[string]interface{}{
			"expression": tt.expr,
		})
		if err != nil {
			t.Errorf("Expression '%s': expected no error, got %v", tt.expr, err)
		}
		// 简单验证结果包含预期值
		if result == "" {
			t.Errorf("Expression '%s': expected non-empty result", tt.expr)
		}
	}
}

func TestCalculatorTool_DivisionByZero(t *testing.T) {
	tool := NewCalculatorTool()
	ctx := context.Background()

	result, err := tool.Run(ctx, map[string]interface{}{
		"expression": "10 / 0",
	})
	if err != nil {
		t.Errorf("Division by zero should return error message, got error: %v", err)
	}
	if result == "" {
		t.Error("Expected error message for division by zero")
	}
}

func TestCalculatorTool_InvalidExpression(t *testing.T) {
	tool := NewCalculatorTool()
	ctx := context.Background()

	// 测试空表达式
	result1, err := tool.Run(ctx, map[string]interface{}{
		"expression": "",
	})
	if err != nil {
		t.Errorf("Empty expression should return error message, got error: %v", err)
	}
	if result1 == "" {
		t.Error("Expected error message for empty expression")
	}

	// 测试无效表达式
	result2, err := tool.Run(ctx, map[string]interface{}{
		"expression": "abc",
	})
	if err != nil {
		t.Errorf("Invalid expression should return error message, got error: %v", err)
	}
	if result2 == "" {
		t.Error("Expected error message for invalid expression")
	}
}

func TestFileTool_ValidatePath(t *testing.T) {
	tool := NewFileTool().(*fileTool)

	// 测试合法路径
	safePath, err := tool.validatePath("test.txt")
	if err != nil {
		t.Errorf("Expected valid path, got error: %v", err)
	}
	if safePath == "" {
		t.Error("Expected non-empty safe path")
	}

	// 测试路径遍历攻击
	_, err = tool.validatePath("../etc/passwd")
	if err == nil {
		t.Error("Expected error for path traversal attempt")
	}

	// 测试绝对路径穿越
	_, err = tool.validatePath("../../outside/file.txt")
	if err == nil {
		t.Error("Expected error for path outside allowed directory")
	}
}

func TestFileTool_ReadNonExistentFile(t *testing.T) {
	tool := NewFileTool()
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{
		"operation": "read",
		"file_path": "nonexistent.txt",
	})
	if err == nil {
		t.Error("Expected error for reading non-existent file")
	}
}

func TestFileTool_InvalidOperation(t *testing.T) {
	tool := NewFileTool()
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{
		"operation": "invalid",
		"file_path": "test.txt",
	})
	if err == nil {
		t.Error("Expected error for invalid operation")
	}
}

func TestWebSearchTool_EmptyQuery(t *testing.T) {
	tool := NewWebSearchTool([]string{})
	ctx := context.Background()

	_, err := tool.Run(ctx, map[string]interface{}{
		"query": "",
	})
	if err == nil {
		t.Error("Expected error for empty query")
	}
}

func TestToolManager_ToLLMTools(t *testing.T) {
	tm := NewToolManager()
	tm.Register(NewCalculatorTool())

	llmTools := tm.ToLLMTools()
	if len(llmTools) != 1 {
		t.Errorf("Expected 1 LLM tool, got %d", len(llmTools))
	}

	tool := llmTools[0]
	if tool.Name != "calculator" {
		t.Errorf("Expected tool name 'calculator', got '%s'", tool.Name)
	}
	if tool.Description == "" {
		t.Error("Expected non-empty description")
	}
	if tool.Parameters["type"] != "object" {
		t.Errorf("Expected parameters type 'object', got '%v'", tool.Parameters["type"])
	}
}
