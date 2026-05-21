import type { PanelViewModel, TranscriptItemViewModel } from "../presentation/viewModel";
import { EmptyState } from "./primitives/EmptyState";
import { Panel } from "./primitives/Panel";

export function TranscriptPanel({ model }: { model: PanelViewModel<TranscriptItemViewModel> }) {
  return (
    <Panel title={model.title} description={model.description} icon={model.icon} className="transcript-panel">
      {model.items.length ? (
        model.items.map((item) => (
          <article className="transcript-item" key={item.id}>
            <strong>{item.title}</strong>
            <span>{item.statusLabel}</span>
            <p>{item.detail}</p>
          </article>
        ))
      ) : (
        <EmptyState state={model.empty} compact />
      )}
    </Panel>
  );
}
