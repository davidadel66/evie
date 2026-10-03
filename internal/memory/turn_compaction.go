package memory

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

// EventContextTurnCompacted records an in-turn compaction (memory
// architecture Stage 8): an accepted summary of the oldest completed tool
// groups of one root turn. It is a separate event type from the rolling
// context_compacted chain, which covers only whole root turns, so session
// episodes and whole-turn compaction keep their exact meaning.
const EventContextTurnCompacted EventType = "context_turn_compacted"

const (
	ContextTurnCompactedSchemaVersion  = 1
	ContextTurnCompactionPromptVersion = "turn-compaction-v1"
)

// ContextTurnCompactedPayload is the content-free provenance of one in-turn
// compaction. The summary is the event content. The covered frontier is a
// contiguous prefix of whole tool groups of the root turn TurnID: it starts at
// an assistant tool-call event, ends at the final result of a group, and
// FirstRetainedEventID is the assistant event of the next group. Each
// generation re-summarizes the prior one of the same turn, so CoveredFirst of
// a later generation is the prior generation's retained frontier.
type ContextTurnCompactedPayload struct {
	SchemaVersion          int     `json:"schema_version"`
	TurnID                 EventID `json:"turn_id"`
	Generation             int64   `json:"generation"`
	PriorCompactionEventID EventID `json:"prior_compaction_event_id,omitempty"`
	CoveredFirstEventID    EventID `json:"covered_first_event_id"`
	CoveredFirstSequence   int64   `json:"covered_first_sequence"`
	CoveredLastEventID     EventID `json:"covered_last_event_id"`
	CoveredLastSequence    int64   `json:"covered_last_sequence"`
	FirstRetainedEventID   EventID `json:"first_retained_event_id"`
	FirstRetainedSequence  int64   `json:"first_retained_sequence"`
	CanonicalModel         string  `json:"canonical_model"`
	PromptVersion          string  `json:"prompt_version"`
	SummaryBytes           int64   `json:"summary_bytes"`
	SummarySHA256          string  `json:"summary_sha256"`
}

func (p ContextTurnCompactedPayload) Validate(summary string) error {
	if p.SchemaVersion != ContextTurnCompactedSchemaVersion {
		return fmt.Errorf("unsupported in-turn compaction schema version %d", p.SchemaVersion)
	}
	if p.TurnID == "" {
		return errors.New("in-turn compaction turn must be present")
	}
	if p.Generation <= 0 {
		return errors.New("in-turn compaction generation must be positive")
	}
	if (p.Generation == 1) != (p.PriorCompactionEventID == "") {
		return errors.New("in-turn compaction generation and prior identity are inconsistent")
	}
	// A covered group has an assistant event and at least one result, and
	// the retained frontier starts a later group.
	if p.CoveredFirstEventID == "" || p.CoveredLastEventID == "" || p.FirstRetainedEventID == "" ||
		p.CoveredFirstSequence <= 0 || p.CoveredLastSequence <= p.CoveredFirstSequence ||
		p.FirstRetainedSequence <= p.CoveredLastSequence {
		return errors.New("in-turn compaction frontier is invalid")
	}
	if strings.TrimSpace(p.CanonicalModel) == "" || strings.TrimSpace(p.PromptVersion) == "" {
		return errors.New("in-turn compaction model and prompt provenance must be present")
	}
	if err := ValidateContextCompactionSummary(summary); err != nil {
		return err
	}
	if p.SummaryBytes != int64(len(summary)) {
		return errors.New("in-turn compaction summary byte count is invalid")
	}
	digest := sha256.Sum256([]byte(summary))
	if !validSHA256(p.SummarySHA256) || p.SummarySHA256 != fmt.Sprintf("%x", digest) {
		return errors.New("in-turn compaction summary hash is invalid")
	}
	return nil
}

// ValidateAdvance checks the exact link to the prior generation of the same
// turn, shared by persistence admission and history reconstruction.
func (p ContextTurnCompactedPayload) ValidateAdvance(priorEventID EventID, prior ContextTurnCompactedPayload) error {
	if priorEventID == "" || p.TurnID != prior.TurnID || p.Generation != prior.Generation+1 ||
		p.PriorCompactionEventID != priorEventID ||
		p.CoveredFirstEventID != prior.FirstRetainedEventID ||
		p.CoveredFirstSequence != prior.FirstRetainedSequence {
		return errors.New("in-turn compaction does not exactly advance the turn's prior generation")
	}
	return nil
}
