import type { PanelViewModel, SummaryCardViewModel } from "../presentation/viewModel";
import { EmptyState } from "./primitives/EmptyState";
import { Icon } from "./primitives/Icon";
import { Panel } from "./primitives/Panel";

export function SummaryReview({ model }: { model: PanelViewModel<SummaryCardViewModel> }) {
  return (
    <Panel id="review" title={model.title} description={model.description} icon={model.icon} className="summaries">
      <div className="summary-grid">
        {model.items.length ? (
          model.items.map((summary) => (
            <article className="summary-card" key={summary.id}>
              <div className="summary-topline">
                <span className={`decision ${summary.decision}`}>{summary.decisionLabel}</span>
                <span>{summary.confidenceLabel}</span>
              </div>
              <h3>{summary.title}</h3>
              <p>{summary.body}</p>
              {summary.quote ? <blockquote>{summary.quote}</blockquote> : null}
              <a className="source-link" href={summary.sourceUrl} target="_blank" rel="noreferrer">
                <Icon name="link" />
                Source
              </a>
            </article>
          ))
        ) : (
          <EmptyState state={model.empty} />
        )}
      </div>
    </Panel>
  );
}
