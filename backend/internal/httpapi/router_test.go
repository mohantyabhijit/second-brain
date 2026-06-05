package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
)

func TestRouterServesHealthAndCORSPreflight(t *testing.T) {
	router := testRouter(t)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("expected health status 200, got %d", health.Code)
	}

	proxiedHealth := httptest.NewRecorder()
	router.ServeHTTP(proxiedHealth, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))
	if proxiedHealth.Code != http.StatusOK {
		t.Fatalf("expected proxied health status 200, got %d", proxiedHealth.Code)
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

	anonymousRefresh := httptest.NewRecorder()
	router.ServeHTTP(anonymousRefresh, httptest.NewRequest(http.MethodPost, "/api/knowledge-runs/refresh", nil))
	if anonymousRefresh.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous refresh status 401, got %d: %s", anonymousRefresh.Code, anonymousRefresh.Body.String())
	}

	router, authHeader := authenticatedTestRouter(t)
	refresh := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/knowledge-runs/refresh", nil)
	request.Header.Set("Authorization", authHeader)
	router.ServeHTTP(refresh, request)
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
	if !containsSubstring(result.Blockers, "public YouTube playlist") {
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

}

func TestRouterDoesNotExposeManualDigestMutations(t *testing.T) {
	router := testRouter(t)
	for _, path := range []string{"/api/digests/generate", "/api/digests/send"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("expected removed manual digest route %s to return 404, got %d", path, response.Code)
		}
	}
}

func TestRouterGeneratesLangfuseSampleDigestFromLatestSources(t *testing.T) {
	cfg := testConfig(t)
	t.Setenv("OPENAI_API_KEY", "openai-key")
	cfg.LangfusePromptManagementEnabled = false
	cfg.MemoryProfilingToken = "debug-secret"
	store := localfile.New(cfg.KnowledgeRunPath)
	generatedAt := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	result := knowledge.Result{
		GeneratedAt: generatedAt,
		Validation:  []knowledge.ValidationItem{{Label: "seed", Status: "pass", Detail: "seeded sample"}},
	}
	for i := 1; i <= 4; i++ {
		sourceID := fmt.Sprintf("source-%d", i)
		result.Summaries = append(result.Summaries, knowledge.Summary{
			ID:         sourceID,
			Source:     "x",
			Title:      fmt.Sprintf("Seed source %d", i),
			SourceURL:  "https://x.example/status/" + sourceID,
			Summary:    "A useful source-backed summary.",
			Confidence: "high",
		})
		result.Insights = append(result.Insights, knowledge.Insight{
			ID:         fmt.Sprintf("insight-%d", i),
			Source:     "x",
			SourceID:   sourceID,
			Title:      fmt.Sprintf("Seed insight %d", i),
			Insight:    "A source-backed insight for sample digest generation.",
			Evidence:   "The source supports this insight.",
			SourceURL:  "https://x.example/status/" + sourceID,
			Confidence: "high",
		})
	}
	if err := store.SaveRun(context.Background(), result, nil); err != nil {
		t.Fatalf("seed latest run: %v", err)
	}
	requests := []string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL.String())
		if request.URL.String() != "https://api.openai.com/v1/responses" {
			t.Fatalf("unexpected URL: %s", request.URL.String())
		}
		if request.Header.Get("Authorization") != "Bearer openai-key" {
			t.Fatalf("unexpected authorization header: %q", request.Header.Get("Authorization"))
		}
		return jsonResponse(`{"output_text":"{\"subject\":\"Sample digest\",\"body_markdown\":\"# Sample digest\\n\\nA generated Langfuse sample.\"}"}`), nil
	})}
	service := knowledge.NewService(cfg, store, client)
	router := NewRouter(cfg, service, logging.Discard())

	request := httptest.NewRequest(http.MethodPost, "/api/debug/langfuse/sample-digest?sources=3", nil)
	request.Header.Set("X-Second-Brain-Profile-Token", "debug-secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected sample digest status 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload knowledge.SampleDigestResult
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode sample digest response: %v", err)
	}
	if payload.SelectedSourceCount != 3 || len(payload.Sources) != 3 {
		t.Fatalf("expected 3 sampled sources, got %#v", payload.Sources)
	}
	if payload.Digest.Status != "sample" || payload.Digest.Subject != "Sample digest" {
		t.Fatalf("expected sample digest payload, got %#v", payload.Digest)
	}
	if payload.Persisted || payload.Delivered || len(payload.Digest.Deliveries) != 0 {
		t.Fatalf("sample route must not persist or deliver, got %#v", payload)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("expected no-store cache header, got %q", got)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one OpenAI synthesis request, got %#v", requests)
	}
	latest, err := store.ReadLatest(context.Background())
	if err != nil {
		t.Fatalf("read latest after sample: %v", err)
	}
	if latest == nil || latest.Digest != nil {
		t.Fatalf("sample digest must not update latest digest, got %#v", latest)
	}
}

