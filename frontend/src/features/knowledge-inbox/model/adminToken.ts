export const adminTokenLocalStorageKey = "second-brain-admin-token";

export function readLocalAdminToken() {
  if (typeof window === "undefined") {
    return "";
  }
  try {
    return window.localStorage.getItem(adminTokenLocalStorageKey)?.trim() ?? "";
  } catch {
    return "";
  }
}

export function clearLocalAdminToken() {
  if (typeof window === "undefined") {
    return;
  }
  try {
    window.localStorage.removeItem(adminTokenLocalStorageKey);
  } catch {
    // Ignore localStorage access failures; the server will reject stale tokens.
  }
}
