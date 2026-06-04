create table if not exists public.youtube_transcript_requests (
  owner_id uuid not null references public.user_profiles(id) on delete cascade,
  video_id text not null,
  status text not null default 'claimed',
  detail text not null default '',
  attempted_at timestamptz not null default now(),
  completed_at timestamptz,
  primary key (owner_id, video_id)
);

alter table public.youtube_transcript_requests enable row level security;

drop policy if exists "youtube_transcript_requests_no_browser_access" on public.youtube_transcript_requests;
create policy "youtube_transcript_requests_no_browser_access"
  on public.youtube_transcript_requests
  for all
  using (false)
  with check (false);

create index if not exists youtube_transcript_requests_status_idx
  on public.youtube_transcript_requests (owner_id, status, attempted_at desc);

insert into public.youtube_transcript_requests (
  owner_id,
  video_id,
  status,
  detail,
  attempted_at,
  completed_at
)
select distinct
  owner_id,
  external_id,
  'legacy_backfill',
  'Backfilled from an existing YouTube source item; Supadata refetch disabled.',
  first_seen_at,
  now()
from public.source_items
where source_type = 'youtube'
  and external_id <> ''
on conflict (owner_id, video_id) do nothing;

insert into public.youtube_transcript_requests (
  owner_id,
  video_id,
  status,
  detail,
  attempted_at,
  completed_at
)
select distinct
  run.owner_id,
  item->>'videoId',
  'legacy_backfill',
  'Backfilled from a historical knowledge run; Supadata refetch disabled.',
  run.generated_at,
  now()
from public.knowledge_runs run
cross join lateral jsonb_array_elements(
  case
    when jsonb_typeof(run.payload->'youtubeItems') = 'array' then run.payload->'youtubeItems'
    else '[]'::jsonb
  end
) item
where coalesce(item->>'videoId', '') <> ''
on conflict (owner_id, video_id) do nothing;
