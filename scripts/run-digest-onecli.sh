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

if [[ -z "${DATABASE_URL:-}" ]]; then
  if DATABASE_URL="$(security find-generic-password -a "$USER" -s "second-brain/DATABASE_URL" -w 2>/dev/null)"; then
    export DATABASE_URL
  else
    unset DATABASE_URL
  fi
fi

for key in OBJECT_STORAGE_BACKEND OBJECT_STORAGE_ROOT OBJECT_STORAGE_BUCKET; do
  if [[ -z "${!key:-}" ]]; then
    if value="$(security find-generic-password -a "$USER" -s "second-brain/$key" -w 2>/dev/null)"; then
      export "$key=$value"
    fi
  fi
done

if [[ -z "${REDIS_URL:-}" ]]; then
  if REDIS_URL="$(security find-generic-password -a "$USER" -s "second-brain/REDIS_URL" -w 2>/dev/null)"; then
    export REDIS_URL
  else
    unset REDIS_URL
  fi
fi

if [[ -z "${X_CLIENT_ID:-}" ]]; then
  if X_CLIENT_ID_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID_PROD" -w 2>/dev/null)"; then
    export X_CLIENT_ID_PROD
    export X_CLIENT_ID="$X_CLIENT_ID_PROD"
  elif X_CLIENT_ID="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID" -w 2>/dev/null)"; then
    export X_CLIENT_ID
  else
    unset X_CLIENT_ID
  fi
fi

if [[ -z "${X_CLIENT_SECRET:-}" ]]; then
  if X_CLIENT_SECRET_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET_PROD" -w 2>/dev/null)"; then
    export X_CLIENT_SECRET_PROD
    export X_CLIENT_SECRET="$X_CLIENT_SECRET_PROD"
  elif X_CLIENT_SECRET="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET" -w 2>/dev/null)"; then
    export X_CLIENT_SECRET
  else
    unset X_CLIENT_SECRET
  fi
fi

if [[ -z "${X_SESSION_SECRET:-}" ]]; then
  if X_SESSION_SECRET="$(security find-generic-password -a "$USER" -s "second-brain/X_SESSION_SECRET" -w 2>/dev/null)"; then
    export X_SESSION_SECRET
  else
    unset X_SESSION_SECRET
  fi
fi

if [[ -z "${X_TOKEN_ENCRYPTION_KEY:-}" ]]; then
  if X_TOKEN_ENCRYPTION_KEY="$(security find-generic-password -a "$USER" -s "second-brain/X_TOKEN_ENCRYPTION_KEY" -w 2>/dev/null)"; then
    export X_TOKEN_ENCRYPTION_KEY
  else
    unset X_TOKEN_ENCRYPTION_KEY
  fi
fi

if [[ -z "${X_USER_ACCESS_TOKEN:-}" ]]; then
  if X_USER_ACCESS_TOKEN="$(security find-generic-password -a "$USER" -s "second-brain/X_USER_ACCESS_TOKEN" -w 2>/dev/null)"; then
    export X_USER_ACCESS_TOKEN
  else
    unset X_USER_ACCESS_TOKEN
  fi
fi

if [[ -z "${X_REFRESH_TOKEN:-}" ]]; then
  if X_REFRESH_TOKEN="$(security find-generic-password -a "$USER" -s "second-brain/X_REFRESH_TOKEN" -w 2>/dev/null)"; then
    export X_REFRESH_TOKEN
  else
    unset X_REFRESH_TOKEN
  fi
fi

if [[ -z "${DIGEST_EMAIL_TO:-}" ]]; then
  if DIGEST_EMAIL_TO="$(security find-generic-password -a "$USER" -s "second-brain/DIGEST_EMAIL_TO" -w 2>/dev/null)"; then
    export DIGEST_EMAIL_TO
  else
    unset DIGEST_EMAIL_TO
  fi
fi

if [[ -z "${DIGEST_EMAIL_FROM:-}" ]]; then
  if DIGEST_EMAIL_FROM="$(security find-generic-password -a "$USER" -s "second-brain/DIGEST_EMAIL_FROM" -w 2>/dev/null)"; then
    export DIGEST_EMAIL_FROM
  else
    unset DIGEST_EMAIL_FROM
  fi
fi

if X_CLIENT_ID_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID_PROD" -w 2>/dev/null)"; then
  export X_CLIENT_ID_PROD
  export X_CLIENT_ID="$X_CLIENT_ID_PROD"
fi

if X_CLIENT_SECRET_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET_PROD" -w 2>/dev/null)"; then
  export X_CLIENT_SECRET_PROD
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
export OPENAI_SYNTHESIS_MODEL="${OPENAI_SYNTHESIS_MODEL:-gpt-5.4-mini}"
export OPENAI_CHAT_MODEL="${OPENAI_CHAT_MODEL:-gpt-5.5}"
export OPENAI_IMAGE_MODEL="${OPENAI_IMAGE_MODEL:-gpt-image-1}"
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-https://abhijitmohanty.com/second-brain}"
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

cd "$BACKEND_DIR"
BACKEND_CMD="${SECOND_BRAIN_BACKEND_CMD:-digest}"
case "$BACKEND_CMD" in
  digest|precompute) ;;
  *)
    echo "Unsupported backend command: $BACKEND_CMD" >&2
    exit 1
    ;;
esac
exec "$ONECLI" run --project "$PROJECT" -- go run "./cmd/$BACKEND_CMD"
