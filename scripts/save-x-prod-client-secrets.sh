#!/usr/bin/env bash
set -euo pipefail

ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
PROJECT="${ONECLI_PROJECT:-second-brain}"

client_id="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_ID_PROD" -w)"
client_secret="$(security find-generic-password -a "$USER" -s "second-brain/X_CLIENT_SECRET_PROD" -w)"
client_basic="$(printf '%s:%s' "$client_id" "$client_secret" | base64 | tr -d '\n')"

upsert_onecli_secret() {
  local name="$1"
  local header_name="$2"
  local value="$3"
  local host_pattern="${4:-second-brain.local}"
  local path_pattern="${5:-}"
  local value_format="${6:-{value}}"
  local id

  id="$("$ONECLI" secrets list --project "$PROJECT" | node -e '
    const fs = require("fs");
    const name = process.argv[1];
    const payload = JSON.parse(fs.readFileSync(0, "utf8"));
    const found = payload.data?.find((item) => item.name === name);
    if (found?.id) process.stdout.write(found.id);
  ' "$name")"

  if [[ -n "$id" ]]; then
    local update_args=(
      secrets update
      --id "$id"
      --value "$value"
      --host-pattern "$host_pattern"
      --header-name "$header_name"
      --value-format "$value_format"
    )
    if [[ -n "$path_pattern" ]]; then
      update_args+=(--path-pattern "$path_pattern")
    fi
    "$ONECLI" "${update_args[@]}" >/dev/null
    echo "updated onecli: $name"
    return
  fi

  local args=(
    secrets create
    --project "$PROJECT"
    --name "$name"
    --type generic
    --value "$value"
    --host-pattern "$host_pattern"
    --header-name "$header_name"
    --value-format "$value_format"
  )
  if [[ -n "$path_pattern" ]]; then
    args+=(--path-pattern "$path_pattern")
  fi
  "$ONECLI" "${args[@]}" >/dev/null
  echo "created onecli: $name"
}

upsert_onecli_param_secret() {
  local name="$1"
  local param_name="$2"
  local value="$3"
  local host_pattern="$4"
  local path_pattern="$5"
  local id

  id="$("$ONECLI" secrets list --project "$PROJECT" | node -e '
    const fs = require("fs");
    const name = process.argv[1];
    const payload = JSON.parse(fs.readFileSync(0, "utf8"));
    const found = payload.data?.find((item) => item.name === name);
    if (found?.id) process.stdout.write(found.id);
  ' "$name")"

  if [[ -n "$id" ]]; then
    "$ONECLI" secrets update \
      --id "$id" \
      --value "$value" \
      --host-pattern "$host_pattern" \
      --path-pattern "$path_pattern" \
      --param-name "$param_name" \
      --param-format "{value}" >/dev/null
    echo "updated onecli: $name"
    return
  fi

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "$name" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --path-pattern "$path_pattern" \
    --param-name "$param_name" \
    --param-format "{value}" >/dev/null
  echo "created onecli: $name"
}

upsert_github_secret() {
  local name="$1"
  local value="$2"
  if command -v gh >/dev/null 2>&1; then
    printf "%s" "$value" | gh secret set "$name" >/dev/null
    echo "updated github actions: $name"
  else
    echo "skip github actions: gh CLI is not installed"
  fi
}

upsert_onecli_secret "Second Brain X prod client id" "X-Second-Brain-X-Client-Id-Prod" "$client_id"
upsert_onecli_secret "Second Brain X prod client secret" "X-Second-Brain-X-Client-Secret-Prod" "$client_secret"
upsert_onecli_param_secret "Second Brain X prod token client id" "client_id" "$client_id" "api.x.com" "/2/oauth2/token"
upsert_onecli_secret "Second Brain X prod token basic auth" "Authorization" "$client_basic" "api.x.com" "/2/oauth2/token" "Basic {value}"
upsert_github_secret X_CLIENT_ID_PROD "$client_id"
upsert_github_secret X_CLIENT_SECRET_PROD "$client_secret"
