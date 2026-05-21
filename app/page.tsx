"use client";

import { startTransition, useEffect, useMemo, useState } from "react";
import type { ReactNode } from "react";
import type { Decision, PhaseOneResult, SourceStatus, Summary, ValidationItem } from "@/lib/types";

type IconName = "key" | "x" | "youtube" | "check" | "run" | "link" | "alert" | "spark";

const initialResult: PhaseOneResult = {
  generatedAt: new Date().toISOString(),
  sourceStatus: {
    x: "needs_secrets",
    youtube: "needs_secrets",
    onecli: "needs_secrets"
  },
  xBookmarks: [],
  youtubeItems: [],
  summaries: [],
  validation: [
    {
      label: "X bookmark request",
      status: "untested",
      detail: "Run Phase 1 validation after adding credentials."
    },
    {
      label: "YouTube playlist check",
      status: "untested",
      detail: "Use a dedicated Second Brain Inbox playlist ID."
    },
    {
      label: "Transcript path",
      status: "untested",
      detail: "The app tests the first playlist video or YOUTUBE_TRANSCRIPT_TEST_VIDEO_ID."
    }
  ],
  blockers: ["OneCLI is installed, but this app has not been run with authenticated provider secrets yet."]
};

const statusLabels: Record<SourceStatus, string> = {
  ready: "Ready",
  partial: "Partial",
  blocked: "Blocked",
  needs_secrets: "Needs secrets"
};

const decisionLabels: Record<Decision, string> = {
  read_now: "Read now",
  later: "Later",
  skip: "Skip"
};

function Icon({ name }: { name: IconName }) {
  const paths: Record<IconName, ReactNode> = {
    key: (
      <>
        <circle cx="7.5" cy="12.5" r="3.5" />
        <path d="M11 12.5h8m-3 0v3m-3-3v2" />
      </>
    ),
    x: (
      <>
        <path d="M5 5l14 14M19 5L5 19" />
      </>
    ),
    youtube: (
      <>
        <rect x="3" y="6.5" width="18" height="11" rx="3" />
        <path d="M10.5 9.5l5 2.5-5 2.5z" />
      </>
    ),
    check: (
      <>
        <path d="M20 6L9 17l-5-5" />
      </>
    ),
    run: (
      <>
        <path d="M4 12a8 8 0 0 1 13.6-5.7" />
        <path d="M18 3v5h-5" />
        <path d="M20 12a8 8 0 0 1-13.6 5.7" />
        <path d="M6 21v-5h5" />
      </>
    ),
    link: (
      <>
        <path d="M10 13a5 5 0 0 0 7.1 0l2-2a5 5 0 0 0-7.1-7.1l-1.1 1.1" />
        <path d="M14 11a5 5 0 0 0-7.1 0l-2 2A5 5 0 0 0 12 20.1l1.1-1.1" />
      </>
    ),
    alert: (
      <>
        <path d="M12 4l9 16H3z" />
        <path d="M12 9v5" />
        <path d="M12 17h.01" />
      </>
    ),
    spark: (
      <>
        <path d="M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z" />
        <path d="M19 15l.8 2.2L22 18l-2.2.8L19 21l-.8-2.2L16 18l2.2-.8z" />
      </>
    )
  };

  return (
    <svg aria-hidden="true" className="icon-svg" viewBox="0 0 24 24">
      {paths[name]}
    </svg>
  );
}

function SourceBadge({ label, detail, status, icon }: { label: string; detail: string; status: SourceStatus; icon: IconName }) {
  return (
    <div className={`source-badge ${status}`}>
      <div className="source-icon">
        <Icon name={icon} />
      </div>
      <div>
        <span>{label}</span>
        <strong>{statusLabels[status]}</strong>
        <small>{detail}</small>
      </div>
    </div>
  );
}

function ValidationRow({ item }: { item: ValidationItem }) {
  const icon = item.status === "pass" ? "check" : "alert";
  return (
    <li className={`validation-row ${item.status}`}>
      <span className="validation-icon">
        <Icon name={icon} />
      </span>
      <div>
        <strong>{item.label}</strong>
        <span>{item.detail}</span>
      </div>
    </li>
  );
}

