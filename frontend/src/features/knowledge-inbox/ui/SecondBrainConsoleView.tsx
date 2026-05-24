"use client";

import { type FormEvent, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { ChatMessage } from "../model/useKnowledgeInboxController";
import type { KnowledgeInboxViewModel, NavigationItemViewModel, SummaryCardViewModel } from "../presentation/viewModel";
import type { DigestIssue, FeedbackSignal, ImportantTimeMarker, QualityScore, RefreshStatus } from "../contracts";
type SecondBrainConsoleViewProps = {
  activePage: KnowledgeInboxPage;
  chatMessages: ChatMessage[];
  isAsking: boolean;
  isDigesting: boolean;
  isLoading: boolean;
  model: KnowledgeInboxViewModel;
  refreshStatus: RefreshStatus | null;
  onAsk: (question: string, useLatest?: boolean) => Promise<void>;
  onDigest: () => void;
  onSendDigest: (recipientEmail: string) => Promise<DigestIssue>;
  onFeedback: (targetType: string, targetId: string, signal: FeedbackSignal, sourceUrl?: string) => void;
};

type FeedSource = "Summary" | "Quote" | "Insight" | "Action" | "X" | "YouTube" | "Newsletter";

type FeedItem = {
  id: string;
  source: FeedSource;
  eyebrow: string;
  title: string;
  body: string;
  quote?: string;
  author: string;
  timestamp: string;
  sourceUrl?: string;
  stats?: string;
  quality?: QualityScore;
  timeMarkers?: ImportantTimeMarker[];
};

const pageCopy: Record<
  KnowledgeInboxPage,
  {
    title: string;
    kicker: string;
    emptyTitle: string;
  }
> = {
  insights: {
    title: "Insights",
    kicker: "Knowledge",
    emptyTitle: "No insights yet"
  },
  "daily-newsletter": {
    title: "Daily Newsletter",
    kicker: "Digest",
    emptyTitle: "No newsletter issues yet"
  },
  "original-x-posts": {
    title: "Original X Bookmarks",
    kicker: "X",
    emptyTitle: "No X bookmarks yet"
  },
  "original-youtube-posts": {
    title: "Original YouTube Videos",
    kicker: "YouTube",
    emptyTitle: "No YouTube videos yet"
  }
};

const initialVisibleCount = 25;
const loadMoreCount = 25;
const summaryPreviewLength = 300;
const quotePreviewLength = 260;

export function SecondBrainConsoleView({ activePage, chatMessages, isAsking, isDigesting, isLoading, model, refreshStatus, onAsk, onDigest, onSendDigest, onFeedback }: SecondBrainConsoleViewProps) {
  const [visibleCount, setVisibleCount] = useState(initialVisibleCount);
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());
  const [digestEmail, setDigestEmail] = useState("");
  const [digestSendMessage, setDigestSendMessage] = useState<string | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const page = pageCopy[activePage];
  const baseItems = useMemo(() => getFeedItems(model, activePage), [model, activePage]);
  const feedItems = useMemo(() => sliceItems(baseItems, visibleCount), [baseItems, visibleCount]);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          setVisibleCount((count) => count + loadMoreCount);
        }
      },
      { rootMargin: "720px 0px" }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [activePage]);

  function toggleCopiedItem(key: string, quote: string, item: FeedItem) {
    setCopiedItems((items) => toggleSetItem(items, key));
    if (navigator.clipboard) {
      void navigator.clipboard.writeText(quote).catch(() => {
        setCopiedItems((items) => toggleSetItem(items, key));
      });
    }
    onFeedback(item.source.toLowerCase(), item.id, "copied", item.sourceUrl);
  }

  async function submitDigestEmail(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const recipient = digestEmail.trim();
    if (!recipient) {
      setDigestSendMessage("Enter an email address.");
      return;
    }
    setDigestSendMessage(null);
    try {
      const digest = await onSendDigest(recipient);
      const delivery = digest.deliveries?.[0];
      if (delivery?.status === "sent") {
        setDigestSendMessage(`Sent latest digest to ${delivery.recipient}.`);
        return;
      }
      setDigestSendMessage(delivery?.error || `Digest delivery ${delivery?.status ?? digest.status}.`);
    } catch (error) {
      setDigestSendMessage(error instanceof Error ? error.message : "Digest delivery failed.");
    }
  }

  return (
    <main className="wall-shell">
      <aside className="wall-sidebar" aria-label="Second Brain navigation">
        <Link className="brand-lockup" href="/">
          <span className="brand-mark">{model.brand.mark}</span>
          <span>
            <strong>{model.brand.name}</strong>
            <small>{model.brand.descriptor}</small>
          </span>
        </Link>

        <nav>
          {model.navigation.map((item) => (
            <NavLink key={item.href} active={isActiveNav(item, activePage)} item={item} />
          ))}
        </nav>
      </aside>

      <section className="wall-workspace">
        <header className="wall-topbar">
          <div>
            <span className="section-label">{page.kicker}</span>
            <h1>{page.title}</h1>
          </div>
          {activePage === "daily-newsletter" ? (
            <div className="digest-email-tools">
              <button className="secondary-action" disabled={isDigesting} onClick={onDigest} type="button">
                {isDigesting ? <span className="button-spinner" aria-hidden="true" /> : null}
                Generate Digest
              </button>
              <form className="digest-email-form" onSubmit={submitDigestEmail}>
                <input
                  aria-label="Digest recipient email"
                  disabled={isDigesting}
                  onChange={(event) => setDigestEmail(event.target.value)}
                  placeholder="reader@example.com"
                  type="email"
                  value={digestEmail}
                />
                <button disabled={isDigesting} type="submit">
                  {isDigesting ? <span className="button-spinner" aria-hidden="true" /> : null}
                  Send Latest
                </button>
              </form>
              {digestSendMessage ? <p role="status">{digestSendMessage}</p> : null}
            </div>
          ) : null}
        </header>

        {model.error ? <div className="error-banner">{model.error}</div> : null}
        {model.header.isRunning || refreshStatus?.status === "running" ? (
          <RefreshProgress status={refreshStatus} />
        ) : null}
        {isLoading ? (
          <div className="loading-strip" role="status">
            <span className="loading-spinner" aria-hidden="true" />
            Loading latest knowledge run
          </div>
        ) : null}

        <div className="wall-layout">
          <section className="feed-column" aria-label={`${page.title} feed`}>
            {feedItems.length ? (
              feedItems.map((item, index) => {
                const itemKey = `${item.id}-${index}`;
                return (
                  <FeedCard
                    key={itemKey}
                    copied={copiedItems.has(itemKey)}
                    index={index}
                    item={item}
                    itemKey={itemKey}
                    onCopy={() => toggleCopiedItem(itemKey, shortText(item.quote ?? item.body, quotePreviewLength), item)}
                  />
                );
              })
            ) : (
              <div className="feed-card empty-feed">
                <span className="feed-source">Source required</span>
                <h2>{page.emptyTitle}</h2>
                <p>No sourced items are available for this view yet. Refresh the inbox after X or YouTube credentials are connected.</p>
              </div>
            )}
            {baseItems.length ? (
              <div ref={sentinelRef} className="feed-sentinel" aria-hidden="true">
                {feedItems.length < baseItems.length ? "Loading more sourced items" : "End of sourced items"}
              </div>
            ) : null}
          </section>

        </div>
      </section>
      <AskSecondBrainWidget isAsking={isAsking} messages={chatMessages} onAsk={onAsk} />
    </main>
  );
}

