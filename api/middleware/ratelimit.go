package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// RateLimiter 简单的令牌桶速率限制器
type RateLimiter struct {
	tokens      map[string]*tokenBucket
	maxTokens   int
	refillRate  time.Duration
	mu          sync.RWMutex
	cleanupMu   sync.Mutex
	lastCleanup time.Time
}

type tokenBucket struct {
	tokens     int
	lastRefill time.Time
}

// NewRateLimiter 创建速率限制器
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	rl := &RateLimiter{
		tokens:      make(map[string]*tokenBucket),
		maxTokens:   maxTokens,
		refillRate:  refillRate,
		lastCleanup: time.Now(),
	}

	// 启动定期清理
	go rl.periodicCleanup()

	return rl
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.tokens[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     rl.maxTokens,
			lastRefill: time.Now(),
		}
		rl.tokens[key] = bucket
	}

	// 补充令牌
	now := time.Now()
	elapsed := now.Sub(bucket.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)
	if tokensToAdd > 0 {
		bucket.tokens += tokensToAdd
		if bucket.tokens > rl.maxTokens {
			bucket.tokens = rl.maxTokens
		}
		bucket.lastRefill = now
	}

	// 检查是否有足够的令牌
	if bucket.tokens <= 0 {
		return false
	}

	bucket.tokens--
	return true
}

// periodicCleanup 定期清理过期的桶
func (rl *RateLimiter) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

func (rl *RateLimiter) cleanup() {
	rl.cleanupMu.Lock()
	defer rl.cleanupMu.Unlock()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for key, bucket := range rl.tokens {
		// 如果超过1小时没有活动，删除该桶
		if now.Sub(bucket.lastRefill) > time.Hour {
			delete(rl.tokens, key)
		}
	}

	rl.lastCleanup = now
}

// Middleware 创建速率限制中间件
func (rl *RateLimiter) Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 使用 IP 地址作为限流 key
		key := getClientIP(r)

		if !rl.Allow(key) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":      "rate_limit_exceeded",
					"message":   "rate limit exceeded, please try again later",
					"retryable": true,
				},
			})
			return
		}

		next(w, r)
	}
}

// getClientIP 获取客户端 IP
func getClientIP(r *http.Request) string {
	// 尝试从 X-Forwarded-For 头获取
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// 尝试从 X-Real-IP 头获取
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// 使用 RemoteAddr
	return r.RemoteAddr
}
