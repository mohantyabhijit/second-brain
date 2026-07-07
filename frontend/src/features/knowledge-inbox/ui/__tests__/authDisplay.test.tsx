import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { initialKnowledgeRun } from "../../model/initialKnowledgeRun";
import { toKnowledgeInboxViewModel } from "../../presentation/viewModel";
import { IdentityBadge, SecondBrainConsoleView } from "../SecondBrainConsoleView";

describe("auth display surfaces", () => {
  it("renders the fixed public owner in the console topbar", () => {
    const html = renderToStaticMarkup(<IdentityBadge />);

    expect(html).toContain("@abhijitmohanty");
    expect(html).not.toContain("Sign In");
    expect(html).not.toContain("Sign Out");
    expect(html).not.toContain("<button");
    expect(html).not.toContain("<input");
  });

  it("keeps the console read-only without Supabase or onboarding actions", () => {
    const html = renderToStaticMarkup(
      <SecondBrainConsoleView
        activePage="daily-newsletter"
        digestIssues={[]}
        hasMorePageItems={false}
        insightGraph={null}
        isLoading={false}
        isLoadingMore={false}
        model={toKnowledgeInboxViewModel(initialKnowledgeRun, false, null)}
        onLoadMoreItems={() => undefined}
        refreshStatus={null}
      />
    );

    expect(html).not.toContain("Sign Up / Sign In");
    expect(html).not.toContain("Sign in with Supabase");
    expect(html).not.toContain("Connect X");
    expect(html).not.toContain("Save Playlist");
    expect(html).not.toContain("Ask Your Second Brain");
    expect(html).not.toContain("Generate Digest");
    expect(html).not.toContain("Send Latest");
    expect(html).not.toContain("Digest recipient email");
  });
});