func TestRouterRequiresDebugTokenForSampleDigestInProduction(t *testing.T) {
	cfg := testConfig(t)
	cfg.Env = "production"
	router := newTestRouter(t, cfg)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/debug/langfuse/sample-digest", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected sample digest debug token status 401, got %d: %s", response.Code, response.Body.String())
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

func TestRouterResolvesPublicAndAuthenticatedWorkspace(t *testing.T) {
	router := testRouter(t)
	publicWorkspace := httptest.NewRecorder()
	router.ServeHTTP(publicWorkspace, httptest.NewRequest(http.MethodGet, "/api/workspace", nil))
	if publicWorkspace.Code != http.StatusOK {
		t.Fatalf("expected public workspace status 200, got %d: %s", publicWorkspace.Code, publicWorkspace.Body.String())
	}
	var publicPayload knowledge.WorkspaceStatus
	if err := json.Unmarshal(publicWorkspace.Body.Bytes(), &publicPayload); err != nil {
		t.Fatalf("decode public workspace: %v", err)
	}
	if !publicPayload.Profile.IsPublicOwner || publicPayload.Profile.Handle != "abhijitmohanty" || publicPayload.Profile.Authenticated {
		t.Fatalf("expected public abhijitmohanty workspace, got %#v", publicPayload.Profile)
	}

	authenticatedRouter, authHeader := authenticatedTestRouter(t)
	authWorkspace := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	request.Header.Set("Authorization", authHeader)
	authenticatedRouter.ServeHTTP(authWorkspace, request)
	if authWorkspace.Code != http.StatusOK {
		t.Fatalf("expected authenticated workspace status 200, got %d: %s", authWorkspace.Code, authWorkspace.Body.String())
	}
	var authPayload knowledge.WorkspaceStatus
	if err := json.Unmarshal(authWorkspace.Body.Bytes(), &authPayload); err != nil {
		t.Fatalf("decode authenticated workspace: %v", err)
	}
	if authPayload.Profile.IsPublicOwner || !authPayload.Profile.Authenticated {
		t.Fatalf("expected private authenticated workspace, got %#v", authPayload.Profile)
	}
}

func TestRouterRejectsInvalidSupabaseBearer(t *testing.T) {
	router, _ := authenticatedTestRouter(t)
	request := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	request.Header.Set("Authorization", "Bearer invalid-token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid bearer status 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRouterStartsAuthenticatedXOAuthWithoutUndocumentedAuthorizeParameters(t *testing.T) {
	cfg := testConfig(t)
	cfg.XClientID = "x-client-id"
	cfg.XClientSecret = "x-client-secret"
	cfg.XRedirectURI = "https://example.com/second-brain/api/auth/x/callback"
	cfg.XOAuthScopes = []string{"tweet.read", "users.read", "bookmark.read", "offline.access"}
	cfg.XTokenEncryptionKey = "0123456789abcdef0123456789abcdef"
	router, authHeader := authenticatedTestRouterWithConfig(t, cfg)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/x/start", nil)
	request.Header.Set("Authorization", authHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected X auth start status 200, got %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode X auth start: %v", err)
	}
	if strings.Contains(payload.URL, "access_type=") {
		t.Fatalf("X auth URL must not include access_type: %s", payload.URL)
	}
	if !strings.Contains(payload.URL, "offline.access") || !strings.Contains(payload.URL, "bookmark.read") {
		t.Fatalf("X auth URL missing expected scopes: %s", payload.URL)
	}
}

func TestRouterSavesAuthenticatedYouTubePlaylistConnection(t *testing.T) {
	cfg := testConfig(t)
	cfg.OneCLIGateway = true
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "www.googleapis.com/youtube/v3/playlistItems":
			return jsonResponse(`{"items":[{"snippet":{"title":"Video","description":"","channelTitle":"Channel","publishedAt":"2026-05-24T00:00:00Z","resourceId":{"videoId":"video-1"}}}]}`), nil
		case "www.googleapis.com/youtube/v3/videos":
			return jsonResponse(`{"items":[{"id":"video-1","contentDetails":{"duration":"PT3M"}}]}`), nil
		default:
			t.Fatalf("unexpected provider request: %s", request.URL.String())
			return nil, nil
		}
	})}
	router, authHeader := authenticatedTestRouterWithConfigAndClient(t, cfg, client)
	body := strings.NewReader(`{"playlistUrl":"https://www.youtube.com/playlist?list=PL123"}`)
	request := httptest.NewRequest(http.MethodPost, "/api/source-connections/youtube", body)
	request.Header.Set("Authorization", authHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected YouTube save status 200, got %d: %s", response.Code, response.Body.String())
	}
	var connection knowledge.SourceProviderConnection
	if err := json.Unmarshal(response.Body.Bytes(), &connection); err != nil {
		t.Fatalf("decode YouTube connection: %v", err)
	}
	if connection.Provider != "youtube" || connection.ProviderAccountID != "PL123" {
		t.Fatalf("unexpected YouTube connection: %#v", connection)
	}
}

