package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestPromptDigestRetriesMalformedJSONResponse(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.URL.String() != "https://api.openai.com/v1/responses" {
			t.Fatalf("unexpected URL: %s", request.URL.String())
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		text, ok := body["text"].(map[string]any)
		if !ok {
			t.Fatalf("expected structured text format, got %#v", body["text"])
		}
		format, ok := text["format"].(map[string]any)
		if !ok || format["type"] != "json_object" {
			t.Fatalf("expected json_object response format, got %#v", text["format"])
		}
		if calls == 1 {
			return jsonResponse(`{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output_text":"{\"subject\":\"truncated\""}`), nil
		}
		if input, _ := body["input"].(string); !strings.Contains(input, "previous response was incomplete or malformed") {
			t.Fatalf("expected retry instruction, got %q", input)
		}
		return jsonResponse(`{"status":"completed","output_text":"{\"subject\":\"Recovered digest\",\"body_lines\":[\"# Digest\",\"\",\"Recovered body.\"]}"}`), nil
	})}
	service := NewService(config.Config{OpenAISynthesisModel: "gpt-5.5"}, nil, client)

	payload, err := service.promptDigestWithLines(context.Background(), "", []string{"Write the digest."}, 5000)
	if err != nil {
		t.Fatalf("prompt digest: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected one retry, got %d calls", calls)
	}
	if payload.Subject != "Recovered digest" || len(payload.BodyLines) != 3 {
		t.Fatalf("unexpected recovered payload: %#v", payload)
	}
}
