package memory

import "encoding/json"

// CompilerGeneration is the immutable, complete extraction contract. Endpoint
// addresses and process identities belong to requests, not generation identity.
type CompilerGeneration struct {
	ModelManifestSHA256 string           `json:"model_manifest_sha256"`
	ModelManifest       []byte           `json:"model_manifest"`
	Version             string           `json:"version"`
	ModelArtifact       string           `json:"model_artifact"`
	ModelSHA256         string           `json:"model_sha256"`
	Quantization        string           `json:"quantization"`
	RuntimeVersion      string           `json:"runtime_version"`
	ProtocolVersion     string           `json:"protocol_version"`
	TokenizerSHA256     string           `json:"tokenizer_sha256"`
	Template            string           `json:"template"`
	TemplateSHA256      string           `json:"template_sha256"`
	Prompt              string           `json:"prompt"`
	Schema              json.RawMessage  `json:"schema"`
	Decoding            CompilerDecoding `json:"decoding"`
	// The selected tokenizer's byte upper bound is evidence, not an estimate.
	TokenBoundProofSHA256 string `json:"token_bound_proof_sha256"`
	TokensPerByte         int    `json:"tokens_per_byte"`
	TemplateTokenOverhead int    `json:"template_token_overhead"`
	EvidencePolicy        string `json:"evidence_policy"`
	SecretPolicy          string `json:"secret_policy"`
	ClosurePolicy         string `json:"closure_policy"`
	WindowPolicy          string `json:"window_policy"`
	PredicatePolicy       string `json:"predicate_policy"`
	EntityPolicy          string `json:"entity_policy"`
	ValidationPolicy      string `json:"validation_policy"`
	EquivalencePolicy     string `json:"equivalence_policy"`
	EffectPolicy          string `json:"effect_policy"`
}

type CompilerDecoding struct {
	ContextTokens int     `json:"context_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	Seed          int     `json:"seed"`
	Temperature   float64 `json:"temperature"`
}

// CompilationSelection names one finite root prefix. A later selection of the
// same root owns only its previously unowned suffix.
type CompilationSelection struct {
	SessionID   SessionID `json:"session_id"`
	RootID      EventID   `json:"root_id"`
	Cutoff      int64     `json:"cutoff"`
	Destination string    `json:"destination"`
}

type CompilerSource struct {
	SourceType    SemanticSourceType `json:"source_type"`
	Locator       EvidenceLocator    `json:"locator"`
	SessionID     SessionID          `json:"session_id"`
	ScopeKey      string             `json:"scope_key"`
	Sequence      int64              `json:"sequence"`
	FormatVersion int                `json:"format_version"`
	ObservedAt    string             `json:"observed_at"`
	Actor         SemanticActor      `json:"actor"`
	Authority     SourceAuthority    `json:"authority"`
	Usage         string             `json:"usage"`
	Evidence      string             `json:"evidence"`
}

type CompilerOmission struct {
	FormatVersion int     `json:"format_version"`
	EventID       EventID `json:"event_id"`
	Sequence      int64   `json:"sequence"`
	Reason        string  `json:"reason"`
}

type CompilerWindow struct {
	Selection     CompilationSelection `json:"selection"`
	FirstSequence int64                `json:"first_sequence"`
	Closure       string               `json:"closure"`
	NewEventIDs   []EventID            `json:"new_event_ids"`
	Sources       []CompilerSource     `json:"sources"`
	Omissions     []CompilerOmission   `json:"omissions"`
}

type CompilerRequest struct {
	// AttemptID identifies one dispatch; it is excluded from the sealed request.
	AttemptID              string              `json:"-"`
	AcceptedContextOmitted bool                `json:"accepted_context_omitted"`
	ID                     string              `json:"request_id"`
	GenerationID           string              `json:"generation_id"`
	WindowSHA256           string              `json:"window_sha256"`
	Window                 CompilerWindow      `json:"window"`
	Entities               []SemanticEntity    `json:"entities"`
	Predicates             []SemanticPredicate `json:"predicates"`
	ScopeRevisions         []ScopeRevision     `json:"scope_revisions"`
}

// ExtractorCandidate is untrusted model output. Scope, authority, projected
// evidence and review state are deliberately absent: the Kernel binds them.
type ExtractorCandidate struct {
	Proposition           ClaimProposition  `json:"proposition"`
	ValidTime             ValidTime         `json:"valid_time"`
	TemporalQualification string            `json:"temporal_qualification"`
	Support               []EvidenceLocator `json:"support"`
	Context               []EvidenceLocator `json:"context"`
}

type CompilerResponse struct {
	RequestID  string               `json:"request_id"`
	Candidates []ExtractorCandidate `json:"candidates"`
}

type MemoryCandidate struct {
	ReviewRevision int64              `json:"review_revision"`
	ID             string             `json:"candidate_id"`
	Proposal       ExtractorCandidate `json:"proposal"`
	Support        []CompilerSource   `json:"support"`
	Context        []CompilerSource   `json:"context"`
	ReviewState    string             `json:"review_state"`
	EquivalentTo   string             `json:"equivalent_to,omitempty"`
}

type Compilation struct {
	SelectionID   string             `json:"selection_id"`
	JobID         string             `json:"job_id"`
	GenerationID  string             `json:"generation_id"`
	Generation    CompilerGeneration `json:"generation"`
	Window        CompilerWindow     `json:"window"`
	State         string             `json:"state"`
	Reason        string             `json:"reason,omitempty"`
	Attempts      int                `json:"attempts"`
	CapacityState string             `json:"capacity_state,omitempty"`
	Candidates    []MemoryCandidate  `json:"candidates"`
}
