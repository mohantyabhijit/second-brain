# Second Brain Research Agent

Phase 1 proves the hard path first: fetch real saved material from X and YouTube, extract source text or transcripts, summarize without losing attribution, and show which API limitations are blocking the product.

## Phase 1 App

```bash
npm install
npm run dev
```

Open `http://localhost:3000` and run the Phase 1 validation. The app writes non-secret validation output to `data/runtime/latest-phase1.json`, which is ignored by git.

## Secrets

OneCLI was installed at:

```bash
/Users/abhijitmohanty/.local/bin/onecli
```

Authenticate it before saving secrets:

```bash
/Users/abhijitmohanty/.local/bin/onecli auth login --api-key oc_...
```

Then export the real values only for the save command and store them in OneCLI:

```bash
export X_USER_ACCESS_TOKEN=...
export YOUTUBE_API_KEY=...
export YOUTUBE_ACCESS_TOKEN=...
npm run onecli:save-secrets
```

Run the app through OneCLI gateway mode when you want HTTP secret injection:

```bash
ONECLI_GATEWAY=true /Users/abhijitmohanty/.local/bin/onecli run --project second-brain npm -- run dev
```

`YOUTUBE_PLAYLIST_ID` is intentionally a non-secret local setting. Use a dedicated playlist such as `Second Brain Inbox`; the official YouTube API blocks Watch Later listing.

## Source Requirements

X requires an OAuth 2.0 user access token with `bookmark.read`, `tweet.read`, and `users.read`.

YouTube can read a public playlist with `YOUTUBE_API_KEY`; private playlist tests need `YOUTUBE_ACCESS_TOKEN`. Transcript extraction is attempted against the first playlist video or `YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID`.

Summaries are deliberately extractive in Phase 1 so every output stays tied to fetched source text.
