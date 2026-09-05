package memory

// CompilerLiveSelector selects one exact source lineage, optionally restricted
// to a session. Global denotes unscoped source sessions, never all scopes.
type CompilerLiveSelector struct {
	SourceScope string    `json:"source_scope"`
	SessionID   SessionID `json:"session_id,omitempty"`
	Destination string    `json:"destination"`
}

type CompilerActivationRequest struct {
	RequestID        string               `json:"request_id"`
	ActivationID     string               `json:"activation_id,omitempty"`
	ExpectedRevision int64                `json:"expected_revision"`
	Selector         CompilerLiveSelector `json:"selector"`
}

// The frontier bounds selection; it is never a claim of historical coverage.
type CompilerActivation struct {
	ID              string               `json:"activation_id"`
	Selector        CompilerLiveSelector `json:"selector"`
	GenerationID    string               `json:"generation_id"`
	Revision        int64                `json:"revision"`
	AfterPosition   int64                `json:"after_position"`
	ThroughPosition *int64               `json:"through_position,omitempty"`
	WorkPaused      bool                 `json:"work_paused"`
}

type CompilerActivationStatus struct {
	Activations            []CompilerActivation           `json:"activations"`
	ActivationsTruncated   bool                           `json:"activations_truncated"`
	Roots                  []CompilerActivationRootStatus `json:"roots"`
	RootsTruncated         bool                           `json:"roots_truncated"`
	SelectedEvents         int64                          `json:"selected_events"`
	OutsideSelectionEvents int64                          `json:"outside_selection_events"`
	PendingRoots           int64                          `json:"pending_roots"`
	SourceErrors           int64                          `json:"source_errors"`
}

type CompilerActivationRootStatus struct {
	ActivationID  string  `json:"activation_id"`
	RootID        EventID `json:"root_id"`
	FirstSequence int64   `json:"first_sequence"`
	LastSequence  int64   `json:"last_sequence"`
	State         string  `json:"state"`
	Reason        string  `json:"reason,omitempty"`
	SelectionID   string  `json:"selection_id,omitempty"`
}

type CompilerReconciliation struct {
	Discovered    bool   `json:"discovered"`
	SelectionID   string `json:"selection_id,omitempty"`
	State         string `json:"state,omitempty"`
	DurationNanos int64  `json:"duration_nanos"`
}
