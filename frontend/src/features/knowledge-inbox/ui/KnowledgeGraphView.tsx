"use client";

import {
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type WheelEvent as ReactWheelEvent,
  useEffect,
  useMemo,
  useRef,
  useState
} from "react";
import {
  canvasRectsIntersect,
  clampCanvasZoom,
  normalizeCanvasRect,
  screenPointToCanvas,
  type CanvasPoint,
  type CanvasRect,
  type CanvasViewport,
  unionCanvasRects,
  zoomViewportAtScreenPoint
} from "../graph/canvasViewport";
import { readInsightGraph } from "../api/knowledgeRuns";
import type { InsightGraphResponse } from "../contracts";
import { createInsightGraphLayout, fallbackGraphLabelMeasure, type GraphLayoutNode } from "../graph/graphLayout";
import { measurePretextGraphLabel } from "../graph/pretextMeasure";

type CanvasTool = "select" | "hand" | "rectangle" | "ellipse" | "arrow" | "line" | "freehand" | "text";
type SelectionId = `node:${string}` | `shape:${string}`;

type RectangleCanvasShape = {
  id: string;
  type: "rectangle";
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
};

type EllipseCanvasShape = {
  id: string;
  type: "ellipse";
  x: number;
  y: number;
  width: number;
  height: number;
  color: string;
};

type LineCanvasShape = {
  id: string;
  type: "line";
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  color: string;
};

type ArrowCanvasShape = {
  id: string;
  type: "arrow";
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  color: string;
};

type FreehandCanvasShape = {
  id: string;
  type: "freehand";
  points: CanvasPoint[];
  color: string;
};

type TextCanvasShape = {
  id: string;
  type: "text";
  x: number;
  y: number;
  width: number;
  height: number;
  text: string;
  color: string;
};

type CanvasShape = RectangleCanvasShape | EllipseCanvasShape | LineCanvasShape | ArrowCanvasShape | FreehandCanvasShape | TextCanvasShape;

type CanvasState = {
  nodePositions: Record<string, CanvasPoint>;
  shapes: CanvasShape[];
};

type CanvasHistory = {
  past: CanvasState[];
  future: CanvasState[];
};

type DragState =
  | {
      type: "pan";
      pointerId: number;
      startClient: CanvasPoint;
      startViewport: CanvasViewport;
    }
  | {
      type: "move";
      pointerId: number;
      startCanvas: CanvasPoint;
      selection: SelectionId[];
      beforeState: CanvasState;
      nodeStarts: Record<string, CanvasPoint>;
      shapeStarts: Record<string, CanvasShape>;
      moved: boolean;
    }
  | {
      type: "select-box";
      pointerId: number;
      startCanvas: CanvasPoint;
      currentCanvas: CanvasPoint;
      addToSelection: boolean;
      moved: boolean;
    }
  | {
      type: "draw";
      pointerId: number;
      tool: Exclude<CanvasTool, "select" | "hand" | "text">;
      startCanvas: CanvasPoint;
      currentCanvas: CanvasPoint;
      points: CanvasPoint[];
      beforeState: CanvasState;
      moved: boolean;
    };

const graphLimit = 180;
const initialViewport: CanvasViewport = { x: 96, y: 76, zoom: 1 };
const historyLimit = 60;
const moveThreshold = 3;
const shapeColor = "#17201c";

