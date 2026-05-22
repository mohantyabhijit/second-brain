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
- `frontend/src/utils/supabase`: optional Supabase Auth cookie helpers using publishable browser keys only.

## Backend

`backend/` is a Go service with a standard internal package layout:

- `cmd/api`: process bootstrap and graceful shutdown.
- `internal/config`: environment parsing.
- `internal/httpapi`: routing, JSON responses, and CORS.
- `internal/knowledge`: source intake, transcript checks, artifact writes, validation, and prompt synthesis.
- `internal/store/postgres`: Supabase Postgres persistence.

The frontend never uses the Supabase database connection directly. It calls the Go API for product data, and the Go API uses the Supabase pooled Postgres connection string. The frontend may use Supabase publishable keys for auth session cookies only.

## Storage Architecture

The app uses four storage surfaces with separate responsibilities:

- Supabase Postgres is the relational system of record for users, source items, runs, chunk metadata, validation state, summaries, and ingestion audit logs.
- Supabase Storage stores raw and derived objects such as fetched documents, transcript files, webpage snapshots, PDFs, images, and export artifacts.
- `pgvector` in Supabase Postgres stores embeddings for chunks, summaries, entities, and other retrieval units that need semantic search.
- Neo4j stores the knowledge graph: entities, concepts, source references, claims, and typed relationships used for multi-hop reasoning.

Supabase remains the canonical source of truth. Neo4j is a derived graph index that can be rebuilt from Postgres records and source artifacts when needed.

## Relational Database

The first migration creates `public.knowledge_runs`, storing each refresh result as JSONB. The normalized source model now sits beside that audit log so the app can avoid recomputing work it has already done.

Core tables:

- `source_items`: saved X posts, YouTube videos, documents, and external URLs keyed by source type and external ID.
- `source_captures`: immutable captures of a source item keyed by capture hash, so the same post can change without losing prior processed versions.
- `source_objects`: pointers to Supabase Storage objects and their checksums, attached to the source item and source capture.
- `source_chunks`: chunked evidence text derived from source captures and generated summaries.
- `source_embeddings`: pgvector-backed embeddings for chunks, summaries, and extracted entity labels, scoped to the capture that produced them.
- `knowledge_syntheses`: prompt-versioned summaries, insights, and action items keyed by source capture, prompt version, and model.
- `knowledge_runs`: ingestion and refresh audit log for UI replay and debugging.
- `theme_clusters` and `source_connections_evidence`: derived cross-source understanding for recurring themes and related-source explanations.
- `feedback_events`: explicit user signals such as useful, obvious, stale, irrelevant, more like this, and less like this.
- `digest_issues` and `digest_deliveries`: idempotent daily digest generation and email delivery status.
- `graph_sync_outbox`: pending Neo4j sync events derived from canonical source records.

The recompute rule is: if the same source capture has the same prompt version and model, reuse the `knowledge_syntheses` row instead of running synthesis again. Source identity dedupe uses `(owner_id, source_type, external_id)`; content-version dedupe uses `(source_item_id, capture_hash)`.

## Object Storage

Supabase Storage is the object store for source material and generated artifacts. Store object metadata in Postgres, including bucket, path, checksum, content type, byte size, source item ID, and capture timestamp.

Use object paths that make provenance obvious, for example:

```text
youtube/{video_id}/{capture_hash}/transcript.txt
x/{tweet_id}/{capture_hash}/article.txt
web/{source_item_id}/{capture_hash}/snapshot.html
documents/{source_item_id}/original.pdf
artifacts/{source_type}/{external_id}/{capture_hash}/{prompt_version}/{model}/summary.json
exports/{run_id}/knowledge-pack.json
```

The backend writes private text objects through Supabase Storage using `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, and `SUPABASE_STORAGE_BUCKET`. Local demos can still record artifact metadata when Storage credentials are missing, but production should treat Storage writes as required.

## Vector Database

Use `pgvector` inside Supabase Postgres as the vector database. Embeddings should live beside their relational metadata so semantic search can be filtered by user, source type, run, timestamp, and permissions.

Expected vector tables:

- `chunk_embeddings`: semantic retrieval over source chunks.
- `summary_embeddings`: retrieval over generated summaries.
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

Neo4j should not be the only copy of source data. Store source text, object pointers, generated summaries, and graph-sync status in Supabase so the graph can be recreated or migrated.

## Security

RLS is enabled with a deny-all browser policy. Backend connections should use the Supabase pooled Postgres connection string from server-side environment variables.

Supabase Storage bucket policies should follow the same rule: browser clients should not get broad object access. The Go API should issue scoped reads or signed URLs only for objects the current user is allowed to inspect.

Neo4j credentials must stay server-side. The frontend should request graph-derived views through the Go API instead of connecting to Neo4j directly.
