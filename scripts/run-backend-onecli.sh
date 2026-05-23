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

if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  if SUPABASE_DB_URL="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_DB_URL" -w 2>/dev/null)"; then
    export SUPABASE_DB_URL
  else
    unset SUPABASE_DB_URL
  fi
fi

if [[ -z "${SUPABASE_URL:-}" ]]; then
  if SUPABASE_URL="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_URL" -w 2>/dev/null)"; then
    export SUPABASE_URL
  else
    unset SUPABASE_URL
  fi
fi

if [[ -z "${SUPABASE_SERVICE_ROLE_KEY:-}" ]]; then
  if SUPABASE_SERVICE_ROLE_KEY="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_SERVICE_ROLE_KEY" -w 2>/dev/null)"; then
    export SUPABASE_SERVICE_ROLE_KEY
  else
    unset SUPABASE_SERVICE_ROLE_KEY
  fi
fi

if [[ -z "${SUPABASE_STORAGE_BUCKET:-}" ]]; then
  if SUPABASE_STORAGE_BUCKET="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_STORAGE_BUCKET" -w 2>/dev/null)"; then
    export SUPABASE_STORAGE_BUCKET
  else
    unset SUPABASE_STORAGE_BUCKET
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

if [[ -z "${EXA_API_KEY:-}" ]]; then
  if EXA_API_KEY="$(security find-generic-password -a "$USER" -s "second-brain/EXA_API_KEY" -w 2>/dev/null)"; then
    export EXA_API_KEY
  else
    unset EXA_API_KEY
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
export OPENAI_SYNTHESIS_MODEL="${OPENAI_SYNTHESIS_MODEL:-gpt-5.5}"
export OPENAI_CHAT_MODEL="${OPENAI_CHAT_MODEL:-gpt-5.5}"
export X_TOKEN_REFRESH_DIRECT="${X_TOKEN_REFRESH_DIRECT:-true}"
export X_REAUTHORIZE_COMMAND="${X_REAUTHORIZE_COMMAND:-npm run x:oauth:prod}"
export X_KEYCHAIN_TOKEN_SUFFIX="${X_KEYCHAIN_TOKEN_SUFFIX:-_PROD}"
export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"

cd "$BACKEND_DIR"
exec "$ONECLI" run --project "$PROJECT" -- go run ./cmd/api
