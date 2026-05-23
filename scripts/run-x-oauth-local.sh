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

if [[ -z "${X_CLIENT_ID:-}" ]]; then
  if X_CLIENT_ID="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID" -w 2>/dev/null)"; then
    export X_CLIENT_ID
  else
    unset X_CLIENT_ID
  fi
fi

if [[ -z "${X_CLIENT_SECRET:-}" ]]; then
  if X_CLIENT_SECRET="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET" -w 2>/dev/null)"; then
    export X_CLIENT_SECRET
  else
    unset X_CLIENT_SECRET
  fi
fi

exec node "$ROOT_DIR/scripts/x-oauth-local.mjs"
