# Supabase Setup

1. Create a Supabase project.
2. Enable the `vector` extension for `pgvector`.
3. Apply the migrations in `supabase/migrations`.
4. Copy the pooled Postgres connection string into `SUPABASE_DB_URL`.
5. Keep `SUPABASE_DB_URL` server-side only. Do not expose it as a `NEXT_PUBLIC_` variable.

The app stores complete refresh payloads in `knowledge_runs.payload` as an audit log. It also writes normalized `source_items`, `source_objects`, and `knowledge_syntheses` rows so source captures can be deduplicated and synthesis can be reused.

## Responsibilities

Supabase owns three parts of the storage architecture:

- Postgres is the relational system of record.
- Storage is the object store for documents, transcripts, webpages, PDFs, images, and generated artifacts.
- `pgvector` is the vector database for chunk, summary, and entity embeddings.

Neo4j owns the knowledge graph and should be treated as a derived index fed from records and object metadata stored in Supabase.

## Storage Buckets

Create private buckets for raw source material and generated artifacts. Suggested buckets:

- `sources`: original or normalized source objects.
- `artifacts`: generated summaries, exports, and intermediate files.

Keep durable metadata in Postgres: bucket, path, checksum, content type, byte size, source item ID, and capture timestamp. Object paths should be deterministic enough to trace provenance.

Backend Storage settings:

- `SUPABASE_URL`: project URL such as `https://project-ref.supabase.co`.
- `SUPABASE_SERVICE_ROLE_KEY`: server-side key for writing private objects.
- `SUPABASE_STORAGE_BUCKET`: private bucket name, defaulting to `sources`.

The backend writes source text to deterministic paths such as `x/{tweet_id}/article.txt` and `youtube/{video_id}/transcript.txt`, then records object metadata in `source_objects`.

## Vector Tables

Use `pgvector` tables for semantic search over retrieval units. Each embedding row should include:

- the owner or workspace ID.
- the source item or chunk ID.
- the embedding model.
- the embedding dimensionality.
- the vector value.
- creation timestamp and run ID.

Do not mix embeddings from different models inside one index. If the model changes, create a new column/table or rebuild the index.
