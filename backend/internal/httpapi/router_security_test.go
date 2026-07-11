package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/httputil"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
	"github.com/abhijitmohanty/second-brain/backend/internal/store/localfile"
)

func TestSupabaseAuthResponseIsBoundedAndStrict(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "oversized", body: fmt.Sprintf(`{"id":"%s"}`, strings.Repeat("a", maxAuthResponseBytes))},
		{name: "multiple values", body: `{"id":"user-1"} {"id":"user-2"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			cfg := testConfig(t)
			cfg.SupabaseURL = server.URL
			cfg.SupabasePublishableKey = "publishable-test-key"

			_, hasBearer, err := readSupabaseAuthUser(context.Background(), cfg, "Bearer token")
			if !hasBearer || err == nil {
				t.Fatalf("expected bounded strict auth response rejection, hasBearer=%v err=%v", hasBearer, err)
			}
		})
	}
}

func TestOperatorRoutesRequireSupabaseAuthentication(t *testing.T) {
	cfg := testConfig(t)
	cfg.XClientID = "x-client-id"
	cfg.XClientSecret = "x-client-secret"
	cfg.XRedirectURI = "https://example.test/api/auth/x/callback"
	cfg.XOAuthScopes = []string{"tweet.read", "bookmark.read"}
	cfg.XTokenEncryptionKey = "0123456789abcdef0123456789abcdef"
	router := newTestRouter(t, cfg)

	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/auth/x", nil),
		httptest.NewRequest(http.MethodGet, "/api/auth/x/start", nil),
		httptest.NewRequest(http.MethodPost, "/api/knowledge-runs/refresh", nil),
		httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(`{"targetType":"insight","targetId":"1","signal":"useful"}`)),
		httptest.NewRequest(http.MethodPost, "/api/share/tweet", strings.NewReader(`{"targetType":"insight","targetId":"1","text":"hello"}`)),
		httptest.NewRequest(http.MethodPost, "/api/source-connections/youtube", strings.NewReader(`{"playlistId":"PL123"}`)),
	} {
		t.Run(request.Method+" "+request.URL.Path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestMutationJSONRejectsUnknownTrailingAndOversizedInput(t *testing.T) {
	router, authHeader := authenticatedTestRouter(t)
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"targetType":"insight","targetId":"1","signal":"useful","admin":true}`},
		{"trailing object", `{"targetType":"insight","targetId":"1","signal":"useful"}{"signal":"stale"}`},
		{"oversized body", `{"targetType":"insight","targetId":"1","signal":"useful","note":"` + strings.Repeat("x", 1_050_000) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(test.body))
			request.Header.Set("Authorization", authHeader)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestResponsesSetBaselineSecurityHeaders(t *testing.T) {
	response := httptest.NewRecorder()
	testRouter(t).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/healthz", nil))

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
	}
	for header, value := range want {
		if got := response.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestAuthenticatedGraphResponsesAreNeverPubliclyCached(t *testing.T) {
	cfg := testConfig(t)
	store := localfile.New(cfg.KnowledgeRunPath)
	seedResultWithGraph(t, store)
	router, authHeader := authenticatedRouterForStore(t, cfg, store)

	request := httptest.NewRequest(http.MethodGet, "/api/knowledge-graph/insights", nil)
	request.Header.Set("Authorization", authHeader)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected graph status 200, got %d: %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("authenticated graph Cache-Control = %q", got)
	}
}

func TestDigestIllustrationsRejectUnsafePersistedContentTypes(t *testing.T) {
	cfg := testConfig(t)
	store := &illustrationStore{
		Store: localfile.New(cfg.KnowledgeRunPath),
		illustration: &knowledge.DigestIllustration{
			ID: "digest-unsafe", MimeType: "text/html", Base64: base64.StdEncoding.EncodeToString([]byte(`<script>alert(1)</script>`)),
		},
	}
	router := NewRouter(cfg, knowledge.NewService(cfg, store, http.DefaultClient), logging.Discard())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/digests/digest-unsafe/illustration", nil))
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected unsafe image type status 415, got %d with %q", response.Code, response.Header().Get("Content-Type"))
	}
}

func TestDecodeIllustrationAcceptsExactLimitAndRejectsLargerPayloads(t *testing.T) {
	for _, test := range []struct {
		name    string
		size    int
		wantErr error
	}{
		{name: "exact limit", size: maxIllustrationBytes},
		{name: "over limit", size: maxIllustrationBytes + 1, wantErr: errIllustrationTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			encoded := base64.StdEncoding.EncodeToString(make([]byte, test.size))
			raw, err := decodeIllustration(encoded)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("decode error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && len(raw) != test.size {
				t.Fatalf("decoded size = %d, want %d", len(raw), test.size)
			}
		})
	}
}

type illustrationStore struct {
	*localfile.Store
	illustration *knowledge.DigestIllustration
}

func (s *illustrationStore) ReadDigestIllustration(context.Context, string, string) (*knowledge.DigestIllustration, error) {
	return s.illustration, nil
}

func seedResultWithGraph(t *testing.T, store *localfile.Store) {
	t.Helper()
	result := knowledge.Result{
		GeneratedAt: time.Now().UTC(),
		Insights: []knowledge.Insight{{
			ID: "insight-1", Title: "Boundary", Insight: "Protect private caches", Source: "x", SourceID: "x-1", Confidence: "high",
		}},
		Validation: []knowledge.ValidationItem{},
		Blockers:   []string{},
	}
	if err := store.SaveRun(context.Background(), result, nil); err != nil {
		t.Fatalf("seed graph result: %v", err)
	}
}

func authenticatedRouterForStore(t *testing.T, cfg config.Config, store *localfile.Store) (http.Handler, string) {
	t.Helper()
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			httputil.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		httputil.JSON(w, http.StatusOK, map[string]string{"id": "user-1", "email": "reader@example.com"})
	}))
	t.Cleanup(authServer.Close)
	cfg.SupabaseURL = authServer.URL
	cfg.SupabasePublishableKey = "publishable-test-key"
	service := knowledge.NewService(cfg, store, http.DefaultClient)
	return NewRouter(cfg, service, logging.Discard()), "Bearer valid-token"
}
