import type { KnowledgeRunResult } from "../contracts";

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
    throw new Error(detail || `API request failed with ${response.status}`);
  }

  return (await response.json()) as T;
}

export async function readLatestKnowledgeRun() {
  const payload = await request<{ latest: KnowledgeRunResult | null }>("/api/knowledge-runs/latest");
  return payload.latest ? normalizeKnowledgeRun(payload.latest) : null;
}

export async function runKnowledgeInboxValidation() {
  const payload = await request<KnowledgeRunResult>("/api/knowledge-runs/refresh", { method: "POST" });
  return normalizeKnowledgeRun(payload);
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
    validation: result.validation ?? [],
    blockers: result.blockers ?? []
  };
}
