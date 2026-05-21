import type { PhaseOneResult } from "./types";

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

export async function readLatestPhaseOneResult() {
  const payload = await request<{ latest: PhaseOneResult | null }>("/api/phase1");
  return payload.latest ? normalizePhaseOneResult(payload.latest) : null;
}

export async function runPhaseOneValidation() {
  const payload = await request<PhaseOneResult>("/api/phase1/run", { method: "POST" });
  return normalizePhaseOneResult(payload);
}

function normalizePhaseOneResult(result: PhaseOneResult): PhaseOneResult {
  return {
    ...result,
    xBookmarks: result.xBookmarks ?? [],
    youtubeItems: result.youtubeItems ?? [],
    summaries: result.summaries ?? [],
    validation: result.validation ?? [],
    blockers: result.blockers ?? []
  };
}
