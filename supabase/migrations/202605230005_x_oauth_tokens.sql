create table if not exists public.x_oauth_tokens (
  owner_id uuid primary key references public.user_profiles(id) on delete cascade,
  access_token_ciphertext text not null,
  refresh_token_ciphertext text not null,
  access_expires_at timestamptz not null,
  scope text not null default '',
  token_type text not null default 'bearer',
  authenticated_x_user_id text not null default '',
  authenticated_x_username text not null default '',
  authenticated_x_name text not null default '',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

alter table public.x_oauth_tokens enable row level security;

drop policy if exists "x_oauth_tokens_no_browser_access" on public.x_oauth_tokens;
create policy "x_oauth_tokens_no_browser_access"
  on public.x_oauth_tokens
  for all
  using (false)
  with check (false);

create index if not exists x_oauth_tokens_updated_idx
  on public.x_oauth_tokens (updated_at desc);
