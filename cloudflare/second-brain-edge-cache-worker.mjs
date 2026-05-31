const HTML_EDGE_TTL_SECONDS = 300;
const APP_STATE_EDGE_TTL_SECONDS = 86400;
const STATIC_EDGE_TTL_SECONDS = 31536000;
const APP_STATE_CACHE_CONTROL = "public, max-age=30, s-maxage=86400, stale-while-revalidate=604800";
const APP_STATE_CLIENT_CACHE_CONTROL = "no-cache, must-revalidate";

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
      return withConditionalRevalidation(request, withClientCacheControl(withEdgeCacheStatus(cached, "HIT"), policy));
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
    response.headers.set("Cloudflare-CDN-Cache-Control", policy.cacheControl);
    response.headers.set("X-Second-Brain-Edge-Cache", "MISS");
    ctx.waitUntil(cache.put(cacheKey, response.clone()));
    return withClientCacheControl(response, policy);
  }
};

export function cachePolicy(request, url) {
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
      cacheControl: APP_STATE_CACHE_CONTROL,
      clientCacheControl: APP_STATE_CLIENT_CACHE_CONTROL
    };
  }

  if (path === "/second-brain/api/digests" || path === "/second-brain/api/knowledge-graph/insights") {
    return {
      edgeTtl: APP_STATE_EDGE_TTL_SECONDS,
      cacheControl: APP_STATE_CACHE_CONTROL,
      clientCacheControl: APP_STATE_CLIENT_CACHE_CONTROL
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

export function isCacheableOriginResponse(response) {
  return response.status === 200 && !response.headers.has("Set-Cookie");
}

export function withEdgeCacheStatus(response, status) {
  const headers = new Headers(response.headers);
  headers.set("X-Second-Brain-Edge-Cache", status);
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers
  });
}

export function withClientCacheControl(response, policy) {
  if (!policy.clientCacheControl) {
    return response;
  }
  const headers = new Headers(response.headers);
  headers.set("Cache-Control", policy.clientCacheControl);
  return new Response(response.body, {
    status: response.status,
    statusText: response.statusText,
    headers
  });
}

export function withConditionalRevalidation(request, response) {
  if (!etagMatches(request.headers.get("If-None-Match"), response.headers.get("ETag"))) {
    return response;
  }
  const headers = new Headers(response.headers);
  headers.delete("Content-Length");
  headers.delete("Content-Type");
  return new Response(null, {
    status: 304,
    headers
  });
}

export function etagMatches(ifNoneMatch, etag) {
  if (!ifNoneMatch || !etag) {
    return false;
  }
  const expected = stripWeakETag(etag.trim());
  return ifNoneMatch.split(",").some((candidate) => {
    const value = candidate.trim();
    return value === "*" || stripWeakETag(value) === expected;
  });
}

function stripWeakETag(value) {
  return value.replace(/^W\//i, "");
}
