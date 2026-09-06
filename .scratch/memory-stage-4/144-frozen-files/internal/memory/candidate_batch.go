package memory

// ReviewEditRevision retains typed interpretation history. Before includes the
// exact prior choices; After is validated against the original frozen window.
type ReviewEditRevision struct {
	Revision              int64             `json:"revision"`
	ParentRevision        int64             `json:"parent_revision"`
	ReviewRevision        int64             `json:"review_revision"`
	AuditID               string            `json:"audit_id"`
	OwnerID               OwnerID           `json:"owner_id"`
	AuthenticationBinding string            `json:"authentication_binding"`
	AuthorizationRevision int64             `json:"authorization_revision"`
	CandidateID           string            `json:"candidate_id"`
	Before                ReviewEditMeaning `json:"before"`
	After                 ReviewEditMeaning `json:"after"`
	Reason                string            `json:"reason"`
}
type ReviewEditMeaning struct {
	Proposal ExtractorCandidate      `json:"proposal"`
	Support  []CompilerSource        `json:"support"`
	Context  []CompilerSource        `json:"context"`
	Identity *ReviewIdentityRevision `json:"identity"`
	Temporal *ReviewTemporalRevision `json:"temporal"`
}
type ReviewEditDecision struct {
	Candidate CandidateRef       `json:"candidate"`
	Proposal  ExtractorCandidate `json:"proposal"`
	Reason    string             `json:"reason"`
}

// A binding explicitly shares a proposed identity inside an atomic group.
// Fields are subject, object, or predicate; the provider must precede its user.
// No binding can reach outside the group's complete candidate closure.
type ReviewDependency struct {
	CandidateID     string `json:"candidate_id"`
	Field           string `json:"field"`
	FromCandidateID string `json:"from_candidate_id"`
	FromField       string `json:"from_field"`
}
type ReviewBatchGroupRequest struct {
	ID           string             `json:"group_id"`
	Action       string             `json:"action"`
	Candidates   []CandidateRef     `json:"candidates"`
	Dependencies []ReviewDependency `json:"dependencies"`
}
type ReviewBatchRequest struct {
	Groups []ReviewBatchGroupRequest `json:"groups"`
}
type ReviewBatchGroup struct {
	ID      string        `json:"group_id"`
	Preview ReviewPreview `json:"preview"`
}
type ReviewBatchPreview struct {
	Version               string             `json:"version"`
	ID                    string             `json:"preview_id"`
	OwnerID               OwnerID            `json:"owner_id"`
	AuthenticationBinding string             `json:"authentication_binding"`
	AuthorizationRevision int64              `json:"authorization_revision"`
	ScopeKey              string             `json:"scope_key"`
	SourcePolicy          string             `json:"source_policy"`
	PriorRevisions        []ScopeRevision    `json:"prior_revisions"`
	FailureBehavior       string             `json:"failure_behavior"`
	Groups                []ReviewBatchGroup `json:"groups"`
	SHA256                string             `json:"preview_sha256"`
}
type ReviewBatchAction struct {
	GroupID string `json:"group_id"`
	Action  string `json:"action"`
}
type ReviewBatchDecision struct {
	DeliveryKey   string              `json:"delivery_key"`
	PreviewID     string              `json:"preview_id"`
	PreviewSHA256 string              `json:"preview_sha256"`
	Actions       []ReviewBatchAction `json:"actions"`
	Reason        string              `json:"reason"`
}
type ReviewBatchGroupResult struct {
	PriorResolutions []ReviewResult `json:"prior_resolutions,omitempty"`
	GroupID          string         `json:"group_id"`
	Outcome          string         `json:"outcome"`
	FailureCode      string         `json:"failure_code"`
	Result           *ReviewResult  `json:"result"`
}
type ReviewBatchResult struct {
	DeliveryKey string                   `json:"delivery_key"`
	PreviewID   string                   `json:"preview_id"`
	Groups      []ReviewBatchGroupResult `json:"groups"`
}

// The recorded advances are the only allowed difference from the preview's
// starting vector. Each is checked against an earlier accepted group operation.
type ReviewBatchCommit struct {
	PreviewID     string                       `json:"preview_id"`
	PreviewSHA256 string                       `json:"preview_sha256"`
	GroupID       string                       `json:"group_id"`
	GroupIndex    int                          `json:"group_index"`
	PriorGroups   []OwnerReviewOperationResult `json:"prior_groups"`
}

// Records enumerate semantic object reuse, creation and state transitions.
// Exact object values and before/after correction data live in the corresponding
// member effect; record IDs link these counts to that canonical manifest.
type ReviewEffectRecord struct {
	Kind        string     `json:"kind"`
	ID          SemanticID `json:"id"`
	Action      string     `json:"action"`
	BeforeState string     `json:"before_state"`
	AfterState  string     `json:"after_state"`
}
