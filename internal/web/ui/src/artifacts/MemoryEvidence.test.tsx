import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { MemoryEvidenceView } from "./MemoryEvidence";
import type { MemoryEvidenceReceipt } from "../api/memoryEvidence";

it("shows exact supplied evidence and attribution separately from current state", () => {
  const receipt: MemoryEvidenceReceipt = { sessionId: "session-1", snapshotId: "request-1", version: "retrieval-v1", status: "success", evidence: [{ reference: { id: "claim-1", kind: "accepted_memory", scope_key: "global", status: "active", as_known_at: "2026-09-10T10:00:00Z", valid_at: "2026-09-10T10:00:00Z", paths: ["exact"], sources: [] }, available: true, current_status: "retired", evidence: { text: "timezone: Detroit", sources: [{ event_id: "event-1", session_id: "source-session", source_scope_key: "global", authority: "owner_statement", observed_at: "2026-09-09T10:00:00Z", evidence: "I live in Detroit.", locator_kind: "whole", locator_value: "", evidence_sha256: "recorded-hash" }] } }] };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  for (const value of ["Accepted memory", "Supplied to model", "timezone: Detroit", "I live in Detroit.", "owner_statement", "event-1", "retired"]) expect(html).toContain(value);
  expect(html).not.toContain("Cited by answer");
});

it("never renders retained source text after current access is denied", () => {
  const receipt: MemoryEvidenceReceipt = { sessionId: "session-1", snapshotId: "request-1", version: "retrieval-v1", status: "success", evidence: [{ reference: { id: "claim-1", kind: "accepted_memory", scope_key: "global", status: "active", as_known_at: "2026-09-10T10:00:00Z", valid_at: "2026-09-10T10:00:00Z", paths: [], sources: [] }, available: false, current_status: "active", evidence: { text: "restricted-owner-fact", sources: [] } }] };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  expect(html).toContain("Source unavailable under current access.");
  expect(html).not.toContain("restricted-owner-fact");
});

it("preserves a conversation excerpt's speaker, uncertainty and exact source range without inventing a Claim", () => {
  const receipt: MemoryEvidenceReceipt = { sessionId: "reader", snapshotId: "request-2", version: "retrieval-v1", status: "success", evidence: [{ reference: { id: "excerpt:source:7:35", kind: "conversation_excerpt", scope_key: "workspace:gardening", status: "active", as_known_at: "2026-09-10T10:00:00Z", valid_at: "2026-09-10T10:00:00Z", paths: ["conversation_fts"], sources: [] }, available: true, current_status: "active", evidence: { text: "She might like café plants.", sources: [{ event_id: "source-event", session_id: "earlier-conversation", source_scope_key: "workspace:gardening", actor: "assistant", authority: "none", observed_at: "2026-09-09T10:00:00Z", evidence: "She might like café plants.", locator_kind: "utf8_byte_range", locator_value: "7:35", evidence_sha256: "9520142f1cd104327a97b8a51c6e075d98d2cdcc6fb63dc234ab1faf9b218370" }] } }] };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  for (const value of ["Conversation excerpt", "She might like café plants.", "Speaker: assistant", "Authority: none", "earlier-conversation", "utf8_byte_range 7:35", "9520142f1cd104327a97b8a51c6e075d98d2cdcc6fb63dc234ab1faf9b218370"]) expect(html).toContain(value);
  expect(html).toContain("Attributed conversation evidence");
  expect(html).not.toContain("Accepted memory");
  expect(html).not.toContain("Claim ID");
});

it("shows a historical retired Claim with actual acceptance time and unknown validity rather than treating read filters as fact dates", () => {
  const receipt = {
    sessionId: "reader", snapshotId: "historical-request", version: "retrieval-v1", status: "success",
    evidence: [{
      reference: { id: "claim-boston", claim_id: "claim-boston", kind: "accepted_memory", scope_key: "global", status: "active", current_status: "retired", intent: "historical", valid_at_constrained: true, as_known_at: "2025-05-01T00:00:00Z", valid_at: "2024-01-01T00:00:00Z", paths: ["exact"], sources: [] },
      available: true, current_status: "retired",
      evidence: { text: "home: Boston", claim: { transaction_time: "2023-08-04T12:00:00Z", valid_time: { from: null, to: null } }, effective_valid_time: { from: null, to: null }, sources: [] },
    }],
  };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  for (const value of ["Historical", "Retired now", "Accepted on: 2023-08-04T12:00:00Z", "Valid from: Unknown", "Valid until: Unknown", "Read filters", "Known by: 2025-05-01T00:00:00Z", "Valid at: 2024-01-01T00:00:00Z"]) expect(html).toContain(value);
  expect(html).not.toContain("Accepted on: 2025");
  expect(html).not.toContain("Valid from: 2024");
});

it("labels a restored historical source as retired at retrieval while keeping its current active state separate", () => {
  const receipt = {
    sessionId: "reader", snapshotId: "restored-request", version: "retrieval-v1", status: "success",
    evidence: [{
      reference: { id: "retired-claim", kind: "accepted_memory", scope_key: "global", status: "retired", current_status: "retired", intent: "historical", as_known_at: "2025-05-01T00:00:00Z", valid_at: "2025-05-01T00:00:00Z", paths: ["exact"], sources: [] },
      available: true, current_status: "active", evidence: { text: "home: Boston", sources: [] },
    }],
  };
  const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
  expect(html).toContain("Retired at retrieval");
  expect(html).toContain("Original state: retired");
  expect(html).toContain("Current state: active");
  expect(html).not.toContain("Retired now");
});

