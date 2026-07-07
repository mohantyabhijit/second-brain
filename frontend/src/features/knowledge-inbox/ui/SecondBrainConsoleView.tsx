"use client";

import { type FormEvent, type ReactNode, useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { KnowledgeInboxViewModel, NavigationItemViewModel, SummaryCardViewModel } from "../presentation/viewModel";
import type { AskSecondBrainSource, DigestIssue, ImportantTimeMarker, InsightGraphResponse, RefreshStatus } from "../contracts";
import { askSecondBrain } from "../api/knowledgeRuns";
import { KnowledgeGraphView } from "./KnowledgeGraphView";
import { formatUsername, publicOwnerHandle } from "./authDisplay";
type SecondBrainConsoleViewProps = {
  activePage: KnowledgeInboxPage;
  digestIssues: DigestIssue[];
  hasMorePageItems: boolean;
  isLoading: boolean;
  isLoadingMore: boolean;
  insightGraph: InsightGraphResponse | null;
  model: KnowledgeInboxViewModel;
  onLoadMoreItems: () => void;
  refreshStatus: RefreshStatus | null;
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
  sortTimestamp?: string;
  stats?: string;
  timeMarkers?: ImportantTimeMarker[];
  fullBody?: string;
  imageUrl?: string;
  imageAlt?: string;
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
  },
  "knowledge-graph": {
    title: "Knowledge Graph",
    kicker: "Graph",
    emptyTitle: "No graph insights yet"
  }
};

const initialVisibleCount = 25;
const loadMoreCount = 25;
const summaryPreviewLength = 220;
const quotePreviewLength = 260;
const themeStorageKey = "second-brain-theme";

type ThemeMode = "light" | "dark";

