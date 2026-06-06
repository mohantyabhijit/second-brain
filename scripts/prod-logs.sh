#!/usr/bin/env bash
set -euo pipefail

host="${SECOND_BRAIN_PROD_SSH_HOST:-do}"
since="1 hour ago"
follow=false
json=false
unit_scope="all"
level=""
request_id=""

usage() {
  cat <<'USAGE'
Usage: scripts/prod-logs.sh [options]

Read structured Second Brain production logs from ubuntu-sgp over SSH.

Options:
  --api                 Show only second-brain-api logs.
  --worker              Show only second-brain-worker logs.
  --since VALUE         Journal start time, e.g. "15 minutes ago" or "today".
  -f, --follow          Follow logs live.
  --level LEVEL         Filter by log_level, e.g. info, warn, error.
  --errors              Shortcut for --level error.
  --request-id ID       Filter to a single request_id.
  --json                Print compact JSON logs instead of human lines.
  --host HOST           SSH host alias. Default: do.
  -h, --help            Show this help.

Examples:
  npm run logs:prod
  npm run logs:prod -- --api -f
  npm run logs:prod -- --errors --since today
  npm run logs:prod -- --request-id req_123 --json
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --api)
      unit_scope="api"
      shift
      ;;
    --worker)
      unit_scope="worker"
      shift
      ;;
    --since)
      since="${2:?--since requires a value}"
      shift 2
      ;;
    -f|--follow)
      follow=true
      shift
      ;;
    --level)
      level="${2:?--level requires a value}"
      shift 2
      ;;
    --errors)
      level="error"
      shift
      ;;
    --request-id)
      request_id="${2:?--request-id requires a value}"
      shift 2
      ;;
    --json)
      json=true
      shift
      ;;
    --host)
      host="${2:?--host requires a value}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

units=()
case "$unit_scope" in
  api)
    units=(second-brain-api)
    ;;
  worker)
    units=(second-brain-worker)
    ;;
  all)
    units=(second-brain-api second-brain-worker)
    ;;
esac

remote_cmd=(journalctl)
for unit in "${units[@]}"; do
  remote_cmd+=(-u "$unit")
done
remote_cmd+=(--since "$since")
if [[ "$follow" == true ]]; then
  remote_cmd+=(-f)
fi
remote_cmd+=(--no-pager -o cat)

quoted_remote_cmd=""
printf -v quoted_remote_cmd "%q " "${remote_cmd[@]}"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for structured log filtering. Install jq or run raw journalctl over SSH." >&2
  exit 127
fi

export LOG_LEVEL_FILTER="$level"
export REQUEST_ID_FILTER="$request_id"

if [[ "$json" == true ]]; then
  ssh "$host" "bash -lc '$quoted_remote_cmd'" | jq --unbuffered -Rrc '
    fromjson?
    | select(.service_name != null)
    | select((env.LOG_LEVEL_FILTER == "") or (.log_level == env.LOG_LEVEL_FILTER))
    | select((env.REQUEST_ID_FILTER == "") or (.request_id == env.REQUEST_ID_FILTER))
  '
else
  ssh "$host" "bash -lc '$quoted_remote_cmd'" | jq --unbuffered -Rr '
    fromjson?
    | select(.service_name != null)
    | select((env.LOG_LEVEL_FILTER == "") or (.log_level == env.LOG_LEVEL_FILTER))
    | select((env.REQUEST_ID_FILTER == "") or (.request_id == env.REQUEST_ID_FILTER))
    | [
        (.timestamp // "-"),
        (.log_level // "-"),
        (.service_name // "-"),
        (.message // "-"),
        (if .status then "status=\(.status)" else empty end),
        (if .route then "route=\(.route)" else empty end),
        (if .latency_ms then "latency_ms=\(.latency_ms)" else empty end),
        (if .request_id then "request_id=\(.request_id)" else empty end),
        (if .trace_id then "trace_id=\(.trace_id)" else empty end),
        (if .error then "error=\(.error)" else empty end)
      ]
    | join(" ")
  '
fi
