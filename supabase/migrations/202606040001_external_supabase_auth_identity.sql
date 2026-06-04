do $$
declare
  constraint_name text;
begin
  for constraint_name in
    select c.conname
    from pg_constraint c
    join pg_class r on r.oid = c.conrelid
    join pg_namespace n on n.oid = r.relnamespace
    where n.nspname = 'public'
      and r.relname = 'user_profiles'
      and c.contype = 'f'
      and pg_get_constraintdef(c.oid) like 'FOREIGN KEY (auth_user_id) REFERENCES auth.users%'
  loop
    execute format('alter table public.user_profiles drop constraint %I', constraint_name);
  end loop;
end $$;
