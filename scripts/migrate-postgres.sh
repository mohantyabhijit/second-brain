#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

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
    DATABASE_URL="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_DB_URL" -w)"
    export DATABASE_URL
  fi
fi
if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  export SUPABASE_DB_URL="$DATABASE_URL"
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

cd "$BACKEND_DIR"
go run ./cmd/migrate ../supabase/migrations
