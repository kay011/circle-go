package tools

import (
	"fmt"
	"strings"
	"time"

	"circle-go/config"
)

type ToolFactory func(cfg *config.Config) Tool

// BuildToolManagerFromConfig 通过配置构建工具管理器。
// 目标：新增工具只需新增 factory 并在配置中启用，无需修改主流程。
func BuildToolManagerFromConfig(cfg *config.Config) (*ToolManager, error) {
	factories := map[string]ToolFactory{
		"calculator":          func(_ *config.Config) Tool { return NewCalculatorTool() },
		"web_search":          func(c *config.Config) Tool { return NewWebSearchTool(c.Search.SearxInstances) },
		"file_operation":      func(_ *config.Config) Tool { return NewFileTool() },
		"http_client":         func(_ *config.Config) Tool { return NewHTTPClientTool() },
		"investment_analyzer": func(_ *config.Config) Tool { return NewInvestmentAnalyzerTool() },
		"fund_compare":        func(_ *config.Config) Tool { return NewFundCompareTool() },
	}

	tm := NewToolManager()
	manifestMap, err := LoadToolManifestMap(cfg.Tools.ManifestPath)
	if err != nil {
		return nil, err
	}
	if cfg.Skills.Enabled {
		skillMap, err := LoadToolManifestMapFromSkillsDir(cfg.Skills.Path)
		if err != nil {
			return nil, err
		}
		for k, v := range skillMap {
			manifestMap[k] = v
		}
	}
	for _, rawName := range cfg.Tools.Enabled {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		factory, ok := factories[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool in config.tools.enabled: %s", name)
		}
		tool := factory(cfg)
		if md, ok := manifestMap[name]; ok {
			tool = WrapToolWithMetadata(tool, md)
		}
		tm.Register(tool)
	}
	if len(tm.List()) == 0 {
		return nil, fmt.Errorf("no tool is enabled")
	}
	return tm, nil
}

// BuildToolPolicyMapFromConfig 从 manifest 生成工具策略映射（目前接入 timeout）。
func BuildToolPolicyMapFromConfig(cfg *config.Config, defaultTimeout time.Duration) (map[string]ToolPolicy, error) {
	out := map[string]ToolPolicy{}
	manifestMap, err := LoadToolManifestMap(cfg.Tools.ManifestPath)
	if err != nil {
		return nil, err
	}
	if cfg.Skills.Enabled {
		skillMap, err := LoadToolManifestMapFromSkillsDir(cfg.Skills.Path)
		if err != nil {
			return nil, err
		}
		for k, v := range skillMap {
			manifestMap[k] = v
		}
	}
	for _, rawName := range cfg.Tools.Enabled {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		md, ok := manifestMap[name]
		if !ok {
			continue
		}
		timeout := defaultTimeout
		if md.Policy.TimeoutSeconds > 0 {
			timeout = time.Duration(md.Policy.TimeoutSeconds) * time.Second
		}
		out[name] = ToolPolicy{Timeout: timeout}
	}
	return out, nil
}

// BuildToolApprovalPolicyMapFromConfig 从 manifest 生成审批策略映射。
func BuildToolApprovalPolicyMapFromConfig(cfg *config.Config) (map[string]string, error) {
	out := map[string]string{}
	manifestMap, err := LoadToolManifestMap(cfg.Tools.ManifestPath)
	if err != nil {
		return nil, err
	}
	if cfg.Skills.Enabled {
		skillMap, err := LoadToolManifestMapFromSkillsDir(cfg.Skills.Path)
		if err != nil {
			return nil, err
		}
		for k, v := range skillMap {
			manifestMap[k] = v
		}
	}
	for _, rawName := range cfg.Tools.Enabled {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		md, ok := manifestMap[name]
		if !ok {
			continue
		}
		mode := strings.ToLower(strings.TrimSpace(md.Policy.Approval))
		if mode == "" {
			continue
		}
		out[name] = mode
	}
	return out, nil
}
