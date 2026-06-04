package localfile

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
)

func TestReadLatestReturnsNilWhenRunFileDoesNotExist(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "missing", "latest.json"))

	latest, err := store.ReadLatest(context.Background())

	if err != nil {
		t.Fatalf("read missing latest: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil latest for missing file, got %#v", latest)
	}
}

func TestYouTubeTranscriptRequestClaimPersistsAcrossStoreInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "latest.json")
	first := New(path)

	claimed, err := first.ClaimYouTubeTranscriptRequest(context.Background(), "owner-1", "video-1", 100)
	if err != nil {
		t.Fatalf("claim transcript request: %v", err)
	}
	if !claimed {
		t.Fatal("expected first transcript request claim to succeed")
	}
	if err := first.CompleteYouTubeTranscriptRequest(context.Background(), "owner-1", "video-1", "missing", "not available"); err != nil {
		t.Fatalf("complete transcript request: %v", err)
	}

	second := New(path)
	claimed, err = second.ClaimYouTubeTranscriptRequest(context.Background(), "owner-1", "video-1", 100)
	if err != nil {
		t.Fatalf("recheck transcript request claim: %v", err)
	}
	if claimed {
		t.Fatal("expected transcript request claim to survive process restart")
	}
}

func TestYouTubeTranscriptRequestClaimHonorsMonthlyLimit(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "runtime", "latest.json"))

	first, err := store.ClaimYouTubeTranscriptRequest(context.Background(), "owner-1", "video-1", 1)
	if err != nil {
		t.Fatalf("claim first transcript request: %v", err)
	}
	second, err := store.ClaimYouTubeTranscriptRequest(context.Background(), "owner-1", "video-2", 1)
	if err != nil {
		t.Fatalf("claim second transcript request: %v", err)
	}

	if !first || second {
		t.Fatalf("expected one claim within monthly limit, got first=%t second=%t", first, second)
	}
}

func TestSaveRunReadLatestAndReadCachedSyntheses(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "runtime", "latest.json"))
	generatedAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	summaryGeneratedAt := generatedAt.Add(5 * time.Minute)
	result := knowledge.Result{
		GeneratedAt: generatedAt,
		Summaries: []knowledge.Summary{
			{
				ID:            "tweet-1",
				Source:        "x",
				Title:         "Cached tweet",
				SourceURL:     "https://x.com/example/status/tweet-1",
				Decision:      knowledge.DecisionReadNow,
				Summary:       "Cached summary",
				Confidence:    "medium",
				CaptureHash:   "hash-1",
				PromptVersion: "prompt-v1",
				Model:         "model-v1",
				GeneratedAt:   &summaryGeneratedAt,
			},
			{
				ID:            "video-1",
				Source:        "youtube",
				Title:         "Unrequested video",
				SourceURL:     "https://www.youtube.com/watch?v=video-1",
				Decision:      knowledge.DecisionLater,
				Summary:       "Other summary",
				Confidence:    "low",
				CaptureHash:   "hash-2",
				PromptVersion: "prompt-v1",
				Model:         "model-v1",
			},
		},
		Insights: []knowledge.Insight{
			{
				ID:         "insight-1",
				Source:     "x",
				SourceID:   "tweet-1",
				Title:      "Cached tweet",
				Insight:    "Reusable insight",
				Evidence:   "Evidence",
				SourceURL:  "https://x.com/example/status/tweet-1",
				Confidence: "high",
			},
		},
		ActionItems: []knowledge.ActionItem{
			{
				ID:        "action-1",
				Source:    "x",
				SourceID:  "tweet-1",
				Title:     "Follow up",
				Action:    "Turn into a note",
				Rationale: "Grounded in source",
				SourceURL: "https://x.com/example/status/tweet-1",
				Priority:  "high",
			},
		},
	}

	if err := store.SaveRun(context.Background(), result, nil); err != nil {
		t.Fatalf("save run: %v", err)
	}

	latest, err := store.ReadLatest(context.Background())
	if err != nil {
		t.Fatalf("read latest: %v", err)
	}
	if latest == nil || !latest.GeneratedAt.Equal(generatedAt) || len(latest.Summaries) != 2 {
		t.Fatalf("unexpected latest result: %#v", latest)
	}

	key := knowledge.SynthesisCacheKey{
		SourceType:    knowledge.SourceTypeX,
		ExternalID:    "tweet-1",
		CaptureHash:   "hash-1",
		PromptVersion: "prompt-v1",
		Model:         "model-v1",
	}
	cached, err := store.ReadCachedSyntheses(context.Background(), []knowledge.SynthesisCacheKey{
		key,
		{
			SourceType:    knowledge.SourceTypeYouTube,
			ExternalID:    "video-1",
			CaptureHash:   "different-hash",
			PromptVersion: "prompt-v1",
			Model:         "model-v1",
		},
	})
	if err != nil {
		t.Fatalf("read cached syntheses: %v", err)
	}
	record, ok := cached[key.String()]
	if !ok {
		t.Fatalf("expected cache hit for %s, got %#v", key.String(), cached)
	}
	if record.Summary.ID != "tweet-1" || len(record.Insights) != 1 || len(record.ActionItems) != 1 {
		t.Fatalf("unexpected cached record: %#v", record)
	}
	if !record.GeneratedAt.Equal(summaryGeneratedAt) {
		t.Fatalf("expected summary generated_at to win, got %s", record.GeneratedAt)
	}
	if len(cached) != 1 {
		t.Fatalf("expected only matching cache key, got %#v", cached)
	}
}
