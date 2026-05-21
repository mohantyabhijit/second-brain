"use client";

import { useMemo } from "react";
import { useKnowledgeInboxController } from "./model/useKnowledgeInboxController";
import { toKnowledgeInboxViewModel } from "./presentation/viewModel";
import { SecondBrainConsoleView } from "./ui/SecondBrainConsoleView";

export default function KnowledgeInboxContainer() {
  const { run, isRunning, error, runValidation } = useKnowledgeInboxController();
  const model = useMemo(() => toKnowledgeInboxViewModel(run, isRunning, error), [run, isRunning, error]);

  return <SecondBrainConsoleView model={model} onRun={runValidation} />;
}