func TestEndToEndAuthenticatedOnboardingKeepsPublicWorkspaceOpen(t *testing.T) {
	cfg := testConfig(t)
	cfg.OneCLIGateway = true
	cfg.XClientID = "x-client-id"
	cfg.XClientSecret = "x-client-secret"
	cfg.XRedirectURI = "https://example.com/second-brain/api/auth/x/callback"
	cfg.XOAuthScopes = []string{"tweet.read", "users.read", "bookmark.read", "offline.access"}
	cfg.XTokenEncryptionKey = "0123456789abcdef0123456789abcdef"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host + request.URL.Path {
		case "www.googleapis.com/youtube/v3/playlistItems":
			if got := request.URL.Query().Get("playlistId"); got != "PLONBOARD" {
				t.Fatalf("unexpected playlist id %q", got)
			}
			return jsonResponse(`{"items":[{"snippet":{"title":"Onboarding video","description":"","channelTitle":"Channel","publishedAt":"2026-05-24T00:00:00Z","resourceId":{"videoId":"video-1"}}}]}`), nil
		case "www.googleapis.com/youtube/v3/videos":
			return jsonResponse(`{"items":[{"id":"video-1","contentDetails":{"duration":"PT3M"}}]}`), nil
		default:
			t.Fatalf("unexpected provider request: %s", request.URL.String())
			return nil, nil
		}
	})}
	router, authHeader := authenticatedTestRouterWithConfigAndClient(t, cfg, client)

	publicWorkspace := httptest.NewRecorder()
	router.ServeHTTP(publicWorkspace, httptest.NewRequest(http.MethodGet, "/api/workspace", nil))
	if publicWorkspace.Code != http.StatusOK {
		t.Fatalf("expected public workspace status 200, got %d: %s", publicWorkspace.Code, publicWorkspace.Body.String())
	}
	var publicStatus knowledge.WorkspaceStatus
	if err := json.Unmarshal(publicWorkspace.Body.Bytes(), &publicStatus); err != nil {
		t.Fatalf("decode public workspace: %v", err)
	}
	if !publicStatus.Profile.IsPublicOwner || publicStatus.Profile.Authenticated || publicStatus.Profile.Handle != "abhijitmohanty" {
		t.Fatalf("expected anonymous public abhijitmohanty workspace, got %#v", publicStatus.Profile)
	}

	anonymousPlaylist := httptest.NewRecorder()
	router.ServeHTTP(anonymousPlaylist, httptest.NewRequest(http.MethodPost, "/api/source-connections/youtube", strings.NewReader(`{"playlistId":"PLONBOARD"}`)))
	if anonymousPlaylist.Code != http.StatusUnauthorized {
		t.Fatalf("expected anonymous playlist save 401, got %d: %s", anonymousPlaylist.Code, anonymousPlaylist.Body.String())
	}

	authWorkspace := httptest.NewRecorder()
	authWorkspaceRequest := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	authWorkspaceRequest.Header.Set("Authorization", authHeader)
	router.ServeHTTP(authWorkspace, authWorkspaceRequest)
	if authWorkspace.Code != http.StatusOK {
		t.Fatalf("expected authenticated workspace status 200, got %d: %s", authWorkspace.Code, authWorkspace.Body.String())
	}
	var authStatus knowledge.WorkspaceStatus
	if err := json.Unmarshal(authWorkspace.Body.Bytes(), &authStatus); err != nil {
		t.Fatalf("decode authenticated workspace: %v", err)
	}
	if authStatus.Profile.IsPublicOwner || !authStatus.Profile.Authenticated {
		t.Fatalf("expected private authenticated workspace, got %#v", authStatus.Profile)
	}
	if authStatus.Onboarding.Complete || !containsSubstring(authStatus.Onboarding.Missing, "x") || !containsSubstring(authStatus.Onboarding.Missing, "youtube") {
		t.Fatalf("expected authenticated onboarding to require X and YouTube, got %#v", authStatus.Onboarding)
	}

	savePlaylist := httptest.NewRecorder()
	savePlaylistRequest := httptest.NewRequest(http.MethodPost, "/api/source-connections/youtube", strings.NewReader(`{"playlistUrl":"https://www.youtube.com/playlist?list=PLONBOARD"}`))
	savePlaylistRequest.Header.Set("Authorization", authHeader)
	router.ServeHTTP(savePlaylist, savePlaylistRequest)
	if savePlaylist.Code != http.StatusOK {
		t.Fatalf("expected playlist save status 200, got %d: %s", savePlaylist.Code, savePlaylist.Body.String())
	}

	authWorkspaceAfterPlaylist := httptest.NewRecorder()
	authWorkspaceAfterPlaylistRequest := httptest.NewRequest(http.MethodGet, "/api/workspace", nil)
	authWorkspaceAfterPlaylistRequest.Header.Set("Authorization", authHeader)
	router.ServeHTTP(authWorkspaceAfterPlaylist, authWorkspaceAfterPlaylistRequest)
	if authWorkspaceAfterPlaylist.Code != http.StatusOK {
		t.Fatalf("expected workspace after playlist status 200, got %d: %s", authWorkspaceAfterPlaylist.Code, authWorkspaceAfterPlaylist.Body.String())
	}
	if err := json.Unmarshal(authWorkspaceAfterPlaylist.Body.Bytes(), &authStatus); err != nil {
		t.Fatalf("decode workspace after playlist: %v", err)
	}
	if !authStatus.YouTube.Configured || authStatus.YouTube.PlaylistID != "PLONBOARD" {
		t.Fatalf("expected saved YouTube playlist in workspace, got %#v", authStatus.YouTube)
	}
	if authStatus.Onboarding.Complete || !containsSubstring(authStatus.Onboarding.Missing, "x") || containsSubstring(authStatus.Onboarding.Missing, "youtube") {
		t.Fatalf("expected only X to remain missing, got %#v", authStatus.Onboarding)
	}

	xStart := httptest.NewRecorder()
	xStartRequest := httptest.NewRequest(http.MethodGet, "/api/auth/x/start", nil)
	xStartRequest.Header.Set("Authorization", authHeader)
	router.ServeHTTP(xStart, xStartRequest)
	if xStart.Code != http.StatusOK {
		t.Fatalf("expected X start status 200, got %d: %s", xStart.Code, xStart.Body.String())
	}
	var xStartPayload struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(xStart.Body.Bytes(), &xStartPayload); err != nil {
		t.Fatalf("decode X start response: %v", err)
	}
	if !strings.Contains(xStartPayload.URL, "bookmark.read") || !strings.Contains(xStartPayload.URL, "offline.access") || strings.Contains(xStartPayload.URL, "access_type=") {
		t.Fatalf("unexpected X authorize URL: %s", xStartPayload.URL)
	}
}

