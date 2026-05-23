#!/usr/bin/env bash
set -euo pipefail

ONECLI="${ONECLI:-/Users/abhijitmohanty/.local/bin/onecli}"
PROJECT="${ONECLI_PROJECT:-second-brain}"
ONLY_SECRETS="${ONECLI_ONLY_SECRETS:-}"

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
  local path_pattern="${6:-}"
  local value="${!env_name:-}"

  if ! should_create_secret "$env_name"; then
    return 0
  fi

  if [[ -z "$value" ]]; then
    echo "skip $env_name: environment variable is not set"
    return 0
  fi

  local args=(
    secrets create
    --project "$PROJECT"
    --name "$display_name"
    --type generic
    --value "$value"
    --host-pattern "$host_pattern"
    --header-name "$header_name"
    --value-format "$value_format"
  )
  if [[ -n "$path_pattern" ]]; then
    args+=(--path-pattern "$path_pattern")
  fi

  "$ONECLI" "${args[@]}"
}

create_param_secret() {
  local env_name="$1"
  local display_name="$2"
  local host_pattern="$3"
  local param_name="$4"
  local path_pattern="${5:-}"
  local value="${!env_name:-}"

  if ! should_create_secret "$env_name"; then
    return 0
  fi

  if [[ -z "$value" ]]; then
    echo "skip $env_name: environment variable is not set"
    return 0
  fi

  local args=(
    secrets create
    --project "$PROJECT"
    --name "$display_name"
    --type generic
    --value "$value"
    --host-pattern "$host_pattern"
    --param-name "$param_name"
    --param-format "{value}"
  )
  if [[ -n "$path_pattern" ]]; then
    args+=(--path-pattern "$path_pattern")
  fi

  "$ONECLI" "${args[@]}"
}

should_create_secret() {
  local env_name="$1"
  if [[ -z "$ONLY_SECRETS" ]]; then
    return 0
  fi
  [[ ",$ONLY_SECRETS," == *",$env_name,"* ]]
}

create_secret X_USER_ACCESS_TOKEN "Second Brain X user access token" "api.x.com" "Authorization" "Bearer {value}" "/2/users/*"
create_param_secret X_REFRESH_TOKEN "Second Brain X refresh token" "api.x.com" "refresh_token" "/2/oauth2/token"
create_secret YOUTUBE_ACCESS_TOKEN "Second Brain YouTube OAuth access token" "www.googleapis.com"
create_param_secret YOUTUBE_API_KEY "Second Brain YouTube API key" "www.googleapis.com" "key"
create_secret SUPADATA_API_KEY "Second Brain Supadata API key" "api.supadata.ai" "x-api-key" "{value}"
create_secret OPENAI_API_KEY "Second Brain OpenAI synthesis key" "api.openai.com"
create_secret EXA_API_KEY "Second Brain Exa search key" "api.exa.ai" "x-api-key" "{value}"
create_secret SUPABASE_SERVICE_ROLE_KEY "Second Brain Supabase service role key" "*.supabase.co" "Authorization" "Bearer {value}"
create_secret SUPABASE_SERVICE_ROLE_KEY "Second Brain Supabase service role apikey" "*.supabase.co" "apikey" "{value}"
create_secret RESEND_API_KEY "Second Brain Resend API key" "api.resend.com"

echo "Done. Run: $ONECLI secrets list --project $PROJECT"
