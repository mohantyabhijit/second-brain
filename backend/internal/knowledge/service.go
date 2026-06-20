package knowledge

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/abhijitmohanty/second-brain/backend/internal/config"
	"github.com/abhijitmohanty/second-brain/backend/internal/platform/logging"
)

type Store interface {
	ReadLatest(ctx context.Context) (*Result, error)
	ReadCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error)
	ReadSourceMaterialStates(ctx context.Context, ownerID string, keys []SourceMaterialKey) (map[string]SourceMaterialState, error)
	SaveRun(ctx context.Context, result Result, sources []ProcessedSource) error
	SaveFeedback(ctx context.Context, event FeedbackEvent) error
	ReadDigests(ctx context.Context, limit int) ([]DigestIssue, error)
	ReadDigestIllustration(ctx context.Context, ownerID string, digestID string) (*DigestIllustration, error)
	SaveDigest(ctx context.Context, digest DigestIssue) (*DigestIssue, error)
	ReadXTokens(ctx context.Context, ownerID string) (*EncryptedXTokens, error)
	SaveXTokens(ctx context.Context, tokens EncryptedXTokens) error
}

type digestSourceReader interface {
	ReadNewDigestSources(ctx context.Context, ownerID string, promptVersion string, model string) ([]DigestSourceRef, error)
}

type authOwnerResolver interface {
	ResolveOwnerForAuthUser(ctx context.Context, authUserID string, email string, publicOwnerID string, publicOwnerEmail string) (string, error)
}

type ownerLatestReader interface {
	ReadLatestForOwner(ctx context.Context, ownerID string) (*Result, error)
}

type readModelViewCache interface {
	ReadAppViewState(ctx context.Context, ownerID string, view string, limit int) (*AppState, error)
}

type latestViewReader interface {
	ReadLatestView(ctx context.Context, view string, limit int) (*Result, error)
}

type ownerLatestViewReader interface {
	ReadLatestViewForOwner(ctx context.Context, ownerID string, view string, limit int) (*Result, error)
}

type ownerDigestReader interface {
	ReadDigestsForOwner(ctx context.Context, ownerID string, limit int) ([]DigestIssue, error)
}

type ownerSynthesisCacheReader interface {
	ReadCachedSynthesesForOwner(ctx context.Context, ownerID string, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error)
}

type sourceProviderConnectionStore interface {
	ReadSourceProviderConnections(ctx context.Context, ownerID string) ([]SourceProviderConnection, error)
	SaveYouTubePlaylistConnection(ctx context.Context, ownerID string, playlistID string) (*SourceProviderConnection, error)
}

type youtubeTranscriptRequestStore interface {
	ClaimYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, monthlyLimit int) (bool, error)
	CompleteYouTubeTranscriptRequest(ctx context.Context, ownerID string, videoID string, status string, detail string) error
}

type readModelSnapshotStore interface {
	ReadLatestReadModelSnapshot(ctx context.Context, ownerID string) (*AppState, error)
	SaveReadModelSnapshot(ctx context.Context, ownerID string, state AppState) error
}

var ErrNoNewDigestSources = errors.New("no new source-grounded digest inputs since last digest")

type Service struct {
	cfg              config.Config
	store            Store
	cache            ReadModelCache
	client           *http.Client
	logger           *logging.Logger
	refreshMu        sync.Mutex
	refresh          RefreshStatus
	appStateViewMu   sync.Mutex
	appStateViewMemo map[string]cachedAppStateView
	xOAuthMu         sync.Mutex
	xOAuthStates     map[string]xOAuthState
}

type cachedAppStateView struct {
	state     *AppState
	expiresAt time.Time
}

type RunOutcome struct {
	Result        Result
	NewContent    bool
	SkippedReason string
}

func NewService(cfg config.Config, store Store, client *http.Client) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(cfg.OwnerID) == "" {
		cfg.OwnerID = config.DefaultOwnerID
	}
	if strings.TrimSpace(cfg.PublicOwnerID) == "" {
		cfg.PublicOwnerID = config.DefaultOwnerID
	}
	if strings.TrimSpace(cfg.PublicOwnerHandle) == "" {
		cfg.PublicOwnerHandle = config.DefaultPublicOwnerHandle
	}
	return &Service{
		cfg:              cfg,
		store:            store,
		client:           client,
		logger:           logging.Default(),
		appStateViewMemo: map[string]cachedAppStateView{},
		xOAuthStates:     map[string]xOAuthState{},
	}
}

func (s *Service) OwnerID() string {
	return s.cfg.OwnerID
}

func (s *Service) ForOwner(ownerID string) *Service {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" || ownerID == s.cfg.OwnerID {
		return s
	}
	cfg := s.cfg
	cfg.OwnerID = ownerID
	if strings.TrimSpace(cfg.PublicOwnerID) != "" && ownerID != cfg.PublicOwnerID {
		cfg.XExpectedUsername = ""
	}
	ownerService := NewService(cfg, s.store, s.client)
	ownerService.SetLogger(s.logger)
	ownerService.SetReadModelCache(s.cache)
	return ownerService
}

func (s *Service) ResolveOwnerForAuthUser(ctx context.Context, authUserID string, email string, publicOwnerID string, publicOwnerEmail string) (string, error) {
	authUserID = strings.TrimSpace(authUserID)
	if authUserID == "" {
		return "", fmt.Errorf("authenticated Supabase user id is required")
	}
	if resolver, ok := s.store.(authOwnerResolver); ok {
		return resolver.ResolveOwnerForAuthUser(ctx, authUserID, email, publicOwnerID, publicOwnerEmail)
	}
	return authUserID, nil
}

