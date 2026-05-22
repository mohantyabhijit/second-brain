package knowledge

import "testing"

func TestCandidatesFromBookmarksNormalizeSourceMetadata(t *testing.T) {
	bookmarks := []XBookmark{
		{
			ID:          "article-1",
			ContentType: "article",
			Text:        "post text",
			Title:       "Deep research note",
			Body:        "long article body",
			AuthorName:  "Ada Lovelace",
			Username:    "ada",
			CreatedAt:   "2026-01-01T00:00:00Z",
			SourceURL:   "https://x.com/ada/article/article-1",
		},
		{
			ID:          "tweet-1",
			ContentType: "tweet",
			Text:        "agent workflow tweet",
			AuthorName:  "Grace Hopper",
			Username:    "grace",
			SourceURL:   "https://x.com/grace/status/tweet-1",
		},
		{
			ID:          "tweet-2",
			ContentType: "tweet",
			Text:        "fallback author tweet",
			AuthorName:  "Katherine Johnson",
			SourceURL:   "https://x.com/i/web/status/tweet-2",
		},
	}

	candidates := candidatesFromBookmarks(bookmarks)

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
	if candidates[0].title != "Deep research note" || candidates[0].artifactKind != "article" || candidates[0].body != "long article body" {
		t.Fatalf("unexpected article candidate: %#v", candidates[0])
	}
	if got := candidates[0].storagePath(); got != "x/article-1/article.txt" {
		t.Fatalf("unexpected article storage path: %q", got)
	}
	if candidates[1].title != "@grace" || candidates[1].artifactKind != "tweet" || candidates[1].body != "agent workflow tweet" {
		t.Fatalf("unexpected tweet candidate: %#v", candidates[1])
	}
	if candidates[2].title != "Katherine Johnson" {
		t.Fatalf("expected author-name title fallback, got %q", candidates[2].title)
	}

	firstHash := candidates[0].captureHash()
	secondHash := candidates[0].captureHash()
	if firstHash == "" || firstHash != secondHash {
		t.Fatalf("expected stable non-empty capture hash, got %q and %q", firstHash, secondHash)
	}
	changed := candidates[0]
	changed.body = "changed body"
	if firstHash == changed.captureHash() {
		t.Fatal("expected capture hash to change when source body changes")
	}
}

func TestCandidatesFromVideosRequiresAvailableTranscriptText(t *testing.T) {
	items := []YouTubeItem{
		{
			VideoID:           "video-1",
			Title:             "",
			ChannelTitle:      "Research Channel",
			PublishedAt:       "2026-01-02T00:00:00Z",
			SourceURL:         "https://www.youtube.com/watch?v=video-1",
			TranscriptStatus:  "available",
			TranscriptPreview: "translated transcript text",
		},
		{
			VideoID:                   "video-2",
			Title:                     "Original only",
			SourceURL:                 "https://www.youtube.com/watch?v=video-2",
			TranscriptStatus:          "available",
			TranscriptOriginalPreview: "original transcript text",
		},
		{
			VideoID:           "video-3",
			Title:             "Missing transcript",
			SourceURL:         "https://www.youtube.com/watch?v=video-3",
			TranscriptStatus:  "missing",
			TranscriptPreview: "ignored text",
		},
		{
			VideoID:          "video-4",
			Title:            "No text",
			SourceURL:        "https://www.youtube.com/watch?v=video-4",
			TranscriptStatus: "available",
		},
	}

	candidates := candidatesFromVideos(items)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 transcript candidates, got %d", len(candidates))
	}
	if candidates[0].title != "Untitled YouTube video" || candidates[0].body != "translated transcript text" {
		t.Fatalf("unexpected first video candidate: %#v", candidates[0])
	}
	if got := candidates[0].storagePath(); got != "youtube/video-1/transcript.txt" {
		t.Fatalf("unexpected first video storage path: %q", got)
	}
	if candidates[1].title != "Original only" || candidates[1].body != "original transcript text" {
		t.Fatalf("unexpected second video candidate: %#v", candidates[1])
	}
}
