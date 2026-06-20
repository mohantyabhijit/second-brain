package knowledge

import (
	"strings"
	"testing"
	"time"
)

func TestCompactAppStateForViewKeepsOnlyRequestedSurface(t *testing.T) {
	now := time.Date(2026, 5, 31, 5, 0, 0, 0, time.UTC)
	state := BuildAppState("owner-1", &Result{
		GeneratedAt: now,
		XBookmarks: []XBookmark{
			{ID: "x1", SourceURL: "https://x.example/1", Body: "one", CreatedAt: "2026-05-30T05:00:00Z"},
			{ID: "x2", SourceURL: "https://x.example/2", Body: "two", CreatedAt: "2026-05-31T05:00:00Z"},
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
	if got := xState.Latest.XBookmarks[0].ID; got != "x2" {
		t.Fatalf("expected newest x bookmark first, got %q", got)
	}
	if got := xState.SourceCounts.XBookmarks; got != 2 {
		t.Fatalf("expected full x bookmark count to be preserved, got %d", got)
	}
	if got := xState.SourceCounts.YouTubeItems; got != 1 {
		t.Fatalf("expected full youtube count to be preserved, got %d", got)
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

	graphState := CompactAppStateForView(&state, "knowledge-graph", 10)
	if graphState == nil || graphState.Graph.InsightGraph == nil {
		t.Fatal("expected compact graph state with precomputed insight graph")
	}
	if got := len(graphState.Graph.InsightGraph.Nodes); got != 2 {
		t.Fatalf("expected 2 precomputed graph nodes, got %d", got)
	}
	if got := len(graphState.Graph.Themes); got != 0 {
		t.Fatalf("expected compact graph view to omit theme list duplication, got %d", got)
	}
}

func TestNormalizeAppStateViewLimitAllowsLargeGraphViews(t *testing.T) {
	if got := NormalizeAppStateViewLimit("insights", 180); got != 50 {
		t.Fatalf("expected non-graph page limit capped at 50, got %d", got)
	}
	if got := NormalizeAppStateViewLimit("original-x-posts", 180); got != 180 {
		t.Fatalf("expected source page limit to preserve 180, got %d", got)
	}
	if got := NormalizeAppStateViewLimit("original-youtube-posts", 999); got != MaxSourceStateLimit {
		t.Fatalf("expected source page limit capped at %d, got %d", MaxSourceStateLimit, got)
	}
	if got := NormalizeAppStateViewLimit("knowledge-graph", 180); got != 180 {
		t.Fatalf("expected graph page limit to preserve 180, got %d", got)
	}
	if got := NormalizeAppStateViewLimit("knowledge-graph", 999); got != MaxInsightGraphLimit {
		t.Fatalf("expected graph page limit capped at %d, got %d", MaxInsightGraphLimit, got)
	}
}

func TestBuildAppStateRunIDChangesWhenDigestChanges(t *testing.T) {
	now := time.Date(2026, 5, 31, 5, 0, 0, 0, time.UTC)
	latest := &Result{
		GeneratedAt: now,
		Insights:    []Insight{{ID: "i1", Title: "one"}},
		Validation:  []ValidationItem{},
		Blockers:    []string{},
	}
	withoutDigest := BuildAppState("owner-1", latest, nil, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "")
	if strings.Contains(withoutDigest.Manifest.RunID, "-d") {
		t.Fatalf("expected no digest suffix before a digest exists, got %q", withoutDigest.Manifest.RunID)
	}

	digest := DigestIssue{
		ID:             "digest-1",
		DigestDate:     "2026-05-31",
		ScheduledFor:   now,
		IdempotencyKey: "daily:2026-05-31",
		Subject:        "Digest one",
		BodyMarkdown:   "# Digest one",
		Status:         "generated",
	}
	latest.Digest = &digest
	first := BuildAppState("owner-1", latest, []DigestIssue{digest}, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "")
	if !strings.Contains(first.Manifest.RunID, "-d") {
		t.Fatalf("expected digest-versioned run ID, got %q", first.Manifest.RunID)
	}

	digest.Subject = "Digest two"
	digest.BodyMarkdown = "# Digest two"
	latest.Digest = &digest
	second := BuildAppState("owner-1", latest, []DigestIssue{digest}, RefreshStatus{ID: "idle", Status: "idle", StartedAt: now}, "")
	if first.Manifest.RunID == second.Manifest.RunID {
		t.Fatalf("expected digest change to create a new read-model run ID, got %q", first.Manifest.RunID)
	}
	if first.Manifest.ETag == second.Manifest.ETag {
		t.Fatal("expected digest change to create a new app-state ETag")
	}
}
