package memory

import "time"

// CompilerTemporalPolicyV3 adds typed modality and explicit correction choices.
// Older generation identities never admit these fields.
const CompilerTemporalPolicyV3 = "temporal-review-v3"

// Modal propositions use these exact, reviewable Predicate definitions. Their
// text describes an intention/possibility, never an already completed event.
const PlanPredicateToken = "intends"
const PlanPredicateLabel = "Intends (uncompleted plan)"
const PossibilityPredicateToken = "considers"
const PossibilityPredicateLabel = "Considers (unrealized possibility)"

type CandidateTemporalProposal struct {
	Meaning    string                       `json:"meaning"` // assertion, plan, possibility
	Correction *CandidateCorrectionProposal `json:"correction"`
}

type CandidateCorrectionProposal struct {
	Modes         []CorrectionMode `json:"modes"`
	EffectiveTime *time.Time       `json:"effective_time"`
}

type ReviewCorrectionAlternative struct {
	Claim SemanticClaim `json:"claim"`
	State SemanticState `json:"state"`
}

type ReviewTemporalOptions struct {
	Candidate      CandidateRef                  `json:"candidate"`
	ScopeKey       string                        `json:"scope_key"`
	ScopeRevisions []ScopeRevision               `json:"scope_revisions"`
	Alternatives   []ReviewCorrectionAlternative `json:"alternatives"`
	Modes          []CorrectionMode              `json:"modes"`
	EffectiveTime  *time.Time                    `json:"effective_time"`
	SHA256         string                        `json:"options_sha256"`
}

type ReviewTemporalChoice struct {
	OldClaimID SemanticID     `json:"old_claim_id"`
	Mode       CorrectionMode `json:"mode"`
}

type ReviewTemporalDecision struct {
	Candidate     CandidateRef         `json:"candidate"`
	OptionsSHA256 string               `json:"options_sha256"`
	Choice        ReviewTemporalChoice `json:"choice"`
}

type ReviewTemporalRevision struct {
	Revision              int64                 `json:"revision"`
	ParentRevision        int64                 `json:"parent_revision"`
	ReviewRevision        int64                 `json:"review_revision"`
	AuditID               string                `json:"audit_id"`
	OwnerID               OwnerID               `json:"owner_id"`
	AuthenticationBinding string                `json:"authentication_binding"`
	AuthorizationRevision int64                 `json:"authorization_revision"`
	Options               ReviewTemporalOptions `json:"options"`
	Choice                ReviewTemporalChoice  `json:"choice"`
}

type ReviewCorrectionEffect struct {
	Revision        ReviewTemporalRevision    `json:"revision"`
	OldClaim        SemanticClaim             `json:"old_claim"`
	OldState        SemanticState             `json:"old_state"`
	Mode            CorrectionMode            `json:"mode"`
	EffectiveTime   *time.Time                `json:"effective_time"`
	ValidTimeEffect CorrectionValidTimeEffect `json:"valid_time_effect"`
	Transition      SemanticTransition        `json:"transition"`
}
