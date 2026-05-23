"use client";

import { useMemo } from "react";
import { useKnowledgeInboxController } from "./model/useKnowledgeInboxController";
import { toKnowledgeInboxViewModel } from "./presentation/viewModel";
import { SecondBrainConsoleView } from "./ui/SecondBrainConsoleView";

export type KnowledgeInboxPage = "insights" | "daily-newsletter" | "original-x-posts" | "original-youtube-posts";

type KnowledgeInboxContainerProps = {
  initialPage?: KnowledgeInboxPage;
};

export default function KnowledgeInboxContainer({ initialPage = "insights" }: KnowledgeInboxContainerProps) {
  const {
    run,
    isLoading,
    isRunning,
    isDigesting,
    isAsking,
    refreshStatus,
    chatMessages,
    error,
    runValidation,
    saveFeedback,
    generateDigest,
    shareTweet,
    askBrain
  } = useKnowledgeInboxController();
  const model = useMemo(() => toKnowledgeInboxViewModel(run, isRunning, error), [run, isRunning, error]);

  return (
    <SecondBrainConsoleView
      activePage={initialPage}
      isDigesting={isDigesting}
      isAsking={isAsking}
      isLoading={isLoading}
      refreshStatus={refreshStatus}
      model={model}
      chatMessages={chatMessages}
      onAsk={askBrain}
      onDigest={generateDigest}
      onFeedback={saveFeedback}
      onRun={runValidation}
      onTweet={shareTweet}
    />
  );
}
