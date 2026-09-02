import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { MemoryView } from "./Memory";

describe("MemoryView", () => {
  it("renders an accessible read-only one-scope browser with provenance and operation history", () => {
    const html = renderToStaticMarkup(<MemoryView
      scopes={[
        { scope_id: "scope-global", scope_key: "global", revision: 4 },
        { scope_id: "scope-sibling", scope_key: "workspace:sibling", revision: 2, quarantined: true, quarantine_reason: "canonical replay mismatch" },
      ]}
      scopeKey="global" kind="claim" history validAt="2026-09-01T12:00" asKnownAt="2026-09-02T12:00"
      page={{
        metadata: { valid_at: "2026-09-01T16:00:00Z", as_known_at: "2026-09-02T16:00:00Z", selected_scope: "global", allowed_scopes: ["global"], scope_revisions: [{ scope_key: "global", revision: 4 }] },
        objects: [{
          object_kind: "claim", object_id: "claim-1", scope_key: "global", status: "active",
          claim: { claim_id: "claim-1", scope_key: "global", subject_entity_id: "owner-1", predicate: { predicate_id: "predicate-1", token: "timezone_name", version: 1, label: "timezone name", object_constraint: "text", cardinality: "one" }, object: { literal: { kind: "text", value: "Detroit" } }, polarity: "affirmed", valid_time: { from: null, to: null }, created_operation_id: "operation-1", transaction_time: "2026-09-02T16:00:00Z" },
        }], next_cursor: "opaque-cursor",
      }}
      detail={{
        object_kind: "claim", object_id: "claim-1", scope: { scope_id: "scope-global", scope_key: "global", revision: 4 }, status: "active",
        claim: { claim_id: "claim-1", scope_key: "global", subject_entity_id: "owner-1", predicate: { predicate_id: "predicate-1", token: "timezone_name", version: 1, label: "timezone name", object_constraint: "text", cardinality: "one" }, object: { literal: { kind: "text", value: "Detroit" } }, polarity: "affirmed", valid_time: { from: null, to: null }, created_operation_id: "operation-1", transaction_time: "2026-09-02T16:00:00Z" },
        sources: [{ source: { source_link_id: "source-1", event_id: "event-1", session_id: "session-1", source_scope_key: "workspace:hidden", authority: "owner_statement", observed_at: "2026-09-02T16:00:00Z", evidence_sha256: "sha256:abc", evidence: "", eligibility: "eligible" }, lifecycle: [] }],
        lifecycle: [{ state: "active", operation_id: "operation-1", scope_revision: 4, transaction_time: "2026-09-02T16:00:00Z" }],
        operations: [{ operation_id: "operation-1", schema_version: 1, kind: "remember_literal_claim", source_event_id: "event-1", proposal_sha256: "sha256:proposal", effect_sha256: "sha256:effect", transaction_time: "2026-09-02T16:00:00Z", prior_revisions: [], resulting_revisions: [{ scope_key: "global", revision: 4 }] }],
        conflicts: [{ code: "opposite_polarity", predicate_token: "timezone_name", claim_ids: ["claim-1", "claim-2"] }],
        metadata: { valid_at: "2026-09-01T16:00:00Z", as_known_at: "2026-09-02T16:00:00Z", selected_scope: "global", allowed_scopes: ["global"], scope_revisions: [{ scope_key: "global", revision: 4 }] },
      }}
      onScope={() => undefined} onKind={() => undefined} onHistory={() => undefined}
      onValidAt={() => undefined} onAsKnownAt={() => undefined} onRefresh={() => undefined}
      onNext={() => undefined} onInspect={() => undefined}
    />);
    for (const text of [
      "Read-only exact inspection", "Memory scope", "Entities", "Claims", "Historical view", "Valid Time",
      "Transaction Time", "scope=global", "Next page", "Record detail", "Provenance", "Source text unavailable in this scope.",
      "Lifecycle", "Conflicts", "opposite_polarity", "Operation history", "schema v1", "sha256:proposal",
    ]) expect(html).toContain(text);
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).not.toContain("textarea");
    expect(html).not.toContain("contenteditable");
    expect(html).not.toContain("SELECT *");
  });
});
