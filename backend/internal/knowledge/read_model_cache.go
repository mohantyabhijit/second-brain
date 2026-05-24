package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

func NormalizeResultForReadModel(result *Result) {
	normalizeResultInsightEngine(result)
	normalizeResultCollections(result)
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
