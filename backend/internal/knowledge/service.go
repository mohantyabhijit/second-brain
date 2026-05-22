package knowledge

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
)

type Store interface {
	ReadLatest(ctx context.Context) (*Result, error)
	ReadCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error)
	SaveRun(ctx context.Context, result Result, sources []ProcessedSource) error
	SaveFeedback(ctx context.Context, event FeedbackEvent) error
	SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error)
}

type Service struct {
	cfg       config.Config
	store     Store
	client    *http.Client
	logger    *slog.Logger
	refreshMu sync.Mutex
	refresh   RefreshStatus
}

func NewService(cfg config.Config, store Store, client *http.Client) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	return &Service{cfg: cfg, store: store, client: client, logger: slog.Default()}
}

func (s *Service) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) ReadLatest(ctx context.Context) (*Result, error) {
	return s.store.ReadLatest(ctx)
}

func (s *Service) StartRefresh() RefreshStatus {
	s.refreshMu.Lock()
	if s.refresh.Status == "running" {
		status := s.refresh
		s.refreshMu.Unlock()
		return status
	}
	status := RefreshStatus{
		ID:        fmt.Sprintf("refresh-%d", time.Now().UTC().UnixNano()),
		Status:    "running",
		StartedAt: time.Now().UTC(),
	}
	s.refresh = status
	s.refreshMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		_, err := s.Run(ctx)
		finishedAt := time.Now().UTC()

		s.refreshMu.Lock()
		defer s.refreshMu.Unlock()
		s.refresh.FinishedAt = &finishedAt
		if err != nil {
			s.refresh.Status = "failed"
			s.refresh.Error = err.Error()
			return
		}
		s.refresh.Status = "completed"
		s.refresh.Error = ""
	}()

	return status
}

func (s *Service) RefreshStatus() RefreshStatus {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refresh.ID == "" {
		return RefreshStatus{
			ID:        "idle",
			Status:    "idle",
			StartedAt: time.Now().UTC(),
		}
	}
	return s.refresh
}

