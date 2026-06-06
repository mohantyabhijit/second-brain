package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestObservationSpansKeepStableTraceAttributes(t *testing.T) {
	originalProvider := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(originalProvider)
	})

	service := NewService(config.Config{
		Env:        "test",
		AppRelease: "release-123",
		OwnerID:    "owner-1",
	}, cacheStore{}, nil)
	ctx, root := service.startOperationSpanWithSession(context.Background(), "generate-digest", "digest:2026-06-06", "digest")
	_, child := service.startObservationSpan(ctx, observationOptions{
		Name:          "digest-newsletter-synthesis",
		Type:          "generation",
		PromptVersion: "prompt-v7",
		Tags:          []string{"digest", "newsletter"},
	})
	child.End()
	root.End()

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected two spans, got %d", len(spans))
	}
	for _, span := range spans {
		attrs := spanAttributes(span.Attributes())
		if attrs["langfuse.trace.name"] != "generate-digest" {
			t.Fatalf("span %q changed trace name: %#v", span.Name(), attrs)
		}
		if attrs["langfuse.session.id"] != "digest:2026-06-06" {
			t.Fatalf("span %q changed session id: %#v", span.Name(), attrs)
		}
		if attrs["langfuse.release"] != "release-123" {
			t.Fatalf("span %q lost release: %#v", span.Name(), attrs)
		}
		if attrs["langfuse.version"] != "release-123" {
			t.Fatalf("span %q changed trace version: %#v", span.Name(), attrs)
		}
		if attrs["langfuse.trace.tags"] != `["second-brain","digest"]` {
			t.Fatalf("span %q changed trace tags: %#v", span.Name(), attrs)
		}
	}
}

func TestTelemetryContentCaptureIsOptInAndRedacted(t *testing.T) {
	summary := map[string]any{"chars": 42}
	content := "email me at person@example.com and open https://example.com/private"

	metadataOnly := telemetryPayload(summary, content, false)
	if _, ok := metadataOnly["content"]; ok {
		t.Fatalf("expected metadata-only telemetry, got %#v", metadataOnly)
	}

	captured := telemetryPayload(summary, content, true)
	value, _ := captured["content"].(string)
	if strings.Contains(value, "person@example.com") || strings.Contains(value, "https://example.com") {
		t.Fatalf("expected content redaction, got %q", value)
	}
	if !strings.Contains(value, "[redacted-email]") || !strings.Contains(value, "[redacted-url]") {
		t.Fatalf("expected redaction markers, got %q", value)
	}
}

func spanAttributes(attrs []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		values[string(attr.Key)] = attr.Value.Emit()
	}
	return values
}
