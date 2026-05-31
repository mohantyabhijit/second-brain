"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import { askSecondBrain, generateDailyDigest, readAppState, readDigestIssues, readKnowledgeRefreshStatus, readLatestKnowledgeRun, saveKnowledgeFeedback, sendLatestDigest, shareInsightToX, startKnowledgeInboxRefresh } from "../api/knowledgeRuns";
import type { KnowledgeInboxPage } from "../KnowledgeInboxContainer";
import type { AppState, AskSecondBrainResponse, DigestIssue, FeedbackSignal, KnowledgeRunResult, RefreshStatus } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";

export type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  content: string;
  sources?: AskSecondBrainResponse["sources"];
  status?: string;
};

export function useKnowledgeInboxController(activePage: KnowledgeInboxPage = "insights") {
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isLoading, setIsLoading] = useState(true);
  const [isRunning, setIsRunning] = useState(false);
  const [isDigesting, setIsDigesting] = useState(false);
  const [isAsking, setIsAsking] = useState(false);
  const [refreshStatus, setRefreshStatus] = useState<RefreshStatus | null>(null);
  const [digestIssues, setDigestIssues] = useState<DigestIssue[]>([]);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([]);
  const [error, setError] = useState<string | null>(null);

  const applyAppState = useCallback((state: AppState) => {
    const latest = state.latest;
    if (latest) {
      startTransition(() => setRun(latest));
    }
    startTransition(() => setDigestIssues(state.digests ?? []));
    setRefreshStatus(state.refreshStatus);
    setIsRunning(state.refreshStatus?.status === "running");
  }, []);

  useEffect(() => {
    let ignore = false;
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
      const state = await readAppState(activePage, appStateLimitForPage(activePage)).catch(() => null);
      if (state) {
        applyAppState(state);
      } else {
        const latest = await readLatestKnowledgeRun();
        if (latest) {
          startTransition(() => setRun(latest));
        }
        const digests = await readDigestIssues().catch(() => []);
        startTransition(() => setDigestIssues(digests));
      }
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

  const generateDigest = useCallback(async () => {
    setIsDigesting(true);
    setError(null);
    try {
      const digest = await generateDailyDigest();
      startTransition(() => setRun((current) => ({ ...current, digest })));
      startTransition(() => setDigestIssues((current) => upsertDigestIssue(current, digest)));
    } catch (digestError) {
      setError(digestError instanceof Error ? digestError.message : "Digest generation failed.");
    } finally {
      setIsDigesting(false);
    }
  }, []);

  const sendDigest = useCallback(async (recipientEmail: string) => {
    setIsDigesting(true);
    setError(null);
    try {
      const currentDigest = run.digest;
      const digest = await sendLatestDigest({ recipientEmail, digest: currentDigest });
      startTransition(() => setRun((current) => ({ ...current, digest })));
      startTransition(() => setDigestIssues((current) => upsertDigestIssue(current, digest)));
      return digest;
    } catch (digestError) {
      const message = digestError instanceof Error ? digestError.message : "Digest delivery failed.";
      setError(message);
      throw digestError;
    } finally {
      setIsDigesting(false);
    }
  }, [run.digest]);

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

  return {
    run,
    isLoading,
    isRunning,
    isDigesting,
    isAsking,
    refreshStatus,
    digestIssues,
    chatMessages,
    error,
    runValidation,
    saveFeedback,
    generateDigest,
    sendDigest,
    shareTweet,
    askBrain
  };
}

function upsertDigestIssue(current: DigestIssue[], digest: DigestIssue) {
  const key = digest.id || digest.idempotencyKey || digest.digestDate;
  const withoutDigest = current.filter((item) => (item.id || item.idempotencyKey || item.digestDate) !== key);
  return [digest, ...withoutDigest];
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
      return 1;
    default:
      return 20;
  }
}
