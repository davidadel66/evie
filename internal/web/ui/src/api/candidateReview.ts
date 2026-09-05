import type { SemanticClaim, SemanticEntity, SemanticPredicate, ScopeRevision, SemanticState } from "./memory";

export type CandidateRef = { candidate_id: string; interpretation_revision: number; review_revision: number };
export type EvidenceLocator = { event_id: string; event_part: string; locator_kind: string; locator_value: string; evidence_sha256: string };
export type Observation = { contract: string; root_id: string; execution_id: string; call_id: string; ancestry_sha256: string };
export type CandidateSource = { observation?: Observation; actor?: string; source_type?: string; format_version?: number; sequence?: number; locator: EvidenceLocator; session_id: string; scope_key: string; observed_at: string; authority: string; evidence: string; usage: string };
export type CandidateProposal = {
  proposition: { subject_entity_id: string; predicate_id: string; object: SemanticClaim["object"]; polarity: string };
  valid_time: SemanticClaim["valid_time"];
  temporal_qualification: string;
  identity?: IdentityProposal | null;
  temporal?: { meaning: string; correction: { modes: string[]; effective_time: string | null } | null } | null;
  support?: EvidenceLocator[]; context?: EvidenceLocator[];
};
export type OwnerCandidate = {
  ref: CandidateRef; job_id: string; generation_id: string; destination: string; redacted: boolean;
  identity?: IdentityRevision; temporal?: TemporalRevision; edit?: EditRevision; original?: OwnerCandidate["candidate"];
  candidate: { candidate_id: string; proposal: CandidateProposal; support: CandidateSource[] | null; context: CandidateSource[] | null; review_state: string; review_revision: number };
};
export type CandidatePage = { scope_key: string; revision: number; candidates: OwnerCandidate[]; next_cursor: string };
export type CandidateScope = { scope_key: string; kind: string; label: string };
export type CandidateScopes = { scopes: CandidateScope[]; next_cursor: string; indexing: boolean };
export type ReviewSource = EvidenceLocator & { actor?: string; source_type?: string; eligibility?: string; source_link_id: string; session_id: string; source_scope_key: string; authority: string; observed_at: string; evidence: string; create: boolean };
export type ReviewClaimEffect = {
  candidate: CandidateRef; claim: SemanticClaim; create: boolean; subject: SemanticEntity & { create?: boolean }; predicate: SemanticPredicate & { create?: boolean };
  object_entity: (SemanticEntity & { create?: boolean }) | null; sources: ReviewSource[]; context: CandidateSource[];
  conflicts: { code: string; predicate_token: string; claim_ids: string[] }[];
  temporal_qualification: string;
};
export type ReviewPreview = {
  version: string; preview_id: string; preview_sha256: string; effect_sha256: string; scope_key: string;
  action: "accept" | "reject"; candidates: OwnerCandidate[]; generation_id: string;
  effect: ReviewEffect | null;
  batch_preview_id?: string; dependencies?: ReviewDependency[]; source_policy?: string;
};
export type ReviewDecision = { delivery_key: string; preview_id: string; preview_sha256: string; action: "accept" | "reject"; reason: string };
export type ReviewResult = { delivery_key: string; preview_id: string; action: string; candidates: CandidateRef[]; audit_id: string; operation: { operation_id: string; claim_ids: string[]; source_link_ids: string[]; transaction_time: string; resulting_revisions: ScopeRevision[] } | null };
export type ReviewOperation = { operation_id: string; preview: ReviewPreview; audit_id: string };

export class CandidateReviewError extends Error {
  code: string;
  result?: ReviewResult;
  constructor(code: string, message: string, result?: ReviewResult) { super(message); this.code = code; this.result = result; }
}

export async function postCandidateReview<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/memory/candidates/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json();
  if (!response.ok) throw new CandidateReviewError(value.code ?? "request_failed", value.error ?? `Review request failed (${response.status})`, value.result);
  return value as T;
}

export const candidateReviewAPI = {
  scopes: (cursor = ""): Promise<CandidateScopes> => postCandidateReview("scopes", { limit: 50, cursor }),
  list: (scope_key: string, cursor = ""): Promise<CandidatePage> => postCandidateReview("list", { scope_key, limit: 25, cursor }),
  inspect: (scope_key: string, id: string): Promise<OwnerCandidate> => postCandidateReview("inspect", { scope_key, id }),
  prepare: (scope_key: string, candidate: CandidateRef, action: "accept" | "reject"): Promise<ReviewPreview> => postCandidateReview("prepare", { scope_key, candidate, action }),
  resolve: (scope_key: string, decision: ReviewDecision): Promise<ReviewResult> => postCandidateReview("resolve", { scope_key, decision }),
  identityOptions: (scope_key: string, input: CandidateRef): Promise<IdentityOptions> => postCandidateReview("identity/options", { scope_key, input }),
  chooseIdentity: (scope_key: string, input: IdentityDecision): Promise<OwnerCandidate> => postCandidateReview("identity/choose", { scope_key, input }),
  temporalOptions: (scope_key: string, input: CandidateRef): Promise<TemporalOptions> => postCandidateReview("temporal/options", { scope_key, input }),
  chooseTemporal: (scope_key: string, input: TemporalDecision): Promise<OwnerCandidate> => postCandidateReview("temporal/choose", { scope_key, input }),
  edit: (scope_key: string, input: EditDecision): Promise<OwnerCandidate> => postCandidateReview("edit", { scope_key, input }),
  editRevision: (scope_key: string, id: string, revision: number): Promise<EditRevision> => postCandidateReview("edit/revision", { scope_key, input: { id, revision } }),
  identityRevision: (scope_key: string, id: string, revision: number): Promise<IdentityRevision> => postCandidateReview("identity/revision", { scope_key, input: { id, revision } }),
  temporalRevision: (scope_key: string, id: string, revision: number): Promise<TemporalRevision> => postCandidateReview("temporal/revision", { scope_key, input: { id, revision } }),
  prepareBatch: (scope_key: string, input: BatchRequest): Promise<BatchPreview> => postCandidateReview("batch/prepare", { scope_key, input }),
  inspectBatch: (scope_key: string, id: string): Promise<BatchPreview> => postCandidateReview("batch/inspect", { scope_key, input: { id, revision: 0 } }),
  resolveBatch: (scope_key: string, input: BatchDecision): Promise<BatchResult> => postCandidateReview("batch/resolve", { scope_key, input }),
  operation: (scope_key: string, id: string): Promise<ReviewOperation> => postCandidateReview("operation", { scope_key, id }),
};
export type CandidateReviewAPI = typeof candidateReviewAPI;

