#!/usr/bin/env bash
set -euo pipefail

REPO="${GITHUB_REPOSITORY:-mohantyabhijit/second-brain}"

if [[ -z "${REDIS_URL:-}" ]]; then
  echo "REDIS_URL is required. Export the Redis connection string, then rerun this script." >&2
  exit 1
fi

security add-generic-password -U -a "$USER" -s "second-brain/REDIS_URL" -w "$REDIS_URL" >/dev/null
echo "saved keychain: second-brain/REDIS_URL"

if command -v gh >/dev/null 2>&1 && gh auth status -h github.com >/dev/null 2>&1; then
  printf '%s' "$REDIS_URL" | gh secret set REDIS_URL --repo "$REPO" --body-file -
  gh secret set REDIS_CACHE_ENABLED --repo "$REPO" --body "true"
  echo "saved github secrets: REDIS_URL, REDIS_CACHE_ENABLED"
else
  echo "gh is not authenticated; skipped GitHub secret update." >&2
fi
