package knowledge

import "time"

type SourceType string

const (
	SourceTypeX       SourceType = "x"
	SourceTypeYouTube SourceType = "youtube"
)

type SourceStatus string

const (
	SourceReady        SourceStatus = "ready"
	SourceBlocked      SourceStatus = "blocked"
	SourceNeedsSecrets SourceStatus = "needs_secrets"
	SourcePartial      SourceStatus = "partial"
)

type Decision string

const (
	DecisionReadNow Decision = "read_now"
	DecisionLater   Decision = "later"
	DecisionSkip    Decision = "skip"
)

type ValidationItem struct {
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type XBookmark struct {
	ID            string         `json:"id"`
	ContentType   string         `json:"contentType"`
	Text          string         `json:"text"`
	Title         string         `json:"title,omitempty"`
	Body          string         `json:"body"`
	PreviewText   string         `json:"previewText,omitempty"`
	ExpandedURL   string         `json:"expandedUrl,omitempty"`
	AuthorID      string         `json:"authorId,omitempty"`
	AuthorName    string         `json:"authorName,omitempty"`
	Username      string         `json:"username,omitempty"`
	CreatedAt     string         `json:"createdAt,omitempty"`
	PublicMetrics map[string]int `json:"publicMetrics,omitempty"`
	SourceURL     string         `json:"sourceUrl"`
}

type YouTubeItem struct {
	VideoID                     string                `json:"videoId"`
	Title                       string                `json:"title"`
	Description                 string                `json:"description,omitempty"`
	ChannelTitle                string                `json:"channelTitle,omitempty"`
	PublishedAt                 string                `json:"publishedAt,omitempty"`
	DurationSeconds             int                   `json:"durationSeconds,omitempty"`
	SourceURL                   string                `json:"sourceUrl"`
	TranscriptStatus            string                `json:"transcriptStatus"`
	TranscriptLang              string                `json:"transcriptLang,omitempty"`
	TranscriptSourceLang        string                `json:"transcriptSourceLang,omitempty"`
	TranscriptAvailableLangs    []string              `json:"transcriptAvailableLangs,omitempty"`
	TranscriptTranslationStatus string                `json:"transcriptTranslationStatus,omitempty"`
	TranscriptPreview           string                `json:"transcriptPreview,omitempty"`
	TranscriptOriginalPreview   string                `json:"transcriptOriginalPreview,omitempty"`
	TranscriptError             string                `json:"transcriptError,omitempty"`
	ImportantTimeMarkers        []ImportantTimeMarker `json:"importantTimeMarkers,omitempty"`
	TranscriptText              string                `json:"-"`
	TranscriptOriginalText      string                `json:"-"`
	TranscriptTimedText         string                `json:"-"`
}

type Summary struct {
	ID                   string                `json:"id"`
	Source               string                `json:"source"`
	Title                string                `json:"title"`
	SourceURL            string                `json:"sourceUrl"`
	Decision             Decision              `json:"decision"`
	Summary              string                `json:"summary"`
	Quote                string                `json:"quote,omitempty"`
	Confidence           string                `json:"confidence"`
	Notes                []string              `json:"notes"`
	Quality              *QualityScore         `json:"quality,omitempty"`
	ImportantTimeMarkers []ImportantTimeMarker `json:"importantTimeMarkers,omitempty"`
	CacheStatus          string                `json:"cacheStatus,omitempty"`
	CaptureHash          string                `json:"captureHash,omitempty"`
	PromptVersion        string                `json:"promptVersion,omitempty"`
	Model                string                `json:"model,omitempty"`
	GeneratedAt          *time.Time            `json:"generatedAt,omitempty"`
}

type Insight struct {
	ID                 string               `json:"id"`
	Source             string               `json:"source"`
	SourceID           string               `json:"sourceId"`
	Title              string               `json:"title"`
	Insight            string               `json:"insight"`
	RawInsight         string               `json:"rawInsight,omitempty"`
	CanonicalInsight   string               `json:"canonicalInsight,omitempty"`
	AbstractInsight    string               `json:"abstractInsight,omitempty"`
	PracticalText      string               `json:"practicalText,omitempty"`
	Mechanism          string               `json:"mechanism,omitempty"`
	InsightType        string               `json:"insightType,omitempty"`
	Domain             string               `json:"domain,omitempty"`
	Topics             []string             `json:"topics,omitempty"`
	Entities           []string             `json:"entities,omitempty"`
	Evidence           string               `json:"evidence"`
	EvidenceRefs       []InsightEvidenceRef `json:"evidenceRefs,omitempty"`
	SourceURL          string               `json:"sourceUrl"`
	Confidence         string               `json:"confidence"`
	ExplicitOrInferred string               `json:"explicitOrInferred,omitempty"`
	ImportanceScore    float64              `json:"importanceScore,omitempty"`
	NoveltyScore       float64              `json:"noveltyScore,omitempty"`
	ActionabilityScore float64              `json:"actionabilityScore,omitempty"`
	Quality            *QualityScore        `json:"quality,omitempty"`
	EmbeddingText      string               `json:"embeddingText,omitempty"`
	CacheStatus        string               `json:"cacheStatus,omitempty"`
	GeneratedAt        *time.Time           `json:"generatedAt,omitempty"`
}

type QualityScore struct {
	Overall     float64 `json:"overall,omitempty"`
	Conciseness float64 `json:"conciseness,omitempty"`
	Efficacy    float64 `json:"efficacy,omitempty"`
	Grounding   float64 `json:"grounding,omitempty"`
	Novelty     float64 `json:"novelty,omitempty"`
	Verdict     string  `json:"verdict,omitempty"`
	Rationale   string  `json:"rationale,omitempty"`
}

type ImportantTimeMarker struct {
	Label        string `json:"label"`
	Timestamp    string `json:"timestamp"`
	Seconds      int    `json:"seconds,omitempty"`
	WhyItMatters string `json:"whyItMatters"`
	Quote        string `json:"quote,omitempty"`
}

type InsightEvidenceRef struct {
	ChunkID    string `json:"chunkId,omitempty"`
	ChunkIndex *int   `json:"chunkIndex,omitempty"`
	Quote      string `json:"quote"`
}

type InsightCluster struct {
	ID                       string   `json:"id"`
	Label                    string   `json:"label"`
	CanonicalInsight         string   `json:"canonicalInsight"`
	Summary                  string   `json:"summary"`
	Layer                    string   `json:"layer"`
	Score                    float64  `json:"score"`
	RepresentativeInsightIDs []string `json:"representativeInsightIds"`
	InsightIDs               []string `json:"insightIds"`
}

type ActionItem struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	SourceID    string     `json:"sourceId"`
	Title       string     `json:"title"`
	Action      string     `json:"action"`
	Rationale   string     `json:"rationale"`
	SourceURL   string     `json:"sourceUrl"`
	Priority    string     `json:"priority"`
	CacheStatus string     `json:"cacheStatus,omitempty"`
	GeneratedAt *time.Time `json:"generatedAt,omitempty"`
}