export type EntityMention = { name: string; entity_type: string; support: EvidenceLocator };
export type PredicateDefinition = Pick<SemanticPredicate, "token" | "label" | "object_constraint" | "cardinality">;
export type IdentityProposal = { subject: EntityMention | null; object: EntityMention | null; predicate: PredicateDefinition | null; uncertainty: string; confidence: number | null };
export type SemanticAlias = { alias_id: string; entity_id: string; scope_key: string; value: string; normalized_value: string; source_event_id: string; create: boolean };
export type EntityAlternative = { entity: SemanticEntity; aliases: SemanticAlias[]; context: SemanticClaim[] };
export type IdentityOptions = { candidate: CandidateRef; scope_key: string; scope_revisions: ScopeRevision[]; subject: EntityAlternative[]; object: EntityAlternative[]; predicates: SemanticPredicate[]; options_sha256: string };
export type EntityChoice = { entity_id: string; create: boolean };
export type IdentityChoices = { subject: EntityChoice | null; object: EntityChoice | null; predicate: { predicate_id: string; create: boolean } | null };
export type IdentityDecision = { candidate: CandidateRef; options_sha256: string; choices: IdentityChoices };
export type InterpretationRevision = { revision: number; parent_revision: number; review_revision: number; audit_id: string };
export type IdentityRevision = InterpretationRevision & { options: IdentityOptions; choices: IdentityChoices };
export type TemporalOptions = { candidate: CandidateRef; scope_key: string; scope_revisions: ScopeRevision[]; alternatives: { claim: SemanticClaim; state: SemanticState }[]; modes: string[]; effective_time: string | null; options_sha256: string };
export type TemporalDecision = { candidate: CandidateRef; options_sha256: string; choice: { old_claim_id: string; mode: string } };
export type TemporalRevision = InterpretationRevision & { options: TemporalOptions; choice: TemporalDecision["choice"] };
export type CorrectionEffect = { revision: TemporalRevision; old_claim: SemanticClaim; old_state: SemanticState; mode: string; effective_time: string | null; valid_time_effect: { old_before: SemanticClaim["valid_time"]; old_after: SemanticClaim["valid_time"]; replacement: SemanticClaim["valid_time"] }; transition: { object_kind: string; object_id: string; state: string } };
export type EditMeaning = { proposal: CandidateProposal; support: CandidateSource[]; context: CandidateSource[]; identity: IdentityRevision | null; temporal: TemporalRevision | null };
export type EditRevision = InterpretationRevision & { candidate_id: string; before: EditMeaning; after: EditMeaning; reason: string };
export type EditDecision = { candidate: CandidateRef; proposal: CandidateProposal; reason: string };
export type ReviewDependency = { candidate_id: string; field: string; from_candidate_id: string; from_field: string };
export type ReviewEffectRecord = { kind: string; id: string; action: string; before_state: string; after_state: string };
export type ReviewEffect = { version: string; operation_id: string; prior_revisions: ScopeRevision[]; claims: ReviewClaimEffect[]; identity?: { revision: IdentityRevision; aliases: SemanticAlias[] }; correction?: CorrectionEffect; members?: ReviewEffect[]; dependencies?: ReviewDependency[]; records?: ReviewEffectRecord[] };
export type BatchGroupRequest = { group_id: string; action: "accept" | "reject"; candidates: CandidateRef[]; dependencies: ReviewDependency[] };
export type BatchRequest = { groups: BatchGroupRequest[] };
export type BatchPreview = { version: string; preview_id: string; scope_key: string; source_policy: string; prior_revisions: ScopeRevision[]; failure_behavior: string; groups: { group_id: string; preview: ReviewPreview }[]; preview_sha256: string };
export type BatchDecision = { delivery_key: string; preview_id: string; preview_sha256: string; actions: { group_id: string; action: "accept" | "reject" }[]; reason: string };
export type BatchResult = { delivery_key: string; preview_id: string; groups: { group_id: string; outcome: string; failure_code: string; result: ReviewResult | null; prior_resolutions?: ReviewResult[] }[] };