func (s *Service) WorkspaceStatus(ctx context.Context, authenticated bool) (WorkspaceStatus, error) {
	status := WorkspaceStatus{
		Profile: WorkspaceProfile{
			OwnerID:       s.cfg.OwnerID,
			Handle:        s.cfg.PublicOwnerHandle,
			DisplayName:   s.cfg.PublicOwnerHandle,
			Email:         "",
			IsPublicOwner: s.isPublicOwner(),
			Authenticated: authenticated,
		},
		X: s.XAuthStatus(ctx),
	}
	if !status.Profile.IsPublicOwner {
		status.Profile.Handle = ""
		status.Profile.DisplayName = "Your Second Brain"
	}
	playlistID := s.ownerYouTubePlaylistID(ctx)
	if playlistID != "" {
		status.YouTube.Configured = true
		status.YouTube.PlaylistID = playlistID
	}
	if connections, err := s.readSourceProviderConnections(ctx); err == nil {
		for _, connection := range connections {
			if connection.Provider != "youtube" {
				continue
			}
			status.YouTube.Configured = true
			status.YouTube.PlaylistID = connection.ProviderAccountID
			if connection.LastValidatedAt != nil {
				value := connection.LastValidatedAt.UTC()
				status.YouTube.LastValidatedAt = &value
			}
			break
		}
	} else {
		return status, err
	}
	if !status.X.Authorized {
		status.Onboarding.Missing = append(status.Onboarding.Missing, "x")
	}
	if !status.YouTube.Configured {
		status.Onboarding.Missing = append(status.Onboarding.Missing, "youtube")
	}
	status.Onboarding.Complete = len(status.Onboarding.Missing) == 0
	return status, nil
}

func (s *Service) SaveYouTubePlaylist(ctx context.Context, input YouTubePlaylistInput) (*SourceProviderConnection, error) {
	playlistID := normalizeYouTubePlaylistID(input.PlaylistID, input.PlaylistURL)
	if playlistID == "" {
		return nil, fmt.Errorf("public YouTube playlist URL or playlist ID is required")
	}
	if _, err := s.fetchPlaylistItems(ctx, playlistID, 1); err != nil {
		return nil, err
	}
	if store, ok := s.store.(sourceProviderConnectionStore); ok {
		return store.SaveYouTubePlaylistConnection(ctx, s.cfg.OwnerID, playlistID)
	}
	now := time.Now().UTC()
	return &SourceProviderConnection{
		ID:                "youtube:" + playlistID,
		Provider:          "youtube",
		ProviderAccountID: playlistID,
		TokenStatus:       "active",
		LastValidatedAt:   &now,
		UpdatedAt:         now,
	}, nil
}

func (s *Service) SetLogger(logger *logging.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) log(ctx context.Context) *logging.Logger {
	return logging.FromContext(ctx, s.logger)
}

func (s *Service) SetReadModelCache(cache ReadModelCache) {
	s.cache = cache
}

func (s *Service) ReadLatest(ctx context.Context) (*Result, error) {
	if s.cache != nil {
		latest, err := s.cache.ReadLatest(ctx, s.cfg.OwnerID)
		if err == nil {
			s.log(ctx).Info("read model cache hit", "surface", "latest")
			if latest == nil {
				return nil, nil
			}
			normalizeResultInsightEngine(latest)
			normalizeResultCollections(latest)
			if latest.Digest != nil {
				s.annotateDigestIllustration(latest.Digest)
			}
			return latest, nil
		}
		if !errors.Is(err, ErrReadModelCacheMiss) {
			s.log(ctx).Warn("read model cache fallback", "surface", "latest", "error", err)
		}
	}
	if state, ok := s.readAppStateSnapshot(ctx); ok {
		if state.Latest != nil {
			s.normalizeAppState(state)
			return state.Latest, nil
		}
		return nil, nil
	}
	return s.readLatestCanonical(ctx)
}

func (s *Service) readLatestCanonical(ctx context.Context) (*Result, error) {
	var latest *Result
	var err error
	if reader, ok := s.store.(ownerLatestReader); ok {
		latest, err = reader.ReadLatestForOwner(ctx, s.cfg.OwnerID)
	} else {
		latest, err = s.store.ReadLatest(ctx)
	}
	if err != nil || latest == nil {
		return latest, err
	}
	normalizeResultInsightEngine(latest)
	if latest.Digest != nil {
		s.annotateDigestIllustration(latest.Digest)
	}
	return latest, nil
}

func (s *Service) isPublicOwner() bool {
	publicOwnerID := strings.TrimSpace(s.cfg.PublicOwnerID)
	if publicOwnerID == "" {
		publicOwnerID = config.DefaultOwnerID
	}
	return strings.TrimSpace(s.cfg.OwnerID) == publicOwnerID
}

func (s *Service) readSourceProviderConnections(ctx context.Context) ([]SourceProviderConnection, error) {
	if store, ok := s.store.(sourceProviderConnectionStore); ok {
		return store.ReadSourceProviderConnections(ctx, s.cfg.OwnerID)
	}
	return []SourceProviderConnection{}, nil
}

func (s *Service) ownerYouTubePlaylistID(ctx context.Context) string {
	if connections, err := s.readSourceProviderConnections(ctx); err == nil {
		for _, connection := range connections {
			if connection.Provider == "youtube" && strings.TrimSpace(connection.ProviderAccountID) != "" {
				return strings.TrimSpace(connection.ProviderAccountID)
			}
		}
	} else if s.logger != nil {
		s.log(ctx).Warn("read youtube source connection failed", "owner_id", s.cfg.OwnerID, "error", err)
	}
	if s.isPublicOwner() {
		return strings.TrimSpace(s.cfg.YouTubePlaylistID)
	}
	return ""
}

