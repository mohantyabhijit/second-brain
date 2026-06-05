#!/usr/bin/env bash
set -euo pipefail

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for the observability smoke test" >&2
  exit 127
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "docker compose is required for the observability smoke test" >&2
  exit 127
fi

cleanup() {
  if [[ "${KEEP_OBSERVABILITY_STACK:-false}" != "true" ]]; then
    docker compose down --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

docker compose up --build -d app loki fluent-bit grafana

wait_for_url() {
  local url="$1"
  local label="$2"
  local attempts="${3:-60}"
  for _ in $(seq 1 "$attempts"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "timed out waiting for ${label} at ${url}" >&2
  return 1
}

wait_for_url "http://127.0.0.1:8080/healthz" "api"
wait_for_url "http://127.0.0.1:3100/ready" "loki"
wait_for_url "http://127.0.0.1:3001/api/health" "grafana"

request_id="obs-smoke-$(date +%s)"
trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
curl -fsS \
  -H "X-Request-ID: ${request_id}" \
  -H "traceparent: 00-${trace_id}-00f067aa0ba902b7-01" \
  "http://127.0.0.1:8080/api/workspace" >/dev/null

loki_query="{service=\"api\"} | json | request_id=\"${request_id}\""
start_ns="$((($(date +%s) - 300) * 1000000000))"

found_log="false"
for _ in $(seq 1 60); do
  response="$(
    curl -fsS --get "http://127.0.0.1:3100/loki/api/v1/query_range" \
      --data-urlencode "query=${loki_query}" \
      --data-urlencode "start=${start_ns}" \
      --data-urlencode "limit=20"
  )"
  found_log="$(
    RESPONSE="${response}" REQUEST_ID="${request_id}" TRACE_ID="${trace_id}" node <<'NODE'
const payload = JSON.parse(process.env.RESPONSE);
const requestID = process.env.REQUEST_ID;
const traceID = process.env.TRACE_ID;
const streams = payload?.data?.result ?? [];
let found = false;
for (const stream of streams) {
  for (const [, line] of stream.values ?? []) {
    const event = JSON.parse(line);
    if (
      event.request_id === requestID &&
      event.trace_id === traceID &&
      event.service_name === "api" &&
      event.environment &&
      event.log_level === "info" &&
      event.message === "http request completed" &&
      typeof event.latency === "number" &&
      event.user_id
    ) {
      found = true;
    }
  }
}
process.stdout.write(found ? "true" : "false");
NODE
  )"
  if [[ "${found_log}" == "true" ]]; then
    break
  fi
  sleep 2
done

if [[ "${found_log}" != "true" ]]; then
  echo "did not find correlated API request log in Loki for request_id=${request_id}" >&2
  exit 1
fi

grafana_user="${GRAFANA_ADMIN_USER:-admin}"
grafana_password="${GRAFANA_ADMIN_PASSWORD:-admin}"
curl -fsS -u "${grafana_user}:${grafana_password}" "http://127.0.0.1:3001/api/datasources/name/Loki" >/dev/null

echo "observability smoke passed"
echo "Grafana: http://127.0.0.1:3001"
echo "Loki query: ${loki_query}"
