# Second Brain Research Agent - Initial Plan

## Summary

This project turns saved material into a personal research system. It starts with unread X bookmarks and YouTube videos, then grows into an autonomous research agent that reads, summarizes, connects ideas, and sends a daily 5pm digest.

The UI is only a presentation layer. The core product is the ingestion, extraction, synthesis, ranking, delivery, and feedback loop.

## North Star

Build a fully autonomous research agent that can:

- Read saved X bookmarks and YouTube videos.
- Extract transcripts, quotes, timestamps, and notable insights.
- Connect related ideas across different sources.
- Send a useful daily email from saved material.
- Eventually discover new relevant content from the web without manual bookmarking.

## Functional Gist

The product should work as a pipeline:

1. **Input**
   - X bookmarks.
   - YouTube saved videos.
   - Later: Apple Notes, WhatsApp notes, Obsidian fragments, screenshots, paper-note images, and rough thoughts.

2. **Extract**
   - Tweet text, author metadata, timestamps, metrics, and URLs.
   - Video metadata, captions, transcripts, and timestamps.
   - Source links and provenance for every generated note.

3. **Understand**
   - Summaries.
   - Notable insights.
   - Original quotes.
   - Best video moments.
   - Topics, entities, and confidence notes.

4. **Connect**
   - Cluster related tweets, videos, and notes.
   - Identify recurring user interests.
   - Surface patterns that would otherwise stay buried in the backlog.

5. **Deliver**
   - Generate a personal 5pm newsletter.
   - Include source-grounded summaries, video timestamps, and follow-up reading.
   - Learn from feedback signals.

## Phase 1: MVP - Prove Ingestion And Summarization

Goal: connect to the first two sources, extract a small real sample, and summarize it with source references.

### Build

- Set up OAuth for X with bookmark read, tweet read, and users read scopes.
- Fetch 5 recent X bookmarks for the authenticated user.
- Preserve original tweet text, author, created time, public metrics, and source URL.
- Summarize each bookmark with the original post quoted or linked.
- Connect to the YouTube API and test access to the user’s saved videos.
- Test transcript extraction on real YouTube videos where captions are available.

### Product Behavior

- Show whether each saved item is worth reading now, later, or skipping.
- Keep summaries short enough to reduce backlog regret.
- Preserve attribution so every summary can be traced back to the original source.
- Avoid advanced personalization until the raw ingestion path is proven.

### Validation

- X bookmark request succeeds for the authenticated user.
- 5 bookmarks are fetched and summarized.
- YouTube Watch Later access is tested and either works or is documented as blocked.
- At least one transcript path is tested on real videos.
- Summary output includes source links and does not hallucinate unsupported claims.

### Important Caveat

X supports bookmarked post lookup through the X API for the authenticated user, but this requires approved developer access and the right OAuth scopes.

YouTube’s official API documentation says some playlists cannot be listed, including Watch Later. The MVP should verify this directly, then fall back to a dedicated “Second Brain Inbox” playlist or manual link import if Watch Later is blocked.

## Phase 2: Improve Core Product By Understanding The User

Goal: move from isolated summaries to a personal knowledge layer that recognizes repeated interests across tweets and videos.

### Build

- Expand beyond 5 bookmarks and videos.
- Store normalized records for sources, excerpts, summaries, tags, timestamps, and model outputs.
- Add embeddings or another retrieval layer for semantic similarity.
- Create topic clusters that connect related tweets, videos, and notes.
- Track user feedback on whether an output was useful, obvious, stale, or irrelevant.

### Product Behavior

- Detect themes the user repeatedly saves.
- Connect similar ideas across X and YouTube.
- Merge related concepts into insight clusters instead of isolated summaries.
- Explain why two items are connected.
- Keep source attribution visible for every synthesized insight.

### Validation

- A tweet and video about the same concept can be surfaced together.
- Each generated insight includes source links and enough context to verify it.
- The system can identify at least a few recurring themes from the user’s saved material.
- Feedback changes later ranking or clustering behavior.

## Phase 3: Newsletter And Self-Learning Loop

