package memory

import "time"

// A session episode is one stored summary per session: the latest accepted
// rolling compaction summary, kept as derived, searchable data. It is never
// standing context, never injected automatically, and never evidence for a
// Claim.
const (
	SessionEpisodeDefaultLimit = 3
	SessionEpisodeMaxLimit     = 5
)

// SessionEpisodeQuery contains caller requests, never authority. The Kernel
// resolves the readable area from the durable session.
type SessionEpisodeQuery struct {
	Text string
	// Limit is 1 through SessionEpisodeMaxLimit; zero selects the default.
	Limit int
	// Intent is current (default) or historical. Historical intent may return
	// a summary of sessions whose memory was later retired, labelled so.
	Intent string
}

// SessionEpisodeSource is the provenance of an episode: the accepted
// context_compacted event whose summary it is and the contiguous range of the
// session's events that summary covers.
type SessionEpisodeSource struct {
	SummaryEventID       EventID `json:"summary_event_id"`
	Generation           int64   `json:"generation"`
	CoveredFirstEventID  EventID `json:"covered_first_event_id"`
	CoveredFirstSequence int64   `json:"covered_first_sequence"`
	CoveredLastEventID   EventID `json:"covered_last_event_id"`
	CoveredLastSequence  int64   `json:"covered_last_sequence"`
}

type SessionEpisode struct {
	EpisodeID string    `json:"episode_id"`
	SessionID SessionID `json:"session_id"`
	ScopeKey  string    `json:"scope"`
	// StartedAt and EndedAt are the recorded times of the first and last
	// covered events; SummarizedAt is when the summary was accepted.
	StartedAt     time.Time            `json:"started_at"`
	EndedAt       time.Time            `json:"ended_at"`
	SummarizedAt  time.Time            `json:"summarized_at"`
	Source        SessionEpisodeSource `json:"source"`
	CurrentStatus SemanticObjectStatus `json:"current_status"`
	// HistoricalClaims names readable Claims, sourced inside the covered
	// range, that were later retired or superseded.
	HistoricalClaims []RetrievalHistoricalClaim `json:"historical_claims,omitempty"`
	Summary          string                     `json:"summary"`
	SummaryBytes     int                        `json:"summary_bytes"`
	SummaryTruncated bool                       `json:"summary_truncated,omitempty"`
}

type SessionEpisodeResult struct {
	Status   string           `json:"status"`
	Intent   string           `json:"intent"`
	Episodes []SessionEpisode `json:"episodes"`
	// Withheld counts matching episodes not returned because their covered
	// sources are retired (ordinary intent) or retracted, or because they
	// could not be verified as the session's latest summary.
	Withheld  int  `json:"withheld,omitempty"`
	Truncated bool `json:"truncated,omitempty"`
}