it("distinguishes correcting an error from a later real-world change and preserves original versus current correction state", () => {
  for (const correction of [
    { original: "error", atRetrieval: "error", current: "error", until: "2024-06-01T00:00:00Z", expected: "Original correction: Corrected as an error" },
    { original: "changed", atRetrieval: "changed", current: "changed", until: "2024-06-01T00:00:00Z", expected: "Original correction: Changed over time" },
    { original: "", atRetrieval: "", current: "changed", until: null, expected: "Current correction: Changed over time" },
  ]) {
    const receipt = {
      sessionId: "reader", snapshotId: "corrected-request", version: "retrieval-v1", status: "success",
      evidence: [{
        reference: { id: "claim-boston", kind: "accepted_memory", scope_key: "global", status: correction.original ? "superseded" : "active", current_status: correction.original ? "superseded" : "active", intent: "historical", as_known_at: correction.original ? "2025-09-01T00:00:00Z" : "2023-09-01T00:00:00Z", valid_at: "2023-09-01T00:00:00Z", paths: ["exact"], sources: [], correction_mode: correction.original, current_correction_mode: correction.atRetrieval },
        available: true, current_status: "superseded",
        evidence: { text: "home: Boston", claim: { transaction_time: "2023-08-04T12:00:00Z", valid_time: { from: "2020-04-01T00:00:00Z", to: null } }, effective_valid_time: { from: "2020-04-01T00:00:00Z", to: correction.until }, correction_mode: correction.original, current_correction_mode: correction.current, sources: [] },
      }],
    };
    const html = renderToStaticMarkup(<MemoryEvidenceView receipt={receipt} />);
    expect(html).toContain(correction.expected);
    expect(html).toContain(`Valid until: ${correction.until ?? "Unknown"}`);
    expect(html).toContain(`Original state: ${correction.original ? "superseded" : "active"}`);
    if (!correction.original) expect(html).toContain("Current state: superseded");
    if (correction.current === "changed") expect(html).not.toContain("Corrected as an error");
  }
});

it("shows explicit Kernel conflicts while labeling newer owner wording only as a potential discrepancy", () => {
  const conflict = { code: "one_cardinality_overlap", predicate_token: "home", claim_ids: ["claim-boston", "claim-denver"] };
  const accepted = {
    sessionId: "reader", snapshotId: "conflict-request", version: "retrieval-v1", status: "success",
    evidence: ["Boston", "Denver"].map((city) => ({
      reference: { id: `claim-${city.toLowerCase()}`, kind: "accepted_memory", scope_key: "global", status: "active", as_known_at: "2026-09-10T00:00:00Z", valid_at: "2026-09-10T00:00:00Z", paths: ["exact"], sources: [], conflicts: [conflict] },
      available: true, current_status: "active", evidence: { text: `home: ${city}`, sources: [], conflicts: [conflict] },
    })),
  };
  const conflictHtml = renderToStaticMarkup(<MemoryEvidenceView receipt={accepted} />);
  for (const text of ["Conflicting accepted Claims", "home", "claim-boston", "claim-denver", "Different accepted values"]) expect(conflictHtml).toContain(text);
  const newer = {
    sessionId: "reader", snapshotId: "newer-request", version: "retrieval-v1", status: "success",
    evidence: [{
      reference: { id: "excerpt-chicago", kind: "conversation_excerpt", scope_key: "global", status: "active", as_known_at: "2026-09-10T00:00:00Z", valid_at: "2026-09-10T00:00:00Z", paths: ["newer_owner_statement"], sources: [], related_claim_ids: ["claim-boston"] },
      available: true, current_status: "active", evidence: { text: "I may move to Chicago; my sister said she likes it.", related_claim_ids: ["claim-boston"], sources: [{ event_id: "chicago-source", session_id: "earlier-conversation", source_scope_key: "global", actor: "owner", authority: "owner_statement", observed_at: "2026-08-20T10:00:00Z", evidence: "I may move to Chicago; my sister said she likes it.", locator_kind: "utf8_byte_range", locator_value: "0:51", evidence_sha256: "source-hash" }] },
    }],
  };
  const newerHtml = renderToStaticMarkup(<MemoryEvidenceView receipt={newer} />);
  for (const text of ["Potential discrepancy", "claim-boston", "I may move to Chicago; my sister said she likes it.", "2026-08-20T10:00:00Z", "Authority: owner_statement"]) expect(newerHtml).toContain(text);
  expect(newerHtml).not.toContain("Conflicting accepted Claims");
  expect(newerHtml).not.toContain("Accepted on:");
  expect(newerHtml).not.toContain("Valid from:");
  const ordinary = { ...newer, evidence: newer.evidence.map((item) => ({ ...item, reference: { ...item.reference, paths: ["conversation_lexical"], related_claim_ids: [] }, evidence: { ...item.evidence, related_claim_ids: [] } })) };
  expect(renderToStaticMarkup(<MemoryEvidenceView receipt={ordinary} />)).not.toContain("Potential discrepancy");
});
