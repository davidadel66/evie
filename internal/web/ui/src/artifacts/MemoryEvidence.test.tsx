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