func (s *Service) Run(ctx context.Context) (Result, error) {
	start := time.Now()
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
	s.logger.Info("knowledge refresh started", "owner_id", s.cfg.OwnerID)

	result.SourceStatus.OneCLI = s.oneCLIStatus(ctx)
	result.SourceStatus.X = SourceNeedsSecrets
	result.SourceStatus.YouTube = SourceNeedsSecrets
	s.logger.Info("onecli status checked", "status", result.SourceStatus.OneCLI)

	type xFetchResult struct {
		items    []XBookmark
		err      error
		duration time.Duration
	}
	type youtubeFetchResult struct {
		items    []YouTubeItem
		err      error
		blocked  bool
		duration time.Duration
	}
	xFetch := make(chan xFetchResult, 1)
	youtubeFetch := make(chan youtubeFetchResult, 1)

	go func() {
		xStart := time.Now()
		items, err := s.fetchXBookmarks(ctx, 10)
		xFetch <- xFetchResult{items: items, err: err, duration: time.Since(xStart)}
	}()

	if s.cfg.YouTubePlaylistID == "" {
		youtubeFetch <- youtubeFetchResult{
			err:     fmt.Errorf("YOUTUBE_PLAYLIST_ID is missing. Use a dedicated Second Brain Inbox playlist because Watch Later is blocked by the YouTube API."),
			blocked: true,
		}
	} else {
		go func() {
			youtubeStart := time.Now()
			items, err := s.fetchYouTubeInboxItems(ctx, s.cfg.YouTubePlaylistID, s.cfg.YouTubeTranscriptTestVideoID)
			youtubeFetch <- youtubeFetchResult{items: items, err: err, blocked: err != nil, duration: time.Since(youtubeStart)}
		}()
	}

	xResult := <-xFetch
	xBookmarks := xResult.items
	if xResult.err != nil {
		s.logger.Warn("x bookmark fetch blocked", "duration_ms", xResult.duration.Milliseconds(), "error", xResult.err)
		blockers = append(blockers, xResult.err.Error())
	} else {
		s.logger.Info("x bookmark fetch completed", "duration_ms", xResult.duration.Milliseconds(), "count", len(xBookmarks))
	}

	youtubeResult := <-youtubeFetch
	youtubeBlocked := youtubeResult.blocked
	youtubeItems := youtubeResult.items
	if youtubeResult.err != nil {
		s.logger.Warn("youtube inbox fetch blocked", "duration_ms", youtubeResult.duration.Milliseconds(), "error", youtubeResult.err)
		blockers = append(blockers, youtubeResult.err.Error())
	} else {
		s.logger.Info(
			"youtube inbox fetch completed",
			"duration_ms", youtubeResult.duration.Milliseconds(),
			"count", len(youtubeItems),
			"transcripts_available", countAvailableTranscripts(youtubeItems),
		)
	}

	result.XBookmarks = xBookmarks
	result.YouTubeItems = youtubeItems

	candidates := append(candidatesFromBookmarks(xBookmarks), candidatesFromVideos(youtubeItems)...)
	s.logger.Info("source candidates prepared", "count", len(candidates), "x_count", len(xBookmarks), "youtube_count", len(youtubeItems))
	processStart := time.Now()
	processed, synthesisBlockers := s.processSourceCandidates(ctx, candidates)
	s.logger.Info("source candidates processed", "duration_ms", time.Since(processStart).Milliseconds(), "count", len(processed), "blockers", len(synthesisBlockers))
	enrichStart := time.Now()
	processed = s.enrichProcessedSources(ctx, processed)
	s.logger.Info("source enrichment completed", "duration_ms", time.Since(enrichStart).Milliseconds(), "count", len(processed))
	blockers = append(blockers, synthesisBlockers...)
	for _, item := range processed {
		result.Summaries = append(result.Summaries, item.Synthesis.Summary)
		result.Insights = append(result.Insights, item.Synthesis.Insights...)
		result.ActionItems = append(result.ActionItems, item.Synthesis.ActionItems...)
		if item.Artifact.Path != "" {
			result.Artifacts = append(result.Artifacts, item.Artifact)
		}
		if item.SummaryArtifact.Path != "" {
			result.Artifacts = append(result.Artifacts, item.SummaryArtifact)
		}
		status := "generated"
		detail := "Generated synthesis for current source capture."
		if item.Cached {
			status = "cached"
			detail = "Skipped synthesis because this source capture was already processed."
		}
		if item.Artifact.Error != "" {
			detail += " " + item.Artifact.Error
		}
		if item.SummaryArtifact.Error != "" {
			detail += " " + item.SummaryArtifact.Error
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

	result.Themes = buildThemeClusters(processed)
	result.InsightClusters = buildInsightClusters(processed)
	result.Connections = buildSourceConnections(processed)
	digest := buildDigestIssue(s.cfg.DigestTimezone, result.GeneratedAt, result.Summaries, result.Themes, result.Connections)
	digest.OwnerID = s.cfg.OwnerID
	result.Digest = &digest

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
		validation("Insight grouping", len(result.InsightClusters) > 0 || len(result.Insights) > 0, fmt.Sprintf("%d insight cluster(s) available.", len(result.InsightClusters)), "No insights reached grouping."),
		validation("Action item extraction", len(result.ActionItems) > 0, fmt.Sprintf("%d action item(s) available.", len(result.ActionItems)), "No action items generated."),
		validation("Recompute control", anyCachedProcessing(result.Processing) || len(processed) > 0, "Source captures use prompt-versioned cache keys.", "No source captures reached the synthesis cache."),
	}
	result.Blockers = blockers

	saveStart := time.Now()
	if err := s.store.SaveRun(ctx, result, processed); err != nil {
		s.logger.Error("knowledge refresh persist failed", "duration_ms", time.Since(saveStart).Milliseconds(), "error", err)
		return result, err
	}
	s.logger.Info(
		"knowledge refresh completed",
		"duration_ms", time.Since(start).Milliseconds(),
		"x_count", len(result.XBookmarks),
		"youtube_count", len(result.YouTubeItems),
		"summaries", len(result.Summaries),
		"insights", len(result.Insights),
		"actions", len(result.ActionItems),
		"artifacts", len(result.Artifacts),
		"stored_artifacts", countStoredArtifacts(result.Artifacts),
		"themes", len(result.Themes),
		"insight_clusters", len(result.InsightClusters),
		"connections", len(result.Connections),
		"blockers", len(result.Blockers),
	)
	return result, nil
}

func (s *Service) SaveFeedback(ctx context.Context, event FeedbackEvent) error {
	event.OwnerID = s.cfg.OwnerID
	event.Signal = strings.TrimSpace(strings.ToLower(event.Signal))
	switch event.Signal {
	case "useful", "obvious", "stale", "irrelevant", "more_like_this", "less_like_this", "archive", "expand":
	default:
		return fmt.Errorf("unsupported feedback signal %q", event.Signal)
	}
	if strings.TrimSpace(event.TargetType) == "" || strings.TrimSpace(event.TargetID) == "" {
		return fmt.Errorf("targetType and targetId are required")
	}
	return s.store.SaveFeedback(ctx, event)
}

func (s *Service) GenerateDigest(ctx context.Context) (*DigestIssue, error) {
	latest, err := s.ReadLatest(ctx)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, fmt.Errorf("no knowledge run is available for digest generation")
	}
	digest := buildDigestIssue(s.cfg.DigestTimezone, time.Now().UTC(), latest.Summaries, latest.Themes, latest.Connections)
	digest.OwnerID = s.cfg.OwnerID
	digest.Deliveries = append(digest.Deliveries, s.deliverDigest(ctx, digest))
	if len(digest.Deliveries) > 0 {
		digest.Status = digest.Deliveries[0].Status
	}
	return s.store.SaveDigest(ctx, digest)
}

