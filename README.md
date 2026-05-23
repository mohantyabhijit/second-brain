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

Run the self-organizing worker locally or on the VPS:

```bash
npm run self:organize
npm run worker:run
```

`self:organize` runs one full cycle: refresh, optional graph sync, then digest delivery. By default the long-running worker refreshes every `2h`, generates and sends the newsletter every `2h`, and schedules the digest issue for `18:00` in `DIGEST_TIMEZONE`. Override with `WORKER_REFRESH_INTERVAL`, `WORKER_DIGEST_INTERVAL`, `REFRESH_TIMEOUT`, `DIGEST_TIME`, and `DIGEST_TIMEZONE`.

Production deploys install a `second-brain-cycle.timer` systemd timer on the VPS. The timer starts shortly after deploy and then every 2 hours, running refresh, graph sync when `NEO4J_*` is configured, and digest delivery under OneCLI secret injection.

Sync pending graph events into Neo4j:

```bash
npm run graph:sync
```

The graph sync reads `graph_sync_outbox` from Supabase and upserts `Source`, `Capture`, `Insight`, and `ActionItem` nodes plus their relationships into Neo4j. Configure `NEO4J_URI`, `NEO4J_USERNAME`, `NEO4J_PASSWORD`, and optionally `NEO4J_DATABASE`.

Run a headless full knowledge refresh, including all available X bookmark pages by default:

```bash
npm run refresh:run
```

Set `X_BOOKMARK_LIMIT` to a positive number for capped validation runs. Leave it unset or set it to `0` to fetch every page returned by the X bookmarks API.

If X returns `401 Unauthorized` or an invalid refresh token error, re-authorize through the Go backend:

```bash
npm run x:oauth
```

The backend uses OAuth 2.0 Authorization Code with PKCE with `tweet.read tweet.write users.read bookmark.read offline.access`, validates the authenticated profile against `X_EXPECTED_USERNAME` (default `mohantyabhijit`), encrypts the access and refresh tokens with `X_TOKEN_ENCRYPTION_KEY`, stores them in Supabase Postgres, and issues only a short-lived HTTP-only `second_brain_session` cookie to the browser. The frontend never receives X tokens.

Use the production X app credentials by default. Local scripts first look for Keychain services `second-brain/X_CLIENT_ID_PROD` and `second-brain/X_CLIENT_SECRET_PROD`, then fall back to the non-prod names. `npm run x:prod:save-client` copies those production Keychain values into OneCLI so token endpoint calls can receive `client_id` and `Authorization: Basic ...` through OneCLI injection when the backend is running under `onecli run`.

Use the same `SUPABASE_DB_URL` and the same `X_TOKEN_ENCRYPTION_KEY` in local dev and production. That makes the single encrypted `x_oauth_tokens` row the token source for both environments; refresh rotation writes the new refresh token back to the same row before the next run uses it. `X_USER_ACCESS_TOKEN` and `X_REFRESH_TOKEN` are now legacy migration fallbacks only.

Generate the encryption key once, then save that exact value in local Keychain and production secrets:

```bash
openssl rand -base64 32
```

Configure the X Developer Console callback URL to match the backend callback, for example:

```bash
http://localhost:8080/api/auth/x/callback
```

For a deployed backend, set `X_REDIRECT_URI` to the deployed `/api/auth/x/callback` URL and run:

```bash
SECOND_BRAIN_API_BASE_URL=https://your-backend.example.com npm run x:oauth:prod
```

Each successful X token refresh writes non-secret rotation metadata to `X_TOKEN_ROTATION_PATH` (default `../data/runtime/x-token-rotation.json` from the backend working directory), including `rotatedAt`, `accessTokenExpiresAt`, `expiresInSeconds`, scope, and the reauthorization command. X access tokens are short lived; `offline.access` gives the app a refresh token so the worker can refresh without browser interaction.

To verify the one-time setup without printing secrets:

```bash
npm run x:check
npm run x:check:prod
npm run x:prod:check
npm run x:token:status
```

The digest command remains idempotent per owner and digest content fingerprint.

## Secrets

Store provider secrets in OneCLI, Keychain, GitHub Actions, or export them only for a local validation session. X user tokens should come from backend OAuth and the shared Postgres token row; do not keep fresh X user tokens in the frontend or checked-in env files.

```bash
export X_CLIENT_ID=...
export X_CLIENT_SECRET=...
export X_SESSION_SECRET=...
export X_TOKEN_ENCRYPTION_KEY=...
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
