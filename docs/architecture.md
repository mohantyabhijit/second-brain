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

## Database

The first migration creates `public.knowledge_runs`, storing each refresh result as JSONB. This keeps the app flexible while preserving complete run output for later normalization.

RLS is enabled with a deny-all browser policy. Backend connections should use the Supabase pooled Postgres connection string from server-side environment variables.