export function SecondBrainConsoleView({
  activePage,
  digestIssues,
  hasMorePageItems,
  insightGraph,
  isLoading,
  isLoadingMore,
  model,
  onLoadMoreItems,
  refreshStatus
}: SecondBrainConsoleViewProps) {
  const [visibleCountState, setVisibleCountState] = useState({ page: activePage, query: "", count: initialVisibleCount });
  const [copiedItems, setCopiedItems] = useState<Set<string>>(new Set());
  const [openSummaryItem, setOpenSummaryItem] = useState<FeedItem | null>(null);
  const [sourceSearchState, setSourceSearchState] = useState({ page: activePage, query: "" });
  const sourceSearch = sourceSearchState.page === activePage ? sourceSearchState.query : "";
  const deferredSourceSearch = useDeferredValue(sourceSearch);
  const theme = useThemeMode();
  const feedScrollerRef = useRef<HTMLElement | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const page = pageCopy[activePage];
  const isSourceLoadingPage = isExternalSourcePage(activePage);
  const isNewsletterPage = activePage === "daily-newsletter";
  const isViewportFeedPage = isLazyLoadedFeedPage(activePage);
  const isSearchableFeedPage = isSearchablePage(activePage);
  const baseItems = useMemo(() => getFeedItems(model, activePage, digestIssues), [model, activePage, digestIssues]);
  const normalizedSourceSearch = useMemo(() => normalizeSearchTerm(deferredSourceSearch), [deferredSourceSearch]);
  const searchedItems = useMemo(
    () => (isSearchableFeedPage ? filterFeedItems(baseItems, normalizedSourceSearch) : baseItems),
    [baseItems, isSearchableFeedPage, normalizedSourceSearch]
  );
  const visibleCount =
    visibleCountState.page === activePage && visibleCountState.query === normalizedSourceSearch
      ? visibleCountState.count
      : initialVisibleCount;
  const feedItems = useMemo(() => sliceItems(searchedItems, visibleCount), [searchedItems, visibleCount]);
  const sourceTotal = sourceTotalForPage(model, activePage);
  const hasActiveSourceSearch = isSearchableFeedPage && sourceSearch.trim().length > 0;
  const shouldRenderSentinel = isViewportFeedPage && baseItems.length > 0 && (!hasActiveSourceSearch || feedItems.length > 0);

  useEffect(() => {
    const frame = window.requestAnimationFrame(() => {
      if (feedScrollerRef.current) {
        feedScrollerRef.current.scrollTop = 0;
      }
    });
    return () => window.cancelAnimationFrame(frame);
  }, [activePage, normalizedSourceSearch]);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) return;
    const root = isViewportFeedPage ? feedScrollerRef.current : null;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          if (visibleCount < searchedItems.length) {
            setVisibleCountState((state) => {
              const count =
                state.page === activePage && state.query === normalizedSourceSearch
                  ? state.count
                  : initialVisibleCount;
              return {
                page: activePage,
                query: normalizedSourceSearch,
                count: Math.min(count + loadMoreCount, searchedItems.length)
              };
            });
            return;
          }
          if (hasMorePageItems && !isLoadingMore && (!hasActiveSourceSearch || feedItems.length > 0)) {
            onLoadMoreItems();
          }
        }
      },
      { root, rootMargin: isViewportFeedPage ? "220px 0px" : "720px 0px" }
    );

    observer.observe(sentinel);
    return () => observer.disconnect();
  }, [
    activePage,
    feedItems.length,
    hasActiveSourceSearch,
    hasMorePageItems,
    isLoadingMore,
    isViewportFeedPage,
    normalizedSourceSearch,
    onLoadMoreItems,
    searchedItems.length,
    visibleCount
  ]);

  function updateSourceSearch(query: string) {
    setSourceSearchState({ page: activePage, query });
  }

  function toggleCopiedItem(key: string, quote: string) {
    setCopiedItems((items) => toggleSetItem(items, key));
    if (navigator.clipboard) {
      void navigator.clipboard.writeText(quote).catch(() => {
        setCopiedItems((items) => toggleSetItem(items, key));
      });
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
          <ThemeToggle mode={theme.mode} onToggle={theme.toggle} />
          <IdentityBadge />
        </header>

        {model.error && activePage !== "knowledge-graph" ? <div className="error-banner">{model.error}</div> : null}
        {model.header.isRunning || refreshStatus?.status === "running" ? (
          <RefreshProgress status={refreshStatus} />
        ) : null}
        {isLoading && activePage !== "knowledge-graph" && !isSourceLoadingPage ? (
          <div className="loading-strip" role="status">
            <span className="loading-spinner" aria-hidden="true" />
            Loading latest knowledge run
          </div>
        ) : null}

        <div className="wall-layout">
          {activePage === "knowledge-graph" ? (
            <KnowledgeGraphView graph={insightGraph} isLoading={isLoading} />
          ) : (
            <section
              className={`feed-column${isViewportFeedPage ? " viewport-feed-column" : ""}${isSourceLoadingPage ? " source-feed-column" : ""}${isNewsletterPage ? " newsletter-feed-column" : ""}`}
              aria-label={`${page.title} feed`}
              ref={feedScrollerRef}
            >
              {isSourceLoadingPage && (!isLoading || sourceTotal > 0) ? (
                <SourceFeedSummary
                  activePage={activePage}
                  isLoadingMore={isLoadingMore}
                  loaded={baseItems.length}
                  matched={searchedItems.length}
                  onSearchChange={updateSourceSearch}
                  searchIsStale={sourceSearch !== deferredSourceSearch}
                  searchQuery={sourceSearch}
                  total={sourceTotal}
                  visible={feedItems.length}
                />
              ) : null}
              {isNewsletterPage && (!isLoading || digestIssues.length > 0) ? (
                <NewsletterFeedSummary
                  hasMorePageItems={hasMorePageItems}
                  isLoadingMore={isLoadingMore}
                  loaded={digestIssues.length}
                  matched={searchedItems.length}
                  onSearchChange={updateSourceSearch}
                  searchIsStale={sourceSearch !== deferredSourceSearch}
                  searchQuery={sourceSearch}
                  visible={feedItems.filter((item) => item.source === "Newsletter").length}
                />
              ) : null}
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
                      onCopy={() => toggleCopiedItem(itemKey, shortText(item.quote ?? item.body, quotePreviewLength))}
                      onOpen={() => setOpenSummaryItem(item)}
                      prioritizeImage={activePage === "daily-newsletter" && index === 0}
                    />
                  );
                })
              ) : hasActiveSourceSearch ? (
                <SourceSearchEmpty
                  activePage={activePage}
                  hasMoreSourceItems={hasMorePageItems}
                  isLoadingMore={isLoadingMore}
                  loaded={baseItems.length}
                  onLoadMoreSources={onLoadMoreItems}
                />
              ) : (
                <EmptyFeedState activePage={activePage} emptyTitle={page.emptyTitle} />
              )}
              {shouldRenderSentinel ? (
                <div ref={sentinelRef} className="feed-sentinel">
                  {feedSentinelLabel({
                    activePage,
                    hasActiveSourceSearch,
                    hasMorePageItems,
                    hasMoreVisibleItems: feedItems.length < searchedItems.length,
                    isLoadingMore
                  })}
                </div>
              ) : null}
            </section>
          )}

        </div>
        <footer className="app-footer">
          <p className="app-footer-credit">
            <span>Made with</span>
            <span aria-hidden="true" className="app-footer-heart">
              <HeartIcon />
            </span>
            <span>by</span>
            <a href="https://www.linkedin.com/in/mohantyabhijit/" rel="noreferrer" target="_blank">
              Abhijit Mohanty
            </a>
          </p>
          <div className="coffee-button-slot" aria-label="Support via Stripe">
            <stripe-buy-button
              buy-button-id="buy_btn_1TfXKIRWkJ6QCdjhHqKmr2xi"
              publishable-key="pk_live_51TOyiQRWkJ6QCdjhlLKvkb0gUa5kT8kku8R5A1LzigLjUp5obbdvTvp30e9gSwHacwuuR5yulsAUEpjcF2KrcXeo00DQKYShcF"
            />
          </div>
        </footer>
      </section>
      {openSummaryItem ? <SummaryModal item={openSummaryItem} onClose={() => setOpenSummaryItem(null)} /> : null}
      <HomeLink />
      <AskChatWidget />
    </main>
  );
}

