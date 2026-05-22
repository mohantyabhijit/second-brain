"use client";

import { startTransition, useCallback, useEffect, useState } from "react";
import { generateDailyDigest, readLatestKnowledgeRun, runKnowledgeInboxValidation, saveKnowledgeFeedback } from "../api/knowledgeRuns";
import type { FeedbackSignal, KnowledgeRunResult } from "../contracts";
import { initialKnowledgeRun } from "./initialKnowledgeRun";

export function useKnowledgeInboxController() {
  const [run, setRun] = useState<KnowledgeRunResult>(() => initialKnowledgeRun);
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let ignore = false;
    readLatestKnowledgeRun()
      .then((latest) => {
        if (!ignore && latest) {
          startTransition(() => setRun(latest));
        }
      })
      .catch(() => undefined);

    return () => {
      ignore = true;
    };
  }, []);

  const runValidation = useCallback(async () => {
    setIsRunning(true);
    setError(null);
    try {
      const payload = await runKnowledgeInboxValidation();
      startTransition(() => setRun(payload));
    } catch (runError) {
      setError(runError instanceof Error ? runError.message : "Knowledge inbox validation failed.");
    } finally {
      setIsRunning(false);
    }
  }, []);

  const saveFeedback = useCallback(async (targetType: string, targetId: string, signal: FeedbackSignal, sourceUrl?: string) => {
    await saveKnowledgeFeedback({ targetType, targetId, signal, sourceUrl });
  }, []);

  const generateDigest = useCallback(async () => {
    const digest = await generateDailyDigest();
    startTransition(() => setRun((current) => ({ ...current, digest })));
  }, []);

  return { run, isRunning, error, runValidation, saveFeedback, generateDigest };
}
