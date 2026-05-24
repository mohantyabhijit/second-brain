"use client";

import { type FormEvent, type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { ChatMessage } from "../model/useKnowledgeInboxController";
import type { KnowledgeInboxViewModel, NavigationItemViewModel, SummaryCardViewModel } from "../presentation/viewModel";
import type { DigestIssue, FeedbackSignal, ImportantTimeMarker, RefreshStatus } from "../contracts";
type SecondBrainConsoleViewProps = {
  activePage: KnowledgeInboxPage;
  chatMessages: ChatMessage[];
  digestIssues: DigestIssue[];
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
  timeMarkers?: ImportantTimeMarker[];
  fullBody?: string;
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
const summaryPreviewLength = 220;
const quotePreviewLength = 260;

export function SecondBrainConsoleView({ activePage, chatMessages, digestIssues, isAsking, isDigesting, isLoading, model, refreshStatus, onAsk, onDigest, onSendDigest, onFeedback }: SecondBrainConsoleViewProps) {
  const [visibleCount, setVisibleCount] = useState(initialVisibleCount);
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());
  const [digestEmail, setDigestEmail] = useState("");
  const [digestSendMessage, setDigestSendMessage] = useState<string | null>(null);
  const [openSummaryItem, setOpenSummaryItem] = useState<FeedItem | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const page = pageCopy[activePage];
  const baseItems = useMemo(() => getFeedItems(model, activePage, digestIssues), [model, activePage, digestIssues]);
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
                    onOpen={() => setOpenSummaryItem(item)}
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
      {openSummaryItem ? <SummaryModal item={openSummaryItem} onClose={() => setOpenSummaryItem(null)} /> : null}
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
  onCopy,
  onOpen
}: {
  copied: boolean;
  index: number;
  item: FeedItem;
  itemKey: string;
  onCopy: () => void;
  onOpen: () => void;
}) {
  const quote = item.quote ? shortText(item.quote, quotePreviewLength) : null;
  const meta = feedMeta(item);
  const identity = feedIdentity(item);
  return (
    <article
      aria-label={`Open AI summary for ${item.title}`}
      aria-labelledby={`${itemKey}-title`}
      className={`feed-card ${item.source.toLowerCase()}`}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      role="button"
      tabIndex={0}
    >
      <div className="feed-identity">
        <span className={`feed-avatar ${item.source.toLowerCase()}`}>{sourceMark(item.source)}</span>
        {identity ? <span>{identity}</span> : null}
      </div>

      <div className="feed-main">
        <div className="feed-card-topline">
          <div>
            <h2 id={`${itemKey}-title`}>
              {item.sourceUrl ? (
                <a className="feed-title-link" href={item.sourceUrl} onClick={(event) => event.stopPropagation()} rel="noreferrer" target="_blank">
                  {item.title}
                </a>
              ) : (
                item.title
              )}
            </h2>
            {meta ? <p className="feed-meta">{meta}</p> : null}
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
              Original Post
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
      </div>
    </article>
  );
}

function SummaryModal({ item, onClose }: { item: FeedItem; onClose: () => void }) {
  return (
    <div className="summary-modal-backdrop" onClick={onClose} role="presentation">
      <section
        aria-labelledby="summary-modal-title"
        aria-modal="true"
        className="summary-modal"
        onClick={(event) => event.stopPropagation()}
        role="dialog"
      >
        <header>
          <div>
            <span className={`feed-source ${item.source.toLowerCase()}`}>AI Summary</span>
            <h2 id="summary-modal-title">
              {item.sourceUrl ? (
                <a href={item.sourceUrl} rel="noreferrer" target="_blank">
                  {item.title}
                </a>
              ) : (
                item.title
              )}
            </h2>
            {feedMeta(item) ? <p className="feed-meta">{feedMeta(item)}</p> : null}
          </div>
          <button aria-label="Close AI summary" onClick={onClose} type="button">
            Close
          </button>
        </header>
        <div className="summary-modal-body">
          {item.source === "Newsletter" && item.fullBody ? <NewsletterBody markdown={item.fullBody} /> : <p>{markdownToPlain(item.body)}</p>}
          {item.quote ? (
            <blockquote>
              <span>Quote</span>
              {shortText(item.quote, quotePreviewLength)}
            </blockquote>
          ) : null}
          {item.timeMarkers?.length ? <TimeMarkerRow markers={item.timeMarkers} sourceUrl={item.sourceUrl} /> : null}
        </div>
      </section>
    </div>
  );
}

function NewsletterBody({ markdown }: { markdown: string }) {
  const blocks = markdown
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);

  return (
    <div className="newsletter-reader">
      {blocks.map((line, index) => {
        const key = `${index}-${line.slice(0, 24)}`;
        if (line.startsWith("# ")) {
          return (
            <h3 key={key} className="newsletter-title">
              {renderInlineMarkdown(line.slice(2))}
            </h3>
          );
        }
        if (line.startsWith("## ")) {
          return <h3 key={key}>{renderInlineMarkdown(line.slice(3))}</h3>;
        }
        if (line.startsWith("### ")) {
          return <h4 key={key}>{renderInlineMarkdown(line.slice(4))}</h4>;
        }
        if (line.startsWith("- ")) {
          return <p key={key}>{renderInlineMarkdown(`- ${line.slice(2)}`)}</p>;
        }
        return <p key={key}>{renderInlineMarkdown(line)}</p>;
      })}
    </div>
  );
}

