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
	VideoID                     string   `json:"videoId"`
	Title                       string   `json:"title"`
	ChannelTitle                string   `json:"channelTitle,omitempty"`
	PublishedAt                 string   `json:"publishedAt,omitempty"`
	SourceURL                   string   `json:"sourceUrl"`
	TranscriptStatus            string   `json:"transcriptStatus"`
	TranscriptLang              string   `json:"transcriptLang,omitempty"`
	TranscriptSourceLang        string   `json:"transcriptSourceLang,omitempty"`
	TranscriptAvailableLangs    []string `json:"transcriptAvailableLangs,omitempty"`
	TranscriptTranslationStatus string   `json:"transcriptTranslationStatus,omitempty"`
	TranscriptPreview           string   `json:"transcriptPreview,omitempty"`
	TranscriptOriginalPreview   string   `json:"transcriptOriginalPreview,omitempty"`
	TranscriptError             string   `json:"transcriptError,omitempty"`
}

type Summary struct {
	ID            string     `json:"id"`
	Source        string     `json:"source"`
	Title         string     `json:"title"`
	SourceURL     string     `json:"sourceUrl"`
	Decision      Decision   `json:"decision"`
	Summary       string     `json:"summary"`
	Quote         string     `json:"quote,omitempty"`
	Confidence    string     `json:"confidence"`
	Notes         []string   `json:"notes"`
	CacheStatus   string     `json:"cacheStatus,omitempty"`
	CaptureHash   string     `json:"captureHash,omitempty"`
	PromptVersion string     `json:"promptVersion,omitempty"`
	Model         string     `json:"model,omitempty"`
	GeneratedAt   *time.Time `json:"generatedAt,omitempty"`
}

type Insight struct {
	ID          string     `json:"id"`
	Source      string     `json:"source"`
	SourceID    string     `json:"sourceId"`
	Title       string     `json:"title"`
	Insight     string     `json:"insight"`
	Evidence    string     `json:"evidence"`
	SourceURL   string     `json:"sourceUrl"`
	Confidence  string     `json:"confidence"`
	CacheStatus string     `json:"cacheStatus,omitempty"`
	GeneratedAt *time.Time `json:"generatedAt,omitempty"`
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

type DigestIssue struct {
	OwnerID        string           `json:"-"`
	ID             string           `json:"id,omitempty"`
	DigestDate     string           `json:"digestDate"`
	ScheduledFor   time.Time        `json:"scheduledFor"`
	IdempotencyKey string           `json:"idempotencyKey"`
	Subject        string           `json:"subject"`
	BodyMarkdown   string           `json:"bodyMarkdown"`
	Status         string           `json:"status"`
	Deliveries     []DigestDelivery `json:"deliveries,omitempty"`
}

type FeedbackEvent struct {
	OwnerID    string `json:"-"`
	TargetType string `json:"targetType"`
	TargetID   string `json:"targetId"`
	Signal     string `json:"signal"`
	Note       string `json:"note,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

type Result struct {
	GeneratedAt  time.Time `json:"generatedAt"`
	SourceStatus struct {
		X       SourceStatus `json:"x"`
		YouTube SourceStatus `json:"youtube"`
		OneCLI  SourceStatus `json:"onecli"`
	} `json:"sourceStatus"`
	XBookmarks   []XBookmark        `json:"xBookmarks"`
	YouTubeItems []YouTubeItem      `json:"youtubeItems"`
	Summaries    []Summary          `json:"summaries"`
	Insights     []Insight          `json:"insights"`
	ActionItems  []ActionItem       `json:"actionItems"`
	Artifacts    []SourceArtifact   `json:"artifacts,omitempty"`
	Processing   []ProcessingEvent  `json:"processing,omitempty"`
	Themes       []ThemeCluster     `json:"themes,omitempty"`
	Connections  []SourceConnection `json:"connections,omitempty"`
	Digest       *DigestIssue       `json:"digest,omitempty"`
	Validation   []ValidationItem   `json:"validation"`
	Blockers     []string           `json:"blockers"`
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
	OwnerID     string
	SourceType  SourceType
	ExternalID  string
	SourceURL   string
	Title       string
	AuthorName  string
	Username    string
	PublishedAt string
	CaptureHash string
	Artifact    SourceArtifact
	Synthesis   SynthesisRecord
	Chunks      []SourceChunk
	Embeddings  []EmbeddingRecord
	Entities    []EntityRecord
	Keywords    []string
	Cached      bool
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
