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

type transcriptClaimStore struct {
	cacheStore
	claimed map[string]bool
}

func (s *transcriptClaimStore) ClaimYouTubeTranscriptRequest(_ context.Context, ownerID string, videoID string, monthlyLimit int) (bool, error) {
	key := ownerID + ":" + videoID
	if s.claimed[key] {
		return false, nil
	}
	s.claimed[key] = true
	return true, nil
}

func (s *transcriptClaimStore) CompleteYouTubeTranscriptRequest(_ context.Context, ownerID string, videoID string, status string, detail string) error {
	return nil
}

func TestFetchYouTubeTranscriptsClaimsVideoOnlyOnce(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")
	requests := 0
	store := &transcriptClaimStore{claimed: map[string]bool{}}
	service := NewService(config.Config{OwnerID: "owner-1"}, store, &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		return jsonResponse(`{"content":"one durable transcript","lang":"en"}`), nil
	})})

	first := service.fetchYouTubeTranscriptsForNewMaterials(context.Background(), []YouTubeItem{{VideoID: "video-1"}}, "", nil)
	second := service.fetchYouTubeTranscriptsForNewMaterials(context.Background(), []YouTubeItem{{VideoID: "video-1"}}, "", nil)

	if requests != 1 {
		t.Fatalf("expected one Supadata request for the same video, got %d", requests)
	}
	if first[0].TranscriptStatus != "available" {
		t.Fatalf("expected first transcript request to succeed, got %#v", first[0])
	}
	if second[0].TranscriptStatus != "cached" {
		t.Fatalf("expected repeated video to skip Supadata, got %#v", second[0])
	}
}

func TestFetchSupadataTranscriptUsesOneAutoLanguageRequest(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")

	requests := []string{}
	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests = append(requests, request.URL.RawQuery)
			return jsonResponse(`{"content":"transcript found","lang":"en","availableLangs":["en"]}`), nil
		})},
	}

	transcript := service.fetchSupadataTranscript(context.Background(), "GCXbjwg7gqY")

	if transcript.TranscriptStatus != "available" {
		t.Fatalf("expected transcript to become available, got %q: %s", transcript.TranscriptStatus, transcript.TranscriptError)
	}
	if transcript.TranscriptPreview != "transcript found" {
		t.Fatalf("unexpected transcript preview: %q", transcript.TranscriptPreview)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one Supadata request, got %d", len(requests))
	}
	if strings.Contains(requests[0], "lang=") || !strings.Contains(requests[0], "mode=native") {
		t.Fatalf("expected one native auto-language request, got requests: %#v", requests)
	}
}

func TestFetchSupadataTranscriptDoesNotRetryMissingTranscript(t *testing.T) {
	t.Setenv("SUPADATA_API_KEY", "test-key")

	requests := 0
	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			return jsonResponse(`{"message":"Transcript Unavailable"}`), nil
		})},
	}

	transcript := service.fetchSupadataTranscript(context.Background(), "missing-video")

	if transcript.TranscriptStatus != "missing" {
		t.Fatalf("expected missing transcript, got %q", transcript.TranscriptStatus)
	}
	if requests != 1 {
		t.Fatalf("expected missing transcript to consume one Supadata request, got %d", requests)
	}
	if !strings.Contains(transcript.TranscriptError, "single Supadata native auto-language request") {
		t.Fatalf("expected one-request budget detail, got %q", transcript.TranscriptError)
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
	if len(transcript.ImportantTimeMarkers) != 2 {
		t.Fatalf("expected timestamp markers for every timed segment, got %#v", transcript.ImportantTimeMarkers)
	}
	if transcript.ImportantTimeMarkers[0].Seconds != 5 || transcript.ImportantTimeMarkers[1].Seconds != 75 {
		t.Fatalf("unexpected marker seconds: %#v", transcript.ImportantTimeMarkers)
	}
}

func TestFetchYouTubeTranscriptsSkipsAlreadyProcessedMaterial(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	called := false
	service := &Service{
		cfg: config.Config{OpenAITranslationModel: "test-model"},
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			called = true
			return jsonResponse(`{"content":"should not be fetched","lang":"en"}`), nil
		})},
	}
	key := SourceMaterialKey{
		SourceType:    SourceTypeYouTube,
		ExternalID:    "video-1",
		PromptVersion: synthesisPromptVersion,
		Model:         extractiveSynthesisModel,
	}

	items := service.fetchYouTubeTranscriptsForNewMaterials(context.Background(), []YouTubeItem{{VideoID: "video-1"}}, "", map[string]SourceMaterialState{
		key.String(): {
			SourceType:        SourceTypeYouTube,
			ExternalID:        "video-1",
			PromptVersion:     synthesisPromptVersion,
			Model:             extractiveSynthesisModel,
			ArtifactKind:      "transcript",
			LatestCaptureHash: "capture-1",
			Processed:         true,
		},
	})

	if called {
		t.Fatal("expected cached YouTube source material to skip Supadata")
	}
	if len(items) != 1 || items[0].TranscriptStatus != "cached" {
		t.Fatalf("expected cached transcript status, got %#v", items)
	}
}

func TestParseYouTubeDuration(t *testing.T) {
	if got := parseYouTubeDuration("PT1H22M25S"); got != 4945 {
		t.Fatalf("expected 4945 seconds, got %d", got)
	}
	if got := parseYouTubeDuration("PT12M4S"); got != 724 {
		t.Fatalf("expected 724 seconds, got %d", got)
	}
}

func TestMergeTranscriptEstimatesMarkersWhenTimedSegmentsAreMissing(t *testing.T) {
	item := YouTubeItem{
		VideoID:         "video-1",
		DurationSeconds: 1200,
	}
	transcript := YouTubeItem{
		TranscriptStatus: "available",
		TranscriptText: strings.Repeat("Build leveraged products before adding more manual effort. ", 12) +
			strings.Repeat("Use customer pull to decide which workflows deserve automation. ", 12) +
			strings.Repeat("Treat distribution as part of the product instead of a launch chore. ", 12),
	}

	merged := mergeTranscript(item, transcript)

	if len(merged.ImportantTimeMarkers) == 0 {
		t.Fatal("expected estimated time markers when transcript has no timed segments")
	}
	for _, marker := range merged.ImportantTimeMarkers {
		if marker.Seconds < 0 || marker.Seconds > item.DurationSeconds {
			t.Fatalf("marker outside video duration: %#v", marker)
		}
	}
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
