import type { XBookmark } from "../types";
import { credentialHint, getEnv, oneCliGatewayEnabled } from "../config";

type XUserResponse = {
  data?: { id: string; name?: string; username?: string };
};

type XBookmarkResponse = {
  data?: Array<{
    id: string;
    text: string;
    author_id?: string;
    created_at?: string;
    public_metrics?: Record<string, number>;
  }>;
  includes?: {
    users?: Array<{ id: string; name?: string; username?: string }>;
  };
};

function authHeaders(): HeadersInit {
  const token = getEnv("X_USER_ACCESS_TOKEN");
  if (token) return { Authorization: `Bearer ${token}` };
  if (oneCliGatewayEnabled()) return {};
  throw new Error(credentialHint("X_USER_ACCESS_TOKEN"));
}

async function xFetch<T>(url: string): Promise<T> {
  const response = await fetch(url, {
    headers: authHeaders(),
    cache: "no-store"
  });

  if (!response.ok) {
    const body = await response.text();
    throw new Error(`X API ${response.status}: ${body}`);
  }

  return (await response.json()) as T;
}

export async function fetchXBookmarks(limit = 5): Promise<XBookmark[]> {
  const me = await xFetch<XUserResponse>("https://api.x.com/2/users/me?user.fields=username,name");
  const userId = me.data?.id;

  if (!userId) {
    throw new Error("X /2/users/me did not return an authenticated user id.");
  }

  const params = new URLSearchParams({
    max_results: String(limit),
    "tweet.fields": "created_at,public_metrics,author_id",
    expansions: "author_id",
    "user.fields": "username,name"
  });
  const payload = await xFetch<XBookmarkResponse>(`https://api.x.com/2/users/${userId}/bookmarks?${params}`);
  const users = new Map((payload.includes?.users ?? []).map((user) => [user.id, user]));

  return (payload.data ?? []).slice(0, limit).map((tweet) => {
    const user = tweet.author_id ? users.get(tweet.author_id) : undefined;
    const username = user?.username;
    return {
      id: tweet.id,
      text: tweet.text,
      authorId: tweet.author_id,
      authorName: user?.name,
      username,
      createdAt: tweet.created_at,
      publicMetrics: tweet.public_metrics,
      sourceUrl: username ? `https://x.com/${username}/status/${tweet.id}` : `https://x.com/i/web/status/${tweet.id}`
    };
  });
}
