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
    isDigesting,
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
      insightGraph={insightGraph}
      auth={auth}
      workspace={workspace}
      model={model}
      chatMessages={chatMessages}
      onSignIn={signIn}
      onSignOut={signOut}
      onConnectX={connectX}
      onSavePlaylist={savePlaylist}
      onAsk={askBrain}
      onDigest={generateDigest}
      onSendDigest={sendDigest}
      onFeedback={saveFeedback}
    />
  );
}
