import type { AppState, AskSecondBrainResponse, DigestIssue, FeedbackSignal, InsightGraphResponse, KnowledgeRunResult, RefreshStatus } from "../contracts";

export type AppStateView = "insights" | "daily-newsletter" | "original-x-posts" | "original-youtube-posts" | "knowledge-graph";

const apiBaseUrl = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...init?.headers
    }
  });

  if (!response.ok) {
    const detail = await response.text();
    throw new Error(readAPIError(detail) || `API request failed with ${response.status}`);
  }

  return (await response.json()) as T;
}

export async function readLatestKnowledgeRun() {
  const payload = await request<{ latest: KnowledgeRunResult | null }>("/api/knowledge-runs/latest");
  return payload.latest ? normalizeKnowledgeRun(payload.latest) : null;
}

export async function readAppState(view?: AppStateView, limit = 25) {
  const params = new URLSearchParams();
  if (view) {
    params.set("view", view);
    params.set("limit", String(limit));
  }
  const path = params.size ? `/api/app-state?${params.toString()}` : "/api/app-state";
  const payload = await request<AppState>(path);
  return normalizeAppState(payload);
}

export async function readDigestIssues() {
  const payload = await request<{ digests?: DigestIssue[] }>("/api/digests");
  return payload.digests ?? [];
}

export async function startKnowledgeInboxRefresh() {
  return request<RefreshStatus>("/api/knowledge-runs/refresh", { method: "POST" });
}

export async function readKnowledgeRefreshStatus() {
  return request<RefreshStatus>("/api/knowledge-runs/refresh");
}

export async function saveKnowledgeFeedback(input: {
  targetType: string;
  targetId: string;
  signal: FeedbackSignal;
  note?: string;
  sourceUrl?: string;
}) {
  return request<{ status: string }>("/api/feedback", {
    method: "POST",
    body: JSON.stringify(input)
  });
}

export async function generateDailyDigest() {
  return request<NonNullable<KnowledgeRunResult["digest"]>>("/api/digests/generate", { method: "POST" });
}

export async function sendLatestDigest(input: { recipientEmail: string; digest?: DigestIssue }) {
  return request<NonNullable<KnowledgeRunResult["digest"]>>("/api/digests/send", {
    method: "POST",
    body: JSON.stringify(input)
  });
}

export async function shareInsightToX(input: {
  targetType: string;
  targetId: string;
  text: string;
  sourceUrl?: string;
}) {
  return request<{ id: string; text: string }>("/api/share/tweet", {
    method: "POST",
    body: JSON.stringify(input)
  });
}

export async function askSecondBrain(input: { question: string; useLatest?: boolean }) {
  return request<AskSecondBrainResponse>("/api/ask", {
    method: "POST",
    body: JSON.stringify(input)
  });
}

export async function readInsightGraph(limit = 180) {
  const params = new URLSearchParams({ limit: String(limit) });
  const payload = await request<InsightGraphResponse>(`/api/knowledge-graph/insights?${params.toString()}`);
  return normalizeInsightGraph(payload);
}

function normalizeKnowledgeRun(result: KnowledgeRunResult): KnowledgeRunResult {
  return {
    ...result,
    xBookmarks: result.xBookmarks ?? [],
    youtubeItems: result.youtubeItems ?? [],
    summaries: result.summaries ?? [],
    insights: result.insights ?? [],
    actionItems: result.actionItems ?? [],
    processing: result.processing ?? [],
    themes: result.themes ?? [],
    connections: result.connections ?? [],
    validation: result.validation ?? [],
    blockers: result.blockers ?? []
  };
}

function normalizeAppState(state: AppState): AppState {
  const latest = state.latest ? normalizeKnowledgeRun(state.latest) : null;
  return {
    ...state,
    latest,
    digests: state.digests ?? [],
    views: {
      insights: state.views?.insights ?? latest?.insights ?? [],
      dailyNewsletter: state.views?.dailyNewsletter ?? latest?.digest,
      originalXBookmarks: state.views?.originalXBookmarks ?? latest?.xBookmarks ?? [],
      originalYouTubePosts: state.views?.originalYouTubePosts ?? latest?.youtubeItems ?? []
    },
    graph: {
      status: state.graph?.status ?? state.manifest?.graphStatus ?? "none",
      themes: state.graph?.themes ?? latest?.themes ?? [],
      insightClusters: state.graph?.insightClusters ?? latest?.insightClusters ?? [],
      connections: state.graph?.connections ?? latest?.connections ?? []
    },
    askContext: {
      runId: state.askContext?.runId ?? state.manifest?.runId ?? "none",
      sources: state.askContext?.sources ?? [],
      updatedAt: state.askContext?.updatedAt ?? state.manifest?.publishedAt ?? new Date(0).toISOString()
    }
  };
}

function normalizeInsightGraph(result: InsightGraphResponse): InsightGraphResponse {
  return {
    nodes: (result.nodes ?? []).map((node) => ({
      ...node,
      topics: node.topics ?? []
    })),
    edges: result.edges ?? [],
    stats: {
      totalInsights: result.stats?.totalInsights ?? 0,
      returnedInsights: result.stats?.returnedInsights ?? result.nodes?.length ?? 0,
      returnedEdges: result.stats?.returnedEdges ?? result.edges?.length ?? 0
    }
  };
}

function readAPIError(detail: string) {
  try {
    const payload = JSON.parse(detail) as { error?: string };
    if (typeof payload.error === "string" && payload.error.trim() !== "") {
      return sanitizeAPIError(payload.error);
    }
  } catch {
    // Fall through to plain text handling.
  }
  return sanitizeAPIError(detail);
}

function sanitizeAPIError(message: string) {
  const trimmed = message.trim();
  if (trimmed === "") {
    return "";
  }
  const lower = trimmed.toLowerCase();
  if (lower.includes("failed to connect") && lower.includes("database=postgres")) {
    return "Local backend cannot reach Supabase Postgres. Use the displayed digest send path or switch local SUPABASE_DB_URL to the pooled connection string.";
  }
  return trimmed;
}
