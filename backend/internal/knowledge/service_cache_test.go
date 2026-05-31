package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestReadAppStateViewChecksReadModelBeforeMemo(t *testing.T) {
	now := time.Date(2026, 5, 31, 6, 0, 0, 0, time.UTC)
	store := &cacheOrderStore{
		latest: &Result{
			GeneratedAt: now,
			Insights:    []Insight{{ID: "fallback", Title: "fallback insight"}},
		},
	}
	cache := &cacheOrderReadModel{err: ErrReadModelCacheMiss}
	service := NewService(config.Config{OwnerID: "owner-1"}, store, nil)
	service.SetReadModelCache(cache)

	state, status, err := service.ReadAppStateView(context.Background(), "insights", 1)
	if err != nil {
		t.Fatalf("read fallback view: %v", err)
	}
	if status != "fallback" {
		t.Fatalf("expected fallback status, got %q", status)
	}
	if got := state.Latest.Insights[0].ID; got != "fallback" {
		t.Fatalf("expected fallback insight, got %q", got)
	}

	redisState := BuildAppState("owner-1", &Result{
		GeneratedAt: now.Add(time.Minute),
		Insights:    []Insight{{ID: "redis", Title: "redis insight"}},
	}, nil, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "")
	cache.state = CompactAppStateForView(&redisState, "insights", 1)
	cache.err = nil

	state, status, err = service.ReadAppStateView(context.Background(), "insights", 1)
	if err != nil {
		t.Fatalf("read cached view: %v", err)
	}
	if status != "hit" {
		t.Fatalf("expected redis hit status, got %q", status)
	}
	if got := state.Latest.Insights[0].ID; got != "redis" {
		t.Fatalf("expected redis insight, got %q", got)
	}
}

func TestGenerateDigestUsesCanonicalLatestInsteadOfRedisLatest(t *testing.T) {
	now := time.Date(2026, 5, 31, 6, 0, 0, 0, time.UTC)
	store := &cacheOrderStore{
		latest: &Result{
			GeneratedAt: now,
			Summaries:   []Summary{},
			Insights:    []Insight{},
			Validation:  []ValidationItem{},
			Blockers:    []string{},
		},
	}
	cache := &cacheOrderReadModel{
		latest: &Result{
			GeneratedAt: now.Add(-time.Hour),
			Summaries: []Summary{{
				ID:         "stale-summary",
				Source:     "x",
				Title:      "stale",
				SourceURL:  "https://x.example/stale",
				Summary:    "stale cached summary",
				Confidence: "high",
			}},
			Insights: []Insight{{
				ID:         "stale-insight",
				Source:     "x",
				SourceID:   "stale-summary",
				Title:      "stale",
				Insight:    "stale cached insight",
				Evidence:   "stale evidence",
				SourceURL:  "https://x.example/stale",
				Confidence: "high",
			}},
		},
	}
	service := NewService(config.Config{OwnerID: "owner-1"}, store, nil)
	service.SetReadModelCache(cache)

	_, err := service.GenerateDigest(context.Background())
	if err == nil {
		t.Fatal("expected digest generation to stop on canonical empty inputs")
	}
	if !strings.Contains(err.Error(), "no source-grounded digest inputs") {
		t.Fatalf("expected canonical empty-input error, got %v", err)
	}
	if cache.readLatestCalls != 0 {
		t.Fatalf("expected digest generation not to read stale Redis latest, got %d cache reads", cache.readLatestCalls)
	}
	if store.readLatestCalls == 0 {
		t.Fatal("expected digest generation to read canonical latest")
	}
}

type cacheOrderStore struct {
	latest          *Result
	digests         []DigestIssue
	readLatestCalls int
}

func (s *cacheOrderStore) ReadLatest(context.Context) (*Result, error) {
	s.readLatestCalls++
	return s.latest, nil
}

func (s *cacheOrderStore) ReadCachedSyntheses(context.Context, []SynthesisCacheKey) (map[string]SynthesisRecord, error) {
	return map[string]SynthesisRecord{}, nil
}

func (s *cacheOrderStore) ReadSourceMaterialStates(context.Context, string, []SourceMaterialKey) (map[string]SourceMaterialState, error) {
	return map[string]SourceMaterialState{}, nil
}

func (s *cacheOrderStore) SaveRun(context.Context, Result, []ProcessedSource) error {
	return nil
}

func (s *cacheOrderStore) SaveFeedback(context.Context, FeedbackEvent) error {
	return nil
}

func (s *cacheOrderStore) ReadDigests(context.Context, int) ([]DigestIssue, error) {
	return s.digests, nil
}

func (s *cacheOrderStore) ReadDigestIllustration(context.Context, string, string) (*DigestIllustration, error) {
	return nil, nil
}

func (s *cacheOrderStore) SaveDigest(_ context.Context, digest DigestIssue) (*DigestIssue, error) {
	return &digest, nil
}

func (s *cacheOrderStore) ReadXTokens(context.Context, string) (*EncryptedXTokens, error) {
	return nil, nil
}

func (s *cacheOrderStore) SaveXTokens(context.Context, EncryptedXTokens) error {
	return nil
}

type cacheOrderReadModel struct {
	state           *AppState
	latest          *Result
	err             error
	readLatestCalls int
}

func (c *cacheOrderReadModel) ReadAppViewState(context.Context, string, string, int) (*AppState, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.state, nil
}

func (c *cacheOrderReadModel) ReadAppState(context.Context, string) (*AppState, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *cacheOrderReadModel) ReadLatest(context.Context, string) (*Result, error) {
	c.readLatestCalls++
	if c.latest != nil {
		return c.latest, nil
	}
	return nil, ErrReadModelCacheMiss
}

func (c *cacheOrderReadModel) ReadDigests(context.Context, string, int) ([]DigestIssue, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *cacheOrderReadModel) ReadSourceMaterialStates(context.Context, string, []SourceMaterialKey) (map[string]SourceMaterialState, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *cacheOrderReadModel) ReadRefreshStatus(context.Context, string) (*RefreshStatus, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *cacheOrderReadModel) WriteRefreshStatus(context.Context, string, RefreshStatus) error {
	return nil
}

func (c *cacheOrderReadModel) PublishSourceMaterialStates(context.Context, string, []SourceMaterialState) error {
	return nil
}

func (c *cacheOrderReadModel) PublishAppState(context.Context, string, AppState) error {
	return nil
}

func (c *cacheOrderReadModel) Close() error {
	return nil
}
