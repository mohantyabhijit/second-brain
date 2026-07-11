#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

production_files() {
  git grep -Il '' -- \
    'frontend/app/**/*.ts' \
    'frontend/app/**/*.tsx' \
    'frontend/src/**/*.ts' \
    'frontend/src/**/*.tsx' \
    'frontend/proxy.ts' \
    'backend/**/*.go' \
    'cloudflare/*.mjs' \
    'scripts/*.sh' \
    'scripts/*.mjs' |
    rg -v '(_test\.go|\.test\.|\.spec\.|/migrations/|/fixtures/|/design-system/|next-env\.d\.ts$)'
}

production_loc="$({
  production_files | while IFS= read -r file; do
    awk 'NF { count++ } END { print count + 0 }' "$file"
  done
} | awk '{ total += $1 } END { print total + 0 }')"
production_file_count="$(production_files | wc -l | tr -d ' ')"

go_tests="$(
  cd backend
  GOCACHE="$repo_root/.cache/go-build" go test ./... -json |
    jq -s '[.[] | select(.Action == "pass" and .Test != null)] | length'
)"

frontend_tests="$(
  npm --prefix frontend test -- --reporter=json --outputFile=/tmp/second-brain-vitest-metrics.json >/dev/null
  jq '.numPassedTests' /tmp/second-brain-vitest-metrics.json
)"

edge_tests="$(node --test --test-reporter=tap cloudflare/*.test.mjs | awk '/^ok [0-9]+ - / { count++ } END { print count + 0 }')"
ops_tests="$(node --test --test-reporter=tap scripts/*.test.mjs | awk '/^ok [0-9]+ - / { count++ } END { print count + 0 }')"
total_tests="$((go_tests + frontend_tests + edge_tests + ops_tests))"

jq -n \
  --argjson production_loc "$production_loc" \
  --argjson production_files "$production_file_count" \
  --argjson go_tests "$go_tests" \
  --argjson frontend_tests "$frontend_tests" \
  --argjson edge_tests "$edge_tests" \
  --argjson ops_tests "$ops_tests" \
  --argjson total_tests "$total_tests" \
  '{
    production_loc: $production_loc,
    production_files: $production_files,
    tests: {
      go: $go_tests,
      frontend: $frontend_tests,
      edge: $edge_tests,
      ops: $ops_tests,
      total: $total_tests
    }
  }'
