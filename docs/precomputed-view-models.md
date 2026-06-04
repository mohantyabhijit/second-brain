# Precomputed View Models

## Contract

Frontend routes must render from precomputed JSON. The normal page path is:

```text
Next route
  -> GET /api/app-state?view={view}&limit={limit}
  -> Cloudflare Worker cache
  -> Redis read model
  -> Postgres read_model_snapshots fallback
  -> React render
```

The API may fall back to canonical `knowledge_runs` and `digest_issues` only when no published read model exists, mainly for local development or cache rebuilds. Production freshness should come from the worker/precompute pipeline, not from page requests.

## Page Mapping

| Frontend route | View key | Precomputed JSON fields | Primary UI use |
| --- | --- | --- | --- |
| `/`, `/insights` | `insights` | `latest.insights`, `views.insights`, manifest, refresh status | Ranked insight feed |
| `/daily-newsletter` | `daily-newsletter` | `latest.digest`, `views.dailyNewsletter`, `digests`, compact summaries | Newsletter issue list and reader |
| `/original-x-bookmarks`, `/original-x-posts` | `original-x-posts` | `latest.xBookmarks`, matching X summaries | Original X source feed with AI summary/evidence |
| `/original-youtube-videos`, `/original-youtube-posts` | `original-youtube-posts` | `latest.youtubeItems`, matching YouTube summaries/time markers | Original YouTube source feed |
| `/knowledge-graph` | `knowledge-graph` | `graph.insightGraph`, graph status, manifest | Interactive graph canvas |

`frontend/src/features/knowledge-inbox/model/useKnowledgeInboxController.ts` is the single page boot reader. It calls `readAppState(activePage, limit)` and passes the graph page its `graph.insightGraph`; the graph page no longer calls `/api/knowledge-graph/insights` during render.

## Publish Pipeline

Precompute happens in three places:

- `service.RunCycle` publishes after source refresh and canonical persistence.
- `service.GenerateDigest` publishes after a digest is saved.
- `backend/cmd/precompute` republishes the latest canonical run and digest list without fetching sources or calling AI providers.

The long-running worker calls precompute after refresh and after the daily digest slot, so a no-new-sources day still republishes a ready JSON snapshot. Manual runs can use:

```bash
npm run precompute:run
```

## Storage And Versioning

Each publish builds `AppState` with `schemaVersion = redis-read-model-v1`, a digest-sensitive `runId`, and an `etag` over the rendered read model. The publish writes:

- Redis manifest and run-scoped keys under `sb:v1:{owner}:...`.
- Postgres `read_model_snapshots.payload` for durable JSON fallback.
- Cloudflare-cacheable API responses with `Cache-Control: public, max-age=30, s-maxage=300, stale-while-revalidate=1800`.

The Redis manifest is the hot-path version pointer. It is updated only after run-scoped keys are written. Cloudflare purge runs after successful refresh, digest, and precompute publishes when purge credentials are configured.

## Remaining Runtime Computation

- `POST /api/ask` is still intentionally interactive and may call pgvector, Neo4j, Exa, and OpenAI. It is user-initiated, not part of page render. The precomputed `askContext` is available for a future thin/cached Ask path.
- `POST /api/knowledge-runs/refresh`, feedback, share, and auth routes are mutations. They are bypassed by Cloudflare and can perform side effects.
- If both Redis and Postgres `read_model_snapshots` miss, app-state endpoints can rebuild from canonical Postgres data to avoid a blank local app. Treat this as a recovery path, not the production page-load path.
