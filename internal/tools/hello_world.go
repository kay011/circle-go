package tools

import (
	"context"
	"strings"
)

type helloWorldTool struct{}

func NewHelloWorldTool() Tool {
	return &helloWorldTool{}
}

func (t *helloWorldTool) Name() string {
	return "hello_world"
}

func (t *helloWorldTool) Description() string {
	return "一个最小示例工具：返回友好的问候语，可用于验证 skills 调用链路。"
}

func (t *helloWorldTool) Parameters() map[string]Property {
	return map[string]Property{
		"name": {
			Type:        "string",
			Description: "可选，用户姓名或称呼。",
		},
	}
}

func (t *helloWorldTool) Required() []string {
	return nil
}

func (t *helloWorldTool) Run(_ context.Context, args map[string]interface{}) (string, error) {
	name := "world"
	if raw, ok := args["name"].(string); ok {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			name = raw
		}
	}
	return "Hello, " + name + "! 欢迎使用 hello-world skill。", nil
}

