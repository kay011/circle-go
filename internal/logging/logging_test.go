package logging

import (
	"strings"
	"testing"
)

func TestRenderPrometheusMetrics(t *testing.T) {
	metricsMutex.Lock()
	metrics = make(map[string]int64)
	metricMetas = make(map[string]metricMeta)
	histograms = make(map[string]*histogramSeries)
	metricsMutex.Unlock()

	IncrMetric("chat_requests_total")
	IncrMetric("chat_requests_total")
	IncrMetric("tool-policy.deny")

	out := RenderPrometheusMetrics()
	if !strings.Contains(out, "# TYPE chat_requests_total counter") {
		t.Fatalf("missing chat metric type line: %s", out)
	}
	if !strings.Contains(out, "chat_requests_total 2") {
		t.Fatalf("missing chat metric value line: %s", out)
	}
	// 非法字符需要被替换为下划线
	if !strings.Contains(out, "tool_policy_deny 1") {
		t.Fatalf("missing sanitized metric line: %s", out)
	}
}

func TestRenderPrometheusHistogram(t *testing.T) {
	metricsMutex.Lock()
	metrics = make(map[string]int64)
	metricMetas = make(map[string]metricMeta)
	histograms = make(map[string]*histogramSeries)
	metricsMutex.Unlock()

	ObserveMetric("chat_request_duration_seconds", 0.02)
	ObserveMetric("chat_request_duration_seconds", 0.20)
	ObserveMetric("chat_request_duration_seconds", 1.50)

	out := RenderPrometheusMetrics()
	if !strings.Contains(out, "# TYPE chat_request_duration_seconds histogram") {
		t.Fatalf("missing histogram type line: %s", out)
	}
	if !strings.Contains(out, "chat_request_duration_seconds_bucket{le=\"0.025\"}") {
		t.Fatalf("missing histogram bucket line: %s", out)
	}
	if !strings.Contains(out, "chat_request_duration_seconds_bucket{le=\"+Inf\"} 3") {
		t.Fatalf("missing histogram +Inf line: %s", out)
	}
	if !strings.Contains(out, "chat_request_duration_seconds_count 3") {
		t.Fatalf("missing histogram count line: %s", out)
	}
}

func TestRenderPrometheusMetricsWithLabels(t *testing.T) {
	metricsMutex.Lock()
	metrics = make(map[string]int64)
	metricMetas = make(map[string]metricMeta)
	histograms = make(map[string]*histogramSeries)
	metricsMutex.Unlock()

	IncrMetricWithLabels("tool_policy_decision_total", map[string]string{
		"tool_name": "http_client",
		"decision":  "deny",
	})
	ObserveMetricWithLabels("chat_request_duration_seconds", map[string]string{
		"endpoint": "/api/chat",
	}, 0.12)

	out := RenderPrometheusMetrics()
	if !strings.Contains(out, `tool_policy_decision_total{decision="deny",tool_name="http_client"} 1`) {
		t.Fatalf("missing labeled counter line: %s", out)
	}
	if !strings.Contains(out, `chat_request_duration_seconds_sum{endpoint="/api/chat"} 0.12`) {
		t.Fatalf("missing labeled histogram sum line: %s", out)
	}
}
