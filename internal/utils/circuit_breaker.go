package utils

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState 断路器状态
type CircuitState int

const (
	StateClosed   CircuitState = iota // 关闭状态（正常）
	StateOpen                         // 打开状态（拒绝请求）
	StateHalfOpen                     // 半开状态（允许试探）
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// CircuitBreakerConfig 断路器配置
type CircuitBreakerConfig struct {
	FailureThreshold int           // 失败多少次后打开断路器
	SuccessThreshold int           // 半开状态下成功多少次后关闭
	RecoveryTimeout  time.Duration // 打开状态多久后进入半开状态
	Timeout          time.Duration // 单次调用超时时间
}

// DefaultCircuitBreakerConfig 默认配置
var DefaultCircuitBreakerConfig = CircuitBreakerConfig{
	FailureThreshold: 5,
	SuccessThreshold: 3,
	RecoveryTimeout:  60 * time.Second,
	Timeout:          30 * time.Second,
}

// CircuitBreaker 断路器实现
type CircuitBreaker struct {
	config        CircuitBreakerConfig
	state         CircuitState
	failureCount  int
	successCount  int
	lastFailTime  time.Time
	mu            sync.RWMutex
	onStateChange func(oldState, newState CircuitState) // 状态变化回调
}

// NewCircuitBreaker 创建断路器
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = DefaultCircuitBreakerConfig.FailureThreshold
	}
	if config.SuccessThreshold <= 0 {
		config.SuccessThreshold = DefaultCircuitBreakerConfig.SuccessThreshold
	}
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = DefaultCircuitBreakerConfig.RecoveryTimeout
	}
	if config.Timeout <= 0 {
		config.Timeout = DefaultCircuitBreakerConfig.Timeout
	}

	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// Execute 执行受保护的函数
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.RLock()
	currentState := cb.state

	// 检查是否应该从 Open 转为 Half-Open
	if currentState == StateOpen {
		if time.Since(cb.lastFailTime) > cb.config.RecoveryTimeout {
			cb.mu.RUnlock()
			cb.mu.Lock()
			// 双重检查
			if cb.state == StateOpen && time.Since(cb.lastFailTime) > cb.config.RecoveryTimeout {
				cb.changeState(StateHalfOpen)
				currentState = StateHalfOpen
			} else {
				currentState = cb.state
			}
			cb.mu.Unlock()
			cb.mu.RLock()
		} else {
			cb.mu.RUnlock()
			return fmt.Errorf("circuit breaker is OPEN, rejecting request")
		}
	}
	cb.mu.RUnlock()

	// 执行函数
	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.handleFailure(currentState)
		return err
	}

	cb.handleSuccess(currentState)
	return nil
}

// handleFailure 处理失败
func (cb *CircuitBreaker) handleFailure(state CircuitState) {
	cb.failureCount++
	cb.successCount = 0
	cb.lastFailTime = time.Now()

	switch state {
	case StateClosed:
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.changeState(StateOpen)
		}
	case StateHalfOpen:
		// 半开状态下任何失败都回到打开状态
		cb.changeState(StateOpen)
	}
}

// handleSuccess 处理成功
func (cb *CircuitBreaker) handleSuccess(state CircuitState) {
	if state == StateHalfOpen {
		cb.successCount++
		if cb.successCount >= cb.config.SuccessThreshold {
			// 重置并关闭断路器
			cb.failureCount = 0
			cb.successCount = 0
			cb.changeState(StateClosed)
		}
	} else {
		// 关闭状态下重置失败计数
		cb.failureCount = 0
	}
}

// changeState 改变状态
func (cb *CircuitBreaker) changeState(newState CircuitState) {
	oldState := cb.state
	cb.state = newState

	// 触发回调
	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// SetStateChangeCallback 设置状态变化回调
func (cb *CircuitBreaker) SetStateChangeCallback(callback func(oldState, newState CircuitState)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = callback
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset 手动重置断路器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.successCount = 0
	cb.changeState(StateClosed)
}

// Metrics 获取断路器指标
type CircuitBreakerMetrics struct {
	State        CircuitState
	FailureCount int
	SuccessCount int
	LastFailTime time.Time
}

func (cb *CircuitBreaker) GetMetrics() CircuitBreakerMetrics {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return CircuitBreakerMetrics{
		State:        cb.state,
		FailureCount: cb.failureCount,
		SuccessCount: cb.successCount,
		LastFailTime: cb.lastFailTime,
	}
}

// CircuitBreakerManager 管理多个断路器
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mu       sync.RWMutex
}

// NewCircuitBreakerManager 创建断路器管理器
func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetBreaker 获取或创建断路器
func (mgr *CircuitBreakerManager) GetBreaker(name string) *CircuitBreaker {
	mgr.mu.RLock()
	breaker, exists := mgr.breakers[name]
	mgr.mu.RUnlock()

	if exists {
		return breaker
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// 双重检查
	if breaker, exists = mgr.breakers[name]; exists {
		return breaker
	}

	breaker = NewCircuitBreaker(mgr.config)
	mgr.breakers[name] = breaker
	return breaker
}

// GetAllMetrics 获取所有断路器的指标
func (mgr *CircuitBreakerManager) GetAllMetrics() map[string]CircuitBreakerMetrics {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	metrics := make(map[string]CircuitBreakerMetrics)
	for name, breaker := range mgr.breakers {
		metrics[name] = breaker.GetMetrics()
	}
	return metrics
}
