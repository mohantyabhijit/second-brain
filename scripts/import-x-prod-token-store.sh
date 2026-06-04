#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

load_keychain() {
  local env_name="$1"
  local service="$2"
  if [[ -z "${!env_name:-}" ]]; then
    if value="$(security find-generic-password -a "$USER" -s "$service" -w 2>/dev/null)"; then
      export "$env_name=$value"
    else
      echo "$service is missing from Keychain" >&2
      exit 1
    fi
  fi
}

if [[ -z "${DATABASE_URL:-}" ]]; then
  if value="$(security find-generic-password -a "$USER" -s "second-brain/DATABASE_URL" -w 2>/dev/null)"; then
    export DATABASE_URL="$value"
  else
    load_keychain DATABASE_URL "second-brain/SUPABASE_DB_URL"
  fi
fi
if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  export SUPABASE_DB_URL="$DATABASE_URL"
fi
load_keychain X_CLIENT_ID "second-brain/X_CLIENT_ID_PROD"
load_keychain X_CLIENT_SECRET "second-brain/X_CLIENT_SECRET_PROD"
load_keychain X_TOKEN_ENCRYPTION_KEY "second-brain/X_TOKEN_ENCRYPTION_KEY"
load_keychain X_REFRESH_TOKEN "second-brain/X_REFRESH_TOKEN_PROD"

export APP_ENV="${APP_ENV:-production}"
export ONECLI_GATEWAY="${ONECLI_GATEWAY:-false}"
export X_EXPECTED_USERNAME="${X_EXPECTED_USERNAME:-mohantyabhijit}"
export X_OAUTH_SCOPES="${X_OAUTH_SCOPES:-tweet.read users.read bookmark.read offline.access}"
export X_TOKEN_REFRESH_DIRECT="${X_TOKEN_REFRESH_DIRECT:-true}"
export X_REAUTHORIZE_COMMAND="${X_REAUTHORIZE_COMMAND:-npm run x:oauth:prod}"
export X_KEYCHAIN_TOKEN_SUFFIX="${X_KEYCHAIN_TOKEN_SUFFIX:-_PROD}"
export X_TOKEN_ROTATION_PATH="${X_TOKEN_ROTATION_PATH:-$ROOT_DIR/data/runtime/x-token-rotation.json}"

cd "$BACKEND_DIR"
exec go run ./cmd/x-token-import
