# Architecture

## Frontend

`frontend/` is a Next.js React app. Route files stay in `frontend/app`, while feature code lives under `frontend/src/features`. The Phase 1 console is intentionally client-side because it drives an operator action and reads the latest run from the API.

Key paths:

- `frontend/app/page.tsx`: thin route entrypoint.
- `frontend/src/features/phase-one/components/PhaseOneConsole.tsx`: dashboard UI.
- `frontend/src/features/phase-one/api.ts`: typed API client.
- `frontend/src/features/phase-one/types.ts`: shared API response shape.
- `frontend/src/utils/supabase`: optional Supabase Auth cookie helpers using publishable browser keys only.

## Backend

`backend/` is a Go service with a standard internal package layout:

- `cmd/api`: process bootstrap and graceful shutdown.
- `internal/config`: environment parsing.
- `internal/httpapi`: routing, JSON responses, and CORS.
- `internal/phaseone`: ingestion, transcript checks, validation, and summarization.
- `internal/store/postgres`: Supabase Postgres persistence.

The frontend never uses the Supabase database connection directly. It calls the Go API for product data, and the Go API uses the Supabase pooled Postgres connection string. The frontend may use Supabase publishable keys for auth session cookies only.

## Database

The first migration creates `public.phase_one_runs`, storing each Phase 1 result as JSONB. This keeps the MVP flexible while preserving complete run output for later normalization.

RLS is enabled with a deny-all browser policy. Backend connections should use the Supabase pooled Postgres connection string from server-side environment variables.
