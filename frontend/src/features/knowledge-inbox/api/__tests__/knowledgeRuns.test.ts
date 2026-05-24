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
