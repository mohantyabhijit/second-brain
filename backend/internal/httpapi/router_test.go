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
	router.ServeHTTP(digestSend, httptest.NewRequest(http.MethodPost, "/api/digests/send", strings.NewReader(`{"recipientEmail":"reader@example.com","digest":{"digestDate":"2026-05-24","subject":"Displayed digest","bodyMarkdown":"# Displayed digest\n\nA source-grounded newsletter body."}}`)))
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

func TestRouterServesAppStateFallback(t *testing.T) {
	router := testRouter(t)

	appState := httptest.NewRecorder()
	router.ServeHTTP(appState, httptest.NewRequest(http.MethodGet, "/api/app-state", nil))
	if appState.Code != http.StatusOK {
		t.Fatalf("expected app-state status 200, got %d: %s", appState.Code, appState.Body.String())
	}
	if got := appState.Header().Get("X-Second-Brain-Cache"); got != "fallback" {
		t.Fatalf("expected app-state cache fallback header, got %q", got)
	}
	if got := appState.Header().Get("Cache-Control"); !strings.Contains(got, "s-maxage=300") {
		t.Fatalf("expected app-state CDN cache header, got %q", got)
	}
	etag := appState.Header().Get("ETag")
	if etag == "" {
		t.Fatal("expected app-state ETag")
	}
	var payload knowledge.AppState
	if err := json.Unmarshal(appState.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode app-state response: %v", err)
	}
	if payload.Manifest.SchemaVersion != knowledge.AppStateSchemaVersion {
		t.Fatalf("expected schema version %q, got %q", knowledge.AppStateSchemaVersion, payload.Manifest.SchemaVersion)
	}
	if payload.Latest != nil {
		t.Fatalf("expected no latest run in empty app-state, got %#v", payload.Latest)
	}
	if payload.Digests == nil {
		t.Fatal("expected normalized digest list")
	}

	if !strings.HasPrefix(etag, `"`) || !strings.HasSuffix(etag, `"`) {
		t.Fatalf("expected quoted app-state ETag, got %q", etag)
	}
}

func TestMemoryProfilingRoutesRequireOptInAndToken(t *testing.T) {
	router := testRouter(t)

	disabled := httptest.NewRecorder()
	router.ServeHTTP(disabled, httptest.NewRequest(http.MethodGet, "/api/debug/memory", nil))
	if disabled.Code != http.StatusNotFound {
		t.Fatalf("expected disabled memory profiling status 404, got %d", disabled.Code)
	}

	cfg := testConfig(t)
	cfg.Env = "production"
	cfg.MemoryProfilingEnabled = true
	cfg.MemoryProfilingToken = "profile-secret"
	router = newTestRouter(t, cfg)

	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/debug/memory", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing profile token status 401, got %d", unauthorized.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/api/debug/memory?gc=1", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer profile-secret")
	authorized := httptest.NewRecorder()
	router.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("expected memory profile status 200, got %d: %s", authorized.Code, authorized.Body.String())
	}
	var payload memoryStatsResponse
	if err := json.Unmarshal(authorized.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode memory profile response: %v", err)
	}
	if !payload.GCTriggered {
		t.Fatal("expected gcTriggered when gc=1")
	}
	if payload.Goroutines == 0 {
		t.Fatal("expected goroutine count")
	}
	if payload.HeapProfilePath == "" {
		t.Fatal("expected heap profile path")
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
	return newTestRouter(t, testConfig(t))
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	clearProviderEnv(t)

	return config.Config{
		Env:                    "test",
		AllowedOrigins:         []string{"http://localhost:3000"},
		OneCLIBin:              filepath.Join(t.TempDir(), "missing-onecli"),
		KnowledgeRunPath:       filepath.Join(t.TempDir(), "latest-knowledge-run.json"),
		YouTubePlaylistID:      "",
		SupabaseStorageBucket:  "sources",
		OpenAITranslationModel: "gpt-4o-mini",
		OpenAISynthesisModel:   "gpt-4o-mini",
	}
}

func newTestRouter(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
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
