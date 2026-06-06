package langfuse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestCompileTextPrompt(t *testing.T) {
	template := "Digest {{digest_date}}\n{{ input_json }}"
	got := CompileTextPrompt(template, map[string]string{
		"digest_date": "2026-06-05",
		"input_json":  `{"items":1}`,
	})
	want := "Digest 2026-06-05\n{\"items\":1}"
	if got != want {
		t.Fatalf("compile prompt mismatch\nwant: %q\n got: %q", want, got)
	}
}

func TestCreateScorePostsLangfuseScore(t *testing.T) {
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/scores" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk:sk"))
		if r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("unexpected authorization: %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode score: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(config.Config{
		LangfuseBaseURL:   server.URL,
		LangfusePublicKey: "pk",
		LangfuseSecretKey: "sk",
	}, server.Client())
	err := client.CreateScore(context.Background(), ScoreInput{
		TraceID:       "trace-1",
		ObservationID: "observation-1",
		Name:          "quality",
		Value:         0.91,
		DataType:      "numeric",
		Comment:       "grounded",
	})
	if err != nil {
		t.Fatalf("create score: %v", err)
	}
	if payload["traceId"] != "trace-1" || payload["observationId"] != "observation-1" || payload["name"] != "quality" || payload["dataType"] != "NUMERIC" {
		t.Fatalf("unexpected score payload: %#v", payload)
	}
}
