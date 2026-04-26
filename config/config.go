package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 应用配置结构
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	LLM          LLMConfig          `yaml:"llm"`
	Memory       MemoryConfig       `yaml:"memory"`
	Redis        RedisConfig        `yaml:"redis"`
	RateLimit    RateLimitConfig    `yaml:"rate_limit"`
	FileTool     FileToolConfig     `yaml:"file_tool"`
	Search       SearchConfig       `yaml:"search"`
	MCP          MCPConfig          `yaml:"mcp"`
	Logging      LoggingConfig      `yaml:"logging"`
	Metrics      MetricsConfig      `yaml:"metrics"`
	AgentRuntime AgentRuntimeConfig `yaml:"agent_runtime"`
	AgentRouting AgentRoutingConfig `yaml:"agent_routing"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string        `yaml:"host"`
	Port         string        `yaml:"port"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout  time.Duration `yaml:"idle_timeout"`
}

// LLMConfig LLM配置
type LLMConfig struct {
	DefaultProvider string                       `yaml:"default_provider"`
	Providers       map[string]LLMProviderConfig `yaml:"providers"`
	APIKey          string                       `yaml:"api_key"`  // legacy
	Model           string                       `yaml:"model"`    // legacy
	BaseURL         string                       `yaml:"base_url"` // legacy
	MaxTokens       int                          `yaml:"max_tokens"`
	Temperature     float64                      `yaml:"temperature"`
	Retry           RetryConfig                  `yaml:"retry"`
	CircuitBreaker  CBConfig                     `yaml:"circuit_breaker"`
}

type LLMProviderConfig struct {
	APIKey       string   `yaml:"api_key"`
	BaseURL      string   `yaml:"base_url"`
	DefaultModel string   `yaml:"default_model"`
	Models       []string `yaml:"models"`
}

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int           `yaml:"max_retries"`
	BaseDelay  time.Duration `yaml:"base_delay"`
	MaxDelay   time.Duration `yaml:"max_delay"`
}

// CBConfig 断路器配置
type CBConfig struct {
	FailureThreshold int           `yaml:"failure_threshold"`
	RecoveryTimeout  time.Duration `yaml:"recovery_timeout"`
	SuccessThreshold int           `yaml:"success_threshold"`
}

// MemoryConfig 记忆配置
type MemoryConfig struct {
	ShortTermSize        int           `yaml:"short_term_size"`
	LongTermPath         string        `yaml:"long_term_path"`
	CompressionThreshold float64       `yaml:"compression_threshold"`
	SessionTTL           time.Duration `yaml:"session_ttl"`
}

// RateLimitConfig 速率限制配置
type RateLimitConfig struct {
	Enabled           bool `yaml:"enabled"`
	RequestsPerMinute int  `yaml:"requests_per_minute"`
	BurstSize         int  `yaml:"burst_size"`
}

// FileToolConfig 文件工具配置
type FileToolConfig struct {
	AllowedDirectory string `yaml:"allowed_directory"`
	MaxFileSize      int64  `yaml:"max_file_size"`
}

// SearchConfig 搜索配置
type SearchConfig struct {
	SearxInstances []string `yaml:"searx_instances"`
}

// MCPConfig MCP配置
type MCPConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

