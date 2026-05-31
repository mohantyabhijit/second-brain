"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Session } from "@supabase/supabase-js";
import { isSupabaseConfigured, tryCreateClient } from "../../../utils/supabase/client";

export type SupabaseAuthState = {
  configured: boolean;
  isLoading: boolean;
  isAuthenticated: boolean;
  email: string | null;
  authVersion: number;
  signIn: (email: string) => Promise<void>;
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

  const signIn = useCallback(async (email: string) => {
    const supabase = tryCreateClient();
    if (!supabase) {
      throw new Error("Supabase auth is not configured.");
    }
    const trimmed = email.trim();
    if (!trimmed) {
      throw new Error("Enter an email address.");
    }
    const redirectTo = typeof window !== "undefined" ? window.location.href.split("#")[0] : undefined;
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
      authVersion,
      signIn,
      signOut
    }),
    [authVersion, isLoading, session, signIn, signOut]
  );
}