function RefreshProgress({ status }: { status: RefreshStatus | null }) {
  const elapsed = formatElapsed(status?.elapsedSeconds ?? 0);
  return (
    <div className="refresh-progress" role="status">
      <span className="loading-spinner" aria-hidden="true" />
      <span>
        <strong>{status?.message ?? "Refreshing the inbox."}</strong>
        <small>{elapsed} elapsed{status?.phase ? ` - ${status.phase.replace(/_/g, " ")}` : ""}</small>
      </span>
    </div>
  );
}

function NavLink({ active, item }: { active: boolean; item: NavigationItemViewModel }) {
  return (
    <Link aria-current={active ? "page" : undefined} className={active ? "active" : undefined} href={item.href}>
      {item.label}
    </Link>
  );
}

function FeedCard({
  copied,
  index,
  item,
  itemKey,
  onCopy
}: {
  copied: boolean;
  index: number;
  item: FeedItem;
  itemKey: string;
  onCopy: () => void;
}) {
  const quote = item.quote ? shortText(item.quote, quotePreviewLength) : null;
  return (
    <article aria-labelledby={`${itemKey}-title`} className={`feed-card ${item.source.toLowerCase()}`}>
      <div className="feed-identity">
        <span className={`feed-avatar ${item.source.toLowerCase()}`}>{sourceMark(item.source)}</span>
        <span>{item.timestamp}</span>
        {item.source === "Newsletter" ? <span className="newsletter-chip">{item.eyebrow}</span> : null}
      </div>

      <div className="feed-main">
        <div className="feed-card-topline">
          <div>
            <h2 id={`${itemKey}-title`}>{item.title}</h2>
            <p className="feed-meta">
              {item.author} - {item.timestamp}
            </p>
          </div>
        </div>

        <div className="summary-column">
          <span className={`feed-source ${item.source.toLowerCase()}`}>AI Summary</span>
          <p>{shortText(item.body, summaryPreviewLength)}</p>
          {item.timeMarkers?.length ? <TimeMarkerRow markers={item.timeMarkers} sourceUrl={item.sourceUrl} /> : null}
        </div>
      </div>

      <div className="quote-column">
        <div className="feed-actions" aria-label={`Actions for item ${index + 1}`}>
          {item.sourceUrl ? (
            <a href={item.sourceUrl} onClick={(event) => event.stopPropagation()} rel="noreferrer" target="_blank">
              Original
            </a>
          ) : null}
          {quote ? (
            <button
              aria-label="Copy quote"
              className={copied ? "active" : undefined}
              onClick={(event) => {
                event.stopPropagation();
                onCopy();
              }}
              type="button"
            >
              {copied ? "Copied" : "Copy Quote"}
            </button>
          ) : null}
        </div>
        {quote ? (
          <blockquote>
            <span>Quote</span>
            {quote}
          </blockquote>
        ) : null}
        {item.quality?.overall ? <QualityPill quality={item.quality} /> : null}
      </div>
    </article>
  );
}

