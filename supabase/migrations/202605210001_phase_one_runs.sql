create extension if not exists pgcrypto;

create table if not exists public.phase_one_runs (
  id uuid primary key default gen_random_uuid(),
  generated_at timestamptz not null,
  payload jsonb not null,
  created_at timestamptz not null default now()
);

alter table public.phase_one_runs enable row level security;

drop policy if exists "phase_one_runs_no_browser_access" on public.phase_one_runs;
create policy "phase_one_runs_no_browser_access"
  on public.phase_one_runs
  for all
  using (false)
  with check (false);

create index if not exists phase_one_runs_generated_at_idx
  on public.phase_one_runs (generated_at desc);

create index if not exists phase_one_runs_payload_gin_idx
  on public.phase_one_runs
  using gin (payload jsonb_path_ops);
