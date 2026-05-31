#!/usr/bin/env node

import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync } from "node:fs";
import { join } from "node:path";

const baseUrl = (process.env.SECOND_BRAIN_BASE_URL ?? "https://abhijitmohanty.com/second-brain").replace(/\/$/, "");
const outputDir = process.env.PERF_OUTPUT_DIR ?? "/tmp/second-brain-performance";
const runLighthouse = process.env.RUN_LIGHTHOUSE === "1";
const lighthouseCommand = (process.env.LIGHTHOUSE_BIN ?? "lighthouse").split(/\s+/).filter(Boolean);
const chromePath = process.env.CHROME_PATH ?? "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";

const routes = [
  { name: "home", path: "/" },
  { name: "insights", path: "/insights/" },
  { name: "daily-newsletter", path: "/daily-newsletter/" },
  { name: "knowledge-graph", path: "/knowledge-graph/" },
  { name: "original-x-bookmarks", path: "/original-x-bookmarks/" },
  { name: "original-youtube-videos", path: "/original-youtube-videos/" }
];

const apiRoutes = [
  { name: "app-state-full", path: "/api/app-state" },
  { name: "app-state-insights", path: "/api/app-state?view=insights&limit=20" },
  { name: "app-state-newsletter", path: "/api/app-state?view=daily-newsletter&limit=10" },
  { name: "app-state-x", path: "/api/app-state?view=original-x-posts&limit=15" },
  { name: "app-state-youtube", path: "/api/app-state?view=original-youtube-posts&limit=15" },
  { name: "graph", path: "/api/knowledge-graph/insights?limit=180" },
  { name: "digests", path: "/api/digests" },
  { name: "latest", path: "/api/knowledge-runs/latest" }
];

mkdirSync(outputDir, { recursive: true });

const curlResults = [...routes, ...apiRoutes].map((route) => measureCurl(route));
const lighthouseResults = runLighthouse ? routes.map((route) => measureLighthouse(route)) : [];
const report = {
  measuredAt: new Date().toISOString(),
  baseUrl,
  curl: curlResults,
  lighthouse: lighthouseResults
};

console.log(JSON.stringify(report, null, 2));

function measureCurl(route) {
  const url = `${baseUrl}${route.path}`;
  const outputPath = join(outputDir, `${safeName(route.name)}.body`);
  const format = [
    "status=%{http_code}",
    "ttfb=%{time_starttransfer}",
    "total=%{time_total}",
    "transfer=%{size_download}",
    "type=%{content_type}"
  ].join(" ");
  const result = spawnSync("curl", ["-sS", "-L", "--compressed", "--max-time", "60", "-o", outputPath, "-w", format, url], {
    encoding: "utf8"
  });
  if (result.status !== 0) {
    return {
      name: route.name,
      url,
      error: (result.stderr || result.stdout || `curl exited ${result.status}`).trim()
    };
  }
  const parsed = Object.fromEntries(
    result.stdout
      .trim()
      .split(/\s+/)
      .map((part) => {
        const index = part.indexOf("=");
        return [part.slice(0, index), part.slice(index + 1)];
      })
  );
  return {
    name: route.name,
    url,
    status: Number(parsed.status),
    ttfbMs: Math.round(Number(parsed.ttfb) * 1000),
    totalMs: Math.round(Number(parsed.total) * 1000),
    transferBytes: Number(parsed.transfer),
    contentType: parsed.type,
    bodyBytes: existsSync(outputPath) ? readFileSync(outputPath).byteLength : 0
  };
}

function measureLighthouse(route) {
  const url = `${baseUrl}${route.path}`;
  const outputPath = join(outputDir, `${safeName(route.name)}.lighthouse.json`);
  const args = [
    url,
    `--chrome-path=${chromePath}`,
    "--chrome-flags=--headless=new --no-sandbox",
    "--preset=desktop",
    "--only-categories=performance",
    "--output=json",
    `--output-path=${outputPath}`,
    "--quiet",
    "--max-wait-for-load=45000"
  ];
  try {
    execFileSync(lighthouseCommand[0], [...lighthouseCommand.slice(1), ...args], { stdio: "pipe" });
    const report = JSON.parse(readFileSync(outputPath, "utf8"));
    const audits = report.audits ?? {};
    const network = audits["network-requests"]?.details?.items ?? [];
    return {
      name: route.name,
      url,
      score: Math.round((report.categories?.performance?.score ?? 0) * 100),
      fcpMs: Math.round(audits["first-contentful-paint"]?.numericValue ?? 0),
      lcpMs: Math.round(audits["largest-contentful-paint"]?.numericValue ?? 0),
      tbtMs: Math.round(audits["total-blocking-time"]?.numericValue ?? 0),
      cls: audits["cumulative-layout-shift"]?.numericValue ?? 0,
      speedIndexMs: Math.round(audits["speed-index"]?.numericValue ?? 0),
      totalTransferBytes: network.reduce((sum, item) => sum + (item.transferSize ?? 0), 0),
      requestCount: network.length,
      topTransfers: network
        .map((item) => ({
          url: item.url,
          type: item.resourceType,
          transferBytes: item.transferSize ?? 0,
          resourceBytes: item.resourceSize ?? 0,
          endMs: Math.round(item.networkEndTime ?? 0)
        }))
        .sort((a, b) => b.transferBytes - a.transferBytes)
        .slice(0, 10)
    };
  } catch (error) {
    return {
      name: route.name,
      url,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}

function safeName(name) {
  return name.replace(/[^a-z0-9-]+/gi, "-");
}
