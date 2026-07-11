import { expect, test, type Page, type Route } from "@playwright/test";

const appState = {
  manifest: {
    schemaVersion: "1",
    runId: "run-e2e",
    generatedAt: "2026-07-11T00:00:00Z",
    publishedAt: "2026-07-11T00:01:00Z",
    etag: "e2e",
    graphStatus: "ready",
    digestStatus: "sent",
  },
  latest: {
    generatedAt: "2026-07-11T00:00:00Z",
    sourceStatus: { x: "ready", youtube: "ready", onecli: "ready" },
    sourceCounts: { xBookmarks: 1, youtubeItems: 1 },
    xBookmarks: [
      {
        id: "x-1",
        contentType: "tweet",
        text: "Evidence beats intuition",
        body: "Measure the system before changing it.",
        authorName: "Ada",
        username: "ada",
        createdAt: "2026-07-10T10:00:00Z",
        sourceUrl: "https://x.com/ada/status/1",
      },
    ],
    youtubeItems: [
      {
        videoId: "video-1",
        title: "Refactoring with confidence",
        channelTitle: "Engineering Notes",
        sourceUrl: "https://youtube.com/watch?v=video-1",
        transcriptStatus: "available",
        transcriptPreview: "Characterization tests protect behavior.",
      },
    ],
    summaries: [
      {
        id: "summary-1",
        source: "x",
        title: "Evidence beats intuition",
        sourceUrl: "https://x.com/ada/status/1",
        decision: "read_now",
        summary: "Baseline first, then simplify.",
        quote: "Measure the system before changing it.",
        confidence: "high",
        notes: [],
      },
    ],
    insights: [
      {
        id: "insight-1",
        source: "x",
        sourceId: "x-1",
        title: "Safe refactoring",
        insight:
          "Characterization tests turn legacy behavior into an explicit contract.",
        evidence: "Measure the system before changing it.",
        sourceUrl: "https://x.com/ada/status/1",
        confidence: "high",
        topics: ["testing"],
      },
    ],
    actionItems: [],
    processing: [],
    themes: [],
    connections: [],
    validation: [{ label: "Fixture", status: "pass", detail: "Ready" }],
    blockers: [],
  },
  views: {
    insights: [],
    originalXBookmarks: [],
    originalYouTubePosts: [],
  },
  sourceCounts: { xBookmarks: 1, youtubeItems: 1 },
  digests: [
    {
      id: "digest-1",
      digestDate: "2026-07-11",
      scheduledFor: "2026-07-11T10:00:00Z",
      idempotencyKey: "digest-e2e",
      subject: "Weekly systems review",
      bodyMarkdown: "## What changed\n\nTests now protect the critical path.",
      status: "sent",
      sources: [],
    },
  ],
  refreshStatus: {
    id: "refresh-e2e",
    status: "idle",
    startedAt: "2026-07-11T00:00:00Z",
  },
  graph: {
    status: "ready",
    themes: [],
    insightClusters: [],
    connections: [],
    insightGraph: {
      nodes: [
        { id: "insight-1", label: "Safe refactoring", topics: ["testing"] },
      ],
      edges: [],
      stats: { totalInsights: 1, returnedInsights: 1, returnedEdges: 0 },
    },
  },
  askContext: {
    runId: "run-e2e",
    sources: [],
    updatedAt: "2026-07-11T00:01:00Z",
  },
};

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    headers: { "Access-Control-Allow-Origin": "*" },
    body: JSON.stringify(body),
  });
}

