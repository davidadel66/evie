package memory

// Diagnostics contains operational metadata only. Neither an owner approval nor
// elapsed inbox age measures extraction accuracy or active human review time.
const CompilerDiagnosticsMaxPage = 32

type CompilerDiagnosticsQuery struct {
	SessionID    SessionID `json:"session_id"`
	View         string    `json:"view"`                    // jobs, candidates, activations, history, selections, live_roots, selection, foreground
	GenerationID string    `json:"generation_id,omitempty"` // required for selection
	Cursor       string    `json:"cursor,omitempty"`
	Limit        int       `json:"limit,omitempty"`
}

type CompilerDiagnosticSessionQuery struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type CompilerDiagnosticSessions struct {
	SessionIDs []SessionID `json:"session_ids"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

type CompilerDiagnostics struct {
	Selections    []CompilerDiagnosticUnit        `json:"selections"`
	LiveRoots     []CompilerDiagnosticRoot        `json:"live_roots"`
	ScopeKey      string                          `json:"scope_key"`
	SessionID     SessionID                       `json:"session_id"`
	View          string                          `json:"view"`
	AsOfUnixMS    int64                           `json:"as_of_unix_ms"`
	Revision      int64                           `json:"revision"`
	Indexing      bool                            `json:"indexing"`
	Counts        map[string]int64                `json:"counts"`
	CapacityState string                          `json:"capacity_state"`
	Jobs          []CompilerDiagnosticJob         `json:"jobs"`
	Candidates    []CompilerDiagnosticCandidate   `json:"candidates"`
	Activations   []CompilerActivation            `json:"activations"`
	History       []CompilerDiagnosticHistory     `json:"history"`
	Selection     []CompilerDiagnosticSelection   `json:"selection"`
	Foreground    []CompilerForegroundMeasurement `json:"foreground"`
	NextCursor    string                          `json:"next_cursor,omitempty"`
}

type CompilerDiagnosticJob struct {
	PublicationNanos        *int64 `json:"publication_nanos"`
	CandidateFreshnessNanos *int64 `json:"candidate_freshness_nanos"`
	CompilerWorkStatus
	QueuedAtUnixMS     *int64                       `json:"queued_at_unix_ms"`
	PublishedAtUnixMS  *int64                       `json:"published_at_unix_ms"`
	SelectedNewEvents  int64                        `json:"selected_new_events"`
	CompletedNewEvents int64                        `json:"completed_new_events"`
	Measurements       []CompilerAttemptMeasurement `json:"measurements"`
}

type CompilerAttemptMeasurement struct {
	Attempt                 int    `json:"attempt"`
	Fence                   int64  `json:"fence"`
	ClaimedAtUnixMS         int64  `json:"claimed_at_unix_ms"`
	QueueWaitNanos          *int64 `json:"queue_wait_nanos"`
	InferenceNanos          *int64 `json:"inference_nanos"`
	ValidationNanos         *int64 `json:"validation_nanos"`
	DatabaseCompletionNanos *int64 `json:"database_completion_nanos"`
	ObservedOutcome         string `json:"observed_outcome"` // incomplete, completed, failed, cancelled, stale
}

type CompilerDiagnosticCandidate struct {
	Ref               CandidateRef `json:"ref"`
	JobID             string       `json:"job_id"`
	GenerationID      string       `json:"generation_id"`
	ReviewState       string       `json:"review_state"`
	EquivalentTo      string       `json:"equivalent_to,omitempty"`
	PublishedAtUnixMS *int64       `json:"published_at_unix_ms"`
	DecidedAtUnixMS   *int64       `json:"decided_at_unix_ms"`
	Edited            bool         `json:"edited"`
}

type CompilerDiagnosticHistory struct {
	RequestID       string `json:"request_id"`
	RangeIndex      int    `json:"range_index"`
	GenerationID    string `json:"generation_id"`
	FirstSequence   int64  `json:"first_sequence"`
	LastSequence    int64  `json:"last_sequence"`
	ScannedSequence int64  `json:"scanned_sequence"`
	Revision        int64  `json:"revision"`
	Cancelled       bool   `json:"cancelled"`
}

type CompilerDiagnosticSelection struct {
	EventID    EventID `json:"event_id"`
	Sequence   int64   `json:"sequence"`
	Membership string  `json:"membership"` // selected_live, selected_history, outside_selection
}

// The host records actual observed boundaries. Terminal commit and response
// finalization are separate; missing observations remain null, including crashes.
// Active owner review measurements are collected by the pilot, never inferred.
type CompilerForegroundMeasurement struct {
	RootID                    EventID `json:"root_id"`
	StartedAtUnixMS           int64   `json:"started_at_unix_ms"`
	TerminalCommittedAtUnixMS *int64  `json:"terminal_committed_at_unix_ms"`
	TerminalCommitNanos       *int64  `json:"terminal_commit_nanos"`
	ResponseFinalizedAtUnixMS *int64  `json:"response_finalized_at_unix_ms"`
	ResponseFinalizationNanos *int64  `json:"response_finalization_nanos"`
	Outcome                   string  `json:"outcome"`
}

// A selection unit's sequence range bounds its exact source members. It does
// not imply that interleaved events or interpretation context were evidence.
type CompilerDiagnosticUnit struct {
	SelectionID       string `json:"selection_id"`
	GenerationID      string `json:"generation_id"`
	JobID             string `json:"job_id,omitempty"`
	FirstSequence     int64  `json:"first_sequence"`
	LastSequence      int64  `json:"last_sequence"`
	State             string `json:"state"`
	Reason            string `json:"reason,omitempty"`
	SelectedNewEvents int64  `json:"selected_new_events"`
}
type CompilerDiagnosticRoot struct {
	ActivationID  string  `json:"activation_id"`
	RootID        EventID `json:"root_id"`
	FirstSequence int64   `json:"first_sequence"`
	LastSequence  int64   `json:"last_sequence"`
	State         string  `json:"state"`
	Reason        string  `json:"reason,omitempty"`
	SelectionID   string  `json:"selection_id,omitempty"`
}
