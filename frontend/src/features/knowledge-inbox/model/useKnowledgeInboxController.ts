"use client";

import { startTransition, useCallback, useEffect, useRef, useState } from "react";
import { clearAppStateCache, readAppState, readKnowledgeRefreshStatus } from "../api/knowledgeRuns";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { AppState, DigestIssue, InsightGraphResponse, KnowledgeRunResult, RefreshStatus } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";

const sourcePageInitialLimit = 25;
const sourcePageBatchSize = 25;
const sourcePageMaxLimit = 1000;

export function useKnowledgeInboxController(activePage: KnowledgeInboxPage = "insights") {
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isLoading, setIsLoading] = useState(true);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [isRunning, setIsRunning] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [digestIssues, setDigestIssues] = useState<DigestIssue[]>([]);
  const [insightGraph, setInsightGraph] = useState<InsightGraphResponse | null>(null);
  const sourceLimitRef = useRef(appStateLimitForPage(activePage));
  const error: string | null = null;

  const applyAppState = useCallback((state: AppState) => {
    const latest = state.latest;
    if (latest) {
      startTransition(() => setRun(latest));
    }
    startTransition(() => setDigestIssues(state.digests ?? []));
    startTransition(() => setInsightGraph(state.graph?.insightGraph ?? null));
    setRefreshStatus(state.refreshStatus);
    setIsRunning(state.refreshStatus?.status === "running");
  }, []);

  useEffect(() => {
    let ignore = false;
    const initialLimit = appStateLimitForPage(activePage);
    sourceLimitRef.current = initialLimit;
    clearAppStateCache();
    readAppState(activePage, initialLimit)
      .then((state) => {
        if (!ignore) {
          applyAppState(state);
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

  const loadMoreSources = useCallback(async () => {
    if (!isExternalSourcePage(activePage) || isLoadingMore) {
      return;
    }
    const sourceLimit = sourceLimitRef.current;
    const nextLimit = Math.min(sourceLimit + sourcePageBatchSize, sourcePageMaxLimit);
    if (nextLimit <= sourceLimit) {
      return;
    }

    setIsLoadingMore(true);
    try {
      const state = await readAppState(activePage, nextLimit);
      applyAppState(state);
      sourceLimitRef.current = nextLimit;
    } catch {
      // Keep the current slice visible; the next sentinel hit can retry.
    } finally {
      setIsLoadingMore(false);
    }
  }, [activePage, applyAppState, isLoadingMore]);

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
        const state = await readAppState(activePage, isExternalSourcePage(activePage) ? sourceLimitRef.current : appStateLimitForPage(activePage)).catch(() => null);
        if (!ignore && state) {
          applyAppState(state);
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
    isRunning,
    loadMoreSources,
    refreshStatus,
    digestIssues,
    insightGraph,
    error
  };
}

function appStateLimitForPage(page: KnowledgeInboxPage) {
  switch (page) {
    case "daily-newsletter":
      return 10;
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

function sourceTotalForPage(run: KnowledgeRunResult, page: KnowledgeInboxPage) {
  if (page === "original-x-posts") {
    return run.sourceCounts?.xBookmarks ?? run.xBookmarks.length;
  }
  if (page === "original-youtube-posts") {
    return run.sourceCounts?.youtubeItems ?? run.youtubeItems.length;
  }
  return 0;
}
