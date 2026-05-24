export type CanvasPoint = {
  x: number;
  y: number;
};

export type CanvasRect = CanvasPoint & {
  width: number;
  height: number;
};

export type CanvasViewport = {
  x: number;
  y: number;
  zoom: number;
};

export const minCanvasZoom = 0.1;
export const maxCanvasZoom = 4;

export function clampCanvasZoom(value: number) {
  return Math.max(minCanvasZoom, Math.min(maxCanvasZoom, value));
}

export function screenPointToCanvas(point: CanvasPoint, viewport: CanvasViewport): CanvasPoint {
  return {
    x: (point.x - viewport.x) / viewport.zoom,
    y: (point.y - viewport.y) / viewport.zoom
  };
}

export function zoomViewportAtScreenPoint(viewport: CanvasViewport, screenPoint: CanvasPoint, nextZoomValue: number): CanvasViewport {
  const zoom = clampCanvasZoom(nextZoomValue);
  const canvasPoint = screenPointToCanvas(screenPoint, viewport);
  return {
    x: screenPoint.x - canvasPoint.x * zoom,
    y: screenPoint.y - canvasPoint.y * zoom,
    zoom
  };
}

export function normalizeCanvasRect(start: CanvasPoint, end: CanvasPoint): CanvasRect {
  const x = Math.min(start.x, end.x);
  const y = Math.min(start.y, end.y);
  return {
    x,
    y,
    width: Math.abs(end.x - start.x),
    height: Math.abs(end.y - start.y)
  };
}

export function canvasRectsIntersect(first: CanvasRect, second: CanvasRect) {
  return (
    first.x <= second.x + second.width &&
    first.x + first.width >= second.x &&
    first.y <= second.y + second.height &&
    first.y + first.height >= second.y
  );
}

export function unionCanvasRects(rects: CanvasRect[]): CanvasRect | null {
  if (!rects.length) return null;
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (const rect of rects) {
    minX = Math.min(minX, rect.x);
    minY = Math.min(minY, rect.y);
    maxX = Math.max(maxX, rect.x + rect.width);
    maxY = Math.max(maxY, rect.y + rect.height);
  }
  return {
    x: minX,
    y: minY,
    width: maxX - minX,
    height: maxY - minY
  };
}
