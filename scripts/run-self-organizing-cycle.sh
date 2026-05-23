#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

npm run refresh:run

set +e
npm run graph:sync
graph_status=$?
set -e
if [[ "$graph_status" -ne 0 ]]; then
  echo "graph sync skipped or failed; continuing to digest delivery"
fi

npm run digest:run
