#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
path="${X_TOKEN_ROTATION_PATH:-$ROOT_DIR/data/runtime/x-token-rotation.json}"

if [[ ! -f "$path" ]]; then
  echo "missing X token rotation metadata: $path"
  echo "Run npm run x:check or npm run refresh:run after backend X OAuth is configured."
  exit 1
fi

node -e '
  const fs = require("fs");
  const path = process.argv[1];
  const metadata = JSON.parse(fs.readFileSync(path, "utf8"));
  const expiresAt = metadata.accessTokenExpiresAt ? new Date(metadata.accessTokenExpiresAt) : null;
  const now = new Date();
  const minutesLeft = expiresAt ? Math.floor((expiresAt - now) / 60000) : null;
  console.log(`rotation_file=${path}`);
  console.log(`rotated_at=${metadata.rotatedAt || "unknown"}`);
  console.log(`access_token_expires_at=${metadata.accessTokenExpiresAt || "unknown"}`);
  if (minutesLeft !== null) console.log(`access_token_minutes_left=${minutesLeft}`);
  console.log(`expires_in_seconds=${metadata.expiresInSeconds || "unknown"}`);
  console.log(`scope=${metadata.scope || "unknown"}`);
  console.log(`keychain_token_suffix=${metadata.keychainTokenSuffix || "(none)"}`);
  console.log(`expected_username=${metadata.expectedUsername || "unknown"}`);
  console.log(`reauthorize_command=${metadata.reauthorizeCommand || "unknown"}`);
' "$path"
