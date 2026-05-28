create table if not exists public.digest_source_items (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  digest_issue_id uuid not null references public.digest_issues(id) on delete cascade,
  source_item_id uuid references public.source_items(id) on delete set null,
  source_capture_id uuid references public.source_captures(id) on delete set null,
  knowledge_synthesis_id uuid references public.knowledge_syntheses(id) on delete set null,
  source_type text not null,
  external_id text not null,
  capture_hash text not null default '',
  source_url text not null default '',
  title text not null default '',
  first_seen_at timestamptz,
  captured_at timestamptz,
  synthesized_at timestamptz,
  digest_role text not null default 'input',
  created_at timestamptz not null default now(),
  unique (digest_issue_id, source_type, external_id, capture_hash)
);

alter table public.digest_source_items enable row level security;

drop policy if exists "digest_source_items_no_browser_access" on public.digest_source_items;
create policy "digest_source_items_no_browser_access"
  on public.digest_source_items
  for all
  using (false)
  with check (false);

create index if not exists digest_source_items_digest_idx
  on public.digest_source_items (digest_issue_id, first_seen_at asc);

create index if not exists digest_source_items_owner_source_idx
  on public.digest_source_items (owner_id, source_type, external_id);

create index if not exists digest_source_items_source_item_idx
  on public.digest_source_items (source_item_id);