function SourceFeedSummary({
  activePage,
  isLoadingMore,
  loaded,
  matched,
  onSearchChange,
  searchIsStale,
  searchQuery,
  total,
  visible
}: {
  activePage: KnowledgeInboxPage;
  isLoadingMore: boolean;
  loaded: number;
  matched: number;
  onSearchChange: (query: string) => void;
  searchIsStale: boolean;
  searchQuery: string;
  total: number;
  visible: number;
}) {
  const label = activePage === "original-youtube-posts" ? "YouTube sources" : "X bookmarks";
  const safeTotal = Math.max(total, loaded);
  const hasSearch = searchQuery.trim().length > 0;
  const shown = hasSearch ? Math.min(visible, matched) : Math.min(visible, safeTotal);
  const searchLabel = activePage === "original-youtube-posts" ? "Search YouTube sources" : "Search X bookmarks";
  return (
    <div className="source-feed-summary">
      <div className="source-feed-counts">
        <span>{formatNumber(safeTotal)} total {label}</span>
        <small>
          {hasSearch
            ? `${formatNumber(shown)} of ${formatNumber(matched)} matches shown from ${formatNumber(loaded)} loaded`
            : `${formatNumber(shown)} shown${loaded < safeTotal ? `, ${formatNumber(loaded)} loaded` : ""}`}
          {searchIsStale ? " - updating" : ""}
          {isLoadingMore ? " - loading more" : ""}
        </small>
      </div>
      <form className="source-search" onSubmit={(event) => event.preventDefault()} role="search">
        <input
          aria-label={searchLabel}
          autoComplete="off"
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder={activePage === "original-youtube-posts" ? "Search videos" : "Search bookmarks"}
          type="search"
          value={searchQuery}
        />
        {searchQuery ? (
          <button aria-label="Clear source search" onClick={() => onSearchChange("")} type="button">
            Clear
          </button>
        ) : null}
      </form>
    </div>
  );
}

