"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Session, User } from "@supabase/supabase-js";
import { isSupabaseConfigured, tryCreateClient } from "../../../utils/supabase/client";

export type SupabaseAuthState = {
  configured: boolean;
  isLoading: boolean;
  isAuthenticated: boolean;
  email: string | null;
  username: string | null;
  authVersion: number;
  signIn: (email: string, redirectPath?: string) => Promise<void>;
  signOut: () => Promise<void>;
};

export function useSupabaseAuth(): SupabaseAuthState {
  const [session, setSession] = useState<Session | null>(null);
  const [isLoading, setIsLoading] = useState(() => isSupabaseConfigured);
  const [authVersion, setAuthVersion] = useState(0);

  useEffect(() => {
    const supabase = tryCreateClient();
    if (!supabase) {
      return undefined;
    }

    let active = true;
    supabase.auth.getSession().then(({ data }) => {
      if (!active) return;
      setSession(data.session ?? null);
      setIsLoading(false);
    });

    const { data: listener } = supabase.auth.onAuthStateChange((_event, nextSession) => {
      setSession(nextSession);
      setAuthVersion((version) => version + 1);
      setIsLoading(false);
    });

    return () => {
      active = false;
      listener.subscription.unsubscribe();
    };
  }, []);

  const signIn = useCallback(async (email: string, redirectPath?: string) => {
    const supabase = tryCreateClient();
    if (!supabase) {
      throw new Error("Supabase auth is not configured.");
    }
    const trimmed = email.trim();
    if (!trimmed) {
      throw new Error("Enter an email address.");
    }
    const redirectTo = typeof window !== "undefined" ? authRedirectUrl(redirectPath, window.location.href) : undefined;
    const { error } = await supabase.auth.signInWithOtp({
      email: trimmed,
      options: {
        emailRedirectTo: redirectTo
      }
    });
    if (error) {
      throw error;
    }
  }, []);

  const signOut = useCallback(async () => {
    const supabase = tryCreateClient();
    if (!supabase) {
      return;
    }
    const { error } = await supabase.auth.signOut();
    if (error) {
      throw error;
    }
  }, []);

  return useMemo(
    () => ({
      configured: isSupabaseConfigured,
      isLoading,
      isAuthenticated: Boolean(session),
      email: session?.user.email ?? null,
      username: usernameFromUser(session?.user),
      authVersion,
      signIn,
      signOut
    }),
    [authVersion, isLoading, session, signIn, signOut]
  );
}

export function usernameFromUser(user: Pick<User, "email" | "user_metadata"> | null | undefined) {
  const metadata = user?.user_metadata as Record<string, unknown> | undefined;
  const metadataUsername = cleanUsername(metadata?.user_name) ?? cleanUsername(metadata?.preferred_username);
  return metadataUsername ?? usernameFromEmail(user?.email);
}

export function usernameFromEmail(email: string | null | undefined) {
  const trimmed = email?.trim();
  if (!trimmed) {
    return null;
  }
  return cleanUsername(trimmed.includes("@") ? trimmed.split("@")[0] : trimmed);
}

export function authRedirectUrl(redirectPath: string | undefined, currentHref: string) {
  const current = new URL(currentHref.split("#")[0]);
  if (!redirectPath) {
    return current.toString();
  }

  const normalizedPath = redirectPath.replace(/^\/+/, "");
  const basePath = current.pathname.endsWith("/") ? current.pathname : `${current.pathname}/`;
  current.pathname = `${basePath}${normalizedPath}`.replace(/\/{2,}/g, "/");
  current.search = "";
  return current.toString();
}

function cleanUsername(value: unknown) {
  if (typeof value !== "string") {
    return null;
  }
  const trimmed = value.trim().replace(/^@+/, "");
  return trimmed || null;
}