async function mockAPI(page: Page) {
  await page.route("http://127.0.0.1:8080/api/**", async (route) => {
    const request = route.request();
    const pathname = new URL(request.url()).pathname;
    if (request.method() === "OPTIONS") {
      await route.fulfill({
        status: 204,
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Headers": "Content-Type, Authorization",
        },
      });
      return;
    }
    if (pathname === "/api/app-state") return json(route, appState);
    if (pathname === "/api/knowledge-runs/refresh")
      return json(route, appState.refreshStatus);
    if (pathname === "/api/ask") {
      return json(route, {
        answer:
          "Use characterization tests before simplifying the implementation.",
        sources: [
          {
            id: "x-1",
            title: "Evidence beats intuition",
            source: "x",
            excerpt: "Measure first.",
          },
        ],
        usedLatest: true,
        generatedAt: "2026-07-11T00:02:00Z",
      });
    }
    return json(
      route,
      { error: `Unexpected E2E request: ${request.method()} ${pathname}` },
      500,
    );
  });
}

test.beforeEach(async ({ page }) => mockAPI(page));

const routes = [
  ["/", "Insights"],
  ["/insights", "Insights"],
  ["/daily-newsletter", "Daily Newsletter"],
  ["/original-x-bookmarks", "Original X Bookmarks"],
  ["/original-x-posts", "Original X Bookmarks"],
  ["/original-youtube-videos", "Original YouTube Videos"],
  ["/original-youtube-posts", "Original YouTube Videos"],
  ["/knowledge-graph", "Knowledge Graph"],
] as const;

for (const [path, heading] of routes) {
  test(`${path} renders its supported view`, async ({ page }) => {
    await page.goto(path);
    await expect(
      page.getByRole("heading", { level: 1, name: heading }),
    ).toBeVisible();
    await expect(page.getByLabel("Second Brain navigation")).toBeVisible();
  });
}

test("insight cards open and close the full summary", async ({ page }) => {
  await page.goto("/insights");
  await page.getByRole("button", { name: "Safe refactoring" }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "Characterization tests",
  );
  await page.getByRole("button", { name: "Close AI summary" }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
});

test("X bookmarks can be searched and cleared", async ({ page }) => {
  await page.goto("/original-x-bookmarks");
  const search = page.getByRole("searchbox", { name: "Search X bookmarks" });
  await expect(
    page.getByText("Evidence beats intuition", { exact: true }),
  ).toBeVisible();
  await search.fill("not present");
  await expect(
    page.getByRole("heading", {
      name: "No loaded X bookmarks match this search",
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Clear source search" }).click();
  await expect(
    page.getByText("Evidence beats intuition", { exact: true }),
  ).toBeVisible();
});

test("YouTube sources render transcript-grounded content", async ({ page }) => {
  await page.goto("/original-youtube-videos");
  await expect(
    page.getByText("Refactoring with confidence", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("searchbox", { name: "Search YouTube sources" })
    .fill("Engineering Notes");
  await expect(
    page.getByText("Refactoring with confidence", { exact: true }),
  ).toBeVisible();
});

test("newsletter archive searches and opens a persisted issue", async ({
  page,
}) => {
  await page.goto("/daily-newsletter");
  await page
    .getByRole("searchbox", { name: "Search newsletter issues" })
    .fill("systems");
  await page.getByRole("button", { name: "Weekly systems review" }).click();
  await expect(page.getByRole("dialog")).toContainText(
    "Tests now protect the critical path",
  );
});

test("knowledge graph supports viewport controls and node selection", async ({
  page,
}) => {
  await page.goto("/knowledge-graph");
  await expect(page.getByLabel("Insight knowledge graph")).toBeVisible();
  await page.getByRole("button", { name: "Zoom in" }).click();
  await page.getByRole("button", { name: "Safe refactoring" }).click();
  await expect(page.getByLabel("Insight knowledge graph")).toContainText(
    "Safe refactoring",
  );
});

test("Ask Second Brain submits a grounded question and renders its source", async ({
  page,
}) => {
  await page.goto("/insights");
  await page.getByRole("button", { name: "Ask Second Brain" }).click();
  await page
    .getByRole("textbox", { name: "Your question" })
    .fill("How should I refactor safely?");
  await page.getByRole("button", { name: "Ask", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Ask Second Brain" }),
  ).toContainText("Use characterization tests");
  await expect(
    page.getByRole("dialog", { name: "Ask Second Brain" }),
  ).toContainText("Evidence beats intuition");
});
