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
