"use client";

import { useMemo } from "react";
import { useKnowledgeInboxController } from "./model/useKnowledgeInboxController";
import { toKnowledgeInboxViewModel } from "./presentation/viewModel";
import { SecondBrainConsoleView } from "./ui/SecondBrainConsoleView";

export type KnowledgeInboxPage = "insights" | "daily-newsletter" | "original-x-posts" | "original-youtube-posts" | "knowledge-graph";

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
    digestIssues,
    chatMessages,
    error,
    saveFeedback,
    generateDigest,
    sendDigest,
    askBrain
  } = useKnowledgeInboxController(initialPage);
  const model = useMemo(() => toKnowledgeInboxViewModel(run, isRunning, error), [run, isRunning, error]);

  return (
    <SecondBrainConsoleView
      activePage={initialPage}
      isDigesting={isDigesting}
      isAsking={isAsking}
      isLoading={isLoading}
      refreshStatus={refreshStatus}
      digestIssues={digestIssues}
      model={model}
      chatMessages={chatMessages}
      onAsk={askBrain}
      onDigest={generateDigest}
      onSendDigest={sendDigest}
      onFeedback={saveFeedback}
    />
  );
}
