import type { PanelViewModel, IntakeRowViewModel } from "../presentation/viewModel";
import { EmptyState } from "./primitives/EmptyState";
import { Panel } from "./primitives/Panel";

export function IntakeTable({ model }: { model: PanelViewModel<IntakeRowViewModel> }) {
  return (
    <Panel id="intake" title={model.title} description={model.description} icon={model.icon} className="wide">
      <div className="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Source</th>
              <th>Item</th>
              <th>Author</th>
              <th>Status</th>
              <th>Link</th>
            </tr>
          </thead>
          <tbody>
            {model.items.map((row) => (
              <tr key={row.id}>
                <td>{row.source}</td>
                <td>{row.item}</td>
                <td>{row.author}</td>
                <td>{row.status}</td>
                <td>
                  <a href={row.sourceUrl} target="_blank" rel="noreferrer">
                    Open
                  </a>
                </td>
              </tr>
            ))}
            {model.items.length === 0 ? (
              <tr>
                <td colSpan={5}>
                  <div className="table-empty">
                    <EmptyState state={model.empty} compact />
                  </div>
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
    </Panel>
  );
}
