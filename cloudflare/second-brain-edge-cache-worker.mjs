const HTML_EDGE_TTL_SECONDS = 300;
const APP_STATE_EDGE_TTL_SECONDS = 300;
const STATIC_EDGE_TTL_SECONDS = 31536000;

const MUTATING_API_PREFIXES = [
  "/second-brain/api/auth/",
  "/second-brain/api/debug/",
  "/second-brain/api/ask",
  "/second-brain/api/digests/send",
  "/second-brain/api/knowledge-runs/refresh",
  "/second-brain/api/share/"
];

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const policy = cachePolicy(request, url);
    if (!policy) {
      return fetch(request);
    }

    const cache = caches.default;
    const cacheKey = new Request(url.toString(), { method: "GET" });
    const cached = await cache.match(cacheKey);
    if (cached) {
      return withEdgeCacheStatus(cached, "HIT");
    }

    const originResponse = await fetch(request, {
      cf: {
        cacheEverything: true,
        cacheTtl: policy.edgeTtl
      }
    });
    if (!isCacheableOriginResponse(originResponse)) {
      return withEdgeCacheStatus(originResponse, "BYPASS");
    }

    const response = new Response(originResponse.body, originResponse);
    response.headers.delete("Set-Cookie");
    response.headers.set("Cache-Control", policy.cacheControl);
    response.headers.set("X-Second-Brain-Edge-Cache", "MISS");
    ctx.waitUntil(cache.put(cacheKey, response.clone()));
    return response;
  }
};

function cachePolicy(request, url) {
  if (request.method !== "GET") {
    return null;
  }
  if (request.headers.has("Authorization") || request.headers.has("Cookie")) {
    return null;
  }

  const path = url.pathname;
  if (MUTATING_API_PREFIXES.some((prefix) => path.startsWith(prefix))) {
    return null;
  }

  if (path.startsWith("/second-brain/_next/static/")) {
    return {
      edgeTtl: STATIC_EDGE_TTL_SECONDS,
      cacheControl: "public, max-age=31536000, immutable"
    };
  }

  if (/^\/second-brain\/api\/digests\/[^/]+\/illustration$/.test(path)) {
    return {
      edgeTtl: STATIC_EDGE_TTL_SECONDS,
      cacheControl: "public, max-age=31536000, s-maxage=31536000, immutable"
    };
  }

  if (path === "/second-brain/api/app-state") {
    return {
      edgeTtl: APP_STATE_EDGE_TTL_SECONDS,
      cacheControl: "public, max-age=30, s-maxage=300, stale-while-revalidate=1800"
    };
  }

  if (path.startsWith("/second-brain/api/")) {
    return null;
  }

  if (path === "/second-brain" || path.startsWith("/second-brain/")) {
    return {
      edgeTtl: HTML_EDGE_TTL_SECONDS,
      cacheControl: "public, max-age=60, s-maxage=300, stale-while-revalidate=1800"
    };
  }

  return null;
}

function isCacheableOriginResponse(response) {
  return response.status === 200 && !response.headers.has("Set-Cookie");
}

function withEdgeCacheStatus(response, status) {
  const headers = new Headers(response.headers);
  headers.set("X-Second-Brain-Edge-Cache", status);
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers
  });
}
