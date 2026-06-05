import { readFileSync } from "node:fs";

const root = new URL("../", import.meta.url);

function read(path) {
  return readFileSync(new URL(path, root), "utf8");
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

function assertIncludes(content, needle, label) {
  assert(content.includes(needle), `${label} is missing ${JSON.stringify(needle)}`);
}

function parseJSON(path) {
  try {
    return JSON.parse(read(path));
  } catch (error) {
    throw new Error(`${path} is not valid JSON: ${error.message}`);
  }
}

const compose = read("docker-compose.yml");
for (const service of ["app:", "loki:", "fluent-bit:", "promtail:", "grafana:"]) {
  assertIncludes(compose, `  ${service}`, "docker-compose.yml");
}
assertIncludes(compose, "grafana/loki:3.7.2", "docker-compose.yml");
assertIncludes(compose, "fluent/fluent-bit:4.1.0", "docker-compose.yml");
assertIncludes(compose, "grafana/promtail:3.6.0", "docker-compose.yml");
assertIncludes(compose, "grafana/grafana:13.0.1", "docker-compose.yml");
assertIncludes(compose, "observability.logs: \"true\"", "docker-compose.yml");
assertIncludes(compose, "/var/lib/docker/containers:/var/lib/docker/containers:ro", "docker-compose.yml");
assertIncludes(compose, "LOKI_RETENTION_PERIOD", "docker-compose.yml");

const fluentBit = read("observability/fluent-bit/fluent-bit.conf");
assertIncludes(fluentBit, "Path              /var/lib/docker/containers/*/*-json.log", "fluent-bit.conf");
assertIncludes(fluentBit, "Parser            docker", "fluent-bit.conf");
assertIncludes(fluentBit, "Parser        zerolog_json", "fluent-bit.conf");
assertIncludes(fluentBit, "Regex  service_name ^.+$", "fluent-bit.conf");
assertIncludes(fluentBit, "Hard_copy  service_name  loki_service", "fluent-bit.conf");
assertIncludes(fluentBit, "Hard_copy  environment   loki_environment", "fluent-bit.conf");
assertIncludes(fluentBit, "Hard_copy  log_level     loki_log_level", "fluent-bit.conf");
assertIncludes(fluentBit, "Labels          job=second-brain,app=second-brain,container=app,service=$loki_service,environment=$loki_environment", "fluent-bit.conf");
assertIncludes(fluentBit, "Remove_Keys     stream,time,loki_service,loki_environment,loki_log_level", "fluent-bit.conf");
for (const highCardinality of ["request_id", "trace_id", "user_id", "path", "error"]) {
  assert(!fluentBit.includes(`=${highCardinality}`), `fluent-bit.conf promotes high-cardinality ${highCardinality} as a label`);
}

const labelMap = parseJSON("observability/fluent-bit/loki-labels.json");
assert(labelMap.loki_log_level === "log_level", "Fluent Bit label map must promote log_level from the temporary label alias only");
for (const forbidden of ["request_id", "trace_id", "user_id", "path", "error"]) {
  assert(!(forbidden in labelMap), `Fluent Bit label map must not promote ${forbidden}`);
}

const promtail = read("observability/promtail/promtail.yml");
assertIncludes(promtail, "docker_sd_configs:", "promtail.yml");
assertIncludes(promtail, "observability_logs", "promtail.yml");
assertIncludes(promtail, "replacement: /var/lib/docker/containers/$1/$1-json.log", "promtail.yml");
assertIncludes(promtail, "log_level:", "promtail.yml");
for (const forbidden of ["request_id:", "trace_id:", "user_id:", "path:", "error:"]) {
  const labelBlock = promtail.split("- labels:")[1] || "";
  assert(!labelBlock.includes(forbidden), `promtail.yml labels block must not promote ${forbidden.replace(":", "")}`);
}

const loki = read("observability/loki/loki.yml");
assertIncludes(loki, "auth_enabled: false", "loki.yml");
assertIncludes(loki, "object_store: filesystem", "loki.yml");
assertIncludes(loki, "schema: v13", "loki.yml");
assertIncludes(loki, "retention_enabled: true", "loki.yml");
assertIncludes(loki, "retention_period: ${LOKI_RETENTION_PERIOD}", "loki.yml");

const datasource = read("observability/grafana/provisioning/datasources/loki.yml");
assertIncludes(datasource, "uid: Loki", "Grafana datasource");
assertIncludes(datasource, "url: http://loki:3100", "Grafana datasource");

const dashboard = parseJSON("observability/grafana/dashboards/second-brain-logs.json");
assert(dashboard.title === "Second Brain Logs", "Grafana dashboard title mismatch");
const dashboardQueries = JSON.stringify(dashboard);
for (const query of [
  "{service=~\\\"$service\\\", environment=~\\\"$environment\\\"} | json",
  "log_level=\\\"error\\\"",
  "| json | latency > 500",
  "quantile_over_time(0.95",
]) {
  assertIncludes(dashboardQueries, query, "Grafana dashboard");
}

const docs = read("docs/observability.md");
for (const query of [
  "{service=\"api\"} |= \"error\"",
  "{service=\"api\"} | json | latency > 500",
  "{service=\"api\"} | json | request_id=\"request-1\"",
  "{service=\"api\"} | json | trace_id=\"4bf92f3577b34da6a3ce929d0e0e4736\"",
]) {
  assertIncludes(docs, query, "observability docs");
}
const sampleMatch = docs.match(/## Sample JSON Log[\s\S]*?```json\n([\s\S]*?)\n```/);
assert(sampleMatch, "observability docs must include a sample JSON log");
const sample = JSON.parse(sampleMatch[1]);
for (const field of ["timestamp", "service_name", "environment", "request_id", "trace_id", "user_id", "log_level", "message", "latency"]) {
  assert(field in sample, `sample JSON log is missing ${field}`);
}

const loggingCode = read("backend/internal/platform/logging/logging.go");
for (const field of ["timestamp", "log_level", "message", "error", "service_name", "environment"]) {
  assertIncludes(loggingCode, field, "logging.go");
}
assertIncludes(loggingCode, "shouldRedact", "logging.go");
assertIncludes(loggingCode, "debugSampleRate", "logging.go");

const routerCode = read("backend/internal/httpapi/router.go");
for (const field of ["X-Request-ID", "propagation.TraceContext{}", "logging.SetUserID", "latency_ms", "latency", "route"]) {
  assertIncludes(routerCode, field, "router.go");
}

console.log("observability verification passed");
