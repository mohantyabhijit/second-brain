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
- `internal/knowledge`: ingestion, transcript checks, validation, and summarization.
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

The first migration creates `public.knowledge_runs`, storing each refresh result as JSONB. This keeps the app flexible while preserving complete run output for later normalization.

As the source model stabilizes, split the JSON payload into normalized relational tables such as:

- `source_items`: saved posts, videos, documents, and external URLs.
- `source_objects`: pointers to Supabase Storage objects and their checksums.
- `chunks`: retrievable text units with source offsets and provenance.
- `summaries`: generated summaries tied to source items and chunks.
- `knowledge_runs`: ingestion and refresh audit log.

## Object Storage

Supabase Storage is the object store for source material and generated artifacts. Store object metadata in Postgres, including bucket, path, checksum, content type, byte size, source item ID, and capture timestamp.

Use object paths that make provenance obvious, for example:

```text
sources/youtube/{video_id}/transcript.json
sources/web/{source_item_id}/snapshot.html
sources/documents/{source_item_id}/original.pdf
exports/{run_id}/knowledge-pack.json
```

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
