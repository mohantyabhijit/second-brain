import type { MetricViewModel } from "../presentation/viewModel";

export function MetricsGrid({ metrics }: { metrics: MetricViewModel[] }) {
  return (
    <section className="metrics-row" aria-label="Knowledge inbox metrics">
      {metrics.map((metric) => (
        <div key={metric.label}>
          <span>{metric.label}</span>
          <strong>{metric.value}</strong>
        </div>
      ))}
    </section>
  );
}