func (s *Service) processSourceCandidates(ctx context.Context, candidates []sourceCandidate) ([]ProcessedSource, []string) {
	if len(candidates) == 0 {
		return nil, nil
	}
	model := s.synthesisModel()
	keys := make([]SynthesisCacheKey, 0, len(candidates))
	sourceKeys := make([]SynthesisCacheKey, 0, len(candidates))
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
		sourceKeys = append(sourceKeys, SynthesisCacheKey{
			SourceType:    candidate.sourceType,
			ExternalID:    candidate.externalID,
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
	sourceCached, err := s.store.ReadCachedSyntheses(ctx, sourceKeys)
	if err != nil {
		blockers = append(blockers, "source cache lookup failed: "+err.Error())
		sourceCached = map[string]SynthesisRecord{}
	}

	processed := make([]ProcessedSource, len(candidates))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	workerCount := 4
	if len(candidates) < workerCount {
		workerCount = len(candidates)
	}
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				candidate := candidates[index]
				source := s.processSourceCandidate(ctx, candidate, captureHashes[string(candidate.sourceType)+":"+candidate.externalID], cached, sourceCached)
				mu.Lock()
				processed[index] = source
				mu.Unlock()
			}
		}()
	}
	for index := range candidates {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	return processed, blockers
}

func (s *Service) processSourceCandidate(ctx context.Context, candidate sourceCandidate, captureHash string, cached map[string]SynthesisRecord, sourceCached map[string]SynthesisRecord) ProcessedSource {
	key := SynthesisCacheKey{
		SourceType:    candidate.sourceType,
		ExternalID:    candidate.externalID,
		CaptureHash:   captureHash,
		PromptVersion: synthesisPromptVersion,
		Model:         s.synthesisModel(),
	}
	record, ok := cached[key.String()]
	if !ok {
		sourceKey := key
		sourceKey.CaptureHash = ""
		record, ok = sourceCached[sourceKey.String()]
		if ok {
			captureHash = record.CaptureHash
		}
	}
	if ok {
		record.Summary.CacheStatus = "cached"
		for index := range record.Insights {
			record.Insights[index].CacheStatus = "cached"
		}
		for index := range record.ActionItems {
			record.ActionItems[index].CacheStatus = "cached"
		}
		return ProcessedSource{
			SourceType:  candidate.sourceType,
			ContentType: candidate.itemContentType(),
			ExternalID:  candidate.externalID,
			SourceURL:   candidate.sourceURL,
			Title:       candidate.title,
			AuthorName:  candidate.authorName,
			Username:    candidate.username,
			PublishedAt: candidate.publishedAt,
			CaptureHash: captureHash,
			Synthesis:   record,
			Cached:      true,
		}
	}

	artifact := s.writeEvidenceArtifact(ctx, candidate, captureHash)
	record = s.synthesizeCandidate(ctx, candidate, captureHash, "generated")
	summaryArtifact := s.writeSynthesisArtifact(ctx, candidate, captureHash, record)
	return ProcessedSource{
		SourceType:      candidate.sourceType,
		ContentType:     candidate.itemContentType(),
		ExternalID:      candidate.externalID,
		SourceURL:       candidate.sourceURL,
		Title:           candidate.title,
		AuthorName:      candidate.authorName,
		Username:        candidate.username,
		PublishedAt:     candidate.publishedAt,
		CaptureHash:     captureHash,
		Artifact:        artifact,
		SummaryArtifact: summaryArtifact,
		Synthesis:       record,
		Cached:          false,
	}
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

func countAvailableTranscripts(items []YouTubeItem) int {
	count := 0
	for _, item := range items {
		if item.TranscriptStatus == "available" {
			count++
		}
	}
	return count
}

func countStoredArtifacts(items []SourceArtifact) int {
	count := 0
	for _, item := range items {
		if item.Stored {
			count++
		}
	}
	return count
}
