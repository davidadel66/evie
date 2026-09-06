import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { SemanticObjectInspection, SemanticObjectPage } from "../api/memory";
import { MemoryView } from "./Memory";

const metadata = {
  valid_at: "2026-09-01T16:00:00Z",
  as_known_at: "2026-09-02T16:00:00Z",
  selected_scope: "global",
  allowed_scopes: ["global"],
  scope_revisions: [{ scope_key: "global", revision: 4 }],
};
const owner = { entity_id: "owner-1", scope_key: "global", canonical_name: "David", entity_type: "person", anchor_kind: "owner" };
const entityPage: SemanticObjectPage = {
  metadata,
  objects: [{ object_kind: "entity", object_id: "owner-1", scope_key: "global", status: "active", entity: owner }],
};
const claim = {
  claim_id: "claim-1", scope_key: "global", subject_entity_id: "owner-1",
  predicate: { predicate_id: "predicate-1", token: "timezone_name", version: 1, label: "time zone", object_constraint: "text", cardinality: "one" },
  object: { literal: { kind: "text", value: "America/Detroit" } }, polarity: "affirmed",
  valid_time: { from: null, to: null }, created_operation_id: "operation-1", transaction_time: "2026-09-02T16:00:00Z",
};
const claimPage: SemanticObjectPage = {
  metadata,
  objects: [{ object_kind: "claim", object_id: "claim-1", scope_key: "global", status: "active", claim, subject: owner }],
  next_cursor: "opaque-cursor",
};
const detail: SemanticObjectInspection = {
  object_kind: "claim", object_id: "claim-1", scope: { scope_id: "scope-global", scope_key: "global", revision: 4 }, status: "active", claim,
  sources: [{ source: { source_link_id: "source-1", event_id: "event-1", session_id: "session-1", source_scope_key: "workspace:hidden", authority: "owner_statement", observed_at: "2026-09-02T16:00:00Z", evidence_sha256: "sha256:abc", evidence: "", eligibility: "eligible" }, lifecycle: [] }],
  lifecycle: [{ state: "active", operation_id: "operation-1", scope_revision: 4, transaction_time: "2026-09-02T16:00:00Z" }],
  operations: [{ operation_id: "operation-1", schema_version: 1, kind: "remember_literal_claim", source_event_id: "event-1", proposal_sha256: "sha256:proposal", effect_sha256: "sha256:effect", transaction_time: "2026-09-02T16:00:00Z", prior_revisions: [], resulting_revisions: [{ scope_key: "global", revision: 4 }] }],
  conflicts: [{ code: "opposite_polarity", predicate_token: "timezone_name", claim_ids: ["claim-1", "claim-2"] }],
  metadata,
};

describe("MemoryView", () => {
  it("renders a graph-first exact-scope Semantic Memory browser", () => {
    const html = renderToStaticMarkup(<MemoryView
      scopes={[
        { scope_id: "scope-global", scope_key: "global", revision: 4 },
        { scope_id: "scope-sibling", scope_key: "workspace:sibling", revision: 2, quarantined: true, quarantine_reason: "canonical replay mismatch" },
      ]}
      scopeKey="global" view="graph" recordKind="claim" atTime validAt="2026-09-01T12:00" asKnownAt="2026-09-02T12:00"
      entityPage={entityPage} claimPage={claimPage} detail={detail}
      onScope={() => undefined} onView={() => undefined} onRecordKind={() => undefined} onAtTime={() => undefined}
      onValidAt={() => undefined} onAsKnownAt={() => undefined} onRefresh={() => undefined}
      onNext={() => undefined} onInspect={() => undefined}
    />);
    for (const text of [
      "Knowledge Graph", "Semantic Memory", "Memory Scope", "Graph", "Records", "View at time", "Valid at",
      "Known by Evie at", "David", "America/Detroit", "time zone", "Exact scope", "revision 4",
    ]) expect(html).toContain(text);
    expect(html).toContain('aria-label="Claim: David time zone America/Detroit"');
    expect(html).not.toContain("Episodes");
    expect(html).not.toContain("Candidates");
    expect(html).not.toContain("textarea");
  });

  it("retains the accessible record fallback and exact provenance detail", () => {
    const html = renderToStaticMarkup(<MemoryView
      scopes={[{ scope_id: "scope-global", scope_key: "global", revision: 4 }]}
      scopeKey="global" view="records" recordKind="claim" atTime={false} validAt="" asKnownAt=""
      entityPage={entityPage} claimPage={claimPage} detail={detail}
      onScope={() => undefined} onView={() => undefined} onRecordKind={() => undefined} onAtTime={() => undefined}
      onValidAt={() => undefined} onAsKnownAt={() => undefined} onRefresh={() => undefined}
      onNext={() => undefined} onInspect={() => undefined}
    />);
    for (const text of ["Claims", "Next page", "Record detail", "Evidence", "Source episode", "event-1", "Lifecycle", "Conflicts", "Operation history"]) expect(html).toContain(text);
    expect(html).toContain('aria-pressed="true"');
  });
});
