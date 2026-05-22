create extension if not exists pgcrypto;

create table if not exists public.source_items (
  id uuid primary key default gen_random_uuid(),
  source_type text not null,
  external_id text not null,
  source_url text not null,
  title text not null default '',
  author_name text not null default '',
  username text not null default '',
  published_at timestamptz,
  capture_hash text not null default '',
  processing_state text not null default 'seen',
  first_seen_at timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  unique (source_type, external_id)
);

create table if not exists public.source_objects (
  id uuid primary key default gen_random_uuid(),
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  kind text not null,
  bucket text not null,
  path text not null,
  checksum text not null,
  content_type text not null,
  byte_size bigint not null,
  captured_at timestamptz not null default now(),
  unique (source_item_id, kind, checksum)
);

create table if not exists public.knowledge_syntheses (
  id uuid primary key default gen_random_uuid(),
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  capture_hash text not null,
  prompt_version text not null,
  model text not null,
  summary jsonb not null,
  insights jsonb not null default '[]'::jsonb,
  action_items jsonb not null default '[]'::jsonb,
  generated_at timestamptz not null default now(),
  unique (source_item_id, capture_hash, prompt_version, model)
);

alter table public.source_items enable row level security;
alter table public.source_objects enable row level security;
alter table public.knowledge_syntheses enable row level security;

drop policy if exists "source_items_no_browser_access" on public.source_items;
create policy "source_items_no_browser_access"
  on public.source_items
  for all
  using (false)
  with check (false);

drop policy if exists "source_objects_no_browser_access" on public.source_objects;
create policy "source_objects_no_browser_access"
  on public.source_objects
  for all
  using (false)
  with check (false);

drop policy if exists "knowledge_syntheses_no_browser_access" on public.knowledge_syntheses;
create policy "knowledge_syntheses_no_browser_access"
  on public.knowledge_syntheses
  for all
  using (false)
  with check (false);

create index if not exists source_items_last_seen_idx
  on public.source_items (last_seen_at desc);

create index if not exists source_items_processing_state_idx
  on public.source_items (processing_state);

create index if not exists source_objects_source_item_idx
  on public.source_objects (source_item_id);

create index if not exists knowledge_syntheses_cache_idx
  on public.knowledge_syntheses (source_item_id, capture_hash, prompt_version, model);
