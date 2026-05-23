package knowledge

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

func TestFetchXBookmarksPaginatesUntilNextTokenEnds(t *testing.T) {
	t.Setenv("X_USER_ACCESS_TOKEN", "token-1")
	bookmarkRequests := []string{}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/2/users/me" {
			return jsonResponse(`{"data":{"id":"user-1","name":"Abhijit","username":"abhijit"}}`), nil
		}
		if request.URL.Path != "/2/users/user-1/bookmarks" {
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
		bookmarkRequests = append(bookmarkRequests, request.URL.RawQuery)
		switch request.URL.Query().Get("pagination_token") {
		case "":
			return jsonResponse(`{
				"data":[{"id":"tweet-1","text":"first bookmarked source","author_id":"author-1","created_at":"2026-01-01T00:00:00Z"}],
				"includes":{"users":[{"id":"author-1","name":"Ada","username":"ada"}]},
				"meta":{"result_count":1,"next_token":"next-page"}
			}`), nil
		case "next-page":
			return jsonResponse(`{
				"data":[{"id":"tweet-2","text":"second bookmarked source","author_id":"author-2","created_at":"2026-01-02T00:00:00Z"}],
				"includes":{"users":[{"id":"author-2","name":"Grace","username":"grace"}]},
				"meta":{"result_count":1}
			}`), nil
		default:
			t.Fatalf("unexpected pagination token: %s", request.URL.Query().Get("pagination_token"))
		}
		return jsonResponse(`{}`), nil
	})}
	service := NewService(config.Config{}, cacheStore{}, client)

	bookmarks, err := service.fetchXBookmarks(context.Background(), 0)
	if err != nil {
		t.Fatalf("fetch X bookmarks: %v", err)
	}
	if len(bookmarks) != 2 {
		t.Fatalf("expected 2 bookmarks, got %#v", bookmarks)
	}
	if bookmarks[0].SourceURL != "https://x.com/ada/status/tweet-1" || bookmarks[1].SourceURL != "https://x.com/grace/status/tweet-2" {
		t.Fatalf("unexpected source URLs: %#v", bookmarks)
	}
	if len(bookmarkRequests) != 2 || !strings.Contains(bookmarkRequests[0], "max_results=100") || !strings.Contains(bookmarkRequests[1], "pagination_token=next-page") {
		t.Fatalf("expected paginated bookmark requests, got %#v", bookmarkRequests)
	}
}

func TestFetchXBookmarksHonorsConfiguredLimit(t *testing.T) {
	t.Setenv("X_USER_ACCESS_TOKEN", "token-1")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/2/users/me" {
			return jsonResponse(`{"data":{"id":"user-1"}}`), nil
		}
		if request.URL.Query().Get("max_results") != "1" {
			t.Fatalf("expected max_results=1, got %q", request.URL.Query().Get("max_results"))
		}
		return jsonResponse(`{
			"data":[{"id":"tweet-1","text":"first","author_id":"author-1"},{"id":"tweet-2","text":"second","author_id":"author-1"}],
			"meta":{"result_count":2,"next_token":"unused"}
		}`), nil
	})}
	service := NewService(config.Config{}, cacheStore{}, client)

	bookmarks, err := service.fetchXBookmarks(context.Background(), 1)
	if err != nil {
		t.Fatalf("fetch X bookmarks: %v", err)
	}
	if len(bookmarks) != 1 || bookmarks[0].ID != "tweet-1" {
		t.Fatalf("expected one capped bookmark, got %#v", bookmarks)
	}
}

func TestRefreshXAccessTokenUsesRotatedRefreshToken(t *testing.T) {
	t.Setenv("X_REFRESH_TOKEN", "refresh token/with special")
	t.Setenv("X_USER_ACCESS_TOKEN", "stale-token")
	t.Setenv("SECOND_BRAIN_SKIP_KEYCHAIN", "true")
	requestBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://api.x.com/2/oauth2/token" {
			t.Fatalf("unexpected request URL: %s", request.URL.String())
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(raw)
		if request.Header.Get("Authorization") == "" {
			t.Fatal("expected confidential client authorization header")
		}
		return jsonResponse(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":7200,"scope":"tweet.read tweet.write users.read bookmark.read offline.access"}`), nil
	})}
	service := NewService(config.Config{XClientID: "client-id", XClientSecret: "client-secret"}, cacheStore{}, client)

	token, err := service.refreshXAccessToken(context.Background())
	if err != nil {
		t.Fatalf("refresh X access token: %v", err)
	}
	if token != "fresh-access" {
		t.Fatalf("expected fresh access token, got %q", token)
	}
	if got := requestBody; !strings.Contains(got, "refresh_token=refresh+token%2Fwith+special") || strings.Contains(got, "client_id=client-id") {
		t.Fatalf("expected confidential refresh form without client_id, got %s", got)
	}
	if got := getenv("X_REFRESH_TOKEN"); got != "fresh-refresh" {
		t.Fatalf("expected rotated refresh token in process env, got %q", got)
	}
}

