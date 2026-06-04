# Paid Service Exit Plan

Audited on 2026-06-04. Verify each provider's account page before cancellation;
public free-tier limits and account-specific billing can change.

## Current Service Inventory

| Service | Current role | Current cost class | Can it be removed? | Replacement or impact |
| --- | --- | --- | --- | --- |
| DigitalOcean `ubuntu-sgp` | nginx, frontend, Go API and worker, Redis, object storage, other sites | Paid and required | Not by itself | Move all hosted sites and objects to another server first |
| DigitalOcean `codex-crapbox` | PostgreSQL plus pgvector, remote devbox, other repos | Paid and required | Not by itself | Move the database and devbox role first |
| Cloudflare | DNS, CDN, edge/app-state cache | Verified Free plan | Yes, but keep it | Direct nginx works, with worse caching and exposure |
| Supabase | Auth only | Free plan at audit time | Yes, with code changes | Cloudflare Access or a small first-party admin session system |
| GitHub Actions | CI and deployment for a public repository | Standard runners are free for public repos | Yes, but keep it | Run deployment manually or from a self-hosted runner |
| Redis | Precomputed read models and source-material cache | Self-hosted | Yes | App falls back to Postgres, but page loads and refreshes get slower |
| Neo4j Aura | Derived graph index | Account plan not verified | Yes | Build graph views from Postgres; Ask loses graph-source enrichment |
| OneCLI | Runtime secret injection and provider gateway | Account plan not verified | Yes, with deployment changes | Root-owned systemd credentials or environment files plus GitHub environment secrets |
| OpenAI API | Synthesis, Ask, embeddings, digest text and illustrations | Usage-priced | Partly | Use extractive fallbacks; add another provider or local model for retained AI features |
| Exa | Optional live web search for Ask | Free credits then usage-priced | Yes now | Ask remains grounded in saved sources |
| Supadata | YouTube transcripts | Free tier, guarded at 100 unique requests/month | Yes, with reliability loss | Best-effort `yt-dlp` caption extraction or omit transcripts |
| Resend | Digest email delivery | Free tier available | Yes | Keep digests in the app, or send through another SMTP provider |
| X API | Bookmark ingestion and OAuth | Pay-per-use | Yes, with feature loss | Manual export/import or a browser extension; automatic bookmark sync stops |
| YouTube Data API | Playlist metadata | Free quota | Yes, with feature loss | Manual URL/import workflow |

## Recommended Order

### 1. Protect Canonical Data First

Before cancelling or moving anything:

1. Schedule recurring encrypted PostgreSQL dumps.
2. Archive `/srv/second-brain/object-storage`.
3. Copy both backups off the two droplets.
4. Run a restore test on a separate host.

[Cloudflare R2](https://developers.cloudflare.com/r2/pricing/) is a practical
backup target because its free allowance includes 10 GB-month of storage and
internet egress is free. The current one-time backups stored on `ubuntu-sgp`
do not protect against losing that droplet.

### 2. Stop Optional Usage Charges

- Remove or disable Exa first. It is optional in the Ask path.
- Disable digest illustrations before digest text. Illustrations are
  nonessential and require a paid generation call.
- Disable OpenAI synthesis judging and use existing extractive fallback paths.
- Put hard budgets and usage alerts on OpenAI, X, and Supadata until their calls
  are removed.
- Supadata is protected in code: each YouTube video is claimed in Postgres
  before the provider call, historical videos are backfilled into the ledger,
  each new video gets one native auto-language request, and
  `SUPADATA_MONTHLY_REQUEST_LIMIT=100` stops calls after the free-tier ceiling.
  A failed or missing transcript is not retried automatically; delete its
  ledger row manually only when intentionally spending another credit.
- Keep Resend only while it stays within its
  [free allowance](https://resend.com/pricing/). The digest is persisted before
  email delivery, so email can be disabled without losing the newsletter page.

### 3. Remove Optional Platforms

- Remove Neo4j Aura after confirming the graph UI and Ask path use Postgres
  read-model/vector fallbacks. Neo4j is derived and rebuildable.
- Replace OneCLI with root-owned systemd credentials or environment files.
  Move every active provider secret first, then change the API and worker
  service commands. Preserve X token rotation or remove X ingestion at the
  same time.
- Keep Supabase Auth while it remains free. For a single private operator,
  [Cloudflare Access Free](https://developers.cloudflare.com/cloudflare-one/plans/)
  can replace it, but the backend must validate Access JWTs and protected route
  policies must be migrated before deleting Supabase.

### 4. Replace Paid AI Calls

The current code already has extractive fallbacks for source synthesis and Ask.
Digest generation needs a deterministic/extractive implementation before
OpenAI can be fully removed.

Possible free-quota alternatives:

- [Gemini API free tier](https://ai.google.dev/gemini-api/docs/pricing/)
- [Cloudflare Workers AI daily free allocation](https://developers.cloudflare.com/workers-ai/platform/pricing/)

Both require a provider abstraction and accept free-tier quotas, changing
terms, and possible data-handling tradeoffs. Running a useful local LLM on
either current 1 GB CPU-only droplet is not realistic.

For transcripts, replace Supadata with best-effort `yt-dlp` caption extraction
only if intermittent failures and YouTube rate limits are acceptable.

For X bookmarks, there is no clean zero-cost drop-in API replacement. The
lowest-cost durable option is a manual export/import flow or a browser extension
that sends selected bookmarks to this app.

## Hosting Options

### Recommended: Keep Both Droplets Until a Planned Consolidation

Do not consolidate PostgreSQL onto `ubuntu-sgp` or the entire application onto
`codex-crapbox` in their current 1 GB configurations. Both are shared, and the
application host already uses swap.

To reduce rather than eliminate hosting cost, move the app, database, cache,
object storage, and devbox workloads onto one new server with at least 2 GB RAM
after offsite backups exist. This saves one droplet but reduces fault isolation.

### Zero-Dollar Hosting: Higher Operational Risk

[Oracle Cloud Always Free](https://www.oracle.com/cloud/free/) or a home server
behind Cloudflare Tunnel could remove VPS fees. These options trade cost for
capacity availability, account-risk, hardware/network reliability, and
operational burden. They should not become the only copy of canonical data.

Free application platforms are a poor fit for the current architecture because
it needs a persistent worker, PostgreSQL with pgvector, Redis, scheduled jobs,
filesystem or object storage, and stable outbound provider access.

## Safe Cancellation Checklist

Do not cancel a service until:

- no production environment or systemd unit references it;
- the replacement path has passed a production smoke test;
- canonical data has a verified offsite backup;
- API keys and OAuth callbacks have been revoked or migrated;
- the account's final usage and billing page has been checked.

Useful current pricing references:

- [DigitalOcean bandwidth](https://docs.digitalocean.com/platform/billing/bandwidth/)
- [DigitalOcean VPC pricing](https://docs.digitalocean.com/products/networking/vpc/details/pricing/)
- [GitHub Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions)
- [Neo4j Aura pricing](https://neo4j.com/pricing/)
- [Exa pricing](https://exa.ai/pricing)
- [Supadata pricing](https://supadata.ai/pricing)
- [X API pricing](https://docs.x.com/x-api/getting-started/pricing)
- [OpenAI API pricing](https://openai.com/api/pricing/)
- [YouTube Data API quota costs](https://developers.google.com/youtube/v3/determine_quota_cost)
