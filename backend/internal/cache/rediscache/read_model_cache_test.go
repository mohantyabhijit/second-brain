package rediscache

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/knowledge"
	goredis "github.com/redis/go-redis/v9"
)

func TestFirstNHandlesNilLimitsAndExistingSlices(t *testing.T) {
	tests := []struct {
		name  string
		items []int
		limit int
		want  []int
	}{
		{"nil becomes non-nil empty", nil, 3, []int{}},
		{"under limit is unchanged", []int{1, 2}, 3, []int{1, 2}},
		{"over limit is truncated", []int{1, 2, 3}, 2, []int{1, 2}},
		{"zero limit returns empty", []int{1, 2}, 0, []int{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := firstN(test.items, test.limit); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("firstN(%v, %d) = %v, want %v", test.items, test.limit, got, test.want)
			}
		})
	}
}

func TestCompactDigestIssuesRemovesPrivateAndHeavyFields(t *testing.T) {
	issues := []knowledge.DigestIssue{{
		ID:                 "digest-1",
		Subject:            "Keep me",
		Deliveries:         []knowledge.DigestDelivery{{Provider: "resend", Recipient: "private@example.test"}},
		SourceRefs:         []knowledge.DigestSourceRef{{ExternalID: "source-1"}},
		IllustrationPrompt: "private prompt",
		IllustrationModel:  "model",
	}}
	got := compactDigestIssues(issues)
	if len(got) != 1 || got[0].Subject != "Keep me" {
		t.Fatalf("unexpected compact digest: %#v", got)
	}
	if got[0].Deliveries != nil || got[0].SourceRefs != nil || got[0].IllustrationPrompt != "" || got[0].IllustrationModel != "" {
		t.Fatalf("private fields were retained: %#v", got[0])
	}
	if result := compactDigestIssues(nil); result == nil || len(result) != 0 {
		t.Fatalf("nil digest input should become an empty JSON-safe slice: %#v", result)
	}
}

func TestCacheKeyNamespaceAndOwnerFallback(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"manifest", manifestKey("owner"), "sb:v1:owner:manifest"},
		{"app state", appStateKey("owner", "run"), "sb:v1:owner:app-state:run"},
		{"latest run", latestRunKey("owner", "run"), "sb:v1:owner:run:run:latest"},
		{"run metadata", runMetaKey("owner", "run"), "sb:v1:owner:run:run:meta"},
		{"view", viewKey("owner", "run", "insights"), "sb:v1:owner:view:run:insights"},
		{"digests", digestsKey("owner", "run"), "sb:v1:owner:digests:run:list"},
		{"refresh", refreshStatusKey("owner"), "sb:v1:owner:refresh:status"},
		{"materials", sourceMaterialsKey("owner"), "sb:v1:owner:source-materials"},
		{"graph", graphKey("owner", "run"), "sb:v1:owner:graph:run:read-model"},
		{"ask", askContextKey("owner", "run"), "sb:v1:owner:ask:context:run"},
		{"blank owner", manifestKey("  "), "sb:v1:default:manifest"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("key = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestRedisHelpersFailClosedAndUseFallbacks(t *testing.T) {
	if !errors.Is(redisError(goredis.Nil), knowledge.ErrReadModelCacheMiss) {
		t.Fatal("redis nil should map to cache miss")
	}
	sentinel := errors.New("redis unavailable")
	if !errors.Is(redisError(sentinel), sentinel) {
		t.Fatal("non-nil Redis errors should be preserved")
	}
	if got := parseDuration("15m", time.Hour); got != 15*time.Minute {
		t.Fatalf("parsed duration = %v", got)
	}
	for _, value := range []string{"", "invalid", "0s", "-1s"} {
		if got := parseDuration(value, time.Hour); got != time.Hour {
			t.Fatalf("parseDuration(%q) = %v, want fallback", value, got)
		}
	}
	if fallback("  ", "default") != "default" || fallback("value", "default") != "value" {
		t.Fatal("fallback should distinguish blank and populated strings")
	}
}

func TestNilCacheCloseIsSafe(t *testing.T) {
	var cache *Cache
	if err := cache.Close(); err != nil {
		t.Fatalf("nil Close returned %v", err)
	}
}
