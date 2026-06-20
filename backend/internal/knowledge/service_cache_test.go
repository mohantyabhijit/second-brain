package knowledge

import (
	"context"
	"fmt"
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

func TestReadAppStateViewPreservesLargeSourceLimitForCanonicalFallback(t *testing.T) {
	now := time.Date(2026, 5, 31, 6, 0, 0, 0, time.UTC)
	bookmarks := make([]XBookmark, 180)
	for index := range bookmarks {
		bookmarks[index] = XBookmark{
			ID:        fmt.Sprintf("x-%03d", index),
			SourceURL: fmt.Sprintf("https://x.example/%03d", index),
			Body:      fmt.Sprintf("bookmark %03d", index),
			CreatedAt: now.Add(time.Duration(index) * time.Minute).Format(time.RFC3339),
		}
	}
	store := &cacheOrderStore{
		latestView: &Result{
			GeneratedAt: now,
			SourceCounts: AppStateSourceCounts{
				XBookmarks:   523,
				YouTubeItems: 33,
			},
			XBookmarks: bookmarks,
		},
	}
	service := NewService(config.Config{OwnerID: "owner-1"}, store, nil)

	state, status, err := service.ReadAppStateView(context.Background(), "original-x-posts", 180)
	if err != nil {
		t.Fatalf("read source view: %v", err)
	}
	if status != "fallback" {
		t.Fatalf("expected canonical fallback status, got %q", status)
	}
	if store.latestViewLimit != 180 {
		t.Fatalf("expected source view limit 180 to reach store, got %d", store.latestViewLimit)
	}
	if got := len(state.Latest.XBookmarks); got != 180 {
		t.Fatalf("expected 180 x bookmarks, got %d", got)
	}
	if got := state.SourceCounts.XBookmarks; got != 523 {
		t.Fatalf("expected full x source count to be preserved, got %d", got)
	}
	if got := state.SourceCounts.YouTubeItems; got != 33 {
		t.Fatalf("expected youtube source count to be preserved, got %d", got)
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
	latest              *Result
	latestView          *Result
	latestViewLimit     int
	latestViewView      string
	latestViewOwnerID   string
	readLatestViewCalls int
	digests             []DigestIssue
	readLatestCalls     int
}

func (s *cacheOrderStore) ReadLatest(context.Context) (*Result, error) {
	s.readLatestCalls++
	return s.latest, nil
}

func (s *cacheOrderStore) ReadLatestViewForOwner(_ context.Context, ownerID string, view string, limit int) (*Result, error) {
	s.readLatestViewCalls++
	s.latestViewOwnerID = ownerID
	s.latestViewView = view
	s.latestViewLimit = limit
	if s.latestView != nil {
		return s.latestView, nil
	}
	if s.latest != nil {
		return s.latest, nil
	}
	return nil, ErrReadModelCacheMiss
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
