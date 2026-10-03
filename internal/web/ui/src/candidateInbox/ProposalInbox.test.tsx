import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { MemoryProposalItem } from "../api/memoryProposals";
import { MemoryPresentationProvider } from "../memory/presentation";
import { Sidebar } from "../shell/Sidebar";
import { ProposalInboxView, type ProposalInboxState } from "./ProposalInbox";
import { ReviewBadge } from "./MemoryReviewTabs";
import { pendingLabel } from "./pendingProposals";

const proposal = JSON.stringify({
  kind: "remember_literal_claim", scope: { scope_key: "global" }, subject: { canonical_name: "owner", anchor_kind: "owner" },
  predicate: { label: "answer style" }, literal: { value: "short answers" },
  source: { authority: "owner_statement", evidence: "Keep it short, I hate long answers." },
  classification: { kind: "preference", standing: true, bytes_per_token_milli: 3000 },
});

const item: MemoryProposalItem = {
  proposal_id: "proposal-1", session_id: "session-1", session_title: "Weekend plans", scope_key: "global", tool_name: "memory_remember_literal",
  reason: "not_requested", queued_at: "2026-10-03T09:00:00Z", revision: 1, proposal, prepared_sha256: "sha256:prepared", approvable: true,
};

const state: ProposalInboxState = { page: { items: [item], pending: 1 }, busy: false, problem: "", notice: "", alwaysApplies: {} };

function render(value: ProposalInboxState) {
  return renderToStaticMarkup(<MemoryPresentationProvider><ProposalInboxView state={value} onAlwaysApplies={() => undefined}
    onApprove={() => undefined} onDecline={() => undefined} onRefresh={() => undefined} /></MemoryPresentationProvider>);
}

describe("memory proposal inbox", () => {
  it("shows Evie's proposal with its kind, scope, the owner's words and the Always applies switch", () => {
    const html = render(state);
    for (const text of ["Proposed by Evie", "Nothing is saved until you approve it", "From Weekend plans", "You · answer style · short answers",
      "Applies to: Everywhere", "Your words", "Keep it short, I hate long answers.", "Always applies", "Preference",
      "Evie sees this memory on every request.", "Approve", "Decline"]) expect(html).toContain(text);
    expect(html).toMatch(/role="switch" aria-checked="true"/);
    expect(html).not.toMatch(/disabled=""[^>]*>Approve/);
  });

  it("shows the owner's switch choice instead of the default", () => {
    const html = render({ ...state, alwaysApplies: { "proposal-1:1": false } });
    expect(html).toMatch(/role="switch" aria-checked="false"/);
    expect(html).toContain("Evie recalls this memory only when it’s relevant.");
  });

  it("explains a proposal held because the owner did not ask, and a refreshed one", () => {
    const html = render({ ...state, page: { items: [{ ...item, reason: "no_explicit_cue", revision: 2 }], pending: 1 } });
    expect(html).toContain("your message didn’t ask her to remember anything");
    expect(html).toContain("Updated against newer memory since Evie proposed it");
  });

  it("blocks approval of a proposal that can no longer apply but still allows declining", () => {
    const html = render({ ...state, page: { items: [{ ...item, approvable: false, problem: "its conversation is archived; restore the conversation to approve it" }], pending: 1 } });
    expect(html).toContain("This can’t be approved now: its conversation is archived");
    expect(html).toMatch(/disabled=""[^>]*>Approve/);
    expect(html).not.toMatch(/disabled=""[^>]*>Decline/);
  });

  it("shows memory full and other refusals, and hides itself when nothing waits", () => {
    const html = render({ ...state, problem: "memory full: standing memory for the profile holds 480 of 500 tokens" });
    expect(html).toContain('role="alert"');
    expect(html).toContain("memory full");
    expect(render({ ...state, page: { items: [], pending: 0 } })).toBe("");
  });

  it("counts waiting proposals with a badge on the Data navigation", () => {
    expect(renderToStaticMarkup(<ReviewBadge pending={0} />)).toBe("");
    expect(renderToStaticMarkup(<ReviewBadge pending={3} />)).toContain(`aria-label="${pendingLabel(3)}"`);
    const sidebar = renderToStaticMarkup(<Sidebar destination="chat" busy={false} mobileOpen={false} memoryReviewPending={2} onSettings={() => undefined}
      onCreateWorkspace={() => undefined} onArchive={() => undefined} onCloseMobile={() => undefined} onData={() => undefined}
      onWorkspace={() => undefined} onNewWorkspaceChat={() => undefined} onSession={() => undefined} />);
    expect(sidebar).toContain("2 memory proposals await review");
  });
});