export function KnowledgeGraphView() {
  const [graph, setGraph] = useState<InsightGraphResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [tool, setTool] = useState<CanvasTool>("select");
  const [isSpacePanning, setIsSpacePanning] = useState(false);
  const [viewport, setViewport] = useState<CanvasViewport>(initialViewport);
  const [canvasState, setCanvasState] = useState<CanvasState>({ nodePositions: {}, shapes: [] });
  const [history, setHistory] = useState<CanvasHistory>({ past: [], future: [] });
  const [selectionIds, setSelectionIds] = useState<SelectionId[]>([]);
  const [drag, setDrag] = useState<DragState | null>(null);
  const viewportElementRef = useRef<HTMLDivElement | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);
  const canvasStateRef = useRef(canvasState);
  const viewportRef = useRef(viewport);
  const selectionRef = useRef(selectionIds);
  const didAutoFitRef = useRef(false);

  useEffect(() => {
    canvasStateRef.current = canvasState;
  }, [canvasState]);

  useEffect(() => {
    viewportRef.current = viewport;
  }, [viewport]);

  useEffect(() => {
    selectionRef.current = selectionIds;
  }, [selectionIds]);

  useEffect(() => {
    let cancelled = false;
    readInsightGraph(graphLimit)
      .then((payload) => {
        if (cancelled) return;
        setGraph(payload);
        setError(null);
        setSelectionIds(payload.nodes[0] ? [nodeSelectionId(payload.nodes[0].id)] : []);
        setCanvasState({ nodePositions: {}, shapes: [] });
        setHistory({ past: [], future: [] });
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
    return layout.nodes.map((node) => ({ ...node, ...(canvasState.nodePositions[node.id] ?? {}) }));
  }, [canvasState.nodePositions, layout]);

  const nodesById = useMemo(() => {
    const map = new Map<string, GraphLayoutNode>();
    for (const node of nodes) {
      map.set(node.id, node);
    }
    return map;
  }, [nodes]);

  const shapesById = useMemo(() => {
    const map = new Map<string, CanvasShape>();
    for (const shape of canvasState.shapes) {
      map.set(shape.id, shape);
    }
    return map;
  }, [canvasState.shapes]);

  const effectiveTool: CanvasTool = isSpacePanning ? "hand" : tool;
  const selectedIdSet = useMemo(() => new Set(selectionIds), [selectionIds]);
  const selectionBounds = useMemo(() => selectedCanvasRects(selectionIds, nodesById, shapesById), [nodesById, selectionIds, shapesById]);
  const draftShape = drag?.type === "draw" ? draftShapeFromDrag(drag) : null;

  useEffect(() => {
    if (!layout || didAutoFitRef.current) return;
    didAutoFitRef.current = true;
    requestAnimationFrame(() => fitGraph());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layout]);

  useEffect(() => {
    function handleWindowKeyDown(event: KeyboardEvent) {
      if (isEditableTarget(event.target)) return;
      if (event.code === "Space") {
        event.preventDefault();
        setIsSpacePanning(true);
        return;
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "z") {
        event.preventDefault();
        if (event.shiftKey) {
          redoCanvas();
        } else {
          undoCanvas();
        }
        return;
      }
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "a") {
        event.preventDefault();
        selectAllCanvasObjects();
        return;
      }
      if (event.key === "Backspace" || event.key === "Delete") {
        event.preventDefault();
        deleteSelectedShapes();
        return;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDrag(null);
        setSelectionIds([]);
        setTool("select");
      }
    }

    function handleWindowKeyUp(event: KeyboardEvent) {
      if (event.code === "Space") {
        setIsSpacePanning(false);
      }
    }

    window.addEventListener("keydown", handleWindowKeyDown);
    window.addEventListener("keyup", handleWindowKeyUp);
    return () => {
      window.removeEventListener("keydown", handleWindowKeyDown);
      window.removeEventListener("keyup", handleWindowKeyUp);
    };
  });

  function eventScreenPoint(event: ReactPointerEvent | ReactWheelEvent): CanvasPoint {
    const rect = viewportElementRef.current?.getBoundingClientRect();
    if (!rect) return { x: 0, y: 0 };
    return {
      x: event.clientX - rect.left,
      y: event.clientY - rect.top
    };
  }

  function eventCanvasPoint(event: ReactPointerEvent | ReactWheelEvent): CanvasPoint {
    return screenPointToCanvas(eventScreenPoint(event), viewportRef.current);
  }

  function capturePointer(event: ReactPointerEvent) {
    svgRef.current?.setPointerCapture(event.pointerId);
  }

  function releasePointer(event: ReactPointerEvent) {
    if (svgRef.current?.hasPointerCapture(event.pointerId)) {
      svgRef.current.releasePointerCapture(event.pointerId);
    }
  }

  function recordHistory(beforeState: CanvasState) {
    setHistory((current) => ({
      past: [...current.past.slice(-(historyLimit - 1)), cloneCanvasState(beforeState)],
      future: []
    }));
  }

  function commitCanvasState(nextState: CanvasState, beforeState = canvasStateRef.current) {
    recordHistory(beforeState);
    setCanvasState(cloneCanvasState(nextState));
  }

  function startPan(event: ReactPointerEvent) {
    capturePointer(event);
    setDrag({
      type: "pan",
      pointerId: event.pointerId,
      startClient: { x: event.clientX, y: event.clientY },
      startViewport: viewportRef.current
    });
  }

  function startObjectMove(event: ReactPointerEvent, selection: SelectionId[]) {
    const beforeState = cloneCanvasState(canvasStateRef.current);
    const nodeStarts: Record<string, CanvasPoint> = {};
    const shapeStarts: Record<string, CanvasShape> = {};
    for (const id of selection) {
      const nodeId = getNodeSelectionId(id);
      if (nodeId) {
        const node = nodesById.get(nodeId);
        if (node) nodeStarts[nodeId] = { x: node.x, y: node.y };
      }
      const shapeId = getShapeSelectionId(id);
      if (shapeId) {
        const shape = shapesById.get(shapeId);
        if (shape) shapeStarts[shapeId] = cloneShape(shape);
      }
    }
    capturePointer(event);
    setDrag({
      type: "move",
      pointerId: event.pointerId,
      startCanvas: eventCanvasPoint(event),
      selection,
      beforeState,
      nodeStarts,
      shapeStarts,
      moved: false
    });
  }

  function handleNodePointerDown(event: ReactPointerEvent<SVGGElement>, node: GraphLayoutNode) {
    event.stopPropagation();
    if (event.button === 1 || effectiveTool === "hand") {
      startPan(event);
      return;
    }
    if (effectiveTool !== "select") return;
    const id = nodeSelectionId(node.id);
    const nextSelection = nextObjectSelection(id, event.shiftKey);
    setSelectionIds(nextSelection);
    if (nextSelection.includes(id)) {
      startObjectMove(event, nextSelection);
    }
  }

  function handleShapePointerDown(event: ReactPointerEvent<SVGGElement>, shape: CanvasShape) {
    event.stopPropagation();
    if (event.button === 1 || effectiveTool === "hand") {
      startPan(event);
      return;
    }
    if (effectiveTool !== "select") return;
    const id = shapeSelectionId(shape.id);
    const nextSelection = nextObjectSelection(id, event.shiftKey);
    setSelectionIds(nextSelection);
    if (nextSelection.includes(id)) {
      startObjectMove(event, nextSelection);
    }
  }

  function handleCanvasPointerDown(event: ReactPointerEvent<SVGSVGElement>) {
    if (event.button === 1 || effectiveTool === "hand") {
      event.preventDefault();
      startPan(event);
      return;
    }
    const startCanvas = eventCanvasPoint(event);
    if (effectiveTool === "select") {
      capturePointer(event);
      setDrag({
        type: "select-box",
        pointerId: event.pointerId,
        startCanvas,
        currentCanvas: startCanvas,
        addToSelection: event.shiftKey,
        moved: false
      });
      return;
    }
    if (effectiveTool === "text") {
      const id = createShapeId();
      const textShape: CanvasShape = {
        id,
        type: "text",
        x: startCanvas.x,
        y: startCanvas.y,
        width: 180,
        height: 58,
        text: "Text",
        color: shapeColor
      };
      commitCanvasState({ ...canvasStateRef.current, shapes: [...canvasStateRef.current.shapes, textShape] });
      setSelectionIds([shapeSelectionId(id)]);
      return;
    }
    capturePointer(event);
    setDrag({
      type: "draw",
      pointerId: event.pointerId,
      tool: effectiveTool,
      startCanvas,
      currentCanvas: startCanvas,
      points: [startCanvas],
      beforeState: cloneCanvasState(canvasStateRef.current),
      moved: false
    });
  }

  function handlePointerMove(event: ReactPointerEvent<SVGSVGElement>) {
    if (!drag) return;
    if (drag.type === "pan") {
      setViewport({
        ...drag.startViewport,
        x: drag.startViewport.x + event.clientX - drag.startClient.x,
        y: drag.startViewport.y + event.clientY - drag.startClient.y
      });
      return;
    }
    if (drag.type === "select-box") {
      const currentCanvas = eventCanvasPoint(event);
      const distance = distanceBetween(drag.startCanvas, currentCanvas);
      setDrag({ ...drag, currentCanvas, moved: drag.moved || distance > moveThreshold / viewportRef.current.zoom });
      return;
    }
    if (drag.type === "draw") {
      const currentCanvas = constrainDraftPoint(drag.startCanvas, eventCanvasPoint(event), drag.tool, event.shiftKey);
      const distance = distanceBetween(drag.startCanvas, currentCanvas);
      const nextPoints = drag.tool === "freehand" ? [...drag.points, currentCanvas] : drag.points;
      setDrag({ ...drag, currentCanvas, points: nextPoints, moved: drag.moved || distance > moveThreshold / viewportRef.current.zoom });
      return;
    }
    const currentCanvas = eventCanvasPoint(event);
    const delta = {
      x: currentCanvas.x - drag.startCanvas.x,
      y: currentCanvas.y - drag.startCanvas.y
    };
    const moved = drag.moved || Math.abs(delta.x) + Math.abs(delta.y) > moveThreshold / viewportRef.current.zoom;
    setDrag({ ...drag, moved });
    setCanvasState((current) => {
      const nextNodePositions = { ...current.nodePositions };
      for (const [nodeId, start] of Object.entries(drag.nodeStarts)) {
        nextNodePositions[nodeId] = { x: start.x + delta.x, y: start.y + delta.y };
      }
      return {
        nodePositions: nextNodePositions,
        shapes: current.shapes.map((shape) => {
          const start = drag.shapeStarts[shape.id];
          return start ? translateShape(start, delta) : shape;
        })
      };
    });
  }

  function handlePointerUp(event: ReactPointerEvent<SVGSVGElement>) {
    if (!drag) return;
    releasePointer(event);
    if (drag.type === "move" && drag.moved) {
      recordHistory(drag.beforeState);
    }
    if (drag.type === "select-box") {
      if (!drag.moved) {
        setSelectionIds(drag.addToSelection ? selectionRef.current : []);
      } else {
        const rect = normalizeCanvasRect(drag.startCanvas, drag.currentCanvas);
        const selected = objectsInRect(rect, nodes, canvasStateRef.current.shapes);
        setSelectionIds(drag.addToSelection ? mergeSelections(selectionRef.current, selected) : selected);
      }
    }
    if (drag.type === "draw") {
      const shape = draftShapeFromDrag(drag);
      if (shape && drag.moved) {
        commitCanvasState({ ...canvasStateRef.current, shapes: [...canvasStateRef.current.shapes, shape] }, drag.beforeState);
        setSelectionIds([shapeSelectionId(shape.id)]);
      }
    }
    setDrag(null);
  }

  function handleWheel(event: ReactWheelEvent<HTMLDivElement>) {
    event.preventDefault();
    const deltaX = normalizeWheelDelta(event.deltaX, event.deltaMode);
    const deltaY = normalizeWheelDelta(event.deltaY, event.deltaMode);
    if (event.metaKey || event.ctrlKey) {
      const zoomFactor = Math.exp(-deltaY * 0.0016);
      setViewport((current) => zoomViewportAtScreenPoint(current, eventScreenPoint(event), current.zoom * zoomFactor));
      return;
    }
    setViewport((current) => ({
      ...current,
      x: current.x - deltaX,
      y: current.y - deltaY
    }));
  }

  function zoomFromButton(direction: 1 | -1) {
    const element = viewportElementRef.current;
    if (!element) return;
    const screenPoint = { x: element.clientWidth / 2, y: element.clientHeight / 2 };
    setViewport((current) => zoomViewportAtScreenPoint(current, screenPoint, current.zoom * (direction > 0 ? 1.2 : 1 / 1.2)));
  }

  function resetView() {
    setViewport(initialViewport);
    setCanvasState((current) => ({ ...current, nodePositions: {} }));
  }

  function fitGraph() {
    const element = viewportElementRef.current;
    if (!element) return;
    const bounds = unionCanvasRects([...nodes.map(nodeBounds), ...canvasStateRef.current.shapes.map(shapeBounds)]);
    if (!bounds) return;
    const paddedWidth = bounds.width + 160;
    const paddedHeight = bounds.height + 160;
    const nextZoom = clampCanvasZoom(Math.min(1.15, element.clientWidth / paddedWidth, element.clientHeight / paddedHeight));
    setViewport({
      x: (element.clientWidth - bounds.width * nextZoom) / 2 - bounds.x * nextZoom,
      y: (element.clientHeight - bounds.height * nextZoom) / 2 - bounds.y * nextZoom,
      zoom: nextZoom
    });
  }

  function undoCanvas() {
    setHistory((current) => {
      const previous = current.past.at(-1);
      if (!previous) return current;
      const present = cloneCanvasState(canvasStateRef.current);
      setCanvasState(cloneCanvasState(previous));
      setSelectionIds([]);
      return {
        past: current.past.slice(0, -1),
        future: [present, ...current.future].slice(0, historyLimit)
      };
    });
  }

  function redoCanvas() {
    setHistory((current) => {
      const next = current.future[0];
      if (!next) return current;
      const present = cloneCanvasState(canvasStateRef.current);
      setCanvasState(cloneCanvasState(next));
      setSelectionIds([]);
      return {
        past: [...current.past, present].slice(-historyLimit),
        future: current.future.slice(1)
      };
    });
  }

  function selectAllCanvasObjects() {
    setSelectionIds([...nodes.map((node) => nodeSelectionId(node.id)), ...canvasStateRef.current.shapes.map((shape) => shapeSelectionId(shape.id))]);
  }

  function deleteSelectedShapes() {
    const selectedShapeIds = new Set(selectionRef.current.map(getShapeSelectionId).filter(Boolean));
    if (!selectedShapeIds.size) {
      setSelectionIds([]);
      return;
    }
    const beforeState = cloneCanvasState(canvasStateRef.current);
    const nextShapes = beforeState.shapes.filter((shape) => !selectedShapeIds.has(shape.id));
    commitCanvasState({ ...beforeState, shapes: nextShapes }, beforeState);
    setSelectionIds(selectionRef.current.filter((id) => !getShapeSelectionId(id)));
  }

  function updateShapeText(id: string, text: string) {
    setCanvasState((current) => ({
      ...current,
      shapes: current.shapes.map((shape) => (shape.id === id && shape.type === "text" ? { ...shape, text } : shape))
    }));
  }

  function nextObjectSelection(id: SelectionId, shiftKey: boolean) {
    const current = selectionRef.current;
    if (!shiftKey && current.includes(id)) return current;
    if (!shiftKey) return [id];
    if (current.includes(id)) {
      return current.filter((selectedId) => selectedId !== id);
    }
    return [...current, id];
  }

  return (
    <section className="knowledge-graph-panel" aria-label="Insight knowledge graph">
      <div className="graph-toolbar">
        <div className="graph-stats" aria-label="Graph statistics">
          <span>{graph?.stats.returnedInsights ?? 0} insights</span>
          <span>{graph?.stats.returnedEdges ?? 0} links</span>
          <span>{graph?.stats.totalInsights ?? 0} total</span>
        </div>
        <div className="graph-toolset" aria-label="Canvas tools">
          <ToolButton active={effectiveTool === "select"} icon="select" label="Select" onClick={() => setTool("select")} />
          <ToolButton active={effectiveTool === "hand"} icon="hand" label="Pan" onClick={() => setTool("hand")} />
          <ToolButton active={tool === "rectangle"} icon="rectangle" label="Rectangle" onClick={() => setTool("rectangle")} />
          <ToolButton active={tool === "ellipse"} icon="ellipse" label="Ellipse" onClick={() => setTool("ellipse")} />
          <ToolButton active={tool === "arrow"} icon="arrow" label="Arrow" onClick={() => setTool("arrow")} />
          <ToolButton active={tool === "line"} icon="line" label="Line" onClick={() => setTool("line")} />
          <ToolButton active={tool === "freehand"} icon="freehand" label="Freehand" onClick={() => setTool("freehand")} />
          <ToolButton active={tool === "text"} icon="text" label="Text" onClick={() => setTool("text")} />
        </div>
        <div className="graph-controls" aria-label="Viewport controls">
          <button aria-label="Undo" disabled={!history.past.length} onClick={undoCanvas} title="Undo" type="button">
            <Icon name="undo" />
          </button>
          <button aria-label="Redo" disabled={!history.future.length} onClick={redoCanvas} title="Redo" type="button">
            <Icon name="redo" />
          </button>
          <button aria-label="Zoom out" onClick={() => zoomFromButton(-1)} title="Zoom out" type="button">
            <Icon name="minus" />
          </button>
          <span className="graph-zoom-readout">{Math.round(viewport.zoom * 100)}%</span>
          <button aria-label="Zoom in" onClick={() => zoomFromButton(1)} title="Zoom in" type="button">
            <Icon name="plus" />
          </button>
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
          <div
            ref={viewportElementRef}
            className={`graph-viewport graph-viewport-${effectiveTool}`}
            onKeyDown={handleViewportKeyDown}
            onWheel={handleWheel}
            tabIndex={0}
          >
            <svg
              ref={svgRef}
              aria-label="Interactive insight graph canvas"
              className="graph-svg"
              onPointerCancel={handlePointerUp}
              onPointerDown={handleCanvasPointerDown}
              onPointerMove={handlePointerMove}
              onPointerUp={handlePointerUp}
              role="application"
            >
              <defs>
                <pattern height="36" id="graph-grid" patternUnits="userSpaceOnUse" width="36">
                  <path d="M 36 0 L 0 0 0 36" />
                </pattern>
                <marker id="graph-arrowhead" markerHeight="8" markerWidth="8" orient="auto" refX="7" refY="4">
                  <path d="M 0 0 L 8 4 L 0 8 z" />
                </marker>
              </defs>
              <rect className="graph-screen-bg" height="100%" width="100%" x="0" y="0" />
              <g transform={`translate(${viewport.x}, ${viewport.y}) scale(${viewport.zoom})`}>
                <rect className="graph-canvas-grid" height="100000" width="100000" x="-50000" y="-50000" />
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
                <g className="graph-shapes">
                  {canvasState.shapes.map((shape) => (
                    <CanvasShapeView
                      key={shape.id}
                      isSelected={selectedIdSet.has(shapeSelectionId(shape.id))}
                      onPointerDown={(event) => handleShapePointerDown(event, shape)}
                      onTextChange={updateShapeText}
                      shape={shape}
                    />
                  ))}
                  {draftShape ? <CanvasShapeView isPreview shape={draftShape} /> : null}
                </g>
                <g className="graph-nodes">
                  {nodes.map((node) => (
                    <g
                      key={node.id}
                      aria-label={node.label}
                      className={`graph-node ${selectedIdSet.has(nodeSelectionId(node.id)) ? "selected" : ""}`}
                      onPointerDown={(event) => handleNodePointerDown(event, node)}
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
                {selectionBounds.map((rect, index) => (
                  <SelectionOutline key={`${rect.x}-${rect.y}-${index}`} rect={rect} zoom={viewport.zoom} />
                ))}
                {drag?.type === "select-box" && drag.moved ? <SelectionMarquee rect={normalizeCanvasRect(drag.startCanvas, drag.currentCanvas)} /> : null}
              </g>
            </svg>
          </div>
        </div>
      ) : null}
    </section>
  );
}

function handleViewportKeyDown(event: ReactKeyboardEvent<HTMLDivElement>) {
  if (event.code === "Space") {
    event.preventDefault();
  }
}

function ToolButton({ active, icon, label, onClick }: { active: boolean; icon: CanvasTool; label: string; onClick: () => void }) {
  return (
    <button aria-label={label} className={active ? "active" : undefined} onClick={onClick} title={label} type="button">
      <ToolIcon name={icon} />
    </button>
  );
}

function ToolIcon({ name }: { name: CanvasTool }) {
  if (name === "select") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d="M5 4l12 8-6 1.3 3.2 5.6-2.7 1.5-3.1-5.5-4.4 4.3z" />
      </svg>
    );
  }
  if (name === "hand") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d="M7 12V7.6a1.4 1.4 0 0 1 2.8 0V11M9.8 11V6.4a1.4 1.4 0 0 1 2.8 0V11M12.6 11V7.2a1.4 1.4 0 0 1 2.8 0v5.4M15.4 12.6v-2.2a1.4 1.4 0 0 1 2.8 0v4.9c0 3.1-2 5.2-5.1 5.2h-.8c-2.2 0-3.8-1-5.1-3L5.1 14a1.4 1.4 0 0 1 2.3-1.5l1.2 1.7" />
      </svg>
    );
  }
  if (name === "rectangle") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <rect height="13" rx="2" width="15" x="4.5" y="5.5" />
      </svg>
    );
  }
  if (name === "ellipse") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <ellipse cx="12" cy="12" rx="7.5" ry="5.8" />
      </svg>
    );
  }
  if (name === "arrow") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d="M5 18 18 5M11 5h7v7" />
      </svg>
    );
  }
  if (name === "line") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d="M5 18 19 6" />
      </svg>
    );
  }
  if (name === "freehand") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d="M4 15c3-7 5 7 8 0s5-2 8-7" />
      </svg>
    );
  }
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path d="M6 6h12M12 6v13M8.5 19h7" />
    </svg>
  );
}

