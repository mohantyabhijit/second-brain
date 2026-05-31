import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const frontendRoot = join(process.cwd());

function read(path: string) {
  return readFileSync(join(frontendRoot, path), "utf8");
}

describe("knowledge inbox page data flow", () => {
  it("boots pages from precomputed app-state instead of page-specific compute APIs", () => {
    const controller = read("src/features/knowledge-inbox/model/useKnowledgeInboxController.ts");
    expect(controller).toContain("readAppState(activePage");

    const graphView = read("src/features/knowledge-inbox/ui/KnowledgeGraphView.tsx");
    expect(graphView).not.toContain("readInsightGraph");
    expect(graphView).not.toContain("fetch(");

    const consoleView = read("src/features/knowledge-inbox/ui/SecondBrainConsoleView.tsx");
    expect(consoleView).toContain("<KnowledgeGraphView graph={insightGraph}");
  });
});
