"use client";

import { startTransition, useEffect, useState } from "react";
import { readLatestPhaseOneResult, runPhaseOneValidation } from "../api";
import type { PhaseOneResult } from "../types";

const initialResult: PhaseOneResult = {
  generatedAt: "1970-01-01T00:00:00.000Z",
  sourceStatus: {
    x: "needs_secrets",
    youtube: "needs_secrets",
    onecli: "needs_secrets"
  },
  xBookmarks: [],
  youtubeItems: [],
  summaries: [],
  validation: [
    {
      label: "X bookmark request",
      status: "untested",
      detail: "Run Phase 1 validation after adding credentials."
    },
    {
      label: "YouTube playlist check",
      status: "untested",
      detail: "Use a dedicated Second Brain Inbox playlist ID."
    },
    {
      label: "Transcript path",
      status: "untested",
      detail: "The app tests every playlist video unless YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID is set."
    }
  ],
  blockers: ["OneCLI is installed, but this app has not been run with authenticated provider secrets yet."]
};

export function usePhaseOne() {
  const [result, setResult] = useState<PhaseOneResult>(() => initialResult);
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let ignore = false;
    readLatestPhaseOneResult()
      .then((latest) => {
        if (!ignore && latest) {
          startTransition(() => setResult(latest));
        }
      })
      .catch(() => undefined);

    return () => {
      ignore = true;
    };
  }, []);

  async function runValidation() {
    setIsRunning(true);
    setError(null);
    try {
      const payload = await runPhaseOneValidation();
      startTransition(() => setResult(payload));
    } catch (phaseError) {
      setError(phaseError instanceof Error ? phaseError.message : "Phase 1 validation failed.");
    } finally {
      setIsRunning(false);
    }
  }

  return { result, isRunning, error, runValidation };
}
