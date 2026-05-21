export type SourceStatus = "ready" | "blocked" | "needs_secrets" | "partial";

export type Decision = "read_now" | "later" | "skip";

export type ValidationItem = {
  label: string;
  status: "pass" | "blocked" | "untested";
  detail: string;
};

export type XBookmark = {
  id: string;
  text: string;
  authorId?: string;
  authorName?: string;
  username?: string;
  createdAt?: string;
  publicMetrics?: Record<string, number>;
  sourceUrl: string;
};

export type YouTubeItem = {
  videoId: string;
  title: string;
  channelTitle?: string;
  publishedAt?: string;
  sourceUrl: string;
  transcriptStatus: "available" | "missing" | "blocked" | "untested";
  transcriptPreview?: string;
  transcriptError?: string;
};

export type Summary = {
  id: string;
  source: "x" | "youtube";
  title: string;
  sourceUrl: string;
  decision: Decision;
  summary: string;
  quote?: string;
  confidence: "medium" | "low";
  notes: string[];
};

export type PhaseOneResult = {
  generatedAt: string;
  sourceStatus: {
    x: SourceStatus;
    youtube: SourceStatus;
    onecli: SourceStatus;
  };
  xBookmarks: XBookmark[];
  youtubeItems: YouTubeItem[];
  summaries: Summary[];
  validation: ValidationItem[];
  blockers: string[];
};
