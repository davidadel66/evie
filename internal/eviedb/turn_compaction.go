package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

// validateContextTurnCompactionCorrelation admits an in-turn compaction
// (Stage 8) only for the newest root turn, immediately after the provider
// trigger it was made for (or after that trigger's rejected request snapshot,
// the C8 compact-and-retry), covering a contiguous prefix of whole, complete
// tool groups that the turn's prior generation did not cover, and retaining
// at least the group whose result triggered it. Agent history validation
// checks the same frontier against the complete group structure.
func validateContextTurnCompactionCorrelation(
	ctx context.Context,
	executor eventQueryExecutor,
	sessionID memory.SessionID,
	parentID memory.EventID,
	payload memory.ContextTurnCompactedPayload,
) error {
	var rootID string
	var rootSequence int64
	if err := executor.queryRowContext(ctx, `
		SELECT id, sequence
		FROM events
		WHERE session_id = ? AND event_type = ? AND role = ? AND parent_id IS NULL
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID, memory.EventUserMessage, memory.RoleUser).Scan(&rootID, &rootSequence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("in-turn compaction has no root turn")
		}
		return fmt.Errorf("load in-turn compaction root: %w", err)
	}
	if memory.EventID(rootID) != payload.TurnID {
		return errors.New("in-turn compaction does not belong to the newest root turn")
	}

	type turnEvent struct {
		typeValue memory.EventType
		parent    string
		sequence  int64
	}
	load := func(id memory.EventID) (turnEvent, error) {
		var value turnEvent
		var eventType string
		err := executor.queryRowContext(ctx, `
			SELECT event_type, COALESCE(parent_id, ''), sequence
			FROM events WHERE session_id = ? AND id = ?
		`, sessionID, id).Scan(&eventType, &value.parent, &value.sequence)
		if errors.Is(err, sql.ErrNoRows) {
			return turnEvent{}, fmt.Errorf("in-turn compaction event %q is not accepted in session %q", id, sessionID)
		}
		if err != nil {
			return turnEvent{}, fmt.Errorf("load in-turn compaction event %q: %w", id, err)
		}
		value.typeValue = memory.EventType(eventType)
		return value, nil
	}
	isToolResult := func(eventType memory.EventType) bool {
		return eventType == memory.EventToolSucceeded || eventType == memory.EventToolFailed || eventType == memory.EventToolCancelled
	}

	parent, err := load(parentID)
	if err != nil {
		return err
	}
	if !isToolResult(parent.typeValue) || parent.sequence <= rootSequence {
		return errors.New("in-turn compaction parent is not a provider trigger of its turn")
	}
	var latestID, latestType, latestParent string
	var latestSequence int64
	if err := executor.queryRowContext(ctx, `
		SELECT id, event_type, COALESCE(parent_id, ''), sequence
		FROM events
		WHERE session_id = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID).Scan(&latestID, &latestType, &latestParent, &latestSequence); err != nil {
		return fmt.Errorf("load latest event: %w", err)
	}
	rejectedRequest := memory.EventType(latestType) == memory.EventContextSnapshot &&
		memory.EventID(latestParent) == parentID && latestSequence == parent.sequence+1
	if memory.EventID(latestID) != parentID && !rejectedRequest {
		return errors.New("in-turn compaction does not immediately follow its provider trigger")
	}

	var priorID, priorContent, priorPayloadJSON string
	priorErr := executor.queryRowContext(ctx, `
		SELECT id, content, payload_json
		FROM events
		WHERE session_id = ? AND event_type = ? AND sequence > ?
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID, memory.EventContextTurnCompacted, rootSequence).Scan(&priorID, &priorContent, &priorPayloadJSON)
	switch {
	case errors.Is(priorErr, sql.ErrNoRows):
		if payload.Generation != 1 || payload.PriorCompactionEventID != "" {
			return errors.New("in-turn compaction first generation is inconsistent")
		}
		var firstGroup string
		if err := executor.queryRowContext(ctx, `
			SELECT id
			FROM events
			WHERE session_id = ? AND sequence > ? AND event_type = ?
			ORDER BY sequence
			LIMIT 1
		`, sessionID, rootSequence, memory.EventAssistantMessage).Scan(&firstGroup); err != nil {
			return fmt.Errorf("load first tool group of the turn: %w", err)
		}
		if memory.EventID(firstGroup) != payload.CoveredFirstEventID {
			return errors.New("in-turn compaction first generation does not start at the turn's first tool group")
		}
	case priorErr != nil:
		return fmt.Errorf("load prior in-turn compaction: %w", priorErr)
	default:
		prior, canonical, err := decodeCanonicalContextTurnCompactedPayload(json.RawMessage(priorPayloadJSON))
		if err != nil || string(canonical) != priorPayloadJSON {
			return errors.New("prior in-turn compaction payload is malformed")
		}
		if err := prior.Validate(priorContent); err != nil {
			return fmt.Errorf("validate prior in-turn compaction: %w", err)
		}
		if err := payload.ValidateAdvance(memory.EventID(priorID), prior); err != nil {
			return err
		}
	}

	first, err := load(payload.CoveredFirstEventID)
	if err != nil {
		return err
	}
	last, err := load(payload.CoveredLastEventID)
	if err != nil {
		return err
	}
	retained, err := load(payload.FirstRetainedEventID)
	if err != nil {
		return err
	}
	if first.typeValue != memory.EventAssistantMessage || first.sequence != payload.CoveredFirstSequence ||
		first.sequence <= rootSequence {
		return errors.New("in-turn compaction covered frontier does not start a tool group of its turn")
	}
	if !isToolResult(last.typeValue) || last.sequence != payload.CoveredLastSequence {
		return errors.New("in-turn compaction covered frontier does not end at a tool result")
	}
	// The retained group starts before the trigger, so the trigger's own
	// group is never covered.
	if retained.typeValue != memory.EventAssistantMessage || retained.sequence != payload.FirstRetainedSequence ||
		retained.sequence >= parent.sequence {
		return errors.New("in-turn compaction retained frontier is invalid")
	}
	// The covered frontier ends its group: the next assistant event is the
	// retained frontier and no tool result lies between them.
	var nextGroup string
	if err := executor.queryRowContext(ctx, `
		SELECT id
		FROM events
		WHERE session_id = ? AND sequence > ? AND event_type = ?
		ORDER BY sequence
		LIMIT 1
	`, sessionID, last.sequence, memory.EventAssistantMessage).Scan(&nextGroup); err != nil {
		return fmt.Errorf("load next tool group: %w", err)
	}
	var between int64
	if err := executor.queryRowContext(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE session_id = ? AND sequence > ? AND sequence < ? AND event_type IN (?, ?, ?)
	`, sessionID, last.sequence, retained.sequence,
		memory.EventToolSucceeded, memory.EventToolFailed, memory.EventToolCancelled).Scan(&between); err != nil {
		return fmt.Errorf("load covered tool group tail: %w", err)
	}
	if memory.EventID(nextGroup) != payload.FirstRetainedEventID || between != 0 {
		return errors.New("in-turn compaction covered frontier splits a tool group")
	}
	// Every covered group is complete: each assistant event requested tools,
	// and the covered range holds exactly one result per requested call.
	var assistants, toolless, calls, results int64
	if err := executor.queryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN COALESCE(json_array_length(payload_json, '$.tool_calls'), 0) = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(COALESCE(json_array_length(payload_json, '$.tool_calls'), 0)), 0)
		FROM events
		WHERE session_id = ? AND sequence BETWEEN ? AND ? AND event_type = ?
	`, sessionID, first.sequence, last.sequence, memory.EventAssistantMessage).Scan(&assistants, &toolless, &calls); err != nil {
		return fmt.Errorf("load covered tool groups: %w", err)
	}
	if err := executor.queryRowContext(ctx, `
		SELECT COUNT(*)
		FROM events
		WHERE session_id = ? AND sequence BETWEEN ? AND ? AND event_type IN (?, ?, ?)
	`, sessionID, first.sequence, last.sequence,
		memory.EventToolSucceeded, memory.EventToolFailed, memory.EventToolCancelled).Scan(&results); err != nil {
		return fmt.Errorf("load covered tool results: %w", err)
	}
	if assistants == 0 || toolless != 0 || calls != results {
		return errors.New("in-turn compaction covers an incomplete tool group")
	}
	return nil
}
