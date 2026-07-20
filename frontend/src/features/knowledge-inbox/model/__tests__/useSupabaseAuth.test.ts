import { describe, expect, it } from "vitest";
import { authRedirectUrl, createOAuthSignInOptions, usernameFromEmail, usernameFromUser } from "../useSupabaseAuth";

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

  it("builds Google and Apple OAuth redirects back to the active page", () => {
    expect(createOAuthSignInOptions("google", "https://abhijitmohanty.com/second-brain/insights?view=latest#code=abc")).toEqual({
      provider: "google",
      options: {
        redirectTo: "https://abhijitmohanty.com/second-brain/insights?view=latest"
      }
    });
    expect(createOAuthSignInOptions("apple", "http://localhost:3000/daily-newsletter")).toEqual({
      provider: "apple",
      options: {
        redirectTo: "http://localhost:3000/daily-newsletter"
      }
    });
  });
});