type SourceArtifact struct {
	Source      string `json:"source"`
	SourceID    string `json:"sourceId"`
	Kind        string `json:"kind"`
	Bucket      string `json:"bucket"`
	Path        string `json:"path"`
	Checksum    string `json:"checksum"`
	ContentType string `json:"contentType"`
	ByteSize    int    `json:"byteSize"`
	Stored      bool   `json:"stored"`
	Error       string `json:"error,omitempty"`
}

type ProcessingEvent struct {
	Source        string `json:"source"`
	SourceID      string `json:"sourceId"`
	Title         string `json:"title"`
	CaptureHash   string `json:"captureHash"`
	PromptVersion string `json:"promptVersion"`
	Model         string `json:"model"`
	Status        string `json:"status"`
	Detail        string `json:"detail"`
}

type ThemeCluster struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Evidence string   `json:"evidence"`
	Score    float64  `json:"score"`
	Sources  []string `json:"sources"`
}

type SourceConnection struct {
	ID            string   `json:"id"`
	LeftSourceID  string   `json:"leftSourceId"`
	RightSourceID string   `json:"rightSourceId"`
	Relationship  string   `json:"relationship"`
	Evidence      string   `json:"evidence"`
	Confidence    string   `json:"confidence"`
	SharedSignals []string `json:"sharedSignals"`
}

