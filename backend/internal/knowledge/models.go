package knowledge

import "time"

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
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Title      string   `json:"title"`
	SourceURL  string   `json:"sourceUrl"`
	Decision   Decision `json:"decision"`
	Summary    string   `json:"summary"`
	Quote      string   `json:"quote,omitempty"`
	Confidence string   `json:"confidence"`
	Notes      []string `json:"notes"`
}

type Result struct {
	GeneratedAt  time.Time `json:"generatedAt"`
	SourceStatus struct {
		X       SourceStatus `json:"x"`
		YouTube SourceStatus `json:"youtube"`
		OneCLI  SourceStatus `json:"onecli"`
	} `json:"sourceStatus"`
	XBookmarks   []XBookmark      `json:"xBookmarks"`
	YouTubeItems []YouTubeItem    `json:"youtubeItems"`
	Summaries    []Summary        `json:"summaries"`
	Validation   []ValidationItem `json:"validation"`
	Blockers     []string         `json:"blockers"`
}
