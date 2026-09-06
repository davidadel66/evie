package memory

// CompilerWorkStatus is a content-free bounded recovery view. It remains usable
// when raw evidence is corrupt or current source rendering is forbidden.
type CompilerWorkStatus struct {
	JobID         string    `json:"job_id"`
	GenerationID  string    `json:"generation_id"`
	SessionID     SessionID `json:"session_id"`
	FirstSequence int64     `json:"first_sequence"`
	LastSequence  int64     `json:"last_sequence"`
	State         string    `json:"state"`
	Reason        string    `json:"reason,omitempty"`
	Attempts      int       `json:"attempts"`
	RetryAt       int64     `json:"retry_at,omitempty"`
	PauseReason   string    `json:"pause_reason,omitempty"`
	Lane          string    `json:"lane"`
	Recovery      string    `json:"recovery,omitempty"`
}

type CompilerStatus struct {
	Jobs               []CompilerWorkStatus `json:"jobs"`
	NextJobID          string               `json:"next_job_id,omitempty"`
	CapacityState      string               `json:"capacity_state"`
	CapacityRecovery   string               `json:"capacity_recovery,omitempty"`
	ReservedStages     int64                `json:"reserved_stages"`
	ReservedStageBytes int64                `json:"reserved_stage_bytes"`
	ReservedCandidates int64                `json:"reserved_candidates"`
}
