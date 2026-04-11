package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel string

const (
	// DEBUG 调试级别
	DEBUG LogLevel = "DEBUG"
	// INFO 信息级别
	INFO LogLevel = "INFO"
	// WARN 警告级别
	WARN LogLevel = "WARN"
	// ERROR 错误级别
	ERROR LogLevel = "ERROR"
	// FATAL 致命级别
	FATAL LogLevel = "FATAL"
)

// Logger 日志记录器
type Logger struct {
	level  LogLevel
	prefix string
}

// NewLogger 创建日志记录器
func NewLogger(level LogLevel, prefix string) *Logger {
	return &Logger{
		level:  level,
		prefix: prefix,
	}
}

// LogEntry 日志条目
type LogEntry struct {
	Level     LogLevel  `json:"level"`
	Timestamp time.Time `json:"timestamp"`
	Prefix    string    `json:"prefix"`
	Message   string    `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
}

// Debug 记录调试日志
func (l *Logger) Debug(message string, data ...map[string]interface{}) {
	if l.shouldLog(DEBUG) {
		l.log(DEBUG, message, data...)
	}
}

// Info 记录信息日志
func (l *Logger) Info(message string, data ...map[string]interface{}) {
	if l.shouldLog(INFO) {
		l.log(INFO, message, data...)
	}
}

// Warn 记录警告日志
func (l *Logger) Warn(message string, data ...map[string]interface{}) {
	if l.shouldLog(WARN) {
		l.log(WARN, message, data...)
	}
}

// Error 记录错误日志
func (l *Logger) Error(message string, data ...map[string]interface{}) {
	if l.shouldLog(ERROR) {
		l.log(ERROR, message, data...)
	}
}

// Fatal 记录致命日志并退出程序
func (l *Logger) Fatal(message string, data ...map[string]interface{}) {
	if l.shouldLog(FATAL) {
		l.log(FATAL, message, data...)
	}
	os.Exit(1)
}

// Metrics 监控指标
var (
	metrics     = make(map[string]int64)
	metricsMutex sync.RWMutex
)

// IncrMetric 增加指标计数
func IncrMetric(name string) {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()
	metrics[name]++
}

// GetMetric 获取指标值
func GetMetric(name string) int64 {
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()
	return metrics[name]
}

// GetAllMetrics 获取所有指标
func GetAllMetrics() map[string]int64 {
	metricsMutex.RLock()
	defer metricsMutex.RUnlock()
	result := make(map[string]int64)
	for k, v := range metrics {
		result[k] = v
	}
	return result
}

// LogMetrics 记录所有指标
func LogMetrics(logger *Logger) {
	metrics := GetAllMetrics()
	// 转换为 map[string]interface{}
	metricsInterface := make(map[string]interface{})
	for k, v := range metrics {
		metricsInterface[k] = v
	}
	logger.Info("系统指标", metricsInterface)
}

// log 记录日志
func (l *Logger) log(level LogLevel, message string, data ...map[string]interface{}) {
	entry := LogEntry{
		Level:     level,
		Timestamp: time.Now(),
		Prefix:    l.prefix,
		Message:   message,
	}

	if len(data) > 0 && data[0] != nil {
		entry.Data = data[0]
	}

	// 输出JSON格式日志
	jsonData, err := json.Marshal(entry)
	if err != nil {
		// 如果JSON序列化失败，输出简单格式
		fmt.Printf("%s [%s] %s: %s\n", time.Now().Format(time.RFC3339), level, l.prefix, message)
		return
	}

	fmt.Println(string(jsonData))
}

// shouldLog 判断是否应该记录指定级别的日志
func (l *Logger) shouldLog(level LogLevel) bool {
	levelOrder := map[LogLevel]int{
		DEBUG: 0,
		INFO:  1,
		WARN:  2,
		ERROR: 3,
		FATAL: 4,
	}

	return levelOrder[level] >= levelOrder[l.level]
}
