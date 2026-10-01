package memory

import "time"

const (
	RetrievalAcceptedMemory        = "accepted_memory"
	RetrievalConversationExcerpt   = "conversation_excerpt"
	RetrievalConversationExpansion = "conversation_expansion"
	RetrievalCurrent               = "current"
	RetrievalHistorical            = "historical"
	RetrievalSuccess               = "success"
	RetrievalEmpty                 = "empty"
	RetrievalUnavailable           = "unavailable"
	RetrievalPartial               = "partial"
	RetrievalFailed                = "failed"
	RetrievalCancelled             = "cancelled"
	RetrievalExhausted             = "exhausted"

	// RetrievalGapDenseScan marks a dense scan its work budget stopped before
	// every vector was compared. It is distinct from pending index work, which
	// also makes a result partial.
	RetrievalGapDenseScan = "dense_scan_budget"

	// RetrievalHistoricalSource marks an excerpt that is a recorded Source of
	// an accepted Claim that is no longer current. RetrievalHistoricalRestatement
	// marks an excerpt that repeats such a Claim's saved value without being
	// one of its Sources.
	RetrievalHistoricalSource      = "source"
	RetrievalHistoricalRestatement = "restatement"
)

// RetrievalHistoricalClaim links a Conversation Excerpt to an accepted Claim
// that is retired or superseded. The excerpt stays attributed history; the
// link only says it cannot stand as a current fact. The Kernel computes it
// from accepted state on every read and names only Claims the reader may
// read. A corrected Claim names its correction mode and replacement.
type RetrievalHistoricalClaim struct {
	Relation           string               `json:"relation"`
	ClaimID            SemanticID           `json:"claim_id"`
	Status             SemanticObjectStatus `json:"status"`
	CorrectionMode     CorrectionMode       `json:"correction_mode,omitempty"`
	ReplacementClaimID SemanticID           `json:"replacement_claim_id,omitempty"`
}

// RetrievalQuery contains caller requests, never authority. The Kernel resolves
// effective scopes from the durable session before looking at any index hit.
type RetrievalQuery struct {
	// Automatic interpretation omits exact copies of the active user request.
	// The Kernel resolves that request from the bound session, never model text.
	ExcludeCurrentRequestCopies bool                 `json:"-"`
	Kind                        string               `json:"kind,omitempty"`
	Intent                      string               `json:"intent,omitempty"`
	AnchorID                    string               `json:"evidence_id,omitempty"`
	Anchor                      *RetrievalReference  `json:"-"`
	Covered                     []RetrievalReference `json:"-"`
	RefreshReferences           []RetrievalReference `json:"-"`
	Before                      int                  `json:"before,omitempty"`
	After                       int                  `json:"after,omitempty"`
	Text                        string               `json:"text"`
	Limit                       int                  `json:"limit,omitempty"`
	MaxBytes                    int                  `json:"max_bytes,omitempty"`
	ValidAt                     *time.Time           `json:"valid_at,omitempty"`
	AsKnownAt                   *time.Time           `json:"as_known_at,omitempty"`
	// Relevance is set only by Automatic Recall, never from model arguments.
	// The Kernel applies it to conversation excerpts.
	Relevance *RetrievalRelevance `json:"-"`
}

// RetrievalRelevance is Automatic Recall's deterministic relevance contract.
// Current holds the active request's content terms. Context holds the term
// groups of earlier topics a short or referring follow-up depends on; each
// group can qualify evidence on its own. LiveFrom is the first event of the
// bound session still in the provider request, empty when nothing has been
// compacted; that session's messages from there on are already visible to the
// model and are not recalled again.
type RetrievalRelevance struct {
	Current  []string
	Context  [][]string
	LiveFrom EventID
}

type RetrievalCoverage struct {
	Generation string `json:"generation"`
	State      string `json:"state"`
	Pending    int64  `json:"pending"`
	Indexed    int64  `json:"indexed"`
}

type RetrievalEvidence struct {
	AsKnownAtConstrained  bool                       `json:"as_known_at_constrained,omitempty"`
	IdentityMatches       []RetrievalIdentityMatch   `json:"identity_matches,omitempty"`
	RetrievalGeneration   string                     `json:"retrieval_generation,omitempty"`
	GraphPaths            []RetrievalGraphPath       `json:"graph_paths,omitempty"`
	Intent                string                     `json:"intent"`
	ValidAtConstrained    bool                       `json:"valid_at_constrained"`
	CurrentStatus         SemanticObjectStatus       `json:"current_status"`
	Claim                 *SemanticClaim             `json:"claim,omitempty"`
	EffectiveValidTime    *ValidTime                 `json:"effective_valid_time,omitempty"`
	CorrectionMode        CorrectionMode             `json:"correction_mode,omitempty"`
	CurrentCorrectionMode CorrectionMode             `json:"current_correction_mode,omitempty"`
	Conflicts             []ClaimConflictWarning     `json:"conflicts,omitempty"`
	RelatedClaimIDs       []SemanticID               `json:"related_claim_ids,omitempty"`
	HistoricalClaims      []RetrievalHistoricalClaim `json:"historical_claims,omitempty"`
	ID                    string                     `json:"id"`
	Kind                  string                     `json:"kind"`
	ClaimID               SemanticID                 `json:"claim_id,omitempty"`
	ClaimOperationID      SemanticID                 `json:"claim_operation_id,omitempty"`
	AsKnownAt             time.Time                  `json:"as_known_at"`
	ValidAt               time.Time                  `json:"valid_at"`
	ScopeKey              string                     `json:"scope_key"`
	Status                SemanticObjectStatus       `json:"status"`
	Text                  string                     `json:"text"`
	Sources               []SemanticSource           `json:"sources"`
	Paths                 []string                   `json:"paths"`
}

