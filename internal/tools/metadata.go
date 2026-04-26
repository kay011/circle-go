package tools

import "strings"

// ToolMetadata 是面向“skill-like”模式的轻量声明层。
// 目标：不修改主流程即可为工具补充意图标签/风险信息。
type ToolMetadata struct {
	ID          string
	Version     string
	IntentTags  []string
	RiskLevel   string
	Owner       string
	DisplayName string
	Policy      ToolMetadataPolicy
}

type ToolMetadataPolicy struct {
	TimeoutSeconds int
	Approval       string
}

// ToolMetadataProvider 可选接口：工具实现后可提供 skill 元数据。
type ToolMetadataProvider interface {
	Metadata() ToolMetadata
}

type metadataWrappedTool struct {
	Tool
	md ToolMetadata
}

func (t metadataWrappedTool) Metadata() ToolMetadata {
	return t.md
}

func WrapToolWithMetadata(tool Tool, md ToolMetadata) Tool {
	if tool == nil {
		return nil
	}
	return metadataWrappedTool{Tool: tool, md: md}
}

// BuildToolDescription 将元数据编入描述，便于检索/路由复用。
func BuildToolDescription(base string, md ToolMetadata) string {
	base = strings.TrimSpace(base)
	if len(md.IntentTags) == 0 {
		return base
	}
	return strings.TrimSpace(base + " [intent_tags: " + strings.Join(md.IntentTags, ",") + "]")
}
