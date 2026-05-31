alter table public.user_profiles
  add column if not exists handle text not null default '',
  add column if not exists display_name text not null default '',
  add column if not exists auth_user_id uuid references auth.users(id) on delete set null,
  add column if not exists is_public_owner boolean not null default false;

update public.user_profiles
set
  handle = 'abhijitmohanty',
  display_name = 'Abhijit Mohanty',
  is_public_owner = true,
  updated_at = now()
where id = '00000000-0000-0000-0000-000000000001';

insert into public.source_connections (
  owner_id,
  provider,
  provider_account_id,
  scopes,
  token_ref,
  token_status,
  last_validated_at
)
values (
  '00000000-0000-0000-0000-000000000001',
  'youtube',
  'PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r',
  '{}'::text[],
  'public-playlist:PLH_SZ1gwLn4gpQyZICprtx3nKRYGPKE7r',
  'active',
  now()
)
on conflict (owner_id, provider, provider_account_id) do update set
  token_ref = excluded.token_ref,
  token_status = excluded.token_status,
  last_validated_at = excluded.last_validated_at,
  updated_at = now();

create unique index if not exists user_profiles_handle_uidx
  on public.user_profiles (lower(handle))
  where handle <> '';

create unique index if not exists user_profiles_auth_user_id_uidx
  on public.user_profiles (auth_user_id)
  where auth_user_id is not null;

create index if not exists user_profiles_public_owner_idx
  on public.user_profiles (is_public_owner, handle);
