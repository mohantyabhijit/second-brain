import type { KnowledgeInboxViewModel, SourceCardViewModel } from "../presentation/viewModel";
import { Icon } from "./primitives/Icon";

function SourceCard({ source }: { source: SourceCardViewModel }) {
  return (
    <div className={`source-badge ${source.status}`}>
      <div className="source-icon">
        <Icon name={source.icon} />
      </div>
      <div>
        <span>{source.label}</span>
        <strong>{source.statusLabel}</strong>
        <small>{source.detail}</small>
      </div>
    </div>
  );
}

export function SourceHealthStrip({ sources, readiness }: Pick<KnowledgeInboxViewModel, "sources" | "readiness">) {
  return (
    <section id="sources" className="status-strip" aria-label="Source status">
      {sources.map((source) => (
        <SourceCard source={source} key={source.label} />
      ))}
      <div className="readiness-card">
        <div>
          <span>{readiness.label}</span>
          <strong>{readiness.value}</strong>
        </div>
        <div className="progress-track" aria-label={readiness.value}>
          <span style={{ width: `${readiness.progress}%` }} />
        </div>
        <small>{readiness.generatedAtLabel}</small>
      </div>
    </section>
  );
}
