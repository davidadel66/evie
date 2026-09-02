package eviedb

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

func normalizeEventInput(input memory.EventInput) (memory.EventInput, error) {
	if input.Type == "" {
		return memory.EventInput{}, errors.New("event type must not be empty")
	}

	switch input.Role {
	case "", memory.RoleUser, memory.RoleAssistant, memory.RoleTool:
	default:
		return memory.EventInput{}, fmt.Errorf("invalid event role %q", input.Role)
	}

	if len(input.Payload) == 0 {
		input.Payload = json.RawMessage(`{}`)
	}

	if !json.Valid(input.Payload) {
		return memory.EventInput{}, errors.New("event payload must be valid JSON")
	}

	if input.Type == memory.EventTurnFailed || input.Type == memory.EventTurnInterrupted {
		terminal, canonical, err := decodeCanonicalTerminalPayload(input.Payload)
		if err != nil {
			return memory.EventInput{}, fmt.Errorf("decode terminal payload: %w", err)
		}
		if err := terminal.Validate(input.Type); err != nil {
			return memory.EventInput{}, err
		}
		if input.Content != terminal.SafeContent() {
			return memory.EventInput{}, errors.New("terminal content must use the safe classification message")
		}
		if input.Role != "" || input.ExecutionID != "" {
			return memory.EventInput{}, errors.New("terminal events cannot carry a role or execution ID")
		}
		input.Payload = canonical
	}
	if input.Type == memory.EventContextSnapshot {
		snapshot, canonical, err := decodeCanonicalContextSnapshotPayload(input.Payload)
		if err != nil {
			return memory.EventInput{}, fmt.Errorf("decode context snapshot payload: %w", err)
		}
		if err := snapshot.Validate(); err != nil {
			return memory.EventInput{}, err
		}
		if input.ParentID == "" || input.Content != "" || input.Role != "" || input.ExecutionID != "" {
			return memory.EventInput{}, errors.New("context snapshots require a parent and cannot carry content, role, or execution ID")
		}
		input.Payload = canonical
	}
	if input.Type == memory.EventContextCompacted {
		compacted, canonical, err := decodeCanonicalContextCompactedPayload(input.Payload)
		if err != nil {
			return memory.EventInput{}, fmt.Errorf("decode context compaction payload: %w", err)
		}
		if err := compacted.Validate(input.Content); err != nil {
			return memory.EventInput{}, err
		}
		if input.ParentID != "" || input.Role != "" || input.ExecutionID != "" {
			return memory.EventInput{}, errors.New("context compactions cannot carry a parent, role, or execution ID")
		}
		input.Payload = canonical
	}

	input.Payload = append(json.RawMessage(nil), input.Payload...)
	return input, nil
}

