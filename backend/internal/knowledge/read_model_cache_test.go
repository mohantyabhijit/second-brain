package knowledge

import (
	"testing"
	"time"
)

func TestCompactAppStateForViewKeepsOnlyRequestedSurface(t *testing.T) {
	now := time.Date(2026, 5, 31, 5, 0, 0, 0, time.UTC)
	state := BuildAppState("owner-1", &Result{
		GeneratedAt: now,
		XBookmarks: []XBookmark{
			{ID: "x1", SourceURL: "https://x.example/1", Body: "one"},
			{ID: "x2", SourceURL: "https://x.example/2", Body: "two"},
		},
		YouTubeItems: []YouTubeItem{
			{VideoID: "y1", SourceURL: "https://youtube.example/1", TranscriptPreview: "video one"},
		},
		Summaries: []Summary{
			{ID: "s1", Source: "x", SourceURL: "https://x.example/1", Summary: "x one"},
			{ID: "s2", Source: "x", SourceURL: "https://x.example/2", Summary: "x two"},
			{ID: "s3", Source: "youtube", SourceURL: "https://youtube.example/1", Summary: "youtube one"},
		},
		Insights: []Insight{
			{ID: "i1", Title: "one"},
			{ID: "i2", Title: "two"},
		},
		ActionItems: []ActionItem{{ID: "a1"}},
		Validation:  []ValidationItem{{Label: "ok", Status: "pass"}},
		Blockers:    []string{},
		Digest: &DigestIssue{
			ID:                 "digest-1",
			DigestDate:         "2026-05-31",
			ScheduledFor:       now,
			IdempotencyKey:     "daily:2026-05-31",
			Subject:            "Digest",
			BodyMarkdown:       "# Digest",
			Status:             "sent",
			IllustrationPrompt: "prompt",
			Deliveries:         []DigestDelivery{{Provider: "test", Status: "sent"}},
			SourceRefs:         []DigestSourceRef{{Source: "x", ExternalID: "x1"}},
		},
	}, []DigestIssue{
		{
			ID:                 "digest-1",
			DigestDate:         "2026-05-31",
			ScheduledFor:       now,
			IdempotencyKey:     "daily:2026-05-31",
			Subject:            "Digest",
			BodyMarkdown:       "# Digest",
			Status:             "sent",
			IllustrationPrompt: "prompt",
			Deliveries:         []DigestDelivery{{Provider: "test", Status: "sent"}},
			SourceRefs:         []DigestSourceRef{{Source: "x", ExternalID: "x1"}},
		},
	}, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "derived")

	xState := CompactAppStateForView(&state, "original-x-posts", 1)
	if xState == nil || xState.Latest == nil {
		t.Fatal("expected compact app state")
	}
	if got := len(xState.Latest.XBookmarks); got != 1 {
		t.Fatalf("expected 1 x bookmark, got %d", got)
	}
	if got := len(xState.Latest.Summaries); got != 1 {
		t.Fatalf("expected 1 matching x summary, got %d", got)
	}
	if got := len(xState.Latest.YouTubeItems); got != 0 {
		t.Fatalf("expected no youtube payload on x view, got %d", got)
	}
	if got := len(xState.Views.OriginalXBookmarks); got != 0 {
		t.Fatalf("expected no duplicate view payload, got %d", got)
	}

	newsletterState := CompactAppStateForView(&state, "daily-newsletter", 10)
	if newsletterState == nil || newsletterState.Latest == nil || newsletterState.Latest.Digest == nil {
		t.Fatal("expected compact newsletter state")
	}
	if newsletterState.Latest.Digest.IllustrationPrompt != "" || len(newsletterState.Latest.Digest.Deliveries) != 0 || len(newsletterState.Latest.Digest.SourceRefs) != 0 {
		t.Fatalf("expected digest metadata stripped, got %#v", newsletterState.Latest.Digest)
	}
	if got := len(newsletterState.Digests); got != 1 {
		t.Fatalf("expected 1 compact digest issue, got %d", got)
	}
}
