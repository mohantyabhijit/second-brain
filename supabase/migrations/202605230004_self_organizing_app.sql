alter table public.user_profiles
  alter column digest_time set default '18:00';

create index if not exists feedback_events_signal_created_idx
  on public.feedback_events (owner_id, signal, created_at desc);
