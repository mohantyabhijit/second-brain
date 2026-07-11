#!/usr/bin/env bash

onecli_runtime_init() {
  ONECLI_RUNTIME_MODE="${1:?runtime mode is required}"
  ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[1]}")/.." && pwd)"
  BACKEND_DIR="$ROOT_DIR/backend"
  ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
  PROJECT="${ONECLI_PROJECT:-second-brain}"

  onecli_source_env "$BACKEND_DIR/.env"
  if [[ "$ONECLI_RUNTIME_MODE" == "api" ]]; then
    onecli_source_env "$ROOT_DIR/frontend/.env.local"
  fi

  local keys=(
    DATABASE_URL OBJECT_STORAGE_BACKEND OBJECT_STORAGE_ROOT OBJECT_STORAGE_BUCKET
    REDIS_URL X_CLIENT_ID X_CLIENT_SECRET X_SESSION_SECRET X_TOKEN_ENCRYPTION_KEY
    X_USER_ACCESS_TOKEN X_REFRESH_TOKEN DIGEST_EMAIL_TO DIGEST_EMAIL_FROM
  )
  case "$ONECLI_RUNTIME_MODE" in
    api|worker) keys+=(EXA_API_KEY) ;;
  esac
  if [[ "$ONECLI_RUNTIME_MODE" == "api" ]]; then
    keys+=(SUPABASE_URL SUPABASE_PUBLISHABLE_KEY)
  fi
  for key in "${keys[@]}"; do
    onecli_load_keychain "$key"
  done

  onecli_load_keychain X_CLIENT_ID_PROD
  onecli_load_keychain X_CLIENT_SECRET_PROD
  onecli_load_keychain X_USER_ACCESS_TOKEN_PROD
  onecli_load_keychain X_REFRESH_TOKEN_PROD
  export X_CLIENT_ID="${X_CLIENT_ID_PROD:-${X_CLIENT_ID:-}}"
  export X_CLIENT_SECRET="${X_CLIENT_SECRET_PROD:-${X_CLIENT_SECRET:-}}"
  export X_USER_ACCESS_TOKEN="${X_USER_ACCESS_TOKEN_PROD:-${X_USER_ACCESS_TOKEN:-}}"
  export X_REFRESH_TOKEN="${X_REFRESH_TOKEN_PROD:-${X_REFRESH_TOKEN:-}}"

  if [[ "$ONECLI_RUNTIME_MODE" == "api" ]]; then
    export SUPABASE_URL="${SUPABASE_URL:-${NEXT_PUBLIC_SUPABASE_URL:-}}"
    export SUPABASE_PUBLISHABLE_KEY="${SUPABASE_PUBLISHABLE_KEY:-${NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY:-}}"
  fi

  export ONECLI_GATEWAY=true
  export LANGFUSE_BASE_URL="${LANGFUSE_BASE_URL:-https://jp.cloud.langfuse.com}"
  export OPENAI_SYNTHESIS_MODEL="${OPENAI_SYNTHESIS_MODEL:-gpt-5.4-mini}"
  export OPENAI_CHAT_MODEL="${OPENAI_CHAT_MODEL:-gpt-5.4}"
  export X_TOKEN_REFRESH_DIRECT="${X_TOKEN_REFRESH_DIRECT:-true}"
  export X_REAUTHORIZE_COMMAND="${X_REAUTHORIZE_COMMAND:-npm run x:oauth:prod}"
  export X_KEYCHAIN_TOKEN_SUFFIX="${X_KEYCHAIN_TOKEN_SUFFIX:-_PROD}"
  export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"
  if [[ -n "${REDIS_URL:-}" ]]; then
    export REDIS_CACHE_ENABLED="${REDIS_CACHE_ENABLED:-true}"
  else
    export REDIS_CACHE_ENABLED="${REDIS_CACHE_ENABLED:-false}"
  fi
  export REDIS_CACHE_TTL="${REDIS_CACHE_TTL:-720h}"
  export REDIS_REFRESH_STATUS_TTL="${REDIS_REFRESH_STATUS_TTL:-24h}"
  export REDIS_ASK_ANSWER_TTL="${REDIS_ASK_ANSWER_TTL:-1h}"

  case "$ONECLI_RUNTIME_MODE" in
    digest)
      export OPENAI_IMAGE_MODEL="${OPENAI_IMAGE_MODEL:-gpt-image-1}"
      export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-https://abhijitmohanty.com/second-brain}"
      ;;
    worker)
      export WORKER_REFRESH_INTERVAL="${WORKER_REFRESH_INTERVAL:-2h}"
      export DIGEST_TIME="${DIGEST_TIME:-18:00}"
      export DIGEST_TIMEZONE="${DIGEST_TIMEZONE:-Asia/Singapore}"
      ;;
    api|refresh) ;;
    *) echo "Unsupported OneCLI runtime mode: $ONECLI_RUNTIME_MODE" >&2; return 1 ;;
  esac
}

onecli_source_env() {
  local path="$1"
  if [[ -f "$path" ]]; then
    set -a
    # shellcheck disable=SC1090
    . "$path"
    set +a
  fi
}

onecli_load_keychain() {
  local key="$1"
  if [[ -n "${!key:-}" ]]; then
    return
  fi
  local value
  if value="$(security find-generic-password -a "$USER" -s "second-brain/$key" -w 2>/dev/null)"; then
    export "$key=$value"
  fi
}

onecli_run_backend() {
  local command="$1"
  case "$command" in
    api|digest|precompute|refresh|worker) ;;
    *) echo "Unsupported backend command: $command" >&2; return 1 ;;
  esac
  cd "$BACKEND_DIR"
  exec "$ONECLI" run --project "$PROJECT" -- go run "./cmd/$command"
}
