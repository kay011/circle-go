package utils

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries int           // 最大重试次数
	BaseDelay  time.Duration // 基础延迟时间
	MaxDelay   time.Duration // 最大延迟时间
	Multiplier float64       // 延迟倍增系数
	Jitter     bool          // 是否添加随机抖动
}

// DefaultRetryConfig 默认重试配置
var DefaultRetryConfig = RetryConfig{
	MaxRetries: 3,
	BaseDelay:  1 * time.Second,
	MaxDelay:   30 * time.Second,
	Multiplier: 2.0,
	Jitter:     true,
}

// IsRetryable 判断错误是否可重试
type IsRetryable func(err error) bool

// DefaultIsRetryable 默认的可重试错误判断
func DefaultIsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	// 网络相关错误通常可重试
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"deadline exceeded",
		"no such host",
		"temporary failure",
		"service unavailable",
		"rate limit",
		"too many requests",
	}

	for _, retryable := range retryableErrors {
		if contains(errMsg, retryable) {
			return true
		}
	}

	return false
}

// RetryWithBackoff 使用指数退避策略执行函数
func RetryWithBackoff(ctx context.Context, config RetryConfig, isRetryable IsRetryable, fn func() error) error {
	if isRetryable == nil {
		isRetryable = DefaultIsRetryable
	}

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// 执行函数
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// 检查是否可重试
		if !isRetryable(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// 如果是最后一次尝试，直接返回错误
		if attempt == config.MaxRetries {
			break
		}

		// 计算延迟时间（指数退避）
		delay := calculateDelay(config, attempt)

		// 等待或直到上下文取消
		select {
		case <-time.After(delay):
			// 继续重试
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}
	}

	return fmt.Errorf("max retries (%d) exceeded, last error: %w", config.MaxRetries, lastErr)
}

// calculateDelay 计算延迟时间
func calculateDelay(config RetryConfig, attempt int) time.Duration {
	// 指数退避：baseDelay * multiplier^attempt
	delay := float64(config.BaseDelay) * math.Pow(config.Multiplier, float64(attempt))

	// 限制最大延迟
	if delay > float64(config.MaxDelay) {
		delay = float64(config.MaxDelay)
	}

	// 添加随机抖动（避免惊群效应）
	if config.Jitter {
		jitter := rand.Float64() * 0.3 * delay // 0-30% 的随机抖动
		delay = delay - jitter*0.5 + jitter
	}

	return time.Duration(delay)
}

// RetryWithData 带返回值的重试
func RetryWithData[T any](ctx context.Context, config RetryConfig, isRetryable IsRetryable, fn func() (T, error)) (T, error) {
	var zero T

	if isRetryable == nil {
		isRetryable = DefaultIsRetryable
	}

	var lastErr error
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return zero, fmt.Errorf("non-retryable error: %w", err)
		}

		if attempt == config.MaxRetries {
			break
		}

		delay := calculateDelay(config, attempt)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return zero, fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}
	}

	return zero, fmt.Errorf("max retries (%d) exceeded, last error: %w", config.MaxRetries, lastErr)
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
