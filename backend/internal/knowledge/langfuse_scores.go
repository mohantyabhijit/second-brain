package knowledge

import (
	"context"
	"strings"

	langfuseclient "github.com/abhijitmohanty/second-brain/backend/internal/platform/langfuse"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func (s *Service) scoreSpan(ctx context.Context, span oteltrace.Span, name string, value any, dataType string, comment string) {
	if !s.cfg.LangfuseTracingEnabled || strings.TrimSpace(s.cfg.LangfuseBaseURL) == "" || span == nil {
		return
	}
	spanContext := span.SpanContext()
	if !spanContext.IsValid() {
		return
	}
	client := langfuseclient.NewClient(s.cfg, s.client)
	err := client.CreateScore(ctx, langfuseclient.ScoreInput{
		TraceID:       spanContext.TraceID().String(),
		ObservationID: spanContext.SpanID().String(),
		Name:          name,
		Value:         value,
		DataType:      dataType,
		Comment:       truncateTelemetryValue(comment),
	})
	if err != nil {
		s.log(ctx).Warn("langfuse score ingestion failed", "score", name, "error", err)
	}
}

func (s *Service) scoreTrace(ctx context.Context, span oteltrace.Span, name string, value any, dataType string, comment string) {
	if !s.cfg.LangfuseTracingEnabled || strings.TrimSpace(s.cfg.LangfuseBaseURL) == "" || span == nil {
		return
	}
	spanContext := span.SpanContext()
	if !spanContext.IsValid() {
		return
	}
	client := langfuseclient.NewClient(s.cfg, s.client)
	err := client.CreateScore(ctx, langfuseclient.ScoreInput{
		TraceID:  spanContext.TraceID().String(),
		Name:     name,
		Value:    value,
		DataType: dataType,
		Comment:  truncateTelemetryValue(comment),
	})
	if err != nil {
		s.log(ctx).Warn("langfuse trace score ingestion failed", "score", name, "error", err)
	}
}

func normalizeLangfuseScore(value float64) float64 {
	if value > 1 && value <= 100 {
		value /= 100
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
