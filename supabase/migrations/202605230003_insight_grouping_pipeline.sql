create extension if not exists pgcrypto;
create extension if not exists vector;

create table if not exists public.insights (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  source_capture_id uuid not null references public.source_captures(id) on delete cascade,
  knowledge_synthesis_id uuid references public.knowledge_syntheses(id) on delete set null,
  external_insight_id text not null,
  title text not null default '',
  raw_text text not null,
  canonical_text text not null,
  abstract_text text not null default '',
  practical_text text not null default '',
  mechanism text not null default '',
  insight_type text not null default 'principle',
  domain text not null default 'general',
  topics text[] not null default '{}',
  entities text[] not null default '{}',
  confidence text not null default 'medium',
  explicit_or_inferred text not null default 'inferred',
  importance_score numeric not null default 0,
  novelty_score numeric not null default 0,
  actionability_score numeric not null default 0,
  embedding_text text not null default '',
  generated_at timestamptz not null default now(),
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (source_capture_id, external_insight_id)
);

create table if not exists public.insight_evidence (
  insight_id uuid not null references public.insights(id) on delete cascade,
  source_item_id uuid not null references public.source_items(id) on delete cascade,
  source_capture_id uuid not null references public.source_captures(id) on delete cascade,
  source_chunk_id uuid references public.source_chunks(id) on delete set null,
  evidence_index integer not null default 0,
  evidence_text text not null,
  created_at timestamptz not null default now(),
  primary key (insight_id, evidence_index)
);

create table if not exists public.insight_embeddings (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  insight_id uuid not null references public.insights(id) on delete cascade,
  embedding_key text not null,
  model text not null,
  dimensions integer not null,
  embedding vector(1536) not null,
  created_at timestamptz not null default now(),
  unique (owner_id, embedding_key, model)
);

create table if not exists public.insight_clusters (
  id uuid primary key default gen_random_uuid(),
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  external_cluster_key text not null,
  label text not null,
  canonical_insight text not null default '',
  cluster_summary text not null default '',
  cluster_layer text not null default 'similar_insight',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  unique (owner_id, cluster_layer, external_cluster_key)
);

create table if not exists public.cluster_memberships (
  cluster_id uuid not null references public.insight_clusters(id) on delete cascade,
  insight_id uuid not null references public.insights(id) on delete cascade,
  similarity_score numeric not null default 0,
  membership_confidence text not null default 'medium',
  created_at timestamptz not null default now(),
  primary key (cluster_id, insight_id)
);

alter table public.insights enable row level security;
alter table public.insight_evidence enable row level security;
alter table public.insight_embeddings enable row level security;
alter table public.insight_clusters enable row level security;
alter table public.cluster_memberships enable row level security;

drop policy if exists "insights_no_browser_access" on public.insights;
create policy "insights_no_browser_access" on public.insights for all using (false) with check (false);

drop policy if exists "insight_evidence_no_browser_access" on public.insight_evidence;
create policy "insight_evidence_no_browser_access" on public.insight_evidence for all using (false) with check (false);

drop policy if exists "insight_embeddings_no_browser_access" on public.insight_embeddings;
create policy "insight_embeddings_no_browser_access" on public.insight_embeddings for all using (false) with check (false);

drop policy if exists "insight_clusters_no_browser_access" on public.insight_clusters;
create policy "insight_clusters_no_browser_access" on public.insight_clusters for all using (false) with check (false);

drop policy if exists "cluster_memberships_no_browser_access" on public.cluster_memberships;
create policy "cluster_memberships_no_browser_access" on public.cluster_memberships for all using (false) with check (false);

create index if not exists insights_owner_capture_idx
  on public.insights (owner_id, source_capture_id, generated_at desc);

create index if not exists insights_owner_type_domain_idx
  on public.insights (owner_id, insight_type, domain);

create index if not exists insights_topics_idx
  on public.insights using gin (topics);

create index if not exists insight_evidence_capture_idx
  on public.insight_evidence (source_capture_id);

create index if not exists insight_embeddings_owner_model_idx
  on public.insight_embeddings (owner_id, model);

create index if not exists insight_embeddings_vector_idx
  on public.insight_embeddings using ivfflat (embedding vector_cosine_ops) with (lists = 100);

create index if not exists insight_clusters_owner_layer_idx
  on public.insight_clusters (owner_id, cluster_layer, updated_at desc);

create index if not exists cluster_memberships_insight_idx
  on public.cluster_memberships (insight_id);
