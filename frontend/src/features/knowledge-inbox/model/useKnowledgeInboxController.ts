"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import { askSecondBrain, clearAppStateCache, readAppState, readKnowledgeRefreshStatus, readWorkspaceStatus, saveKnowledgeFeedback, saveYouTubePlaylist, shareInsightToX, startKnowledgeInboxRefresh, startXAuth } from "../api/knowledgeRuns";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { AppState, AskSecondBrainResponse, DigestIssue, FeedbackSignal, InsightGraphResponse, KnowledgeRunResult, RefreshStatus, WorkspaceStatus } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";
import { useSupabaseAuth } from "./useSupabaseAuth";

export type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  content: string;
  sources?: AskSecondBrainResponse["sources"];
  status?: string;
};

export function useKnowledgeInboxController(activePage: KnowledgeInboxPage = "insights") {
  const auth = useSupabaseAuth();
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isLoading, setIsLoading] = useState(true);
  const [isRunning, setIsRunning] = useState(false);
  const [isAsking, setIsAsking] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [digestIssues, setDigestIssues] = useState<DigestIssue[]>([]);
  const [insightGraph, setInsightGraph] = useState<InsightGraphResponse | null>(null);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [workspace, setWorkspace] = useState<WorkspaceStatus | null>(null);

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
    readWorkspaceStatus()
      .then((status) => {
        if (!ignore) {
          setWorkspace(status);
        }
      })
      .catch(() => undefined);
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
  }, [activePage, applyAppState, auth.authVersion]);

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
  }, [activePage, applyAppState, auth.authVersion]);

  const runValidation = useCallback(async () => {
    if (isRunning) {
      const status = await readKnowledgeRefreshStatus().catch(() => null);
      if (status) {
        setRefreshStatus(status);
      }
      setError("A refresh is already underway. The progress tracker will update until it completes.");
      return;
    }
    setIsRunning(true);
    setError(null);
    try {
      let status = await startKnowledgeInboxRefresh();
      setRefreshStatus(status);
      while (status.status === "running") {
        await delay(2500);
        status = await readKnowledgeRefreshStatus();
        setRefreshStatus(status);
      }
      if (status.status === "failed") {
        throw new Error(status.error || "Knowledge inbox validation failed.");
      }
      const state = await readAppState(activePage, appStateLimitForPage(activePage));
      applyAppState(state);
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : "Knowledge inbox validation failed.");
    } finally {
      setIsRunning(false);
    }
  }, [activePage, applyAppState, isRunning]);

  const saveFeedback = useCallback(async (targetType: string, targetId: string, signal: FeedbackSignal, sourceUrl?: string) => {
    try {
      await saveKnowledgeFeedback({ targetType, targetId, signal, sourceUrl });
    } catch (feedbackError) {
      setError(feedbackError instanceof Error ? feedbackError.message : "Feedback could not be saved.");
    }
  }, []);

  const shareTweet = useCallback(async (targetType: string, targetId: string, text: string, sourceUrl?: string) => {
    setError(null);
    try {
      await shareInsightToX({ targetType, targetId, text, sourceUrl });
    } catch (tweetError) {
      setError(tweetError instanceof Error ? tweetError.message : "Tweet could not be posted.");
      throw tweetError;
    }
  }, []);

  const askBrain = useCallback(async (question: string, useLatest?: boolean) => {
    const trimmed = question.trim();
    if (!trimmed) return;
    const userMessage: ChatMessage = {
      id: `user-${Date.now()}`,
      role: "user",
      content: trimmed
    };
    setChatMessages((messages) => [...messages, userMessage]);
    setIsAsking(true);
    setError(null);
    try {
      const response = await askSecondBrain({ question: trimmed, useLatest });
      setChatMessages((messages) => [
        ...messages,
        {
          id: `assistant-${Date.now()}`,
          role: "assistant",
          content: response.answer,
          sources: response.sources,
          status: response.searchStatus
        }
      ]);
    } catch (askError) {
      const message = askError instanceof Error ? askError.message : "Ask Your Second Brain failed.";
      setChatMessages((messages) => [
        ...messages,
        {
          id: `assistant-error-${Date.now()}`,
          role: "assistant",
          content: message
        }
      ]);
      setError(message);
    } finally {
      setIsAsking(false);
    }
  }, []);

  const signIn = useCallback(
    async (email: string) => {
      setError(null);
      await auth.signIn(email);
    },
    [auth]
  );

  const signOut = useCallback(async () => {
    setError(null);
    await auth.signOut();
    setWorkspace(null);
    setRun(initialKnowledgeRun);
    clearAppStateCache();
  }, [auth]);

  const connectX = useCallback(async () => {
    setError(null);
    try {
      const { url } = await startXAuth();
      window.location.assign(url);
    } catch (xError) {
      setError(xError instanceof Error ? xError.message : "X authorization could not be started.");
    }
  }, []);

  const savePlaylist = useCallback(async (playlist: string) => {
    setError(null);
    try {
      await saveYouTubePlaylist({ playlistId: playlist, playlistUrl: playlist });
      const status = await readWorkspaceStatus();
      setWorkspace(status);
    } catch (playlistError) {
      setError(playlistError instanceof Error ? playlistError.message : "YouTube playlist could not be saved.");
      throw playlistError;
    }
  }, []);

  return {
    auth,
    run,
    isLoading,
    isRunning,
    isAsking,
    refreshStatus,
    digestIssues,
    insightGraph,
    chatMessages,
    workspace,
    error,
    signIn,
    signOut,
    connectX,
    savePlaylist,
    runValidation,
    saveFeedback,
    shareTweet,
    askBrain
  };
}

function delay(ms: number) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
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