function renderInlineMarkdown(value: string) {
  const parts: ReactNode[] = [];
  const linkPattern = /\[([^\]]+)\]\(([^)]+)\)/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = linkPattern.exec(value)) !== null) {
    if (match.index > lastIndex) {
      parts.push(stripInlineMarks(value.slice(lastIndex, match.index)));
    }
    parts.push(
      <a key={`${match[2]}-${match.index}`} href={match[2]} rel="noreferrer" target="_blank">
        {stripInlineMarks(match[1])}
      </a>
    );
    lastIndex = match.index + match[0].length;
  }
  if (lastIndex < value.length) {
    parts.push(stripInlineMarks(value.slice(lastIndex)));
  }
  return parts;
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
              <small>Ask across saved X bookmarks, YouTube videos, and knowledge graph signals.</small>
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

function feedIdentity(item: FeedItem) {
  if (item.timestamp === "Summary") return "";
  return item.timestamp;
}

function feedMeta(item: FeedItem) {
  if (item.timestamp === "Summary") return "";
  return `${item.author} - ${item.timestamp}`;
}

function getFeedItems(model: KnowledgeInboxViewModel, activePage: KnowledgeInboxPage, digestIssues: DigestIssue[]): FeedItem[] {
  const summaries = model.summaries.items.map(summaryToFeedItem);
  const summariesBySourceUrl = new Map(
    model.summaries.items
      .filter((item) => item.source === "x" || item.source === "youtube")
      .map((item) => [item.sourceUrl, item])
  );
  const insightItems = model.summaries.items
    .filter((item) => item.source === "insight")
    .map(summaryToFeedItem);
  const xPosts = model.intake.items
    .filter((item) => item.source === "X")
    .map((item): FeedItem => {
      const synthesis = summariesBySourceUrl.get(item.sourceUrl);
      return {
        id: item.id,
        source: "X",
        eyebrow: item.author,
        title: synthesis?.title ?? item.item,
        body: synthesis?.body ?? item.body,
        quote: synthesis?.quote ?? (item.body.length > 130 ? `${item.body.slice(0, 130).trim()}...` : item.body),
        author: item.author,
        timestamp: item.timestamp,
        sourceUrl: item.sourceUrl,
        stats: item.stats ?? item.status
      };
    });
  const youtubePosts = model.transcripts.items.map((item): FeedItem => ({
    id: item.id,
    source: "YouTube",
    eyebrow: item.statusLabel,
    title: summariesBySourceUrl.get(item.sourceUrl)?.title ?? item.title,
    body: summariesBySourceUrl.get(item.sourceUrl)?.body ?? item.detail,
    quote: summariesBySourceUrl.get(item.sourceUrl)?.quote ?? (item.detail.length > 150 ? `${item.detail.slice(0, 150).trim()}...` : item.detail),
    author: item.author,
    timestamp: item.timestamp,
    sourceUrl: item.sourceUrl,
    stats: item.statusLabel,
    timeMarkers: item.timeMarkers?.length ? item.timeMarkers : summariesBySourceUrl.get(item.sourceUrl)?.timeMarkers
  }));

  if (activePage === "insights") return insightItems;
  if (activePage === "original-x-posts") return xPosts;
  if (activePage === "original-youtube-posts") return youtubePosts;

  return buildNewsletterItems(model.digest, digestIssues, summaries);
}

function summaryToFeedItem(summary: SummaryCardViewModel): FeedItem {
  const source = summary.source === "insight" ? "Insight" : summary.source === "action" ? "Action" : summary.quote ? "Quote" : "Summary";
  const eyebrow = summary.source === "insight" ? "Insight" : summary.source === "action" ? "Action item" : summary.author;
  return {
    id: summary.id,
    source,
    eyebrow,
    title: summary.title,
    body: summary.body,
    quote: summary.quote,
    author: summary.author,
    timestamp: "Summary",
    sourceUrl: summary.sourceUrl,
    stats: `${summary.decisionLabel} - ${summary.confidenceLabel}${summary.cacheStatus ? ` - ${summary.cacheStatus}` : ""}`,
    timeMarkers: summary.timeMarkers
  };
}

function buildNewsletterItems(digest: KnowledgeInboxViewModel["digest"], digestIssues: DigestIssue[], items: FeedItem[]): FeedItem[] {
  const supportingItems = items.slice(0, 8).map((item, index) => ({
    ...item,
    id: `newsletter-${item.id}`,
    source: "Newsletter" as const,
    eyebrow: `Source ${index + 1}`,
    title: item.title,
    body: item.body,
    stats: "linked source note"
  }));
  const issues = digestIssues.length ? digestIssues : digest ? [digest] : [];
  const issueItems = issues.map((issue) => {
    const lines = issue.bodyMarkdown.split("\n");
    return {
      id: `digest-${issue.id ?? issue.idempotencyKey ?? issue.digestDate}`,
      source: "Newsletter" as const,
      eyebrow: issue.status,
      title: issue.subject,
      body: firstNewsletterParagraph(lines),
      author: "Abhijit's Second Brain",
      timestamp: issue.digestDate,
      stats: `${issue.status} - ${issue.scheduledFor}`,
      fullBody: issue.bodyMarkdown
    };
  });
  return [...issueItems, ...supportingItems];
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

function stripInlineMarks(value: string) {
  return value.replace(/\*\*/g, "");
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
