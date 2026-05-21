import type { EmptyStateViewModel } from "../../presentation/viewModel";
import { Icon } from "./Icon";

export function EmptyState({ state, compact = false }: { state: EmptyStateViewModel; compact?: boolean }) {
  return (
    <div className={`empty-panel${compact ? " compact" : ""}`}>
      <Icon name={state.icon} />
      <strong>{state.title}</strong>
      <p>{state.body}</p>
    </div>
  );
}