function Icon({ name }: { name: "undo" | "redo" | "minus" | "plus" }) {
  if (name === "undo" || name === "redo") {
    return (
      <svg aria-hidden="true" viewBox="0 0 24 24">
        <path d={name === "undo" ? "M9 8H4V3M5 8a8 8 0 1 1 2 8" : "M15 8h5V3M19 8a8 8 0 1 0-2 8"} />
      </svg>
    );
  }
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24">
      <path d={name === "plus" ? "M12 5v14M5 12h14" : "M5 12h14"} />
    </svg>
  );
}

function CanvasShapeView({
  isPreview = false,
  isSelected = false,
  onPointerDown,
  onTextChange,
  shape
}: {
  isPreview?: boolean;
  isSelected?: boolean;
  onPointerDown?: (event: ReactPointerEvent<SVGGElement>) => void;
  onTextChange?: (id: string, text: string) => void;
  shape: CanvasShape;
}) {
  const className = `canvas-shape ${isPreview ? "preview" : ""} ${isSelected ? "selected" : ""}`;
  if (shape.type === "rectangle") {
    return (
      <g className={className} onPointerDown={onPointerDown}>
        <rect height={shape.height} width={shape.width} x={shape.x} y={shape.y} />
      </g>
    );
  }
  if (shape.type === "ellipse") {
    return (
      <g className={className} onPointerDown={onPointerDown}>
        <ellipse cx={shape.x + shape.width / 2} cy={shape.y + shape.height / 2} rx={shape.width / 2} ry={shape.height / 2} />
      </g>
    );
  }
  if (shape.type === "line" || shape.type === "arrow") {
    return (
      <g className={className} onPointerDown={onPointerDown}>
        <line markerEnd={shape.type === "arrow" ? "url(#graph-arrowhead)" : undefined} x1={shape.x1} x2={shape.x2} y1={shape.y1} y2={shape.y2} />
      </g>
    );
  }
  if (shape.type === "freehand") {
    return (
      <g className={className} onPointerDown={onPointerDown}>
        <polyline points={shape.points.map((point) => `${point.x},${point.y}`).join(" ")} />
      </g>
    );
  }
  if (isSelected && onTextChange) {
    return (
      <g className={className} onPointerDown={onPointerDown}>
        <foreignObject height={shape.height} width={shape.width} x={shape.x} y={shape.y}>
          <textarea
            className="canvas-text-editor"
            onChange={(event) => onTextChange(shape.id, event.target.value)}
            onPointerDown={(event) => event.stopPropagation()}
            value={shape.text}
          />
        </foreignObject>
      </g>
    );
  }
  return (
    <g className={className} onPointerDown={onPointerDown}>
      <text x={shape.x} y={shape.y + 22}>
        {shape.text || "Text"}
      </text>
    </g>
  );
}

