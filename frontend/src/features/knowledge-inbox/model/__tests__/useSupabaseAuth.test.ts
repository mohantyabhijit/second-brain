import { describe, expect, it } from "vitest";
import { authRedirectUrl, usernameFromEmail, usernameFromUser } from "../useSupabaseAuth";

describe("Supabase auth identity helpers", () => {
  it("derives a display username without exposing the full email address", () => {
    expect(usernameFromEmail("abhijit@example.com")).toBe("abhijit");
    expect(usernameFromUser({ email: "ada@example.com", user_metadata: { preferred_username: "ada_lovelace" } })).toBe("ada_lovelace");
  });

  it("keeps magic-link redirects inside the deployed base path", () => {
    expect(authRedirectUrl("insights", "https://abhijitmohanty.com/second-brain/#access_token=abc")).toBe(
      "https://abhijitmohanty.com/second-brain/insights"
    );
    expect(authRedirectUrl("insights", "http://localhost:3000/")).toBe("http://localhost:3000/insights");
  });
});
