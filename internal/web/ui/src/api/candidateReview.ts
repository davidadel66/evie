import type { SemanticClaim, SemanticEntity, SemanticPredicate, ScopeRevision } from "./memory";

export type CandidateRef = { candidate_id: string; interpretation_revision: number; review_revision: number };
export type EvidenceLocator = { event_id: string; event_part: string; locator_kind: string; locator_value: string; evidence_sha256: string };
export type CandidateSource = { locator: EvidenceLocator; session_id: string; scope_key: string; observed_at: string; authority: string; evidence: string; usage: string };
export type CandidateProposal = {
  proposition: { subject_entity_id: string; predicate_id: string; object: SemanticClaim["object"]; polarity: string };
  valid_time: SemanticClaim["valid_time"];
  temporal_qualification: string;
  identity?: unknown;
};
export type OwnerCandidate = {
  ref: CandidateRef; job_id: string; generation_id: string; destination: string; redacted: boolean;
  identity?: unknown;
  candidate: { candidate_id: string; proposal: CandidateProposal; support: CandidateSource[] | null; context: CandidateSource[] | null; review_state: string; review_revision: number };
};
export type CandidatePage = { scope_key: string; revision: number; candidates: OwnerCandidate[]; next_cursor: string };
export type CandidateScope = { scope_key: string; kind: string; label: string };
export type CandidateScopes = { scopes: CandidateScope[]; next_cursor: string; indexing: boolean };
export type ReviewSource = EvidenceLocator & { source_link_id: string; session_id: string; source_scope_key: string; authority: string; observed_at: string; evidence: string; create: boolean };
export type ReviewClaimEffect = {
  candidate: CandidateRef; claim: SemanticClaim; create: boolean; subject: SemanticEntity; predicate: SemanticPredicate;
  object_entity: SemanticEntity | null; sources: ReviewSource[]; context: CandidateSource[];
  conflicts: { code: string; predicate_token: string; claim_ids: string[] }[];
  temporal_qualification: string;
};
export type ReviewPreview = {
  version: string; preview_id: string; preview_sha256: string; effect_sha256: string; scope_key: string;
  action: "accept" | "reject"; candidates: OwnerCandidate[]; generation_id: string;
  effect: { version: string; operation_id: string; prior_revisions: ScopeRevision[]; claims: ReviewClaimEffect[]; identity?: unknown } | null;
};
export type ReviewDecision = { delivery_key: string; preview_id: string; preview_sha256: string; action: "accept" | "reject"; reason: string };
export type ReviewResult = { delivery_key: string; preview_id: string; action: string; candidates: CandidateRef[]; audit_id: string; operation: { operation_id: string; claim_ids: string[]; source_link_ids: string[]; transaction_time: string; resulting_revisions: ScopeRevision[] } | null };
export type ReviewOperation = { operation_id: string; preview: ReviewPreview; audit_id: string };

export class CandidateReviewError extends Error {
  code: string;
  result?: ReviewResult;
  constructor(code: string, message: string, result?: ReviewResult) { super(message); this.code = code; this.result = result; }
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/memory/candidates/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new CandidateReviewError(value.code ?? "request_failed", value.error ?? `Review request failed (${response.status})`, value.result);
  return value as T;
}

export const candidateReviewAPI = {
  scopes: (cursor = ""): Promise<CandidateScopes> => post("scopes", { limit: 50, cursor }),
  list: (scope_key: string, cursor = ""): Promise<CandidatePage> => post("list", { scope_key, limit: 25, cursor }),
  inspect: (scope_key: string, id: string): Promise<OwnerCandidate> => post("inspect", { scope_key, id }),
  prepare: (scope_key: string, candidate: CandidateRef, action: "accept" | "reject"): Promise<ReviewPreview> => post("prepare", { scope_key, candidate, action }),
  resolve: (scope_key: string, decision: ReviewDecision): Promise<ReviewResult> => post("resolve", { scope_key, decision }),
  operation: (scope_key: string, id: string): Promise<ReviewOperation> => post("operation", { scope_key, id }),
};
export type CandidateReviewAPI = typeof candidateReviewAPI;
