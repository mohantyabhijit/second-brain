package knowledge

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestFetchSupadataTranscriptRetriesMissingNativeTranscript(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")

	requests := []string{}
	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.RawQuery)
			body := `{"message":"Transcript Unavailable"}`
			if len(requests) == 2 {
				body = `{"content":"transcript found on retry","lang":"en","availableLangs":["en"]}`
			}
			return jsonResponse(body), nil
		})},
	}

	transcript := service.fetchSupadataTranscript(context.Background(), "GCXbjwg7gqY")

	if transcript.TranscriptStatus != "available" {
		t.Fatalf("expected transcript to become available, got %q: %s", transcript.TranscriptStatus, transcript.TranscriptError)
	}
	if transcript.TranscriptPreview != "transcript found on retry" {
		t.Fatalf("unexpected transcript preview: %q", transcript.TranscriptPreview)
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 Supadata attempts, got %d", len(requests))
	}
	if !strings.Contains(requests[0], "lang=en") || strings.Contains(requests[1], "lang=") {
		t.Fatalf("expected second attempt to use auto-language fallback, got requests: %#v", requests)
	}
}

func TestFetchSupadataTranscriptReportsAllMissingAttempts(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")

	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return jsonResponse(`{"message":"Transcript Unavailable"}`), nil
		})},
	}

	transcript := service.fetchSupadataTranscript(context.Background(), "missing-video")

	if transcript.TranscriptStatus != "missing" {
		t.Fatalf("expected missing transcript, got %q", transcript.TranscriptStatus)
	}
	for _, label := range []string{"english native", "native auto-language", "hindi native", "default transcript"} {
		if !strings.Contains(transcript.TranscriptError, label) {
			t.Fatalf("expected error to mention %q, got %q", label, transcript.TranscriptError)
		}
	}
}

func TestFetchSupadataTranscriptKeepsTimedSegments(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")

	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return jsonResponse(`{"content":[{"text":"first useful point","start":5.2},{"text":"second useful point","start":75}],"lang":"en","availableLangs":["en"]}`), nil
		})},
	}

	transcript := service.fetchSupadataTranscript(context.Background(), "timed-video")

	if transcript.TranscriptStatus != "available" {
		t.Fatalf("expected available transcript, got %q: %s", transcript.TranscriptStatus, transcript.TranscriptError)
	}
	if !strings.Contains(transcript.TranscriptTimedText, "[0:05] first useful point") || !strings.Contains(transcript.TranscriptTimedText, "[1:15] second useful point") {
		t.Fatalf("expected timestamped transcript text, got %q", transcript.TranscriptTimedText)
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
