import { describe, expect, it } from "vitest";

import type { KnowledgeRunResult } from "../../contracts";
import { toKnowledgeInboxViewModel } from "../viewModel";

const run: KnowledgeRunResult = {
  generatedAt: "2026-01-01T00:00:00.000Z",
  sourceStatus: {
    x: "ready",
    youtube: "partial",
    onecli: "ready"
  },
  xBookmarks: [
    {
      id: "tweet-1",
      contentType: "article",
      text: "Short post",
      title: "Source article",
      body: "A longer captured article body.",
      previewText: "Captured article preview.",
      authorName: "Ada",
      username: "ada",
      createdAt: "2026-01-02T00:00:00.000Z",
      publicMetrics: { like_count: 10, repost_count: 2, reply_count: 0 },
      sourceUrl: "https://x.com/ada/article/tweet-1"
    }
  ],
  youtubeItems: [
    {
      videoId: "video-1",
      title: "Research talk",
      channelTitle: "Inbox Channel",
      publishedAt: "2026-01-03T00:00:00.000Z",
      sourceUrl: "https://www.youtube.com/watch?v=video-1",
      transcriptStatus: "available",
      transcriptLang: "en",
      transcriptSourceLang: "hi",
      transcriptTranslationStatus: "translated",
      transcriptPreview: "Translated transcript excerpt."
    }
  ],
  summaries: [
    {
      id: "tweet-1",
      source: "x",
      title: "Source article",
      sourceUrl: "https://x.com/ada/article/tweet-1",
      decision: "read_now",
      summary: "Useful source summary.",
      quote: "Supporting quote",
      confidence: "high",
      notes: [],
      cacheStatus: "cached"
    }
  ],
  insights: [
    {
      id: "insight-1",
      source: "youtube",
      sourceId: "video-1",
      title: "Research talk",
      insight: "The transcript has a reusable idea.",
      evidence: "Transcript evidence.",
      sourceUrl: "https://www.youtube.com/watch?v=video-1",
      confidence: "medium"
    }
  ],
  actionItems: [
    {
      id: "action-1",
      source: "x",
      sourceId: "tweet-1",
      title: "Follow up",
      action: "Turn this into a note.",
      rationale: "It is concrete.",
      sourceUrl: "https://x.com/ada/article/tweet-1",
      priority: "low"
    }
  ],
  processing: [
    {
      source: "x",
      sourceId: "tweet-1",
      title: "Source article",
      captureHash: "hash-1",
      promptVersion: "source-grounded-insights-v1",
      model: "extractive-fallback-v1",
      status: "cached",
      detail: "Skipped synthesis because this source capture was already processed."
    }
  ],
  validation: [
    { label: "X bookmark request", status: "pass", detail: "Fetched bookmarks." },
    { label: "YouTube playlist check", status: "pass", detail: "Fetched playlist items." },
    { label: "Transcript path", status: "blocked", detail: "No transcript extracted yet." }
  ],
  blockers: ["SUPADATA_API_KEY is missing."]
};

describe("toKnowledgeInboxViewModel", () => {
  it("summarizes source readiness, metrics, and validation progress", () => {
    const viewModel = toKnowledgeInboxViewModel(run, false, null);

    expect(viewModel.header.actionLabel).toBe("Refresh Inbox");
    expect(viewModel.sources.map((source) => source.statusLabel)).toEqual(["Ready", "Ready", "Partial"]);
    expect(viewModel.readiness.label).toBe("2/3 checks passing");
    expect(viewModel.readiness.value).toBe("67% indexed");
    expect(viewModel.metrics).toEqual([
      { label: "Captured items", value: "2" },
      { label: "Summaries", value: "1" },
      { label: "Insights", value: "1" },
      { label: "Action items", value: "1" },
      { label: "Cache hits", value: "1" },
      { label: "Transcripts", value: "1" },
      { label: "Themes", value: "0" },
      { label: "Connections", value: "0" },
      { label: "Digest", value: "not generated" },
      { label: "Open blockers", value: "1" }
    ]);
  });

  it("keeps source evidence visible across intake, review, and transcript panels", () => {
    const viewModel = toKnowledgeInboxViewModel(run, true, "Refresh failed");

    expect(viewModel.header.actionLabel).toBe("Running");
    expect(viewModel.error).toBe("Refresh failed");
    expect(viewModel.intake.items).toHaveLength(2);
    expect(viewModel.intake.items[0]).toMatchObject({
      id: "x-tweet-1",
      source: "X",
      item: "Source article",
      status: "Article captured",
      sourceUrl: "https://x.com/ada/article/tweet-1"
    });
    expect(viewModel.summaries.items.map((item) => item.decisionLabel)).toEqual(["Read now", "Insight", "low priority"]);
    expect(viewModel.transcripts.items[0]).toMatchObject({
      id: "video-1",
      statusLabel: "available (hi to en)",
      detail: "Translated transcript excerpt."
    });
  });
});
