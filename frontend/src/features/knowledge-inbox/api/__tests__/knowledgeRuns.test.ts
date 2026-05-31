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

  it("reads normalized app state from the Redis-backed boot endpoint", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          manifest: {
            schemaVersion: "redis-read-model-v1",
            runId: "run-1",
            generatedAt: "2026-05-24T00:00:00.000Z",
            publishedAt: "2026-05-24T00:00:01.000Z",
            etag: "abc",
            graphStatus: "derived",
            digestStatus: "generated"
          },
          latest: {
            generatedAt: "2026-05-24T00:00:00.000Z",
            sourceStatus: { x: "ready", youtube: "ready", onecli: "ready" },
            xBookmarks: null,
            youtubeItems: null,
            summaries: null,
            insights: null,
            actionItems: null,
            processing: null,
            validation: null,
            blockers: null
          },
          views: null,
          digests: null,
          refreshStatus: {
            id: "idle",
            status: "idle",
            startedAt: "2026-05-24T00:00:00.000Z"
          },
          graph: null,
          askContext: null
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readAppState } = await import("../knowledgeRuns");
    const state = await readAppState();

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/app-state", {
      headers: { "Content-Type": "application/json" }
    });
    expect(state.latest?.xBookmarks).toEqual([]);
    expect(state.digests).toEqual([]);
    expect(state.views.originalXBookmarks).toEqual([]);
    expect(state.graph.connections).toEqual([]);
    expect(state.askContext.sources).toEqual([]);
  });

  it("requests view-scoped app state for page boot", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          manifest: {
            schemaVersion: "redis-read-model-v1",
            runId: "run-1",
            generatedAt: "2026-05-24T00:00:00.000Z",
            publishedAt: "2026-05-24T00:00:01.000Z",
            etag: "abc",
            graphStatus: "derived",
            digestStatus: "sent"
          },
          latest: null,
          views: null,
          digests: null,
          refreshStatus: {
            id: "idle",
            status: "idle",
            startedAt: "2026-05-24T00:00:00.000Z"
          },
          graph: null,
          askContext: null
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readAppState } = await import("../knowledgeRuns");
    await readAppState("daily-newsletter", 10);

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/app-state?view=daily-newsletter&limit=10", {
      headers: { "Content-Type": "application/json" }
    });
  });

  it("reuses cached app state when the backend returns an unchanged ETag", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const payload = {
      manifest: {
        schemaVersion: "redis-read-model-v1",
        runId: "run-1",
        generatedAt: "2026-05-24T00:00:00.000Z",
        publishedAt: "2026-05-24T00:00:01.000Z",
        etag: "abc",
        graphStatus: "derived",
        digestStatus: "sent"
      },
      latest: null,
      views: null,
      digests: null,
      refreshStatus: {
        id: "idle",
        status: "idle",
        startedAt: "2026-05-24T00:00:00.000Z"
      },
      graph: null,
      askContext: null
    };
    const fetchMock = vi.fn(async (_url: string, init?: RequestInit) => {
      if (fetchMock.mock.calls.length === 1) {
        return new Response(JSON.stringify(payload), {
          status: 200,
          headers: { "Content-Type": "application/json", ETag: '"etag-1"' }
        });
      }
      expect(init?.headers).toMatchObject({
        "Content-Type": "application/json",
        "If-None-Match": '"etag-1"'
      });
      return new Response(null, { status: 304 });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readAppState } = await import("../knowledgeRuns");
    const first = await readAppState("daily-newsletter", 10);
    const second = await readAppState("daily-newsletter", 10);

    expect(second).toBe(first);
    expect(fetchMock).toHaveBeenNthCalledWith(2, "https://api.example.test/api/app-state?view=daily-newsletter&limit=10", {
      headers: {
        "Content-Type": "application/json",
        "If-None-Match": '"etag-1"'
      }
    });
  });

  it("reads persisted newsletter issues", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          digests: [
            {
              digestDate: "2026-05-24",
              scheduledFor: "2026-05-24T10:00:00Z",
              idempotencyKey: "daily:2026-05-24",
              subject: "Displayed digest",
              bodyMarkdown: "# Displayed digest",
              status: "generated"
            }
          ]
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readDigestIssues } = await import("../knowledgeRuns");
    const digests = await readDigestIssues();

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/digests", {
      headers: { "Content-Type": "application/json" }
    });
    expect(digests).toHaveLength(1);
    expect(digests[0]?.subject).toBe("Displayed digest");
  });

  it("normalizes insight graph responses", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          nodes: [
            {
              id: "insight-1",
              label: "Coordination cost",
              topics: null,
              domain: "organizations"
            }
          ],
          edges: null,
          stats: null
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { readInsightGraph } = await import("../knowledgeRuns");
    const graph = await readInsightGraph(25);

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/knowledge-graph/insights?limit=25", {
      headers: { "Content-Type": "application/json" }
    });
    expect(graph.nodes[0]?.topics).toEqual([]);
    expect(graph.edges).toEqual([]);
    expect(graph.stats).toEqual({ totalInsights: 0, returnedInsights: 1, returnedEdges: 0 });
  });

  it("sends the displayed digest payload for one-off email delivery", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    const fetchMock = vi.fn(async () => {
      return new Response(
        JSON.stringify({
          digestDate: "2026-05-24",
          subject: "Displayed digest",
          bodyMarkdown: "# Displayed digest",
          status: "sent"
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    });
    vi.stubGlobal("fetch", fetchMock);

    const { sendLatestDigest } = await import("../knowledgeRuns");
    await sendLatestDigest({
      recipientEmail: "reader@example.com",
      digest: {
        digestDate: "2026-05-24",
        scheduledFor: "2026-05-24T10:00:00Z",
        idempotencyKey: "daily:2026-05-24",
        subject: "Displayed digest",
        bodyMarkdown: "# Displayed digest",
        status: "sent"
      }
    });

    expect(fetchMock).toHaveBeenCalledWith("https://api.example.test/api/digests/send", {
      method: "POST",
      body: JSON.stringify({
        recipientEmail: "reader@example.com",
        digest: {
          digestDate: "2026-05-24",
          scheduledFor: "2026-05-24T10:00:00Z",
          idempotencyKey: "daily:2026-05-24",
          subject: "Displayed digest",
          bodyMarkdown: "# Displayed digest",
          status: "sent"
        }
      }),
      headers: { "Content-Type": "application/json" }
    });
  });

  it("sanitizes raw database connection errors from JSON error payloads", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE_URL", "https://api.example.test");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ error: "failed to connect to `user=postgres database=postgres`: dial tcp [2406::1]:5432: connect: no route to host" }), { status: 400 }))
    );

    const { sendLatestDigest } = await import("../knowledgeRuns");

    await expect(sendLatestDigest({ recipientEmail: "reader@example.com" })).rejects.toThrow("Local backend cannot reach Supabase Postgres.");
  });
});
