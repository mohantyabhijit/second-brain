import assert from "node:assert/strict";
import test from "node:test";

import {
  cachePolicy,
  etagMatches,
  withConditionalRevalidation,
  withEdgeCacheStatus
} from "./second-brain-edge-cache-worker.mjs";

test("returns 304 for cached app-state hits with a matching ETag", async () => {
  const request = new Request("https://abhijitmohanty.com/second-brain/api/app-state?view=insights&limit=20", {
    headers: {
      "If-None-Match": '"app-state-etag:insights:20"'
    }
  });
  const cached = withEdgeCacheStatus(
    new Response(JSON.stringify({ ok: true }), {
      headers: {
        "Cache-Control": "public, max-age=30, s-maxage=300",
        "Content-Type": "application/json",
        ETag: '"app-state-etag:insights:20"'
      }
    }),
    "HIT"
  );

  const response = withConditionalRevalidation(request, cached);

  assert.equal(response.status, 304);
  assert.equal(await response.text(), "");
  assert.equal(response.headers.get("ETag"), '"app-state-etag:insights:20"');
  assert.equal(response.headers.get("X-Second-Brain-Edge-Cache"), "HIT");
  assert.equal(response.headers.has("Content-Type"), false);
});

test("keeps cached hits as 200 responses when the ETag changed", async () => {
  const request = new Request("https://abhijitmohanty.com/second-brain/api/app-state?view=insights&limit=20", {
    headers: {
      "If-None-Match": '"old-etag"'
    }
  });
  const cached = withEdgeCacheStatus(
    new Response("fresh body", {
      headers: {
        ETag: '"new-etag"'
      }
    }),
    "HIT"
  );

  const response = withConditionalRevalidation(request, cached);

  assert.equal(response.status, 200);
  assert.equal(await response.text(), "fresh body");
});

test("matches strong and weak validators from If-None-Match lists", () => {
  assert.equal(etagMatches('"other", W/"target"', '"target"'), true);
  assert.equal(etagMatches("*", '"target"'), true);
  assert.equal(etagMatches('"other"', '"target"'), false);
});

test("only app-state API requests are cached among JSON API routes", () => {
  const appStateURL = new URL("https://abhijitmohanty.com/second-brain/api/app-state?view=insights");
  const askURL = new URL("https://abhijitmohanty.com/second-brain/api/ask");
  const appState = cachePolicy(new Request(appStateURL.toString()), appStateURL);
  const ask = cachePolicy(new Request(askURL.toString()), askURL);

  assert.equal(appState.edgeTtl, 300);
  assert.equal(ask, null);
});
