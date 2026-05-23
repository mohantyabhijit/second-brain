#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
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

if X_USER_ACCESS_TOKEN_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_USER_ACCESS_TOKEN_PROD" -w 2>/dev/null)"; then
  export X_USER_ACCESS_TOKEN_PROD
  export X_USER_ACCESS_TOKEN="$X_USER_ACCESS_TOKEN_PROD"
else
  unset X_USER_ACCESS_TOKEN
fi

if X_REFRESH_TOKEN_PROD="$(security find-generic-password -a "$USER" -s "second-brain/X_REFRESH_TOKEN_PROD" -w 2>/dev/null)"; then
  export X_REFRESH_TOKEN_PROD
  export X_REFRESH_TOKEN="$X_REFRESH_TOKEN_PROD"
else
  echo "second-brain/X_REFRESH_TOKEN_PROD is missing from Keychain; run npm run x:oauth:prod" >&2
  exit 1
fi

export ONECLI_GATEWAY=true
export X_TOKEN_REFRESH_DIRECT=true
export X_REAUTHORIZE_COMMAND="npm run x:oauth:prod"
export X_KEYCHAIN_TOKEN_SUFFIX="_PROD"
export X_TOKEN_ROTATION_PATH="${X_TOKEN_ROTATION_PATH:-$ROOT_DIR/data/runtime/x-token-rotation.json}"
export KNOWLEDGE_RUN_PATH="${KNOWLEDGE_RUN_PATH:-$ROOT_DIR/data/runtime/latest-knowledge-run.json}"

cd "$BACKEND_DIR"
exec go run ./cmd/x-auth-check
