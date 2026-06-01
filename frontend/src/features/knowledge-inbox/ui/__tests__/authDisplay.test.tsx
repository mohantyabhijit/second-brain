import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { WorkspaceStatus } from "../../contracts";
import type { SupabaseAuthState } from "../../model/useSupabaseAuth";
import { IdentityBadge } from "../SecondBrainConsoleView";
import { LandingAuthPanel } from "../SecondBrainLanding";

describe("auth display surfaces", () => {
  it("renders only a username in the console topbar for signed-in users", () => {
    const html = renderToStaticMarkup(
      <IdentityBadge
        auth={authState({ email: "abhijit@example.com", isAuthenticated: true, username: "abhijit" })}
        workspace={workspaceStatus({ authenticated: true, handle: "" })}
      />
    );

    expect(html).toContain("@abhijit");
    expect(html).not.toContain("abhijit@example.com");
    expect(html).not.toContain("Sign In");
    expect(html).not.toContain("Sign Out");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<input");
  });

  it("keeps the landing page as the only sign-in surface and exposes the public continue button", () => {
    const html = renderToStaticMarkup(
      <LandingAuthPanel configured currentUsername={null} isLoading={false} onSignIn={vi.fn()} />
    );

    expect(html).toContain("Sign Up / Sign In");
    expect(html).toContain("Continue as abhijitmohanty");
    expect(html).toContain('href="/insights"');
  });
});

function authState(overrides: Partial<SupabaseAuthState> = {}): SupabaseAuthState {
  return {
    configured: true,
    isLoading: false,
    isAuthenticated: false,
    email: null,
    username: null,
    authVersion: 0,
    signIn: vi.fn(),
    signOut: vi.fn(),
    ...overrides
  };
}

function workspaceStatus(profile: Partial<WorkspaceStatus["profile"]> = {}): WorkspaceStatus {
  return {
    profile: {
      ownerId: "owner-1",
      handle: "abhijitmohanty",
      displayName: "Abhijit Mohanty",
      isPublicOwner: true,
      authenticated: false,
      ...profile
    },
    x: {
      configured: true,
      authorized: true
    },
    youtube: {
      configured: true
    },
    onboarding: {
      complete: true,
      missing: []
    }
  };
}
