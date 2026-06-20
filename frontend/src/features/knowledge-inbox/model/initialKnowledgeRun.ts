import type { KnowledgeRunResult } from "../contracts";

export const initialKnowledgeRun: KnowledgeRunResult = {
  generatedAt: "1970-01-01T00:00:00.000Z",
  sourceStatus: {
    x: "needs_secrets",
    youtube: "needs_secrets",
    onecli: "needs_secrets"
  },
  sourceCounts: {
    xBookmarks: 0,
    youtubeItems: 0
  },
  xBookmarks: [],
  youtubeItems: [],
  summaries: [],
  insights: [],
  actionItems: [],
  processing: [],
  validation: [
    {
      label: "X bookmark request",
      status: "untested",
      detail: "Run the intake check after adding authenticated source credentials."
    },
    {
      label: "YouTube inbox check",
      status: "untested",
      detail: "Use a dedicated Second Brain Inbox playlist ID."
    },
    {
      label: "Transcript path",
      status: "untested",
      detail: "The app verifies captions before generating source-grounded summaries."
    }
  ],
  blockers: ["OneCLI is installed, but this app has not been run with authenticated provider secrets yet."]
};