function NewsletterFeedSummary({
  hasMorePageItems,
  isLoadingMore,
  loaded,
  matched,
  onSearchChange,
  searchIsStale,
  searchQuery,
  visible
}: {
  hasMorePageItems: boolean;
  isLoadingMore: boolean;
  loaded: number;
  matched: number;
  onSearchChange: (query: string) => void;
  searchIsStale: boolean;
  searchQuery: string;
  visible: number;
}) {
  const hasSearch = searchQuery.trim().length > 0;
  const shown = hasSearch ? Math.min(visible, matched) : Math.min(visible, loaded);
  return (
    <div className="source-feed-summary newsletter-feed-summary">
      <div className="source-feed-counts">
        <span>Previous newsletters</span>
        <small>
          {hasSearch
            ? `${formatNumber(shown)} of ${formatNumber(matched)} matches shown from ${formatNumber(loaded)} loaded`
            : `${formatNumber(shown)} shown, ${formatNumber(loaded)} loaded`}
          {searchIsStale ? " - updating" : ""}
          {isLoadingMore ? " - loading more" : hasMorePageItems ? " - more available" : loaded ? " - all loaded" : ""}
        </small>
      </div>
      <form className="source-search" onSubmit={(event) => event.preventDefault()} role="search">
        <input
          aria-label="Search newsletter issues"
          autoComplete="off"
          onChange={(event) => onSearchChange(event.target.value)}
          placeholder="Search newsletters"
          type="search"
          value={searchQuery}
        />
        {searchQuery ? (
          <button aria-label="Clear newsletter search" onClick={() => onSearchChange("")} type="button">
            Clear
          </button>
        ) : null}
      </form>
    </div>
  );
}

function EmptyFeedState({ activePage, emptyTitle }: { activePage: KnowledgeInboxPage; emptyTitle: string }) {
  if (isExternalSourcePage(activePage)) {
    const message = activePage === "original-youtube-posts" ? "Loading your YouTube saved videos" : "Loading your X saved posts";
    return (
      <div aria-busy="true" className="feed-card empty-feed source-loading-feed" role="status">
        <span className="source-loading-icon" aria-hidden="true">
          <span className="loading-spinner" />
        </span>
        <span aria-hidden="true" />
        <p>{message}</p>
      </div>
    );
  }

  return (
    <div className="feed-card empty-feed">
      <span className="feed-source">Source required</span>
      <h2>{emptyTitle}</h2>
      <p>No sourced items are available for this view yet. Refresh the inbox after X or YouTube credentials are connected.</p>
    </div>
  );
}

function SourceSearchEmpty({
  activePage,
  hasMoreSourceItems,
  isLoadingMore,
  loaded,
  onLoadMoreSources
}: {
  activePage: KnowledgeInboxPage;
  hasMoreSourceItems: boolean;
  isLoadingMore: boolean;
  loaded: number;
  onLoadMoreSources: () => void;
}) {
  const isNewsletter = activePage === "daily-newsletter";
  const label = isNewsletter ? "newsletter issues" : activePage === "original-youtube-posts" ? "YouTube videos" : "X bookmarks";
  return (
    <div className="feed-card empty-feed source-search-empty">
      <span className="feed-source">No matches</span>
      <h2>No loaded {label} match this search</h2>
      <p>
        {hasMoreSourceItems
          ? `${formatNumber(loaded)} ${isNewsletter ? "issues" : "sources"} are loaded. ${isNewsletter ? "Load previous newsletters" : "Load another batch"} or try a broader query.`
          : "Try a broader query."}
      </p>
      {hasMoreSourceItems ? (
        <button className="source-load-button" disabled={isLoadingMore} onClick={onLoadMoreSources} type="button">
          {isLoadingMore ? "Loading" : isNewsletter ? "Load previous newsletters" : "Load more sources"}
        </button>
      ) : null}
    </div>
  );
}

function useThemeMode() {
  const [mode, setMode] = useState<ThemeMode>("light");

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    const frame = window.requestAnimationFrame(() => {
      const stored = window.localStorage.getItem(themeStorageKey);
      if (stored === "light" || stored === "dark") {
        setMode(stored);
        return;
      }
      setMode(window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
    });
    return () => window.cancelAnimationFrame(frame);
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = mode;
    window.localStorage.setItem(themeStorageKey, mode);
  }, [mode]);

  return {
    mode,
    toggle: () => setMode((current) => (current === "dark" ? "light" : "dark"))
  };
}

function HomeLink() {
  return (
    <a
      aria-label="Go to abhijitmohanty.com"
      className="home-link"
      href="https://www.abhijitmohanty.com"
      rel="noreferrer"
      title="Home"
    >
      <svg aria-hidden="true" className="home-link-icon" viewBox="0 0 24 24">
        <path d="M4 11.5 12 4l8 7.5" />
        <path d="M6 10v9.5h12V10" />
        <path d="M10 19.5v-5h4v5" />
      </svg>
    </a>
  );
}