// MetricsConfig 监控配置
type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Addr     string `yaml:"addr"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
	Prefix   string `yaml:"prefix"`
}

// AgentRuntimeConfig Agent 运行时预算配置
type AgentRuntimeConfig struct {
	MaxSteps           int           `yaml:"max_steps"`
	MaxToolCalls       int           `yaml:"max_tool_calls"`
	MaxDuration        time.Duration `yaml:"max_duration"`
	ToolTimeout        time.Duration `yaml:"tool_timeout"`
	ApprovalTimeout    time.Duration `yaml:"approval_timeout"`
	TrustedHTTPDomains []string      `yaml:"trusted_http_domains"`
}

type AgentRoutingConfig struct {
	Enabled              bool          `yaml:"enabled"`
	TopK                 int           `yaml:"top_k"`
	MinScore             int           `yaml:"min_score"`
	FallbackToAll        bool          `yaml:"fallback_to_all"`
	RouterEnabled        bool          `yaml:"router_enabled"`
	RouterMinConfidence  float64       `yaml:"router_min_confidence"`
	RouterTimeout        time.Duration `yaml:"router_timeout"`
	ErrorRerouteEnabled  bool          `yaml:"error_reroute_enabled"`
	ErrorRerouteTimeout  time.Duration `yaml:"error_reroute_timeout"`
	ResponseMode         string        `yaml:"response_mode"` // direct|summarize|hybrid
	SummarizeTimeout     time.Duration `yaml:"summarize_timeout"`
	SummarizeOnToolError bool          `yaml:"summarize_on_tool_error"`
	FeatureFlagLegacy    bool          `yaml:"feature_flag_legacy"`
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
	setDefaults(&cfg)

	// 验证配置
	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

// setDefaults 设置默认值
func setDefaults(cfg *Config) {
	// Server defaults
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30 * time.Second
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 60 * time.Second
	}
	if cfg.Server.IdleTimeout == 0 {
		cfg.Server.IdleTimeout = 120 * time.Second
	}

	// LLM defaults
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]LLMProviderConfig{}
	}
	// 兼容旧版单模型配置：自动映射到 default provider
	if len(cfg.LLM.Providers) == 0 {
		provider := LLMProviderConfig{
			APIKey:       cfg.LLM.APIKey,
			BaseURL:      cfg.LLM.BaseURL,
			DefaultModel: cfg.LLM.Model,
		}
		if provider.DefaultModel == "" {
			provider.DefaultModel = "gpt-4"
		}
		if provider.BaseURL == "" {
			provider.BaseURL = "https://api.openai.com/v1"
		}
		if len(provider.Models) == 0 {
			provider.Models = []string{provider.DefaultModel}
		}
		cfg.LLM.Providers["default"] = provider
	}
	if cfg.LLM.DefaultProvider == "" {
		if _, ok := cfg.LLM.Providers["qwen"]; ok {
			cfg.LLM.DefaultProvider = "qwen"
		} else {
			cfg.LLM.DefaultProvider = "default"
		}
	}
	// 同步 legacy 字段，减少旧代码改动面
	defaultCfg, ok := cfg.LLM.Providers[cfg.LLM.DefaultProvider]
	if !ok && len(cfg.LLM.Providers) > 0 {
		for name, p := range cfg.LLM.Providers {
			cfg.LLM.DefaultProvider = name
			defaultCfg = p
			ok = true
			break
		}
	}
	if ok {
		if defaultCfg.DefaultModel == "" && len(defaultCfg.Models) > 0 {
			defaultCfg.DefaultModel = defaultCfg.Models[0]
		}
		if len(defaultCfg.Models) == 0 && defaultCfg.DefaultModel != "" {
			defaultCfg.Models = []string{defaultCfg.DefaultModel}
		}
		cfg.LLM.Providers[cfg.LLM.DefaultProvider] = defaultCfg
		cfg.LLM.APIKey = defaultCfg.APIKey
		cfg.LLM.BaseURL = defaultCfg.BaseURL
		cfg.LLM.Model = defaultCfg.DefaultModel
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 2000
	}
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.7
	}

	// LLM Retry defaults
	if cfg.LLM.Retry.MaxRetries == 0 {
		cfg.LLM.Retry.MaxRetries = 3
	}
	if cfg.LLM.Retry.BaseDelay == 0 {
		cfg.LLM.Retry.BaseDelay = 1 * time.Second
	}
	if cfg.LLM.Retry.MaxDelay == 0 {
		cfg.LLM.Retry.MaxDelay = 30 * time.Second
	}

	// Circuit Breaker defaults
	if cfg.LLM.CircuitBreaker.FailureThreshold == 0 {
		cfg.LLM.CircuitBreaker.FailureThreshold = 5
	}
	if cfg.LLM.CircuitBreaker.RecoveryTimeout == 0 {
		cfg.LLM.CircuitBreaker.RecoveryTimeout = 60 * time.Second
	}
	if cfg.LLM.CircuitBreaker.SuccessThreshold == 0 {
		cfg.LLM.CircuitBreaker.SuccessThreshold = 3
	}

	// Memory defaults
	if cfg.Memory.ShortTermSize == 0 {
		cfg.Memory.ShortTermSize = 10
	}
	if cfg.Memory.LongTermPath == "" {
		cfg.Memory.LongTermPath = "./memory"
	}
	if cfg.Memory.CompressionThreshold == 0 {
		cfg.Memory.CompressionThreshold = 0.8
	}
	if cfg.Memory.SessionTTL == 0 {
		cfg.Memory.SessionTTL = 24 * time.Hour
	}

	// Rate Limit defaults
	if !cfg.RateLimit.Enabled {
		cfg.RateLimit.Enabled = true
	}

	// Redis defaults
	if cfg.Redis.Addr == "" {
		cfg.Redis.Addr = "localhost:6379"
	}
	if cfg.Redis.Prefix == "" {
		cfg.Redis.Prefix = "circle_go"
	}
	if cfg.RateLimit.RequestsPerMinute == 0 {
		cfg.RateLimit.RequestsPerMinute = 60
	}
	if cfg.RateLimit.BurstSize == 0 {
		cfg.RateLimit.BurstSize = 10
	}

	// File Tool defaults
	if cfg.FileTool.AllowedDirectory == "" {
		cfg.FileTool.AllowedDirectory = "./data"
	}
	if cfg.FileTool.MaxFileSize == 0 {
		cfg.FileTool.MaxFileSize = 10 * 1024 * 1024 // 10MB
	}

	// Search defaults
	if cfg.Search.SearxInstances == nil {
		cfg.Search.SearxInstances = []string{
			"https://searx.tiekoetter.com",
			"https://searx.be",
			"https://paulgo.io",
			"https://search.mdosch.de",
		}
	}

	// MCP defaults
	if cfg.MCP.URL == "" {
		cfg.MCP.URL = "http://localhost:8000"
	}

	// Logging defaults
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Output == "" {
		cfg.Logging.Output = "stdout"
	}

	// Metrics defaults
	if !cfg.Metrics.Enabled {
		cfg.Metrics.Enabled = true
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}

	// Agent runtime defaults
	if cfg.AgentRuntime.MaxSteps == 0 {
		cfg.AgentRuntime.MaxSteps = 5
	}
	if cfg.AgentRuntime.MaxToolCalls == 0 {
		cfg.AgentRuntime.MaxToolCalls = 20
	}
	if cfg.AgentRuntime.MaxDuration == 0 {
		cfg.AgentRuntime.MaxDuration = 2 * time.Minute
	}
	if cfg.AgentRuntime.ToolTimeout == 0 {
		cfg.AgentRuntime.ToolTimeout = 20 * time.Second
	}
	if cfg.AgentRuntime.ApprovalTimeout == 0 {
		cfg.AgentRuntime.ApprovalTimeout = 2 * time.Minute
	}
	if cfg.AgentRuntime.TrustedHTTPDomains == nil {
		cfg.AgentRuntime.TrustedHTTPDomains = []string{}
	}

	if cfg.AgentRouting.TopK == 0 && cfg.AgentRouting.MinScore == 0 && cfg.AgentRouting.ResponseMode == "" && cfg.AgentRouting.RouterTimeout == 0 {
		cfg.AgentRouting.Enabled = true
		cfg.AgentRouting.FallbackToAll = true
		cfg.AgentRouting.RouterEnabled = true
		cfg.AgentRouting.ErrorRerouteEnabled = true
	}
	if cfg.AgentRouting.TopK <= 0 {
		cfg.AgentRouting.TopK = 4
	}
	if cfg.AgentRouting.MinScore <= 0 {
		cfg.AgentRouting.MinScore = 1
	}
	if cfg.AgentRouting.RouterMinConfidence <= 0 {
		cfg.AgentRouting.RouterMinConfidence = 0.68
	}
	if cfg.AgentRouting.RouterTimeout <= 0 {
		cfg.AgentRouting.RouterTimeout = 8 * time.Second
	}
	if cfg.AgentRouting.ErrorRerouteTimeout <= 0 {
		cfg.AgentRouting.ErrorRerouteTimeout = 6 * time.Second
	}
	if cfg.AgentRouting.ResponseMode == "" {
		cfg.AgentRouting.ResponseMode = "hybrid"
	}
	if cfg.AgentRouting.SummarizeTimeout <= 0 {
		cfg.AgentRouting.SummarizeTimeout = 12 * time.Second
	}
}

// validate 验证配置
func validate(cfg *Config) error {
	// 验证服务器端口
	if cfg.Server.Port == "" {
		return fmt.Errorf("server port cannot be empty")
	}

	// 验证 LLM 配置
	if len(cfg.LLM.Providers) == 0 {
		return fmt.Errorf("LLM providers cannot be empty")
	}
	if cfg.LLM.DefaultProvider == "" {
		return fmt.Errorf("LLM default_provider cannot be empty")
	}
	defaultCfg, ok := cfg.LLM.Providers[cfg.LLM.DefaultProvider]
	if !ok {
		return fmt.Errorf("LLM default_provider %q not found in providers", cfg.LLM.DefaultProvider)
	}
	if defaultCfg.APIKey == "" {
		return fmt.Errorf("LLM provider %q api_key cannot be empty", cfg.LLM.DefaultProvider)
	}
	if defaultCfg.BaseURL == "" {
		return fmt.Errorf("LLM provider %q base_url cannot be empty", cfg.LLM.DefaultProvider)
	}
	if defaultCfg.DefaultModel == "" {
		return fmt.Errorf("LLM provider %q default_model cannot be empty", cfg.LLM.DefaultProvider)
	}

	// 验证温度范围
	if cfg.LLM.Temperature < 0 || cfg.LLM.Temperature > 2 {
		return fmt.Errorf("LLM temperature must be between 0 and 2")
	}

	// 验证最大 token 数
	if cfg.LLM.MaxTokens <= 0 {
		return fmt.Errorf("LLM max_tokens must be positive")
	}

	// 验证短期记忆大小
	if cfg.Memory.ShortTermSize <= 0 {
		return fmt.Errorf("memory short_term_size must be positive")
	}

	// 验证压缩阈值
	if cfg.Memory.CompressionThreshold <= 0 || cfg.Memory.CompressionThreshold > 1 {
		return fmt.Errorf("memory compression_threshold must be between 0 and 1")
	}

	// 验证速率限制
	if cfg.RateLimit.RequestsPerMinute <= 0 {
		return fmt.Errorf("rate_limit requests_per_minute must be positive")
	}

	// 验证文件大小限制
	if cfg.FileTool.MaxFileSize <= 0 {
		return fmt.Errorf("file_tool max_file_size must be positive")
	}

	// 验证日志级别
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[cfg.Logging.Level] {
		return fmt.Errorf("invalid logging level: %s (must be debug, info, warn, or error)", cfg.Logging.Level)
	}

	if cfg.Redis.Enabled && cfg.Redis.Addr == "" {
		return fmt.Errorf("redis addr cannot be empty when redis is enabled")
	}

	// 验证 Agent 运行时预算
	if cfg.AgentRuntime.MaxSteps <= 0 {
		return fmt.Errorf("agent_runtime max_steps must be positive")
	}
	if cfg.AgentRuntime.MaxToolCalls <= 0 {
		return fmt.Errorf("agent_runtime max_tool_calls must be positive")
	}
	if cfg.AgentRuntime.MaxDuration <= 0 {
		return fmt.Errorf("agent_runtime max_duration must be positive")
	}
	if cfg.AgentRuntime.ToolTimeout <= 0 {
		return fmt.Errorf("agent_runtime tool_timeout must be positive")
	}
	if cfg.AgentRuntime.ApprovalTimeout <= 0 {
		return fmt.Errorf("agent_runtime approval_timeout must be positive")
	}
	if cfg.AgentRouting.TopK <= 0 {
		return fmt.Errorf("agent_routing top_k must be positive")
	}
	if cfg.AgentRouting.RouterMinConfidence < 0 || cfg.AgentRouting.RouterMinConfidence > 1 {
		return fmt.Errorf("agent_routing router_min_confidence must be between 0 and 1")
	}
	switch cfg.AgentRouting.ResponseMode {
	case "", "direct", "summarize", "hybrid":
	default:
		return fmt.Errorf("agent_routing response_mode must be one of direct|summarize|hybrid")
	}

	return nil
}

// GetRetryConfig 获取重试配置
func (cfg *Config) GetRetryConfig() RetryConfig {
	return cfg.LLM.Retry
}

// GetCircuitBreakerConfig 获取断路器配置
func (cfg *Config) GetCircuitBreakerConfig() CBConfig {
	return cfg.LLM.CircuitBreaker
}
