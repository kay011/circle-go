package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	LLM      LLMConfig      `yaml:"llm"`
	Memory   MemoryConfig   `yaml:"memory"`
	MCP      MCPConfig      `yaml:"mcp"`
	Search   SearchConfig   `yaml:"search"`
}

// SearchConfig 搜索配置
type SearchConfig struct {
	SearxInstances []string `yaml:"searx_instances"` // 可选：SearxNG 实例根 URL（无 Key 聚合），如 https://searx.be
	WebSearchMock  bool     `yaml:"web_search_mock"` // true：web_search 不访问外网，返回模拟结果（联调/无外网环境）
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host string `yaml:"host"`
	Port string `yaml:"port"`
}

// LLMConfig LLM配置
type LLMConfig struct {
	APIKey     string `yaml:"api_key"`
	Model      string `yaml:"model"`
	BaseURL    string `yaml:"base_url"`
	MaxTokens  int    `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	ShortTermSize int    `yaml:"short_term_size"`
	LongTermPath  string `yaml:"long_term_path"`
}

// MCPConfig MCP配置
type MCPConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	// 读取配置文件
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析YAML
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 设置默认值
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.LLM.Model == "" {
		cfg.LLM.Model = "gpt-4"
	}
	if cfg.LLM.BaseURL == "" {
		cfg.LLM.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 1000
	}
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.7
	}
	if cfg.Memory.ShortTermSize == 0 {
		cfg.Memory.ShortTermSize = 10
	}
	if cfg.Memory.LongTermPath == "" {
		cfg.Memory.LongTermPath = "./memory"
	}

	// searx_instances 为空时由 tools 包使用内置列表；若配置了则完全以配置为准

	return &cfg, nil
}