function ThemeToggle({ mode, onToggle }: { mode: ThemeMode; onToggle: () => void }) {
  const nextMode = mode === "dark" ? "light" : "dark";

  return (
    <button
      aria-label={`Switch to ${nextMode} mode`}
      className={`theme-toggle ${mode === "dark" ? "is-lit" : ""}`}
      onClick={onToggle}
      title={`Switch to ${nextMode} mode`}
      type="button"
    >
      <svg aria-hidden="true" className="theme-toggle-icon" viewBox="0 0 24 24">
        <path d="M9 18h6" />
        <path d="M10 21h4" />
        <path d="M8.7 14.7c-1.2-.9-2-2.4-2-4.1C6.7 7.7 9.1 5.3 12 5.3s5.3 2.4 5.3 5.3c0 1.7-.8 3.2-2 4.1-.8.6-1.2 1.2-1.4 2.1h-3.8c-.2-.9-.6-1.5-1.4-2.1Z" />
      </svg>
    </button>
  );
}

function HeartIcon() {
  return (
    <svg aria-hidden="true" className="footer-icon heart-icon" viewBox="0 0 24 24">
      <path d="M12 20.4 4.95 13.6A4.87 4.87 0 0 1 3.5 10.1C3.5 7.57 5.48 5.6 8.01 5.6c1.57 0 3.05.78 3.99 2.08A4.9 4.9 0 0 1 16 5.6c2.52 0 4.5 1.97 4.5 4.5 0 1.34-.52 2.61-1.45 3.5Z" />
    </svg>
  );
}

export function IdentityBadge() {
  return (
    <div aria-label="Current workspace user" className="auth-compact user-badge">
      {formatUsername(publicOwnerHandle)}
    </div>
  );
}

function isExternalSourcePage(activePage: KnowledgeInboxPage) {
  return activePage === "original-youtube-posts" || activePage === "original-x-posts";
}

function isLazyLoadedFeedPage(activePage: KnowledgeInboxPage) {
  return activePage === "daily-newsletter" || isExternalSourcePage(activePage);
}

function isSearchablePage(activePage: KnowledgeInboxPage) {
  return activePage === "daily-newsletter" || isExternalSourcePage(activePage);
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
  onOpen,
  prioritizeImage
}: {
  copied: boolean;
  index: number;
  item: FeedItem;
  itemKey: string;
  onCopy: () => void;
  onOpen: () => void;
  prioritizeImage: boolean;
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
        {item.imageUrl ? (
          // eslint-disable-next-line @next/next/no-img-element -- Digest images are served by the Go API for static-export newsletters.
          <img
            alt={item.imageAlt ?? ""}
            className="newsletter-thumb"
            decoding="async"
            fetchPriority={prioritizeImage ? "high" : "auto"}
            height={236}
            loading={prioritizeImage ? "eager" : "lazy"}
            src={item.imageUrl}
            width={236}
          />
        ) : null}
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
          {/* eslint-disable-next-line @next/next/no-img-element -- Digest images are served by the Go API for static-export newsletters. */}
          {item.imageUrl ? <img alt={item.imageAlt ?? ""} className="newsletter-hero-image" src={item.imageUrl} /> : null}
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
        sortTimestamp: item.sortTimestamp,
        sourceUrl: item.sourceUrl,
        stats: item.stats ?? item.status
      };
    })
    .sort(compareFeedItemsNewestFirst);
  const youtubePosts = model.transcripts.items
    .map((item): FeedItem => ({
      id: item.id,
      source: "YouTube",
      eyebrow: item.statusLabel,
      title: summariesBySourceUrl.get(item.sourceUrl)?.title ?? item.title,
      body: summariesBySourceUrl.get(item.sourceUrl)?.body ?? item.detail,
      quote: summariesBySourceUrl.get(item.sourceUrl)?.quote ?? (item.detail.length > 150 ? `${item.detail.slice(0, 150).trim()}...` : item.detail),
      author: item.author,
      timestamp: item.timestamp,
      sortTimestamp: item.sortTimestamp,
      sourceUrl: item.sourceUrl,
      stats: item.statusLabel,
      timeMarkers: item.timeMarkers?.length ? item.timeMarkers : summariesBySourceUrl.get(item.sourceUrl)?.timeMarkers
    }))
    .sort(compareFeedItemsNewestFirst);

  if (activePage === "insights") return insightItems;
  if (activePage === "knowledge-graph") return [];
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
      fullBody: issue.bodyMarkdown,
      imageUrl: issue.illustrationUrl,
      imageAlt: issue.illustrationAlt
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

