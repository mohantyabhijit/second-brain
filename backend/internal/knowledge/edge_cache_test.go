package knowledge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestPublishAppStatePurgesEdgeCacheForDigestPublish(t *testing.T) {
	var purgedFiles []string
	var purgeRequests int
	client := &http.Client{Transport: edgeCacheRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/client/v4/zones/test-zone/purge_cache" {
			t.Fatalf("unexpected purge path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		var payload struct {
			Files []string `json:"files"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode purge payload: %v", err)
		}
		if len(payload.Files) > cloudflarePurgeFileBatchSize {
			t.Fatalf("expected purge batch under %d files, got %d", cloudflarePurgeFileBatchSize, len(payload.Files))
		}
		purgeRequests++
		purgedFiles = append(purgedFiles, payload.Files...)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			Request:    r,
		}, nil
	})}

	now := time.Date(2026, 5, 31, 6, 0, 0, 0, time.UTC)
	state := BuildAppState("owner-1", &Result{
		GeneratedAt: now,
		Digest: &DigestIssue{
			ID:             "digest-1",
			DigestDate:     "2026-05-31",
			ScheduledFor:   now,
			IdempotencyKey: "daily:2026-05-31",
			Subject:        "Digest",
			BodyMarkdown:   "# Digest",
			Status:         "sent",
		},
		Validation: []ValidationItem{},
		Blockers:   []string{},
	}, nil, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "")
	cache := &recordingReadModelCache{}
	service := NewService(config.Config{
		OwnerID:                     "owner-1",
		PublicBaseURL:               "https://abhijitmohanty.com/second-brain",
		CloudflareAPIToken:          "test-token",
		CloudflareZoneID:            "test-zone",
		CloudflareAPIBaseURL:        "https://cloudflare.example/client/v4",
		CloudflareCachePurgeEnabled: true,
	}, &cacheOrderStore{}, client)
	service.SetReadModelCache(cache)

	if err := service.publishAppStateBestEffort(context.Background(), state, "digest_publish"); err != nil {
		t.Fatalf("publish app state: %v", err)
	}
	if cache.publishCalls != 1 {
		t.Fatalf("expected one Redis publish, got %d", cache.publishCalls)
	}
	if !containsString(purgedFiles, "https://abhijitmohanty.com/second-brain/daily-newsletter/") {
		t.Fatalf("expected daily newsletter page purge, got %#v", purgedFiles)
	}
	if !containsString(purgedFiles, "https://abhijitmohanty.com/second-brain/api/app-state?view=daily-newsletter&limit=10") {
		t.Fatalf("expected view-scoped app-state purge, got %#v", purgedFiles)
	}
	if !containsString(purgedFiles, "https://www.abhijitmohanty.com/second-brain/api/app-state?view=original-x-posts&limit=25") {
		t.Fatalf("expected www source app-state purge, got %#v", purgedFiles)
	}
	if purgeRequests < 2 {
		t.Fatalf("expected purge URLs to be chunked across multiple requests, got %d", purgeRequests)
	}
}

func TestEdgeCachePurgeURLsIncludesApexAndWWWVariants(t *testing.T) {
	fromApex := edgeCachePurgeURLs("https://abhijitmohanty.com/second-brain")
	if !containsString(fromApex, "https://abhijitmohanty.com/second-brain/original-x-bookmarks/") {
		t.Fatalf("expected apex purge URL, got %#v", fromApex)
	}
	if !containsString(fromApex, "https://www.abhijitmohanty.com/second-brain/original-x-bookmarks/") {
		t.Fatalf("expected www purge URL, got %#v", fromApex)
	}

	fromWWW := edgeCachePurgeURLs("https://www.abhijitmohanty.com/second-brain")
	if !containsString(fromWWW, "https://www.abhijitmohanty.com/second-brain/api/app-state?view=original-x-posts&limit=1000") {
		t.Fatalf("expected www source purge URL, got %#v", fromWWW)
	}
	if !containsString(fromWWW, "https://abhijitmohanty.com/second-brain/api/app-state?view=original-x-posts&limit=1000") {
		t.Fatalf("expected apex source purge URL, got %#v", fromWWW)
	}
}

type edgeCacheRoundTripFunc func(*http.Request) (*http.Response, error)

func (f edgeCacheRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type recordingReadModelCache struct {
	publishCalls int
}

func (c *recordingReadModelCache) ReadAppState(context.Context, string) (*AppState, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *recordingReadModelCache) ReadLatest(context.Context, string) (*Result, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *recordingReadModelCache) ReadDigests(context.Context, string, int) ([]DigestIssue, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *recordingReadModelCache) ReadSourceMaterialStates(context.Context, string, []SourceMaterialKey) (map[string]SourceMaterialState, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *recordingReadModelCache) ReadRefreshStatus(context.Context, string) (*RefreshStatus, error) {
	return nil, ErrReadModelCacheMiss
}

func (c *recordingReadModelCache) WriteRefreshStatus(context.Context, string, RefreshStatus) error {
	return nil
}

func (c *recordingReadModelCache) PublishSourceMaterialStates(context.Context, string, []SourceMaterialState) error {
	return nil
}

func (c *recordingReadModelCache) PublishAppState(context.Context, string, AppState) error {
	c.publishCalls++
	return nil
}

func (c *recordingReadModelCache) Close() error {
	return nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
