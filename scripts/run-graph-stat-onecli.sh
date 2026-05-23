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

for key in NEO4J_URI NEO4J_USERNAME NEO4J_PASSWORD NEO4J_DATABASE; do
  if [[ -z "${!key:-}" ]]; then
    if value="$(security find-generic-password -a "$USER" -s "second-brain/$key" -w 2>/dev/null)"; then
      export "$key=$value"
    fi
  fi
done

cd "$BACKEND_DIR"
exec "$ONECLI" run --project "$PROJECT" -- go run ./cmd/graph-stat
