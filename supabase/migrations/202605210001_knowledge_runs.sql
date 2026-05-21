create extension if not exists pgcrypto;

do $$
begin
  if to_regclass('public.knowledge_runs') is null and to_regclass('public.phase_one_runs') is not null then
    alter table public.phase_one_runs rename to knowledge_runs;
  end if;
end $$;

create table if not exists public.knowledge_runs (
  id uuid primary key default gen_random_uuid(),
  generated_at timestamptz not null,
  payload jsonb not null,
  created_at timestamptz not null default now()
);

alter table public.knowledge_runs enable row level security;

drop policy if exists "phase_one_runs_no_browser_access" on public.knowledge_runs;
drop policy if exists "knowledge_runs_no_browser_access" on public.knowledge_runs;
create policy "knowledge_runs_no_browser_access"
  on public.knowledge_runs
  for all
  using (false)
  with check (false);

create index if not exists knowledge_runs_generated_at_idx
  on public.knowledge_runs (generated_at desc);

create index if not exists knowledge_runs_payload_gin_idx
  on public.knowledge_runs
  using gin (payload jsonb_path_ops);
