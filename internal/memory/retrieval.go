package memory

import "time"

const (
	RetrievalAcceptedMemory      = "accepted_memory"
	RetrievalConversationExcerpt = "conversation_excerpt"
	RetrievalSuccess             = "success"
	RetrievalEmpty               = "empty"
	RetrievalUnavailable         = "unavailable"
	RetrievalPartial             = "partial"
	RetrievalFailed              = "failed"
	RetrievalCancelled           = "cancelled"
	RetrievalExhausted           = "exhausted"
)

// RetrievalQuery contains caller requests, never authority. The Kernel resolves
// effective scopes from the durable session before looking at any index hit.
type RetrievalQuery struct {
	Kind      string     `json:"kind,omitempty"`
	Text      string     `json:"text"`
	Limit     int        `json:"limit,omitempty"`
	MaxBytes  int        `json:"max_bytes,omitempty"`
	ValidAt   *time.Time `json:"valid_at,omitempty"`
	AsKnownAt *time.Time `json:"as_known_at,omitempty"`
}

type RetrievalCoverage struct {
	Generation string `json:"generation"`
	State      string `json:"state"`
	Pending    int64  `json:"pending"`
	Indexed    int64  `json:"indexed"`
}

type RetrievalEvidence struct {
	ID               string               `json:"id"`
	Kind             string               `json:"kind"`
	ClaimID          SemanticID           `json:"claim_id,omitempty"`
	ClaimOperationID SemanticID           `json:"claim_operation_id,omitempty"`
	AsKnownAt        time.Time            `json:"as_known_at"`
	ValidAt          time.Time            `json:"valid_at"`
	ScopeKey         string               `json:"scope_key"`
	Status           SemanticObjectStatus `json:"status"`
	Text             string               `json:"text"`
	Sources          []SemanticSource     `json:"sources"`
	Paths            []string             `json:"paths"`
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
	ID               string                     `json:"id"`
	Kind             string                     `json:"kind"`
	ClaimID          SemanticID                 `json:"claim_id,omitempty"`
	ClaimOperationID SemanticID                 `json:"claim_operation_id,omitempty"`
	AsKnownAt        time.Time                  `json:"as_known_at"`
	ValidAt          time.Time                  `json:"valid_at"`
	ScopeKey         string                     `json:"scope_key"`
	Status           SemanticObjectStatus       `json:"status"`
	Sources          []RetrievalSourceReference `json:"sources"`
	Paths            []string                   `json:"paths"`
}

func (e RetrievalEvidence) Reference() RetrievalReference {
	r := RetrievalReference{ID: e.ID, Kind: e.Kind, ClaimID: e.ClaimID, ClaimOperationID: e.ClaimOperationID,
		AsKnownAt: e.AsKnownAt, ValidAt: e.ValidAt, ScopeKey: e.ScopeKey, Status: e.Status, Paths: append([]string(nil), e.Paths...)}
	for _, s := range e.Sources {
		r.Sources = append(r.Sources, RetrievalSourceReference{SourceLinkID: s.ID, SessionID: s.SessionID, ScopeKey: s.ScopeKey,
			Authority: s.Authority, ObservedAt: s.ObservedAt, EvidenceLocator: EvidenceLocator{EventID: s.EventID,
				EventPart: s.EventPart, LocatorKind: s.LocatorKind, LocatorValue: s.LocatorValue, EvidenceSHA256: s.EvidenceSHA256}})
	}
	return r
}

type RetrievalResult struct {
	Status          string              `json:"status"`
	Evidence        []RetrievalEvidence `json:"evidence"`
	Coverage        RetrievalCoverage   `json:"coverage"`
	Truncated       bool                `json:"truncated"`
	SerializedBytes int                 `json:"serialized_bytes"`
}

type RetrievalInspection struct {
	Reference     RetrievalReference   `json:"reference"`
	Evidence      *RetrievalEvidence   `json:"evidence,omitempty"`
	CurrentStatus SemanticObjectStatus `json:"current_status"`
	Available     bool                 `json:"available"`
}
