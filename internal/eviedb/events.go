package eviedb

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
		var terminal memory.TurnTerminalPayload
		decoder := json.NewDecoder(bytes.NewReader(input.Payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&terminal); err != nil {
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
	}

	input.Payload = append(json.RawMessage(nil), input.Payload...)
	return input, nil
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
