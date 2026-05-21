import type { KnowledgeInboxViewModel } from "../presentation/viewModel";
import { BlockerList } from "./BlockerList";
import { CommandHeader } from "./CommandHeader";
import { IntakeTable } from "./IntakeTable";
import { MetricsGrid } from "./MetricsGrid";
import { NavigationRail } from "./NavigationRail";
import { SourceHealthStrip } from "./SourceHealthStrip";
import { SummaryReview } from "./SummaryReview";
import { TranscriptPanel } from "./TranscriptPanel";
import { ValidationPanel } from "./ValidationPanel";

type SecondBrainConsoleViewProps = {
  model: KnowledgeInboxViewModel;
  onRun: () => void;
};

export function SecondBrainConsoleView({ model, onRun }: SecondBrainConsoleViewProps) {
  return (
    <main className="app-shell">
      <NavigationRail brand={model.brand} navigation={model.navigation} note={model.sidebarNote} />

      <section className="workspace">
        <CommandHeader header={model.header} onRun={onRun} />
        <SourceHealthStrip sources={model.sources} readiness={model.readiness} />
        <MetricsGrid metrics={model.metrics} />

        {model.error ? <div className="error-banner">{model.error}</div> : null}

        <div className="main-grid">
          <IntakeTable model={model.intake} />
          <ValidationPanel model={model.validation} />
          <SummaryReview model={model.summaries} />
          <TranscriptPanel model={model.transcripts} />
        </div>

        <BlockerList blockers={model.blockers} />
      </section>
    </main>
  );
}
