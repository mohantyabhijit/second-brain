# Redis Read Model Plan

## Goal

Make normal page load fast by serving frontend data from Redis-backed read models instead of querying Supabase Postgres and Neo4j on every render.

Supabase remains the canonical system of record. Neo4j remains the derived graph index. Redis stores only precomputed render payloads and short-lived operational state that can be rebuilt from Supabase, Supabase Storage, pgvector, and Neo4j.

The browser must not connect to Redis directly. The frontend calls the Go API, and the Go API reads Redis first with Supabase fallback.

## Target Load Path

Current page load does multiple API calls that can hit Supabase:

```text
Frontend mount
  -> GET /api/knowledge-runs/latest
  -> GET /api/digests
  -> GET /api/knowledge-runs/refresh
  -> backend reads Supabase/refresh memory
```

Target page load should be one fast API call backed by Redis:

```text
Frontend mount
  -> GET /api/app-state
  -> backend MGET/pipeline from Redis
  -> render immediately
```

Existing endpoints should stay available for compatibility, but their implementation should become Redis-first:

- `GET /api/knowledge-runs/latest`
- `GET /api/digests`
- `GET /api/knowledge-runs/refresh`

## Performance Targets

- Initial app state API response from Redis: less than 75 ms server-side p95.
- Frontend first meaningful render after API response: less than 150 ms for already-built JS.
- No Supabase or Neo4j calls in the normal initial render path when Redis has a current manifest.
- One network round trip for boot data through `GET /api/app-state`.
- Redis payloads should be bounded. Keep the app-state response under 1-2 MB uncompressed; split large source lists into paged keys if needed.
- Use `ETag` or manifest versioning so the frontend can skip re-rendering unchanged snapshots.

## Data Stored In Redis

Redis stores derived read models, not canonical data.

| Key | Type | Contents | TTL |
| --- | --- | --- | --- |
| `sb:v1:{owner}:manifest` | hash | Current `run_id`, `generated_at`, schema version, etag, graph status, digest status, publish timestamp | none |
| `sb:v1:{owner}:app-state:{run_id}` | string JSON | Boot payload for the entire UI | 30 days |
| `sb:v1:{owner}:run:{run_id}:latest` | string JSON | Existing `KnowledgeRunResult` payload | 30 days |
| `sb:v1:{owner}:view:{run_id}:insights` | string JSON | Ranked insights page read model | 30 days |
| `sb:v1:{owner}:view:{run_id}:daily-newsletter` | string JSON | Daily newsletter page read model | 30 days |
| `sb:v1:{owner}:view:{run_id}:original-x-bookmarks` | string JSON | X bookmark page read model | 30 days |
| `sb:v1:{owner}:view:{run_id}:original-youtube-posts` | string JSON | YouTube page read model | 30 days |
| `sb:v1:{owner}:digests:{run_id}:list` | string JSON | Precomputed digest list for `/api/digests` | 30 days |
| `sb:v1:{owner}:graph:{run_id}:read-model` | string JSON | Precomputed knowledge graph canvas payload | 30 days |
| `sb:v1:{owner}:refresh:status` | string JSON | Shared refresh status and progress message | 24 hours |
| `sb:v1:{owner}:source-materials` | hash | Derived source-material states keyed by `{source_type}:{external_id}:{prompt_version}:{model}` so scheduled refreshes can skip already processed captures before expensive provider/model work | 30 days |
| `sb:v1:{owner}:graph:{run_id}:read-model` | string JSON | Themes, source connections, graph-derived cards, graph sync metadata | 30 days |
| `sb:v1:{owner}:ask:context:{run_id}` | string JSON | Compact RAG/GraphRAG source bundle for the current run | 30 days |
| `sb:v1:{owner}:ask:answer:{run_id}:{question_hash}` | string JSON | Optional repeated-question answer cache | 15 minutes to 6 hours |

Do not store secrets, provider tokens, Supabase credentials, service-role keys, full raw transcript archives, or the only copy of any canonical source record in Redis.

## App State Shape

`GET /api/app-state` should return the complete boot payload:

```json
{
  "manifest": {
    "schemaVersion": "redis-read-model-v1",
    "runId": "uuid-or-generated-run-id",
    "generatedAt": "2026-05-24T12:00:00Z",
    "etag": "sha256-of-published-payload",
    "graphStatus": "synced",
    "digestStatus": "generated"
  },
  "latest": {},
  "views": {
    "insights": {},
    "dailyNewsletter": {},
    "originalXBookmarks": {},
    "originalYouTubePosts": {}
  },
  "digests": [],
  "refreshStatus": {},
  "graph": {}
}
```

This lets the frontend render all current pages from one response. Route navigation then becomes client-side selection from the already-loaded read model.

## Read Model Contents

### Insights View

Store only display-ready fields:

- ranked insights
- title, source type, source URL, source ID
- canonical insight text
- practical text
- evidence snippet
- topics and entities
- confidence and score fields
- cache status
- cluster membership
- existing feedback summary if available