function compareFeedItemsNewestFirst(a: FeedItem, b: FeedItem) {
  const diff = feedSortTime(b) - feedSortTime(a);
  if (diff !== 0) return diff;
  return a.id.localeCompare(b.id);
}

function feedSortTime(item: FeedItem) {
  if (!item.sortTimestamp) return 0;
  const time = Date.parse(item.sortTimestamp);
  return Number.isFinite(time) ? time : 0;
}

function sliceItems(items: FeedItem[], visibleCount: number) {
  return items.slice(0, visibleCount);
}

function feedSentinelLabel({
  activePage,
  hasActiveSourceSearch,
  hasMorePageItems,
  hasMoreVisibleItems,
  isLoadingMore
}: {
  activePage: KnowledgeInboxPage;
  hasActiveSourceSearch: boolean;
  hasMorePageItems: boolean;
  hasMoreVisibleItems: boolean;
  isLoadingMore: boolean;
}) {
  if (activePage === "daily-newsletter") {
    if (isLoadingMore) return "Loading previous newsletters";
    if (hasMoreVisibleItems || hasMorePageItems) {
      return hasActiveSourceSearch ? "Scroll for more matching newsletters" : "Scroll for previous newsletters";
    }
    return hasActiveSourceSearch ? "End of matching newsletters" : "End of newsletter archive";
  }
  if (isLoadingMore) return "Loading more sourced items";
  if (hasMoreVisibleItems || hasMorePageItems) {
    return hasActiveSourceSearch ? "Scroll for more matching sources" : "Scroll for more sourced items";
  }
  return "End of sourced items";
}

function filterFeedItems(items: FeedItem[], normalizedQuery: string) {
  if (!normalizedQuery) return items;

  const results: Array<{ item: FeedItem; score: number }> = [];
  for (const item of items) {
    const score = feedItemSearchScore(item, normalizedQuery);
    if (score > 0) {
      results.push({ item, score });
    }
  }

  return results
    .sort((a, b) => b.score - a.score || compareFeedItemsNewestFirst(a.item, b.item))
    .map((result) => result.item);
}

function feedItemSearchScore(item: FeedItem, normalizedQuery: string) {
  const terms = normalizedQuery.split(" ").filter(Boolean);
  if (!terms.length) return 0;

  const fields = [
    { text: item.title, weight: 6 },
    { text: item.author, weight: 4 },
    { text: item.body, weight: 3 },
    { text: item.fullBody ?? "", weight: 3, allowSubsequence: false },
    { text: item.quote ?? "", weight: 3 },
    { text: item.stats ?? "", weight: 2 },
    { text: item.sourceUrl ?? "", weight: 2 },
    { text: item.timestamp, weight: 1 }
  ];

  let totalScore = 0;
  for (const term of terms) {
    let bestTermScore = 0;
    for (const field of fields) {
      const normalizedField = normalizeSearchTerm(field.text);
      const fieldScore =
        field.allowSubsequence === false ? substringFieldScore(normalizedField, term) : fuzzyFieldScore(normalizedField, term);
      bestTermScore = Math.max(bestTermScore, fieldScore * field.weight);
    }
    if (bestTermScore === 0) return 0;
    totalScore += bestTermScore;
  }
  return totalScore;
}

function substringFieldScore(text: string, term: string) {
  if (!text || !term) return 0;
  if (text === term) return 10;
  if (text.startsWith(term)) return 8;
  if (text.includes(term)) return 6;
  return 0;
}

function fuzzyFieldScore(text: string, term: string) {
  if (!text || !term) return 0;
  if (text === term) return 10;
  if (text.startsWith(term)) return 8;
  if (text.includes(term)) return 6;
  return isOrderedSubsequence(term, text) ? 2 : 0;
}

function isOrderedSubsequence(needle: string, haystack: string) {
  let needleIndex = 0;
  for (let haystackIndex = 0; haystackIndex < haystack.length && needleIndex < needle.length; haystackIndex += 1) {
    if (haystack[haystackIndex] === needle[needleIndex]) {
      needleIndex += 1;
    }
  }
  return needleIndex === needle.length;
}

function normalizeSearchTerm(value: string) {
  return value
    .toLowerCase()
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .replace(/[^a-z0-9]+/g, " ")
    .trim();
}

