# Second Brain Architecture Overview

```mermaid
flowchart TB
  W[Two-hour self-organizing worker] --> X[X bookmarks]
  W --> Y[YouTube playlist metadata]
  X --> A[Source adapter seam]
  Y --> A

  A --> B{Source material already processed?}
  R[(Redis source-material cache)] --> B
  D[(Supabase canonical ledger)] --> B
  B -->|same source type, external ID, prompt, model, capture hash| Z[Skip re-ingestion and expensive processing]
  B -->|new or changed material| C[Evidence artifact module]

  Z --> Q[Digest generation from latest saved run]
  C --> T[Transcript and body fetch]
  T --> E[(Supabase Storage)]
  T --> S[Prompt synthesis module]
  S --> D
  S --> F[(pgvector embeddings)]
  S --> G[Graph sync outbox]
  G --> H[(Neo4j derived index)]

  D --> M[Redis read-model publisher]
  Q --> M
  M --> N[(Redis app-state, views, digests)]
  N --> I[Go API Redis-first reads]
  D --> I
  H --> I
  I --> J[Next.js UI]
  Q --> K[Resend email digest]

  classDef deep fill:#0f172a,color:#fff,stroke:#0f172a,stroke-width:2px;
  class A,B,C,I,M,Q,S,T deep;
```

Supabase Postgres is the canonical source of truth for source identity,
capture hashes, synthesis cache rows, digest records, and artifact metadata.
Redis stores derived source-material and read-model state so scheduled refreshes
can skip already processed captures before transcript fetches, synthesis,
embeddings, storage rewrites, and graph sync.

Supabase Storage holds raw and derived text artifacts such as X article bodies
and YouTube transcripts. `pgvector` and Neo4j are derived indexes fed after
source-grounded records exist in Supabase. Digest generation remains active even
when refresh work is skipped because there is no new source material.