type DigestDelivery struct {
	Provider          string     `json:"provider"`
	Recipient         string     `json:"recipient"`
	Status            string     `json:"status"`
	ProviderMessageID string     `json:"providerMessageId,omitempty"`
	Error             string     `json:"error,omitempty"`
	AttemptedAt       *time.Time `json:"attemptedAt,omitempty"`
}

type DigestSourceRef struct {
	SourceItemID         string     `json:"sourceItemId,omitempty"`
	SourceCaptureID      string     `json:"sourceCaptureId,omitempty"`
	KnowledgeSynthesisID string     `json:"knowledgeSynthesisId,omitempty"`
	Source               string     `json:"source"`
	ExternalID           string     `json:"externalId"`
	SourceURL            string     `json:"sourceUrl,omitempty"`
	Title                string     `json:"title,omitempty"`
	CaptureHash          string     `json:"captureHash,omitempty"`
	FirstSeenAt          *time.Time `json:"firstSeenAt,omitempty"`
	CapturedAt           *time.Time `json:"capturedAt,omitempty"`
	SynthesizedAt        *time.Time `json:"synthesizedAt,omitempty"`
	DigestRole           string     `json:"digestRole,omitempty"`
}

type DigestIssue struct {
	OwnerID              string            `json:"-"`
	ID                   string            `json:"id,omitempty"`
	DigestDate           string            `json:"digestDate"`
	ScheduledFor         time.Time         `json:"scheduledFor"`
	IdempotencyKey       string            `json:"idempotencyKey"`
	Subject              string            `json:"subject"`
	BodyMarkdown         string            `json:"bodyMarkdown"`
	Status               string            `json:"status"`
	IllustrationPrompt   string            `json:"illustrationPrompt,omitempty"`
	IllustrationAlt      string            `json:"illustrationAlt,omitempty"`
	IllustrationMimeType string            `json:"illustrationMimeType,omitempty"`
	IllustrationModel    string            `json:"illustrationModel,omitempty"`
	IllustrationBase64   string            `json:"-"`
	IllustrationURL      string            `json:"illustrationUrl,omitempty"`
	Deliveries           []DigestDelivery  `json:"deliveries,omitempty"`
	SourceRefs           []DigestSourceRef `json:"sources,omitempty"`
}

type DigestIllustration struct {
	ID       string
	Alt      string
	MimeType string
	Base64   string
}

type DigestSendRequest struct {
	RecipientEmail string       `json:"recipientEmail"`
	Digest         *DigestIssue `json:"digest,omitempty"`
}

