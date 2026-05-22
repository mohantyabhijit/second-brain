"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { KnowledgeInboxViewModel, NavigationItemViewModel, SummaryCardViewModel } from "../presentation/viewModel";
import { Icon } from "./primitives/Icon";

type SecondBrainConsoleViewProps = {
  activePage: KnowledgeInboxPage;
  model: KnowledgeInboxViewModel;
  onRun: () => void;
};

type FeedSource = "Summary" | "Quote" | "X" | "YouTube" | "Newsletter";

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
};

type ExpandablePanelKey = "digest" | "topics" | "sources" | "blockers" | "sidebar-note";

type TopicRow = {
  key: string;
  label: string;
  value: string;
  dot: "blue" | "green" | "amber";
};

const pageCopy: Record<
  KnowledgeInboxPage,
  {
    title: string;
    description: string;
    kicker: string;
    emptyTitle: string;
  }
> = {
  home: {
    title: "Home",
    description: "A running wall of summaries and quotes from everything entering your second brain.",
    kicker: "Today",
    emptyTitle: "No timeline items yet"
  },
  "daily-newsletter": {
    title: "Daily Newsletter",
    description: "A scrollable digest of the ideas worth revisiting, grouped like a daily briefing.",
    kicker: "Digest",
    emptyTitle: "No newsletter issues yet"
  },
  "original-x-posts": {
    title: "Original X Posts",
    description: "The raw X posts behind the summaries, kept intact for attribution and rereading.",
    kicker: "X",
    emptyTitle: "No X posts yet"
  },
  "original-youtube-posts": {
    title: "Original YouTube Posts",
    description: "The source videos and transcript evidence behind the reading queue.",
    kicker: "YouTube",
    emptyTitle: "No YouTube posts yet"
  }
};

