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

should_create_langfuse_secret() {
  if [[ -z "$ONLY_SECRETS" ]]; then
    return 0
  fi
  [[ ",$ONLY_SECRETS," == *",LANGFUSE_AUTH_TOKEN,"* ]] ||
    [[ ",$ONLY_SECRETS," == *",LANGFUSE_PUBLIC_KEY,"* ]] ||
    [[ ",$ONLY_SECRETS," == *",LANGFUSE_SECRET_KEY,"* ]]
}

create_langfuse_secret() {
  if ! should_create_langfuse_secret; then
    return 0
  fi

  local value="${LANGFUSE_AUTH_TOKEN:-}"
  if [[ -z "$value" && -n "${LANGFUSE_PUBLIC_KEY:-}" && -n "${LANGFUSE_SECRET_KEY:-}" ]]; then
    value="$(printf '%s:%s' "$LANGFUSE_PUBLIC_KEY" "$LANGFUSE_SECRET_KEY" | base64 | tr -d '\n')"
  fi
  if [[ -z "$value" ]]; then
    echo "skip LANGFUSE_AUTH_TOKEN: set LANGFUSE_AUTH_TOKEN or both LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY"
    return 0
  fi

  local base_url="${LANGFUSE_BASE_URL:-https://jp.cloud.langfuse.com}"
  local host_pattern="${LANGFUSE_HOST_PATTERN:-${base_url#http://}}"
  host_pattern="${host_pattern#https://}"
  host_pattern="${host_pattern%%/*}"
  local path_pattern="${LANGFUSE_OTEL_PATH_PATTERN:-/api/public/otel/v1/traces}"

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "Second Brain Langfuse OTLP credentials" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --header-name "Authorization" \
    --value-format "Basic {value}" \
    --path-pattern "$path_pattern"

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "Second Brain Langfuse Prompt API credentials" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --header-name "Authorization" \
    --value-format "Basic {value}" \
    --path-pattern "${LANGFUSE_PROMPT_API_PATH_PATTERN:-/api/public/v2/prompts*}"

  "$ONECLI" secrets create \
    --project "$PROJECT" \
    --name "Second Brain Langfuse Score API credentials" \
    --type generic \
    --value "$value" \
    --host-pattern "$host_pattern" \
    --header-name "Authorization" \
    --value-format "Basic {value}" \
    --path-pattern "${LANGFUSE_SCORE_API_PATH_PATTERN:-/api/public/scores*}"
}

create_secret X_USER_ACCESS_TOKEN "Second Brain X user access token" "api.x.com" "Authorization" "Bearer {value}" "/2/users/*"
create_param_secret X_REFRESH_TOKEN "Second Brain X refresh token" "api.x.com" "refresh_token" "/2/oauth2/token"
create_secret YOUTUBE_ACCESS_TOKEN "Second Brain YouTube OAuth access token" "www.googleapis.com"
create_param_secret YOUTUBE_API_KEY "Second Brain YouTube API key" "www.googleapis.com" "key"
create_secret SUPADATA_API_KEY "Second Brain Supadata API key" "api.supadata.ai" "x-api-key" "{value}"
create_secret OPENAI_API_KEY "Second Brain OpenAI synthesis key" "api.openai.com"
create_secret EXA_API_KEY "Second Brain Exa search key" "api.exa.ai" "x-api-key" "{value}"
create_secret RESEND_API_KEY "Second Brain Resend API key" "api.resend.com"
create_langfuse_secret

echo "Done. Run: $ONECLI secrets list --project $PROJECT"
