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
  description?: string;
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
  importantTimeMarkers?: ImportantTimeMarker[];
};

export type QualityScore = {
  overall?: number;
  conciseness?: number;
  efficacy?: number;
  grounding?: number;
  novelty?: number;
  verdict?: string;
  rationale?: string;
};

export type ImportantTimeMarker = {
  label: string;
  timestamp: string;
  seconds?: number;
  whyItMatters: string;
  quote?: string;
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
  quality?: QualityScore;
  importantTimeMarkers?: ImportantTimeMarker[];
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
  rawInsight?: string;
  canonicalInsight?: string;
  abstractInsight?: string;
  practicalText?: string;
  mechanism?: string;
  insightType?: string;
  domain?: string;
  topics?: string[];
  entities?: string[];
  evidence: string;
  evidenceRefs?: Array<{
    chunkId?: string;
    chunkIndex?: number;
    quote: string;
  }>;
  sourceUrl: string;
  confidence: "high" | "medium" | "low";
  explicitOrInferred?: "explicit" | "inferred";
  importanceScore?: number;
  noveltyScore?: number;
  actionabilityScore?: number;
  quality?: QualityScore;
  embeddingText?: string;
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

export type ThemeCluster = {
  id: string;
  label: string;
  evidence: string;
  score: number;
  sources: string[];
};

export type InsightCluster = {
  id: string;
  label: string;
  canonicalInsight: string;
  summary: string;
  layer: string;
  score: number;
  representativeInsightIds: string[];
  insightIds: string[];
};

export type SourceConnection = {
  id: string;
  leftSourceId: string;
  rightSourceId: string;
  relationship: string;
  evidence: string;
  confidence: "high" | "medium" | "low";
  sharedSignals: string[];
};

export type DigestIssue = {
  id?: string;
  digestDate: string;
  scheduledFor: string;
  idempotencyKey: string;
  subject: string;
  bodyMarkdown: string;
  status: "generated" | "sent" | "failed" | "blocked";
  deliveries?: Array<{
    provider: string;
    recipient: string;
    status: "sent" | "failed" | "blocked";
    providerMessageId?: string;
    error?: string;
    attemptedAt?: string;
  }>;
};

export type FeedbackSignal =
  | "useful"
  | "obvious"
  | "stale"
  | "irrelevant"
  | "more_like_this"
  | "less_like_this"
  | "archive"
  | "expand"
  | "copied"
  | "tweeted"
  | "upvote"
  | "downvote";

export type RefreshStatus = {
  id: string;
  status: "idle" | "running" | "completed" | "failed";
  startedAt: string;
  finishedAt?: string;
  error?: string;
  phase?: string;
  message?: string;
  elapsedSeconds?: number;
};

export type AskSecondBrainSource = {
  id: string;
  title: string;
  source: string;
  sourceUrl?: string;
  excerpt: string;
  score?: number;
};

export type AskSecondBrainResponse = {
  answer: string;
  sources: AskSecondBrainSource[];
  usedLatest: boolean;
  guardrail?: string;
  model?: string;
  generatedAt: string;
  searchStatus?: string;
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
  themes?: ThemeCluster[];
  insightClusters?: InsightCluster[];
  connections?: SourceConnection[];
  digest?: DigestIssue;
  validation: ValidationItem[];
  blockers: string[];
};