function sourceTotalForPage(model: KnowledgeInboxViewModel, activePage: KnowledgeInboxPage) {
  if (activePage === "original-x-posts") {
    return model.sourceCounts.xBookmarks;
  }
  if (activePage === "original-youtube-posts") {
    return model.sourceCounts.youtubeItems;
  }
  return 0;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("en").format(value);
}

function isActiveNav(item: NavigationItemViewModel, activePage: KnowledgeInboxPage) {
  return (
    (activePage === "insights" && (item.href === "/" || item.href === "/insights")) ||
    (activePage === "knowledge-graph" && item.href === "/knowledge-graph") ||
    (activePage === "daily-newsletter" && item.href === "/daily-newsletter") ||
    (activePage === "original-x-posts" && (item.href === "/original-x-posts" || item.href === "/original-x-bookmarks")) ||
    (activePage === "original-youtube-posts" && (item.href === "/original-youtube-posts" || item.href === "/original-youtube-videos"))
  );
}

type AskMessage = { role: "user" | "assistant" | "pending"; text: string; sources?: AskSecondBrainSource[] };

function AskChatWidget() {
  const [open, setOpen] = useState(false);
  const [question, setQuestion] = useState("");
  const [useLatest, setUseLatest] = useState(false);
  const [pending, setPending] = useState(false);
  const [messages, setMessages] = useState<AskMessage[]>([]);
  const scrollRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [messages]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const q = question.trim();
    if (!q || pending) return;
    setQuestion("");
    setMessages((prev) => [...prev, { role: "user", text: q }, { role: "pending", text: "" }]);
    setPending(true);
    try {
      const result = await askSecondBrain({ question: q, useLatest });
      setMessages((prev) => [
        ...prev.filter((m) => m.role !== "pending"),
        { role: "assistant", text: result.answer, sources: result.sources }
      ]);
    } catch (error) {
      setMessages((prev) => [
        ...prev.filter((m) => m.role !== "pending"),
        { role: "assistant", text: error instanceof Error ? error.message : "Something went wrong. Try again." }
      ]);
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="ask-brain">
      {open ? (
        <div className="ask-panel" role="dialog" aria-label="Ask Second Brain">
          <header>
            <div>
              <strong>Ask Second Brain</strong>
              <small>Source-grounded answers from your knowledge base</small>
            </div>
            <button aria-label="Close" onClick={() => setOpen(false)} type="button">×</button>
          </header>
          <div className="ask-messages" ref={scrollRef}>
            {messages.length === 0 ? (
              <p className="ask-empty">Ask anything about your saved X bookmarks, YouTube videos, insights, or themes.</p>
            ) : (
              messages.map((msg, index) => {
                if (msg.role === "pending") {
                  return (
                    <div key={index} className="ask-message pending">
                      <span className="loading-spinner" aria-hidden="true" />
                      Thinking…
                    </div>
                  );
                }
                return (
                  <div key={index} className={`ask-message ${msg.role}`}>
                    <p>{msg.text}</p>
                    {msg.sources?.length ? (
                      <ul style={{ margin: "6px 0 0", padding: "0 0 0 14px", fontSize: "0.76rem", color: "var(--muted)" }}>
                        {msg.sources.slice(0, 4).map((s) => (
                          <li key={s.id}>
                            {s.sourceUrl ? (
                              <a href={s.sourceUrl} rel="noreferrer" target="_blank">{s.title}</a>
                            ) : (
                              s.title
                            )}
                          </li>
                        ))}
                      </ul>
                    ) : null}
                  </div>
                );
              })
            )}
          </div>
          <form onSubmit={handleSubmit}>
            <label className="latest-toggle">
              <input
                checked={useLatest}
                onChange={(e) => setUseLatest(e.target.checked)}
                type="checkbox"
              />
              <span>Include live web search</span>
            </label>
            <div className="ask-input-row">
              <input
                aria-label="Your question"
                disabled={pending}
                onChange={(e) => setQuestion(e.target.value)}
                placeholder="What have you learned about…"
                type="text"
                value={question}
              />
              <button disabled={pending || !question.trim()} type="submit">Ask</button>
            </div>
          </form>
        </div>
      ) : null}
      <button className="ask-launcher" onClick={() => setOpen((v) => !v)} type="button">
        Ask Second Brain
      </button>
    </div>
  );
}
