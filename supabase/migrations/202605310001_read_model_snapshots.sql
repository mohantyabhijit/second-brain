create table if not exists public.read_model_snapshots (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  schema_version text not null,
  run_id text not null,
  etag text not null default '',
  generated_at timestamptz not null,
  published_at timestamptz not null,
  payload jsonb not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (owner_id, run_id)
);

alter table public.read_model_snapshots enable row level security;

drop policy if exists "read_model_snapshots_no_browser_access" on public.read_model_snapshots;
create policy "read_model_snapshots_no_browser_access"
  on public.read_model_snapshots
  for all
  using (false)
  with check (false);

create index if not exists read_model_snapshots_owner_published_idx
  on public.read_model_snapshots (owner_id, schema_version, published_at desc);

create index if not exists read_model_snapshots_payload_gin_idx
  on public.read_model_snapshots
  using gin (payload jsonb_path_ops);