func (s *Service) ReadAppState(ctx context.Context) (*AppState, string, error) {
	if s.cache != nil {
		state, err := s.cache.ReadAppState(ctx, s.cfg.OwnerID)
		if err == nil {
			s.log(ctx).Info("read model cache hit", "surface", "app-state")
			s.normalizeAppState(state)
			return state, "hit", nil
		}
		if !errors.Is(err, ErrReadModelCacheMiss) {
			s.log(ctx).Warn("read model cache fallback", "surface", "app-state", "error", err)
		}
	}
	if state, ok := s.readAppStateSnapshot(ctx); ok {
		s.normalizeAppState(state)
		return state, "snapshot", nil
	}

	latest, err := s.readLatestCanonical(ctx)
	if err != nil {
		return nil, "error", err
	}
	digests, err := s.readDigestsCanonical(ctx, 50)
	if err != nil {
		return nil, "error", err
	}
	state := BuildAppState(s.cfg.OwnerID, latest, digests, s.ReadRefreshStatus(ctx), "")
	s.normalizeAppState(&state)
	_ = s.publishAppStateBestEffort(ctx, state, "fallback_warm")
	return &state, "fallback", nil
}

func (s *Service) ReadAppStateView(ctx context.Context, view string, limit int) (*AppState, string, error) {
	view = strings.TrimSpace(view)
	if view == "" || view == "full" {
		return s.ReadAppState(ctx)
	}
	limit = NormalizeAppStateViewLimit(view, limit)
	if viewCache, ok := s.cache.(readModelViewCache); ok {
		state, err := viewCache.ReadAppViewState(ctx, s.cfg.OwnerID, view, limit)
		if err == nil {
			s.log(ctx).Info("read model cache hit", "surface", "app-state", "view", view)
			s.normalizeAppState(state)
			s.memoizeAppStateView(view, limit, state)
			return state, "hit", nil
		}
		if !errors.Is(err, ErrReadModelCacheMiss) {
			s.log(ctx).Warn("read model cache fallback", "surface", "app-state", "view", view, "error", err)
		}
	}
	if state, ok := s.readAppStateSnapshot(ctx); ok {
		compact := CompactAppStateForView(state, view, limit)
		s.normalizeAppState(compact)
		s.memoizeAppStateView(view, limit, compact)
		return compact, "snapshot", nil
	}
	if state, cacheStatus, ok := s.readMemoizedAppStateView(view, limit); ok {
		return state, cacheStatus, nil
	}
	latest, err := s.readLatestViewCanonical(ctx, view, limit)
	if err != nil {
		return nil, "error", err
	}
	digests := []DigestIssue{}
	switch view {
	case "daily-newsletter":
		digests, err = s.readDigestsCanonical(ctx, NormalizePageStateLimit(limit))
		if err != nil {
			return nil, "error", err
		}
	}
	state := BuildAppState(s.cfg.OwnerID, latest, digests, s.ReadRefreshStatus(ctx), "")
	s.normalizeAppState(&state)
	compact := CompactAppStateForView(&state, view, limit)
	s.memoizeAppStateView(view, limit, compact)
	return compact, "fallback", nil
}

func (s *Service) readAppStateSnapshot(ctx context.Context) (*AppState, bool) {
	store, ok := s.store.(readModelSnapshotStore)
	if !ok {
		return nil, false
	}
	state, err := store.ReadLatestReadModelSnapshot(ctx, s.cfg.OwnerID)
	if err == nil && state != nil {
		s.log(ctx).Info("read model snapshot hit", "surface", "app-state")
		return state, true
	}
	if err != nil && !errors.Is(err, ErrReadModelCacheMiss) {
		s.log(ctx).Warn("read model snapshot fallback", "surface", "app-state", "error", err)
	}
	return nil, false
}

func (s *Service) readMemoizedAppStateView(view string, limit int) (*AppState, string, bool) {
	s.appStateViewMu.Lock()
	defer s.appStateViewMu.Unlock()
	if s.appStateViewMemo == nil {
		s.appStateViewMemo = map[string]cachedAppStateView{}
		return nil, "", false
	}
	key := appStateViewMemoKey(view, limit)
	cached, ok := s.appStateViewMemo[key]
	if !ok || cached.state == nil || time.Now().After(cached.expiresAt) {
		delete(s.appStateViewMemo, key)
		return nil, "", false
	}
	return cached.state, "memory", true
}

func (s *Service) memoizeAppStateView(view string, limit int, state *AppState) {
	if state == nil {
		return
	}
	s.appStateViewMu.Lock()
	defer s.appStateViewMu.Unlock()
	if s.appStateViewMemo == nil {
		s.appStateViewMemo = map[string]cachedAppStateView{}
	}
	s.appStateViewMemo[appStateViewMemoKey(view, limit)] = cachedAppStateView{
		state:     state,
		expiresAt: time.Now().Add(30 * time.Second),
	}
}

func appStateViewMemoKey(view string, limit int) string {
	return strings.TrimSpace(view) + ":" + fmt.Sprint(NormalizeAppStateViewLimit(view, limit))
}

func (s *Service) readLatestViewCanonical(ctx context.Context, view string, limit int) (*Result, error) {
	if strings.TrimSpace(view) == "knowledge-graph" {
		return s.readLatestCanonical(ctx)
	}
	if reader, ok := s.store.(ownerLatestViewReader); ok {
		latest, err := reader.ReadLatestViewForOwner(ctx, s.cfg.OwnerID, view, NormalizePageStateLimit(limit))
		if err == nil {
			normalizeResultInsightEngine(latest)
			normalizeResultCollections(latest)
			return latest, nil
		}
		s.log(ctx).Warn("view-scoped latest fallback", "view", view, "error", err)
	}
	if reader, ok := s.store.(latestViewReader); ok {
		latest, err := reader.ReadLatestView(ctx, view, NormalizePageStateLimit(limit))
		if err == nil {
			normalizeResultInsightEngine(latest)
			normalizeResultCollections(latest)
			return latest, nil
		}
		s.log(ctx).Warn("view-scoped latest fallback", "view", view, "error", err)
	}
	return s.readLatestCanonical(ctx)
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
	s.writeRefreshStatusBestEffort(status)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout())
		defer cancel()
		_, err := s.Run(ctx)
		finishedAt := time.Now().UTC()

		s.refreshMu.Lock()
		s.refresh.FinishedAt = &finishedAt
		if err != nil {
			s.refresh.Status = "failed"
			s.refresh.Error = err.Error()
			s.refresh.Phase = "failed"
			s.refresh.Message = "Refresh failed before the inbox could be updated."
			status := s.refresh
			s.refreshMu.Unlock()
			s.writeRefreshStatusBestEffort(status)
			return
		}
		s.refresh.Status = "completed"
		s.refresh.Error = ""
		s.refresh.Phase = "completed"
		s.refresh.Message = "Refresh completed. The latest source-grounded insights are ready."
		status := s.refresh
		s.refreshMu.Unlock()
		s.writeRefreshStatusBestEffort(status)
	}()

	return status
}

