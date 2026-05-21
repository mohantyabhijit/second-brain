import { execFile } from "child_process";
import { promisify } from "util";
import { getEnv, hasOneCli, ONECLI_BIN } from "./config";
import { saveLatestResult } from "./runtime-store";
import { fetchXBookmarks } from "./sources/x";
import { fetchYouTubePhaseOneItems } from "./sources/youtube";
import { summarizeBookmark, summarizeVideo } from "./summarize";
import type { PhaseOneResult, SourceStatus, ValidationItem, XBookmark, YouTubeItem } from "./types";

const execFileAsync = promisify(execFile);

async function oneCliStatus(): Promise<SourceStatus> {
  if (!hasOneCli()) return "blocked";
  try {
    await execFileAsync(ONECLI_BIN, ["auth", "status"], { timeout: 5000 });
    return "ready";
  } catch {
    return "needs_secrets";
  }
}

function validation(label: string, passed: boolean, passDetail: string, failDetail: string): ValidationItem {
  return {
    label,
    status: passed ? "pass" : "blocked",
    detail: passed ? passDetail : failDetail
  };
}

export async function runPhaseOne(): Promise<PhaseOneResult> {
  const blockers: string[] = [];
  const onecli = await oneCliStatus();
  let xStatus: SourceStatus = "needs_secrets";
  let youtubeStatus: SourceStatus = "needs_secrets";
  let youtubeBlocked = false;
  let xBookmarks: XBookmark[] = [];
  let youtubeItems: YouTubeItem[] = [];

  const xPromise = fetchXBookmarks(5).catch((error: unknown) => {
    blockers.push(error instanceof Error ? error.message : "X bookmark ingestion failed.");
    return [] as XBookmark[];
  });

  const playlistId = getEnv("YOUTUBE_PLAYLIST_ID");
  const youtubePromise = playlistId
    ? fetchYouTubePhaseOneItems(playlistId, getEnv("YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID")).catch((error: unknown) => {
        youtubeBlocked = true;
        blockers.push(error instanceof Error ? error.message : "YouTube playlist validation failed.");
        return [] as YouTubeItem[];
      })
    : Promise.resolve([] as YouTubeItem[]).then((items) => {
        blockers.push("YOUTUBE_PLAYLIST_ID is missing. Use a dedicated Second Brain Inbox playlist because Watch Later is blocked by the YouTube API.");
        return items;
      });

  [xBookmarks, youtubeItems] = await Promise.all([xPromise, youtubePromise]);

  if (xBookmarks.length >= 5) xStatus = "ready";
  else if (xBookmarks.length > 0) xStatus = "partial";

  if (youtubeBlocked) youtubeStatus = "blocked";
  else if (youtubeItems.length > 0) youtubeStatus = "ready";
  else if (playlistId) youtubeStatus = "partial";

  const summaries = [
    ...xBookmarks.map((bookmark) => summarizeBookmark(bookmark)),
    ...youtubeItems.filter((item) => item.transcriptStatus === "available").slice(0, 1).map((item) => summarizeVideo(item))
  ];

  const validationItems: ValidationItem[] = [
    validation("X bookmark request", xBookmarks.length > 0, `${xBookmarks.length} bookmark(s) fetched.`, "No X bookmarks fetched."),
    validation("5 X bookmarks", xBookmarks.length >= 5, "Fetched 5 X bookmarks.", `Fetched ${xBookmarks.length} X bookmark(s).`),
    validation("X source links", xBookmarks.length > 0 && xBookmarks.every((bookmark) => bookmark.sourceUrl), "Every bookmark has a source URL.", "One or more bookmarks are missing source URLs."),
    validation("YouTube playlist check", youtubeItems.length > 0, `${youtubeItems.length} YouTube item(s) fetched.`, "No YouTube playlist items fetched."),
    validation("Transcript path", youtubeItems.some((item) => item.transcriptStatus === "available"), "At least one transcript was extracted.", "No transcript extracted yet."),
    validation("Source-grounded summaries", summaries.length > 0, `${summaries.length} summary item(s) generated.`, "No summaries generated.")
  ];

  const result: PhaseOneResult = {
    generatedAt: new Date().toISOString(),
    sourceStatus: {
      x: xStatus,
      youtube: youtubeStatus,
      onecli
    },
    xBookmarks,
    youtubeItems,
    summaries,
    validation: validationItems,
    blockers
  };

  await saveLatestResult(result);
  return result;
}