### Daily Newsletter View

- current digest subject
- body markdown
- generated/sent/failed status
- illustration URL or backend image endpoint
- digest date and scheduled time
- delivery status
- selected insight IDs and source links

### Original X Bookmarks View

- bookmark ID
- author name and username
- title/text/preview text
- source URL
- created/published timestamp
- related summary ID
- related insight IDs
- cache status

If the X bookmark list grows too large, split it:

```text
sb:v1:{owner}:view:{run_id}:original-x-bookmarks:page:1
sb:v1:{owner}:view:{run_id}:original-x-bookmarks:page:2
```

### Original YouTube Posts View

- video ID
- title and channel
- source URL
- published timestamp
- transcript status
- transcript preview only
- important time markers
- related summary ID
- related insight IDs
- cache status

### Graph Read Model

- theme clusters
- insight clusters
- source connections
- graph sync status
- graph outbox pending/processed counts if available
- graph-derived cards for the UI

The graph read model is generated ahead of time from the latest processed result and stored under `graph.insightGraph`. Page render must not traverse Neo4j. A failed Neo4j sync should not block publishing a usable app-state snapshot; it should mark `graphStatus` as `stale` or `skipped`.

### Ask Context

The Ask path can start from Redis but should not be limited to Redis forever.

Store:

- top source-grounded insights
- top summaries
- clusters
- graph connections
- compact evidence snippets
- source URLs
- embedding/search metadata needed for ranking fallback

For arbitrary questions, the service can use Redis context first, then pgvector/Neo4j if deeper retrieval is needed. Repeated identical questions can use the short-lived answer cache.

## Refresh Publish Flow

Redis should be populated after canonical persistence succeeds.

```text
1. Start refresh
   -> SET sb:v1:{owner}:refresh:status running

2. Fetch and process sources
   -> X bookmarks
   -> YouTube videos
   -> cached or generated syntheses
   -> embeddings
   -> clusters and connections
   -> digest

3. Persist canonical data
   -> Supabase Postgres
   -> Supabase Storage
   -> pgvector tables
   -> graph_sync_outbox

4. Run graph sync when configured
   -> update Neo4j
   -> collect graph status

5. Build read models in memory
   -> latest run
   -> app state
   -> page views
   -> digests list
   -> graph read model
   -> Ask context

6. Publish Redis run-scoped keys
   -> write every `:{run_id}:` key first
   -> use Redis pipeline for speed

7. Flip manifest last
   -> HSET sb:v1:{owner}:manifest current_run_id {run_id} ...
   -> SET sb:v1:{owner}:refresh:status completed
```

The manifest update is the publish boundary. If the refresh fails halfway, the manifest continues to point at the previous good snapshot.

## Atomicity Rules

- Never update `sb:v1:{owner}:manifest` before all read models for the new run are present.
- Treat Redis publish failure as a cache publish failure, not as a canonical refresh failure, after Supabase writes have succeeded.
- Keep the previous manifest if publish fails.
- Use a temporary publish marker if needed:

```text
sb:v1:{owner}:publish:{run_id}:status = staging
sb:v1:{owner}:publish:{run_id}:status = ready
```

- The API should only serve snapshots referenced by the manifest.

## API Behavior

### `GET /api/app-state`

1. Read manifest from Redis.
2. Read `app-state:{run_id}`.
3. If found, return it with `X-Second-Brain-Cache: hit`.
4. If missing, read latest from Supabase, build app state, write Redis best-effort, return with `X-Second-Brain-Cache: fallback`.
5. If both Redis and Supabase fail, return an error.

### Existing Endpoints

- `/api/knowledge-runs/latest`: read `run:{run_id}:latest` first.
- `/api/digests`: read `digests:{run_id}:list` first.
- `/api/knowledge-runs/refresh`: read/write `refresh:status` so progress survives API process restarts.
- `/api/ask`: read `ask:context:{run_id}` first, then pgvector/Neo4j fallback for deeper retrieval.

### Writes

Writes must update canonical storage first:

- feedback
- digest generation
- digest send
- tweet share
- OAuth token flows
- manual refresh

After the canonical write succeeds, either update the affected Redis keys or mark the manifest as needing rebuild. Do not let Redis become the only place a user action exists.

## Frontend Changes

1. Add `readAppState()` in `frontend/src/features/knowledge-inbox/api/knowledgeRuns.ts`.
2. Change `useKnowledgeInboxController` to boot from `GET /api/app-state` instead of separate latest/digest calls.
3. Keep existing endpoint functions for refresh completion and compatibility.
4. Store page-specific data in controller state from the app-state payload.
5. Use the manifest etag to avoid unnecessary state replacement when data has not changed.
6. Keep polling refresh status, but have that endpoint served from Redis.

## Backend Changes

1. Add Redis config:

```text
REDIS_URL=
REDIS_CACHE_ENABLED=true
REDIS_CACHE_TTL=720h
REDIS_REFRESH_STATUS_TTL=24h
REDIS_ASK_ANSWER_TTL=1h
```

