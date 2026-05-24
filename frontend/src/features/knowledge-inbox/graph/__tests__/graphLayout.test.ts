import { describe, expect, it } from "vitest";
import type { InsightGraphResponse } from "../../contracts";
import { createInsightGraphLayout, fallbackGraphLabelMeasure, graphNodeLabel } from "../graphLayout";

describe("insight graph layout", () => {
  it("creates deterministic node positions and styled edges", () => {
    const graph: InsightGraphResponse = {
      nodes: [
        {
          id: "insight-a",
          label: "Coordination cost",
          canonicalInsight: "Team speed improves when coordination overhead is low.",
          topics: ["teams"],
          domain: "organizations"
        },
        {
          id: "insight-b",
          label: "Alignment load",
          topics: ["teams"],
          type: "warning"
        }
      ],
      edges: [
        {
          id: "insight-a::insight-b",
          source: "insight-a",
          target: "insight-b",
          reason: "shared_topic",
          label: "teams",
          weight: 2.4
        }
      ],
      stats: { totalInsights: 2, returnedInsights: 2, returnedEdges: 1 }
    };

    const first = createInsightGraphLayout(graph, fallbackGraphLabelMeasure);
    const second = createInsightGraphLayout(graph, fallbackGraphLabelMeasure);

    expect(first.nodes.map((node) => ({ id: node.id, x: node.x, y: node.y }))).toEqual(
      second.nodes.map((node) => ({ id: node.id, x: node.x, y: node.y }))
    );
    expect(first.nodes[0]?.labelLines.length).toBeGreaterThan(1);
    expect(first.edges[0]).toMatchObject({ color: "#2d6d58", dash: "8 7" });
  });

  it("uses canonical insight text in node labels without letting labels grow unbounded", () => {
    const label = graphNodeLabel({
      id: "insight-a",
      label: "Coordination cost",
      canonicalInsight:
        "Team speed improves when coordination overhead is low, especially when the work is tightly coupled and context needs to be shared repeatedly across teams.",
      topics: []
    });

    expect(label).toContain("Coordination cost.");
    expect(label.length).toBeLessThanOrEqual(130);
  });
});
