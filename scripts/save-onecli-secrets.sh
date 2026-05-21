#!/usr/bin/env bash
set -euo pipefail

ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
PROJECT="${ONECLI_PROJECT:-second-brain}"

if ! "$ONECLI" auth status >/dev/null 2>&1; then
  echo "OneCLI is installed but not authenticated."
  echo "Run: $ONECLI auth login --api-key oc_..."
  exit 1
fi

create_secret() {
  local env_name="$1"
  local display_name="$2"
  local host_pattern="$3"
  local header_name="${4:-Authorization}"
  local value_format="${5:-Bearer {value}}"
  local value="${!env_name:-}"

  if [[ -z "$value" ]]; then
    echo "skip $env_name: environment variable is not set"
    return 0
  fi

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "$display_name" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --header-name "$header_name" \
    --value-format "$value_format"
}

create_param_secret() {
  local env_name="$1"
  local display_name="$2"
  local host_pattern="$3"
  local param_name="$4"
  local value="${!env_name:-}"

  if [[ -z "$value" ]]; then
    echo "skip $env_name: environment variable is not set"
    return 0
  fi

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "$display_name" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --param-name "$param_name" \
    --param-format "{value}"
}

create_secret X_USER_ACCESS_TOKEN "Second Brain X user access token" "api.x.com"
create_secret YOUTUBE_ACCESS_TOKEN "Second Brain YouTube OAuth access token" "www.googleapis.com"
create_param_secret YOUTUBE_API_KEY "Second Brain YouTube API key" "www.googleapis.com" "key"
create_secret SUPADATA_API_KEY "Second Brain Supadata API key" "api.supadata.ai" "x-api-key" "{value}"
create_secret OPENAI_API_KEY "Second Brain OpenAI translation key" "api.openai.com"

echo "Done. Run: $ONECLI secrets list --project $PROJECT"
