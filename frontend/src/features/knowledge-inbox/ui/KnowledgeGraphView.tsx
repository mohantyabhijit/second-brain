"use client";

import { type PointerEvent, useEffect, useMemo, useRef, useState } from "react";
import { readInsightGraph } from "../api/knowledgeRuns";
import type { InsightGraphNode, InsightGraphResponse } from "../contracts";
import { createInsightGraphLayout, fallbackGraphLabelMeasure, type GraphLayoutNode } from "../graph/graphLayout";
import { measurePretextGraphLabel } from "../graph/pretextMeasure";

type DragState =
  | {
      type: "pan";
      pointerId: number;
      startX: number;
      startY: number;
      scrollLeft: number;
      scrollTop: number;
    }
  | {
      type: "node";
      pointerId: number;
      nodeId: string;
      startX: number;
      startY: number;
      nodeX: number;
      nodeY: number;
      moved: boolean;
    };

const graphLimit = 180;

export function KnowledgeGraphView() {
  const [graph, setGraph] = useState<InsightGraphResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [nodePositions, setNodePositions] = useState<Record<string, { x: number; y: number }>>({});
  const [scale, setScale] = useState(1);
  const [drag, setDrag] = useState<DragState | null>(null);
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);

  useEffect(() => {
    let cancelled = false;
    readInsightGraph(graphLimit)
      .then((payload) => {
        if (cancelled) return;
        setGraph(payload);
        setError(null);
        setSelectedNodeId(payload.nodes[0]?.id ?? null);
        setNodePositions({});
      })
      .catch((requestError: unknown) => {
        if (cancelled) return;
        setError(requestError instanceof Error ? requestError.message : "Unable to load the insight graph.");
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const layout = useMemo(() => {
    if (!graph) return null;
    return createInsightGraphLayout(graph, safePretextMeasure);
  }, [graph]);

  const nodes = useMemo(() => {
    if (!layout) return [];
    return layout.nodes.map((node) => ({ ...node, ...(nodePositions[node.id] ?? {}) }));
  }, [layout, nodePositions]);

  const nodesById = useMemo(() => {
    const map = new Map<string, GraphLayoutNode>();
    for (const node of nodes) {
      map.set(node.id, node);
    }
    return map;
  }, [nodes]);

  const selectedNode = selectedNodeId ? nodesById.get(selectedNodeId) ?? null : null;

  function startPan(event: PointerEvent<SVGRectElement>) {
    if (!viewportRef.current || !svgRef.current) return;
    svgRef.current.setPointerCapture(event.pointerId);
    setDrag({
      type: "pan",
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      scrollLeft: viewportRef.current.scrollLeft,
      scrollTop: viewportRef.current.scrollTop
    });
  }

  function startNodeDrag(event: PointerEvent<SVGGElement>, node: GraphLayoutNode) {
    event.stopPropagation();
    if (!svgRef.current) return;
    svgRef.current.setPointerCapture(event.pointerId);
    setDrag({
      type: "node",
      pointerId: event.pointerId,
      nodeId: node.id,
      startX: event.clientX,
      startY: event.clientY,
      nodeX: node.x,
      nodeY: node.y,
      moved: false
    });
  }

  function handlePointerMove(event: PointerEvent<SVGSVGElement>) {
    if (!drag) return;
    if (drag.type === "pan") {
      if (!viewportRef.current) return;
      viewportRef.current.scrollLeft = drag.scrollLeft - (event.clientX - drag.startX);
      viewportRef.current.scrollTop = drag.scrollTop - (event.clientY - drag.startY);
      return;
    }
    const deltaX = (event.clientX - drag.startX) / scale;
    const deltaY = (event.clientY - drag.startY) / scale;
    setDrag({ ...drag, moved: drag.moved || Math.abs(deltaX) + Math.abs(deltaY) > 4 });
    setNodePositions((positions) => ({
      ...positions,
      [drag.nodeId]: {
        x: Math.max(32, drag.nodeX + deltaX),
        y: Math.max(32, drag.nodeY + deltaY)
      }
    }));
  }

  function handlePointerUp(event: PointerEvent<SVGSVGElement>) {
    if (!drag) return;
    if (svgRef.current?.hasPointerCapture(event.pointerId)) {
      svgRef.current.releasePointerCapture(event.pointerId);
    }
    if (drag.type === "node" && !drag.moved) {
      setSelectedNodeId(drag.nodeId);
    }
    setDrag(null);
  }

  function resetView() {
    setScale(1);
    setNodePositions({});
    if (viewportRef.current) {
      viewportRef.current.scrollLeft = 0;
      viewportRef.current.scrollTop = 0;
    }
  }

  function fitGraph() {
    if (!viewportRef.current || !layout) return;
    const nextScale = Math.max(0.45, Math.min(1, (viewportRef.current.clientWidth - 28) / layout.width, (viewportRef.current.clientHeight - 28) / layout.height));
    setScale(nextScale);
    requestAnimationFrame(() => {
      if (!viewportRef.current) return;
      viewportRef.current.scrollLeft = 0;
      viewportRef.current.scrollTop = 0;
    });
  }

  return (
    <section className="knowledge-graph-panel" aria-label="Insight knowledge graph">
      <div className="graph-toolbar">
        <div className="graph-stats" aria-label="Graph statistics">
          <span>{graph?.stats.returnedInsights ?? 0} insights</span>
          <span>{graph?.stats.returnedEdges ?? 0} links</span>
          <span>{graph?.stats.totalInsights ?? 0} total</span>
        </div>
        <div className="graph-controls">
          <button onClick={fitGraph} type="button">
            Fit
          </button>
          <button onClick={resetView} type="button">
            Reset
          </button>
        </div>
      </div>

      {error ? <div className="error-banner">{error}</div> : null}
      {isLoading ? (
        <div className="loading-strip" role="status">
          <span className="loading-spinner" aria-hidden="true" />
          Loading Neo4j insight graph
        </div>
      ) : null}

      {layout && !error ? (
        <div className="graph-stage">
          <div ref={viewportRef} className="graph-viewport">
            <svg
              ref={svgRef}
              aria-label="Draggable insight graph"
              className="graph-svg"
              height={layout.height * scale}
              onPointerCancel={handlePointerUp}
              onPointerMove={handlePointerMove}
              onPointerUp={handlePointerUp}
              role="img"
              width={layout.width * scale}
            >
              <g transform={`scale(${scale})`}>
                <rect className="graph-canvas-bg" height={layout.height} onPointerDown={startPan} width={layout.width} x={0} y={0} />
                <g className="graph-edges">
                  {layout.edges.map((edge) => {
                    const source = nodesById.get(edge.source);
                    const target = nodesById.get(edge.target);
                    if (!source || !target) return null;
                    return (
                      <line
                        key={edge.id}
                        className="graph-edge"
                        stroke={edge.color}
                        strokeDasharray={edge.dash}
                        strokeWidth={Math.max(1.2, edge.weight)}
                        x1={source.x}
                        x2={target.x}
                        y1={source.y}
                        y2={target.y}
                      />
                    );
                  })}
                </g>
                <g className="graph-nodes">
                  {nodes.map((node) => (
                    <g
                      key={node.id}
                      aria-label={node.label}
                      className={`graph-node ${selectedNodeId === node.id ? "selected" : ""}`}
                      onPointerDown={(event) => startNodeDrag(event, node)}
                      role="button"
                      tabIndex={0}
                      transform={`translate(${node.x}, ${node.y})`}
                    >
                      <rect
                        fill={node.color}
                        height={node.height}
                        rx={8}
                        ry={8}
                        width={node.width}
                        x={-node.width / 2}
                        y={-node.height / 2}
                      />
                      <text textAnchor="middle" y={-(node.labelLines.length - 1) * 8}>
                        {node.labelLines.map((line, index) => (
                          <tspan key={`${node.id}-line-${index}`} x={0} dy={index === 0 ? 0 : 16}>
                            {line}
                          </tspan>
                        ))}
                      </text>
                    </g>
                  ))}
                </g>
              </g>
            </svg>
          </div>
          <InsightDetails node={selectedNode} />
        </div>
      ) : null}
    </section>
  );
}

function InsightDetails({ node }: { node: InsightGraphNode | null }) {
  if (!node) {
    return (
      <aside className="graph-details">
        <span className="feed-source insight">Selection</span>
        <h2>No insight selected</h2>
        <p>Click a node to inspect its canonical form, topic signals, and source link.</p>
      </aside>
    );
  }
  return (
    <aside className="graph-details">
      <span className="feed-source insight">Insight</span>
      <h2>{node.label}</h2>
      {node.canonicalInsight ? <p>{node.canonicalInsight}</p> : null}
      {node.mechanism ? (
        <dl>
          <dt>Mechanism</dt>
          <dd>{node.mechanism}</dd>
        </dl>
      ) : null}
      <div className="graph-detail-grid">
        {node.domain ? <span>{node.domain}</span> : null}
        {node.type ? <span>{node.type}</span> : null}
        {node.confidence ? <span>{node.confidence}</span> : null}
      </div>
      {node.topics.length ? (
        <div className="graph-topic-list">
          {node.topics.slice(0, 8).map((topic) => (
            <span key={`${node.id}-${topic}`}>{topic}</span>
          ))}
        </div>
      ) : null}
      {node.sourceUrl ? (
        <a className="graph-source-link" href={node.sourceUrl} rel="noreferrer" target="_blank">
          Open source
        </a>
      ) : null}
    </aside>
  );
}

function safePretextMeasure(text: string, maxWidth: number, font: string, lineHeight: number) {
  try {
    return measurePretextGraphLabel(text, maxWidth, font, lineHeight);
  } catch {
    return fallbackGraphLabelMeasure(text, maxWidth, font, lineHeight);
  }
}
