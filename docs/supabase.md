# Supabase Setup

1. Create a Supabase project.
2. Apply `supabase/migrations/202605210001_phase_one_runs.sql`.
3. Copy the pooled Postgres connection string into `SUPABASE_DB_URL`.
4. Keep `SUPABASE_DB_URL` server-side only. Do not expose it as a `NEXT_PUBLIC_` variable.

The app currently stores complete Phase 1 run payloads in `phase_one_runs.payload`. When the source model stabilizes, split this into normalized `source_items`, `transcripts`, and `summaries` tables while keeping `phase_one_runs` as the audit log.
