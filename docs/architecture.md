# Architecture

## Frontend

`frontend/` is a Next.js React app. Route files stay in `frontend/app`, while feature code lives under `frontend/src/features`. The knowledge inbox is intentionally client-side because it drives an operator action and reads the latest run from the API.

Key paths:

- `frontend/app/page.tsx`: thin route entrypoint.
- `frontend/src/features/knowledge-inbox/KnowledgeInboxContainer.tsx`: container that connects state/actions to the rendered view.
- `frontend/src/features/knowledge-inbox/model`: stateful controller hook and initial run state.
- `frontend/src/features/knowledge-inbox/presentation/viewModel.ts`: maps API data into UI-safe props.
- `frontend/src/features/knowledge-inbox/ui`: reusable, presentational components that do not import the API client.
- `frontend/src/features/knowledge-inbox/contracts.ts`: shared API response shape.
- `frontend/src/utils/supabase`: Supabase Auth session helpers using publishable browser keys only.

## Backend

`backend/` is a Go service with a standard internal package layout:

- `cmd/api`: process bootstrap and graceful shutdown.
- `internal/config`: environment parsing.
- `internal/httpapi`: routing, JSON responses, and CORS.
- `internal/knowledge`: source intake, transcript checks, artifact writes, validation, and prompt synthesis.
- `internal/store/postgres`: Postgres persistence.

The frontend never uses the database connection directly. It calls the Go API for product data, and the Go API uses `DATABASE_URL`. Supabase Auth is the sole authentication provider; the Go API validates its bearer sessions before protected operator actions.

## Production Topology

Production is split across two shared 1 GB DigitalOcean droplets:

- `ubuntu-sgp` runs nginx, the static Next.js export, the Go API and worker,
  Redis, and private filesystem object storage.
- `codex-crapbox` runs PostgreSQL 17 with pgvector. The application connects to
  it over the DigitalOcean private VPC.

Cloudflare Free fronts nginx and caches public static files and app-state
responses. Supabase provides Auth only. Neo4j Aura, OneCLI, OpenAI, X,
Supadata, Exa, and Resend are external provider dependencies with the optional
and required boundaries documented in `docs/service-exit-plan.md`.

## Storage Architecture

The app uses five storage surfaces with separate responsibilities:

- Postgres is the relational system of record for users, source items, runs, chunk metadata, validation state, summaries, and ingestion audit logs.
- Filesystem object storage stores raw and derived objects such as fetched documents, transcript files, webpage snapshots, PDFs, images, and export artifacts.
- `pgvector` in Postgres stores embeddings for chunks, summaries, entities, and other retrieval units that need semantic search.
- Neo4j stores the knowledge graph: entities, concepts, source references, claims, and typed relationships used for multi-hop reasoning.
- Redis stores precomputed read models for fast frontend rendering. It is a derived cache, not canonical storage.

Postgres plus object storage is the canonical source of truth. Neo4j is a derived graph index that can be rebuilt from Postgres records and source artifacts when needed. Redis read models can be rebuilt after each refresh from Postgres, object storage metadata, pgvector, and Neo4j-derived state. The detailed Redis plan lives in `docs/redis-read-model-plan.md`.

Normal frontend routes render from precomputed page view models, not request-time Supabase joins or Neo4j traversal. The current route-to-view contract, cache/versioning strategy, and remaining runtime exceptions are documented in `docs/precomputed-view-models.md`.

## Relational Database

The first migration creates `public.knowledge_runs`, storing each refresh result as JSONB. The normalized source model now sits beside that audit log so the app can avoid recomputing work it has already done.

Core tables:

- `source_items`: saved X posts, YouTube videos, documents, and external URLs keyed by source type and external ID.
- `source_captures`: immutable captures of a source item keyed by capture hash, so the same post can change without losing prior processed versions.
- `source_objects`: pointers to object-storage paths and their checksums, attached to the source item and source capture.
- `source_chunks`: chunked evidence text derived from source captures and generated summaries.
- `source_embeddings`: pgvector-backed embeddings for chunks, summaries, extracted entity labels, and compatibility insight vectors, scoped to the capture that produced them.
- `youtube_transcript_requests`: atomic per-owner/per-video Supadata request
  ledger. It prevents repeated transcription requests across refreshes,
  deployments, prompt/model changes, and concurrent workers, and enforces the
  configured monthly request ceiling.
