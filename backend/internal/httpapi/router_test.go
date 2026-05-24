package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
)

func TestRouterServesHealthAndCORSPreflight(t *testing.T) {
	router := testRouter(t)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", health.Code)
	}

	preflight := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/knowledge-runs/refresh", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	router.ServeHTTP(preflight, request)

	if preflight.Code != http.StatusNoContent {
		t.Fatalf("expected preflight status 204, got %d", preflight.Code)
	}
	if got := preflight.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
}

func TestRouterReadsAndRefreshesKnowledgeRunsWithoutProviderSecrets(t *testing.T) {
	router := testRouter(t)

	latest := httptest.NewRecorder()
	router.ServeHTTP(latest, httptest.NewRequest(http.MethodGet, "/api/knowledge-runs/latest", nil))
	if latest.Code != http.StatusOK {
		t.Fatalf("expected latest status 200, got %d: %s", latest.Code, latest.Body.String())
	}

	var latestPayload struct {
		Latest *knowledge.Result `json:"latest"`
	}
	if err := json.Unmarshal(latest.Body.Bytes(), &latestPayload); err != nil {
		t.Fatalf("decode latest response: %v", err)
	}
	if latestPayload.Latest != nil {
		t.Fatalf("expected no saved run before refresh, got %#v", latestPayload.Latest)
	}

	refresh := httptest.NewRecorder()
	router.ServeHTTP(refresh, httptest.NewRequest(http.MethodPost, "/api/knowledge-runs/refresh", nil))
	if refresh.Code != http.StatusAccepted {
		t.Fatalf("expected refresh status 202, got %d: %s", refresh.Code, refresh.Body.String())
	}

	var status knowledge.RefreshStatus
	if err := json.Unmarshal(refresh.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode refresh response: %v", err)
	}
	if status.Status != "running" {
		t.Fatalf("expected running refresh status, got %#v", status)
	}

	result := waitForLatestRun(t, router)
	if len(result.Validation) == 0 {
		t.Fatal("expected validation checks in persisted refresh response")
	}
	if len(result.Blockers) < 2 {
		t.Fatalf("expected missing-secret blockers, got %#v", result.Blockers)
	}
	if !containsSubstring(result.Blockers, "X_USER_ACCESS_TOKEN") {
		t.Fatalf("expected X credential blocker, got %#v", result.Blockers)
	}
	if !containsSubstring(result.Blockers, "YOUTUBE_PLAYLIST_ID") {
		t.Fatalf("expected YouTube playlist blocker, got %#v", result.Blockers)
	}

	latestAfterRefresh := httptest.NewRecorder()
	router.ServeHTTP(latestAfterRefresh, httptest.NewRequest(http.MethodGet, "/api/knowledge-runs/latest", nil))
	if latestAfterRefresh.Code != http.StatusOK {
		t.Fatalf("expected latest-after-refresh status 200, got %d", latestAfterRefresh.Code)
	}
	if !strings.Contains(latestAfterRefresh.Body.String(), "\"latest\"") {
		t.Fatalf("expected persisted latest run payload, got %s", latestAfterRefresh.Body.String())
	}

	invalidDigestSend := httptest.NewRecorder()
	router.ServeHTTP(invalidDigestSend, httptest.NewRequest(http.MethodPost, "/api/digests/send", strings.NewReader(`{"recipientEmail":"not-an-email"}`)))
	if invalidDigestSend.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid digest send status 400, got %d", invalidDigestSend.Code)
	}

	digestSend := httptest.NewRecorder()
	router.ServeHTTP(digestSend, httptest.NewRequest(http.MethodPost, "/api/digests/send", strings.NewReader(`{"recipientEmail":"reader@example.com"}`)))
	if digestSend.Code != http.StatusOK {
		t.Fatalf("expected digest send status 200, got %d: %s", digestSend.Code, digestSend.Body.String())
	}
	var digest knowledge.DigestIssue
	if err := json.Unmarshal(digestSend.Body.Bytes(), &digest); err != nil {
		t.Fatalf("decode digest send response: %v", err)
	}
	if len(digest.Deliveries) != 1 || digest.Deliveries[0].Recipient != "reader@example.com" {
		t.Fatalf("expected delivery to requested recipient, got %#v", digest.Deliveries)
	}
}

func waitForLatestRun(t *testing.T, router http.Handler) knowledge.Result {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		latestAfterRefresh := httptest.NewRecorder()
		router.ServeHTTP(latestAfterRefresh, httptest.NewRequest(http.MethodGet, "/api/knowledge-runs/latest", nil))
		if latestAfterRefresh.Code != http.StatusOK {
			t.Fatalf("expected latest-after-refresh status 200, got %d", latestAfterRefresh.Code)
		}
		var payload struct {
			Latest *knowledge.Result `json:"latest"`
		}
		if err := json.Unmarshal(latestAfterRefresh.Body.Bytes(), &payload); err != nil {
			t.Fatalf("decode latest-after-refresh response: %v", err)
		}
		if payload.Latest != nil {
			return *payload.Latest
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for async refresh to persist latest run")
	return knowledge.Result{}
}

func testRouter(t *testing.T) http.Handler {
	t.Helper()
	clearProviderEnv(t)

	cfg := config.Config{
		Env:                    "test",
		AllowedOrigins:         []string{"http://localhost:3000"},
		OneCLIBin:              filepath.Join(t.TempDir(), "missing-onecli"),
		KnowledgeRunPath:       filepath.Join(t.TempDir(), "latest-knowledge-run.json"),
		YouTubePlaylistID:      "",
		SupabaseStorageBucket:  "sources",
		OpenAITranslationModel: "gpt-4o-mini",
		OpenAISynthesisModel:   "gpt-4o-mini",
	}
	store := localfile.New(cfg.KnowledgeRunPath)
	service := knowledge.NewService(cfg, store, http.DefaultClient)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, service, logger)
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"X_USER_ACCESS_TOKEN",
		"YOUTUBE_API_KEY",
		"YOUTUBE_ACCESS_TOKEN",
		"SUPADATA_API_KEY",
		"OPENAI_API_KEY",
		"ONECLI_GATEWAY",
	} {
		t.Setenv(key, "")
	}
}

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
