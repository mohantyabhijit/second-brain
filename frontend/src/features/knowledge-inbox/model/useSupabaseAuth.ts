import type { User } from "@supabase/supabase-js";

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
