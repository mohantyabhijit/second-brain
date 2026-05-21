import type { Summary, XBookmark, YouTubeItem } from "./types";

type RawSummaryInput = {
  id: string;
  source: Summary["source"];
  title: string;
  sourceUrl: string;
  body: string;
  quote?: string;
};

const strongSignals = ["research", "agent", "workflow", "build", "evidence", "benchmark", "model", "product"];
const weakSignals = ["giveaway", "discount", "sale", "promo"];

function sentenceSplit(text: string) {
  return text
    .replace(/\s+/g, " ")
    .split(/(?<=[.!?])\s+/)
    .map((sentence) => sentence.trim())
    .filter(Boolean);
}

function decisionFor(text: string): Summary["decision"] {
  const lower = text.toLowerCase();
  if (weakSignals.some((word) => lower.includes(word))) return "skip";
  if (strongSignals.some((word) => lower.includes(word))) return "read_now";
  return "later";
}

function extractiveSummary(input: RawSummaryInput): Summary {
  const sentences = sentenceSplit(input.body);
  const first = sentences[0] ?? input.body.slice(0, 220);
  const second = sentences.find((sentence) => sentence !== first && sentence.length > 50);
  const summary = [first, second].filter(Boolean).join(" ");

  return {
    id: input.id,
    source: input.source,
    title: input.title,
    sourceUrl: input.sourceUrl,
    decision: decisionFor(input.body),
    summary: summary || "No summary could be produced because the source text was empty.",
    quote: input.quote,
    confidence: input.body.length > 120 ? "medium" : "low",
    notes: ["Extractive fallback: no unsupported claims added beyond available source text."]
  };
}

export function summarizeBookmark(bookmark: XBookmark) {
  return extractiveSummary({
    id: bookmark.id,
    source: "x",
    title: bookmark.username ? `@${bookmark.username}` : bookmark.authorName ?? "X bookmark",
    sourceUrl: bookmark.sourceUrl,
    body: bookmark.text,
    quote: bookmark.text
  });
}

export function summarizeVideo(item: YouTubeItem) {
  return extractiveSummary({
    id: item.videoId,
    source: "youtube",
    title: item.title,
    sourceUrl: item.sourceUrl,
    body: item.transcriptPreview ?? item.title,
    quote: item.transcriptPreview
  });
}
