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
