create extension if not exists vector;

create table if not exists public.user_profiles (
  id uuid primary key,
  email text,
  timezone text not null default 'Asia/Singapore',
  digest_time time not null default '17:00',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

insert into public.user_profiles (id, email)
values ('00000000-0000-0000-0000-000000000001', null)
on conflict (id) do nothing;

alter table public.source_items
  add column if not exists owner_id uuid not null default '00000000-0000-0000-0000-000000000001' references public.user_profiles(id) on delete cascade;

alter table public.knowledge_runs
  add column if not exists owner_id uuid not null default '00000000-0000-0000-0000-000000000001' references public.user_profiles(id) on delete cascade;

do $$
begin
  if exists (
    select 1
    from pg_constraint
    where conname = 'source_items_source_type_external_id_key'
      and conrelid = 'public.source_items'::regclass
  ) then
    alter table public.source_items drop constraint source_items_source_type_external_id_key;
  end if;
end $$;

create unique index if not exists source_items_owner_source_external_uidx
  on public.source_items (owner_id, source_type, external_id);

create table if not exists public.source_connections (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  provider text not null,
  provider_account_id text not null default '',
  scopes text[] not null default '{}',
  token_ref text not null,
  token_status text not null default 'active',
  last_validated_at timestamptz,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (owner_id, provider, provider_account_id)
);

create table if not exists public.source_chunks (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  chunk_index integer not null,
  content text not null,
  token_estimate integer not null default 0,
  checksum text not null,
  created_at timestamptz not null default now(),
  unique (source_item_id, chunk_index, checksum)
);

create table if not exists public.source_embeddings (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  source_item_id uuid references public.source_items(id) on delete cascade,
  source_chunk_id uuid references public.source_chunks(id) on delete cascade,
  embedding_type text not null,
  embedding_key text not null,
  label text not null default '',
  model text not null,
  dimensions integer not null,
  embedding vector(1536) not null,
  created_at timestamptz not null default now(),
  unique (owner_id, embedding_key, model)
);

create table if not exists public.theme_clusters (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  run_id uuid references public.knowledge_runs(id) on delete cascade,
  label text not null,
  evidence text not null default '',
  score numeric not null default 0,
  created_at timestamptz not null default now()
);

create table if not exists public.theme_cluster_items (
  theme_cluster_id uuid not null references public.theme_clusters(id) on delete cascade,
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  evidence text not null default '',
  primary key (theme_cluster_id, source_item_id)
);

create table if not exists public.source_connections_evidence (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  run_id uuid references public.knowledge_runs(id) on delete cascade,
  left_source_item_id uuid not null references public.source_items(id) on delete cascade,
  right_source_item_id uuid not null references public.source_items(id) on delete cascade,
  relationship text not null default 'related_to',
  evidence text not null,
  confidence text not null default 'medium',
  created_at timestamptz not null default now(),
  unique (run_id, left_source_item_id, right_source_item_id, relationship)
);

create table if not exists public.feedback_events (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  target_type text not null,
  target_id text not null,
  signal text not null,
  note text not null default '',
  source_url text not null default '',
  created_at timestamptz not null default now()
);

create table if not exists public.digest_issues (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  digest_date date not null,
  scheduled_for timestamptz not null,
  idempotency_key text not null,
  subject text not null,
  body_markdown text not null,
  status text not null default 'generated',
  generated_from_run_id uuid references public.knowledge_runs(id) on delete set null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (owner_id, idempotency_key)
);

create table if not exists public.digest_deliveries (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  digest_issue_id uuid not null references public.digest_issues(id) on delete cascade,
  provider text not null,
  recipient text not null,
  status text not null,
  provider_message_id text not null default '',
  error text not null default '',
  attempted_at timestamptz not null default now()
);

create table if not exists public.graph_sync_outbox (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  aggregate_type text not null,
  aggregate_id uuid not null,
  event_type text not null,
  payload jsonb not null,
  status text not null default 'pending',
  attempts integer not null default 0,
  available_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  processed_at timestamptz
);

alter table public.user_profiles enable row level security;
alter table public.source_connections enable row level security;
alter table public.source_chunks enable row level security;
alter table public.source_embeddings enable row level security;
alter table public.theme_clusters enable row level security;
alter table public.theme_cluster_items enable row level security;
alter table public.source_connections_evidence enable row level security;
alter table public.feedback_events enable row level security;
alter table public.digest_issues enable row level security;
alter table public.digest_deliveries enable row level security;
alter table public.graph_sync_outbox enable row level security;

drop policy if exists "user_profiles_no_browser_access" on public.user_profiles;
create policy "user_profiles_no_browser_access" on public.user_profiles for all using (false) with check (false);

drop policy if exists "source_connections_no_browser_access" on public.source_connections;
create policy "source_connections_no_browser_access" on public.source_connections for all using (false) with check (false);

drop policy if exists "source_chunks_no_browser_access" on public.source_chunks;
create policy "source_chunks_no_browser_access" on public.source_chunks for all using (false) with check (false);

drop policy if exists "source_embeddings_no_browser_access" on public.source_embeddings;
create policy "source_embeddings_no_browser_access" on public.source_embeddings for all using (false) with check (false);

drop policy if exists "theme_clusters_no_browser_access" on public.theme_clusters;
create policy "theme_clusters_no_browser_access" on public.theme_clusters for all using (false) with check (false);

drop policy if exists "theme_cluster_items_no_browser_access" on public.theme_cluster_items;
create policy "theme_cluster_items_no_browser_access" on public.theme_cluster_items for all using (false) with check (false);

drop policy if exists "source_connections_evidence_no_browser_access" on public.source_connections_evidence;
create policy "source_connections_evidence_no_browser_access" on public.source_connections_evidence for all using (false) with check (false);

drop policy if exists "feedback_events_no_browser_access" on public.feedback_events;
create policy "feedback_events_no_browser_access" on public.feedback_events for all using (false) with check (false);

drop policy if exists "digest_issues_no_browser_access" on public.digest_issues;
create policy "digest_issues_no_browser_access" on public.digest_issues for all using (false) with check (false);

drop policy if exists "digest_deliveries_no_browser_access" on public.digest_deliveries;
create policy "digest_deliveries_no_browser_access" on public.digest_deliveries for all using (false) with check (false);

drop policy if exists "graph_sync_outbox_no_browser_access" on public.graph_sync_outbox;
create policy "graph_sync_outbox_no_browser_access" on public.graph_sync_outbox for all using (false) with check (false);

create index if not exists knowledge_runs_owner_generated_at_idx
  on public.knowledge_runs (owner_id, generated_at desc);

create index if not exists source_chunks_source_item_idx
  on public.source_chunks (source_item_id, chunk_index);

create index if not exists source_embeddings_owner_type_idx
  on public.source_embeddings (owner_id, embedding_type, model);

create index if not exists source_embeddings_vector_idx
  on public.source_embeddings using ivfflat (embedding vector_cosine_ops) with (lists = 100);

create index if not exists theme_clusters_owner_run_idx
  on public.theme_clusters (owner_id, run_id, score desc);

create index if not exists feedback_events_owner_target_idx
  on public.feedback_events (owner_id, target_type, target_id, created_at desc);

create index if not exists digest_issues_owner_date_idx
  on public.digest_issues (owner_id, digest_date desc);

create index if not exists graph_sync_outbox_status_idx
  on public.graph_sync_outbox (status, available_at);
