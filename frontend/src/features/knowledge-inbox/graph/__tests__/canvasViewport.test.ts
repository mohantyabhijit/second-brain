import { describe, expect, it } from "vitest";
import {
  canvasRectsIntersect,
  clampCanvasZoom,
  normalizeCanvasRect,
  screenPointToCanvas,
  unionCanvasRects,
  zoomViewportAtScreenPoint
} from "../canvasViewport";

describe("canvas viewport helpers", () => {
  it("converts screen positions into canvas coordinates", () => {
    expect(screenPointToCanvas({ x: 140, y: 90 }, { x: 40, y: -10, zoom: 2 })).toEqual({ x: 50, y: 50 });
  });

  it("zooms around the pointer without moving the focused canvas point", () => {
    const viewport = { x: 20, y: 40, zoom: 1 };
    const pointer = { x: 220, y: 140 };
    const before = screenPointToCanvas(pointer, viewport);
    const next = zoomViewportAtScreenPoint(viewport, pointer, 2.5);

    expect(next.zoom).toBe(2.5);
    expect(screenPointToCanvas(pointer, next)).toEqual(before);
  });

  it("clamps zoom and normalizes selection rectangles", () => {
    expect(clampCanvasZoom(0.02)).toBe(0.1);
    expect(clampCanvasZoom(9)).toBe(4);
    expect(normalizeCanvasRect({ x: 120, y: 80 }, { x: 20, y: 140 })).toEqual({ x: 20, y: 80, width: 100, height: 60 });
  });

  it("detects intersecting canvas bounds and unions multiple rects", () => {
    expect(canvasRectsIntersect({ x: 0, y: 0, width: 10, height: 10 }, { x: 9, y: 9, width: 4, height: 4 })).toBe(true);
    expect(canvasRectsIntersect({ x: 0, y: 0, width: 10, height: 10 }, { x: 30, y: 30, width: 4, height: 4 })).toBe(false);
    expect(
      unionCanvasRects([
        { x: 10, y: 20, width: 30, height: 40 },
        { x: -10, y: 15, width: 5, height: 10 }
      ])
    ).toEqual({ x: -10, y: 15, width: 50, height: 45 });
  });
});
