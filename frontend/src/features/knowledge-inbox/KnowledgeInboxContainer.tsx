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
    connectX,
    savePlaylist,
    saveFeedback,
    askBrain
  } = useKnowledgeInboxController(initialPage);
  const model = useMemo(() => toKnowledgeInboxViewModel(run, isRunning, error), [run, isRunning, error]);

  return (
    <SecondBrainConsoleView
      activePage={initialPage}
      isAsking={isAsking}
      isLoading={isLoading}
      refreshStatus={refreshStatus}
      digestIssues={digestIssues}
      insightGraph={insightGraph}
      auth={auth}
      workspace={workspace}
      model={model}
      chatMessages={chatMessages}
      onConnectX={connectX}
      onSavePlaylist={savePlaylist}
      onAsk={askBrain}
      onFeedback={saveFeedback}
    />
  );
}
