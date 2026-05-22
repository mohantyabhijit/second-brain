# Second Brain

Source-grounded knowledge inbox for saved X posts, YouTube videos, transcripts, and reading decisions. The repo is split into a React frontend, a Go backend, Supabase-managed relational/vector/object storage, and a Neo4j knowledge graph.

## Architecture

```text
frontend/Next.js
    |
backend/Go API
    |
    +-- Supabase Postgres: relational system of record
    +-- Supabase Storage: documents and object storage
    +-- Supabase pgvector: vector database for embeddings
    +-- Neo4j: knowledge graph for entities and relationships
```

Supabase is the canonical data layer. Neo4j is a derived graph index for multi-hop reasoning and relationship-heavy retrieval.

`design/architecture-overview.md` keeps the current system diagram for the ingestion, artifact, synthesis, and read-model flow.

## Structure

```text
frontend/   Next.js React app
backend/    Go API, ingestion pipeline, persistence adapters
supabase/   Database migrations for Postgres and pgvector
docs/       Architecture and operating notes
design/     Durable diagrams and design artifacts
scripts/    Local setup helpers
```

## Local Setup

Apply the Supabase migrations in `supabase/migrations`, enable `pgvector`, create private Supabase Storage buckets, then set `SUPABASE_DB_URL` to the pooled Postgres connection string. Neo4j credentials are server-side settings once the graph sync is implemented.

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env.local
npm install --prefix frontend
cd backend && go mod download
```

Apply migrations:

```bash
npm run db:migrate
```

Run the services in two terminals:

```bash
npm run backend:dev
npm run frontend:dev
```

Open `http://localhost:3000`. The frontend calls the Go API at `NEXT_PUBLIC_API_BASE_URL`, defaulting to `http://localhost:8080`.

`npm run backend:dev` reads `SUPABASE_DB_URL` from Keychain when it is not already exported, then runs the Go API through `onecli run` so outbound provider requests use OneCLI gateway injection.

For local demos without Supabase configured, the Go API falls back to `data/runtime/latest-knowledge-run.json` and still serves the same knowledge-run contract to the frontend.

## Secrets

Store provider secrets in OneCLI or export them only for a local validation session:

```bash
export X_USER_ACCESS_TOKEN=...
export YOUTUBE_API_KEY=...
export YOUTUBE_ACCESS_TOKEN=...
export SUPADATA_API_KEY=...
export OPENAI_API_KEY=...
export SUPABASE_SERVICE_ROLE_KEY=...
npm run onecli:save-secrets
```

`YOUTUBE_PLAYLIST_ID` is intentionally a non-secret backend setting. Use a dedicated playlist such as `Second Brain Inbox`; the official YouTube API blocks Watch Later listing.

## Validation

```bash
npm run ci
```

For targeted checks:

```bash
npm run frontend:test
npm run backend:test
npm run typecheck
npm run lint
npm run build
```
