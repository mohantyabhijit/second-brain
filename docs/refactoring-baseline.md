# Refactoring and Security Baseline

Baseline commit: `e1edd5f`

Captured: 2026-07-11

Scope: tracked application and operational source in `frontend`, `backend`,
`cloudflare`, and `scripts`.

This document freezes the starting measurements and behavioral boundaries for
the staged refactoring and security-hardening effort. Measurements must use the
same definitions in the final comparison.

## Reproducible Baseline

| Measure | Baseline | Command |
| --- | ---: | --- |
| Production nonblank LOC | 22,337 | `scripts/codebase-metrics.sh` |
| Production source files | 124 | `scripts/codebase-metrics.sh` |
| Meaningful automated tests | 136 | `scripts/codebase-metrics.sh` |
| Go statement coverage | 38.0% | `cd backend && go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` |
| Frontend coverage | Not measurable | Vitest reports that `@vitest/coverage-v8` is missing |
| Typecheck, lint, tests, build | Pass | `npm run ci` |
| Browser E2E suite | Absent | No Playwright/Cypress dependency, config, or test tree exists |
| npm audit | 10 packages: 5 high, 3 moderate, 2 low | `npm audit` |
| Reachable Go vulnerabilities | 11 | `cd backend && govulncheck ./...` |

The test total is the number of passing named cases, not files: 105 Go tests,
24 Vitest tests, five Cloudflare edge-worker tests, and two operational script
tests. The required 200% target is therefore at least 272 meaningful cases.

Production LOC counts nonblank tracked `.go`, `.ts`, `.tsx`, `.mjs`, and `.sh`
lines in the runtime paths. It excludes tests/specs, generated declarations,
fonts/design assets, dependencies, docs, fixtures, migrations, outputs, and
other non-runtime artifacts. The 30% reduction target is 15,636 lines or fewer.

## Architecture and Trust Boundaries

- The Next.js frontend is statically rendered and reads product state only
  through the Go API. Supabase provides browser authentication; publishable
  Supabase configuration is the only provider configuration allowed client-side.
- The Go API owns routing, authorization of operator actions, validation,
  provider calls, and persistence. The worker owns ingestion, synthesis,
  graph sync, digest generation, and read-model publication.
- PostgreSQL plus private filesystem objects are canonical. `pgvector` supports
  semantic retrieval. Redis app-state/source indexes and Neo4j graph data are
  rebuildable derived stores.
- Cloudflare and nginx front the static UI and API. OneCLI injects or proxies
  provider credentials. External integrations include OpenAI, X, YouTube,
  Supadata, Exa, Resend, Langfuse, Neo4j, Supabase Auth, and Redis.
- Normal page loads must stay on precomputed read models. OpenAI, Exa, Neo4j
  traversal, and provider refresh work must remain user-initiated or scheduled.

## Critical Workflows to Preserve

1. Application boot: each public route loads the correct app-state view,
   handles ETags/cache misses, and renders empty, loading, error, and populated
   states without exposing credentials.
2. Source browsing: X and YouTube feeds support bounded incremental loading,
   fuzzy search, aliases, source links, and item details.
3. Newsletter: archive loading, search, issue selection, content rendering,
   illustration access, and persisted delivery state remain compatible.
4. Knowledge graph: precomputed graph loading, layout, selection, filtering,
   zoom/pan, and fallback behavior remain usable.
5. Ask Second Brain: anonymous owner context and authenticated bearer sessions
   both work; vector, graph, and optional web evidence remain source-grounded.
6. Refresh: authenticated start, status polling, source-material reuse,
   synthesis, canonical persistence, read-model publication, and cache purge.
7. Feedback and tweet sharing: inputs are validated, operator actions are
   authorized, provider failures are bounded, and responses do not leak data.
8. X OAuth: start, callback/state validation, token persistence, status, and
   local/production operational wrappers remain compatible.
9. Scheduled worker: startup refresh, interval refresh, daily digest,
   illustration/delivery ordering, graph sync, and graceful shutdown.
10. Deployment and operations: migrations, static build, Go binaries, nginx/
    systemd wiring, OneCLI fallback behavior, Redis maintenance, observability,
    and production audit continue to fail safely.

## Test Gaps Before Refactoring

- No real-browser E2E coverage exists for any route or critical journey.
- Frontend statement/branch/function coverage is not collected.
- Many command packages, Redis integration paths, provider clients, tracing,
  graph RAG, vector RAG, synthesis, learning, and major UI components have no
  direct tests or only partial happy-path coverage.
- Most backend tests use in-memory/fake boundaries; the Postgres migration test
  validates SQL shape but does not exercise a live database in the default suite.
- Authentication and authorization are covered inside router tests but lack a
  browser-to-API regression suite and a systematic endpoint policy matrix.
- Provider error handling is unevenly characterized, especially timeouts,
  oversized responses, malformed JSON, rate limits, and partial persistence.
- Deployment scripts and OneCLI wrappers largely lack automated contract tests.
- Accessibility, mobile layout, keyboard navigation, browser caching, and
  client-side data-exposure checks are not automated.

## Initial Security Findings

`npm audit` reports vulnerable transitive packages under Next.js/Vite/Wrangler,
including `undici`, `ws`, `miniflare`, `postcss`, `esbuild`, `js-yaml`, and
`@babel/core`. `govulncheck` finds reachable issues in `pgx`, `go-redis`, the
OpenTelemetry SDK/exporter, `x/net`, and the installed Go standard library.

The initial tracked-file token-pattern scan found no high-confidence private
keys or common provider-token formats. This does not replace history scanning,
runtime configuration review, authorization analysis, or provider-specific
secret validation; those remain required security work.

## Phase Gates

Every behavior-changing batch must pass its focused tests plus `npm run ci`.
Dependency/security batches must rerun `npm audit` and `govulncheck`. Browser
work must pass the E2E suite once established. Commits must contain only the
logical batch and must not include the pre-existing user-owned README or docs.
