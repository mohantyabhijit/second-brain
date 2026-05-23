#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if X_CLIENT_ID_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID_PROD" -w 2>/dev/null)"; then
  export X_CLIENT_ID_PROD
  export X_CLIENT_ID="$X_CLIENT_ID_PROD"
else
  echo "second-brain/X_CLIENT_ID_PROD is missing from Keychain" >&2
  exit 1
fi

if X_CLIENT_SECRET_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET_PROD" -w 2>/dev/null)"; then
  export X_CLIENT_SECRET_PROD
  export X_CLIENT_SECRET="$X_CLIENT_SECRET_PROD"
else
  echo "second-brain/X_CLIENT_SECRET_PROD is missing from Keychain" >&2
  exit 1
fi

export X_EXPECTED_USERNAME="${X_EXPECTED_USERNAME:-mohantyabhijit}"
export X_OAUTH_SCOPES="${X_OAUTH_SCOPES:-tweet.read users.read bookmark.read offline.access}"
export X_REAUTHORIZE_COMMAND="${X_REAUTHORIZE_COMMAND:-npm run x:oauth:prod}"
export X_KEYCHAIN_TOKEN_SUFFIX="${X_KEYCHAIN_TOKEN_SUFFIX:-_PROD}"
export X_OAUTH_TOKEN_SUFFIX="${X_OAUTH_TOKEN_SUFFIX:-_PROD}"
export X_OAUTH_UPDATE_ONECLI="${X_OAUTH_UPDATE_ONECLI:-true}"
export X_TOKEN_ROTATION_PATH="${X_TOKEN_ROTATION_PATH:-$ROOT_DIR/data/runtime/x-token-rotation.json}"

if [[ "${X_OAUTH_LEGACY_HELPER:-false}" == "true" ]]; then
  export X_REDIRECT_URI="${X_REDIRECT_URI:-http://127.0.0.1:8765/callback}"
  exec node "$ROOT_DIR/scripts/x-oauth-local.mjs"
fi

if [[ -z "${SECOND_BRAIN_API_BASE_URL:-}" ]]; then
  cat >&2 <<'EOF'
Set SECOND_BRAIN_API_BASE_URL to the production backend origin, then rerun.
Example: SECOND_BRAIN_API_BASE_URL=https://second-brain.example.com npm run x:oauth:prod

For a local localhost callback against the production X app:
X_OAUTH_LEGACY_HELPER=true npm run x:oauth:prod
EOF
  exit 2
fi

exec "$ROOT_DIR/scripts/run-x-oauth-local.sh"
