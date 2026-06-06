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

  it("loads the public owner feed directly without the Supabase landing flow", () => {
    const homePage = read("app/page.tsx");
    expect(homePage).toContain("<KnowledgeInboxContainer initialPage=\"insights\" />");
    expect(homePage).not.toContain("SecondBrainLanding");

    const apiClient = read("src/features/knowledge-inbox/api/knowledgeRuns.ts");
    expect(apiClient).not.toContain("Authorization");
    expect(apiClient).not.toContain("tryCreateClient");

    const consoleView = read("src/features/knowledge-inbox/ui/SecondBrainConsoleView.tsx");
    expect(consoleView).not.toContain("onFeedback");
    expect(consoleView).not.toContain("AskSecondBrainWidget");
  });
});
