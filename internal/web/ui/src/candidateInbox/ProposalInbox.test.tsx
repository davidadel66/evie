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
  it("puts only the memory on the face, and where it came from on the back", () => {
    const html = render(state);
    const [front, back] = html.split("[transform:rotateY(180deg)]");
    for (const text of ["1 to review", ">answer style<", ">short answers<", ">Everywhere<", ">Preference<", "Always applies"]) expect(front).toContain(text);
    for (const gone of ["Keep it short", "Evie’s wording", "Exact change"]) expect(front).not.toContain(gone);
    for (const text of ["Weekend plans", "Your words", "“Keep it short, I hate long answers.”", "Exact change"]) expect(back).toContain(text);
    expect(html).toMatch(/role="switch" aria-checked="true" aria-label="Always applies"/);
    for (const label of ["Decline", "Save", "Show where it came from"]) expect(html).toContain(`aria-label="${label}"`);
    expect(html).not.toMatch(/aria-label="Save"[^>]*disabled=""/);
    expect(html).not.toContain('aria-label="Skip"');
  });

  it("shows the owner's switch choice instead of the default", () => {
    const html = render({ ...state, alwaysApplies: { "proposal-1:1": false } });
    expect(html).toMatch(/role="switch" aria-checked="false" aria-label="Always applies"/);
    expect(html).toContain("Evie recalls this only when it’s relevant.");
  });

  it("deals one card at a time from a stack, with chips by place and decline-all on the back", () => {
    const evie = (value: string, scope: string) => JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: scope }, predicate: { label: "monthly item" },
      literal: { value }, source: { authority: "evie_proposed", evidence: "" }, classification: { kind: "fact", standing: false } });
    const message = "add the cru sum every month these are tithes. also the credit cards are $100 each";
    const items: MemoryProposalItem[] = [
      { ...item, proposal_id: "a", reason: "background", scope_key: "workspace:fin", proposal: evie("CRU tithe", "workspace:fin"), source_message: message },
      { ...item, proposal_id: "b", reason: "background", scope_key: "workspace:fin", proposal: evie("$100 per credit card", "workspace:fin"), source_message: message },
      { ...item, proposal_id: "c", session_id: "session-2", session_title: "Other chat", source_message: "Keep it short, I hate long answers." },
    ];
    const html = renderToStaticMarkup(<MemoryPresentationProvider snapshot={{ workspaces: [{ id: "fin", displayName: "Finance", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "" }], projects: [], sessions: [] }}>
      <ProposalInboxView state={{ ...state, page: { items, pending: 3 } }} onAlwaysApplies={() => undefined} onApprove={() => undefined}
        onDecline={() => undefined} onDeclineAll={() => undefined} onRefresh={() => undefined} /></MemoryPresentationProvider>);
    expect(html.match(/aria-roledescription="card"/g)).toHaveLength(1);
    expect(html.match(/aria-hidden="true" class="border-hair bg-card absolute inset-0 rounded-2xl/g)).toHaveLength(2);
    for (const text of ["3 to review", ">All 3<", ">Finance 2<", ">Everywhere 1<", ">monthly item<", ">CRU tithe<", ">Finance<", ">Fact<", "Evie’s wording",
      message, "Decline all 2 from this message", 'aria-label="Skip"']) expect(html).toContain(text);
    expect(html).not.toContain("$100 per credit card");
    expect(html).toMatch(/aria-pressed="true"[^>]*>All 3</);
  });

  it("shows an update as the new value with the old one struck through", () => {
    const html = render({ ...state, page: { items: [{ ...item, reason: "background", tool_name: "memory_correct_claim", proposal: JSON.stringify({ kind: "correct_claim" }),
      change: { action: "update", current: "owner — home city: Boston", replacement: "owner — home city: Chicago", evidence: "I moved to Chicago last month." } }], pending: 1 } });
    for (const text of [">home city<", ">Chicago<", 'aria-label="Was">Boston<', ">Update<", "“I moved to Chicago last month.”"]) expect(html).toContain(text);
  });

  it("offers a Workspace memory everywhere when Evie suggests it, and shows the chosen version", () => {
    const own = JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: "workspace:fin" } });
    const global = JSON.stringify({ ...JSON.parse(proposal), scope: { scope_key: "global" }, operation_id: "global-version" });
    const offered: MemoryProposalItem = { ...item, reason: "background", scope_key: "workspace:fin", proposal: own,
      everywhere: { scope_key: "global", proposal: global, prepared_sha256: "sha256:global" } };
    const html = render({ ...state, page: { items: [offered], pending: 1 } });
    expect(html).toContain(">Workspace<");
    expect(html).toContain("Evie thinks this may apply beyond Workspace.");
    expect(html).toMatch(/role="switch" aria-checked="false" aria-label="Everywhere instead"/);
    expect(html).not.toContain("global-version");
    const chosen = render({ ...state, page: { items: [offered], pending: 1 }, everywhere: { "proposal-1:1": true } });
    expect(chosen).toContain(">Everywhere<");
    expect(chosen).toMatch(/role="switch" aria-checked="true" aria-label="Everywhere instead"/);
    expect(chosen).toContain("global-version");
    expect(render(state)).not.toContain("Everywhere instead");
  });

  it("keeps the whole message on the back", () => {
    const long = "word ".repeat(80).trim();
    expect(render({ ...state, page: { items: [{ ...item, source_message: long }], pending: 1 } })).toContain(long);
  });

  it("flags a proposal held because the owner did not ask, and a refreshed one", () => {
    const html = render({ ...state, page: { items: [{ ...item, reason: "no_explicit_cue", revision: 2 }], pending: 1 } });
    expect(html).toContain("You didn’t ask Evie to remember this.");
    expect(html).toContain("updated against newer memory");
  });

  it("blocks saving a proposal that can no longer apply but still allows declining", () => {
    const html = render({ ...state, page: { items: [{ ...item, approvable: false, problem: "its conversation is archived; restore the conversation to approve it" }], pending: 1 } });
    expect(html).toContain("Can’t save: its conversation is archived");
    expect(html).toMatch(/aria-label="Save"[^>]*disabled=""/);
    expect(html).not.toMatch(/aria-label="Decline"[^>]*disabled=""/);
  });

  it("shows refusals, and All caught up once nothing waits", () => {
    const html = render({ ...state, problem: "memory full: standing memory for the profile holds 480 of 500 tokens" });
    expect(html).toContain('role="alert"');
    expect(html).toContain("memory full");
    const empty = render({ ...state, page: { items: [], pending: 0 } });
    expect(empty).toContain("All caught up");
    expect(empty).not.toContain("to review");
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