function TimeMarkerRow({ markers, sourceUrl }: { markers: ImportantTimeMarker[]; sourceUrl?: string }) {
  return (
    <div className="time-marker-row" aria-label="Important time markers">
      {markers.slice(0, 3).map((marker) => {
        const href = sourceUrl && marker.seconds !== undefined ? `${sourceUrl}${sourceUrl.includes("?") ? "&" : "?"}t=${marker.seconds}s` : sourceUrl;
        const label = `${marker.timestamp} ${marker.label}`;
        return href ? (
          <a key={`${marker.timestamp}-${marker.label}`} href={href} rel="noreferrer" target="_blank" title={marker.whyItMatters}>
            {label}
          </a>
        ) : (
          <span key={`${marker.timestamp}-${marker.label}`} title={marker.whyItMatters}>
            {label}
          </span>
        );
      })}
    </div>
  );
}

function QualityPill({ quality }: { quality: QualityScore }) {
  const score = Math.round((quality.overall ?? 0) * 100);
  return (
    <span className="quality-pill" title={quality.rationale || quality.verdict || "LLM quality judge score"}>
      Q {score}
    </span>
  );
}

function AskSecondBrainWidget({
  isAsking,
  messages,
  onAsk
}: {
  isAsking: boolean;
  messages: ChatMessage[];
  onAsk: (question: string, useLatest?: boolean) => Promise<void>;
}) {
  const [open, setOpen] = useState(false);
  const [question, setQuestion] = useState("");
  const [useLatest, setUseLatest] = useState(false);

  function submitQuestion(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmed = question.trim();
    if (!trimmed || isAsking) return;
    setQuestion("");
    void onAsk(trimmed, useLatest);
  }

  return (
    <section className={`ask-brain ${open ? "open" : ""}`} aria-label="Ask Your Second Brain">
      {open ? (
        <div className="ask-panel">
          <header>
            <span>
              <strong>Ask Your Second Brain</strong>
              <small>Answers cite saved X bookmarks, YouTube videos, and knowledge graph signals.</small>
            </span>
            <button aria-label="Close Ask Your Second Brain" onClick={() => setOpen(false)} type="button">
              ×
            </button>
          </header>
          <div className="ask-messages">
            {messages.length ? (
              messages.map((message) => (
                <div key={message.id} className={`ask-message ${message.role}`}>
                  <p>{formatChatContent(message.content)}</p>
                  {message.sources?.length ? (
                    <ul>
                      {message.sources.map((source) => (
                        <li key={`${message.id}-${source.id}`}>
                          {source.sourceUrl ? (
                            <a href={source.sourceUrl} rel="noreferrer" target="_blank">
                              {source.id}: {source.title}
                            </a>
                          ) : (
                            <span>
                              {source.id}: {source.title}
                            </span>
                          )}
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </div>
              ))
            ) : (
              <div className="ask-empty">
                Ask about a saved source, a repeated idea, or what your bookmarks suggest you should do next.
              </div>
            )}
            {isAsking ? (
              <div className="ask-message assistant pending">
                <span className="loading-spinner" aria-hidden="true" />
                Thinking with your knowledge base
              </div>
            ) : null}
          </div>
          <form onSubmit={submitQuestion}>
            <label className="latest-toggle">
              <input checked={useLatest} onChange={(event) => setUseLatest(event.target.checked)} type="checkbox" />
              Use latest web context
            </label>
            <div className="ask-input-row">
              <input
                aria-label="Question for Ask Your Second Brain"
                maxLength={1200}
                onChange={(event) => setQuestion(event.target.value)}
                placeholder="Ask about your insights..."
                value={question}
              />
              <button disabled={isAsking || !question.trim()} type="submit">
                Ask
              </button>
            </div>
          </form>
        </div>
      ) : null}
      {open ? null : (
        <button className="ask-launcher" onClick={() => setOpen(true)} type="button">
          Ask Your Second Brain
        </button>
      )}
    </section>
  );
}

function toggleSetItem<T>(items: Set<T>, item: T) {
  const next = new Set(items);
  if (next.has(item)) {
    next.delete(item);
  } else {
    next.add(item);
  }
  return next;
}

function formatElapsed(seconds: number) {
  const safeSeconds = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safeSeconds / 60);
  const rest = safeSeconds % 60;
  if (minutes === 0) return `${rest}s`;
  return `${minutes}m ${String(rest).padStart(2, "0")}s`;
}

function formatChatContent(value: string) {
  return value
    .replace(/\*\*/g, "")
    .replace(/^#{1,4}\s+/gm, "")
    .replace(/^\s*-\s+/gm, "• ");
}

function sourceMark(source: FeedSource) {
  if (source === "X") return "X";
  if (source === "YouTube") return "Y";
  if (source === "Newsletter") return "N";
  if (source === "Insight") return "I";
  if (source === "Action") return "A";
  if (source === "Quote") return "Q";
  return "S";
}

function getFeedItems(model: KnowledgeInboxViewModel, activePage: KnowledgeInboxPage): FeedItem[] {
  const summaries = model.summaries.items.map(summaryToFeedItem);
  const insightItems = model.summaries.items
    .filter((item) => item.source === "insight")
    .map(summaryToFeedItem);
  const xPosts = model.intake.items
    .filter((item) => item.source === "X")
    .map((item): FeedItem => ({
      id: item.id,
      source: "X",
      eyebrow: item.author,
      title: item.item,
      body: item.body,
      quote: item.body.length > 130 ? `${item.body.slice(0, 130).trim()}...` : item.body,
      author: item.author,
      timestamp: item.timestamp,
      sourceUrl: item.sourceUrl,
      stats: item.stats ?? item.status
    }));
  const youtubePosts = model.transcripts.items.map((item): FeedItem => ({
    id: item.id,
    source: "YouTube",
    eyebrow: item.statusLabel,
    title: item.title,
    body: item.detail,
    quote: item.detail.length > 150 ? `${item.detail.slice(0, 150).trim()}...` : item.detail,
    author: item.author,
    timestamp: item.timestamp,
    sourceUrl: item.sourceUrl,
    stats: item.statusLabel,
    timeMarkers: item.timeMarkers
  }));

  if (activePage === "insights") return insightItems;
  if (activePage === "original-x-posts") return xPosts;
  if (activePage === "original-youtube-posts") return youtubePosts;

  return buildNewsletterItems(model.digest, summaries);
}

function summaryToFeedItem(summary: SummaryCardViewModel): FeedItem {
  const isX = summary.id.startsWith("x-");
  const source = summary.source === "insight" ? "Insight" : summary.source === "action" ? "Action" : summary.quote ? "Quote" : "Summary";
  const eyebrow = summary.source === "insight" ? "Insight" : summary.source === "action" ? "Action item" : isX ? "X" : "YouTube";
  return {
    id: summary.id,
    source,
    eyebrow,
    title: summary.title,
    body: summary.body,
    quote: summary.quote,
    author: isX ? "X" : "YouTube",
    timestamp: "Summary",
    sourceUrl: summary.sourceUrl,
    stats: `${summary.decisionLabel} - ${summary.confidenceLabel}${summary.cacheStatus ? ` - ${summary.cacheStatus}` : ""}`,
    quality: summary.quality,
    timeMarkers: summary.timeMarkers
  };
}

function buildNewsletterItems(digest: KnowledgeInboxViewModel["digest"], items: FeedItem[]): FeedItem[] {
  const supportingItems = items.slice(0, 8).map((item, index) => ({
    ...item,
    id: `newsletter-${item.id}`,
    source: "Newsletter" as const,
    eyebrow: `Source ${index + 1}`,
    title: item.title,
    body: item.body,
    stats: "linked source note"
  }));
  if (!digest) return supportingItems;
  const lines = digest.bodyMarkdown.split("\n");
  return [
    {
      id: `digest-${digest.digestDate}`,
      source: "Newsletter" as const,
      eyebrow: digest.status,
      title: digest.subject,
      body: firstNewsletterParagraph(lines),
      author: "Second Brain",
      timestamp: digest.digestDate,
      stats: `${digest.status} - ${digest.scheduledFor}`
    },
    ...supportingItems
  ];
}

function firstNewsletterParagraph(lines: string[]) {
  const paragraph = lines
    .map((line) => line.trim())
    .find((line) => line && !line.startsWith("#") && !line.startsWith("- "));
  return paragraph ? markdownToPlain(paragraph) : "A mobile-first, source-linked digest from the latest knowledge run.";
}

function markdownToPlain(value: string) {
  return value
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, "$1")
    .replace(/\*\*/g, "")
    .trim();
}

function shortText(value: string, maxLength: number) {
  const plain = markdownToPlain(value).replace(/\s+/g, " ").trim();
  if (plain.length <= maxLength) return plain;
  return `${plain.slice(0, maxLength).trim()}...`;
}

function sliceItems(items: FeedItem[], visibleCount: number) {
  return items.slice(0, visibleCount);
}

function isActiveNav(item: NavigationItemViewModel, activePage: KnowledgeInboxPage) {
  return (
    (activePage === "insights" && (item.href === "/" || item.href === "/insights")) ||
    (activePage === "daily-newsletter" && item.href === "/daily-newsletter") ||
    (activePage === "original-x-posts" && (item.href === "/original-x-posts" || item.href === "/original-x-bookmarks")) ||
    (activePage === "original-youtube-posts" && (item.href === "/original-youtube-posts" || item.href === "/original-youtube-videos"))
  );
}