function SelectionOutline({ rect, zoom }: { rect: CanvasRect; zoom: number }) {
  const handleSize = 8 / zoom;
  const strokeWidth = 1.4 / zoom;
  const handles = [
    { x: rect.x, y: rect.y },
    { x: rect.x + rect.width, y: rect.y },
    { x: rect.x, y: rect.y + rect.height },
    { x: rect.x + rect.width, y: rect.y + rect.height }
  ];
  return (
    <g className="canvas-selection-outline" pointerEvents="none">
      <rect height={rect.height} strokeWidth={strokeWidth} width={rect.width} x={rect.x} y={rect.y} />
      {handles.map((handle) => (
        <rect
          height={handleSize}
          key={`${handle.x}-${handle.y}`}
          strokeWidth={strokeWidth}
          width={handleSize}
          x={handle.x - handleSize / 2}
          y={handle.y - handleSize / 2}
        />
      ))}
    </g>
  );
}

function SelectionMarquee({ rect }: { rect: CanvasRect }) {
  return <rect className="canvas-selection-marquee" height={rect.height} width={rect.width} x={rect.x} y={rect.y} />;
}

function selectedCanvasRects(selectionIds: SelectionId[], nodesById: Map<string, GraphLayoutNode>, shapesById: Map<string, CanvasShape>) {
  const rects: CanvasRect[] = [];
  for (const id of selectionIds) {
    const nodeId = getNodeSelectionId(id);
    if (nodeId) {
      const node = nodesById.get(nodeId);
      if (node) rects.push(nodeBounds(node));
    }
    const shapeId = getShapeSelectionId(id);
    if (shapeId) {
      const shape = shapesById.get(shapeId);
      if (shape) rects.push(shapeBounds(shape));
    }
  }
  return rects;
}

