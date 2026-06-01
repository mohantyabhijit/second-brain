"use client";

import { type FormEvent, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { useSupabaseAuth } from "../model/useSupabaseAuth";
import { formatUsername, publicOwnerHandle } from "./authDisplay";

const appEntryPath = "insights";
const landingPreviewImage = `${process.env.NEXT_PUBLIC_BASE_PATH ?? ""}/second-brain-wall-final-chrome.png`;

export function SecondBrainLanding() {
  const auth = useSupabaseAuth();
  return (
    <LandingAuthPanel
      configured={auth.configured}
      currentUsername={auth.isAuthenticated ? auth.username : null}
      isLoading={auth.isLoading}
      onSignIn={auth.signIn}
    />
  );
}

type LandingAuthPanelProps = {
  configured: boolean;
  currentUsername: string | null;
  isLoading: boolean;
  onSignIn: (email: string, redirectPath?: string) => Promise<void>;
};

export function LandingAuthPanel({ configured, currentUsername, isLoading, onSignIn }: LandingAuthPanelProps) {
  const [email, setEmail] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const signedInUsername = formatUsername(currentUsername);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage(null);
    try {
      await onSignIn(email, appEntryPath);
      setEmail("");
      setMessage("Check your email for the sign-in link.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Sign-in failed.");
    }
  }

  return (
    <main className="landing-shell">
      <Image
        alt="Second Brain knowledge inbox preview"
        className="landing-media"
        fill
        priority
        sizes="100vw"
        src={landingPreviewImage}
      />
      <div className="landing-overlay" />
      <section className="landing-content" aria-labelledby="landing-title">
        <div className="landing-brand" aria-label="Second Brain">
          <span>SB</span>
          <strong>Second Brain</strong>
        </div>
        <div className="landing-copy">
          <span className="landing-kicker">Knowledge Inbox</span>
          <h1 id="landing-title">Second Brain</h1>
          <p>Source-grounded insights from saved X bookmarks, YouTube videos, transcripts, and daily digests.</p>
        </div>
        <div className="landing-actions">
          {signedInUsername ? <p className="landing-status">Signed in as {signedInUsername}</p> : null}
          <form className="landing-auth-form" onSubmit={submit}>
            <input
              aria-label="Email address"
              autoComplete="email"
              disabled={!configured || isLoading}
              onChange={(event) => setEmail(event.target.value)}
              placeholder="you@example.com"
              type="email"
              value={email}
            />
            <button disabled={!configured || isLoading} type="submit">
              Sign Up / Sign In
            </button>
          </form>
          {message ? <p className="landing-message" role="status">{message}</p> : null}
          {!configured ? <p className="landing-message">Supabase auth is not configured in this environment.</p> : null}
          <Link className="landing-secondary-action" href="/insights">
            Continue as {publicOwnerHandle}
          </Link>
        </div>
      </section>
    </main>
  );
}