func TestXTokenKeychainServicesUseConfiguredSuffix(t *testing.T) {
	accessService, refreshService := xTokenKeychainServices("_PROD")
	if accessService != "second-brain/X_USER_ACCESS_TOKEN_PROD" {
		t.Fatalf("unexpected access service: %s", accessService)
	}
	if refreshService != "second-brain/X_REFRESH_TOKEN_PROD" {
		t.Fatalf("unexpected refresh service: %s", refreshService)
	}
}

func TestRecordXTokenRotationWritesMetadata(t *testing.T) {
	path := t.TempDir() + "/x-token-rotation.json"
	service := NewService(config.Config{
		XTokenRotationPath:   path,
		XKeychainTokenSuffix: "_PROD",
		XExpectedUsername:    "mohantyabhijit",
		XReauthorizeCommand:  "npm run x:oauth:prod",
		OneCLIGateway:        true,
	}, cacheStore{}, http.DefaultClient)
	rotatedAt := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	if err := service.recordXTokenRotation(context.Background(), xTokenResponse{
		TokenType: "bearer",
		ExpiresIn: 7200,
		Scope:     "tweet.read offline.access",
	}, rotatedAt); err != nil {
		t.Fatalf("record rotation: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rotation metadata: %v", err)
	}
	text := string(body)
	for _, want := range []string{
		`"rotatedAt": "2026-05-23T12:00:00Z"`,
		`"accessTokenExpiresAt": "2026-05-23T14:00:00Z"`,
		`"keychainTokenSuffix": "_PROD"`,
		`"reauthorizeCommand": "npm run x:oauth:prod"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected metadata to contain %s, got %s", want, text)
		}
	}
}

func TestRefreshXAccessTokenAllowsOneCLIInjectedRefreshToken(t *testing.T) {
	t.Setenv("X_REFRESH_TOKEN", "")
	t.Setenv("X_USER_ACCESS_TOKEN", "stale-token")
	t.Setenv("SECOND_BRAIN_SKIP_KEYCHAIN", "true")
	requestBody := ""
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = string(raw)
		return jsonResponse(`{"access_token":"fresh-access","refresh_token":"fresh-refresh","expires_in":7200}`), nil
	})}
	service := NewService(config.Config{
		XClientID:     "client-id",
		OneCLIGateway: true,
		OneCLIBin:     "missing-onecli",
		OneCLIProject: "second-brain",
	}, cacheStore{}, client)

	token, err := service.refreshXAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected OneCLI secret rotation error because fake onecli is unavailable")
	}
	if token != "" {
		t.Fatalf("expected no token when OneCLI rotation fails, got %q", token)
	}
	if strings.Contains(requestBody, "refresh_token=") {
		t.Fatalf("expected refresh token to be injected by OneCLI, got body %s", requestBody)
	}
	if !strings.Contains(requestBody, "client_id=client-id") {
		t.Fatalf("expected client id in refresh form, got %s", requestBody)
	}
	if got := getenv("X_REFRESH_TOKEN"); got != "fresh-refresh" {
		t.Fatalf("expected rotated refresh token in process env before OneCLI update, got %q", got)
	}
}

func TestRefreshXAccessTokenStopsOnInvalidRefreshToken(t *testing.T) {
	t.Setenv("X_REFRESH_TOKEN", "stale-refresh")
	t.Setenv("X_USER_ACCESS_TOKEN", "stale-access")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return responseWithStatus(http.StatusBadRequest, `{"error":"invalid_request","error_description":"Value passed for the token was invalid."}`), nil
	})}
	service := NewService(config.Config{XClientID: "client-id", XClientSecret: "client-secret", OneCLIGateway: true}, cacheStore{}, client)

	token, err := service.refreshXAccessToken(context.Background())
	if err == nil {
		t.Fatal("expected invalid refresh token error")
	}
	if token != "" {
		t.Fatalf("expected no fallback token, got %q", token)
	}
	if !strings.Contains(err.Error(), "npm run x:oauth") {
		t.Fatalf("expected reauthorization guidance, got %v", err)
	}
}

func getenv(key string) string {
	return os.Getenv(key)
}

func responseWithStatus(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
