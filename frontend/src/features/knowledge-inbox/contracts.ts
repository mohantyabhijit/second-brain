export type SourceStatus = "ready" | "blocked" | "needs_secrets" | "partial";

export type Decision = "read_now" | "later" | "skip";

export type ValidationItem = {
  label: string;
  status: "pass" | "blocked" | "untested";
  detail: string;
};

export type SavedXItem = {
  id: string;
  contentType: "tweet" | "article";
  text: string;
  title?: string;
  body: string;
  previewText?: string;
  expandedUrl?: string;
  authorId?: string;
  authorName?: string;
  username?: string;
  createdAt?: string;
  publicMetrics?: Record<string, number>;
  sourceUrl: string;
};

export type SavedYouTubeItem = {
  videoId: string;
  title: string;
  channelTitle?: string;
  publishedAt?: string;
  sourceUrl: string;
  transcriptStatus: "available" | "missing" | "blocked" | "untested";
  transcriptLang?: string;
  transcriptSourceLang?: string;
  transcriptAvailableLangs?: string[];
  transcriptTranslationStatus?: "none" | "translated" | "blocked";
  transcriptPreview?: string;
  transcriptOriginalPreview?: string;
  transcriptError?: string;
};

export type KnowledgeSummary = {
  id: string;
  source: "x" | "youtube";
  title: string;
  sourceUrl: string;
  decision: Decision;
  summary: string;
  quote?: string;
  confidence: "high" | "medium" | "low";
  notes: string[];
  cacheStatus?: "generated" | "cached";
  captureHash?: string;
  promptVersion?: string;
  model?: string;
  generatedAt?: string;
};

export type KnowledgeInsight = {
  id: string;
  source: "x" | "youtube";
  sourceId: string;
  title: string;
  insight: string;
  evidence: string;
  sourceUrl: string;
  confidence: "high" | "medium" | "low";
  cacheStatus?: "generated" | "cached";
  generatedAt?: string;
};

export type KnowledgeActionItem = {
  id: string;
  source: "x" | "youtube";
  sourceId: string;
  title: string;
  action: string;
  rationale: string;
  sourceUrl: string;
  priority: "high" | "medium" | "low";
  cacheStatus?: "generated" | "cached";
  generatedAt?: string;
};

export type ProcessingEvent = {
  source: "x" | "youtube";
  sourceId: string;
  title: string;
  captureHash: string;
  promptVersion: string;
  model: string;
  status: "generated" | "cached";
  detail: string;
};

export type KnowledgeRunResult = {
  generatedAt: string;
  sourceStatus: {
    x: SourceStatus;
    youtube: SourceStatus;
    onecli: SourceStatus;
  };
  xBookmarks: SavedXItem[];
  youtubeItems: SavedYouTubeItem[];
  summaries: KnowledgeSummary[];
  insights: KnowledgeInsight[];
  actionItems: KnowledgeActionItem[];
  processing?: ProcessingEvent[];
  validation: ValidationItem[];
  blockers: string[];
};
