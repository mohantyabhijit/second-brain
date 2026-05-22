"use client";

import { useMemo } from "react";
import { useKnowledgeInboxController } from "./model/useKnowledgeInboxController";
import { toKnowledgeInboxViewModel } from "./presentation/viewModel";
import { SecondBrainConsoleView } from "./ui/SecondBrainConsoleView";

export type KnowledgeInboxPage = "home" | "daily-newsletter" | "original-x-posts" | "original-youtube-posts";

type KnowledgeInboxContainerProps = {
  initialPage?: KnowledgeInboxPage;
};

export default function KnowledgeInboxContainer({ initialPage = "home" }: KnowledgeInboxContainerProps) {
  const { run, isRunning, error, runValidation } = useKnowledgeInboxController();
  const model = useMemo(() => toKnowledgeInboxViewModel(run, isRunning, error), [run, isRunning, error]);

  return <SecondBrainConsoleView activePage={initialPage} model={model} onRun={runValidation} />;
}
