import assert from "node:assert/strict";
import { chmod, mkdtemp, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";

const root = path.resolve(import.meta.dirname, "..");

async function fakeRuntime() {
  const directory = await mkdtemp(path.join(tmpdir(), "second-brain-onecli-"));
  const security = path.join(directory, "security");
  const onecli = path.join(directory, "onecli");
  await writeFile(
    security,
    `#!/usr/bin/env bash
service=""
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "-s" ]]; then service="$2"; shift 2; else shift; fi
done
case "$service" in
  second-brain/X_CLIENT_ID_PROD) echo prod-client ;;
  second-brain/X_CLIENT_SECRET_PROD) echo prod-secret ;;
  second-brain/X_USER_ACCESS_TOKEN_PROD) echo prod-access ;;
  second-brain/X_REFRESH_TOKEN_PROD) echo prod-refresh ;;
  *) exit 1 ;;
esac
`,
  );
  await writeFile(
    onecli,
    `#!/usr/bin/env bash
printf 'args=%s\n' "$*"
printf 'gateway=%s redis=%s prod_client=%s supabase=%s image=%s interval=%s\n' \
  "\${ONECLI_GATEWAY:-}" "\${REDIS_CACHE_ENABLED:-}" \
  "$([[ "\${X_CLIENT_ID:-}" == prod-client ]] && echo yes || echo no)" \
  "$([[ -n "\${SUPABASE_URL:-}" ]] && echo yes || echo no)" \
  "\${OPENAI_IMAGE_MODEL:-}" "\${WORKER_REFRESH_INTERVAL:-}"
`,
  );
  await Promise.all([chmod(security, 0o700), chmod(onecli, 0o700)]);
  return { directory, onecli };
}

for (const scenario of [
  { name: "API", script: "run-backend-onecli.sh", command: "api", expected: "supabase=yes" },
  { name: "refresh", script: "run-refresh-onecli.sh", command: "refresh", expected: "image= interval=" },
  { name: "digest", script: "run-digest-onecli.sh", command: "digest", expected: "image=gpt-image-1" },
  { name: "worker", script: "run-worker-onecli.sh", command: "worker", expected: "interval=2h" },
]) {
  test(`OneCLI ${scenario.name} wrapper preserves runtime contract`, async () => {
    const fake = await fakeRuntime();
    const result = spawnSync("bash", [path.join(root, "scripts", scenario.script)], {
      cwd: root,
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${fake.directory}:${process.env.PATH}`,
        ONECLI: fake.onecli,
        ONECLI_PROJECT: "test-project",
        USER: "test-user",
        DATABASE_URL: "postgres://configured",
        REDIS_URL: "redis://configured",
        SUPABASE_URL: "https://supabase.example.test",
      },
    });
    assert.equal(result.status, 0, result.stderr);
    assert.match(result.stdout, new RegExp(`args=run --project test-project -- go run ./cmd/${scenario.command}`));
    assert.match(result.stdout, /gateway=true redis=true prod_client=yes/);
    assert.ok(result.stdout.includes(scenario.expected), result.stdout);
  });
}
