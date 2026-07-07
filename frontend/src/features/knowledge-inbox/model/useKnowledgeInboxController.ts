"use client";

import { startTransition, useCallback, useEffect, useRef, useState } from "react";
import { clearAppStateCache, readAppState, readKnowledgeRefreshStatus } from "../api/knowledgeRuns";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { AppState, DigestIssue, InsightGraphResponse, KnowledgeRunResult, RefreshStatus } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";

const sourcePageInitialLimit = 25;
const sourcePageBatchSize = 25;
const sourcePageMaxLimit = 1000;
const newsletterPageInitialLimit = 10;
const newsletterPageBatchSize = 10;
const newsletterPageMaxLimit = 100;

export function useKnowledgeInboxController(activePage: KnowledgeInboxPage = "insights") {
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [hasMorePageItems, setHasMorePageItems] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [digestIssues, setDigestIssues] = useState<DigestIssue[]>([]);
  const [insightGraph, setInsightGraph] = useState<InsightGraphResponse | null>(null);
  const pageLimitRef = useRef(appStateLimitForPage(activePage));
  const error: string | null = null;

  const applyAppState = useCallback((state: AppState, requestedLimit = pageLimitRef.current) => {
    const latest = state.latest;
    if (latest) {
      startTransition(() => setRun(latest));
    }
    startTransition(() => setDigestIssues(state.digests ?? []));
    startTransition(() => setInsightGraph(state.graph?.insightGraph ?? null));
    setHasMorePageItems(hasMoreItemsForPage(activePage, state, requestedLimit));
    setRefreshStatus(state.refreshStatus);
    setIsRunning(state.refreshStatus?.status === "running");
  }, [activePage]);

  useEffect(() => {
    let ignore = false;
    const initialLimit = appStateLimitForPage(activePage);
    pageLimitRef.current = initialLimit;
    clearAppStateCache();
    readAppState(activePage, initialLimit)
      .then((state) => {
        if (!ignore) {
          applyAppState(state, initialLimit);
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (!ignore) {
          setIsLoading(false);
        }
      });

    return () => {
      ignore = true;
    };
  }, [activePage, applyAppState]);

  const loadMorePageItems = useCallback(async () => {
    if (!isPaginatedPage(activePage) || isLoadingMore || !hasMorePageItems) {
      return;
    }
    const pageLimit = pageLimitRef.current;
    const pagination = paginationForPage(activePage);
    const nextLimit = Math.min(pageLimit + pagination.batchSize, pagination.maxLimit);
    if (nextLimit <= pageLimit) {
      setHasMorePageItems(false);
      return;
    }

    setIsLoadingMore(true);
    try {
      const state = await readAppState(activePage, nextLimit);
      applyAppState(state, nextLimit);
      pageLimitRef.current = nextLimit;
    } catch {
      // Keep the current slice visible; the next sentinel hit can retry.
    } finally {
      setIsLoadingMore(false);
    }
  }, [activePage, applyAppState, hasMorePageItems, isLoadingMore]);

  useEffect(() => {
    let ignore = false;
    let timer: number | undefined;

    async function pollRefresh() {
      const status = await readKnowledgeRefreshStatus().catch(() => null);
      if (ignore || !status) return;
      setRefreshStatus(status);
      setIsRunning(status.status === "running");
      if (status.status === "running") {
        timer = window.setTimeout(pollRefresh, 2500);
        return;
      }
      if (status.status === "completed") {
        const requestedLimit = isPaginatedPage(activePage) ? pageLimitRef.current : appStateLimitForPage(activePage);
        const state = await readAppState(activePage, requestedLimit).catch(() => null);
        if (!ignore && state) {
          applyAppState(state, requestedLimit);
        }
      }
    }

    void pollRefresh();
    return () => {
      ignore = true;
      if (timer) {
        window.clearTimeout(timer);
      }
    };
  }, [activePage, applyAppState]);

  return {
    run,
    isLoading,
    isLoadingMore,
    hasMorePageItems,
    isRunning,
    loadMorePageItems,
    refreshStatus,
    digestIssues,
    insightGraph,
    error
  };
}

function appStateLimitForPage(page: KnowledgeInboxPage) {
  switch (page) {
    case "daily-newsletter":
      return newsletterPageInitialLimit;
    case "original-x-posts":
    case "original-youtube-posts":
      return sourcePageInitialLimit;
    case "knowledge-graph":
      return 180;
    default:
      return 20;
  }
}

function isExternalSourcePage(page: KnowledgeInboxPage) {
  return page === "original-x-posts" || page === "original-youtube-posts";
}

function isPaginatedPage(page: KnowledgeInboxPage) {
  return page === "daily-newsletter" || isExternalSourcePage(page);
}

function paginationForPage(page: KnowledgeInboxPage) {
  if (page === "daily-newsletter") {
    return {
      batchSize: newsletterPageBatchSize,
      maxLimit: newsletterPageMaxLimit
    };
  }
  return {
    batchSize: sourcePageBatchSize,
    maxLimit: sourcePageMaxLimit
  };
}

function hasMoreItemsForPage(page: KnowledgeInboxPage, state: AppState, requestedLimit: number) {
  if (page === "daily-newsletter") {
    return (state.digests?.length ?? 0) >= requestedLimit && requestedLimit < newsletterPageMaxLimit;
  }
  if (page === "original-x-posts") {
    const loaded = state.latest?.xBookmarks.length ?? state.views?.originalXBookmarks?.length ?? 0;
    const total = sourceTotalForPage(state.latest, page, state.sourceCounts);
    return loaded < total && requestedLimit < sourcePageMaxLimit;
  }
  if (page === "original-youtube-posts") {
    const loaded = state.latest?.youtubeItems.length ?? state.views?.originalYouTubePosts?.length ?? 0;
    const total = sourceTotalForPage(state.latest, page, state.sourceCounts);
    return loaded < total && requestedLimit < sourcePageMaxLimit;
  }
  return false;
}

function sourceTotalForPage(run: KnowledgeRunResult | null, page: KnowledgeInboxPage, counts = run?.sourceCounts) {
  if (page === "original-x-posts") {
    return counts?.xBookmarks ?? run?.xBookmarks.length ?? 0;
  }
  if (page === "original-youtube-posts") {
    return counts?.youtubeItems ?? run?.youtubeItems.length ?? 0;
  }
  return 0;
}
