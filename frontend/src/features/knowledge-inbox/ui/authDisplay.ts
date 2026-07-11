export const publicOwnerHandle = "abhijitmohanty";

export function formatUsername(username: string | null | undefined) {
  const trimmed = username?.trim().replace(/^@+/, "");
  return trimmed ? `@${trimmed}` : null;
}