func (s *Service) ReadRefreshStatus(ctx context.Context) RefreshStatus {
	if s.cache != nil {
		status, err := s.cache.ReadRefreshStatus(ctx, s.cfg.OwnerID)
		if err == nil && status != nil {
			return *status
		}
		if err != nil && !errors.Is(err, ErrReadModelCacheMiss) {
			s.log(ctx).Warn("read model cache fallback", "surface", "refresh-status", "error", err)
		}
	}
	return s.RefreshStatus()
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
	outcome, err := s.RunCycle(ctx)
	return outcome.Result, err
}

func (s *Service) RunCycle(ctx context.Context) (RunOutcome, error) {
	ctx, span := s.startOperationSpan(ctx, "knowledge-refresh", "refresh")
	defer span.End()
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
	s.log(ctx).Info("knowledge refresh started", "owner_id", s.cfg.OwnerID)
	s.setRefreshStage("checking_credentials", "Checking OneCLI, X, YouTube, Supabase, and model configuration.")

	result.SourceStatus.OneCLI = s.oneCLIStatus(ctx)
	result.SourceStatus.X = SourceNeedsSecrets
	result.SourceStatus.YouTube = SourceNeedsSecrets
	s.log(ctx).Info("onecli status checked", "status", result.SourceStatus.OneCLI)
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

	youtubePlaylistID := s.ownerYouTubePlaylistID(ctx)
	if youtubePlaylistID == "" {
		youtubeFetch <- youtubeFetchResult{
			err:     fmt.Errorf("public YouTube playlist is missing. Add a public playlist during onboarding because Watch Later is blocked by the YouTube API."),
			blocked: true,
		}
	} else {
		go func() {
			youtubeStart := time.Now()
			items, err := s.fetchPlaylistItems(ctx, youtubePlaylistID, 5)
			youtubeFetch <- youtubeFetchResult{items: items, err: err, blocked: err != nil, duration: time.Since(youtubeStart)}
		}()
	}

	xResult := <-xFetch
	xBookmarks := xResult.items
	if xResult.err != nil {
		s.log(ctx).Warn("x bookmark fetch blocked", "duration_ms", xResult.duration.Milliseconds(), "error", xResult.err)
		blockers = append(blockers, xResult.err.Error())
	} else {
		s.log(ctx).Info("x bookmark fetch completed", "duration_ms", xResult.duration.Milliseconds(), "count", len(xBookmarks))
	}

	youtubeResult := <-youtubeFetch
	youtubeBlocked := youtubeResult.blocked
	youtubeItems := youtubeResult.items
	if youtubeResult.err != nil {
		s.log(ctx).Warn("youtube inbox fetch blocked", "duration_ms", youtubeResult.duration.Milliseconds(), "error", youtubeResult.err)
		blockers = append(blockers, youtubeResult.err.Error())
	} else {
		s.log(ctx).Info(
			"youtube playlist fetch completed",
			"duration_ms", youtubeResult.duration.Milliseconds(),
			"count", len(youtubeItems),
		)
	}

	model := s.synthesisModel()
	sourceMaterials, materialBlockers := s.readSourceMaterialStates(ctx, sourceMaterialKeysFromFetched(xBookmarks, youtubeItems, synthesisPromptVersion, model))
	blockers = append(blockers, materialBlockers...)
	materialLookupFailed := len(materialBlockers) > 0
	if !youtubeBlocked && !materialLookupFailed {
		transcriptStart := time.Now()
		youtubeItems = s.fetchYouTubeTranscriptsForNewMaterials(ctx, youtubeItems, s.cfg.YouTubeTranscriptTestVideoID, sourceMaterials)
		s.log(ctx).Info(
			"youtube transcript fetch completed",
			"duration_ms", time.Since(transcriptStart).Milliseconds(),
			"count", len(youtubeItems),
			"transcripts_available", countAvailableTranscripts(youtubeItems),
			"transcripts_cached", countCachedTranscripts(youtubeItems),
		)
	} else if materialLookupFailed {
		s.log(ctx).Warn("youtube transcript fetch skipped because source material lookup failed")
	}

	result.XBookmarks = xBookmarks
	result.YouTubeItems = youtubeItems

	latest, latestErr := s.readLatestCanonical(ctx)
	if latestErr != nil {
		s.log(ctx).Warn("latest run lookup failed before refresh merge", "error", latestErr)
		blockers = append(blockers, "latest run lookup failed: "+latestErr.Error())
	}
	if materialLookupFailed {
		if latest != nil {
			s.setRefreshStage("completed", "Source material lookup failed; skipped refresh processing to avoid duplicate provider and model work.")
			setSpanOutputSummary(span, map[string]any{"new_content": false, "skipped_reason": "source_material_lookup_failed"})
			return RunOutcome{Result: *latest, NewContent: false, SkippedReason: "source_material_lookup_failed"}, nil
		}
		err := fmt.Errorf(strings.Join(materialBlockers, "; "))
		setSpanError(span, err)
		setSpanOutputSummary(span, map[string]any{"new_content": false, "skipped_reason": "source_material_lookup_failed"})
		return RunOutcome{Result: result, NewContent: false, SkippedReason: "source_material_lookup_failed"}, err
	}

	xCandidates := candidatesFromBookmarks(xBookmarks)
	xProcessCount := len(xCandidates)
	if s.cfg.XBookmarkProcessLimit > 0 && len(xCandidates) > s.cfg.XBookmarkProcessLimit {
		xCandidates = xCandidates[:s.cfg.XBookmarkProcessLimit]
		xProcessCount = len(xCandidates)
	}
	candidates := append(xCandidates, candidatesFromVideos(youtubeItems)...)
	newCandidates, skippedCandidates := filterNewSourceCandidates(candidates, sourceMaterials, model)
	s.log(ctx).Info(
		"source candidates prepared",
		"count", len(candidates),
		"new_count", len(newCandidates),
		"skipped_count", len(skippedCandidates),
		"x_count", len(xBookmarks),
		"x_processing_count", xProcessCount,
		"youtube_count", len(youtubeItems),
	)
	noNewContent := len(newCandidates) == 0 && len(blockers) == 0 && latest != nil && (len(candidates) > 0 || len(youtubeItems) > 0 || len(xBookmarks) > 0)
	if noNewContent {
		s.setRefreshStage("completed", "No new source materials found; skipped refresh processing.")
		s.log(ctx).Info(
			"knowledge refresh skipped; no new source materials",
			"x_count", len(xBookmarks),
			"youtube_count", len(youtubeItems),
			"cached_transcripts", countCachedTranscripts(youtubeItems),
		)
		if err := s.publishAppStateForResult(ctx, *latest, "", "refresh_noop_publish"); err != nil {
			s.log(ctx).Warn("read model cache noop publish failed", "error", err)
		}
		setSpanOutputSummary(span, map[string]any{"new_content": false, "skipped_reason": "no_new_source_materials", "x_count": len(xBookmarks), "youtube_count": len(youtubeItems)})
		return RunOutcome{Result: *latest, NewContent: false, SkippedReason: "no_new_source_materials"}, nil
	}

	s.setRefreshStage("gleaning_insights", fmt.Sprintf("Gleaning insights from %d new source material(s).", len(newCandidates)))
	processStart := time.Now()
	processed, synthesisBlockers := s.processSourceCandidates(ctx, newCandidates)
	s.log(ctx).Info("source candidates processed", "duration_ms", time.Since(processStart).Milliseconds(), "count", len(processed), "blockers", len(synthesisBlockers))
	s.setRefreshStage("enriching_memory", "Embedding, ranking, clustering, and connecting repeated ideas across sources.")
	enrichStart := time.Now()
	processed = s.enrichProcessedSources(ctx, processed)
	s.log(ctx).Info("source enrichment completed", "duration_ms", time.Since(enrichStart).Milliseconds(), "count", len(processed))
	blockers = append(blockers, synthesisBlockers...)
	excludeSourceIDs := processedSourceIDs(processed)
	saveSources := processed
	if latest != nil {
		result.XBookmarks = mergeXBookmarks(latest.XBookmarks, result.XBookmarks)
		result.YouTubeItems = mergeYouTubeItems(latest.YouTubeItems, result.YouTubeItems)
		result.Summaries = append(result.Summaries, summariesExcluding(latest.Summaries, excludeSourceIDs)...)
		result.Insights = append(result.Insights, insightsExcluding(latest.Insights, excludeSourceIDs)...)
		result.ActionItems = append(result.ActionItems, actionItemsExcluding(latest.ActionItems, excludeSourceIDs)...)
		saveSources = append(processedSourcesFromResult(latest, excludeSourceIDs), processed...)
	}
	appendProcessedOutput(&result, processed)

	graphSources := s.enrichProcessedSources(ctx, processedSourcesFromResult(&result, nil))
	result.Themes = buildThemeClusters(graphSources)
	result.InsightClusters = buildInsightClusters(graphSources)
	result.Insights = rankInsights(result.Insights, result.InsightClusters)
	result.Connections = buildSourceConnections(graphSources)

	switch {
	case len(xBookmarks) > 0:
		result.SourceStatus.X = SourceReady
	}

	switch {
	case youtubeBlocked:
		result.SourceStatus.YouTube = SourceBlocked
	case len(youtubeItems) > 0:
		result.SourceStatus.YouTube = SourceReady
	case youtubePlaylistID != "":
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
	if err := s.store.SaveRun(ctx, result, saveSources); err != nil {
		s.log(ctx).Error("knowledge refresh persist failed", "duration_ms", time.Since(saveStart).Milliseconds(), "error", err)
		setSpanError(span, err)
		setSpanOutputSummary(span, map[string]any{"new_content": len(processed) > 0, "blockers": len(result.Blockers)})
		return RunOutcome{Result: result, NewContent: len(processed) > 0}, err
	}
	if err := s.publishAppStateForResult(ctx, result, "", "refresh_publish"); err != nil {
		s.log(ctx).Warn("read model cache refresh publish failed", "error", err)
	}
	s.log(ctx).Info(
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
	setSpanOutputSummary(span, map[string]any{
		"new_content":      len(processed) > 0,
		"x_count":          len(result.XBookmarks),
		"youtube_count":    len(result.YouTubeItems),
		"summaries":        len(result.Summaries),
		"insights":         len(result.Insights),
		"actions":          len(result.ActionItems),
		"themes":           len(result.Themes),
		"insight_clusters": len(result.InsightClusters),
		"connections":      len(result.Connections),
		"blockers":         len(result.Blockers),
	})
	return RunOutcome{Result: result, NewContent: len(processed) > 0}, nil
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
	status := s.refresh
	go s.writeRefreshStatusBestEffort(status)
}

func (s *Service) refreshTimeout() time.Duration {
	timeout, err := time.ParseDuration(strings.TrimSpace(s.cfg.RefreshTimeout))
	if err != nil || timeout <= 0 {
		return 90 * time.Minute
	}
	return timeout
}

func (s *Service) publishAppStateForResult(ctx context.Context, result Result, graphStatus string, reason string) error {
	digests, err := s.readDigestsCanonical(ctx, 50)
	if err != nil {
		s.log(ctx).Warn("read digests for Redis publish failed", "error", err)
		if result.Digest != nil {
			digests = []DigestIssue{*result.Digest}
		}
	}
	state := BuildAppState(s.cfg.OwnerID, &result, digests, s.RefreshStatus(), graphStatus)
	s.normalizeAppState(&state)
	return s.publishAppStateBestEffort(ctx, state, reason)
}

func (s *Service) PublishReadModels(ctx context.Context, reason string) (*AppState, error) {
	latest, err := s.readLatestCanonical(ctx)
	if err != nil {
		return nil, err
	}
	digests, err := s.readDigestsCanonical(ctx, 50)
	if err != nil {
		return nil, err
	}
	state := BuildAppState(s.cfg.OwnerID, latest, digests, s.ReadRefreshStatus(ctx), "")
	s.normalizeAppState(&state)
	if strings.TrimSpace(reason) == "" {
		reason = "precompute_publish"
	}
	return &state, s.publishAppStateBestEffort(ctx, state, reason)
}

func (s *Service) publishAppStateBestEffort(ctx context.Context, state AppState, reason string) error {
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var snapshotErr error
	snapshotPublished := false
	if store, ok := s.store.(readModelSnapshotStore); ok {
		if err := store.SaveReadModelSnapshot(publishCtx, s.cfg.OwnerID, state); err != nil {
			snapshotErr = err
			s.log(ctx).Warn("read model snapshot publish failed", "reason", reason, "run_id", state.Manifest.RunID, "error", err)
		} else {
			snapshotPublished = true
			s.log(ctx).Info("read model snapshot publish completed", "reason", reason, "run_id", state.Manifest.RunID, "etag", state.Manifest.ETag)
		}
	}
	if s.cache == nil {
		if snapshotPublished {
			s.purgeEdgeCacheBestEffort(publishCtx, reason)
		}
		return snapshotErr
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := s.cache.PublishAppState(publishCtx, s.cfg.OwnerID, state); err != nil {
			lastErr = err
			s.log(ctx).Warn("read model cache publish failed", "reason", reason, "run_id", state.Manifest.RunID, "attempt", attempt, "error", err)
			if publishCtx.Err() != nil {
				break
			}
			time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
			continue
		}
		s.log(ctx).Info("read model cache publish completed", "reason", reason, "run_id", state.Manifest.RunID, "etag", state.Manifest.ETag)
		s.purgeEdgeCacheBestEffort(publishCtx, reason)
		return snapshotErr
	}
	if snapshotErr != nil {
		return fmt.Errorf("snapshot publish: %v; redis publish: %w", snapshotErr, lastErr)
	}
	return lastErr
}

func (s *Service) writeRefreshStatusBestEffort(status RefreshStatus) {
	if s.cache == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.cache.WriteRefreshStatus(ctx, s.cfg.OwnerID, status); err != nil {
		s.log(ctx).Warn("write refresh status to read model cache failed", "status", status.Status, "error", err)
	}
}

func (s *Service) normalizeAppState(state *AppState) {
	if state == nil {
		return
	}
	if state.Latest != nil {
		normalizeResultInsightEngine(state.Latest)
		normalizeResultCollections(state.Latest)
		if state.Latest.Digest != nil {
			s.annotateDigestIllustration(state.Latest.Digest)
		}
	}
	fallbackCounts := sourceCountsFromResult(state.Latest)
	if state.SourceCounts.XBookmarks == 0 && fallbackCounts.XBookmarks > 0 {
		state.SourceCounts.XBookmarks = fallbackCounts.XBookmarks
	}
	if state.SourceCounts.YouTubeItems == 0 && fallbackCounts.YouTubeItems > 0 {
		state.SourceCounts.YouTubeItems = fallbackCounts.YouTubeItems
	}
	if state.Latest != nil {
		state.Latest.SourceCounts = state.SourceCounts
	}
	if state.Digests == nil {
		state.Digests = []DigestIssue{}
	}
	for index := range state.Digests {
		s.annotateDigestIllustration(&state.Digests[index])
	}
	if state.Views.Insights == nil {
		state.Views.Insights = []Insight{}
	}
	if state.Views.OriginalXBookmarks == nil {
		state.Views.OriginalXBookmarks = []XBookmark{}
	}
	if state.Views.OriginalYouTubePosts == nil {
		state.Views.OriginalYouTubePosts = []YouTubeItem{}
	}
	if state.Graph.Themes == nil {
		state.Graph.Themes = []ThemeCluster{}
	}
	if state.Graph.InsightClusters == nil {
		state.Graph.InsightClusters = []InsightCluster{}
	}
	if state.Graph.Connections == nil {
		state.Graph.Connections = []SourceConnection{}
	}
	if state.Graph.InsightGraph != nil {
		if state.Graph.InsightGraph.Nodes == nil {
			state.Graph.InsightGraph.Nodes = []InsightGraphNode{}
		}
		if state.Graph.InsightGraph.Edges == nil {
			state.Graph.InsightGraph.Edges = []InsightGraphEdge{}
		}
	}
	if state.AskContext.Sources == nil {
		state.AskContext.Sources = []AskSecondBrainSource{}
	}
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
	ctx, span := s.startOperationSpan(ctx, "generate-digest", "digest")
	defer span.End()
	latest, err := s.readLatestCanonical(ctx)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	if latest == nil {
		err := fmt.Errorf("no knowledge run is available for digest generation")
		setSpanError(span, err)
		return nil, err
	}
	if isSentDigestForDate(latest.Digest, digestDateFor(s.cfg.DigestTimezone, s.cfg.DigestTime, time.Now().UTC())) {
		setSpanOutputSummary(span, map[string]any{"status": "already_sent", "digest_date": latest.Digest.DigestDate})
		return latest.Digest, nil
	}
	if !hasDigestInputs(latest.Summaries, latest.Insights) {
		err := fmt.Errorf("no source-grounded digest inputs are available")
		setSpanError(span, err)
		return nil, err
	}
	sourceRefs, summaries, insights, themes, insightClusters, connections, err := s.digestInputsForLatest(ctx, latest)
	if errors.Is(err, ErrNoNewDigestSources) {
		s.log(ctx).Info("no new digest source refs; composing continuity digest from latest run")
		sourceRefs = nil
		summaries = latest.Summaries
		insights = latest.Insights
		themes = latest.Themes
		insightClusters = latest.InsightClusters
		connections = latest.Connections
		err = nil
	}
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	if !hasDigestInputs(summaries, insights) {
		setSpanError(span, ErrNoNewDigestSources)
		return nil, ErrNoNewDigestSources
	}
	digest, err := s.composeDigestIssue(ctx, time.Now().UTC(), summaries, insights, themes, insightClusters, connections)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	digest.OwnerID = s.cfg.OwnerID
	digest.SourceRefs = sourceRefs
	if err := ensureDigestID(&digest); err != nil {
		setSpanError(span, err)
		return nil, err
	}
	s.annotateDigestIllustration(&digest)
	digest.Deliveries = append(digest.Deliveries, s.deliverDigest(ctx, digest))
	if len(digest.Deliveries) > 0 {
		digest.Status = digest.Deliveries[0].Status
	}
	saved, err := s.store.SaveDigest(ctx, digest)
	if err != nil {
		setSpanError(span, err)
		return nil, err
	}
	if saved != nil {
		latest.Digest = saved
		if err := s.publishAppStateForResult(ctx, *latest, "", "digest_publish"); err != nil {
			s.log(ctx).Warn("digest saved but read model cache publish failed", "digest_id", saved.ID, "error", err)
		}
	}
	if saved != nil {
		setSpanOutputSummary(span, map[string]any{"digest_id": saved.ID, "digest_date": saved.DigestDate, "status": saved.Status, "source_refs": len(saved.SourceRefs)})
	}
	return saved, nil
}

func digestDateFor(timezone string, digestTime string, generatedAt time.Time) string {
	return buildDigestIssue(timezone, digestTime, generatedAt, nil, nil, nil, nil, nil).DigestDate
}

func isSentDigestForDate(digest *DigestIssue, digestDate string) bool {
	if digest == nil {
		return false
	}
	return digest.DigestDate == digestDate &&
		strings.EqualFold(strings.TrimSpace(digest.Status), "sent") &&
		strings.TrimSpace(digest.BodyMarkdown) != ""
}

func (s *Service) digestInputsForLatest(ctx context.Context, latest *Result) ([]DigestSourceRef, []Summary, []Insight, []ThemeCluster, []InsightCluster, []SourceConnection, error) {
	sourceRefs, err := s.readNewDigestSourceRefs(ctx, latest)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, err
	}
	if len(sourceRefs) == 0 {
		return nil, nil, nil, nil, nil, nil, ErrNoNewDigestSources
	}
	allowed := digestSourceRefMap(sourceRefs)
	summaries := filterDigestSummaries(latest.Summaries, allowed)
	insights := filterDigestInsights(latest.Insights, allowed)
	sourceRefs = filterDigestSourceRefsForInputs(sourceRefs, summaries, insights)
	if len(sourceRefs) == 0 {
		return nil, nil, nil, nil, nil, nil, ErrNoNewDigestSources
	}
	allowed = digestSourceRefMap(sourceRefs)
	insightIDs := digestInsightIDSet(insights)
	return sourceRefs,
		summaries,
		insights,
		filterDigestThemes(latest.Themes, allowed),
		filterDigestInsightClusters(latest.InsightClusters, insightIDs),
		filterDigestConnections(latest.Connections, allowed),
		nil
}

func (s *Service) readNewDigestSourceRefs(ctx context.Context, latest *Result) ([]DigestSourceRef, error) {
	if reader, ok := s.store.(digestSourceReader); ok {
		return reader.ReadNewDigestSources(ctx, s.cfg.OwnerID, synthesisPromptVersion, s.synthesisModel())
	}
	return newDigestSourceRefsFromLatest(latest), nil
}

func newDigestSourceRefsFromLatest(latest *Result) []DigestSourceRef {
	if latest == nil {
		return nil
	}
	cutoff := time.Time{}
	if latest.Digest != nil && !latest.Digest.ScheduledFor.IsZero() {
		cutoff = latest.Digest.ScheduledFor
	}
	refsByKey := map[string]DigestSourceRef{}
	addRef := func(source string, externalID string, title string, sourceURL string, captureHash string, generatedAt *time.Time) {
		if strings.TrimSpace(source) == "" || strings.TrimSpace(externalID) == "" {
			return
		}
		seenAt := latest.GeneratedAt
		if generatedAt != nil && !generatedAt.IsZero() {
			seenAt = generatedAt.UTC()
		}
		if !cutoff.IsZero() && !seenAt.After(cutoff) {
			return
		}
		key := digestSourceKey(source, externalID)
		if _, exists := refsByKey[key]; exists {
			return
		}
		seenAtCopy := seenAt
		refsByKey[key] = DigestSourceRef{
			Source:        source,
			ExternalID:    externalID,
			SourceURL:     sourceURL,
			Title:         title,
			CaptureHash:   captureHash,
			FirstSeenAt:   &seenAtCopy,
			SynthesizedAt: &seenAtCopy,
			DigestRole:    "input",
		}
	}
	for _, summary := range latest.Summaries {
		addRef(summary.Source, summary.ID, summary.Title, summary.SourceURL, summary.CaptureHash, summary.GeneratedAt)
	}
	for _, insight := range latest.Insights {
		addRef(insight.Source, insight.SourceID, insight.Title, insight.SourceURL, "", insight.GeneratedAt)
	}
	refs := make([]DigestSourceRef, 0, len(refsByKey))
	for _, ref := range refsByKey {
		refs = append(refs, ref)
	}
	return refs
}

func digestSourceRefMap(refs []DigestSourceRef) map[string]DigestSourceRef {
	allowed := map[string]DigestSourceRef{}
	for _, ref := range refs {
		key := digestSourceKey(ref.Source, ref.ExternalID)
		if key != ":" {
			allowed[key] = ref
		}
	}
	return allowed
}

func filterDigestSummaries(summaries []Summary, allowed map[string]DigestSourceRef) []Summary {
	filtered := []Summary{}
	for _, summary := range summaries {
		if _, ok := allowed[digestSourceKey(summary.Source, summary.ID)]; ok {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

func filterDigestInsights(insights []Insight, allowed map[string]DigestSourceRef) []Insight {
	filtered := []Insight{}
	for _, insight := range insights {
		if _, ok := allowed[digestSourceKey(insight.Source, insight.SourceID)]; ok {
			filtered = append(filtered, insight)
		}
	}
	return filtered
}

func filterDigestSourceRefsForInputs(refs []DigestSourceRef, summaries []Summary, insights []Insight) []DigestSourceRef {
	used := map[string]bool{}
	for _, summary := range summaries {
		used[digestSourceKey(summary.Source, summary.ID)] = true
	}
	for _, insight := range insights {
		used[digestSourceKey(insight.Source, insight.SourceID)] = true
	}
	filtered := []DigestSourceRef{}
	for _, ref := range refs {
		if used[digestSourceKey(ref.Source, ref.ExternalID)] {
			if strings.TrimSpace(ref.DigestRole) == "" {
				ref.DigestRole = "input"
			}
			filtered = append(filtered, ref)
		}
	}
	return filtered
}

func filterDigestThemes(themes []ThemeCluster, allowed map[string]DigestSourceRef) []ThemeCluster {
	filtered := []ThemeCluster{}
	for _, theme := range themes {
		sources := []string{}
		for _, source := range theme.Sources {
			if _, ok := allowed[source]; ok {
				sources = append(sources, source)
			}
		}
		if len(sources) == 0 {
			continue
		}
		theme.Sources = sources
		filtered = append(filtered, theme)
	}
	return filtered
}

func filterDigestInsightClusters(clusters []InsightCluster, insightIDs map[string]bool) []InsightCluster {
	filtered := []InsightCluster{}
	for _, cluster := range clusters {
		ids := []string{}
		for _, id := range cluster.InsightIDs {
			if insightIDs[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			continue
		}
		representatives := []string{}
		for _, id := range cluster.RepresentativeInsightIDs {
			if insightIDs[id] {
				representatives = append(representatives, id)
			}
		}
		cluster.InsightIDs = ids
		cluster.RepresentativeInsightIDs = representatives
		filtered = append(filtered, cluster)
	}
	return filtered
}

func filterDigestConnections(connections []SourceConnection, allowed map[string]DigestSourceRef) []SourceConnection {
	filtered := []SourceConnection{}
	for _, connection := range connections {
		if _, ok := allowed[connection.LeftSourceID]; !ok {
			continue
		}
		if _, ok := allowed[connection.RightSourceID]; !ok {
			continue
		}
		filtered = append(filtered, connection)
	}
	return filtered
}

func digestInsightIDSet(insights []Insight) map[string]bool {
	ids := map[string]bool{}
	for _, insight := range insights {
		if strings.TrimSpace(insight.ID) != "" {
			ids[insight.ID] = true
		}
	}
	return ids
}

func digestSourceKey(source string, externalID string) string {
	return strings.TrimSpace(source) + ":" + strings.TrimSpace(externalID)
}

func (s *Service) ReadDigests(ctx context.Context, limit int) ([]DigestIssue, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if s.cache != nil {
		digests, err := s.cache.ReadDigests(ctx, s.cfg.OwnerID, limit)
		if err == nil {
			s.log(ctx).Info("read model cache hit", "surface", "digests")
			for index := range digests {
				s.annotateDigestIllustration(&digests[index])
			}
			return digests, nil
		}
		if !errors.Is(err, ErrReadModelCacheMiss) {
			s.log(ctx).Warn("read model cache fallback", "surface", "digests", "error", err)
		}
	}
	if state, ok := s.readAppStateSnapshot(ctx); ok {
		digests := state.Digests
		if digests == nil {
			digests = []DigestIssue{}
		}
		if limit > 0 && len(digests) > limit {
			digests = digests[:limit]
		}
		for index := range digests {
			s.annotateDigestIllustration(&digests[index])
		}
		return digests, nil
	}
	return s.readDigestsCanonical(ctx, limit)
}

func (s *Service) readDigestsCanonical(ctx context.Context, limit int) ([]DigestIssue, error) {
	var digests []DigestIssue
	var err error
	if reader, ok := s.store.(ownerDigestReader); ok {
		digests, err = reader.ReadDigestsForOwner(ctx, s.cfg.OwnerID, limit)
	} else {
		digests, err = s.store.ReadDigests(ctx, limit)
	}
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
	cached, err := s.readCachedSyntheses(ctx, keys)
	blockers := []string{}
	if err != nil {
		blockers = append(blockers, "synthesis cache lookup failed: "+err.Error())
		cached = map[string]SynthesisRecord{}
	}
	sourceCached, err := s.readCachedSyntheses(ctx, sourceKeys)
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

func (s *Service) readCachedSyntheses(ctx context.Context, keys []SynthesisCacheKey) (map[string]SynthesisRecord, error) {
	if reader, ok := s.store.(ownerSynthesisCacheReader); ok {
		return reader.ReadCachedSynthesesForOwner(ctx, s.cfg.OwnerID, keys)
	}
	return s.store.ReadCachedSyntheses(ctx, keys)
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
		if candidate.sourceType != SourceTypeYouTube {
			record, ok = sourceCached[sourceKey.String()]
		}
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
