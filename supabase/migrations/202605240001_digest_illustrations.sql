alter table public.digest_issues
  add column if not exists illustration_prompt text not null default '',
  add column if not exists illustration_alt text not null default '',
  add column if not exists illustration_mime_type text not null default '',
  add column if not exists illustration_base64 text not null default '',
  add column if not exists illustration_model text not null default '';
