package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const AppStateSchemaVersion = "redis-read-model-v1"

var ErrReadModelCacheMiss = errors.New("read model cache miss")

type ReadModelCache interface {
	ReadAppState(ctx context.Context, ownerID string) (*AppState, error)
	ReadLatest(ctx context.Context, ownerID string) (*Result, error)
	ReadDigests(ctx context.Context, ownerID string, limit int) ([]DigestIssue, error)
	ReadSourceMaterialStates(ctx context.Context, ownerID string, keys []SourceMaterialKey) (map[string]SourceMaterialState, error)
	ReadRefreshStatus(ctx context.Context, ownerID string) (*RefreshStatus, error)
	WriteRefreshStatus(ctx context.Context, ownerID string, status RefreshStatus) error
	PublishSourceMaterialStates(ctx context.Context, ownerID string, states []SourceMaterialState) error
	PublishAppState(ctx context.Context, ownerID string, state AppState) error
	Close() error
}

func BuildAppState(ownerID string, latest *Result, digests []DigestIssue, refresh RefreshStatus, graphStatus string) AppState {
	now := time.Now().UTC()
	runID := "none"
	generatedAt := now
	var latestCopy *Result
	if latest != nil {
		copied := *latest
		NormalizeResultForReadModel(&copied)
		latestCopy = &copied
		generatedAt = copied.GeneratedAt.UTC()
		if !generatedAt.IsZero() {
			runID = generatedAt.Format("20060102T150405.000000000Z")
		}
	}

	digests = normalizeDigests(digests)
	runID = appStateRunID(runID, latestCopy, digests)
	digestStatus := digestStatusFor(latestCopy, digests)
	if graphStatus == "" {
		graphStatus = graphStatusFor(latestCopy)
	}
	if refresh.ID == "" {
		refresh = idleRefreshStatus()
	}

	state := AppState{
		Manifest: AppStateManifest{
			SchemaVersion: AppStateSchemaVersion,
			RunID:         runID,
			GeneratedAt:   generatedAt,
			PublishedAt:   now,
			GraphStatus:   graphStatus,
			DigestStatus:  digestStatus,
		},
		Latest:        latestCopy,
		Digests:       digests,
		RefreshStatus: refresh,
	}
	state.Views = buildAppStateViews(latestCopy)
	state.Graph = buildAppStateGraph(latestCopy, graphStatus)
	state.AskContext = buildAppStateAskContext(runID, latestCopy, now)
	state.Manifest.ETag = appStateETag(ownerID, state)
	return state
}

func CompactAppStateForView(state *AppState, view string, limit int) *AppState {
	if state == nil {
		return nil
	}
	normalizedView := strings.TrimSpace(view)
	if normalizedView == "" || normalizedView == "full" {
		return state
	}
	limit = NormalizePageStateLimit(limit)
	compact := *state
	compact.Views = AppStateViews{
		Insights:             []Insight{},
		OriginalXBookmarks:   []XBookmark{},
		OriginalYouTubePosts: []YouTubeItem{},
	}
	compact.Graph = AppStateGraph{
		Status:          state.Graph.Status,
		Themes:          []ThemeCluster{},
		InsightClusters: []InsightCluster{},
		Connections:     []SourceConnection{},
	}
	compact.AskContext.Sources = nil
	compact.Digests = nil
	if state.Latest == nil {
		return &compact
	}

	latest := resultShell(state.Latest)
	switch normalizedView {
	case "insights":
		latest.Insights = firstN(state.Latest.Insights, limit)
	case "daily-newsletter":
		latest.Digest = compactDigestIssue(state.Latest.Digest)
		latest.Summaries = firstN(state.Latest.Summaries, 8)
		if len(state.Digests) > 0 {
			compact.Digests = compactDigestIssues(firstN(state.Digests, limit))
		} else if latest.Digest != nil {
			compact.Digests = []DigestIssue{*latest.Digest}
		} else {
			compact.Digests = []DigestIssue{}
		}
	case "original-x-posts", "original-x-bookmarks":
		latest.XBookmarks = firstN(state.Latest.XBookmarks, limit)
		latest.Summaries = summariesForSourceURLs(state.Latest.Summaries, "x", xBookmarkURLs(latest.XBookmarks))
	case "original-youtube-posts", "original-youtube-videos":
		latest.YouTubeItems = firstN(state.Latest.YouTubeItems, limit)
		latest.Summaries = summariesForSourceURLs(state.Latest.Summaries, "youtube", youtubeItemURLs(latest.YouTubeItems))
	case "knowledge-graph":
		compact.Graph = AppStateGraph{
			Status:       state.Graph.Status,
			InsightGraph: LimitInsightGraphResponsePointer(state.Graph.InsightGraph, MaxInsightGraphLimit),
		}
	default:
		return state
	}
	compact.Latest = &latest
	if compact.Digests == nil {
		compact.Digests = []DigestIssue{}
	}
	return &compact
}

