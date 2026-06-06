package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestGenerateDigestIllustrationUsesOpenAIImagesAPI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	requestBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.openai.com/v1/images/generations" {
			t.Fatalf("unexpected URL: %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(raw)
		image := base64.StdEncoding.EncodeToString([]byte("fake image bytes"))
		return jsonResponse(`{"data":[{"b64_json":` + string(mustJSON(t, image)) + `}]}`), nil
	})}
	service := NewService(config.Config{OpenAIImageModel: "test-image-model", OpenAIImageQuality: "medium"}, cacheStore{}, client)

	illustration, err := service.generateDigestIllustration(context.Background(), DigestIssue{
		Subject:      "Build better learning loops",
		BodyMarkdown: "# Abhijit's Second Brain\n\nWelcome back. Better filters make each next move more valuable.",
	}, []Insight{{Title: "Learning loops"}}, []ThemeCluster{{Label: "Feedback"}}, nil)
	if err != nil {
		t.Fatalf("generate illustration: %v", err)
	}
	if illustration.model != "test-image-model" || illustration.mimeType != "image/png" || illustration.base64 == "" {
		t.Fatalf("unexpected illustration: %#v", illustration)
	}
	if !strings.Contains(requestBody, "Excalidraw-like") || !strings.Contains(requestBody, "black ink only") || !strings.Contains(requestBody, `"size":"1024x1024"`) || !strings.Contains(requestBody, `"quality":"medium"`) {
		t.Fatalf("expected black-on-white illustration prompt and size, got %s", requestBody)
	}
}

func mustJSON(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}
