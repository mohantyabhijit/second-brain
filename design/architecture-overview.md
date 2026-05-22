# Second Brain Architecture Overview

```mermaid
flowchart LR
  X[X bookmarks] --> A[Source intake ledger]
  Y[YouTube playlist] --> A
  A --> B[Evidence artifact module]
  B --> C[Prompt synthesis module]
  C --> D[(Supabase Postgres)]
  B --> E[(Supabase Storage)]
  C --> F[(pgvector)]
  C --> G[Graph sync outbox]
  G --> H[(Neo4j derived index)]
  D --> I[Knowledge Inbox read model]
  I --> J[Next.js UI]

  classDef deep fill:#0f172a,color:#fff,stroke:#0f172a,stroke-width:2px;
  class A,B,C,I deep;
```

Supabase Postgres is the canonical source of truth for source identity,
processing state, synthesis cache rows, and artifact metadata. Supabase Storage
holds raw and derived text artifacts such as X article bodies and YouTube
transcripts. `pgvector` and Neo4j are derived indexes fed after source-grounded
records exist in Supabase.
