import type { WorkspaceStatus } from "../contracts";
import type { SupabaseAuthState } from "../model/useSupabaseAuth";

export const publicOwnerHandle = "abhijitmohanty";

export function displayUsername(auth: Pick<SupabaseAuthState, "isAuthenticated" | "username" | "email">, workspace: WorkspaceStatus | null) {
  if (auth.isAuthenticated) {
    return formatUsername(workspace?.profile.handle || auth.username || usernameFromEmail(auth.email)) ?? "signed in";
  }
  return formatUsername(workspace?.profile.handle || publicOwnerHandle);
}

export function formatUsername(username: string | null | undefined) {
  const trimmed = username?.trim().replace(/^@+/, "");
  return trimmed ? `@${trimmed}` : null;
}

function usernameFromEmail(email: string | null | undefined) {
  const trimmed = email?.trim();
  if (!trimmed) {
    return null;
  }
  return trimmed.includes("@") ? trimmed.split("@")[0] : trimmed;
}