// RetrievalGraphPath names accepted, source-bearing Claims in traversal order.
// Every supporting Claim must also occur in the selected evidence set. A path
// explains discovery; it does not accept an inferred relationship.
type RetrievalGraphPath struct {
	AnchorEntityID SemanticID   `json:"anchor_entity_id"`
	ClaimIDs       []SemanticID `json:"claim_ids"`
}

// RetrievalSourceReference is content-free and can survive in a request
// snapshot. Inspection must resolve its exact locator and apply current access.
type RetrievalSourceReference struct {
	SourceLinkID SemanticID      `json:"source_link_id,omitempty"`
	SessionID    SessionID       `json:"session_id"`
	ScopeKey     string          `json:"scope_key"`
	Authority    SourceAuthority `json:"authority"`
	ObservedAt   string          `json:"observed_at"`
	EvidenceLocator
}

type RetrievalReference struct {
	AsKnownAtConstrained  bool                         `json:"as_known_at_constrained,omitempty"`
	IdentityMatches       []RetrievalIdentityReference `json:"identity_matches,omitempty"`
	RetrievalGeneration   string                       `json:"retrieval_generation,omitempty"`
	GraphPaths            []RetrievalGraphPath         `json:"graph_paths,omitempty"`
	Intent                string                       `json:"intent"`
	ValidAtConstrained    bool                         `json:"valid_at_constrained"`
	CurrentStatus         SemanticObjectStatus         `json:"current_status"`
	CorrectionMode        CorrectionMode               `json:"correction_mode,omitempty"`
	CurrentCorrectionMode CorrectionMode               `json:"current_correction_mode,omitempty"`
	Conflicts             []ClaimConflictWarning       `json:"conflicts,omitempty"`
	RelatedClaimIDs       []SemanticID                 `json:"related_claim_ids,omitempty"`
	HistoricalClaims      []RetrievalHistoricalClaim   `json:"historical_claims,omitempty"`
	ID                    string                       `json:"id"`
	Kind                  string                       `json:"kind"`
	ClaimID               SemanticID                   `json:"claim_id,omitempty"`
	ClaimOperationID      SemanticID                   `json:"claim_operation_id,omitempty"`
	AsKnownAt             time.Time                    `json:"as_known_at"`
	ValidAt               time.Time                    `json:"valid_at"`
	ScopeKey              string                       `json:"scope_key"`
	Status                SemanticObjectStatus         `json:"status"`
	Sources               []RetrievalSourceReference   `json:"sources"`
	Paths                 []string                     `json:"paths"`
}

func (e RetrievalEvidence) Reference() RetrievalReference {
	r := RetrievalReference{AsKnownAtConstrained: e.AsKnownAtConstrained, ID: e.ID, Kind: e.Kind, ClaimID: e.ClaimID, ClaimOperationID: e.ClaimOperationID,
		RetrievalGeneration: e.RetrievalGeneration,
		AsKnownAt:           e.AsKnownAt, ValidAt: e.ValidAt, ScopeKey: e.ScopeKey, Status: e.Status, Paths: append([]string(nil), e.Paths...),
		Intent: e.Intent, ValidAtConstrained: e.ValidAtConstrained, CurrentStatus: e.CurrentStatus,
		CorrectionMode: e.CorrectionMode, CurrentCorrectionMode: e.CurrentCorrectionMode,
		RelatedClaimIDs:  append([]SemanticID(nil), e.RelatedClaimIDs...),
		HistoricalClaims: append([]RetrievalHistoricalClaim(nil), e.HistoricalClaims...)}
	for _, match := range e.IdentityMatches {
		ref := match.RetrievalIdentityReference
		if ref.Source != nil {
			source := *ref.Source
			ref.Source = &source
		}
		r.IdentityMatches = append(r.IdentityMatches, ref)
	}
	for _, conflict := range e.Conflicts {
		conflict.ClaimIDs = append([]SemanticID(nil), conflict.ClaimIDs...)
		r.Conflicts = append(r.Conflicts, conflict)
	}
	for _, path := range e.GraphPaths {
		path.ClaimIDs = append([]SemanticID(nil), path.ClaimIDs...)
		r.GraphPaths = append(r.GraphPaths, path)
	}
	for _, s := range e.Sources {
		r.Sources = append(r.Sources, RetrievalSourceReference{SourceLinkID: s.ID, SessionID: s.SessionID, ScopeKey: s.ScopeKey,
			Authority: s.Authority, ObservedAt: s.ObservedAt, EvidenceLocator: EvidenceLocator{EventID: s.EventID,
				EventPart: s.EventPart, LocatorKind: s.LocatorKind, LocatorValue: s.LocatorValue, EvidenceSHA256: s.EvidenceSHA256}})
	}
	return r
}

type RetrievalResult struct {
	DenseCoverage   *RetrievalCoverage  `json:"dense_coverage,omitempty"`
	Status          string              `json:"status"`
	Evidence        []RetrievalEvidence `json:"evidence"`
	Coverage        RetrievalCoverage   `json:"coverage"`
	Truncated       bool                `json:"truncated"`
	SerializedBytes int                 `json:"serialized_bytes"`
	// Gaps names coverage a budget cut short, such as RetrievalGapDenseScan.
	Gaps []string `json:"gaps,omitempty"`
}

type RetrievalInspection struct {
	Reference     RetrievalReference   `json:"reference"`
	Evidence      *RetrievalEvidence   `json:"evidence,omitempty"`
	CurrentStatus SemanticObjectStatus `json:"current_status"`
	Available     bool                 `json:"available"`
}
