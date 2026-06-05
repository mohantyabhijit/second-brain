#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
PROJECT="${ONECLI_PROJECT:-second-brain}"

if [[ -f "$BACKEND_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  . "$BACKEND_DIR/.env"
  set +a
fi

for key in \
  DATABASE_URL \
  OBJECT_STORAGE_BACKEND \
  OBJECT_STORAGE_ROOT \
  OBJECT_STORAGE_BUCKET \
  REDIS_URL \
  X_CLIENT_ID_PROD \
  X_CLIENT_SECRET_PROD \
  X_CLIENT_ID \
  X_CLIENT_SECRET \
  X_SESSION_SECRET \
  X_TOKEN_ENCRYPTION_KEY \
  X_USER_ACCESS_TOKEN \
  X_REFRESH_TOKEN \
  DIGEST_EMAIL_TO \
  DIGEST_EMAIL_FROM \
  EXA_API_KEY; do
  if [[ -z "${!key:-}" ]]; then
    if value="$(security find-generic-password -a "$USER" -s "second-brain/$key" -w 2>/dev/null)"; then
      export "$key=$value"
    fi
  fi
done
export X_CLIENT_ID="${X_CLIENT_ID:-${X_CLIENT_ID_PROD:-}}"
export X_CLIENT_SECRET="${X_CLIENT_SECRET:-${X_CLIENT_SECRET_PROD:-}}"
if [[ -n "${X_CLIENT_ID_PROD:-}" ]]; then
  export X_CLIENT_ID="$X_CLIENT_ID_PROD"
fi
if [[ -n "${X_CLIENT_SECRET_PROD:-}" ]]; then
  export X_CLIENT_SECRET="$X_CLIENT_SECRET_PROD"
fi
if X_USER_ACCESS_TOKEN_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_USER_ACCESS_TOKEN_PROD" -w 2>/dev/null)"; then
  export X_USER_ACCESS_TOKEN_PROD
  export X_USER_ACCESS_TOKEN="$X_USER_ACCESS_TOKEN_PROD"
fi
if X_REFRESH_TOKEN_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_REFRESH_TOKEN_PROD" -w 2>/dev/null)"; then
  export X_REFRESH_TOKEN_PROD
  export X_REFRESH_TOKEN="$X_REFRESH_TOKEN_PROD"
fi

export ONECLI_GATEWAY=true
export LANGFUSE_BASE_URL="${LANGFUSE_BASE_URL:-https://jp.cloud.langfuse.com}"
export OPENAI_SYNTHESIS_MODEL="${OPENAI_SYNTHESIS_MODEL:-gpt-5.4-mini}"
export OPENAI_CHAT_MODEL="${OPENAI_CHAT_MODEL:-gpt-5.4}"
export X_TOKEN_REFRESH_DIRECT="${X_TOKEN_REFRESH_DIRECT:-true}"
export X_REAUTHORIZE_COMMAND="${X_REAUTHORIZE_COMMAND:-npm run x:oauth:prod}"
export X_KEYCHAIN_TOKEN_SUFFIX="${X_KEYCHAIN_TOKEN_SUFFIX:-_PROD}"
export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"
export WORKER_REFRESH_INTERVAL="${WORKER_REFRESH_INTERVAL:-2h}"
export DIGEST_TIME="${DIGEST_TIME:-18:00}"
export DIGEST_TIMEZONE="${DIGEST_TIMEZONE:-Asia/Singapore}"
if [[ -n "${REDIS_URL:-}" ]]; then
  export REDIS_CACHE_ENABLED="${REDIS_CACHE_ENABLED:-true}"
else
  export REDIS_CACHE_ENABLED="${REDIS_CACHE_ENABLED:-false}"
fi
export REDIS_CACHE_TTL="${REDIS_CACHE_TTL:-720h}"
export REDIS_REFRESH_STATUS_TTL="${REDIS_REFRESH_STATUS_TTL:-24h}"
export REDIS_ASK_ANSWER_TTL="${REDIS_ASK_ANSWER_TTL:-1h}"

cd "$BACKEND_DIR"
exec "$ONECLI" run --project "$PROJECT" -- go run ./cmd/worker
