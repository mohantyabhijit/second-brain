# Deployment

Second Brain is deployed by GitHub Actions from this repository to the same
`ubuntu-sgp` VPS that serves `abhijitmohanty.com`. Its PostgreSQL database runs
on the separate `codex-crapbox` VPS.

## Runtime Shape

- The Next.js frontend is exported as static files with `NEXT_PUBLIC_BASE_PATH=/second-brain`.
- The browser calls the API through the same origin at `/second-brain/api`.
- nginx routes `/second-brain/` to `/srv/second-brain/frontend/current/`.
- nginx routes `/second-brain/api/` to the Go API on `127.0.0.1:8090`.
- The Go API runs under systemd as `second-brain-api`.
- The refresh and digest worker runs under systemd as `second-brain-worker`.
- Postgres migrations are applied on every deploy before the API restarts.
- PostgreSQL 17 plus pgvector runs on `codex-crapbox` and listens on the
  DigitalOcean private VPC, not the public internet.
- Redis runs locally on `ubuntu-sgp` and is a rebuildable read-model cache.
- Runtime object storage is a private filesystem tree at `/srv/second-brain/object-storage`.

## Secret Placement

GitHub Actions stores only deploy/runtime secrets needed outside OneCLI:

- `DO_HOST`, `DO_PORT`, `DO_USER`, `DO_SSH_KEY`: SSH deployment to the VPS.
- `DATABASE_URL`: required by the API and migration binary for the self-hosted
  PostgreSQL database on `codex-crapbox`.
- `OBJECT_STORAGE_BACKEND`, `OBJECT_STORAGE_ROOT`, `OBJECT_STORAGE_BUCKET`: backend object storage settings. Production uses `filesystem`, `/srv/second-brain/object-storage`, and `sources`.
- `SUPABASE_URL`, `SUPABASE_PUBLISHABLE_KEY`: required by the Go API to validate Supabase Auth sessions. Supabase Auth is the only authentication provider.
- `NEXT_PUBLIC_SUPABASE_URL`, `NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY`: required by the static frontend for Supabase magic-link sign-in.
- `SUPADATA_MONTHLY_REQUEST_LIMIT`: hard ceiling for unique Supadata transcript
  requests per calendar month. Production sets it to `100` for the free tier.
- `REDIS_URL`: optional backend-only Redis read-model cache override. When absent, deploy provisions Redis on the VPS and uses `redis://127.0.0.1:6379/0`; deploy enables `REDIS_CACHE_ENABLED=true`.
- `CLOUDFLARE_API_TOKEN`, `CLOUDFLARE_ZONE_ID`, `CLOUDFLARE_CACHE_PURGE_ENABLED`: optional edge-cache purge credentials. When present, successful refresh and digest read-model publishes purge the static pages and app-state URLs that Cloudflare caches.
- `MEMORY_PROFILING_ENABLED`, `MEMORY_PROFILE_TOKEN`: optional backend profiling controls. When enabled in production, call `/second-brain/api/debug/memory` or `/second-brain/api/debug/pprof/heap?debug=1` with `Authorization: Bearer $MEMORY_PROFILE_TOKEN`.
- `ONECLI_API_KEY`: logs the VPS deploy user into OneCLI so the API can run behind the OneCLI gateway.

The `second-brain-edge-cache` Worker is deployed separately from the VPS workflow with `npm run edge-cache:deploy`. That deploy path needs `CLOUDFLARE_ACCOUNT_ID` or the checked-in Worker Wrangler config plus a token with Workers edit permissions; the runtime purge token only needs zone read and cache purge permissions.

Provider API credentials stay in OneCLI instead of GitHub Secrets where possible:

- X access token and refresh token.
- YouTube API key.
- Supadata API key.
- OpenAI API key.

The frontend receives only the public Supabase Auth configuration. It does not receive provider secrets, Redis credentials, database credentials, or object-storage credentials.

Do not configure `SUPABASE_DB_URL`, `SUPABASE_SERVICE_ROLE_KEY`, or `SUPABASE_STORAGE_BUCKET`. Supabase is an Auth-only dependency, and the backend intentionally does not support Supabase Database or Storage fallbacks.

## Backup Requirement

Before shutting down, rebuilding, or consolidating either droplet, create and
verify recurring encrypted offsite backups for:

- the `second_brain` PostgreSQL database;
- `/srv/second-brain/object-storage`;
- the deployment and service configuration needed to restore the API, worker,
  nginx, Redis read models, and provider credentials.

Database dumps and object archives stored only on `ubuntu-sgp` are recovery
copies, not offsite backups. The recommended low-cost target is Cloudflare R2's
free allowance, with restore tests performed from a different host.
