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
	ReadDigests(ctx context.Context, limit int) ([]DigestIssue, error)
	ReadDigestIllustration(ctx context.Context, ownerID string, digestID string) (*DigestIllustration, error)
	SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error)
	ReadXTokens(ctx context.Context, ownerID string) (*EncryptedXTokens, error)
	SaveXTokens(ctx context.Context, tokens EncryptedXTokens) error
}

type Service struct {
	cfg          config.Config
	store        Store
	client       *http.Client
	logger       *slog.Logger
	refreshMu    sync.Mutex
	refresh      RefreshStatus
	xOAuthMu     sync.Mutex
	xOAuthStates map[string]xOAuthState
}

func NewService(cfg config.Config, store Store, client *http.Client) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	return &Service{cfg: cfg, store: store, client: client, logger: slog.Default(), xOAuthStates: map[string]xOAuthState{}}
}

func (s *Service) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) ReadLatest(ctx context.Context) (*Result, error) {
	latest, err := s.store.ReadLatest(ctx)
	if err != nil || latest == nil {
		return latest, err
	}
	normalizeResultInsightEngine(latest)
	if latest.Digest != nil {
		s.annotateDigestIllustration(latest.Digest)
	}
	return latest, nil
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
		Phase:     "starting",
		Message:   "Starting knowledge refresh.",
	}
	s.refresh = status
	s.refreshMu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout())
		defer cancel()
		_, err := s.Run(ctx)
		finishedAt := time.Now().UTC()

		s.refreshMu.Lock()
		defer s.refreshMu.Unlock()
		s.refresh.FinishedAt = &finishedAt
		if err != nil {
			s.refresh.Status = "failed"
			s.refresh.Error = err.Error()
			s.refresh.Phase = "failed"
			s.refresh.Message = "Refresh failed before the inbox could be updated."
			return
		}
		s.refresh.Status = "completed"
		s.refresh.Error = ""
		s.refresh.Phase = "completed"
		s.refresh.Message = "Refresh completed. The latest source-grounded insights are ready."
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
			Phase:     "idle",
			Message:   "No refresh is currently running.",
		}
	}
	status := s.refresh
	status.ElapsedSeconds = int64(time.Since(status.StartedAt).Seconds())
	return status
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
	s.setRefreshStage("checking_credentials", "Checking OneCLI, X, YouTube, Supabase, and model configuration.")

	result.SourceStatus.OneCLI = s.oneCLIStatus(ctx)
	result.SourceStatus.X = SourceNeedsSecrets
	result.SourceStatus.YouTube = SourceNeedsSecrets
	s.logger.Info("onecli status checked", "status", result.SourceStatus.OneCLI)
	s.setRefreshStage("fetching_sources", "Fetching X bookmarks and YouTube playlist videos.")

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
		items, err := s.fetchXBookmarks(ctx, s.cfg.XBookmarkLimit)
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

	xCandidates := candidatesFromBookmarks(xBookmarks)
	xProcessCount := len(xCandidates)
	if s.cfg.XBookmarkProcessLimit > 0 && len(xCandidates) > s.cfg.XBookmarkProcessLimit {
		xCandidates = xCandidates[:s.cfg.XBookmarkProcessLimit]
		xProcessCount = len(xCandidates)
	}
	candidates := append(xCandidates, candidatesFromVideos(youtubeItems)...)
	s.logger.Info(
		"source candidates prepared",
		"count", len(candidates),
		"x_count", len(xBookmarks),
		"x_processing_count", xProcessCount,
		"youtube_count", len(youtubeItems),
	)
	s.setRefreshStage("gleaning_insights", fmt.Sprintf("Gleaning insights from %d/%d X bookmark(s) and %d YouTube video(s).", xProcessCount, len(xBookmarks), len(youtubeItems)))
	processStart := time.Now()
	processed, synthesisBlockers := s.processSourceCandidates(ctx, candidates)
	s.logger.Info("source candidates processed", "duration_ms", time.Since(processStart).Milliseconds(), "count", len(processed), "blockers", len(synthesisBlockers))
	s.setRefreshStage("enriching_memory", "Embedding, ranking, clustering, and connecting repeated ideas across sources.")
	enrichStart := time.Now()
	processed = s.enrichProcessedSources(ctx, processed)
	s.logger.Info("source enrichment completed", "duration_ms", time.Since(enrichStart).Milliseconds(), "count", len(processed))
	blockers = append(blockers, synthesisBlockers...)
	for _, item := range processed {
		result.Summaries = append(result.Summaries, item.Synthesis.Summary)
		result.Insights = append(result.Insights, item.Synthesis.Insights...)
		result.ActionItems = append(result.ActionItems, item.Synthesis.ActionItems...)
		if item.SourceType == SourceTypeYouTube && len(item.Synthesis.Summary.ImportantTimeMarkers) > 0 {
			attachYouTubeTimeMarkers(result.YouTubeItems, item.ExternalID, item.Synthesis.Summary.ImportantTimeMarkers)
		}
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
	result.Insights = rankInsights(result.Insights, result.InsightClusters)
	result.Connections = buildSourceConnections(processed)
	if hasDigestInputs(result.Summaries, result.Insights) {
		digest, err := s.composeDigestIssue(ctx, result.GeneratedAt, result.Summaries, result.Insights, result.Themes, result.InsightClusters, result.Connections)
		if err != nil {
			return result, err
		}
		digest.OwnerID = s.cfg.OwnerID
		result.Digest = &digest
	}

	switch {
	case len(xBookmarks) > 0:
		result.SourceStatus.X = SourceReady
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
		validation(xBookmarkCoverageLabel(s.cfg.XBookmarkLimit), xBookmarkCoverageOK(len(xBookmarks), s.cfg.XBookmarkLimit), xBookmarkCoveragePassDetail(len(xBookmarks), s.cfg.XBookmarkLimit), xBookmarkCoverageBlockerDetail(len(xBookmarks), s.cfg.XBookmarkLimit)),
		validation("X source links", allXBookmarksHaveSourceURLs(xBookmarks), "Every bookmark has a source URL.", "One or more bookmarks are missing source URLs."),
		validation("X article bodies", anyXArticleBody(xBookmarks), "At least one X Article body was extracted.", "No expanded X Article body was extracted."),
		validation("YouTube playlist check", len(youtubeItems) > 0, fmt.Sprintf("%d YouTube item(s) fetched.", len(youtubeItems)), "No YouTube playlist items fetched."),
		validation("YouTube transcript checks", allYouTubeTranscriptsTested(youtubeItems), fmt.Sprintf("Checked transcripts for %d playlist video(s).", len(youtubeItems)), "One or more playlist videos were not transcript-tested."),
		validation("Transcript path", anyYouTubeTranscriptAvailable(youtubeItems), "At least one transcript was extracted.", "No transcript extracted yet."),
		validation("Hindi transcript translation", anyHindiTranscriptTranslated(youtubeItems), "Hindi transcript translated to English locally.", "No Hindi transcript was translated to English."),
		validation("Source-grounded summaries", len(result.Summaries) > 0, fmt.Sprintf("%d summary item(s) generated.", len(result.Summaries)), "No summaries generated."),
		validation("Insight extraction", len(result.Insights) > 0, fmt.Sprintf("%d insight(s) available.", len(result.Insights)), "No insights generated."),
		validation("LLM quality judge", anyJudgedContent(result.Summaries, result.Insights), "Synthesis carries LLM quality scores for summaries and insights.", "No LLM quality scores are present yet."),
		validation("YouTube time markers", anyYouTubeTimeMarkers(result.Summaries), "At least one YouTube synthesis includes important timestamp markers.", "No YouTube timestamp markers extracted yet."),
		validation("Insight grouping", len(result.InsightClusters) > 0 || len(result.Insights) > 0, fmt.Sprintf("%d insight cluster(s) available.", len(result.InsightClusters)), "No insights reached grouping."),
		validation("Action item extraction", len(result.ActionItems) > 0, fmt.Sprintf("%d action item(s) available.", len(result.ActionItems)), "No action items generated."),
		validation("Recompute control", anyCachedProcessing(result.Processing) || len(processed) > 0, "Source captures use prompt-versioned cache keys.", "No source captures reached the synthesis cache."),
	}
	result.Blockers = blockers

	saveStart := time.Now()
	s.setRefreshStage("saving_memory", "Saving source captures, vectors, feedback-ready insights, digest inputs, and graph sync events.")
	if progressStore, ok := s.store.(interface {
		SetRefreshProgressReporter(func(done int, total int))
	}); ok {
		progressStore.SetRefreshProgressReporter(func(done int, total int) {
			s.setRefreshStage("saving_memory", fmt.Sprintf("Saving memory %d/%d: source captures, vectors, feedback-ready insights, digest inputs, and graph sync events.", done, total))
		})
		defer progressStore.SetRefreshProgressReporter(nil)
	}
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

func (s *Service) setRefreshStage(phase string, message string) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if s.refresh.Status != "running" {
		return
	}
	s.refresh.Phase = phase
	s.refresh.Message = message
	s.refresh.ElapsedSeconds = int64(time.Since(s.refresh.StartedAt).Seconds())
}

