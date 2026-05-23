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

Apply the Supabase migrations in `supabase/migrations`, enable `pgvector`, create private Supabase Storage buckets, then set `SUPABASE_DB_URL` to the pooled Postgres connection string. The backend writes source text assets to Supabase Storage when `SUPABASE_URL` and `SUPABASE_SERVICE_ROLE_KEY` are available, and records artifact metadata either way. Neo4j credentials are server-side settings for the graph outbox worker.

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

Cloudflare Workers deploys use Workers Static Assets from `frontend/out`. In the Cloudflare dashboard, use:

```bash
npm run cloudflare:build
npm run cloudflare:deploy
```

The deploy script runs Wrangler from `frontend/` with an explicit `wrangler.jsonc`, which avoids Wrangler's workspace-root detection failure.

Generate the daily digest from the latest persisted run:

```bash
npm run digest:run
```

The digest sender uses Resend. Store `RESEND_API_KEY` in OneCLI for `api.resend.com`; configure `DIGEST_EMAIL_TO` and a verified-domain `DIGEST_EMAIL_FROM` through `backend/.env`, process env, or matching Keychain services.

Run a headless full knowledge refresh, including all available X bookmark pages by default:

```bash
npm run refresh:run
```

Set `X_BOOKMARK_LIMIT` to a positive number for capped validation runs. Leave it unset or set it to `0` to fetch every page returned by the X bookmarks API.

If X returns `401 Unauthorized` or an invalid refresh token error, re-authorize the local app:

```bash
npm run x:oauth
```

The helper uses OAuth 2.0 Authorization Code with PKCE with `tweet.read users.read bookmark.read offline.access`, saves fresh `X_USER_ACCESS_TOKEN` and `X_REFRESH_TOKEN` values to Keychain, and updates matching OneCLI token secrets when they exist. The default callback is `http://127.0.0.1:8765/callback`; it must exactly match a callback URL configured on the X app. Override it with `X_REDIRECT_URI` if the app uses a different local callback.

Use an external scheduler, such as GitHub Actions, cron, or a platform scheduler, to run that command at 5pm in `DIGEST_TIMEZONE`. The command is idempotent per owner and digest date.

## Secrets

Store provider secrets in OneCLI or export them only for a local validation session:

```bash
export X_USER_ACCESS_TOKEN=...
export YOUTUBE_API_KEY=...
export YOUTUBE_ACCESS_TOKEN=...
export SUPADATA_API_KEY=...
export OPENAI_API_KEY=...
export SUPABASE_SERVICE_ROLE_KEY=...
export RESEND_API_KEY=...
npm run onecli:save-secrets
```

To save only the Resend API key to OneCLI:

```bash
ONECLI_ONLY_SECRETS=RESEND_API_KEY RESEND_API_KEY=... npm run onecli:save-secrets
```

For local digest delivery, save non-secret email settings directly to Keychain:

```bash
DIGEST_EMAIL_TO=you@example.com \
DIGEST_EMAIL_FROM='Second Brain <digest@updates.example.com>' \
npm run email:save-secrets
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