func BenchmarkAppStateRepeatFetchTransfer(b *testing.B) {
	router := benchmarkRouter(b)
	prime := httptest.NewRecorder()
	router.ServeHTTP(prime, httptest.NewRequest(http.MethodGet, "/api/app-state?view=insights&limit=20", nil))
	if prime.Code != http.StatusOK {
		b.Fatalf("prime app-state status %d: %s", prime.Code, prime.Body.String())
	}
	etag := prime.Header().Get("ETag")
	if etag == "" {
		b.Fatal("expected app-state ETag")
	}

	b.Run("no-conditional-request", func(b *testing.B) {
		var responseBytes int64
		for i := 0; i < b.N; i++ {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/app-state?view=insights&limit=20", nil))
			if response.Code != http.StatusOK {
				b.Fatalf("expected app-state 200, got %d", response.Code)
			}
			responseBytes += int64(response.Body.Len())
		}
		b.ReportMetric(float64(responseBytes)/float64(b.N), "response_bytes/op")
	})

	b.Run("if-none-match-304", func(b *testing.B) {
		var responseBytes int64
		for i := 0; i < b.N; i++ {
			request := httptest.NewRequest(http.MethodGet, "/api/app-state?view=insights&limit=20", nil)
			request.Header.Set("If-None-Match", etag)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusNotModified {
				b.Fatalf("expected app-state 304, got %d", response.Code)
			}
			responseBytes += int64(response.Body.Len())
		}
		b.ReportMetric(float64(responseBytes)/float64(b.N), "response_bytes/op")
	})
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

func authenticatedTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	return authenticatedTestRouterWithConfig(t, testConfig(t))
}

