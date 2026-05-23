#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
API_BASE_URL="${SECOND_BRAIN_API_BASE_URL:-http://localhost:8080}"
status=0

if [[ -f "$BACKEND_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  . "$BACKEND_DIR/.env"
  set +a
fi

check_secret() {
  local key="$1"
  if [[ -n "${!key:-}" ]]; then
    echo "ok env: $key"
  elif security find-generic-password -a "$USER" -s "second-brain/$key" -w >/dev/null 2>&1; then
    echo "ok keychain: second-brain/$key"
  else
    echo "missing secret: $key"
    status=1
  fi
}

check_any_secret() {
  local label="$1"
  shift
  for key in "$@"; do
    if [[ -n "${!key:-}" ]] || security find-generic-password -a "$USER" -s "second-brain/$key" -w >/dev/null 2>&1; then
      echo "ok secret: $label via $key"
      return 0
    fi
  done
  echo "missing secret: $label ($*)"
  status=1
}

echo "Checking backend-managed X OAuth setup without printing secret values..."
check_secret SUPABASE_DB_URL
check_any_secret X_CLIENT_ID X_CLIENT_ID_PROD X_CLIENT_ID
check_any_secret X_CLIENT_SECRET X_CLIENT_SECRET_PROD X_CLIENT_SECRET
check_secret X_REDIRECT_URI
check_secret X_SESSION_SECRET
check_secret X_TOKEN_ENCRYPTION_KEY

if curl -fsS "$API_BASE_URL/healthz" >/dev/null; then
  echo "ok backend: $API_BASE_URL"
  if auth_status="$(curl -fsS "$API_BASE_URL/api/auth/x/status")"; then
    node -e '
      const status = JSON.parse(process.argv[1]);
      console.log(`x_oauth_configured=${status.configured}`);
      console.log(`x_oauth_authorized=${status.authorized}`);
      if (status.username) console.log(`x_username=@${status.username}`);
      if (status.accessExpiresAt) console.log(`access_expires_at=${status.accessExpiresAt}`);
      if (!status.configured || !status.authorized) process.exit(1);
    ' "$auth_status" || status=1
  else
    echo "missing backend auth status: $API_BASE_URL/api/auth/x/status"
    status=1
  fi
else
  echo "skip backend status: $API_BASE_URL is not running"
fi

if [[ "$status" -eq 0 ]]; then
  echo "X OAuth setup looks ready."
else
  echo "X OAuth setup is incomplete. Start the backend and run npm run x:oauth after configuring the X callback URL."
fi
exit "$status"
