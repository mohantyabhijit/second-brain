package knowledge

import (
	"context"
	"net/http"
	"testing"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type cacheStore struct {
	cached map[string]SynthesisRecord
}

func (s cacheStore) ReadLatest(ctx context.Context) (*Result, error) {
	return nil, nil
}

func (s cacheStore) ReadCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error) {
	return s.cached, nil
}

func (s cacheStore) SaveRun(ctx context.Context, result Result, sources []ProcessedSource) error {
	return nil
}

func TestProcessSourceCandidatesUsesCachedSynthesis(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	candidate := sourceCandidate{
		sourceType:   SourceTypeX,
		externalID:   "tweet-1",
		sourceURL:    "https://x.com/example/status/tweet-1",
		title:        "Cached post",
		body:         "This source has already been synthesized.",
		artifactKind: "tweet",
		contentType:  "text/plain; charset=utf-8",
	}
	captureHash := candidate.captureHash()
	key := SynthesisCacheKey{
		SourceType:    SourceTypeX,
		ExternalID:    candidate.externalID,
		CaptureHash:   captureHash,
		PromptVersion: synthesisPromptVersion,
		Model:         extractiveSynthesisModel,
	}
	store := cacheStore{cached: map[string]SynthesisRecord{
		key.String(): {
			SourceType:    SourceTypeX,
			ExternalID:    candidate.externalID,
			CaptureHash:   captureHash,
			PromptVersion: synthesisPromptVersion,
			Model:         extractiveSynthesisModel,
			Summary: Summary{
				ID:            candidate.externalID,
				Source:        string(SourceTypeX),
				Title:         "Cached post",
				SourceURL:     candidate.sourceURL,
				Decision:      DecisionReadNow,
				Summary:       "Cached summary",
				Confidence:    "medium",
				CaptureHash:   captureHash,
				PromptVersion: synthesisPromptVersion,
				Model:         extractiveSynthesisModel,
			},
		},
	}}
	service := NewService(config.Config{SupabaseStorageBucket: "sources"}, store, http.DefaultClient)

	processed, blockers := service.processSourceCandidates(context.Background(), []sourceCandidate{candidate})
	if len(blockers) != 0 {
		t.Fatalf("unexpected blockers: %v", blockers)
	}
	if len(processed) != 1 {
		t.Fatalf("expected one processed source, got %d", len(processed))
	}
	if !processed[0].Cached {
		t.Fatal("expected synthesis cache hit")
	}
	if processed[0].Synthesis.Summary.CacheStatus != "cached" {
		t.Fatalf("expected cached status, got %q", processed[0].Synthesis.Summary.CacheStatus)
	}
}
