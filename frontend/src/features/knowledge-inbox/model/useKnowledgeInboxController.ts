"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import { clearAppStateCache, readAppState, readKnowledgeRefreshStatus } from "../api/knowledgeRuns";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { AppState, DigestIssue, InsightGraphResponse, KnowledgeRunResult, RefreshStatus } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";

export function useKnowledgeInboxController(activePage: KnowledgeInboxPage = "insights") {
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isLoading, setIsLoading] = useState(true);
  const [isRunning, setIsRunning] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [digestIssues, setDigestIssues] = useState<DigestIssue[]>([]);
  const [insightGraph, setInsightGraph] = useState<InsightGraphResponse | null>(null);
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
    clearAppStateCache();
    readAppState(activePage, appStateLimitForPage(activePage))
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
        const state = await readAppState(activePage, appStateLimitForPage(activePage)).catch(() => null);
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
    isRunning,
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
      return 15;
    case "knowledge-graph":
      return 180;
    default:
      return 20;
  }
}
