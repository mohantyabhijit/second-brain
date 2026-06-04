#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
API_BASE_URL="${SECOND_BRAIN_API_BASE_URL:-http://localhost:8080}"

if [[ "${X_OAUTH_LEGACY_HELPER:-false}" == "true" ]]; then
  exec node "$ROOT_DIR/scripts/x-oauth-local.mjs"
fi

if [[ -f "$BACKEND_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  . "$BACKEND_DIR/.env"
  set +a
fi

if ! curl -fsS "$API_BASE_URL/healthz" >/dev/null; then
  cat >&2 <<EOF
Backend is not reachable at $API_BASE_URL.
Start it with npm run backend:dev, with DATABASE_URL, X_CLIENT_ID,
X_CLIENT_SECRET, X_SESSION_SECRET, and X_TOKEN_ENCRYPTION_KEY configured.
EOF
  exit 1
fi

auth_url="$API_BASE_URL/api/auth/x"
echo "Opening backend-managed X OAuth flow:"
echo "$auth_url"
open "$auth_url"