function SummaryCard({ summary }: { summary: Summary }) {
  return (
    <article className="summary-card">
      <div className="summary-topline">
        <span className={`decision ${summary.decision}`}>{decisionLabels[summary.decision]}</span>
        <span>{summary.confidence} confidence</span>
      </div>
      <h3>{summary.title}</h3>
      <p>{summary.summary}</p>
      {summary.quote ? <blockquote>{summary.quote}</blockquote> : null}
      <a className="source-link" href={summary.sourceUrl} target="_blank" rel="noreferrer">
        <Icon name="link" />
        Source
      </a>
    </article>
  );
}

export default function Home() {
  const [result, setResult] = useState<PhaseOneResult>(() => initialResult);
  const [isRunning, setIsRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let ignore = false;
    fetch("/api/phase1")
      .then((response) => response.json())
      .then((payload: { latest: PhaseOneResult | null }) => {
        if (!ignore && payload.latest) {
          startTransition(() => setResult(payload.latest as PhaseOneResult));
        }
      })
      .catch(() => undefined);
    return () => {
      ignore = true;
    };
  }, []);

  async function runValidation() {
    setIsRunning(true);
    setError(null);
    try {
      const response = await fetch("/api/phase1", { method: "POST" });
      const payload = (await response.json()) as PhaseOneResult;
      if (!response.ok) throw new Error("Phase 1 validation failed.");
      startTransition(() => setResult(payload));
    } catch (phaseError) {
      setError(phaseError instanceof Error ? phaseError.message : "Phase 1 validation failed.");
    } finally {
      setIsRunning(false);
    }
  }

  const validationPassCount = useMemo(() => result.validation.filter((item) => item.status === "pass").length, [result.validation]);
  const totalFetched = result.xBookmarks.length + result.youtubeItems.length;
  const transcriptCount = result.youtubeItems.filter((item) => item.transcriptStatus === "available").length;
  const progress = result.validation.length ? Math.round((validationPassCount / result.validation.length) * 100) : 0;
  const latestDate = useMemo(
    () =>
      new Intl.DateTimeFormat("en", {
        dateStyle: "medium",
        timeStyle: "short"
      }).format(new Date(result.generatedAt)),
    [result.generatedAt]
  );

  return (
    <main className="app-shell">
      <aside className="sidebar" aria-label="Product navigation">
        <div className="brand-lockup">
          <div className="brand-mark">SB</div>
          <div>
            <strong>Second Brain</strong>
            <span>Phase 1 MVP</span>
          </div>
        </div>
        <nav>
          <a href="#sources">Sources</a>
          <a href="#ingestion">Ingestion</a>
          <a href="#summaries">Summaries</a>
          <a href="#validation">Validation</a>
        </nav>
        <div className="sidebar-note">
          <span>Scope</span>
          <strong>X + YouTube proof path</strong>
        </div>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <span className="section-label">Research inbox operator console</span>
            <h1>Second Brain Research Agent</h1>
            <p>Phase 1 console for proving real ingestion, transcript extraction, and source-grounded summaries.</p>
          </div>
          <button className="primary-action" type="button" onClick={runValidation} disabled={isRunning}>
            <Icon name="run" />
            {isRunning ? "Running" : "Run Phase 1"}
          </button>
        </header>

        <section id="sources" className="status-strip" aria-label="Source status">
          <SourceBadge label="OneCLI Secrets" detail="Credential vault" status={result.sourceStatus.onecli} icon="key" />
          <SourceBadge label="X Bookmarks" detail="5 recent saves" status={result.sourceStatus.x} icon="x" />
          <SourceBadge label="YouTube" detail="Playlist + captions" status={result.sourceStatus.youtube} icon="youtube" />
          <div className="readiness-card">
            <div>
              <span>{validationPassCount}/{result.validation.length} checks passing</span>
              <strong>{progress}% ready</strong>
            </div>
            <div className="progress-track" aria-label={`${progress}% ready`}>
              <span style={{ width: `${progress}%` }} />
            </div>
            <small>{latestDate}</small>
          </div>
        </section>

        <section className="metrics-row" aria-label="Phase 1 metrics">
          <div>
            <span>Fetched items</span>
            <strong>{totalFetched}</strong>
          </div>
          <div>
            <span>Summaries</span>
            <strong>{result.summaries.length}</strong>
          </div>
          <div>
            <span>Transcripts</span>
            <strong>{transcriptCount}</strong>
          </div>
          <div>
            <span>Open blockers</span>
            <strong>{result.blockers.length}</strong>
          </div>
        </section>

        {error ? <div className="error-banner">{error}</div> : null}

        <div className="main-grid">
          <section id="ingestion" className="panel wide">
            <div className="panel-header">
              <div>
                <h2>Recent Ingestion</h2>
                <p>Fetches 5 X bookmarks and a dedicated YouTube inbox playlist sample.</p>
              </div>
              <span className="panel-symbol">
                <Icon name="spark" />
              </span>
            </div>

            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>Source</th>
                    <th>Item</th>
                    <th>Author</th>
                    <th>Status</th>
                    <th>Link</th>
                  </tr>
                </thead>
                <tbody>
                  {result.xBookmarks.map((bookmark) => (
                    <tr key={bookmark.id}>
                      <td>X</td>
                      <td>{bookmark.text}</td>
                      <td>{bookmark.username ? `@${bookmark.username}` : bookmark.authorName ?? bookmark.authorId ?? "Unknown"}</td>
                      <td>Fetched</td>
                      <td>
                        <a href={bookmark.sourceUrl} target="_blank" rel="noreferrer">
                          Open
                        </a>
                      </td>
                    </tr>
                  ))}
                  {result.youtubeItems.map((item) => (
                    <tr key={item.videoId}>
                      <td>YouTube</td>
                      <td>{item.title}</td>
                      <td>{item.channelTitle ?? "Unknown"}</td>
                      <td>{item.transcriptStatus}</td>
                      <td>
                        <a href={item.sourceUrl} target="_blank" rel="noreferrer">
                          Open
                        </a>
                      </td>
                    </tr>
                  ))}
                  {result.xBookmarks.length + result.youtubeItems.length === 0 ? (
                    <tr>
                      <td colSpan={5}>
                        <div className="empty-inline">
                          <strong>No source rows yet</strong>
                          <span>Add OneCLI secrets and a playlist ID, then run validation.</span>
                        </div>
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          </section>

          <aside id="validation" className="panel">
            <div className="panel-header">
              <div>
                <h2>Validation</h2>
                <p>Phase 1 acceptance checks from the plan.</p>
              </div>
              <span className="panel-symbol">
                <Icon name="check" />
              </span>
            </div>
            <ul className="validation-list">
              {result.validation.map((item) => (
                <ValidationRow item={item} key={item.label} />
              ))}
            </ul>
          </aside>

          <section id="summaries" className="panel summaries">
            <div className="panel-header">
              <div>
                <h2>Summary Review</h2>
                <p>Short backlog decisions with attribution preserved.</p>
              </div>
              <span className="panel-symbol">
                <Icon name="link" />
              </span>
            </div>
            <div className="summary-grid">
              {result.summaries.length ? (
                result.summaries.map((summary) => <SummaryCard summary={summary} key={`${summary.source}-${summary.id}`} />)
              ) : (
                <div className="empty-panel">
                  <Icon name="spark" />
                  <strong>No summaries yet</strong>
                  <p>The extractive summarizer will run once source text is available.</p>
                </div>
              )}
            </div>
          </section>

          <aside className="panel transcript-panel">
            <div className="panel-header">
              <div>
                <h2>Transcript Status</h2>
                <p>Proves at least one real caption path.</p>
              </div>
              <span className="panel-symbol">
                <Icon name="youtube" />
              </span>
            </div>
            {result.youtubeItems.length ? (
              result.youtubeItems.slice(0, 3).map((item) => (
                <article className="transcript-item" key={item.videoId}>
                  <strong>{item.title}</strong>
                  <span>{item.transcriptStatus}</span>
                  <p>{item.transcriptPreview ?? item.transcriptError ?? "Not tested in this run."}</p>
                </article>
              ))
            ) : (
              <div className="empty-panel compact">
                <Icon name="youtube" />
                <strong>Waiting for playlist</strong>
                <p>Set <code>YOUTUBE_PLAYLIST_ID</code> to a Second Brain Inbox playlist and run the check.</p>
              </div>
            )}
          </aside>
        </div>

        {result.blockers.length ? (
          <section className="blockers" aria-label="Current blockers">
            <div>
              <span className="section-label">Setup required</span>
              <h2>Current blockers</h2>
            </div>
            {result.blockers.map((blocker) => (
              <p key={blocker}>{blocker}</p>
            ))}
          </section>
        ) : null}
      </section>
    </main>
  );
}
