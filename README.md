# Second Brain Research Agent

Source-grounded research inbox for saved X posts and YouTube videos. The repo is now split into a React frontend, a Go backend, and Supabase-managed database schema.

## Structure

```text
frontend/   Next.js React console
backend/    Go API, ingestion pipeline, Supabase persistence
supabase/   Database migrations
docs/       Architecture and operating notes
scripts/    Local setup helpers
```

## Local Setup

Apply the Supabase migration in `supabase/migrations`, then set `SUPABASE_DB_URL` to the pooled Postgres connection string.

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

## Secrets

Store provider secrets in OneCLI or export them only for a local validation session:

```bash
export X_USER_ACCESS_TOKEN=...
export YOUTUBE_API_KEY=...
export YOUTUBE_ACCESS_TOKEN=...
export SUPADATA_API_KEY=...
export OPENAI_API_KEY=...
npm run onecli:save-secrets
```

`YOUTUBE_PLAYLIST_ID` is intentionally a non-secret backend setting. Use a dedicated playlist such as `Second Brain Inbox`; the official YouTube API blocks Watch Later listing.

## Validation

```bash
npm run typecheck
npm run lint
npm run build
npm run backend:test
```