func (s *Service) refreshTimeout() time.Duration {
	timeout, err := time.ParseDuration(strings.TrimSpace(s.cfg.RefreshTimeout))
	if err != nil || timeout <= 0 {
		return 90 * time.Minute
	}
	return timeout
}

func (s *Service) SaveFeedback(ctx context.Context, event FeedbackEvent) error {
	event.OwnerID = s.cfg.OwnerID
	event.Signal = strings.TrimSpace(strings.ToLower(event.Signal))
	switch event.Signal {
	case "useful", "obvious", "stale", "irrelevant", "more_like_this", "less_like_this", "archive", "expand", "copied", "tweeted", "upvote", "downvote":
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
	if !hasDigestInputs(latest.Summaries, latest.Insights) {
		return nil, fmt.Errorf("no source-grounded digest inputs are available")
	}
	digest, err := s.composeDigestIssue(ctx, time.Now().UTC(), latest.Summaries, latest.Insights, latest.Themes, latest.InsightClusters, latest.Connections)
	if err != nil {
		return nil, err
	}
	digest.OwnerID = s.cfg.OwnerID
	if err := ensureDigestID(&digest); err != nil {
		return nil, err
	}
	s.annotateDigestIllustration(&digest)
	digest.Deliveries = append(digest.Deliveries, s.deliverDigest(ctx, digest, ""))
	if len(digest.Deliveries) > 0 {
		digest.Status = digest.Deliveries[0].Status
	}
	return s.store.SaveDigest(ctx, digest)
}

func (s *Service) ReadDigests(ctx context.Context, limit int) ([]DigestIssue, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	digests, err := s.store.ReadDigests(ctx, limit)
	if err != nil {
		return nil, err
	}
	for index := range digests {
		s.annotateDigestIllustration(&digests[index])
	}
	return digests, nil
}

func (s *Service) ReadDigestIllustration(ctx context.Context, digestID string) (*DigestIllustration, error) {
	return s.store.ReadDigestIllustration(ctx, s.cfg.OwnerID, digestID)
}

func (s *Service) SendLatestDigest(ctx context.Context, recipientEmail string) (*DigestIssue, error) {
	recipient, err := normalizeDigestRecipient(recipientEmail)
	if err != nil {
		return nil, err
	}
	latest, err := s.ReadLatest(ctx)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, fmt.Errorf("no knowledge run is available for digest delivery")
	}
	var digest DigestIssue
	if latest.Digest != nil && strings.TrimSpace(latest.Digest.BodyMarkdown) != "" {
		digest = *latest.Digest
	} else {
		if !hasDigestInputs(latest.Summaries, latest.Insights) {
			return nil, fmt.Errorf("no source-grounded digest inputs are available")
		}
		digest, err = s.composeDigestIssue(ctx, time.Now().UTC(), latest.Summaries, latest.Insights, latest.Themes, latest.InsightClusters, latest.Connections)
		if err != nil {
			return nil, err
		}
	}
	digest.OwnerID = s.cfg.OwnerID
	if err := ensureDigestID(&digest); err != nil {
		return nil, err
	}
	s.annotateDigestIllustration(&digest)
	delivery := s.deliverDigest(ctx, digest, recipient)
	digest.Deliveries = []DigestDelivery{delivery}
	digest.Status = delivery.Status
	return s.store.SaveDigest(ctx, digest)
}