func decodeCanonicalContextCompactedPayload(raw json.RawMessage) (memory.ContextCompactedPayload, json.RawMessage, error) {
	if err := rejectDuplicateTopLevelJSONFields(raw); err != nil {
		return memory.ContextCompactedPayload{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var compacted memory.ContextCompactedPayload
	if err := decoder.Decode(&compacted); err != nil {
		return memory.ContextCompactedPayload{}, nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return memory.ContextCompactedPayload{}, nil, err
	}
	canonical, err := json.Marshal(compacted)
	if err != nil {
		return memory.ContextCompactedPayload{}, nil, err
	}
	return compacted, canonical, nil
}

func decodeCanonicalContextSnapshotPayload(raw json.RawMessage) (memory.ContextSnapshotPayload, json.RawMessage, error) {
	if err := rejectDuplicateTopLevelJSONFields(raw); err != nil {
		return memory.ContextSnapshotPayload{}, nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var snapshot memory.ContextSnapshotPayload
	if err := decoder.Decode(&snapshot); err != nil {
		return memory.ContextSnapshotPayload{}, nil, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return memory.ContextSnapshotPayload{}, nil, err
	}
	canonical, err := json.Marshal(snapshot)
	if err != nil {
		return memory.ContextSnapshotPayload{}, nil, err
	}
	return snapshot, canonical, nil
}

func rejectDuplicateTopLevelJSONFields(raw json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return errors.New("payload must be a JSON object")
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("payload key must be a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("payload repeats field %q", key)
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	return ensureJSONEOF(decoder)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if token, err := decoder.Token(); err == nil {
		return fmt.Errorf("payload has trailing data %v", token)
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func decodeCanonicalTerminalPayload(raw json.RawMessage) (memory.TurnTerminalPayload, json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil {
		return memory.TurnTerminalPayload{}, nil, err
	}
	if delim, ok := opening.(json.Delim); !ok || delim != '{' {
		return memory.TurnTerminalPayload{}, nil, errors.New("terminal payload must be a JSON object")
	}
	var terminal memory.TurnTerminalPayload
	seen := make(map[string]struct{}, 4)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return memory.TurnTerminalPayload{}, nil, err
		}
		key, ok := token.(string)
		if !ok {
			return memory.TurnTerminalPayload{}, nil, errors.New("terminal payload key must be a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return memory.TurnTerminalPayload{}, nil, fmt.Errorf("terminal payload repeats field %q", key)
		}
		seen[key] = struct{}{}
		switch key {
		case "turn_id":
			err = decoder.Decode(&terminal.TurnID)
		case "classification":
			err = decoder.Decode(&terminal.Classification)
		case "stage":
			err = decoder.Decode(&terminal.Stage)
		case "http_status":
			err = decoder.Decode(&terminal.HTTPStatus)
			if err == nil && terminal.HTTPStatus == nil {
				return memory.TurnTerminalPayload{}, nil, errors.New("terminal http_status must be numeric when present")
			}
		default:
			return memory.TurnTerminalPayload{}, nil, fmt.Errorf("terminal payload contains unknown field %q", key)
		}
		if err != nil {
			return memory.TurnTerminalPayload{}, nil, err
		}
	}
	closing, err := decoder.Token()
	if err != nil {
		return memory.TurnTerminalPayload{}, nil, err
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return memory.TurnTerminalPayload{}, nil, errors.New("terminal payload object is not closed")
	}
	if token, err := decoder.Token(); err == nil {
		return memory.TurnTerminalPayload{}, nil, fmt.Errorf("terminal payload has trailing data %v", token)
	} else if !errors.Is(err, io.EOF) {
		return memory.TurnTerminalPayload{}, nil, fmt.Errorf("read terminal payload trailer: %w", err)
	}
	canonical, err := json.Marshal(terminal)
	if err != nil {
		return memory.TurnTerminalPayload{}, nil, err
	}
	return terminal, canonical, nil
}

// AppendEventWithLease performs the event mutation and both ownership fences in
// one BEGIN IMMEDIATE transaction. The executor cannot outlive the transaction
// or expose arbitrary SQL outside this package.
func (s *Store) AppendEventWithLease(
	ctx context.Context,
	sessionID memory.SessionID,
	holderID memory.LeaseHolderID,
	token memory.FencingToken,
	input memory.EventInput,
) (event memory.Event, err error) {
	err = s.withTurnLeaseWrite(ctx, sessionID, holderID, token, func(writer turnLeaseWriteExecutor) error {
		var appendErr error
		event, appendErr = s.appendEvent(ctx, writer, sessionID, input)
		return appendErr
	})
	if err != nil {
		return memory.Event{}, err
	}
	return event, nil
}

type eventQueryExecutor interface {
	queryRowContext(context.Context, string, ...any) rowScanner
}

func (s *Store) appendEvent(
	ctx context.Context,
	executor eventQueryExecutor,
	sessionID memory.SessionID,
	input memory.EventInput,
) (memory.Event, error) {
	input, err := normalizeEventInput(input)
	if err != nil {
		return memory.Event{}, err
	}
	if input.Type == memory.EventTurnFailed || input.Type == memory.EventTurnInterrupted {
		var terminal memory.TurnTerminalPayload
		if err := json.Unmarshal(input.Payload, &terminal); err != nil {
			return memory.Event{}, fmt.Errorf("decode canonical terminal payload: %w", err)
		}
		if err := validateTerminalCorrelation(ctx, executor, sessionID, input.ParentID, terminal.TurnID); err != nil {
			return memory.Event{}, err
		}
	}
	if input.Type == memory.EventContextSnapshot {
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(input.Payload, &snapshot); err != nil {
			return memory.Event{}, fmt.Errorf("decode canonical context snapshot payload: %w", err)
		}
		if err := validateContextSnapshotCorrelation(ctx, executor, sessionID, input.ParentID, snapshot); err != nil {
			return memory.Event{}, err
		}
	}
	if input.Type == memory.EventContextCompacted {
		var compacted memory.ContextCompactedPayload
		if err := json.Unmarshal(input.Payload, &compacted); err != nil {
			return memory.Event{}, fmt.Errorf("decode canonical context compaction payload: %w", err)
		}
		if err := validateContextCompactionCorrelation(ctx, executor, sessionID, compacted); err != nil {
			return memory.Event{}, err
		}
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return memory.Event{}, fmt.Errorf("generate event ID: %w", err)
	}

	now := time.Now().UTC()
	event := memory.Event{
		ID:            memory.EventID(id.String()),
		SessionID:     sessionID,
		ParentID:      input.ParentID,
		Type:          input.Type,
		Role:          input.Role,
		ExecutionID:   input.ExecutionID,
		Content:       input.Content,
		Payload:       input.Payload,
		RecordedAt:    now,
		FormatVersion: 1,
	}

	var workspaceID, projectID sql.NullString
	err = executor.queryRowContext(ctx, `
		INSERT INTO events (
		id, session_id, sequence, workspace_id, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version
		)
		SELECT ?, sessions.id,
		COALESCE((
		SELECT MAX(existing.sequence)
		FROM events AS existing
		WHERE existing.session_id = sessions.id
		), 0) + 1,
		sessions.workspace_id,
		sessions.project_id,
		NULLIF(?, ''),
		?,
		NULLIF(?, ''),
		NULLIF(?, ''),
		?,
		?,
		?,
		1
		FROM sessions
		WHERE sessions.id = ? AND sessions.status = ?
		RETURNING sequence, workspace_id, project_id
		`,
		event.ID,
		event.ParentID,
		event.Type,
		event.Role,
		event.ExecutionID,
		event.Content,
		string(event.Payload),
		event.RecordedAt.Format(time.RFC3339Nano),
		sessionID,
		memory.SessionActive,
	).Scan(&event.Sequence, &workspaceID, &projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Event{}, fmt.Errorf("session %q is missing or closed: %w", sessionID, err)
	}
	if err != nil {
		return memory.Event{}, fmt.Errorf("append event: %w", err)
	}
	if title := memory.SessionTitleCandidate(input.Type, input.Role, input.ParentID, input.Content); title != "" {
		if err := executor.queryRowContext(ctx, `
				UPDATE sessions
				SET title = ?
				WHERE id = ? AND title IS NULL
				RETURNING id
			`, title, sessionID).Scan(new(string)); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return memory.Event{}, fmt.Errorf("initialize session title: %w", err)
		}
	}

	if workspaceID.Valid {
		event.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	}
	if projectID.Valid {
		event.ProjectID = memory.ProjectID(projectID.String)
	}
	return event, nil
}

func validateContextCompactionCorrelation(
	ctx context.Context,
	executor eventQueryExecutor,
	sessionID memory.SessionID,
	payload memory.ContextCompactedPayload,
) error {
	var priorID, priorContent, priorPayloadJSON string
	priorErr := executor.queryRowContext(ctx, `
		SELECT id, content, payload_json
		FROM events
		WHERE session_id = ? AND event_type = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID, memory.EventContextCompacted).Scan(&priorID, &priorContent, &priorPayloadJSON)
	var priorPayload memory.ContextCompactedPayload
	switch {
	case errors.Is(priorErr, sql.ErrNoRows):
		if payload.Generation != 1 || payload.PriorCompactionEventID != "" {
			return errors.New("context compaction first generation is inconsistent")
		}
	case priorErr != nil:
		return fmt.Errorf("load prior context compaction: %w", priorErr)
	default:
		decoded, canonical, err := decodeCanonicalContextCompactedPayload(json.RawMessage(priorPayloadJSON))
		if err != nil || string(canonical) != priorPayloadJSON {
			return errors.New("prior context compaction payload is malformed")
		}
		if err := decoded.Validate(priorContent); err != nil {
			return fmt.Errorf("validate prior context compaction: %w", err)
		}
		priorPayload = decoded
		if err := payload.ValidateAdvance(memory.EventID(priorID), priorPayload); err != nil {
			return err
		}
	}

	type frontier struct {
		typeValue memory.EventType
		role      memory.EventRole
		parent    sql.NullString
		sequence  int64
		payload   json.RawMessage
	}
	load := func(id memory.EventID) (frontier, error) {
		var eventType, role, payloadJSON string
		var value frontier
		err := executor.queryRowContext(ctx, `
			SELECT event_type, COALESCE(role, ''), parent_id, sequence, payload_json
			FROM events WHERE session_id = ? AND id = ?
		`, sessionID, id).Scan(&eventType, &role, &value.parent, &value.sequence, &payloadJSON)
		if errors.Is(err, sql.ErrNoRows) {
			return frontier{}, fmt.Errorf("context compaction frontier event %q is not accepted in session %q", id, sessionID)
		}
		if err != nil {
			return frontier{}, fmt.Errorf("load context compaction frontier event %q: %w", id, err)
		}
		value.typeValue = memory.EventType(eventType)
		value.role = memory.EventRole(role)
		value.payload = json.RawMessage(payloadJSON)
		return value, nil
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
	if first.typeValue != memory.EventUserMessage || first.role != memory.RoleUser || first.parent.Valid ||
		first.sequence != payload.CoveredFirstSequence {
		return errors.New("context compaction covered frontier does not start at a root turn")
	}
	if priorErr == nil {
		if payload.CoveredFirstEventID == priorPayload.CoveredFirstEventID {
			return errors.New("context compaction covered frontier overlaps or skips the prior frontier")
		}
	} else if first.sequence != 1 {
		return errors.New("context compaction first generation does not start at the first root turn")
	}
	if last.sequence != payload.CoveredLastSequence || retained.sequence <= last.sequence ||
		retained.typeValue != memory.EventUserMessage || retained.role != memory.RoleUser || retained.parent.Valid {
		return errors.New("context compaction retained frontier is invalid")
	}
	var nextRootID string
	var nextRootSequence int64
	if err := executor.queryRowContext(ctx, `
		SELECT id, sequence
		FROM events
		WHERE session_id = ? AND sequence > ? AND event_type = ? AND role = ? AND parent_id IS NULL
		ORDER BY sequence
		LIMIT 1
	`, sessionID, last.sequence, memory.EventUserMessage, memory.RoleUser).Scan(&nextRootID, &nextRootSequence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("context compaction retained frontier has no next root turn")
		}
		return fmt.Errorf("load next retained root turn: %w", err)
	}
	if memory.EventID(nextRootID) != payload.FirstRetainedEventID || nextRootSequence != retained.sequence {
		return errors.New("context compaction retained frontier is not the next root turn")
	}
	if last.typeValue == memory.EventAssistantMessage {
		var assistant memory.AssistantMessagePayload
		if err := json.Unmarshal(last.payload, &assistant); err != nil {
			return fmt.Errorf("decode covered terminal assistant payload: %w", err)
		}
		if len(assistant.ToolCalls) != 0 {
			return errors.New("context compaction covered frontier ends with an unfinished tool call")
		}
	} else if last.typeValue != memory.EventTurnFailed && last.typeValue != memory.EventTurnInterrupted {
		return errors.New("context compaction covered frontier does not end at a completed turn")
	}
	return nil
}

func validateContextSnapshotCorrelation(
	ctx context.Context,
	executor eventQueryExecutor,
	sessionID memory.SessionID,
	parentID memory.EventID,
	snapshot memory.ContextSnapshotPayload,
) error {
	var parentType string
	var parentSequence int64
	err := executor.queryRowContext(ctx, `
		SELECT event_type, sequence
		FROM events
		WHERE session_id = ? AND id = ?
	`, sessionID, parentID).Scan(&parentType, &parentSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("context snapshot parent ID %q is not an accepted event in session %q", parentID, sessionID)
	}
	if err != nil {
		return fmt.Errorf("validate context snapshot parent: %w", err)
	}
	parentEventType := memory.EventType(parentType)
	if parentEventType != memory.EventUserMessage && parentEventType != memory.EventToolSucceeded &&
		parentEventType != memory.EventToolFailed && parentEventType != memory.EventToolCancelled {
		return fmt.Errorf("context snapshot parent %q is not a durable provider trigger", parentID)
	}
	if snapshot.RetainedLastEventID != parentID || snapshot.RetainedLastSequence != parentSequence {
		return errors.New("context snapshot retained endpoint does not match its provider trigger")
	}
	var latestEventID, latestEventType, latestPayloadJSON string
	var latestSequence int64
	err = executor.queryRowContext(ctx, `
		SELECT id, event_type, sequence, payload_json
		FROM events
		WHERE session_id = ?
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID).Scan(&latestEventID, &latestEventType, &latestSequence, &latestPayloadJSON)
	if err != nil {
		return fmt.Errorf("validate latest context snapshot trigger: %w", err)
	}
	if memory.EventID(latestEventID) != parentID {
		if memory.EventID(latestEventID) != snapshot.ActiveCompactionEventID ||
			memory.EventType(latestEventType) != memory.EventContextCompacted || latestSequence != parentSequence+1 {
			return errors.New("context snapshot does not immediately follow its provider trigger or active automatic compaction")
		}
		compacted, _, err := decodeCanonicalContextCompactedPayload(json.RawMessage(latestPayloadJSON))
		if err != nil || compacted.Trigger != memory.ContextCompactionAutomatic {
			return errors.New("context snapshot intervening compaction is invalid")
		}
		if snapshot.RetainedFirstEventID != compacted.FirstRetainedEventID {
			return errors.New("context snapshot retained frontier does not match its automatic compaction")
		}
	}
	var firstType, firstRole string
	var firstParent sql.NullString
	var firstSequence int64
	err = executor.queryRowContext(ctx, `
		SELECT event_type, COALESCE(role, ''), parent_id, sequence
		FROM events
		WHERE session_id = ? AND id = ?
	`, sessionID, snapshot.RetainedFirstEventID).Scan(&firstType, &firstRole, &firstParent, &firstSequence)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("context snapshot retained first event %q is not accepted in session %q", snapshot.RetainedFirstEventID, sessionID)
	}
	if err != nil {
		return fmt.Errorf("validate context snapshot retained frontier: %w", err)
	}
	if memory.EventType(firstType) != memory.EventUserMessage || memory.EventRole(firstRole) != memory.RoleUser ||
		firstParent.Valid || firstSequence != snapshot.RetainedFirstSequence || firstSequence > parentSequence {
		return errors.New("context snapshot retained starting frontier is inconsistent")
	}
	var previousPlaceholderSequence int64
	for _, placeholder := range snapshot.Placeholders {
		var eventType string
		var content string
		var sequence int64
		err := executor.queryRowContext(ctx, `
			SELECT event_type, content, sequence
			FROM events
			WHERE session_id = ? AND id = ?
		`, sessionID, placeholder.EventID).Scan(&eventType, &content, &sequence)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("context snapshot placeholder event %q is not accepted in session %q", placeholder.EventID, sessionID)
		}
		if err != nil {
			return fmt.Errorf("validate context snapshot placeholder: %w", err)
		}
		typeValue := memory.EventType(eventType)
		if (typeValue != memory.EventToolSucceeded && typeValue != memory.EventToolFailed &&
			typeValue != memory.EventToolCancelled) || sequence < firstSequence || sequence > parentSequence ||
			sequence <= previousPlaceholderSequence {
			return fmt.Errorf("context snapshot placeholder event %q is outside the retained ordered tool results", placeholder.EventID)
		}
		digest := sha256.Sum256([]byte(content))
		if placeholder.OriginalBytes != int64(len(content)) || placeholder.SHA256 != fmt.Sprintf("%x", digest) {
			return fmt.Errorf("context snapshot placeholder event %q does not match durable content", placeholder.EventID)
		}
		previousPlaceholderSequence = sequence
	}
	return nil
}

func validateTerminalCorrelation(
	ctx context.Context,
	executor eventQueryExecutor,
	sessionID memory.SessionID,
	parentID memory.EventID,
	turnID memory.EventID,
) error {
	if parentID == "" {
		return errors.New("terminal event must have a parent")
	}
	var rootType, rootRole string
	var rootParent sql.NullString
	err := executor.queryRowContext(ctx, `
		SELECT event_type, COALESCE(role, ''), parent_id
		FROM events
		WHERE session_id = ? AND id = ?
	`, sessionID, turnID).Scan(&rootType, &rootRole, &rootParent)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("terminal turn ID %q is not an accepted event in session %q", turnID, sessionID)
	}
	if err != nil {
		return fmt.Errorf("validate terminal turn root: %w", err)
	}
	if memory.EventType(rootType) != memory.EventUserMessage ||
		memory.EventRole(rootRole) != memory.RoleUser || rootParent.Valid {
		return fmt.Errorf("terminal turn ID %q is not a root user event", turnID)
	}
	var latestRootID string
	err = executor.queryRowContext(ctx, `
		SELECT id
		FROM events
		WHERE session_id = ? AND event_type = ? AND role = ? AND parent_id IS NULL
		ORDER BY sequence DESC
		LIMIT 1
	`, sessionID, memory.EventUserMessage, memory.RoleUser).Scan(&latestRootID)
	if err != nil {
		return fmt.Errorf("validate latest terminal turn root: %w", err)
	}
	if memory.EventID(latestRootID) != turnID {
		return fmt.Errorf("terminal turn ID %q is not the latest accepted turn root", turnID)
	}

	var parentType string
	err = executor.queryRowContext(ctx, `
		SELECT event_type
		FROM events
		WHERE session_id = ? AND id = ?
	`, sessionID, parentID).Scan(&parentType)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("terminal parent ID %q is not an accepted event in session %q", parentID, sessionID)
	}
	if err != nil {
		return fmt.Errorf("validate terminal parent: %w", err)
	}
	parentEventType := memory.EventType(parentType)
	if parentID == turnID {
		if parentEventType != memory.EventUserMessage {
			return errors.New("terminal root parent is not a user event")
		}
	} else if parentEventType != memory.EventToolSucceeded &&
		parentEventType != memory.EventToolFailed &&
		parentEventType != memory.EventToolCancelled {
		return fmt.Errorf("terminal parent %q is not a durable provider trigger", parentID)
	}

	var connected bool
	err = executor.queryRowContext(ctx, `
		WITH RECURSIVE lineage(id, parent_id) AS (
			SELECT id, parent_id FROM events WHERE session_id = ? AND id = ?
			UNION
			SELECT parent.id, parent.parent_id
			FROM events AS parent
			JOIN lineage AS child ON parent.id = child.parent_id
			WHERE parent.session_id = ?
		)
		SELECT EXISTS(SELECT 1 FROM lineage WHERE id = ?)
	`, sessionID, parentID, sessionID, turnID).Scan(&connected)
	if err != nil {
		return fmt.Errorf("validate terminal ancestry: %w", err)
	}
	if !connected {
		return fmt.Errorf("terminal parent %q is not descended from turn root %q", parentID, turnID)
	}
	return nil
}

