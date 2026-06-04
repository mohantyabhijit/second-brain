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
    echo "DATABASE_URL is required" >&2
    exit 1
  fi
fi

cd "$BACKEND_DIR"
go run ./cmd/migrate ../supabase/migrations
