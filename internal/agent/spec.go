package agent

import (
	"fmt"
	"os"
	"strings"
)

// ExecutionMode 智能体执行模式
type ExecutionMode string

const (
	// ModeReAct 思考-行动-观察交错循环（工具调用与模型交替）
	ModeReAct ExecutionMode = "react"
	// ModePlanExecute 先规划再按步执行，最后综合回答
	ModePlanExecute ExecutionMode = "plan_execute"
)

// Spec 单个智能体定义（可由 YAML 加载）
type Spec struct {
	ID               string        `yaml:"id"`
	DisplayName      string        `yaml:"name"`
	ExecutionMode    ExecutionMode `yaml:"execution_mode"`
	MaxSteps         int           `yaml:"max_steps"`
	HumanInTheLoop   bool          `yaml:"human_in_the_loop"`
	SystemPrompt     string        `yaml:"system_prompt"`
	SystemPromptFile string        `yaml:"system_prompt_file"`
	Tools            []string      `yaml:"tools"` // 为空或 nil 表示允许全部已注册工具
}

// DefaultSpec 内置默认智能体（未配置 agents 文件时使用）
func DefaultSpec() Spec {
	return Spec{
		ID:               "default",
		DisplayName:      "默认助手",
		ExecutionMode:    ModeReAct,
		MaxSteps:         10,
		HumanInTheLoop:   false,
		SystemPrompt:     "",
		SystemPromptFile: "",
		Tools:            nil,
	}
}

// ResolveSystemPrompt 返回最终系统提示词（文件优先，其次内联，否则内置）
func (s *Spec) ResolveSystemPrompt() (string, error) {
	if p := strings.TrimSpace(s.SystemPromptFile); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read system_prompt_file %q: %w", p, err)
		}
		if len(strings.TrimSpace(string(b))) == 0 {
			return "", fmt.Errorf("system_prompt_file %q is empty", p)
		}
		return string(b), nil
	}
	if strings.TrimSpace(s.SystemPrompt) != "" {
		return s.SystemPrompt, nil
	}
	return BuiltInSystemPrompt, nil
}

// Normalize 补全默认值并校验模式字符串
func (s *Spec) Normalize() error {
	if s.ID == "" {
		s.ID = "default"
	}
	if s.DisplayName == "" {
		s.DisplayName = s.ID
	}
	switch strings.ToLower(string(s.ExecutionMode)) {
	case "", string(ModeReAct):
		s.ExecutionMode = ModeReAct
	case string(ModePlanExecute):
		s.ExecutionMode = ModePlanExecute
	default:
		return fmt.Errorf("unknown execution_mode %q (use react or plan_execute)", s.ExecutionMode)
	}
	if s.MaxSteps <= 0 {
		s.MaxSteps = 10
	}
	return nil
}
