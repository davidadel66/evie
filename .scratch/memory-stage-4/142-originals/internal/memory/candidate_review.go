package memory

import "time"

// CandidateRef binds one immutable interpretation and its current resolution.
type CandidateRef struct {
	ID                     string `json:"candidate_id"`
	InterpretationRevision int64  `json:"interpretation_revision"`
	ReviewRevision         int64  `json:"review_revision"`
}

type OwnerCandidate struct {
	Identity     *ReviewIdentityRevision `json:"identity,omitempty"`
	Ref          CandidateRef            `json:"ref"`
	JobID        string                  `json:"job_id"`
	GenerationID string                  `json:"generation_id"`
	Destination  string                  `json:"destination"`
	Candidate    MemoryCandidate         `json:"candidate"`
	Redacted     bool                    `json:"redacted"`
}

type OwnerCandidatePage struct {
	ScopeKey   string           `json:"scope_key"`
	Revision   int64            `json:"revision"`
	Candidates []OwnerCandidate `json:"candidates"`
	NextCursor string           `json:"next_cursor"`
}

type OwnerCandidateQuery struct {
	Limit  int    `json:"limit"`
	Cursor string `json:"cursor"`
}

type ReviewClaimEffect struct {
	Candidate             CandidateRef           `json:"candidate"`
	Claim                 SemanticClaim          `json:"claim"`
	Create                bool                   `json:"create"`
	Subject               SemanticEntity         `json:"subject"`
	Predicate             SemanticPredicate      `json:"predicate"`
	ObjectEntity          *SemanticEntity        `json:"object_entity"`
	Sources               []SemanticSource       `json:"sources"`
	Conflicts             []ClaimConflictWarning `json:"conflicts"`
	Context               []CompilerSource       `json:"context"`
	TemporalQualification string                 `json:"temporal_qualification"`
}

type ReviewEffect struct {
	Identity       *ReviewIdentityEffect `json:"identity,omitempty"`
	Version        string                `json:"version"`
	OperationID    SemanticID            `json:"operation_id"`
	Scope          SemanticScope         `json:"scope"`
	Scopes         []SemanticScope       `json:"scopes"`
	PriorRevisions []ScopeRevision       `json:"prior_revisions"`
	Claims         []ReviewClaimEffect   `json:"claims"`
}

// ReviewPreview contains only one complete atomic group in the first review
// slice. Later batch preparation can compose already explicit groups.
type ReviewPreview struct {
	Version               string           `json:"version"`
	ID                    string           `json:"preview_id"`
	OwnerID               OwnerID          `json:"owner_id"`
	AuthenticationBinding string           `json:"authentication_binding"`
	AuthorizationRevision int64            `json:"authorization_revision"`
	ScopeKey              string           `json:"scope_key"`
	JobID                 string           `json:"job_id"`
	GenerationID          string           `json:"generation_id"`
	Action                string           `json:"action"`
	Candidates            []OwnerCandidate `json:"candidates"`
	SourcePolicy          string           `json:"source_policy"`
	Effect                *ReviewEffect    `json:"effect"`
	EffectSHA256          string           `json:"effect_sha256"`
	SHA256                string           `json:"preview_sha256"`
}

type ReviewDecision struct {
	DeliveryKey   string `json:"delivery_key"`
	PreviewID     string `json:"preview_id"`
	PreviewSHA256 string `json:"preview_sha256"`
	Action        string `json:"action"`
	Reason        string `json:"reason"`
}

type ReviewResult struct {
	DeliveryKey string                      `json:"delivery_key"`
	PreviewID   string                      `json:"preview_id"`
	Action      string                      `json:"action"`
	AuditID     string                      `json:"audit_id"`
	Candidates  []CandidateRef              `json:"candidates"`
	Operation   *OwnerReviewOperationResult `json:"operation"`
}

type OwnerReviewOperation struct {
	SchemaVersion  int           `json:"schema_version"`
	Kind           string        `json:"kind"`
	OperationID    SemanticID    `json:"operation_id"`
	IdempotencyKey string        `json:"idempotency_key"`
	Actor          SemanticActor `json:"actor"`
	SessionID      SessionID     `json:"session_id"`
	SourceEventID  EventID       `json:"source_event_id"`
	Preview        ReviewPreview `json:"preview"`
	AuditID        string        `json:"audit_id"`
}

type OwnerReviewOperationResult struct {
	OperationID        SemanticID      `json:"operation_id"`
	ClaimIDs           []SemanticID    `json:"claim_ids"`
	SourceLinkIDs      []SemanticID    `json:"source_link_ids"`
	TransactionTime    time.Time       `json:"transaction_time"`
	ResultingRevisions []ScopeRevision `json:"resulting_revisions"`
}