func (s *Service) SendProvidedDigest(ctx context.Context, recipientEmail string, digest DigestIssue) (*DigestIssue, error) {
	recipient, err := normalizeDigestRecipient(recipientEmail)
	if err != nil {
		return nil, err
	}
	digest.Subject = strings.TrimSpace(digest.Subject)
	digest.BodyMarkdown = strings.TrimSpace(digest.BodyMarkdown)
	if digest.Subject == "" || digest.BodyMarkdown == "" {
		return nil, fmt.Errorf("displayed digest is not available for delivery")
	}
	if digest.DigestDate == "" {
		digest.DigestDate = time.Now().UTC().Format("2006-01-02")
	}
	if strings.TrimSpace(digest.IdempotencyKey) == "" {
		digest.IdempotencyKey = "manual:" + digest.DigestDate + ":" + digestBodyFingerprint(digest.BodyMarkdown)
	}
	digest.OwnerID = s.cfg.OwnerID
	s.annotateDigestIllustration(&digest)
	delivery := s.deliverDigest(ctx, digest, recipient)
	digest.Deliveries = []DigestDelivery{delivery}
	digest.Status = delivery.Status
	return &digest, nil
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
	workerCount := s.cfg.ProcessWorkerCount
	if workerCount <= 0 {
		workerCount = 8
	}
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

func xBookmarkCoverageLabel(limit int) string {
	if limit > 0 {
		return fmt.Sprintf("%d X bookmarks", limit)
	}
	return "All X bookmarks"
}

func xBookmarkCoverageOK(count int, limit int) bool {
	if limit > 0 {
		return count >= limit
	}
	return count > 0
}

func xBookmarkCoveragePassDetail(count int, limit int) string {
	if limit > 0 {
		return fmt.Sprintf("Fetched %d configured X bookmark(s).", count)
	}
	return fmt.Sprintf("Fetched all available X bookmark page(s); %d bookmark(s) returned.", count)
}

func xBookmarkCoverageBlockerDetail(count int, limit int) string {
	if limit > 0 {
		return fmt.Sprintf("Fetched %d of %d configured X bookmark(s).", count, limit)
	}
	return fmt.Sprintf("Fetched %d X bookmark(s); expected at least one bookmark from the full pagination run.", count)
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

func attachYouTubeTimeMarkers(items []YouTubeItem, videoID string, markers []ImportantTimeMarker) {
	for index := range items {
		if items[index].VideoID == videoID {
			items[index].ImportantTimeMarkers = markers
			return
		}
	}
}

func anyJudgedContent(summaries []Summary, insights []Insight) bool {
	for _, summary := range summaries {
		if summary.Quality != nil && summary.Quality.Overall > 0 {
			return true
		}
	}
	for _, insight := range insights {
		if insight.Quality != nil && insight.Quality.Overall > 0 {
			return true
		}
	}
	return false
}

func anyYouTubeTimeMarkers(summaries []Summary) bool {
	for _, summary := range summaries {
		if summary.Source == string(SourceTypeYouTube) && len(summary.ImportantTimeMarkers) > 0 {
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
