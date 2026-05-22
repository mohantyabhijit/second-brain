import type { Decision, KnowledgeRunResult, SavedYouTubeItem, SourceStatus, ValidationItem } from "../contracts";

export type IconName = "key" | "x" | "youtube" | "check" | "run" | "link" | "alert" | "spark";

export type NavigationItemViewModel = {
  label: string;
  href: string;
};

export type SourceCardViewModel = {
  label: string;
  detail: string;
  status: SourceStatus;
  statusLabel: string;
  icon: IconName;
};

export type ReadinessViewModel = {
  label: string;
  value: string;
  progress: number;
  generatedAtLabel: string;
};

export type MetricViewModel = {
  label: string;
  value: string;
};

export type IntakeRowViewModel = {
  id: string;
  source: string;
  item: string;
  body: string;
  author: string;
  status: string;
  timestamp: string;
  stats?: string;
  sourceUrl: string;
};

export type ValidationItemViewModel = ValidationItem & {
  icon: IconName;
};

export type SummaryCardViewModel = {
  id: string;
  source: "x" | "youtube" | "insight" | "action";
  title: string;
  body: string;
  quote?: string;
  sourceUrl: string;
  decision: Decision;
  decisionLabel: string;
  confidenceLabel: string;
  cacheStatus?: string;
};

export type TranscriptItemViewModel = {
  id: string;
  title: string;
  author: string;
  statusLabel: string;
  detail: string;
  timestamp: string;
  sourceUrl: string;
};

export type EmptyStateViewModel = {
  icon: IconName;
  title: string;
  body: string;
};

export type PanelViewModel<T> = {
  title: string;
  description: string;
  icon: IconName;
  items: T[];
  empty: EmptyStateViewModel;
};

export type KnowledgeInboxViewModel = {
  brand: {
    mark: string;
    name: string;
    descriptor: string;
  };
  navigation: NavigationItemViewModel[];
  sidebarNote: {
    label: string;
    value: string;
  };
  header: {
    eyebrow: string;
    title: string;
    description: string;
    actionLabel: string;
    isRunning: boolean;
  };
  sources: SourceCardViewModel[];
  readiness: ReadinessViewModel;
  metrics: MetricViewModel[];
  intake: PanelViewModel<IntakeRowViewModel>;
  validation: PanelViewModel<ValidationItemViewModel>;
  summaries: PanelViewModel<SummaryCardViewModel>;
  transcripts: PanelViewModel<TranscriptItemViewModel>;
  blockers: {
    eyebrow: string;
    title: string;
    items: string[];
  };
  error: string | null;
};

const statusLabels: Record<SourceStatus, string> = {
  ready: "Ready",
  partial: "Partial",
  blocked: "Blocked",
  needs_secrets: "Needs secrets"
};

const decisionLabels: Record<Decision, string> = {
  read_now: "Read now",
  later: "Later",
  skip: "Skip"
};

const navItems: NavigationItemViewModel[] = [
  { label: "Home", href: "/" },
  { label: "Daily Newsletter", href: "/daily-newsletter" },
  { label: "Original X Posts", href: "/original-x-posts" },
  { label: "Original YouTube Posts", href: "/original-youtube-posts" }
];

