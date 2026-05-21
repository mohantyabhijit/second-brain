package phaseone

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type Store interface {
	ReadLatest(ctx context.Context) (*Result, error)
	SaveLatest(ctx context.Context, result Result) error
}

type Service struct {
	cfg    config.Config
	store  Store
	client *http.Client
}

func NewService(cfg config.Config, store Store, client *http.Client) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	return &Service{cfg: cfg, store: store, client: client}
}

func (s *Service) ReadLatest(ctx context.Context) (*Result, error) {
	return s.store.ReadLatest(ctx)
}

func (s *Service) Run(ctx context.Context) (Result, error) {
	blockers := []string{}
	result := Result{
		GeneratedAt:  time.Now().UTC(),
		XBookmarks:   []XBookmark{},
		YouTubeItems: []YouTubeItem{},
		Summaries:    []Summary{},
		Validation:   []ValidationItem{},
		Blockers:     []string{},
	}

	result.SourceStatus.OneCLI = s.oneCLIStatus(ctx)
	result.SourceStatus.X = SourceNeedsSecrets
	result.SourceStatus.YouTube = SourceNeedsSecrets

	xBookmarks, err := s.fetchXBookmarks(ctx, 10)
	if err != nil {
		blockers = append(blockers, err.Error())
	}

	youtubeBlocked := false
	youtubeItems := []YouTubeItem{}
	if s.cfg.YouTubePlaylistID == "" {
		blockers = append(blockers, "YOUTUBE_PLAYLIST_ID is missing. Use a dedicated Second Brain Inbox playlist because Watch Later is blocked by the YouTube API.")
	} else {
		youtubeItems, err = s.fetchYouTubePhaseOneItems(ctx, s.cfg.YouTubePlaylistID, s.cfg.YouTubeTranscriptTestVideoID)
		if err != nil {
			youtubeBlocked = true
			blockers = append(blockers, err.Error())
		}
	}

	result.XBookmarks = xBookmarks
	result.YouTubeItems = youtubeItems
	result.Summaries = append(result.Summaries, summarizeBookmarks(xBookmarks)...)
	result.Summaries = append(result.Summaries, summarizeVideos(youtubeItems)...)

	switch {
	case len(xBookmarks) >= 10:
		result.SourceStatus.X = SourceReady
	case len(xBookmarks) > 0:
		result.SourceStatus.X = SourcePartial
	}

	switch {
	case youtubeBlocked:
		result.SourceStatus.YouTube = SourceBlocked
	case len(youtubeItems) > 0:
		result.SourceStatus.YouTube = SourceReady
	case s.cfg.YouTubePlaylistID != "":
		result.SourceStatus.YouTube = SourcePartial
	}

	result.Validation = []ValidationItem{
		validation("X bookmark request", len(xBookmarks) > 0, fmt.Sprintf("%d bookmark(s) fetched.", len(xBookmarks)), "No X bookmarks fetched."),
		validation("10 X bookmarks", len(xBookmarks) >= 10, "Fetched 10 X bookmarks.", fmt.Sprintf("Fetched %d X bookmark(s).", len(xBookmarks))),
		validation("X source links", allXBookmarksHaveSourceURLs(xBookmarks), "Every bookmark has a source URL.", "One or more bookmarks are missing source URLs."),
		validation("X article bodies", anyXArticleBody(xBookmarks), "At least one X Article body was extracted.", "No expanded X Article body was extracted."),
		validation("YouTube playlist check", len(youtubeItems) > 0, fmt.Sprintf("%d YouTube item(s) fetched.", len(youtubeItems)), "No YouTube playlist items fetched."),
		validation("YouTube transcript checks", allYouTubeTranscriptsTested(youtubeItems), fmt.Sprintf("Checked transcripts for %d playlist video(s).", len(youtubeItems)), "One or more playlist videos were not transcript-tested."),
		validation("Transcript path", anyYouTubeTranscriptAvailable(youtubeItems), "At least one transcript was extracted.", "No transcript extracted yet."),
		validation("Hindi transcript translation", anyHindiTranscriptTranslated(youtubeItems), "Hindi transcript translated to English locally.", "No Hindi transcript was translated to English."),
		validation("Source-grounded summaries", len(result.Summaries) > 0, fmt.Sprintf("%d summary item(s) generated.", len(result.Summaries)), "No summaries generated."),
	}
	result.Blockers = blockers

	if err := s.store.SaveLatest(ctx, result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) oneCLIStatus(ctx context.Context) SourceStatus {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := exec.LookPath(s.cfg.OneCLIBin); err != nil {
		return SourceBlocked
	}
	if err := exec.CommandContext(checkCtx, s.cfg.OneCLIBin, "auth", "status").Run(); err != nil {
		return SourceNeedsSecrets
	}
	return SourceReady
}

func validation(label string, passed bool, passDetail string, failDetail string) ValidationItem {
	if passed {
		return ValidationItem{Label: label, Status: "pass", Detail: passDetail}
	}
	return ValidationItem{Label: label, Status: "blocked", Detail: failDetail}
}

func allXBookmarksHaveSourceURLs(bookmarks []XBookmark) bool {
	if len(bookmarks) == 0 {
		return false
	}
	for _, bookmark := range bookmarks {
		if bookmark.SourceURL == "" {
			return false
		}
	}
	return true
}

func anyXArticleBody(bookmarks []XBookmark) bool {
	for _, bookmark := range bookmarks {
		if bookmark.ContentType == "article" && len(bookmark.Body) > len(bookmark.Text) {
			return true
		}
	}
	return false
}

func allYouTubeTranscriptsTested(items []YouTubeItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item.TranscriptStatus == "untested" {
			return false
		}
	}
	return true
}

func anyYouTubeTranscriptAvailable(items []YouTubeItem) bool {
	for _, item := range items {
		if item.TranscriptStatus == "available" {
			return true
		}
	}
	return false
}

func anyHindiTranscriptTranslated(items []YouTubeItem) bool {
	for _, item := range items {
		if item.TranscriptTranslationStatus == "translated" && item.TranscriptSourceLang == "hi" && item.TranscriptLang == "en" {
			return true
		}
	}
	return false
}
