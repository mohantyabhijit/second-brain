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

if [[ -z "${SUPABASE_DB_URL:-}" ]]; then
  SUPABASE_DB_URL="$(security find-generic-password -a "$USER" -s "second-brain/SUPABASE_DB_URL" -w)"
  export SUPABASE_DB_URL
fi

cd "$BACKEND_DIR"
go run ./cmd/migrate ../supabase/migrations