export function toKnowledgeInboxViewModel(run: KnowledgeRunResult, isRunning: boolean, error: string | null): KnowledgeInboxViewModel {
  const validationPassCount = run.validation.filter((item) => item.status === "pass").length;
  const totalFetched = run.xBookmarks.length + run.youtubeItems.length;
  const transcriptCount = run.youtubeItems.filter((item) => item.transcriptStatus === "available").length;
  const cachedCount = (run.processing ?? []).filter((item) => item.status === "cached").length;
  const progress = run.validation.length ? Math.round((validationPassCount / run.validation.length) * 100) : 0;

  return {
    brand: {
      mark: "SB",
      name: "Second Brain",
      descriptor: "Knowledge Inbox"
    },
    navigation: navItems,
    sidebarNote: {
      label: "Operating mode",
      value: "Source-grounded research memory"
    },
    header: {
      eyebrow: "Knowledge inbox",
      title: "Second Brain Command Center",
      description: "A durable intake surface for saved links, videos, transcripts, and source-grounded reading decisions.",
      actionLabel: isRunning ? "Running" : "Refresh Inbox",
      isRunning
    },
    sources: [
      sourceCard("OneCLI Secrets", "Credential vault", run.sourceStatus.onecli, "key"),
      sourceCard("X Bookmarks", "Recent saved posts", run.sourceStatus.x, "x"),
      sourceCard("YouTube Inbox", "Playlist and captions", run.sourceStatus.youtube, "youtube")
    ],
    readiness: {
      label: `${validationPassCount}/${run.validation.length} checks passing`,
      value: `${progress}% indexed`,
      progress,
      generatedAtLabel: formatGeneratedAt(run.generatedAt)
    },
    metrics: [
      { label: "Captured items", value: String(totalFetched) },
      { label: "Summaries", value: String(run.summaries.length) },
      { label: "Insights", value: String(run.insights.length) },
      { label: "Action items", value: String(run.actionItems.length) },
      { label: "Cache hits", value: String(cachedCount) },
      { label: "Transcripts", value: String(transcriptCount) },
      { label: "Themes", value: String(run.themes?.length ?? 0) },
      { label: "Connections", value: String(run.connections?.length ?? 0) },
      { label: "Digest", value: run.digest?.status ?? "not generated" },
      { label: "Open blockers", value: String(run.blockers.length) }
    ],
    intake: {
      title: "Recent Intake",
      description: "Source rows normalized for review across saved posts and video inboxes.",
      icon: "spark",
      items: [
        ...run.xBookmarks.map((bookmark): IntakeRowViewModel => ({
          id: `x-${bookmark.id}`,
          source: "X",
          item: bookmark.title ?? bookmark.text,
          body: bookmark.previewText ?? bookmark.body ?? bookmark.text,
          author: bookmark.username ? `@${bookmark.username}` : bookmark.authorName ?? bookmark.authorId ?? "Unknown",
          status: bookmark.contentType === "article" ? "Article captured" : "Post captured",
          timestamp: formatSourceDate(bookmark.createdAt),
          stats: formatPublicMetrics(bookmark.publicMetrics),
          sourceUrl: bookmark.sourceUrl
        })),
        ...run.youtubeItems.map((item): IntakeRowViewModel => ({
          id: `youtube-${item.videoId}`,
          source: "YouTube",
          item: item.title,
          body: item.transcriptPreview ?? item.transcriptOriginalPreview ?? item.transcriptError ?? "Transcript not available in this run.",
          author: item.channelTitle ?? "Unknown",
          status: transcriptLabel(item),
          timestamp: formatSourceDate(item.publishedAt),
          stats: item.transcriptStatus === "available" ? "transcript available" : item.transcriptStatus,
          sourceUrl: item.sourceUrl
        }))
      ],
      empty: {
        icon: "spark",
        title: "No source rows yet",
        body: "Connect source credentials and refresh the inbox."
      }
    },
    validation: {
      title: "Quality Gate",
      description: "Acceptance checks before the inbox can be trusted.",
      icon: "check",
      items: run.validation.map((item) => ({
        ...item,
        icon: item.status === "pass" ? "check" : "alert"
      })),
      empty: {
        icon: "check",
        title: "No checks configured",
        body: "Validation rules will appear once the backend returns them."
      }
    },
    summaries: {
      title: "Review Queue",
      description: "Reading decisions, insights, action items, and cache-aware synthesis with attribution preserved.",
      icon: "link",
      items: [
        ...run.summaries.map((summary) => ({
          id: `${summary.source}-${summary.id}`,
          source: summary.source,
          title: summary.title,
          body: summary.summary,
          quote: summary.quote,
          sourceUrl: summary.sourceUrl,
          decision: summary.decision,
          decisionLabel: decisionLabels[summary.decision],
          confidenceLabel: `${summary.confidence} confidence`,
          cacheStatus: summary.cacheStatus
        })),
        ...run.insights.map((insight) => ({
          id: `insight-${insight.id}`,
          source: "insight" as const,
          title: insight.title,
          body: insight.insight,
          quote: insight.evidence,
          sourceUrl: insight.sourceUrl,
          decision: "read_now" as Decision,
          decisionLabel: "Insight",
          confidenceLabel: `${insight.confidence} confidence`,
          cacheStatus: insight.cacheStatus
        })),
        ...run.actionItems.map((action) => ({
          id: `action-${action.id}`,
          source: "action" as const,
          title: action.title,
          body: action.action,
          quote: action.rationale,
          sourceUrl: action.sourceUrl,
          decision: action.priority === "low" ? ("later" as Decision) : ("read_now" as Decision),
          decisionLabel: `${action.priority} priority`,
          confidenceLabel: action.cacheStatus === "cached" ? "cached" : "generated",
          cacheStatus: action.cacheStatus
        }))
      ],
      empty: {
        icon: "spark",
        title: "No summaries yet",
        body: "Source-grounded summaries will appear once usable content is available."
      }
    },
    transcripts: {
      title: "Transcript Evidence",
      description: "Caption status for videos entering the research memory.",
      icon: "youtube",
      items: run.youtubeItems.map((item) => ({
        id: item.videoId,
        title: item.title,
        author: item.channelTitle ?? "YouTube",
        statusLabel: transcriptLabel(item),
        detail: item.transcriptPreview ?? item.transcriptOriginalPreview ?? item.transcriptError ?? "Not tested in this run.",
        timestamp: formatSourceDate(item.publishedAt),
        sourceUrl: item.sourceUrl
      })),
      empty: {
        icon: "youtube",
        title: "Waiting for video inbox",
        body: "Set a YouTube inbox playlist and refresh the app."
      }
    },
    blockers: {
      eyebrow: "Setup required",
      title: "Current blockers",
      items: run.blockers
    },
    error
  };
}

function sourceCard(label: string, detail: string, status: SourceStatus, icon: IconName): SourceCardViewModel {
  return {
    label,
    detail,
    status,
    statusLabel: statusLabels[status],
    icon
  };
}

function transcriptLabel(item: SavedYouTubeItem) {
  if (item.transcriptTranslationStatus === "translated" && item.transcriptSourceLang) {
    return `${item.transcriptStatus} (${item.transcriptSourceLang} to en)`;
  }
  if (item.transcriptTranslationStatus === "blocked" && item.transcriptSourceLang) {
    return `${item.transcriptStatus} (${item.transcriptSourceLang}, translation blocked)`;
  }
  if (item.transcriptLang) return `${item.transcriptStatus} (${item.transcriptLang})`;
  return item.transcriptStatus;
}

function formatGeneratedAt(generatedAt: string) {
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "UTC"
  }).format(new Date(generatedAt));
}

function formatSourceDate(value: string | undefined) {
  if (!value) return "Source date unknown";
  return new Intl.DateTimeFormat("en", {
    dateStyle: "medium",
    timeZone: "UTC"
  }).format(new Date(value));
}

function formatPublicMetrics(metrics: Record<string, number> | undefined) {
  if (!metrics) return undefined;

  return Object.entries(metrics)
    .filter(([, value]) => value > 0)
    .slice(0, 3)
    .map(([key, value]) => `${value} ${key.replace(/_/g, " ")}`)
    .join(" - ");
}