func (s *Store) LoadEvents(
	ctx context.Context,
	sessionID memory.SessionID,
) ([]memory.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, sequence, workspace_id, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version FROM events WHERE session_id = ?
		ORDER BY sequence
		`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}

	defer rows.Close()

	var events []memory.Event
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read events: %w", err)
	}
	return events, nil
}

func scanEvent(scanner rowScanner) (memory.Event, error) {
	var (
		id, sessionID, eventType                            string
		content, payloadText, recordedText                  string
		sequence                                            int64
		formatVersion                                       int
		workspaceID, projectID, parentID, role, executionID sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&sessionID,
		&sequence,
		&workspaceID,
		&projectID,
		&parentID,
		&eventType,
		&role,
		&executionID,
		&content,
		&payloadText,
		&recordedText,
		&formatVersion,
	); err != nil {
		return memory.Event{}, err
	}

	recordedAt, err := time.Parse(time.RFC3339Nano, recordedText)
	if err != nil {
		return memory.Event{}, fmt.Errorf("parse event recorded_at: %w", err)
	}

	event := memory.Event{
		ID:            memory.EventID(id),
		SessionID:     memory.SessionID(sessionID),
		Sequence:      sequence,
		Type:          memory.EventType(eventType),
		Content:       content,
		Payload:       json.RawMessage([]byte(payloadText)),
		RecordedAt:    recordedAt,
		FormatVersion: formatVersion,
	}
	if workspaceID.Valid {
		event.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	}
	if projectID.Valid {
		event.ProjectID = memory.ProjectID(projectID.String)
	}
	if parentID.Valid {
		event.ParentID = memory.EventID(parentID.String)
	}
	if role.Valid {
		event.Role = memory.EventRole(role.String)
	}
	if executionID.Valid {
		event.ExecutionID = memory.ExecutionID(executionID.String)
	}

	return event, nil
}
