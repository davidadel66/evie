import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { SpendingAccountsSnapshot, SpendingLink } from "../api/spendingAccounts";
import { SpendingAccountsView } from "./SpendingAccounts";

const snapshot: SpendingAccountsSnapshot = {
  linkAvailable: true,
  institutions: [
    { id: "institution-private", name: "Everyday Bank", accountsKnown: true, accountsUpdatedAt: null, accounts: [
      { id: "account-private", name: "Everyday checking", mask: "1234", type: "depository", subtype: "checking" },
      { id: "credit-private", name: "Rewards card", mask: "987654321", type: "credit", subtype: "credit_card" },
    ] },
    { id: "legacy-private", name: "Older connection", accountsKnown: false, accounts: [] },
  ],
};
const link: SpendingLink = { id: "link-private", hostedUrl: "https://secure.plaid.com/link/test", expiresAt: "2030-09-21T18:00:00Z" };
const callbacks = { onReload: () => undefined, onRefresh: () => undefined, onAdd: () => undefined, onCancel: () => undefined };
function render(overrides: Partial<Parameters<typeof SpendingAccountsView>[0]> = {}) {
  return renderToStaticMarkup(<SpendingAccountsView snapshot={snapshot} loading={false} {...callbacks} {...overrides} />);
}

describe("Connected spending accounts", () => {
  it("shows every account grouped by institution with masked digits and type", () => {
    const html = render();
    for (const text of ["Connected accounts", "Everyday Bank", "Everyday checking", "Rewards card", "checking", "credit card", "•••• 1234", "•••• 4321", "Add account", "Refresh accounts"]) expect(html).toContain(text);
    for (const privateValue of ["institution-private", "account-private", "credit-private", "987654321"]) expect(html).not.toContain(privateValue);
    expect(html).not.toContain("Balance");
  });

  it("distinguishes unknown legacy inventory from a known empty result", () => {
    const html = render();
    expect(html).toContain("Older connection");
    expect(html).toContain("Account details have not been loaded. Choose Refresh accounts to load them.");
    expect(html).not.toContain("No accounts were returned");
    expect(render({ snapshot: { ...snapshot, institutions: [{ ...snapshot.institutions[1], accountsKnown: true }] } })).toContain("No accounts were returned for this institution.");
  });

  it("preserves cached rows and offers local reload after failures", () => {
    const html = render({ problem: "1 institution refreshed; 1 could not be refreshed. Saved details are kept." });
    expect(html).toContain('role="alert"');
    expect(html).toContain("Everyday checking");
    expect(html).toContain("Reload saved accounts");
    expect(render({ snapshot: undefined, loading: true })).toContain("Loading accounts…");
    expect(render({ snapshot: { institutions: [], linkAvailable: true } })).toContain("No connected accounts yet");
  });

  it("shows an actionable unavailable state without inventing credentials", () => {
    const html = render({ snapshot: { ...snapshot, linkAvailable: false } });
    expect(html).toContain("Add account is unavailable until Plaid is configured.");
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>[\s\S]*?Add account<\/button>/);
    expect(html).toContain("Everyday checking");
  });

  it("always provides a safe continue link for recovered or blocked popup flows", () => {
    const html = render({ link });
    expect(html).toContain('href="https://secure.plaid.com/link/test"');
    expect(html).toContain('target="_blank" rel="noopener noreferrer"');
    expect(html).toContain("Continue with Plaid");
    expect(html).toContain("Cancel connection");
    expect(html).not.toContain("link-private");
  });

  it("hides an expired Hosted Link while a server completion remains in progress", () => {
    const html = render({ link, hostedExpired: true });
    expect(html).toContain("Checking whether Plaid finished connecting…");
    expect(html).toContain("The Plaid page has expired.");
    expect(html).not.toContain("Continue with Plaid");
    expect(html).not.toContain('href="https://secure.plaid.com');
  });

  it("offers explicit status retry and saved-account recovery for uncertain outcomes", () => {
    expect(render({ link, pollStopped: true, linkProblem: "Connection status could not be checked." })).toContain("Check connection");
    const html = render({ notice: "The connection expired.", checkSavedAccounts: true });
    expect(html).toContain("Check saved accounts");
    expect(html).not.toContain("Continue with Plaid");
  });
});
