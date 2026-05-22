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

export ONECLI_GATEWAY=true
export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"

cd "$BACKEND_DIR"
exec "$ONECLI" run --project "$PROJECT" -- go run ./cmd/api
