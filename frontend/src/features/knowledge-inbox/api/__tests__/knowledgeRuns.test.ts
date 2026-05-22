import { afterEach, describe, expect, it, vi } from "vitest";

afterEach(() => {
  vi.resetModules();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

describe("knowledge run API client", () => {
  it("normalizes nullable collection fields from the latest-run endpoint", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          latest: {
            generatedAt: "2026-01-01T00:00:00.000Z",
            sourceStatus: { x: "ready", youtube: "blocked", onecli: "ready" },
            xBookmarks: null,
            youtubeItems: null,
            summaries: null,
            insights: null,
            actionItems: null,
            processing: null,
            validation: null,
            blockers: null
          }
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readLatestKnowledgeRun } = await import("../knowledgeRuns");
    const latest = await readLatestKnowledgeRun();

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/knowledge-runs/latest", {
      headers: { "Content-Type": "application/json" }
    });
    expect(latest).toMatchObject({
      xBookmarks: [],
      youtubeItems: [],
      summaries: [],
      insights: [],
      actionItems: [],
      processing: [],
      validation: [],
      blockers: []
    });
  });

  it("starts refresh requests as async POST jobs and surfaces backend errors", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("provider unavailable", { status: 503 }))
    );

    const { startKnowledgeInboxRefresh } = await import("../knowledgeRuns");

    await expect(startKnowledgeInboxRefresh()).rejects.toThrow("provider unavailable");
    expect(fetch).toHaveBeenCalledWith("https://api.example.test/api/knowledge-runs/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" }
    });
  });
});
