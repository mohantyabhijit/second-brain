# Performance Runbook

## Measurement

Run the reusable production measurement script before and after performance work:

```bash
npm run perf:measure
```

For Lighthouse desktop traces, install Lighthouse or run through `npx` and set:

```bash
RUN_LIGHTHOUSE=1 LIGHTHOUSE_BIN="npx --yes lighthouse@13.3.0" npm run perf:measure
```

The script measures public pages plus the full and view-scoped API boot payloads. It writes response bodies and optional Lighthouse JSON under `/tmp/second-brain-performance` by default.

## Cloudflare Caching

The origin now emits cache headers that Cloudflare can honor:

- Static Next assets under `/second-brain/_next/static/`: `public, max-age=31536000, immutable`.
- Static page HTML under `/second-brain/`: short browser TTL with longer CDN reuse and stale revalidation.
- View-scoped app state under `/second-brain/api/app-state?view=...`: short browser TTL, CDN `s-maxage`, and `stale-while-revalidate`.
- Digest illustrations under `/second-brain/api/digests/{id}/illustration`: long immutable TTL.

Cloudflare routes `abhijitmohanty.com/second-brain*` and `www.abhijitmohanty.com/second-brain*` through `second-brain-edge-cache`, whose source lives at `cloudflare/second-brain-edge-cache-worker.mjs`. The Worker caches:

1. `/second-brain/_next/static/*` with an Edge TTL of one year.
2. `/second-brain/api/digests/*/illustration` with an Edge TTL of one year.
3. `GET /second-brain/api/app-state*` with an Edge TTL of five minutes.
4. `/second-brain/*` HTML with an Edge TTL of five minutes.
5. It bypasses non-`GET` requests, requests with `Authorization` or `Cookie`, `/second-brain/api/auth/*`, `/second-brain/api/debug/*`, and mutation endpoints.

Purge Cloudflare cache after a successful deploy or refresh publish for:

- `/second-brain/`
- `/second-brain/insights/`
- `/second-brain/daily-newsletter/`
- `/second-brain/original-x-bookmarks/`
- `/second-brain/original-youtube-videos/`
- `/second-brain/knowledge-graph/`
- `/second-brain/api/app-state*`

## Backend Targets

- Redis cache hit for normal page boot.
- `X-Second-Brain-Cache: hit` on `/api/app-state?view=...` in production.
- View-scoped boot JSON under 100 KB uncompressed for normal feed pages.
- Server timing under 75 ms from Redis for app-state handlers.
