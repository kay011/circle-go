package test

import (
	"context"
	"testing"

	"circle-go/internal/tools"
)

func TestCalculatorTool(t *testing.T) {
	// 创建计算器工具
	tool := tools.NewCalculatorTool()

	// 测试工具基本信息
	if tool.Name() != "calculator" {
		t.Errorf("Expected tool name 'calculator', got '%s'", tool.Name())
	}

	if tool.Description() != "执行数学计算" {
		t.Errorf("Expected tool description '执行数学计算', got '%s'", tool.Description())
	}

	// 测试工具参数
	params := tool.Parameters()
	if _, exists := params["expression"]; !exists {
		t.Error("Expected 'expression' parameter")
	}

	// 测试必填参数
	required := tool.Required()
	if len(required) != 1 || required[0] != "expression" {
		t.Error("Expected 'expression' to be required")
	}

	// 测试工具执行
	testCases := []struct {
		expression string
		expected  string
	}{
		{"2 + 3", "计算结果: 5"},
		{"10 - 4", "计算结果: 6"},
		{"5 * 6", "计算结果: 30"},
		{"10 / 2", "计算结果: 5"},
		{"10 / 0", "计算结果: 错误：除数不能为零"},
		{"42", "计算结果: 42"},
	}

	for _, tc := range testCases {
		result, err := tool.Run(context.Background(), map[string]interface{}{
			"expression": tc.expression,
		})
		if err != nil {
			t.Errorf("Error running calculator: %v", err)
		}
		if result != tc.expected {
			t.Errorf("For expression '%s', expected '%s', got '%s'", tc.expression, tc.expected, result)
		}
	}
}

func TestToolManager(t *testing.T) {
	// 创建工具管理器
	manager := tools.NewToolManager()

	// 注册工具
	calculator := tools.NewCalculatorTool()
	manager.Register(calculator)

	// 测试获取工具
	tool := manager.Get("calculator")
	if tool == nil {
		t.Error("Expected to get calculator tool")
	}

	// 测试列出工具
	toolsList := manager.List()
	if len(toolsList) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(toolsList))
	}

	// 测试运行工具
	result, err := manager.Run(context.Background(), "calculator", map[string]interface{}{
		"expression": "2 + 2",
	})
	if err != nil {
		t.Errorf("Error running tool: %v", err)
	}
	if result != "计算结果: 4" {
		t.Errorf("Expected '计算结果: 4', got '%s'", result)
	}

	// 测试转换为LLM工具格式
	llmTools := manager.ToLLMTools()
	if len(llmTools) != 1 {
		t.Errorf("Expected 1 LLM tool, got %d", len(llmTools))
	}
}
