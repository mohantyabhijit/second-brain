# Deployment

Second Brain is deployed by GitHub Actions from this repository to the same VPS that serves `abhijitmohanty.com`.

## Runtime Shape

- The Next.js frontend is exported as static files with `NEXT_PUBLIC_BASE_PATH=/second-brain`.
- The browser calls the API through the same origin at `/second-brain/api`.
- nginx routes `/second-brain/` to `/srv/second-brain/frontend/current/`.
- nginx routes `/second-brain/api/` to the Go API on `127.0.0.1:8090`.
- The Go API runs under systemd as `second-brain-api`.
- Supabase migrations are applied on every deploy before the API restarts.

## Secret Placement

GitHub Actions stores only deploy/runtime secrets needed outside OneCLI:

- `DO_HOST`, `DO_PORT`, `DO_USER`, `DO_SSH_KEY`: SSH deployment to the VPS.
- `SUPABASE_DB_URL`: required by the API and migration binary for Postgres.
- `SUPABASE_URL`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_STORAGE_BUCKET`: backend-only Supabase Storage access.
- `REDIS_URL`: optional backend-only Redis read-model cache override. When absent, deploy provisions Redis on the VPS and uses `redis://127.0.0.1:6379/0`; deploy enables `REDIS_CACHE_ENABLED=true`.
- `MEMORY_PROFILING_ENABLED`, `MEMORY_PROFILE_TOKEN`: optional backend profiling controls. When enabled in production, call `/second-brain/api/debug/memory` or `/second-brain/api/debug/pprof/heap?debug=1` with `Authorization: Bearer $MEMORY_PROFILE_TOKEN`.
- `ONECLI_API_KEY`: logs the VPS deploy user into OneCLI so the API can run behind the OneCLI gateway.

Provider API credentials stay in OneCLI instead of GitHub Secrets where possible:

- X access token and refresh token.
- YouTube API key.
- Supadata API key.
- OpenAI API key.

The frontend does not receive provider secrets, Redis credentials, or Supabase service credentials.
