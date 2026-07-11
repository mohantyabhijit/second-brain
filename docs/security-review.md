# Security Review

This is a living record for the staged codebase security review. Findings are
recorded by evidence source and are closed only after the relevant scanner and
behavioral gates pass.

## Dependency and Toolchain Findings

### Remediated

- Upgraded the build, CI, and deployment toolchain from Go 1.23 to Go 1.26.5.
  This closes the reachable standard-library findings reported against the
  original local and deployment toolchains.
- Upgraded `pgx` from 5.7.2 to 5.10.0, closing the reachable placeholder-
  confusion SQL injection advisory.
- Upgraded `go-redis` from 9.7.0 to 9.21.0, closing the connection setup
  response-ordering advisory.
- Upgraded OpenTelemetry core, SDK, and OTLP HTTP exporter from 1.37.0 to
  1.44.0, closing PATH-hijacking and oversized-response findings.
- Upgraded `x/net` and the related Go dependency graph. `govulncheck` now
  reports `No vulnerabilities found` under Go 1.26.5.
- Upgraded Wrangler from 4.94.0 to 4.110.0, eliminating the high-severity
  Miniflare, Undici, WebSocket, and esbuild findings.
- Upgraded Next.js from 16.2.6 to the latest stable 16.2.10 and applied safe
  npm transitive repairs. npm findings fell from 10 total, including five high,
  to two moderate scanner entries representing one transitive issue.

### Residual dependency risk

`npm audit` still reports Next.js and its nested PostCSS 8.4.31 for
GHSA-qx2v-qp2m-jg93. The registry currently proposes Next.js 9.3.3 as the only
automated remediation, which is an unsafe and invalid downgrade for this
Next.js 16 application. Next.js 16.2.10 is the latest stable release at the
time of this review and pins the nested PostCSS version.

The affected PostCSS stringify path is used during trusted application builds;
the application does not accept user-supplied CSS for compilation. This lowers
current exploitability but does not erase the dependency finding. Retain it as
a monitored residual, upgrade as soon as Next.js ships a fixed stable release,
and do not use `npm audit fix --force` to bypass compatibility gates.

## Required Remaining Review Lanes

- Repository and history secret scanning, plus build-artifact data-exposure
  checks without printing secret values.
- Authentication and authorization policy mapping for every API route.
- Input size, format, URL, identifier, and content-type validation.
- SQL, command, template, header, URL-fetch/SSRF, and prompt-injection review.
- Error/log/trace privacy, CORS, cache-control, cookie, and security-header
  review.
- Deployment, environment, OneCLI, OAuth state/token, Redis, database, object
  storage, and observability configuration review.

## API Trust-Boundary Findings

### Remediated

- Required a validated Supabase session for both X OAuth start entrypoints. The
  legacy redirect endpoint previously allowed an anonymous caller to create an
  OAuth flow scoped to the public owner workspace.
- Marked authenticated knowledge-graph reads `private, no-store`; they
  previously inherited public shared-cache headers even when resolved to a
  private owner.
- Replaced four permissive JSON decoders with one strict decoder that caps
  mutation bodies at 1 MiB, rejects unknown fields, and rejects trailing JSON.
- Added `nosniff`, `no-referrer`, and frame-denial headers consistently across
  API responses.
- Restricted persisted digest illustrations to an explicit raster image MIME
  allowlist and a 20 MiB decoded-size limit. Arbitrary persisted content types
  can no longer be served as immutable public images.

Regression coverage proves anonymous operator routes fail closed, strict JSON
parsing rejects ambiguous and oversized payloads, authenticated graph data is
not publicly cacheable, unsafe illustration media is rejected, and baseline
security headers are present.

## Secret and Static-Analysis Review

The redacted full-history Gitleaks scan reported five candidates. Manual review
confirmed all five are non-secret test fixtures, environment-variable
references, or the Stripe publishable key intentionally embedded for browser
checkout. No private credential was confirmed in tracked history. Production
bundle scanning found configuration identifier strings, but no private-key
material or server-side credential value.

Gosec initially reported 32 candidates. Confirmed findings were remediated:

- Local knowledge-run and feedback files now use mode `0600`; their runtime
  directories use `0700`.
- Transcript-request ledgers, source artifacts, and newsletter experiment
  reports now use private file/directory modes.
- The memory-profile timestamp conversion clamps unsigned nanoseconds before
  conversion to a signed duration.
- Rotated X-token environment writes now handle errors, transaction rollback
  intent is explicit, and OneCLI secret-update failures no longer include
  command output that could contain sensitive provider detail.

The remaining gosec candidates are reviewed false positives or controlled
operational boundaries: constant *names* of secret slots, production-conditional
cookie security, argv-based (not shell-based) execution of configured OneCLI,
macOS Keychain, and sibling deployment binaries, operator-selected migration
and lint paths, administrator-selected CA bundle paths, and an embedding index
guarded by a preceding equal-length check. These remain documented because
subprocess arguments containing rotated credentials can be visible to a local
same-host process observer; eliminating that residual requires OneCLI/Keychain
support for stdin or file-descriptor secret input.
