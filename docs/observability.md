# Logging and Observability

This app emits structured JSON logs with zerolog and ships local container logs to Loki through Fluent Bit by default. Grafana is provisioned with a Loki datasource and a starter dashboard.

Promtail config is also included under an optional Compose profile, but Promtail is not the default because Grafana's current Promtail docs state that it reached EOL on March 2, 2026.

## Application Logging

The logging layer lives in `backend/internal/platform/logging`.

Runtime behavior:

- Logs are JSON and are written to stdout.
- Field names are normalized for Loki and Grafana: `timestamp`, `service_name`, `environment`, `log_level`, `message`, and `error`.
- HTTP requests receive `request_id`; the value is reused from `X-Request-ID`, `X-Request-Id`, or `X-Correlation-ID`, or generated when absent.
- Incoming W3C `traceparent` headers are extracted and logged as `trace_id` when available.
- Authenticated request scope sets `user_id` to the resolved owner/workspace id.
- HTTP access logs include `method`, `route`, `path`, `status`, `latency`, `latency_ms`, `bytes`, `user_agent`, and `remote_addr`.
- Sensitive structured fields whose keys include `password`, `token`, `secret`, `authorization`, `cookie`, `api_key`, `apikey`, or `credential` are redacted.
- Error logs include a `stack` field when `LOG_ERROR_STACK=true`.
- Debug logs can be sampled in production with `LOG_DEBUG_SAMPLE_RATE`; `1` means no sampling.

Environment knobs:

```env
LOG_LEVEL=info
LOG_ERROR_STACK=true
LOG_DEBUG_SAMPLE_RATE=1
```

## Local Stack

Verify the checked-in observability bundle before starting containers:

```bash
npm run observability:verify
```

Start the API plus Fluent Bit, Loki, and Grafana:

```bash
docker compose up --build
```

Run the end-to-end smoke test:

```bash
npm run observability:smoke
```

The smoke test builds and starts the stack, sends a request with `X-Request-ID` and `traceparent`, queries Loki for the correlated JSON log, and checks Grafana's Loki datasource. It tears the stack down by default; set `KEEP_OBSERVABILITY_STACK=true` to leave it running.

Endpoints:

- API: `http://localhost:8080`
- Loki: `http://localhost:3100`
- Grafana: `http://localhost:3001`

Grafana defaults to `admin` / `admin` unless `GRAFANA_ADMIN_USER` and `GRAFANA_ADMIN_PASSWORD` are set.

## Docker Compose

The root `docker-compose.yml` includes:

- `app`: Go API built from `backend/Dockerfile`; JSON logs go to stdout through Docker's `json-file` driver.
- `fluent-bit`: Tails Docker stdout JSON logs, parses zerolog JSON, keeps records with `service_name`, and ships to Loki.
- `promtail`: Optional legacy profile for teams that still need Promtail-compatible config.
- `loki`: Filesystem-backed local Loki with configurable retention.
- `grafana`: Provisioned Loki datasource and `Second Brain Logs` dashboard.

Low-cardinality Loki labels:

- `app`
- `service`
- `environment`
- `container`
- `job`
- `log_level`

High-cardinality fields such as `request_id`, `trace_id`, `user_id`, `path`, and `error` stay in the JSON log body and are not labels.

## Fluent Bit

Fluent Bit config lives in `observability/fluent-bit`.

It reads Docker stdout logs from `/var/lib/docker/containers/*/*-json.log`, parses the Docker log envelope, then parses the zerolog payload from the `log` field.

It keeps only structured app logs where `service_name` exists. Low-cardinality labels are sent to Loki with `service`, `environment`, `app`, `container`, `job`, and `log_level`; high-cardinality fields remain in the JSON body.

## Promtail

Promtail config lives in `observability/promtail/promtail.yml`.

Promtail is optional because it is EOL as of March 2, 2026. To run it for local compatibility testing:

```bash
docker compose --profile promtail up --build
```

It reads Docker stdout logs via:

- `/var/run/docker.sock`
- `/var/lib/docker/containers`

It parses the Docker log envelope, then parses the zerolog JSON payload. Parsed fields remain available to LogQL through `| json`; only `log_level` is promoted from the payload to a Loki label.

## Loki

Loki config lives in `observability/loki/loki.yml`.

Local storage is filesystem-backed under the `loki_data` Docker volume. Retention is configurable:

```bash
LOKI_RETENTION_PERIOD=168h docker compose up --build
```

## Grafana

Provisioning files:

- Datasource: `observability/grafana/provisioning/datasources/loki.yml`
- Dashboard provider: `observability/grafana/provisioning/dashboards/dashboards.yml`
- Dashboard JSON: `observability/grafana/dashboards/second-brain-logs.json`

The default dashboard includes:

- Recent structured logs
- Error rate
- HTTP p95 latency
- Error log stream
- Slow requests over 500 ms

## Example LogQL

Recent API errors:

```logql
{service="api"} |= "error"
```

Slow API requests:

```logql
{service="api"} | json | latency > 500
```

Request correlation:

```logql
{service="api"} | json | request_id="request-1"
```

Trace correlation:

```logql
{service="api"} | json | trace_id="4bf92f3577b34da6a3ce929d0e0e4736"
```

## Sample JSON Log

```json
{
  "log_level": "info",
  "service_name": "api",
  "environment": "development",
  "request_id": "request-obs-workspace",
  "user_id": "00000000-0000-0000-0000-000000000001",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "method": "GET",
  "route": "GET /api/workspace",
  "path": "/api/workspace",
  "status": 200,
  "latency_ms": 0,
  "latency": 0,
  "bytes": 335,
  "user_agent": "curl/8.7.1",
  "remote_addr": "127.0.0.1:64539",
  "timestamp": "2026-06-05T23:23:35.24985+08:00",
  "message": "http request completed"
}
```
