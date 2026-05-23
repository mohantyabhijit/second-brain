package knowledge

import (
	"context"
	"net/http"
	"strings"
	"testing"

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
