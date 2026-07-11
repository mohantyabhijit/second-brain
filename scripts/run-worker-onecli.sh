#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/onecli-runtime.sh
. "$ROOT_DIR/scripts/lib/onecli-runtime.sh"
onecli_runtime_init worker
onecli_run_backend worker
