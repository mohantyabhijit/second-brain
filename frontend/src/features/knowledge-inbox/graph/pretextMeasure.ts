import { layoutWithLines, prepareWithSegments, type PreparedTextWithSegments } from "@chenglou/pretext";
import type { GraphLabelLayout } from "./graphLayout";

const preparedCache = new Map<string, PreparedTextWithSegments>();

export function measurePretextGraphLabel(text: string, maxWidth: number, font: string, lineHeight: number): GraphLabelLayout {
  const cacheKey = `${font}\n${text}`;
  let prepared = preparedCache.get(cacheKey);
  if (!prepared) {
    prepared = prepareWithSegments(text, font);
    preparedCache.set(cacheKey, prepared);
  }
  const layout = layoutWithLines(prepared, maxWidth, lineHeight);
  const lines = layout.lines.slice(0, 4);
  const width = Math.max(72, ...lines.map((line) => line.width));
  return {
    width: Math.min(maxWidth, width),
    height: Math.max(lineHeight, lines.length * lineHeight),
    lines: lines.map((line, index) => {
      if (index === 3 && layout.lines.length > 4) {
        return `${line.text.replace(/[.\s]+$/, "")}...`;
      }
      return line.text;
    })
  };
}
