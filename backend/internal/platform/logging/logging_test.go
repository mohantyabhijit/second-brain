package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestLoggerEmitsRequiredStructuredFields(t *testing.T) {
	var buffer bytes.Buffer
	logger := New(Options{
		ServiceName: "api",
		Environment: "test",
		Level:       "info",
		Writer:      &buffer,
	})
	ctx := WithRequestMetadata(context.Background(), "request-1")
	SetTraceID(ctx, "trace-1")
	SetUserID(ctx, "user-1")

	logger.InfoContext(ctx, "request handled", "method", "GET", "status", 200)

	var event map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &event); err != nil {
		t.Fatalf("unmarshal log event: %v", err)
	}
	for _, key := range []string{"timestamp", "service_name", "environment", "request_id", "trace_id", "user_id", "log_level", "message"} {
		if _, ok := event[key]; !ok {
			t.Fatalf("expected log field %q in %#v", key, event)
		}
	}
	if event["service_name"] != "api" || event["environment"] != "test" || event["request_id"] != "request-1" || event["trace_id"] != "trace-1" || event["user_id"] != "user-1" {
		t.Fatalf("unexpected correlation fields: %#v", event)
	}
}

func TestLoggerRedactsSensitiveFieldsAndAddsErrorStack(t *testing.T) {
	var buffer bytes.Buffer
	logger := New(Options{
		ServiceName:       "api",
		Environment:       "test",
		Level:             "info",
		Writer:            &buffer,
		IncludeErrorStack: true,
	})

	logger.Error("request failed", "error", errors.New("boom"), "access_token", "secret-value")

	var event map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &event); err != nil {
		t.Fatalf("unmarshal log event: %v", err)
	}
	if event["error"] != "boom" {
		t.Fatalf("expected error field, got %#v", event)
	}
	if event["access_token"] != "[REDACTED]" {
		t.Fatalf("expected redacted token field, got %#v", event)
	}
	if _, ok := event["stack"].(string); !ok {
		t.Fatalf("expected stack field, got %#v", event)
	}
}