2. Add `backend/internal/cache` with:

```go
type ReadModelCache interface {
  ReadManifest(ctx context.Context, ownerID string) (Manifest, error)
  ReadAppState(ctx context.Context, ownerID string, runID string) (*AppState, error)
  PublishRun(ctx context.Context, input PublishRunInput) error
  ReadRefreshStatus(ctx context.Context, ownerID string) (*RefreshStatus, error)
  WriteRefreshStatus(ctx context.Context, ownerID string, status RefreshStatus) error
}
```

3. Add a no-op cache implementation for local runs without Redis.
4. Inject cache into `knowledge.Service` or into the HTTP layer.
5. Add read model builders in `backend/internal/knowledge` so Redis payloads use the same domain objects as current UI contracts.
6. Call `PublishRun` after `SaveRun` succeeds and after graph/digest data is available.
7. Use Redis pipelines for multi-key writes.
8. Add structured logs for `cache_hit`, `cache_miss`, `cache_fallback`, `cache_publish_failed`, and `cache_publish_completed`.

## Supabase Redis Wrapper

The enabled Supabase Redis wrapper should be used for diagnostics and admin SQL visibility only.

Create a Vault-backed Redis server in Supabase, then create small foreign tables:

```sql
create schema if not exists redis;

create server redis_server
foreign data wrapper redis_wrapper
options (
  conn_url_id '<vault_key_id>'
);

create foreign table redis.second_brain_manifest (
  key text,
  value text
)
server redis_server
options (
  src_type 'hash',
  src_key 'sb:v1:<owner_id>:manifest'
);

create foreign table redis.second_brain_views (
  key text,
  items jsonb
)
server redis_server
options (
  src_type 'multi_hash',
  src_key 'sb:v1:*:view:*'
);
```

Do not make the app read Redis through Supabase FDW tables. The wrapper is read-only, has no query pushdown, and can load full result sets into memory.

## Failure Modes

| Failure | Behavior |
| --- | --- |
| Redis unavailable on page load | Backend falls back to Supabase, returns `X-Second-Brain-Cache: fallback`, logs warning |
| Redis unavailable after successful refresh | Supabase remains updated, manifest is not flipped, next page load may use old Redis or Supabase fallback |
| Refresh fails before Supabase save | Keep old Redis manifest and old app snapshot |
| Refresh saves Supabase but graph sync fails | Publish app state with `graphStatus: stale` or `skipped` |
| Read model builder fails | Do not flip manifest; return previous good snapshot |
| Large X bookmark payload | Split into paged Redis keys and lazy-load pages through backend |

## Test Plan

Backend unit tests:

- cache hit returns app-state without calling Supabase store
- cache miss falls back to Supabase and warms Redis
- publish writes all run-scoped keys before manifest
- publish failure before manifest preserves old manifest
- refresh status is written to Redis during stage transitions
- Redis disabled uses no-op cache and current behavior still works

Frontend tests:

- controller boots from `readAppState`
- app-state normalizes nullable collections
- route navigation uses already-loaded state
- refresh completion replaces state only when manifest etag changes

Integration checks:

- start backend with Redis and Supabase envs
- run `npm run refresh:run`
- verify manifest points to the new run
- load frontend and confirm no Supabase queries are needed for initial render
- kill Redis and confirm Supabase fallback still renders

## Rollout

### Phase 1: Redis Foundation

- Add config, Redis client, no-op cache, key builder, manifest model.
- Add cache health logs.
- No frontend behavior change yet.

### Phase 2: Fast Existing Endpoints

- Make `/api/knowledge-runs/latest`, `/api/digests`, and `/api/knowledge-runs/refresh` Redis-first.
- Publish latest-run, digest-list, and refresh-status keys after refresh.
- Verify existing UI gets faster without route changes.

### Phase 3: Single Boot Payload

- Add `/api/app-state`.
- Build app-state during refresh.
- Change frontend boot to one request.
- Use etag to avoid unnecessary state churn.

### Phase 4: Page Read Models

- Split read models by page.
- Add optional pagination for large X bookmark lists.
- Keep one small boot payload and lazy-load heavy pages only when needed if payload grows beyond budget.

### Phase 5: Graph And Ask Speedups

- Publish graph read model after graph sync.
- Add Ask context bundle.
- Keep pgvector/Neo4j fallback for arbitrary deep questions.
- Add short-lived repeated-question cache.

### Phase 6: Supabase FDW Diagnostics

- Add Redis wrapper SQL migration or runbook.
- Expose manifest/read-model visibility in Supabase SQL for debugging.

## Acceptance Criteria

- Fresh app load renders from Redis with no Supabase or Neo4j calls on cache hit.
- Browser boot uses one API request for current UI state.
- Failed refresh never replaces the previous good snapshot.
- Redis can be rebuilt from Supabase and Neo4j-derived data.
- Redis outage degrades to Supabase fallback rather than blank UI.
- Logs and response headers make cache behavior visible.
