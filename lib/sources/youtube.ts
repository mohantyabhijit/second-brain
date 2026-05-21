import type { YouTubeItem } from "../types";
import { credentialHint, getEnv, oneCliGatewayEnabled } from "../config";

type PlaylistResponse = {
  items?: Array<{
    snippet?: {
      title?: string;
      channelTitle?: string;
      publishedAt?: string;
      resourceId?: { videoId?: string };
    };
  }>;
};

function youTubeHeaders(): HeadersInit {
  const token = getEnv("YOUTUBE_ACCESS_TOKEN");
  if (token) return { Authorization: `Bearer ${token}` };
  if (oneCliGatewayEnabled()) return {};
  return {};
}

function appendApiKey(url: URL) {
  const apiKey = getEnv("YOUTUBE_API_KEY");
  if (apiKey) {
    url.searchParams.set("key", apiKey);
  }
}

async function youtubeFetch<T>(url: URL): Promise<T> {
  appendApiKey(url);
  const hasAuth = Boolean(getEnv("YOUTUBE_API_KEY") || getEnv("YOUTUBE_ACCESS_TOKEN") || oneCliGatewayEnabled());
  if (!hasAuth) {
    throw new Error(credentialHint("YOUTUBE_API_KEY or YOUTUBE_ACCESS_TOKEN"));
  }

  const response = await fetch(url, {
    headers: youTubeHeaders(),
    cache: "no-store"
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`YouTube API ${response.status}: ${body}`);
  }

  return (await response.json()) as T;
}

function decodeXml(value: string) {
  return value
    .replace(/&amp;/g, "&")
    .replace(/&quot;/g, "\"")
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">");
}

function pickTrack(xml: string) {
  const tracks = [...xml.matchAll(/<track\b[^>]*lang_code="([^"]+)"[^>]*(?:name="([^"]*)")?[^>]*>/g)];
  const english = tracks.find((match) => match[1] === "en");
  const track = english ?? tracks[0];
  if (!track) return null;
  return {
    lang: track[1],
    name: track[2] ? decodeXml(track[2]) : undefined
  };
}

export async function fetchPlaylistItems(playlistId: string, limit = 5): Promise<YouTubeItem[]> {
  if (!playlistId) {
    throw new Error("YOUTUBE_PLAYLIST_ID is required for Phase 1 YouTube playlist validation.");
  }

  const url = new URL("https://www.googleapis.com/youtube/v3/playlistItems");
  url.searchParams.set("part", "snippet");
  url.searchParams.set("playlistId", playlistId);
  url.searchParams.set("maxResults", String(limit));

  const payload = await youtubeFetch<PlaylistResponse>(url);
  return (payload.items ?? [])
    .flatMap((item) => {
      const snippet = item.snippet;
      const videoId = snippet?.resourceId?.videoId;
      if (!snippet || !videoId) return [];
      return [
        {
          videoId,
          title: snippet.title ?? "Untitled YouTube video",
          channelTitle: snippet.channelTitle,
          publishedAt: snippet.publishedAt,
          sourceUrl: `https://www.youtube.com/watch?v=${videoId}`,
          transcriptStatus: "untested" as const
        }
      ];
    });
}

export async function testTranscript(videoId: string): Promise<Pick<YouTubeItem, "transcriptStatus" | "transcriptPreview" | "transcriptError">> {
  try {
    const listUrl = new URL("https://video.google.com/timedtext");
    listUrl.searchParams.set("type", "list");
    listUrl.searchParams.set("v", videoId);
    const listResponse = await fetch(listUrl, { cache: "no-store" });
    const listXml = await listResponse.text();
    const track = pickTrack(listXml);
    if (!track) {
      return { transcriptStatus: "missing", transcriptError: "No public transcript tracks were listed." };
    }

    const textUrl = new URL("https://video.google.com/timedtext");
    textUrl.searchParams.set("v", videoId);
    textUrl.searchParams.set("lang", track.lang);
    if (track.name) textUrl.searchParams.set("name", track.name);
    const textResponse = await fetch(textUrl, { cache: "no-store" });
    const textXml = await textResponse.text();
    const preview = [...textXml.matchAll(/<text\b[^>]*>(.*?)<\/text>/g)]
      .map((match) => decodeXml(match[1]).replace(/\s+/g, " ").trim())
      .filter(Boolean)
      .slice(0, 16)
      .join(" ")
      .slice(0, 1200);

    if (!preview) {
      return { transcriptStatus: "missing", transcriptError: "Transcript lookup returned no text." };
    }

    return { transcriptStatus: "available", transcriptPreview: preview };
  } catch (error) {
    return {
      transcriptStatus: "blocked",
      transcriptError: error instanceof Error ? error.message : "Transcript extraction failed."
    };
  }
}

export async function fetchYouTubePhaseOneItems(playlistId: string, transcriptVideoId?: string) {
  const items = await fetchPlaylistItems(playlistId, 5);
  const targetVideoId = transcriptVideoId ?? items[0]?.videoId;

  if (!targetVideoId) {
    return items;
  }

  const transcript = await testTranscript(targetVideoId);
  return items.map((item) => (item.videoId === targetVideoId ? { ...item, ...transcript } : item));
}
