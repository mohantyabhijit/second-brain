import { mkdtempSync, rmSync, writeFileSync, chmodSync, utimesSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import assert from "node:assert/strict";

const scriptPath = resolve("scripts/second-brain-redis-maintenance.sh");

function makeTempDir() {
  return mkdtempSync(join(tmpdir(), "second-brain-redis-maintenance-"));
}

function writeExecutable(path, body) {
  writeFileSync(path, body);
  chmodSync(path, 0o755);
}

function makeFakeRedisCli(dir, infoBody, exists = "1") {
  const commandLog = join(dir, "redis-commands.log");
  const infoPath = join(dir, "redis-info.txt");
  writeFileSync(infoPath, infoBody);
  const redisCliPath = join(dir, "redis-cli");
  writeExecutable(
    redisCliPath,
    `#!/usr/bin/env bash
set -euo pipefail
if [[ "\${1:-}" == "-u" ]]; then
  shift 2
fi
cmd="\${1:-}"
shift || true
printf '%s %s\\n' "$cmd" "$*" >> "${commandLog}"
case "$cmd" in
  PING) printf 'PONG\\n' ;;
  INFO) cat "${infoPath}" ;;
  CONFIG) printf 'OK\\n' ;;
  EXISTS) printf '${exists}\\n' ;;
  *) printf 'OK\\n' ;;
esac
`
  );
  return { redisCliPath, commandLog };
}

function makeFakeSystemctl(dir) {
  const systemctlPath = join(dir, "systemctl");
  writeExecutable(
    systemctlPath,
    `#!/usr/bin/env bash
set -euo pipefail
printf 'systemctl %s\\n' "$*" >> "${join(dir, "systemctl.log")}"
`
  );
  return systemctlPath;
}

function runMaintenance(dir, redisCliPath, systemctlPath, extraEnv = {}) {
  const result = spawnSync("bash", [scriptPath], {
    cwd: resolve("."),
    env: {
      ...process.env,
      SECOND_BRAIN_ENV_FILE: join(dir, "missing.env"),
      REDIS_CLI: redisCliPath,
      SYSTEMCTL: systemctlPath,
      REDIS_URL: "redis://example.invalid:6379/0",
      SECOND_BRAIN_REDIS_DIR: dir,
      SECOND_BRAIN_REDIS_STALE_TEMP_MINUTES: "1",
      SECOND_BRAIN_REDIS_DUMP_MAX_BYTES: "128",
      SECOND_BRAIN_REDIS_REBUILD_ON_MISSING: "false",
      ...extraEnv
    },
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr || result.stdout);
  return result;
}

test("redis maintenance removes stale temp files and oversized cache dumps", () => {
  const dir = makeTempDir();
  try {
    const staleTemp = join(dir, "temp-111.rdb");
    const freshTemp = join(dir, "temp-222.rdb");
    const dump = join(dir, "dump.rdb");
    writeFileSync(staleTemp, "stale");
    writeFileSync(freshTemp, "fresh");
    writeFileSync(dump, "x".repeat(256));
    const old = new Date(Date.now() - 5 * 60 * 1000);
    utimesSync(staleTemp, old, old);

    const { redisCliPath } = makeFakeRedisCli(
      dir,
      "loading:0\nrdb_bgsave_in_progress:0\nused_memory_human:12M\n"
    );
    const systemctlPath = makeFakeSystemctl(dir);

    runMaintenance(dir, redisCliPath, systemctlPath);

    assert.equal(existsSync(staleTemp), false, "stale temp RDB should be removed");
    assert.equal(existsSync(freshTemp), true, "fresh temp RDB should be preserved");
    assert.equal(existsSync(dump), false, "oversized dump should be removed");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test("redis maintenance skips RDB cleanup while bgsave is active", () => {
  const dir = makeTempDir();
  try {
    const staleTemp = join(dir, "temp-111.rdb");
    const dump = join(dir, "dump.rdb");
    writeFileSync(staleTemp, "stale");
    writeFileSync(dump, "x".repeat(256));
    const old = new Date(Date.now() - 5 * 60 * 1000);
    utimesSync(staleTemp, old, old);

    const { redisCliPath } = makeFakeRedisCli(
      dir,
      "loading:0\nrdb_bgsave_in_progress:1\nused_memory_human:12M\n"
    );
    const systemctlPath = makeFakeSystemctl(dir);

    runMaintenance(dir, redisCliPath, systemctlPath);

    assert.equal(existsSync(staleTemp), true, "temp RDB should remain during BGSAVE");
    assert.equal(existsSync(dump), true, "dump should remain during BGSAVE");
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