function objectsInRect(rect: CanvasRect, nodes: GraphLayoutNode[], shapes: CanvasShape[]): SelectionId[] {
  return [
    ...nodes.filter((node) => canvasRectsIntersect(rect, nodeBounds(node))).map((node) => nodeSelectionId(node.id)),
    ...shapes.filter((shape) => canvasRectsIntersect(rect, shapeBounds(shape))).map((shape) => shapeSelectionId(shape.id))
  ];
}

function nodeBounds(node: GraphLayoutNode): CanvasRect {
  return {
    x: node.x - node.width / 2,
    y: node.y - node.height / 2,
    width: node.width,
    height: node.height
  };
}

function shapeBounds(shape: CanvasShape): CanvasRect {
  if (shape.type === "rectangle" || shape.type === "ellipse" || shape.type === "text") {
    return { x: shape.x, y: shape.y, width: shape.width, height: shape.height };
  }
  if (shape.type === "line" || shape.type === "arrow") {
    return normalizeCanvasRect({ x: shape.x1, y: shape.y1 }, { x: shape.x2, y: shape.y2 });
  }
  return unionCanvasRects(shape.points.map((point) => ({ x: point.x, y: point.y, width: 1, height: 1 }))) ?? {
    x: 0,
    y: 0,
    width: 1,
    height: 1
  };
}

