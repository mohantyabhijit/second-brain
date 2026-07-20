import { useCallback, useEffect, useMemo, useState } from "react";
import type { Provider, User } from "@supabase/supabase-js";
import { isSupabaseConfigured, tryCreateClient } from "../../../utils/supabase/client";

export type SupabaseOAuthProvider = Extract<Provider, "apple" | "google">;

type AuthUser = {
  displayName: string | null;
  email: string | null;
};

type AuthState = {
  error: string | null;
  isConfigured: boolean;
  isLoading: boolean;
  pendingProvider: SupabaseOAuthProvider | null;
  user: AuthUser | null;
};

const oauthProviders = ["google", "apple"] as const satisfies readonly SupabaseOAuthProvider[];

export function useSupabaseAuth() {
  const supabase = useMemo(() => tryCreateClient(), []);
  const [state, setState] = useState<AuthState>({
    error: null,
    isConfigured: isSupabaseConfigured,
    isLoading: Boolean(supabase),
    pendingProvider: null,
    user: null
  });

  useEffect(() => {
    if (!supabase) {
      return;
    }

    let isMounted = true;

    supabase.auth
      .getSession()
      .then(({ data }) => {
        if (isMounted) {
          setState((current) => ({ ...current, isLoading: false, user: authUserFromSupabaseUser(data.session?.user) }));
        }
      })
      .catch(() => {
        if (isMounted) {
          setState((current) => ({ ...current, error: "Could not read the Supabase session.", isLoading: false }));
        }
      });

    const { data: listener } = supabase.auth.onAuthStateChange((_event, session) => {
      setState((current) => ({
        ...current,
        error: null,
        isLoading: false,
        pendingProvider: null,
        user: authUserFromSupabaseUser(session?.user)
      }));
    });

    return () => {
      isMounted = false;
      listener.subscription.unsubscribe();
    };
  }, [supabase]);

  const signInWithProvider = useCallback(
    async (provider: SupabaseOAuthProvider) => {
      if (!supabase) {
        setState((current) => ({ ...current, error: "Supabase auth is not configured for this environment." }));
        return;
      }

      setState((current) => ({ ...current, error: null, pendingProvider: provider }));
      const { error } = await supabase.auth.signInWithOAuth(createOAuthSignInOptions(provider, window.location.href));

      if (error) {
        setState((current) => ({ ...current, error: error.message, pendingProvider: null }));
      }
    },
    [supabase]
  );

  const signOut = useCallback(async () => {
    if (!supabase) {
      return;
    }
    setState((current) => ({ ...current, error: null, isLoading: true }));
    const { error } = await supabase.auth.signOut();
    setState((current) => ({
      ...current,
      error: error?.message ?? null,
      isLoading: false,
      user: error ? current.user : null
    }));
  }, [supabase]);

  return {
    ...state,
    providers: oauthProviders,
    signInWithProvider,
    signOut
  };
}

export function createOAuthSignInOptions(provider: SupabaseOAuthProvider, currentHref: string) {
  return {
    provider,
    options: {
      redirectTo: authRedirectUrl(undefined, currentHref)
    }
  };
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

function authUserFromSupabaseUser(user: User | null | undefined): AuthUser | null {
  if (!user) {
    return null;
  }
  return {
    displayName: usernameFromUser(user),
    email: user.email ?? null
  };
}