func authenticatedTestRouterWithConfig(t *testing.T, cfg config.Config) (http.Handler, string) {
	t.Helper()
	return authenticatedTestRouterWithConfigAndClient(t, cfg, http.DefaultClient)
}

func authenticatedTestRouterWithConfigAndClient(t *testing.T, cfg config.Config, client *http.Client) (http.Handler, string) {
	t.Helper()
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/user" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("apikey") != "publishable-test-key" || r.Header.Get("Authorization") != "Bearer valid-token" {
			httputil.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]string{
			"id":    "11111111-1111-1111-1111-111111111111",
			"email": "reader@example.com",
		})
	}))
	t.Cleanup(authServer.Close)
	cfg.SupabaseURL = authServer.URL
	cfg.SupabasePublishableKey = "publishable-test-key"
	return newTestRouterWithClient(t, cfg, client), "Bearer valid-token"
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
		ObjectStorageBucket:    "sources",
		OpenAITranslationModel: "gpt-4o-mini",
		OpenAISynthesisModel:   "gpt-4o-mini",
	}
}

func newTestRouter(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	return newTestRouterWithClient(t, cfg, http.DefaultClient)
}

func newTestRouterWithClient(t *testing.T, cfg config.Config, client *http.Client) http.Handler {
	t.Helper()
	store := localfile.New(cfg.KnowledgeRunPath)
	service := knowledge.NewService(cfg, store, client)
	return NewRouter(cfg, service, logging.Discard())
}

func benchmarkRouter(b *testing.B) http.Handler {
	b.Helper()
	clearProviderEnvForBenchmark(b)
	cfg := config.Config{
		Env:                    "test",
		AllowedOrigins:         []string{"http://localhost:3000"},
		OneCLIBin:              filepath.Join(b.TempDir(), "missing-onecli"),
		KnowledgeRunPath:       filepath.Join(b.TempDir(), "latest-knowledge-run.json"),
		YouTubePlaylistID:      "",
		ObjectStorageBucket:    "sources",
		OpenAITranslationModel: "gpt-4o-mini",
		OpenAISynthesisModel:   "gpt-4o-mini",
	}
	store := localfile.New(cfg.KnowledgeRunPath)
	now := time.Date(2026, 5, 31, 6, 0, 0, 0, time.UTC)
	result := knowledge.Result{
		GeneratedAt: now,
		Summaries:   []knowledge.Summary{},
		Insights:    []knowledge.Insight{},
		Validation:  []knowledge.ValidationItem{{Label: "ok", Status: "pass", Detail: "seeded benchmark run"}},
		Blockers:    []string{},
	}
	result.SourceStatus.X = knowledge.SourceReady
	result.SourceStatus.YouTube = knowledge.SourceReady
	result.SourceStatus.OneCLI = knowledge.SourceReady
	for i := 0; i < 100; i++ {
		sourceID := fmt.Sprintf("source-%03d", i)
		result.Summaries = append(result.Summaries, knowledge.Summary{
			ID:         sourceID,
			Source:     "x",
			Title:      fmt.Sprintf("Seed summary %03d", i),
			SourceURL:  "https://x.example/" + sourceID,
			Summary:    "A benchmark summary with enough text to behave like a real app-state payload.",
			Confidence: "high",
		})
		result.Insights = append(result.Insights, knowledge.Insight{
			ID:         fmt.Sprintf("insight-%03d", i),
			Source:     "x",
			SourceID:   sourceID,
			Title:      fmt.Sprintf("Seed insight %03d", i),
			Insight:    "A benchmark insight that stands in for the rendered feed payload.",
			Evidence:   "Source-backed evidence for the benchmark insight.",
			SourceURL:  "https://x.example/" + sourceID,
			Confidence: "high",
		})
	}
	if err := store.SaveRun(context.Background(), result, nil); err != nil {
		b.Fatalf("seed benchmark store: %v", err)
	}
	service := knowledge.NewService(cfg, store, http.DefaultClient)
	return NewRouter(cfg, service, logging.Discard())
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

func clearProviderEnvForBenchmark(b *testing.B) {
	b.Helper()
	for _, key := range []string{
		"X_USER_ACCESS_TOKEN",
		"YOUTUBE_API_KEY",
		"YOUTUBE_ACCESS_TOKEN",
		"SUPADATA_API_KEY",
		"OPENAI_API_KEY",
		"ONECLI_GATEWAY",
	} {
		b.Setenv(key, "")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
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
