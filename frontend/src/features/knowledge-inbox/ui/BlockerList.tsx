import type { KnowledgeInboxViewModel } from "../presentation/viewModel";

export function BlockerList({ blockers }: { blockers: KnowledgeInboxViewModel["blockers"] }) {
  if (blockers.items.length === 0) return null;

  return (
    <section className="blockers" aria-label={blockers.title}>
      <div>
        <span className="section-label">{blockers.eyebrow}</span>
        <h2>{blockers.title}</h2>
      </div>
      {blockers.items.map((blocker) => (
        <p key={blocker}>{blocker}</p>
      ))}
    </section>
  );
}