export function SecondBrainConsoleView({ activePage, model, onRun }: SecondBrainConsoleViewProps) {
  const [visibleCount, setVisibleCount] = useState(10);
  const [expandedItems, setExpandedItems] = useState<Set<string>>(new Set());
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());
  const [reviewedItems, setReviewedItems] = useState<Set<string>>(new Set());
  const [savedItems, setSavedItems] = useState<Set<string>>(new Set());
  const [expandedPanels, setExpandedPanels] = useState<Set<ExpandablePanelKey>>(new Set(["digest", "topics", "sources"]));
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const page = pageCopy[activePage];
  const baseItems = useMemo(() => getFeedItems(model, activePage), [model, activePage]);
  const feedItems = useMemo(() => sliceItems(baseItems, visibleCount), [baseItems, visibleCount]);
  const topics = useMemo(() => deriveTopicRows(baseItems), [baseItems]);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          setVisibleCount((count) => count + 8);
        }
      },
      { rootMargin: "720px 0px" }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [activePage]);

  function toggleExpandedItem(key: string) {
    setExpandedItems((items) => toggleSetItem(items, key));
  }

  function toggleCopiedItem(key: string, quote: string) {
    setCopiedItems((items) => toggleSetItem(items, key));
    if (navigator.clipboard) {
      void navigator.clipboard.writeText(quote).catch(() => {
        setCopiedItems((items) => toggleSetItem(items, key));
      });
    }
  }

  function toggleReviewedItem(key: string) {
    setReviewedItems((items) => toggleSetItem(items, key));
  }

  function toggleSavedItem(key: string) {
    setSavedItems((items) => toggleSetItem(items, key));
  }

  function togglePanel(key: ExpandablePanelKey) {
    setExpandedPanels((items) => toggleSetItem(items, key));
  }

  function toggleRow(key: string) {
    setExpandedRows((items) => toggleSetItem(items, key));
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

        <button
          aria-expanded={expandedPanels.has("sidebar-note")}
          className="sidebar-note interactive-panel-trigger"
          onClick={() => togglePanel("sidebar-note")}
          type="button"
        >
          <span>{model.sidebarNote.label}</span>
          <strong>{model.sidebarNote.value}</strong>
          {expandedPanels.has("sidebar-note") ? (
            <small>Feeds stay source-grounded and preserve the original trail for every quote.</small>
          ) : null}
        </button>
      </aside>

      <section className="wall-workspace">
        <header className="wall-topbar">
          <div>
            <span className="section-label">{page.kicker}</span>
            <h1>{page.title}</h1>
            <p>{page.description}</p>
          </div>
          <button className="primary-action" disabled={model.header.isRunning} onClick={onRun} type="button">
            <Icon name="run" />
            {model.header.actionLabel}
          </button>
        </header>

        {model.error ? <div className="error-banner">{model.error}</div> : null}

        <div className="wall-layout">
          <section className="feed-column" aria-label={`${page.title} feed`}>
            {feedItems.length ? (
              feedItems.map((item, index) => {
                const itemKey = `${item.id}-${index}`;
                return (
                  <FeedCard
                    key={itemKey}
                    copied={copiedItems.has(itemKey)}
                    expanded={expandedItems.has(itemKey)}
                    index={index}
                    item={item}
                    itemKey={itemKey}
                    reviewed={reviewedItems.has(itemKey)}
                    saved={savedItems.has(itemKey)}
                    onCopy={() => toggleCopiedItem(itemKey, item.quote ?? item.body)}
                    onExpand={() => toggleExpandedItem(itemKey)}
                    onReview={() => toggleReviewedItem(itemKey)}
                    onSave={() => toggleSavedItem(itemKey)}
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

          <aside className="context-rail" aria-label="Daily context">
            <div className="rail-panel digest-panel">
              <button
                aria-expanded={expandedPanels.has("digest")}
                className="rail-heading"
                onClick={() => togglePanel("digest")}
                type="button"
              >
                <span>
                  <span className="section-label">Today</span>
                  <strong>Daily Digest</strong>
                </span>
                <span className="expand-glyph">{expandedPanels.has("digest") ? "Hide" : "Show"}</span>
              </button>
              {expandedPanels.has("digest") ? (
                <dl>
                  {model.metrics.map((metric) => {
                    const rowKey = `metric-${metric.label}`;
                    return (
                      <div key={metric.label} className={expandedRows.has(rowKey) ? "expanded" : undefined}>
                        <button aria-expanded={expandedRows.has(rowKey)} onClick={() => toggleRow(rowKey)} type="button">
                          <dt>{metric.label}</dt>
                          <dd>{metric.value}</dd>
                        </button>
                        {expandedRows.has(rowKey) ? <p>{metricDetail(metric.label, metric.value)}</p> : null}
                      </div>
                    );
                  })}
                </dl>
              ) : null}
            </div>

            <div className="rail-panel topic-panel">
              <button
                aria-expanded={expandedPanels.has("topics")}
                className="rail-heading"
                onClick={() => togglePanel("topics")}
                type="button"
              >
                <span>
                  <span className="section-label">Top topics</span>
                  <strong>Review lanes</strong>
                </span>
                <span className="expand-glyph">{expandedPanels.has("topics") ? "Hide" : "Show"}</span>
              </button>
              {expandedPanels.has("topics") ? (
                topics.length ? (
                  <ul>
                    {topics.map((topic) => {
                      const rowKey = `topic-${topic.key}`;
                      return (
                        <li key={topic.key} className={expandedRows.has(rowKey) ? "expanded" : undefined}>
                          <button aria-expanded={expandedRows.has(rowKey)} onClick={() => toggleRow(rowKey)} type="button">
                            <span className={`topic-dot ${topic.dot}`} />
                            <span>{topic.label}</span>
                            <strong>{topic.value}</strong>
                          </button>
                          {expandedRows.has(rowKey) ? <p>{topic.label} appears in {topic.value} sourced feed items.</p> : null}
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <p className="rail-empty">Topics will appear after real source items are loaded.</p>
                )
              ) : null}
            </div>

            <div className="rail-panel source-panel">
              <button
                aria-expanded={expandedPanels.has("sources")}
                className="rail-heading"
                onClick={() => togglePanel("sources")}
                type="button"
              >
                <span>
                  <span className="section-label">Sources</span>
                  <strong>Inbox health</strong>
                </span>
                <span className="expand-glyph">{expandedPanels.has("sources") ? "Hide" : "Show"}</span>
              </button>
              {expandedPanels.has("sources") ? (
                <ul>
                  {model.sources.map((source) => {
                    const rowKey = `source-${source.label}`;
                    return (
                      <li key={source.label} className={`${source.status} ${expandedRows.has(rowKey) ? "expanded" : ""}`}>
                        <button aria-expanded={expandedRows.has(rowKey)} onClick={() => toggleRow(rowKey)} type="button">
                          <span className="source-dot" />
                          <span>
                            <strong>{source.label}</strong>
                            <small>{source.statusLabel}</small>
                          </span>
                        </button>
                        {expandedRows.has(rowKey) ? <p>{source.detail}. Status: {source.statusLabel}.</p> : null}
                      </li>
                    );
                  })}
                </ul>
              ) : null}
            </div>

            {model.blockers.items.length ? (
              <div className="rail-panel blocker-panel">
                <button
                  aria-expanded={expandedPanels.has("blockers")}
                  className="rail-heading"
                  onClick={() => togglePanel("blockers")}
                  type="button"
                >
                  <span>
                    <span className="section-label">{model.blockers.eyebrow}</span>
                    <strong>{model.blockers.title}</strong>
                  </span>
                  <span className="expand-glyph">{expandedPanels.has("blockers") ? "Hide" : "Show"}</span>
                </button>
                {expandedPanels.has("blockers")
                  ? model.blockers.items.map((blocker) => (
                      <button key={blocker} className="blocker-row" onClick={() => toggleRow(`blocker-${blocker}`)} type="button">
                        <span>{blocker}</span>
                        {expandedRows.has(`blocker-${blocker}`) ? <small>Refresh after provider credentials are available.</small> : null}
                      </button>
                    ))
                  : null}
              </div>
            ) : null}
          </aside>
        </div>
      </section>
    </main>
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
  expanded,
  index,
  item,
  itemKey,
  reviewed,
  saved,
  onCopy,
  onExpand,
  onReview,
  onSave
}: {
  copied: boolean;
  expanded: boolean;
  index: number;
  item: FeedItem;
  itemKey: string;
  reviewed: boolean;
  saved: boolean;
  onCopy: () => void;
  onExpand: () => void;
  onReview: () => void;
  onSave: () => void;
}) {
  return (
    <article
      aria-labelledby={`${itemKey}-title`}
      className={`feed-card ${expanded ? "expanded" : ""} ${reviewed ? "reviewed" : ""}`}
      onClick={onExpand}
    >
      <button
        aria-expanded={expanded}
        aria-label={`${expanded ? "Collapse" : "Expand"} ${item.title}`}
        className="feed-identity"
        onClick={(event) => {
          event.stopPropagation();
          onExpand();
        }}
        type="button"
      >
        <span className={`feed-avatar ${item.source.toLowerCase()}`}>{sourceMark(item.source)}</span>
        <span>{item.timestamp}</span>
      </button>

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
          <span className={`feed-source ${item.source.toLowerCase()}`}>{item.source === "Newsletter" ? "Summary" : item.source}</span>
          <p>{item.body}</p>
        </div>
      </div>

      <div className="quote-column">
        <div className="feed-actions" aria-label={`Actions for item ${index + 1}`}>
          {item.sourceUrl ? (
            <a
              href={item.sourceUrl}
              onClick={(event) => event.stopPropagation()}
              rel="noreferrer"
              target="_blank"
            >
              Open Source
            </a>
          ) : null}
          <button
            className={saved ? "active" : undefined}
            onClick={(event) => {
              event.stopPropagation();
              onSave();
            }}
            type="button"
          >
            {saved ? "Saved" : "Save"}
          </button>
          <button
            className={copied ? "active" : undefined}
            onClick={(event) => {
              event.stopPropagation();
              onCopy();
            }}
            type="button"
          >
            {copied ? "Copied" : "Copy Quote"}
          </button>
          <button
            className={reviewed ? "active" : undefined}
            onClick={(event) => {
              event.stopPropagation();
              onReview();
            }}
            type="button"
          >
            {reviewed ? "Reviewed" : "Review"}
          </button>
        </div>
        {item.quote ? (
          <blockquote>
            <span>Quote</span>
            {item.quote}
          </blockquote>
        ) : null}
        {item.stats ? <div className="feed-stats">{item.stats}</div> : null}
        {expanded ? (
          <div className="feed-expanded">
            <span>{item.eyebrow}</span>
            <p>{expandedDetail(item)}</p>
          </div>
        ) : null}
      </div>
    </article>
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

function sourceMark(source: FeedSource) {
  if (source === "X") return "X";
  if (source === "YouTube") return "Y";
  if (source === "Newsletter") return "N";
  if (source === "Quote") return "Q";
  return "S";
}

function expandedDetail(item: FeedItem) {
  if (item.source === "X") return "Original post view preserves author, source text, quote, and engagement context for attribution.";
  if (item.source === "YouTube") return "Video view preserves transcript status, channel context, and the quote that supports the summary.";
  if (item.source === "Newsletter") return "Newsletter view groups related summary and quote material into a digest-ready issue.";
  return "Expanded view keeps the short summary, quote, source context, and review decision together.";
}

function metricDetail(label: string, value: string) {
  if (label === "Captured items") return `${value} total items are currently available from the connected sources.`;
  if (label === "Summaries") return `${value} summaries are ready for review in this run.`;
  if (label === "Transcripts") return `${value} YouTube transcripts are currently usable for evidence.`;
  return `${value} blockers need attention before the inbox can be trusted end to end.`;
}

function getFeedItems(model: KnowledgeInboxViewModel, activePage: KnowledgeInboxPage): FeedItem[] {
  const summaries = model.summaries.items.map(summaryToFeedItem);
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
    stats: item.statusLabel
  }));

  const homeItems = [...summaries, ...xPosts, ...youtubePosts];

  if (activePage === "home") return homeItems;
  if (activePage === "original-x-posts") return xPosts;
  if (activePage === "original-youtube-posts") return youtubePosts;

  return buildNewsletterItems(summaries);
}

function summaryToFeedItem(summary: SummaryCardViewModel): FeedItem {
  const isX = summary.id.startsWith("x-");
  return {
    id: summary.id,
    source: summary.quote ? "Quote" : "Summary",
    eyebrow: isX ? "X" : "YouTube",
    title: summary.title,
    body: summary.body,
    quote: summary.quote,
    author: isX ? "X" : "YouTube",
    timestamp: "Summary",
    sourceUrl: summary.sourceUrl,
    stats: `${summary.decisionLabel} - ${summary.confidenceLabel}`
  };
}

function buildNewsletterItems(items: FeedItem[]): FeedItem[] {
  return items.map((item, index) => ({
    ...item,
    id: `newsletter-${item.id}`,
    source: "Newsletter",
    eyebrow: `Issue ${index + 1}`,
    title: item.title,
    body: item.body,
    stats: "newsletter candidate"
  }));
}

function sliceItems(items: FeedItem[], visibleCount: number) {
  return items.slice(0, visibleCount);
}

function deriveTopicRows(items: FeedItem[]): TopicRow[] {
  const topicMatchers: Array<{ key: string; label: string; dot: TopicRow["dot"]; patterns: RegExp[] }> = [
    { key: "ai", label: "AI", dot: "blue", patterns: [/\bai\b/i, /artificial intelligence/i, /model/i, /llm/i] },
    { key: "productivity", label: "Productivity", dot: "green", patterns: [/productivity/i, /workflow/i, /habit/i, /review/i] },
    { key: "systems", label: "Systems Thinking", dot: "amber", patterns: [/system/i, /process/i, /architecture/i, /workflow/i] }
  ];

  return topicMatchers
    .map((topic) => {
      const value = items.filter((item) => {
        const text = `${item.title} ${item.body} ${item.quote ?? ""}`;
        return topic.patterns.some((pattern) => pattern.test(text));
      }).length;

      return {
        key: topic.key,
        label: topic.label,
        value: String(value),
        dot: topic.dot
      };
    })
    .filter((topic) => topic.value !== "0");
}

function isActiveNav(item: NavigationItemViewModel, activePage: KnowledgeInboxPage) {
  return (
    (activePage === "home" && item.href === "/") ||
    (activePage === "daily-newsletter" && item.href === "/daily-newsletter") ||
    (activePage === "original-x-posts" && item.href === "/original-x-posts") ||
    (activePage === "original-youtube-posts" && item.href === "/original-youtube-posts")
  );
}
