import type { CandidateReviewAPI, OwnerCandidate, ReviewPreview, ReviewResult } from "../api/candidateReview";

export const candidate: OwnerCandidate = {
  ref: { candidate_id: "candidate-1", interpretation_revision: 0, review_revision: 0 }, job_id: "job-1", generation_id: "generation-1", destination: "global", redacted: false,
  candidate: { candidate_id: "candidate-1", review_state: "unresolved", review_revision: 0,
    proposal: { proposition: { subject_entity_id: "owner-1", predicate_id: "drink-1", object: { literal: { kind: "text", value: "café" } }, polarity: "affirmed" }, valid_time: { from: null, to: null }, temporal_qualification: "" },
    support: [{ locator: { event_id: "event-1", event_part: "content", locator_kind: "utf8_byte_range", locator_value: "0:15", evidence_sha256: "evidence-hash" }, session_id: "closed-session", scope_key: "global", observed_at: "2026-09-01T00:00:00Z", authority: "owner_statement", usage: "new_support", evidence: "I prefer café." }],
    context: [{ locator: { event_id: "event-2", event_part: "content", locator_kind: "whole_part", locator_value: "", evidence_sha256: "context-hash" }, session_id: "closed-session", scope_key: "global", observed_at: "2026-09-01T00:00:01Z", authority: "none", usage: "context", evidence: "Recorded." }],
  },
};
const predicate = { predicate_id: "drink-1", token: "drink", version: 1, label: "drink", object_constraint: "text", cardinality: "one" };
export const preview: ReviewPreview = {
  version: "owner-review-preview-v1", preview_id: "preview-1", preview_sha256: "preview-hash", effect_sha256: "effect-hash", scope_key: "global", action: "accept", candidates: [candidate], generation_id: "generation-1",
  effect: { version: "owner-review-effect-v1", operation_id: "operation-1", prior_revisions: [{ scope_key: "global", revision: 1 }], claims: [{
    candidate: candidate.ref, create: true, subject: { entity_id: "owner-1", scope_key: "global", canonical_name: "David", entity_type: "person", anchor_kind: "owner" }, predicate, object_entity: null,
    claim: { claim_id: "claim-2", scope_key: "global", subject_entity_id: "owner-1", predicate, object: { literal: { kind: "text", value: "café" } }, polarity: "affirmed", valid_time: { from: null, to: null }, created_operation_id: "operation-1", transaction_time: "" },
    sources: [{ source_link_id: "source-2", event_id: "event-1", event_part: "content", locator_kind: "utf8_byte_range", locator_value: "0:15", evidence_sha256: "evidence-hash", session_id: "closed-session", source_scope_key: "global", authority: "owner_statement", observed_at: "2026-09-01T00:00:00Z", evidence: "I prefer café.", create: true }],
    context: candidate.candidate.context ?? [], conflicts: [], temporal_qualification: "",
  }] },
};
export const result: ReviewResult = { delivery_key: "idem:v1:delivery-1", preview_id: "preview-1", action: "accept", audit_id: "audit-1", candidates: [{ ...candidate.ref, review_revision: 1 }], operation: { operation_id: "operation-1", claim_ids: ["claim-2"], source_link_ids: ["source-2"], transaction_time: "2026-09-02T00:00:00Z", resulting_revisions: [{ scope_key: "global", revision: 2 }] } };
export function fakeAPI(overrides: Partial<CandidateReviewAPI> = {}): CandidateReviewAPI {
  return {
    scopes: async () => ({ scopes: [{ scope_key: "global", kind: "global", label: "Global memory" }], next_cursor: "", indexing: false }),
    list: async (scope) => ({ scope_key: scope, revision: 1, candidates: [candidate], next_cursor: "next-1" }),
    inspect: async () => candidate,
    prepare: async (_scope, _ref, action) => ({ ...preview, action, effect: action === "reject" ? null : preview.effect }),
    resolve: async () => result,
    operation: async () => ({ operation_id: "operation-1", preview, audit_id: "audit-1" }),
    ...overrides,
  };
}
