# Postgres and Legacy Supabase Setup

The app has been migrated off Supabase as the production runtime. Keep this page for legacy Supabase rollback/export context and for the migration files under `supabase/migrations`.

Production setup:

1. Provision PostgreSQL with the `vector` extension enabled.
2. Apply the migrations in `supabase/migrations`.
3. Put the app role connection string in `DATABASE_URL`.
4. Keep `DATABASE_URL` server-side only. Do not expose it as a `NEXT_PUBLIC_` variable.
5. Configure object storage with `OBJECT_STORAGE_BACKEND=filesystem`, `OBJECT_STORAGE_ROOT`, and `OBJECT_STORAGE_BUCKET`.

The app stores complete refresh payloads in `knowledge_runs.payload` as an audit log. It also writes normalized `source_items`, `source_captures`, `source_objects`, and `knowledge_syntheses` rows so source identity, content captures, Storage objects, and AI processing caches stay separately deduplicated. Scheduled refreshes use those tables as the canonical source-material list before fetching expensive transcripts or running model synthesis again.

## Responsibilities

Production owns three parts of the storage architecture:

- Postgres is the relational system of record.
- Filesystem object storage is the object store for documents, transcripts, webpages, PDFs, images, and generated artifacts.
- `pgvector` is the vector database for chunk, summary, and entity embeddings.

Neo4j owns the knowledge graph and should be treated as a derived index fed from records and object metadata stored in Postgres.

## Storage Buckets

Create private buckets or filesystem prefixes for raw source material and generated artifacts. Suggested buckets:

- `sources`: original or normalized source objects.
- `artifacts`: generated summaries, exports, and intermediate files.

Keep durable metadata in Postgres: bucket, path, checksum, content type, byte size, source item ID, source capture ID, and capture timestamp. Object paths include the capture hash so a changed post/video can be stored without overwriting the previous version.

Backend object-storage settings:

- `OBJECT_STORAGE_BACKEND`: `filesystem` in production.
- `OBJECT_STORAGE_ROOT`: root path such as `/srv/second-brain/object-storage`.
- `OBJECT_STORAGE_BUCKET`: private bucket/prefix name, defaulting to `sources`.

Legacy Supabase Storage settings still exist for rollback while the old project is active: `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, and `SUPABASE_STORAGE_BUCKET`.

The backend writes source text to deterministic paths such as `x/{tweet_id}/{capture_hash}/article.txt` and `youtube/{video_id}/{capture_hash}/transcript.txt`, writes processed output to paths such as `artifacts/{source_type}/{external_id}/{capture_hash}/{prompt_version}/{model}/summary.json`, then records object metadata in `source_objects`.

## Vector Tables

Use `pgvector` tables for semantic search over retrieval units. Each embedding row should include:

- the owner or workspace ID.
- the source item or chunk ID.
- the embedding model.
- the embedding dimensionality.
- the vector value.
- creation timestamp and run ID.

Do not mix embeddings from different models inside one index. If the model changes, create a new column/table or rebuild the index.

## Digest, Feedback, and Graph Sync

The current schema keeps the single-user default owner ID while making every new durable table owner-scoped. `source_connections` stores provider connection metadata and token references, not plaintext OAuth tokens.

Feedback events are append-only in `feedback_events`. Daily digests are upserted into `digest_issues` by owner and idempotency key, with delivery attempts tracked separately in `digest_deliveries`.

Neo4j sync starts from `graph_sync_outbox`. The graph is derived from Postgres records and source artifacts, so failed graph writes can be retried or rebuilt without losing canonical source truth.