function draftShapeFromDrag(drag: Extract<DragState, { type: "draw" }>): CanvasShape | null {
  const id = "draft";
  if (drag.tool === "freehand") {
    if (drag.points.length < 2) return null;
    return { id, type: "freehand", points: drag.points, color: shapeColor };
  }
  if (drag.tool === "line" || drag.tool === "arrow") {
    return { id: createShapeId(), type: drag.tool, x1: drag.startCanvas.x, y1: drag.startCanvas.y, x2: drag.currentCanvas.x, y2: drag.currentCanvas.y, color: shapeColor };
  }
  const rect = normalizeCanvasRect(drag.startCanvas, drag.currentCanvas);
  if (rect.width < 2 || rect.height < 2) return null;
  return { id: createShapeId(), type: drag.tool, ...rect, color: shapeColor };
}

function constrainDraftPoint(start: CanvasPoint, current: CanvasPoint, tool: CanvasTool, shiftKey: boolean): CanvasPoint {
  if (!shiftKey) return current;
  const deltaX = current.x - start.x;
  const deltaY = current.y - start.y;
  if (tool === "rectangle" || tool === "ellipse") {
    const size = Math.max(Math.abs(deltaX), Math.abs(deltaY));
    return {
      x: start.x + Math.sign(deltaX || 1) * size,
      y: start.y + Math.sign(deltaY || 1) * size
    };
  }
  if (tool === "line" || tool === "arrow") {
    const angle = Math.atan2(deltaY, deltaX);
    const snapped = Math.round(angle / (Math.PI / 4)) * (Math.PI / 4);
    const length = Math.hypot(deltaX, deltaY);
    return {
      x: start.x + Math.cos(snapped) * length,
      y: start.y + Math.sin(snapped) * length
    };
  }
  return current;
}

