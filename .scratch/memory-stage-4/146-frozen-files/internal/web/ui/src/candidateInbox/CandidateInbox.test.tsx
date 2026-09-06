import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CandidateInboxView } from "./CandidateInbox";
import { MemoryReviewTabs } from "./MemoryReviewTabs";
import type { InboxState } from "./controller";
import { candidate, preview } from "./fixtures.test-support";

const state: InboxState = { scope: "global", scopes: { scopes: [{ scope_key: "global", kind: "global", label: "Global memory" }], next_cursor: "scope-next", indexing: false }, busy: false, scopesBusy: false, problem: "", notice: "", reason: "", page: { scope_key: "global", revision: 7, candidates: [candidate], next_cursor: "next" }, detail: candidate, preview };
function render(value = state) { return renderToStaticMarkup(<CandidateInboxView state={value} onScope={() => undefined} onScopes={() => undefined} onLoad={() => undefined} onInspect={() => undefined} onPrepare={() => undefined} onResolve={() => undefined} onRecover={() => undefined} onReason={() => undefined} />); }

describe("candidate review disclosure", () => {
  it("shows exact source, authority, scope, generation, reviewed effect and intentional approval", () => {
    const html = render();
    for (const text of ["Global memory", "Review scope", "Exact scope: global", "inbox revision 7", "Next candidate page", "More scopes", "I prefer café.", "owner_statement", "Recorded.", "none", "closed-session", "generation-1", "event-1", "evidence-hash", "David", "drink", "affirmed", "Unknown", "Create claim", "Supporting Source Links", "source-2", "Accept this exact memory", "preview-hash", "effect-hash"]) expect(html).toContain(text);
    expect(html).toContain("not supporting evidence"); expect(html).not.toContain("textarea");
  });

  it("shows redacted evidence without exposing retained proposal text and permits only rejection", () => {
    const html = render({ ...state, page: undefined, preview: undefined, detail: { ...candidate, redacted: true } });
    expect(html).not.toContain("café"); expect(html).not.toContain("Recorded."); expect(html).toContain("Acceptance is blocked"); expect(html).toContain("Preview rejection");
    expect(html).toMatch(/disabled=""[^>]*>Preview acceptance/);
  });

  it("never offers acceptance of an undisclosed identity effect", () => {
    const advanced = { ...candidate, candidate: { ...candidate.candidate, proposal: { ...candidate.candidate.proposal, identity: { subject: null, object: null, predicate: null, uncertainty: "", confidence: null } } } };
    const html = render({ ...state, detail: advanced, preview: { ...preview, version: "owner-review-preview-v2" } });
    expect(html).toContain("Additional identity review"); expect(html).toMatch(/disabled=""[^>]*>Accept this exact memory/);
  });

  it("keeps the accepted memory view as the default and mounts suggestions separately", () => {
    const html = renderToStaticMarkup(<MemoryReviewTabs><p>Existing accepted graph</p></MemoryReviewTabs>);
    expect(html).toContain("Existing accepted graph"); expect(html).toContain("Review candidates"); expect(html).not.toContain("Candidate inbox");
  });

  it("makes retry, indexing and empty states actionable", () => {
    const html = render({ ...state, scopes: { ...state.scopes!, indexing: true }, page: { scope_key: "global", revision: 0, candidates: [], next_cursor: "" }, pending: { delivery_key: "idem:v1:retry", preview_id: "preview-1", preview_sha256: "preview-hash", action: "accept", reason: "" }, problem: "Response interrupted; retry the same decision." });
    expect(html).toContain("Continue loading older scopes"); expect(html).toContain("No unresolved candidates"); expect(html).toContain("Retry the same decision"); expect(html).toContain('role="alert"');
  });
});
