import type { PanelViewModel, ValidationItemViewModel } from "../presentation/viewModel";
import { EmptyState } from "./primitives/EmptyState";
import { Icon } from "./primitives/Icon";
import { Panel } from "./primitives/Panel";

export function ValidationPanel({ model }: { model: PanelViewModel<ValidationItemViewModel> }) {
  return (
    <Panel id="quality" title={model.title} description={model.description} icon={model.icon}>
      {model.items.length ? (
        <ul className="validation-list">
          {model.items.map((item) => (
            <li className={`validation-row ${item.status}`} key={item.label}>
              <span className="validation-icon">
                <Icon name={item.icon} />
              </span>
              <div>
                <strong>{item.label}</strong>
                <span>{item.detail}</span>
              </div>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyState state={model.empty} compact />
      )}
    </Panel>
  );
}