function translateShape(shape: CanvasShape, delta: CanvasPoint): CanvasShape {
  if (shape.type === "rectangle" || shape.type === "ellipse" || shape.type === "text") {
    return { ...shape, x: shape.x + delta.x, y: shape.y + delta.y };
  }
  if (shape.type === "line" || shape.type === "arrow") {
    return { ...shape, x1: shape.x1 + delta.x, y1: shape.y1 + delta.y, x2: shape.x2 + delta.x, y2: shape.y2 + delta.y };
  }
  return { ...shape, points: shape.points.map((point) => ({ x: point.x + delta.x, y: point.y + delta.y })) };
}

function cloneCanvasState(state: CanvasState): CanvasState {
  return {
    nodePositions: Object.fromEntries(Object.entries(state.nodePositions).map(([key, value]) => [key, { ...value }])),
    shapes: state.shapes.map(cloneShape)
  };
}

function cloneShape(shape: CanvasShape): CanvasShape {
  if (shape.type === "freehand") {
    return { ...shape, points: shape.points.map((point) => ({ ...point })) };
  }
  return { ...shape };
}

function mergeSelections(current: SelectionId[], next: SelectionId[]) {
  return Array.from(new Set([...current, ...next]));
}

function nodeSelectionId(id: string): SelectionId {
  return `node:${id}`;
}

function shapeSelectionId(id: string): SelectionId {
  return `shape:${id}`;
}

function getNodeSelectionId(id: SelectionId) {
  return id.startsWith("node:") ? id.slice("node:".length) : "";
}

function getShapeSelectionId(id: SelectionId) {
  return id.startsWith("shape:") ? id.slice("shape:".length) : "";
}

function distanceBetween(first: CanvasPoint, second: CanvasPoint) {
  return Math.hypot(first.x - second.x, first.y - second.y);
}

function normalizeWheelDelta(value: number, mode: number) {
  if (mode === 1) return value * 16;
  if (mode === 2) return value * 800;
  return value;
}

function createShapeId() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `shape-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  return target.isContentEditable || target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.tagName === "SELECT";
}

function safePretextMeasure(text: string, maxWidth: number, font: string, lineHeight: number) {
  try {
    return measurePretextGraphLabel(text, maxWidth, font, lineHeight);
  } catch {
    return fallbackGraphLabelMeasure(text, maxWidth, font, lineHeight);
  }
}
