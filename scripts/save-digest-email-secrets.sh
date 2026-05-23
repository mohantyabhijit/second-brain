#!/usr/bin/env bash
set -euo pipefail

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "$name is required." >&2
    exit 1
  fi
}

save_secret() {
  local name="$1"
  local service="second-brain/$name"
  security add-generic-password -U -a "$USER" -s "$service" -w "${!name}" >/dev/null
  echo "saved $service"
}

require_env DIGEST_EMAIL_TO
require_env DIGEST_EMAIL_FROM

save_secret DIGEST_EMAIL_TO
save_secret DIGEST_EMAIL_FROM

echo "Digest email settings saved. Store RESEND_API_KEY in OneCLI, then run: npm run digest:run"
