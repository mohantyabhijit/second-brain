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

Recommended Cloudflare Cache Rules:

1. Cache `/second-brain/_next/static/*` with Edge TTL of one year and browser TTL respecting origin.
2. Cache `/second-brain/api/digests/*/illustration` with Edge TTL of one year.
3. Cache `GET /second-brain/api/app-state*` with Edge TTL of five minutes and stale-while-revalidate enabled.
4. Cache `/second-brain/*` HTML with Edge TTL of five minutes unless request method is not `GET` or the request carries the backend session cookie.
5. Bypass cache for `/second-brain/api/auth/*`, `/second-brain/api/debug/*`, and all mutation endpoints.

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