func NormalizePageStateLimit(limit int) int {
	if limit <= 0 {
		return 25
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func NormalizeResultForReadModel(result *Result) {
	normalizeResultInsightEngine(result)
	normalizeResultCollections(result)
}

func resultShell(latest *Result) Result {
	result := Result{
		GeneratedAt:     latest.GeneratedAt,
		XBookmarks:      []XBookmark{},
		YouTubeItems:    []YouTubeItem{},
		Summaries:       []Summary{},
		Insights:        []Insight{},
		ActionItems:     []ActionItem{},
		Processing:      []ProcessingEvent{},
		Themes:          []ThemeCluster{},
		InsightClusters: []InsightCluster{},
		Connections:     []SourceConnection{},
		Validation:      latest.Validation,
		Blockers:        latest.Blockers,
	}
	result.SourceStatus = latest.SourceStatus
	if result.Validation == nil {
		result.Validation = []ValidationItem{}
	}
	if result.Blockers == nil {
		result.Blockers = []string{}
	}
	return result
}

func compactDigestIssue(digest *DigestIssue) *DigestIssue {
	if digest == nil {
		return nil
	}
	compact := *digest
	compact.Deliveries = nil
	compact.SourceRefs = nil
	compact.IllustrationPrompt = ""
	compact.IllustrationModel = ""
	return &compact
}

func compactDigestIssues(digests []DigestIssue) []DigestIssue {
	if digests == nil {
		return []DigestIssue{}
	}
	compact := make([]DigestIssue, 0, len(digests))
	for index := range digests {
		if digest := compactDigestIssue(&digests[index]); digest != nil {
			compact = append(compact, *digest)
		}
	}
	return compact
}

func firstN[T any](items []T, limit int) []T {
	if items == nil {
		return []T{}
	}
	if limit >= 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func summariesForSourceURLs(summaries []Summary, source string, urls map[string]struct{}) []Summary {
	if len(summaries) == 0 || len(urls) == 0 {
		return []Summary{}
	}
	filtered := []Summary{}
	for _, summary := range summaries {
		if summary.Source != source {
			continue
		}
		if _, ok := urls[summary.SourceURL]; ok {
			filtered = append(filtered, summary)
		}
	}
	return filtered
}

func xBookmarkURLs(bookmarks []XBookmark) map[string]struct{} {
	urls := map[string]struct{}{}
	for _, bookmark := range bookmarks {
		if strings.TrimSpace(bookmark.SourceURL) != "" {
			urls[bookmark.SourceURL] = struct{}{}
		}
	}
	return urls
}

func youtubeItemURLs(items []YouTubeItem) map[string]struct{} {
	urls := map[string]struct{}{}
	for _, item := range items {
		if strings.TrimSpace(item.SourceURL) != "" {
			urls[item.SourceURL] = struct{}{}
		}
	}
	return urls
}

func normalizeResultCollections(result *Result) {
	if result == nil {
		return
	}
	if result.XBookmarks == nil {
		result.XBookmarks = []XBookmark{}
	}
	if result.YouTubeItems == nil {
		result.YouTubeItems = []YouTubeItem{}
	}
	if result.Summaries == nil {
		result.Summaries = []Summary{}
	}
	if result.Insights == nil {
		result.Insights = []Insight{}
	}
	if result.ActionItems == nil {
		result.ActionItems = []ActionItem{}
	}
	if result.Processing == nil {
		result.Processing = []ProcessingEvent{}
	}
	if result.Themes == nil {
		result.Themes = []ThemeCluster{}
	}
	if result.InsightClusters == nil {
		result.InsightClusters = []InsightCluster{}
	}
	if result.Connections == nil {
		result.Connections = []SourceConnection{}
	}
	if result.Validation == nil {
		result.Validation = []ValidationItem{}
	}
	if result.Blockers == nil {
		result.Blockers = []string{}
	}
}

func idleRefreshStatus() RefreshStatus {
	return RefreshStatus{
		ID:        "idle",
		Status:    "idle",
		StartedAt: time.Now().UTC(),
		Phase:     "idle",
		Message:   "No refresh is currently running.",
	}
}

func normalizeDigests(digests []DigestIssue) []DigestIssue {
	if digests == nil {
		return []DigestIssue{}
	}
	return digests
}

func buildAppStateViews(latest *Result) AppStateViews {
	if latest == nil {
		return AppStateViews{
			Insights:             []Insight{},
			OriginalXBookmarks:   []XBookmark{},
			OriginalYouTubePosts: []YouTubeItem{},
		}
	}
	return AppStateViews{
		Insights:             latest.Insights,
		DailyNewsletter:      latest.Digest,
		OriginalXBookmarks:   latest.XBookmarks,
		OriginalYouTubePosts: latest.YouTubeItems,
	}
}

func buildAppStateGraph(latest *Result, status string) AppStateGraph {
	graph := AppStateGraph{
		Status:          status,
		Themes:          []ThemeCluster{},
		InsightClusters: []InsightCluster{},
		Connections:     []SourceConnection{},
	}
	if latest == nil {
		return graph
	}
	graph.Themes = latest.Themes
	graph.InsightClusters = latest.InsightClusters
	graph.Connections = latest.Connections
	insightGraph := buildInsightGraphFromResult(latest, MaxInsightGraphLimit)
	graph.InsightGraph = &insightGraph
	return graph
}

func buildAppStateAskContext(runID string, latest *Result, updatedAt time.Time) AppStateAskContext {
	context := AppStateAskContext{
		RunID:     runID,
		Sources:   []AskSecondBrainSource{},
		UpdatedAt: updatedAt,
	}
	if latest == nil {
		return context
	}
	sources := []AskSecondBrainSource{}
	for _, insight := range latest.Insights {
		sources = append(sources, AskSecondBrainSource{
			ID:        fmt.Sprintf("A%d", len(sources)+1),
			Title:     fallback(insight.Title, "Insight"),
			Source:    insight.Source,
			SourceURL: insight.SourceURL,
			Excerpt:   truncateDigestText(insight.Insight+" Evidence: "+insight.Evidence, 760),
			Score:     insight.ImportanceScore + insight.NoveltyScore + insight.ActionabilityScore,
		})
		if len(sources) >= 12 {
			context.Sources = sources
			return context
		}
	}
	for _, summary := range latest.Summaries {
		sources = append(sources, AskSecondBrainSource{
			ID:        fmt.Sprintf("A%d", len(sources)+1),
			Title:     fallback(summary.Title, "Summary"),
			Source:    summary.Source,
			SourceURL: summary.SourceURL,
			Excerpt:   truncateDigestText(summary.Summary+" Evidence: "+summary.Quote, 700),
		})
		if len(sources) >= 12 {
			break
		}
	}
	context.Sources = sources
	return context
}

func digestStatusFor(latest *Result, digests []DigestIssue) string {
	if latest != nil && latest.Digest != nil && latest.Digest.Status != "" {
		return latest.Digest.Status
	}
	if len(digests) > 0 && digests[0].Status != "" {
		return digests[0].Status
	}
	return "none"
}

func appStateRunID(baseRunID string, latest *Result, digests []DigestIssue) string {
	if !hasDigestVersionData(latest, digests) {
		return baseRunID
	}
	payload := struct {
		LatestDigest *digestVersionRecord  `json:"latestDigest,omitempty"`
		Digests      []digestVersionRecord `json:"digests"`
	}{
		Digests: make([]digestVersionRecord, 0, len(digests)),
	}
	if latest != nil && latest.Digest != nil {
		record := digestVersion(*latest.Digest)
		payload.LatestDigest = &record
	}
	for _, digest := range digests {
		payload.Digests = append(payload.Digests, digestVersion(digest))
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return baseRunID
	}
	sum := sha256.Sum256(raw)
	return baseRunID + "-d" + hex.EncodeToString(sum[:])[:12]
}

func hasDigestVersionData(latest *Result, digests []DigestIssue) bool {
	return (latest != nil && latest.Digest != nil) || len(digests) > 0
}

type digestVersionRecord struct {
	ID                   string    `json:"id,omitempty"`
	DigestDate           string    `json:"digestDate"`
	ScheduledFor         time.Time `json:"scheduledFor"`
	IdempotencyKey       string    `json:"idempotencyKey"`
	Subject              string    `json:"subject"`
	BodyMarkdown         string    `json:"bodyMarkdown"`
	Status               string    `json:"status"`
	IllustrationAlt      string    `json:"illustrationAlt,omitempty"`
	IllustrationMimeType string    `json:"illustrationMimeType,omitempty"`
	IllustrationModel    string    `json:"illustrationModel,omitempty"`
	IllustrationReady    bool      `json:"illustrationReady"`
}

func digestVersion(digest DigestIssue) digestVersionRecord {
	return digestVersionRecord{
		ID:                   digest.ID,
		DigestDate:           digest.DigestDate,
		ScheduledFor:         digest.ScheduledFor.UTC(),
		IdempotencyKey:       digest.IdempotencyKey,
		Subject:              digest.Subject,
		BodyMarkdown:         digest.BodyMarkdown,
		Status:               digest.Status,
		IllustrationAlt:      digest.IllustrationAlt,
		IllustrationMimeType: digest.IllustrationMimeType,
		IllustrationModel:    digest.IllustrationModel,
		IllustrationReady:    digest.IllustrationAvailable || strings.TrimSpace(digest.IllustrationBase64) != "",
	}
}

func graphStatusFor(latest *Result) string {
	if latest == nil {
		return "none"
	}
	if len(latest.Themes) > 0 || len(latest.InsightClusters) > 0 || len(latest.Connections) > 0 {
		return "derived"
	}
	return "skipped"
}

func appStateETag(ownerID string, state AppState) string {
	payload := struct {
		OwnerID       string             `json:"ownerId"`
		SchemaVersion string             `json:"schemaVersion"`
		RunID         string             `json:"runId"`
		Latest        *Result            `json:"latest"`
		Digests       []DigestIssue      `json:"digests"`
		Graph         AppStateGraph      `json:"graph"`
		AskContext    AppStateAskContext `json:"askContext"`
	}{
		OwnerID:       ownerID,
		SchemaVersion: state.Manifest.SchemaVersion,
		RunID:         state.Manifest.RunID,
		Latest:        state.Latest,
		Digests:       state.Digests,
		Graph:         state.Graph,
		AskContext:    state.AskContext,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
