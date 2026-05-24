import type { InsightGraphEdge, InsightGraphNode, InsightGraphResponse } from "../contracts";

export type GraphLabelLayout = {
  width: number;
  height: number;
  lines: string[];
};

export type GraphLayoutNode = InsightGraphNode & {
  x: number;
  y: number;
  width: number;
  height: number;
  radius: number;
  color: string;
  labelLines: string[];
};

export type GraphLayoutEdge = InsightGraphEdge & {
  color: string;
  dash?: string;
};

export type InsightGraphLayout = {
  nodes: GraphLayoutNode[];
  edges: GraphLayoutEdge[];
  width: number;
  height: number;
};

export type GraphLabelMeasurer = (text: string, maxWidth: number, font: string, lineHeight: number) => GraphLabelLayout;

const graphFont = "700 13px Inter, sans-serif";
const graphLineHeight = 17;
const labelMaxWidth = 156;
const nodePaddingX = 18;
const nodePaddingY = 14;
const minNodeWidth = 132;
const maxNodeWidth = 196;
const minNodeHeight = 74;
const horizontalGap = 250;
const verticalGap = 168;
const graphPadding = 140;

const palette = ["#285c8f", "#2d6d58", "#8a5517", "#5b5bd6", "#0f766e", "#805ad5", "#a13d30", "#3f6b73"];

export function createInsightGraphLayout(graph: InsightGraphResponse, measureLabel: GraphLabelMeasurer): InsightGraphLayout {
  const nodes = graph.nodes.map((node, index) => {
    const labelText = graphNodeLabel(node);
    const label = measureLabel(labelText, labelMaxWidth, graphFont, graphLineHeight);
    const columns = Math.max(1, Math.ceil(Math.sqrt(graph.nodes.length)));
    const row = Math.floor(index / columns);
    const column = index % columns;
    const width = clamp(label.width + nodePaddingX * 2, minNodeWidth, maxNodeWidth);
    const height = Math.max(minNodeHeight, label.height + nodePaddingY * 2 + 18);
    const stagger = row % 2 === 0 ? 0 : horizontalGap * 0.34;

    return {
      ...node,
      x: graphPadding + column * horizontalGap + stagger,
      y: graphPadding + row * verticalGap + hashOffset(node.id, 34),
      width,
      height,
      radius: Math.max(width, height) / 2,
      color: graphNodeColor(node),
      labelLines: label.lines
    };
  });

  const rows = Math.max(1, Math.ceil(nodes.length / Math.max(1, Math.ceil(Math.sqrt(nodes.length)))));
  const columns = Math.max(1, Math.ceil(Math.sqrt(nodes.length)));
  return {
    nodes,
    edges: graph.edges.map((edge) => ({
      ...edge,
      color: edgeColor(edge.reason),
      dash: edge.reason === "same_capture" ? undefined : edge.reason === "shared_topic" ? "8 7" : "3 7"
    })),
    width: Math.max(960, graphPadding * 2 + columns * horizontalGap),
    height: Math.max(640, graphPadding * 2 + rows * verticalGap)
  };
}

export function graphNodeLabel(node: InsightGraphNode) {
  const canonical = (node.canonicalInsight ?? "").trim();
  const label = node.label.trim();
  const text = canonical && canonical.toLowerCase() !== label.toLowerCase() ? `${label}. ${canonical}` : label;
  return truncateGraphLabel(text);
}

export function fallbackGraphLabelMeasure(text: string, maxWidth: number, _font: string, lineHeight: number): GraphLabelLayout {
  const averageCharWidth = 7.2;
  const words = text.split(/\s+/).filter(Boolean);
  const lines: string[] = [];
  let current = "";
  for (const word of words) {
    const next = current ? `${current} ${word}` : word;
    if (next.length * averageCharWidth > maxWidth && current) {
      lines.push(current);
      current = word;
    } else {
      current = next;
    }
    if (lines.length === 3) break;
  }
  if (current && lines.length < 4) {
    lines.push(current);
  }
  return {
    width: Math.min(maxWidth, Math.max(72, ...lines.map((line) => line.length * averageCharWidth))),
    height: Math.max(lineHeight, lines.length * lineHeight),
    lines
  };
}

export function edgeColor(reason: InsightGraphEdge["reason"]) {
  if (reason === "same_capture") return "#285c8f";
  if (reason === "shared_topic") return "#2d6d58";
  if (reason === "shared_domain") return "#8a5517";
  return "#6c7772";
}

function graphNodeColor(node: InsightGraphNode) {
  const key = (node.domain || node.type || node.topics[0] || node.id).toLowerCase();
  return palette[stableHash(key) % palette.length] ?? palette[0];
}

function truncateGraphLabel(value: string) {
  const collapsed = value.replace(/\s+/g, " ").trim();
  if (collapsed.length <= 130) return collapsed;
  return `${collapsed.slice(0, 127).trim()}...`;
}

function hashOffset(value: string, amplitude: number) {
  return (stableHash(value) % (amplitude * 2)) - amplitude;
}

function stableHash(value: string) {
  let hash = 0;
  for (let index = 0; index < value.length; index++) {
    hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
  }
  return hash;
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value));
}
