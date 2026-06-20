#!/usr/bin/env bash
set -euo pipefail

log() {
  printf '[second-brain-redis-maintenance] %s\n' "$*"
}

bool() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

env_file="${SECOND_BRAIN_ENV_FILE:-/srv/second-brain/api/second-brain.env}"
if [[ -f "$env_file" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$env_file"
  set +a
fi

redis_url="${REDIS_URL:-redis://127.0.0.1:6379/0}"
redis_dir="${SECOND_BRAIN_REDIS_DIR:-/var/lib/redis}"
redis_cli="${REDIS_CLI:-redis-cli}"
systemctl_bin="${SYSTEMCTL:-systemctl}"
onecli_bin="${ONECLI_BIN:-/home/deploy/.local/bin/onecli}"
onecli_project="${ONECLI_PROJECT:-second-brain}"
precompute_path="${SECOND_BRAIN_PRECOMPUTE_PATH:-/srv/second-brain/api/second-brain-precompute}"
redis_service="${SECOND_BRAIN_REDIS_SERVICE:-redis-server}"
stale_temp_minutes="${SECOND_BRAIN_REDIS_STALE_TEMP_MINUTES:-30}"
maxmemory="${SECOND_BRAIN_REDIS_MAXMEMORY:-256mb}"
maxmemory_policy="${SECOND_BRAIN_REDIS_MAXMEMORY_POLICY:-allkeys-lru}"
dump_max_bytes="${SECOND_BRAIN_REDIS_DUMP_MAX_BYTES:-268435456}"
owner_id="${PUBLIC_OWNER_ID:-00000000-0000-0000-0000-000000000001}"
rebuild_on_missing="${SECOND_BRAIN_REDIS_REBUILD_ON_MISSING:-true}"
recover_unhealthy="${SECOND_BRAIN_REDIS_RECOVER_UNHEALTHY:-true}"
delete_large_dump="${SECOND_BRAIN_REDIS_DELETE_LARGE_DUMP:-true}"
dry_run="${SECOND_BRAIN_REDIS_DRY_RUN:-false}"

redis() {
  "$redis_cli" -u "$redis_url" "$@"
}

run() {
  if bool "$dry_run"; then
    printf '[second-brain-redis-maintenance] dry-run:'
    printf ' %q' "$@"
    printf '\n'
    return 0
  fi
  "$@"
}

redis_ping() {
  redis PING 2>&1 || true
}

redis_info() {
  redis INFO server persistence memory stats 2>&1 || true
}

redis_is_healthy() {
  [[ "$(redis_ping)" == "PONG" ]]
}

redis_is_loading() {
  redis_info | grep -q '^loading:1'
}

bgsave_in_progress() {
  redis_info | grep -q '^rdb_bgsave_in_progress:1'
}

file_size() {
  local path="$1"
  if [[ ! -e "$path" ]]; then
    printf '0\n'
    return 0
  fi
  stat -c '%s' "$path" 2>/dev/null || stat -f '%z' "$path"
}

manifest_key() {
  printf 'sb:v1:%s:manifest' "$owner_id"
}

configure_redis_for_cache() {
  log "configuring Redis as bounded rebuildable cache"
  redis CONFIG SET maxmemory "$maxmemory" >/dev/null || log "warning: CONFIG SET maxmemory failed"
  redis CONFIG SET maxmemory-policy "$maxmemory_policy" >/dev/null || log "warning: CONFIG SET maxmemory-policy failed"
  redis CONFIG SET appendonly no >/dev/null || log "warning: CONFIG SET appendonly failed"
  redis CONFIG SET save "" >/dev/null || log "warning: CONFIG SET save failed"
  redis CONFIG REWRITE >/dev/null || log "warning: CONFIG REWRITE failed; timer will reapply runtime Redis cache settings"
}

cleanup_temp_rdb_files() {
  if [[ ! -d "$redis_dir" ]]; then
    log "redis dir missing; skip temp cleanup: $redis_dir"
    return 0
  fi
  if bgsave_in_progress; then
    log "Redis BGSAVE in progress; skip temp RDB cleanup"
    return 0
  fi
  log "removing temp RDB files older than ${stale_temp_minutes}m from $redis_dir"
  if bool "$dry_run"; then
    find "$redis_dir" -maxdepth 1 -type f -name 'temp-*.rdb' -mmin "+$stale_temp_minutes" -print
  else
    find "$redis_dir" -maxdepth 1 -type f -name 'temp-*.rdb' -mmin "+$stale_temp_minutes" -print -delete
  fi
}

cleanup_large_dump() {
  local dump="$redis_dir/dump.rdb"
  if ! bool "$delete_large_dump" || [[ ! -f "$dump" ]]; then
    return 0
  fi
  if bgsave_in_progress; then
    log "Redis BGSAVE in progress; skip dump cleanup"
    return 0
  fi
  local size
  size="$(file_size "$dump")"
  if (( size <= dump_max_bytes )); then
    log "dump.rdb size is ${size}B; below ${dump_max_bytes}B limit"
    return 0
  fi
  log "removing oversized Redis cache dump $dump (${size}B > ${dump_max_bytes}B)"
  run rm -f "$dump"
}

start_redis() {
  log "starting $redis_service"
  run "$systemctl_bin" start "$redis_service"
}

stop_redis() {
  log "stopping $redis_service"
  run "$systemctl_bin" stop "$redis_service"
}

recover_redis_if_needed() {
  if redis_is_healthy; then
    return 0
  fi
  if ! bool "$recover_unhealthy"; then
    log "Redis is unhealthy and recovery is disabled: $(redis_ping)"
    return 0
  fi

  local dump="$redis_dir/dump.rdb"
  local dump_size=0
  dump_size="$(file_size "$dump")"
  local should_clear=false
  if (( dump_size > dump_max_bytes )); then
    should_clear=true
  fi
  if redis_is_loading; then
    should_clear=true
  fi

  if [[ "$should_clear" != "true" ]]; then
    log "Redis unhealthy but no oversized/loading cache dump found; attempting service start"
    start_redis || true
    return 0
  fi

  log "Redis cache is unhealthy/loading or oversized; clearing rebuildable Redis cache files"
  stop_redis || true
  if [[ -d "$redis_dir" ]]; then
    run rm -f "$redis_dir"/temp-*.rdb
    if (( dump_size > 0 )); then
      run rm -f "$dump"
    fi
  fi
  start_redis || true
}

run_precompute_if_needed() {
  if ! bool "$rebuild_on_missing"; then
    return 0
  fi
  if ! redis_is_healthy; then
    log "Redis still unhealthy; skip read-model rebuild"
    return 0
  fi
  if redis EXISTS "$(manifest_key)" 2>/dev/null | grep -q '^1$'; then
    return 0
  fi
  if [[ ! -x "$precompute_path" || ! -x "$onecli_bin" ]]; then
    log "precompute binary or onecli missing; skip read-model rebuild"
    return 0
  fi
  log "Redis read-model manifest missing; rebuilding from Postgres snapshot/canonical data"
  if [[ "$(id -u)" -eq 0 ]] && id deploy >/dev/null 2>&1; then
    run sudo -u deploy env \
      SECOND_BRAIN_ENV_FILE="$env_file" \
      SECOND_BRAIN_PRECOMPUTE_PATH="$precompute_path" \
      ONECLI_BIN="$onecli_bin" \
      ONECLI_PROJECT="$onecli_project" \
      bash -lc 'set -euo pipefail; set -a; . "$SECOND_BRAIN_ENV_FILE"; set +a; cd "$(dirname "$SECOND_BRAIN_PRECOMPUTE_PATH")"; "$ONECLI_BIN" run --project "$ONECLI_PROJECT" -- "$SECOND_BRAIN_PRECOMPUTE_PATH"'
  else
    run env \
      SECOND_BRAIN_ENV_FILE="$env_file" \
      SECOND_BRAIN_PRECOMPUTE_PATH="$precompute_path" \
      ONECLI_BIN="$onecli_bin" \
      ONECLI_PROJECT="$onecli_project" \
      bash -lc 'set -euo pipefail; set -a; . "$SECOND_BRAIN_ENV_FILE"; set +a; cd "$(dirname "$SECOND_BRAIN_PRECOMPUTE_PATH")"; "$ONECLI_BIN" run --project "$ONECLI_PROJECT" -- "$SECOND_BRAIN_PRECOMPUTE_PATH"'
  fi
}

main() {
  recover_redis_if_needed
  if redis_is_healthy; then
    configure_redis_for_cache
    cleanup_temp_rdb_files
    cleanup_large_dump
    run_precompute_if_needed
    log "completed"
  else
    log "Redis is not healthy after recovery attempt: $(redis_ping)"
  fi
}

main "$@"
