package knowledge

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
	ReadCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error)
	SaveRun(ctx context.Context, result Result, sources []ProcessedSource) error
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
		Insights:     []Insight{},
		ActionItems:  []ActionItem{},
		Artifacts:    []SourceArtifact{},
		Processing:   []ProcessingEvent{},
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
		youtubeItems, err = s.fetchYouTubeInboxItems(ctx, s.cfg.YouTubePlaylistID, s.cfg.YouTubeTranscriptTestVideoID)
		if err != nil {
			youtubeBlocked = true
			blockers = append(blockers, err.Error())
		}
	}

	result.XBookmarks = xBookmarks
	result.YouTubeItems = youtubeItems

	processed, synthesisBlockers := s.processSourceCandidates(ctx, append(candidatesFromBookmarks(xBookmarks), candidatesFromVideos(youtubeItems)...))
	blockers = append(blockers, synthesisBlockers...)
	for _, item := range processed {
		result.Summaries = append(result.Summaries, item.Synthesis.Summary)
		result.Insights = append(result.Insights, item.Synthesis.Insights...)
		result.ActionItems = append(result.ActionItems, item.Synthesis.ActionItems...)
		result.Artifacts = append(result.Artifacts, item.Artifact)
		status := "generated"
		detail := "Generated synthesis for current source capture."
		if item.Cached {
			status = "cached"
			detail = "Skipped synthesis because this source capture was already processed."
		}
		if item.Artifact.Error != "" {
			detail += " " + item.Artifact.Error
		}
		result.Processing = append(result.Processing, ProcessingEvent{
			Source:        string(item.SourceType),
			SourceID:      item.ExternalID,
			Title:         item.Title,
			CaptureHash:   item.CaptureHash,
			PromptVersion: item.Synthesis.PromptVersion,
			Model:         item.Synthesis.Model,
			Status:        status,
			Detail:        detail,
		})
	}

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
		validation("Insight extraction", len(result.Insights) > 0, fmt.Sprintf("%d insight(s) available.", len(result.Insights)), "No insights generated."),
		validation("Action item extraction", len(result.ActionItems) > 0, fmt.Sprintf("%d action item(s) available.", len(result.ActionItems)), "No action items generated."),
		validation("Recompute control", anyCachedProcessing(result.Processing) || len(processed) > 0, "Source captures use prompt-versioned cache keys.", "No source captures reached the synthesis cache."),
	}
	result.Blockers = blockers

	if err := s.store.SaveRun(ctx, result, processed); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) processSourceCandidates(ctx context.Context, candidates []sourceCandidate) ([]ProcessedSource, []string) {
	if len(candidates) == 0 {
		return nil, nil
	}
	model := s.synthesisModel()
	keys := make([]SynthesisCacheKey, 0, len(candidates))
	captureHashes := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		captureHash := candidate.captureHash()
		captureHashes[string(candidate.sourceType)+":"+candidate.externalID] = captureHash
		keys = append(keys, SynthesisCacheKey{
			SourceType:    candidate.sourceType,
			ExternalID:    candidate.externalID,
			CaptureHash:   captureHash,
			PromptVersion: synthesisPromptVersion,
			Model:         model,
		})
	}
	cached, err := s.store.ReadCachedSyntheses(ctx, keys)
	blockers := []string{}
	if err != nil {
		blockers = append(blockers, "synthesis cache lookup failed: "+err.Error())
		cached = map[string]SynthesisRecord{}
	}

	processed := make([]ProcessedSource, 0, len(candidates))
	for _, candidate := range candidates {
		captureHash := captureHashes[string(candidate.sourceType)+":"+candidate.externalID]
		key := SynthesisCacheKey{
			SourceType:    candidate.sourceType,
			ExternalID:    candidate.externalID,
			CaptureHash:   captureHash,
			PromptVersion: synthesisPromptVersion,
			Model:         model,
		}
		artifact := s.writeEvidenceArtifact(ctx, candidate, captureHash)
		record, ok := cached[key.String()]
		if ok {
			record.Summary.CacheStatus = "cached"
			for index := range record.Insights {
				record.Insights[index].CacheStatus = "cached"
			}
			for index := range record.ActionItems {
				record.ActionItems[index].CacheStatus = "cached"
			}
		} else {
			record = s.synthesizeCandidate(ctx, candidate, captureHash, "generated")
		}
		processed = append(processed, ProcessedSource{
			SourceType:  candidate.sourceType,
			ExternalID:  candidate.externalID,
			SourceURL:   candidate.sourceURL,
			Title:       candidate.title,
			AuthorName:  candidate.authorName,
			Username:    candidate.username,
			PublishedAt: candidate.publishedAt,
			CaptureHash: captureHash,
			Artifact:    artifact,
			Synthesis:   record,
			Cached:      ok,
		})
	}
	return processed, blockers
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

func anyCachedProcessing(items []ProcessingEvent) bool {
	for _, item := range items {
		if item.Status == "cached" {
			return true
		}
	}
	return false
}
