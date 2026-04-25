package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
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
	Level     LogLevel               `json:"level"`
	Timestamp time.Time              `json:"timestamp"`
	Prefix    string                 `json:"prefix"`
	Message   string                 `json:"message"`
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
	metrics      = make(map[string]int64)
	metricMetas  = make(map[string]metricMeta)
	histograms   = make(map[string]*histogramSeries)
	metricsMutex sync.RWMutex
)

type histogramMetric struct {
	Buckets []float64
	Counts  []uint64
	Sum     float64
	Count   uint64
}

type metricMeta struct {
	Name   string
	Labels map[string]string
}

type histogramSeries struct {
	meta metricMeta
	data *histogramMetric
}

// IncrMetric 增加指标计数
func IncrMetric(name string) {
	IncrMetricWithLabels(name, nil)
}

// IncrMetricWithLabels 增加带标签的指标计数。
func IncrMetricWithLabels(name string, labels map[string]string) {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()
	key := seriesKey(name, labels)
	metrics[key]++
	if _, exists := metricMetas[key]; !exists {
		metricMetas[key] = metricMeta{Name: name, Labels: cloneLabels(labels)}
	}
}

// ObserveMetric 记录观测值到直方图。
func ObserveMetric(name string, value float64) {
	ObserveMetricWithLabels(name, labelsNone(), value)
}

// ObserveMetricWithLabels 记录带标签的观测值到直方图。
func ObserveMetricWithLabels(name string, labels map[string]string, value float64) {
	metricsMutex.Lock()
	defer metricsMutex.Unlock()

	key := seriesKey(name, labels)
	series, ok := histograms[key]
	if !ok {
		// 默认秒级时延桶：10ms 到 10s
		series = &histogramSeries{
			meta: metricMeta{
				Name:   name,
				Labels: cloneLabels(labels),
			},
			data: &histogramMetric{
				Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
				Counts:  make([]uint64, 10),
			},
		}
		histograms[key] = series
	}

	h := series.data
	h.Sum += value
	h.Count++
	for i, bound := range h.Buckets {
		if value <= bound {
			h.Counts[i]++
		}
	}
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

// RenderPrometheusMetrics 以 Prometheus 文本格式导出指标。
func RenderPrometheusMetrics() string {
	all := GetAllMetrics()

	metricsMutex.RLock()
	metaCopy := make(map[string]metricMeta, len(metricMetas))
	for k, v := range metricMetas {
		metaCopy[k] = metricMeta{Name: v.Name, Labels: cloneLabels(v.Labels)}
	}

	hCopy := make(map[string]histogramSeries, len(histograms))
	for k, v := range histograms {
		cp := histogramSeries{
			meta: metricMeta{
				Name:   v.meta.Name,
				Labels: cloneLabels(v.meta.Labels),
			},
			data: &histogramMetric{
				Buckets: append([]float64(nil), v.data.Buckets...),
				Counts:  append([]uint64(nil), v.data.Counts...),
				Sum:     v.data.Sum,
				Count:   v.data.Count,
			},
		}
		hCopy[k] = cp
	}
	metricsMutex.RUnlock()

	if len(all) == 0 && len(hCopy) == 0 {
		return ""
	}

	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		meta := metaCopy[k]
		name := sanitizeMetricName(meta.Name)
		labels := renderPromLabels(meta.Labels)
		sb.WriteString(fmt.Sprintf("# TYPE %s counter\n", name))
		sb.WriteString(fmt.Sprintf("%s%s %d\n", name, labels, all[k]))
	}

	hKeys := make([]string, 0, len(hCopy))
	for k := range hCopy {
		hKeys = append(hKeys, k)
	}
	sort.Strings(hKeys)
	for _, k := range hKeys {
		series := hCopy[k]
		name := sanitizeMetricName(series.meta.Name)
		labelSet := series.meta.Labels
		h := series.data
		sb.WriteString(fmt.Sprintf("# TYPE %s histogram\n", name))
		var cumulative uint64
		for i, bound := range h.Buckets {
			cumulative += h.Counts[i]
			sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", name, renderPromLabels(withExtraLabel(labelSet, "le", fmt.Sprintf("%g", bound))), cumulative))
		}
		sb.WriteString(fmt.Sprintf("%s_bucket%s %d\n", name, renderPromLabels(withExtraLabel(labelSet, "le", "+Inf")), h.Count))
		sb.WriteString(fmt.Sprintf("%s_sum%s %g\n", name, renderPromLabels(labelSet), h.Sum))
		sb.WriteString(fmt.Sprintf("%s_count%s %d\n", name, renderPromLabels(labelSet), h.Count))
	}
	return sb.String()
}

func labelsNone() map[string]string {
	return nil
}

func seriesKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(name)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(labels[k])
	}
	return b.String()
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	cp := make(map[string]string, len(labels))
	for k, v := range labels {
		cp[k] = v
	}
	return cp
}

func withExtraLabel(labels map[string]string, key, value string) map[string]string {
	cp := cloneLabels(labels)
	if cp == nil {
		cp = make(map[string]string, 1)
	}
	cp[key] = value
	return cp
}

func renderPromLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		safeKey := sanitizeMetricName(k)
		safeVal := strings.ReplaceAll(labels[k], "\\", "\\\\")
		safeVal = strings.ReplaceAll(safeVal, "\"", "\\\"")
		safeVal = strings.ReplaceAll(safeVal, "\n", "\\n")
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", safeKey, safeVal))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func sanitizeMetricName(name string) string {
	if name == "" {
		return "circle_go_metric"
	}
	var b strings.Builder
	for i, r := range name {
		isValid := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			r == '_' ||
			(r >= '0' && r <= '9' && i > 0)
		if isValid {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
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
