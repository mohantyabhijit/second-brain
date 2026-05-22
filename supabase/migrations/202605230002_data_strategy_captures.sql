create extension if not exists pgcrypto;
create extension if not exists vector;

alter table public.source_items
  add column if not exists content_type text not null default 'post';

alter table public.source_items
  add column if not exists latest_capture_hash text not null default '';

update public.source_items
set latest_capture_hash = capture_hash
where latest_capture_hash = ''
  and capture_hash <> '';

create table if not exists public.source_captures (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null default '00000000-0000-0000-0000-000000000001' references public.user_profiles(id) on delete cascade,
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  capture_hash text not null,
  captured_at timestamptz not null default now(),
  raw_object_id uuid references public.source_objects(id) on delete set null,
  normalized_object_id uuid references public.source_objects(id) on delete set null,
  metadata jsonb not null default '{}'::jsonb,
  unique (source_item_id, capture_hash)
);

insert into public.source_captures (
  owner_id,
  source_item_id,
  capture_hash,
  captured_at,
  metadata
)
select
  si.owner_id,
  si.id,
  si.capture_hash,
  coalesce(si.last_seen_at, now()),
  jsonb_build_object(
    'sourceType', si.source_type,
    'contentType', si.content_type,
    'externalId', si.external_id,
    'sourceUrl', si.source_url,
    'title', si.title,
    'authorName', si.author_name,
    'username', si.username,
    'backfilled', true
  )
from public.source_items si
where si.capture_hash <> ''
on conflict (source_item_id, capture_hash) do nothing;

alter table public.source_objects
  add column if not exists owner_id uuid not null default '00000000-0000-0000-0000-000000000001' references public.user_profiles(id) on delete cascade;

alter table public.source_objects
  add column if not exists source_capture_id uuid references public.source_captures(id) on delete cascade;

alter table public.source_objects
  add column if not exists metadata jsonb not null default '{}'::jsonb;

update public.source_objects so
set owner_id = si.owner_id
from public.source_items si
where so.source_item_id = si.id
  and so.owner_id <> si.owner_id;

update public.source_objects so
set source_capture_id = sc.id
from public.source_captures sc
where so.source_item_id = sc.source_item_id
  and so.source_capture_id is null;

do $$
begin
  if exists (
    select 1
    from pg_constraint
    where conname = 'source_objects_source_item_id_kind_checksum_key'
      and conrelid = 'public.source_objects'::regclass
  ) then
    alter table public.source_objects drop constraint source_objects_source_item_id_kind_checksum_key;
  end if;
end $$;

alter table public.knowledge_syntheses
  add column if not exists owner_id uuid not null default '00000000-0000-0000-0000-000000000001' references public.user_profiles(id) on delete cascade;

alter table public.knowledge_syntheses
  add column if not exists source_capture_id uuid references public.source_captures(id) on delete cascade;

alter table public.knowledge_syntheses
  add column if not exists summary_object_id uuid references public.source_objects(id) on delete set null;

update public.knowledge_syntheses ks
set owner_id = si.owner_id
from public.source_items si
where ks.source_item_id = si.id
  and ks.owner_id <> si.owner_id;

update public.knowledge_syntheses ks
set source_capture_id = sc.id
from public.source_captures sc
where ks.source_item_id = sc.source_item_id
  and ks.capture_hash = sc.capture_hash
  and ks.source_capture_id is null;

alter table public.source_chunks
  add column if not exists source_capture_id uuid references public.source_captures(id) on delete cascade;

update public.source_chunks sch
set source_capture_id = sc.id
from public.source_captures sc
join public.source_items si on si.id = sc.source_item_id
where sch.source_item_id = sc.source_item_id
  and si.latest_capture_hash = sc.capture_hash
  and sch.source_capture_id is null;

do $$
begin
  if exists (
    select 1
    from pg_constraint
    where conname = 'source_chunks_source_item_id_chunk_index_checksum_key'
      and conrelid = 'public.source_chunks'::regclass
  ) then
    alter table public.source_chunks drop constraint source_chunks_source_item_id_chunk_index_checksum_key;
  end if;
end $$;

alter table public.source_chunks
  add column if not exists search_vector tsvector
  generated always as (to_tsvector('english', coalesce(content, ''))) stored;

alter table public.source_embeddings
  add column if not exists source_capture_id uuid references public.source_captures(id) on delete cascade;

update public.source_embeddings se
set source_capture_id = sc.id
from public.source_captures sc
join public.source_items si on si.id = sc.source_item_id
where se.source_item_id = sc.source_item_id
  and si.latest_capture_hash = sc.capture_hash
  and se.source_capture_id is null;

alter table public.source_captures enable row level security;

drop policy if exists "source_captures_no_browser_access" on public.source_captures;
create policy "source_captures_no_browser_access"
  on public.source_captures
  for all
  using (false)
  with check (false);

create index if not exists source_items_owner_state_seen_idx
  on public.source_items (owner_id, processing_state, last_seen_at desc);

create index if not exists source_items_owner_content_seen_idx
  on public.source_items (owner_id, content_type, last_seen_at desc);

create index if not exists source_captures_owner_captured_idx
  on public.source_captures (owner_id, captured_at desc);

create index if not exists source_captures_source_item_idx
  on public.source_captures (source_item_id, captured_at desc);

with ranked_source_objects as (
  select
    id,
    row_number() over (
      partition by bucket, path
      order by captured_at desc, id desc
    ) as rn
  from public.source_objects
)
delete from public.source_objects so
using ranked_source_objects ranked
where so.id = ranked.id
  and ranked.rn > 1;

create unique index if not exists source_objects_bucket_path_uidx
  on public.source_objects (bucket, path);

create index if not exists source_objects_capture_kind_idx
  on public.source_objects (source_capture_id, kind);

create unique index if not exists source_objects_capture_kind_checksum_uidx
  on public.source_objects (source_capture_id, kind, checksum);

create unique index if not exists knowledge_syntheses_capture_prompt_model_uidx
  on public.knowledge_syntheses (source_capture_id, prompt_version, model);

create index if not exists knowledge_syntheses_owner_generated_idx
  on public.knowledge_syntheses (owner_id, generated_at desc);

create unique index if not exists source_chunks_capture_index_checksum_uidx
  on public.source_chunks (source_capture_id, chunk_index, checksum);

create index if not exists source_chunks_capture_idx
  on public.source_chunks (source_capture_id, chunk_index);

create index if not exists source_chunks_search_idx
  on public.source_chunks using gin (search_vector);

create index if not exists source_embeddings_capture_type_idx
  on public.source_embeddings (source_capture_id, embedding_type, model);