- `knowledge_syntheses`: prompt-versioned summaries, insights, and action items keyed by source capture, prompt version, and model. The JSON insight payload remains as a run artifact for compatibility.
- `insights`, `insight_evidence`, and `insight_embeddings`: first-class insight records with raw, canonical, abstract, and practical forms, grounded evidence, and vectors for similarity search.
- `knowledge_runs`: ingestion and refresh audit log for UI replay and debugging.
- `insight_clusters` and `cluster_memberships`: many-to-many groupings of similar insights. These group insights, not posts, so one source can contribute to several recurring patterns.
- `theme_clusters` and `source_connections_evidence`: derived cross-source understanding for recurring themes and related-source explanations.
- `feedback_events`: explicit user signals such as useful, obvious, stale, irrelevant, more like this, and less like this.
- `digest_issues`, `digest_source_items`, and `digest_deliveries`: idempotent daily digest generation, exact source membership for each issue, and email delivery status.
- `graph_sync_outbox`: pending Neo4j sync events derived from canonical source records.

The recompute rule is: if the same source capture has the same prompt version and model, reuse the `knowledge_syntheses` row instead of running synthesis again. Source identity dedupe uses `(owner_id, source_type, external_id)`; content-version dedupe uses `(source_item_id, capture_hash)`.

Daily digest selection uses the canonical ledger instead of the latest run payload alone: the 6 PM digest reads source items first seen after the previous digest and records the exact source item, capture, synthesis, URL, title, and timestamps in `digest_source_items`. Once a source item has appeared in a digest, it is excluded from later digest selection.

Refreshes first read a source-material index keyed by source type, external ID, prompt version, and model. Postgres is canonical through `source_items`, `source_captures`, `source_objects`, and `knowledge_syntheses`; Redis can serve the same source-material state as a derived fast path. If every fetched source is already present with the same capture hash, the refresh skips synthesis, embeddings, storage rewrites, and graph sync. Digest generation still runs from the latest saved knowledge run.

## Object Storage

The production object store for source material and generated artifacts is a private filesystem tree. Store object metadata in Postgres, including bucket, path, checksum, content type, byte size, source item ID, and capture timestamp.

Use object paths that make provenance obvious, for example:

```text
youtube/{video_id}/{capture_hash}/transcript.txt
x/{tweet_id}/{capture_hash}/article.txt
web/{source_item_id}/{capture_hash}/snapshot.html
documents/{source_item_id}/original.pdf
artifacts/{source_type}/{external_id}/{capture_hash}/{prompt_version}/{model}/summary.json
exports/{run_id}/knowledge-pack.json
```

The backend writes private text objects with `OBJECT_STORAGE_BACKEND=filesystem`, `OBJECT_STORAGE_ROOT=/srv/second-brain/object-storage`, and `OBJECT_STORAGE_BUCKET=sources`. Supabase is used only for Auth in production. The backend intentionally has no Supabase Database or Storage fallback.

## Vector Database

Use `pgvector` inside Postgres as the vector database. Embeddings should live beside their relational metadata so semantic search can be filtered by user, source type, run, timestamp, and permissions.

Expected vector tables:

- `chunk_embeddings`: semantic retrieval over source chunks.
- `summary_embeddings`: retrieval over generated summaries.
- `insight_embeddings`: similarity search over canonical insight text plus domain, type, and topics.
- `entity_embeddings`: retrieval over entity labels and descriptions.

Embedding rows must record the model name and dimensionality. Do not compare embeddings produced by different models in the same vector index.

## Knowledge Graph

Neo4j is the graph database for relationship-heavy retrieval. It should receive normalized entities, claims, concepts, source references, and edges after ingestion has produced stable records in Postgres.

Expected node types:

- `SourceItem`
- `Chunk`
- `Entity`
- `Concept`
- `Claim`
- `Summary`

Expected relationship types:

- `MENTIONS`
- `SUPPORTS`
- `CONTRADICTS`
- `DERIVED_FROM`
- `RELATED_TO`
- `PRECEDES`
- `AUTHORED_BY`

Neo4j should not be the only copy of source data. Store source text, object pointers, generated summaries, and graph-sync status in Postgres plus object storage so the graph can be recreated or migrated.

## Security

The browser must not connect directly to Postgres. Backend connections should use `DATABASE_URL` from server-side environment variables.

Browser clients should not get broad object storage access. The Go API should issue scoped reads or signed URLs only for objects the current user is allowed to inspect.

Neo4j credentials must stay server-side. The frontend should request graph-derived views through the Go API instead of connecting to Neo4j directly.

## Operational Constraints

- Neither current 1 GB droplet has enough headroom to safely consolidate all
  application, database, cache, and development workloads.
- Redis and Neo4j are derived stores; PostgreSQL and filesystem objects are
  canonical and must be backed up before either host is replaced.
- Backups must be recurring, encrypted, and copied off both droplets. A dump on
  `ubuntu-sgp` does not protect against losing that host.
- `codex-crapbox` development workloads must be isolated from or moved away
  from PostgreSQL before the database is treated as highly available.
