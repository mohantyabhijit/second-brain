import { describe, expect, it } from "vitest";
import { usernameFromEmail, usernameFromUser } from "../useSupabaseAuth";

describe("Supabase auth identity helpers", () => {
  it("derives a display username without exposing the full email address", () => {
    expect(usernameFromEmail("abhijit@example.com")).toBe("abhijit");
    expect(usernameFromUser({ email: "ada@example.com", user_metadata: { preferred_username: "ada_lovelace" } })).toBe("ada_lovelace");
  });
});