type FeedbackEvent struct {
	OwnerID    string `json:"-"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Signal     string `json:"signal"`
	Note       string `json:"note,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

type TweetShareRequest struct {
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Text       string `json:"text"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

type TweetShareResult struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Result struct {
	GeneratedAt  time.Time `json:"generatedAt"`
	SourceStatus struct {
		X       SourceStatus `json:"x"`
		YouTube SourceStatus `json:"youtube"`
		OneCLI  SourceStatus `json:"onecli"`
	} `json:"sourceStatus"`
	XBookmarks      []XBookmark        `json:"xBookmarks"`
	YouTubeItems    []YouTubeItem      `json:"youtubeItems"`
	Summaries       []Summary          `json:"summaries"`
	Insights        []Insight          `json:"insights"`
	ActionItems     []ActionItem       `json:"actionItems"`
	Artifacts       []SourceArtifact   `json:"artifacts,omitempty"`
	Processing      []ProcessingEvent  `json:"processing,omitempty"`
	Themes          []ThemeCluster     `json:"themes,omitempty"`
	InsightClusters []InsightCluster   `json:"insightClusters,omitempty"`
	Connections     []SourceConnection `json:"connections,omitempty"`
	Digest          *DigestIssue       `json:"digest,omitempty"`
	Validation      []ValidationItem   `json:"validation"`
	Blockers        []string           `json:"blockers"`
}

type RefreshStatus struct {
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"startedAt"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	Error          string     `json:"error,omitempty"`
	Phase          string     `json:"phase,omitempty"`
	Message        string     `json:"message,omitempty"`
	ElapsedSeconds int64      `json:"elapsedSeconds,omitempty"`
}

type AskSecondBrainRequest struct {
	Question  string `json:"question"`
	UseLatest bool   `json:"useLatest,omitempty"`
}

type AskSecondBrainSource struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Source    string  `json:"source"`
	SourceURL string  `json:"sourceUrl"`
	Excerpt   string  `json:"excerpt"`
	Score     float64 `json:"score,omitempty"`
}

type AskSecondBrainResponse struct {
	Answer       string                 `json:"answer"`
	Sources      []AskSecondBrainSource `json:"sources"`
	UsedLatest   bool                   `json:"usedLatest"`
	Guardrail    string                 `json:"guardrail,omitempty"`
	Model        string                 `json:"model,omitempty"`
	GeneratedAt  time.Time              `json:"generatedAt"`
	SearchStatus string                 `json:"searchStatus,omitempty"`
}

type AppStateManifest struct {
	SchemaVersion string    `json:"schemaVersion"`
	RunID         string    `json:"runId"`
	GeneratedAt   time.Time `json:"generatedAt"`
	PublishedAt   time.Time `json:"publishedAt"`
	ETag          string    `json:"etag"`
	GraphStatus   string    `json:"graphStatus"`
	DigestStatus  string    `json:"digestStatus"`
}

type AppStateViews struct {
	Insights             []Insight     `json:"insights"`
	DailyNewsletter      *DigestIssue  `json:"dailyNewsletter,omitempty"`
	OriginalXBookmarks   []XBookmark   `json:"originalXBookmarks"`
	OriginalYouTubePosts []YouTubeItem `json:"originalYouTubePosts"`
}

type AppStateGraph struct {
	Status          string             `json:"status"`
	Themes          []ThemeCluster     `json:"themes"`
	InsightClusters []InsightCluster   `json:"insightClusters"`
	Connections     []SourceConnection `json:"connections"`
}

type AppStateAskContext struct {
	RunID     string                 `json:"runId"`
	Sources   []AskSecondBrainSource `json:"sources"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

type AppState struct {
	Manifest      AppStateManifest   `json:"manifest"`
	Latest        *Result            `json:"latest"`
	Views         AppStateViews      `json:"views"`
	Digests       []DigestIssue      `json:"digests"`
	RefreshStatus RefreshStatus      `json:"refreshStatus"`
	Graph         AppStateGraph      `json:"graph"`
	AskContext    AppStateAskContext `json:"askContext"`
}

type SynthesisCacheKey struct {
	SourceType    SourceType
	ExternalID    string
	CaptureHash   string
	PromptVersion string
	Model         string
}

func (key SynthesisCacheKey) String() string {
	return string(key.SourceType) + ":" + key.ExternalID + ":" + key.CaptureHash + ":" + key.PromptVersion + ":" + key.Model
}

type SynthesisRecord struct {
	SourceType    SourceType
	ExternalID    string
	CaptureHash   string
	PromptVersion string
	Model         string
	Summary       Summary
	Insights      []Insight
	ActionItems   []ActionItem
	GeneratedAt   time.Time
}

type ProcessedSource struct {
	OwnerID         string
	SourceType      SourceType
	ContentType     string
	ExternalID      string
	SourceURL       string
	Title           string
	AuthorName      string
	Username        string
	PublishedAt     string
	CaptureHash     string
	Artifact        SourceArtifact
	SummaryArtifact SourceArtifact
	Synthesis       SynthesisRecord
	Chunks          []SourceChunk
	Embeddings      []EmbeddingRecord
	Entities        []EntityRecord
	Keywords        []string
	Cached          bool
}

type SourceChunk struct {
	Index         int
	Content       string
	TokenEstimate int
	Checksum      string
}

type EmbeddingRecord struct {
	Type       string
	Label      string
	Model      string
	Dimensions int
	Vector     string
	ChunkIndex *int
}

type EntityRecord struct {
	Label      string
	Kind       string
	Confidence string
	Evidence   string
}
