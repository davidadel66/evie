package eviedb

import (
	"bytes"
	"context"
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

	input.Payload = append(json.RawMessage(nil), input.Payload...)
	return input, nil
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

	var projectID sql.NullString
	err = executor.queryRowContext(ctx, `
		INSERT INTO events (
		id, session_id, sequence, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version
		)
		SELECT ?, sessions.id,
		COALESCE((
		SELECT MAX(existing.sequence)
		FROM events AS existing
		WHERE existing.session_id = sessions.id
		), 0) + 1,
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
		RETURNING sequence, project_id
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
	).Scan(&event.Sequence, &projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.Event{}, fmt.Errorf("session %q is missing or closed: %w", sessionID, err)
	}
	if err != nil {
		return memory.Event{}, fmt.Errorf("append event: %w", err)
	}

	if projectID.Valid {
		event.ProjectID = memory.ProjectID(projectID.String)
	}
	return event, nil
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
		SELECT id, session_id, sequence, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version FROM events WHERE session_id = ? 
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
		id, sessionID, eventType               string
		content, payloadText, recordedText     string
		sequence                               int64
		formatVersion                          int
		projectID, parentID, role, executionID sql.NullString
	)

	if err := scanner.Scan(
		&id,
		&sessionID,
		&sequence,
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
