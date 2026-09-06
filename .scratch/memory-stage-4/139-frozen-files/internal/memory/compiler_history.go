package memory

// CompilerHistoryRange is an explicit inclusive selection. Boundary event IDs
// prevent a stale or mistyped sequence from selecting a different episode.
type CompilerHistoryRange struct {
	SourceScope   string    `json:"source_scope"`
	Destination   string    `json:"destination"`
	SessionID     SessionID `json:"session_id"`
	FirstSequence int64     `json:"first_sequence"`
	LastSequence  int64     `json:"last_sequence"`
	FirstEventID  EventID   `json:"first_event_id"`
	LastEventID   EventID   `json:"last_event_id"`
}

type CompilerHistoryRequest struct {
	RequestID string                 `json:"request_id"`
	Ranges    []CompilerHistoryRange `json:"ranges"`
}

// The original receipt never changes after cancellation, resume, or appends.
type CompilerHistoryReceipt struct {
	Request        CompilerHistoryRequest `json:"request"`
	GenerationID   string                 `json:"generation_id"`
	SelectedEvents int64                  `json:"selected_events"`
	SelectionOrder int64                  `json:"selection_order"`
}

type CompilerHistoryChange struct {
	RequestID        string `json:"request_id"`
	OperationID      string `json:"operation_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type CompilerHistoryState struct {
	RequestID string `json:"request_id"`
	Revision  int64  `json:"revision"`
	Cancelled bool   `json:"cancelled"`
}

type CompilerHistoryInterval struct {
	FirstSequence int64  `json:"first_sequence"`
	LastSequence  int64  `json:"last_sequence"`
	State         string `json:"state"`
	Reason        string `json:"reason,omitempty"`
	JobID         string `json:"job_id,omitempty"`
	SelectionID   string `json:"selection_id,omitempty"`
	Attempts      int    `json:"attempts"`
	RetryAt       int64  `json:"retry_at,omitempty"`
}

// Progress is for one exact request range. Its frontier never crosses a gap or
// borrows a sequence from another session; exclusions remain separate outcomes.
type CompilerHistoryProgress struct {
	Receipt                CompilerHistoryReceipt    `json:"receipt"`
	RequestState           CompilerHistoryState      `json:"request_state"`
	RangeIndex             int                       `json:"range_index"`
	Intervals              []CompilerHistoryInterval `json:"intervals"`
	NextSequence           int64                     `json:"next_sequence,omitempty"`
	ContiguousFrontier     int64                     `json:"contiguous_frontier"`
	SelectedSessionEvents  int64                     `json:"selected_session_events"`
	OutsideSelectionEvents int64                     `json:"outside_selection_events"`
	SkipSupported          bool                      `json:"skip_supported"`
}
