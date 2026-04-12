package observability

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Config OTLP 追踪（可对接 Langfuse Cloud、Jaeger、Grafana Tempo 等 OTLP 兼容后端）
type Config struct {
	Enabled      bool   `yaml:"enabled"`
	OTLPEndpoint string `yaml:"otlp_endpoint"` // 例: http://localhost:4318/v1/traces
	ServiceName  string `yaml:"service_name"`
	Insecure     bool   `yaml:"insecure"` // 本地 http 常为 true
}

// Init 安装全局 TracerProvider；未启用或 endpoint 为空时返回空 shutdown。
func Init(ctx context.Context, c Config) (func(context.Context) error, error) {
	if !c.Enabled || strings.TrimSpace(c.OTLPEndpoint) == "" {
		return func(context.Context) error { return nil }, nil
	}
	name := c.ServiceName
	if name == "" {
		name = "circle-go"
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpointURL(strings.TrimSpace(c.OTLPEndpoint)),
	}
	if c.Insecure || strings.HasPrefix(strings.ToLower(c.OTLPEndpoint), "http://") {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithAttributes(semconv.ServiceName(name)),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer 返回命名 tracer（与 Langfuse 等产品在 UI 上按 instrumentation scope 分组类似）
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Span 便捷包装：ctx 上挂 span，结束时可附加事件
func Span(ctx context.Context, tracer trace.Tracer, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return tracer.Start(ctx, name, opts...)
}

// RecordLLMCall 在 span 上记录一次 LLM 调用元数据（类 Langfuse generation 的精简版）
func RecordLLMCall(span trace.Span, model string, inputTokens, outputTokens int, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.SetAttributes(attribute.String("llm.model", model))
	if inputTokens > 0 {
		span.SetAttributes(attribute.Int("llm.usage.input_tokens", inputTokens))
	}
	if outputTokens > 0 {
		span.SetAttributes(attribute.Int("llm.usage.output_tokens", outputTokens))
	}
	if err != nil {
		span.RecordError(err)
	}
}

// RecordTool 记录工具调用
func RecordTool(span trace.Span, toolName string, err error) {
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent("tool_call", trace.WithAttributes(attribute.String("tool.name", toolName)))
	if err != nil {
		span.RecordError(err)
	}
}

// Since 用于简单耗时属性
func Since(start time.Time) float64 {
	return time.Since(start).Seconds()
}
