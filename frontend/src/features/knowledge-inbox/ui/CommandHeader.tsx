import type { KnowledgeInboxViewModel } from "../presentation/viewModel";
import { ActionButton } from "./primitives/ActionButton";

type CommandHeaderProps = {
  header: KnowledgeInboxViewModel["header"];
  onRun: () => void;
};

export function CommandHeader({ header, onRun }: CommandHeaderProps) {
  return (
    <header className="topbar">
      <div>
        <span className="section-label">{header.eyebrow}</span>
        <h1>{header.title}</h1>
        <p>{header.description}</p>
      </div>
      <ActionButton label={header.actionLabel} isRunning={header.isRunning} onClick={onRun} />
    </header>
  );
}