Goal: generate and send a useful daily 5pm personal newsletter from saved X and YouTube material.

### Build

- Create scheduled digest generation for 5pm in the user’s configured timezone.
- Rank items by novelty, relevance, source quality, and backlog age.
- Send email with source quotes, YouTube timestamps, synthesized themes, and follow-up links.
- Add lightweight feedback actions such as more like this, less like this, expand, and archive.

### Product Behavior

- Make the email feel like a personal research newsletter, not a generic AI summary dump.
- Include best moments from videos so long content becomes skimmable.
- Mix related tweets and videos when they discuss the same idea.
- Use feedback to improve future digests.
- Start polishing the UI only where it improves trust, source review, and retention.

### Validation

- A digest is generated from mixed X and YouTube inputs.
- Every section includes source attribution.
- Video sections include timestamps when transcripts support them.
- Feedback changes ranking or selection in a later digest.
- The digest is useful even when the user only has a few minutes to read it.

## Phase 4: Multi-User Support

Goal: add the account, permission, privacy, and operations layer needed to support more than one person safely.

### Build

- Add authentication.
- Add per-user OAuth connections.
- Encrypt and isolate source tokens per user.
- Scope all saved items, summaries, preferences, and digests to a user account.
- Move ingestion and digest generation into background jobs with retries and observability.
- Add export, deletion, consent, and source-connection management.

### Product Behavior

- Let each user configure sources, digest timing, content preferences, and retention rules.
- Keep the product self-learning per user rather than training one shared preference profile.
- Make onboarding validate source access before promising digest quality.
- Ensure failed source connections do not affect other users.

### Validation

- Two users cannot see each other’s data or source tokens.
- A failed source connection does not break another user’s digest.
- A user can disconnect, export, or delete stored data.
- Background jobs can be monitored and retried.

## Feasibility Risks

### X Bookmark Access

X supports bookmark lookup for the authenticated user, but this depends on developer access, OAuth setup, and the right scopes. This should be validated before building downstream product behavior.

### YouTube Watch Later Access

YouTube Watch Later may not be available through the public API. If blocked, the MVP should use one of these alternatives:

- A dedicated YouTube playlist called “Second Brain Inbox”.
- Manual YouTube URL import.
- Browser-assisted export later, if appropriate.

### Transcript Coverage

Not every video has a usable transcript. Captions may be missing, private, auto-generated, multilingual, or restricted. The system should store transcript status and explain when a video cannot be summarized from transcript data.

### Source-Grounded Summaries

Every insight should trace back to an original tweet, quote, transcript passage, or timestamp. The product should not become a generic summarizer that the user cannot verify later.

## Acceptance Criteria

### Phase 1

- Fetch 5 X bookmarks for the authenticated user.
- Summarize them with source links.
- Test YouTube saved-video access.
- Extract at least one real video transcript if available.
- Document any API limitation clearly.

### Phase 2

- Store normalized source records and summaries.
- Identify recurring themes.
- Connect related tweets and videos.
- Preserve source attribution for synthesized insights.
- Capture user feedback.

### Phase 3

- Generate a mixed-source daily digest.
- Send it at 5pm.
- Include quotes, timestamps, and source links.
- Use feedback to influence future ranking.

### Phase 4

- Support multiple authenticated users.
- Isolate user data and source tokens.
- Support per-user digest preferences.
- Provide export and deletion controls.
- Monitor background ingestion and digest jobs.

## Current Artifact

The interactive browser version of this plan is saved as:

`project-plan.html`

The durable Markdown version is saved as:

`initial_plan.md`

## Reference Links

- [X API Bookmarks Lookup](https://docs.x.com/x-api/posts/bookmarks/quickstart/bookmarks-lookup)
- [X API Bookmarks Overview](https://docs.x.com/x-api/posts/bookmarks/introduction)
- [YouTube API playlists.list](https://developers.google.com/youtube/v3/docs/playlists/list)
- [YouTube API captions.list](https://developers.google.com/youtube/v3/docs/captions/list)
