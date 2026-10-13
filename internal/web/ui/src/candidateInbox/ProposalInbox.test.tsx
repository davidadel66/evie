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
  it("shows the memory as one line with its kind, where it applies, the owner's words and the Always applies switch", () => {
    const html = render(state);
    for (const text of ["Proposed by Evie", "Nothing is saved until you approve it", "Weekend plans", "You — answer style: short answers",
      "Preference · applies everywhere", "Your words", "Keep it short, I hate long answers.", "Always applies",
      "Evie sees this on every request.", "Exact change", "Approve", "Decline"]) expect(html).toContain(text);
    expect(html).not.toContain("Evie’s wording");
    expect(html).toMatch(/role="switch" aria-checked="true"/);
    expect(html).not.toMatch(/disabled=""[^>]*>Approve/);
  });

  it("shows the owner's switch choice instead of the default", () => {
    const html = render({ ...state, alwaysApplies: { "proposal-1:1": false } });
    expect(html).toMatch(/role="switch" aria-checked="false"/);
    expect(html).toContain("Evie recalls this only when it’s relevant.");
  });

  it("groups proposals under the message Evie read and tags Evie's wording once explained", () => {
    const evie = (value: string, scope: string) => JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: scope }, predicate: { label: "monthly item" },
      literal: { value }, source: { authority: "evie_proposed", evidence: "" }, classification: { kind: "fact", standing: false } });
    const message = "add the cru sum every month these are tithes. also the credit cards are $100 each";
    const items: MemoryProposalItem[] = [
      { ...item, proposal_id: "a", reason: "background", scope_key: "workspace:fin", proposal: evie("CRU tithe", "workspace:fin"), source_message: message },
      { ...item, proposal_id: "b", reason: "background", scope_key: "workspace:fin", proposal: evie("$100 per credit card", "workspace:fin"), source_message: message },
      { ...item, proposal_id: "c", session_id: "session-2", session_title: "Other chat", source_message: "Keep it short, I hate long answers." },
    ];
    const html = render({ ...state, page: { items, pending: 3 } });
    expect(html.match(/aria-label="Proposals from one message"/g)).toHaveLength(2);
    expect(html.split(message)).toHaveLength(2);
    for (const text of ["From your message", "Found in your conversations", "You — monthly item: CRU tithe", "You — monthly item: $100 per credit card",
      "Fact · applies in Workspace", "means you didn’t say it in these words"]) expect(html).toContain(text);
    expect(html.match(/Evie’s wording/g)).toHaveLength(3);
  });

  it("offers a Workspace memory everywhere when Evie suggests it, and shows the chosen version", () => {
    const own = JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: "workspace:fin" } });
    const global = JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: "global" }, operation_id: "global-version" });
    const offered: MemoryProposalItem = { ...item, reason: "background", scope_key: "workspace:fin", proposal: own,
      everywhere: { scope_key: "global", proposal: global, prepared_sha256: "sha256:global" } };
    const html = render({ ...state, page: { items: [offered], pending: 1 } });
    for (const text of ["Preference · applies in Workspace", "Save everywhere instead", "Evie thinks this may apply beyond Workspace."]) expect(html).toContain(text);
    expect(html).toMatch(/role="switch" aria-checked="false" aria-label="Save everywhere instead"/);
    expect(html).not.toContain("global-version");
    const chosen = render({ ...state, page: { items: [offered], pending: 1 }, everywhere: { "proposal-1:1": true } });
    expect(chosen).toContain("Preference · applies everywhere");
    expect(chosen).toMatch(/role="switch" aria-checked="true" aria-label="Save everywhere instead"/);
    expect(chosen).toContain("global-version");
    expect(render(state)).not.toContain("Save everywhere instead");
  });

  it("folds a long source message behind Show all", () => {
    const html = render({ ...state, page: { items: [{ ...item, source_message: "word ".repeat(80) }], pending: 1 } });
    expect(html).toContain("line-clamp-3");
    expect(html).toContain("Show all");
    expect(render(state)).not.toContain("Show all");
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
