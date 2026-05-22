package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONWritesStatusHeaderAndPayload(t *testing.T) {
	recorder := httptest.NewRecorder()

	JSON(recorder, http.StatusAccepted, map[string]string{"status": "ok"})

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestErrorWritesJSONErrorPayload(t *testing.T) {
	recorder := httptest.NewRecorder()

	Error(recorder, http.StatusBadGateway, "upstream failed")

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, recorder.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload["error"] != "upstream failed" {
		t.Fatalf("unexpected error payload: %#v", payload)
	}
}
