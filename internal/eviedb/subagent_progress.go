package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// SubagentProgress is one delegated child as the owner watches it from the
// parent conversation (chat-activity.spec.md, subagent inspector amendment):
// what it was asked, where it is, and the last action it requested. It is a
// read-only view; nothing here grants the child or the parent anything.
type SubagentProgress struct {
	ExecutionID string     `json:"executionId"`
	Worker      string     `json:"worker"`
	Key         string     `json:"key"`
	Objective   string     `json:"objective"`
	Context     string     `json:"context,omitempty"`
	State       string     `json:"state"`
	Continues   string     `json:"continues,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	// DeadlineMS is the child's wall-clock budget from its pinned policy.
	DeadlineMS int64 `json:"deadlineMs,omitempty"`
	// Actions counts the tool calls the child requested in this attempt.
	Actions int             `json:"actions"`
	Latest  *SubagentAction `json:"latest,omitempty"`
}

// SubagentAction is the child's latest requested tool call. Done is true once
// any outcome (success, failure or cancellation) is recorded for it.
type SubagentAction struct {
	Name string `json:"name"`
	Args string `json:"args"`
	Done bool   `json:"done"`
}

// subagentActionArgsBytes bounds the latest action's arguments in a listing;
// the row shows a short subject, and the full call is in the child's trace.
const subagentActionArgsBytes = 1024

// subagentCallTools are the parent tools whose calls start or continue children.
var subagentCallTools = []string{delegation.ToolName, delegation.ExploreToolName, delegation.BuildToolName, delegation.ContinueToolName}

// SubagentsOfCall lists the children that one delegation call of session
// admitted, continued or resolved by idempotency key, in assignment order.
// Provider call IDs can repeat across turns, so turn (the root user event the
// call belongs to) picks the call; empty takes the session's latest. A call
// that delegated nothing, or another session's call, lists none.
func (s *Store) SubagentsOfCall(ctx context.Context, session memory.SessionID, turn memory.EventID, callID string) ([]SubagentProgress, error) {
	var list []SubagentProgress
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		var intentID memory.EventID
		var args string
		names, err := json.Marshal(subagentCallTools)
		if err != nil {
			return err
		}
		err = conn.QueryRowContext(ctx, `WITH root AS (SELECT sequence FROM events WHERE id=?1 AND session_id=?2 AND event_type='user_message'),
 call AS (SELECT id, sequence, json_extract(payload_json,'$.call.arguments') AS args FROM events
  WHERE session_id=?2 AND event_type='tool_intent' AND json_extract(payload_json,'$.call.id')=?3
  AND json_extract(payload_json,'$.call.name') IN (SELECT value FROM json_each(?4)))
 SELECT id, args FROM call WHERE ?1='' OR (sequence>(SELECT sequence FROM root) AND NOT EXISTS (SELECT 1 FROM events u
  WHERE u.session_id=?2 AND u.event_type='user_message' AND u.sequence>(SELECT sequence FROM root) AND u.sequence<call.sequence))
 ORDER BY sequence DESC LIMIT 1`, turn, session, callID, string(names)).Scan(&intentID, &args)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("find delegation call: %w", err)
		}
		// A repeated key resolves to the attempt an earlier call admitted, so
		// the call's own keys find replayed children too, but only an identical
		// assignment: a call refused for reusing a key started nothing.
		var request struct {
			Assignments []delegation.Assignment `json:"assignments"`
		}
		_ = json.Unmarshal([]byte(args), &request)
		keys := make([]string, 0, len(request.Assignments))
		for _, assignment := range request.Assignments {
			keys = append(keys, assignment.Key)
		}
		keysJSON, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, `SELECT record_json FROM subagent_executions WHERE parent_session_id=?
 AND (json_extract(record_json,'$.parent.IntentEventID')=? OR idempotency_key IN (SELECT value FROM json_each(?))) ORDER BY rowid`, session, intentID, string(keysJSON))
		if err != nil {
			return fmt.Errorf("list delegated children: %w", err)
		}
		var attempts []delegation.Attempt
		for rows.Next() {
			a, err := readSubagent(rows)
			if err != nil {
				rows.Close()
				return err
			}
			if a.Parent.IntentEventID != intentID && !slices.Contains(request.Assignments, a.Assignment) {
				continue
			}
			attempts = append(attempts, a)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if err = rows.Err(); err != nil {
			return err
		}
		order := func(a delegation.Attempt) int {
			if i := slices.Index(keys, a.Assignment.Key); i >= 0 {
				return i
			}
			return len(keys)
		}
		slices.SortStableFunc(attempts, func(a, b delegation.Attempt) int { return order(a) - order(b) })
		for _, a := range attempts {
			progress, err := subagentProgress(ctx, conn, a)
			if err != nil {
				return err
			}
			list = append(list, progress)
		}
		return nil
	})
	return list, err
}

// SubagentEvents returns one child of session with the child-session events
// that belong to that attempt, for display. Another session's child is
// indistinguishable from a missing one (sql.ErrNoRows).
func (s *Store) SubagentEvents(ctx context.Context, session memory.SessionID, executionID string) (SubagentProgress, []memory.Event, error) {
	var progress SubagentProgress
	var events []memory.Event
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=? AND parent_session_id=?`, executionID, session))
		if err != nil {
			return err
		}
		if progress, err = subagentProgress(ctx, conn, a); err != nil {
			return err
		}
		after, until, err := attemptRange(ctx, conn, a)
		if err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, `SELECT id, session_id, sequence, workspace_id, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version
 FROM events WHERE session_id=? AND sequence>? AND sequence<=? ORDER BY sequence`, a.Child.ID, after, until)
		if err != nil {
			return fmt.Errorf("load child events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanEvent(rows)
			if err != nil {
				return err
			}
			events = append(events, event)
		}
		return rows.Err()
	})
	return progress, events, err
}

// attemptRange is the child sequences an attempt's events occupy, (after,
// until]: from the start of the child (an original assignment) or of its own
// turn (a continuation), up to where the next continuation of the same child
// began, if one has.
func attemptRange(ctx context.Context, conn *sql.Conn, a delegation.Attempt) (int64, int64, error) {
	var after int64
	if a.Continues != nil {
		after = a.Continues.AfterSequence
	}
	var next sql.NullInt64
	err := conn.QueryRowContext(ctx, `SELECT MIN(json_extract(record_json,'$.continues.after_sequence')) FROM subagent_executions
 WHERE child_session_id=? AND id<>? AND json_extract(record_json,'$.continues.after_sequence')>?`, a.Child.ID, a.ID, after).Scan(&next)
	if err != nil {
		return 0, 0, fmt.Errorf("bound child attempt: %w", err)
	}
	if !next.Valid {
		return after, math.MaxInt64, nil
	}
	return after, next.Int64, nil
}

func subagentProgress(ctx context.Context, conn *sql.Conn, a delegation.Attempt) (SubagentProgress, error) {
	p := SubagentProgress{
		ExecutionID: a.ID, Worker: a.Worker(), Key: a.Assignment.Key, Objective: a.Assignment.Objective, Context: a.Assignment.Context,
		State: a.State, CreatedAt: a.CreatedAt, StartedAt: a.StartedAt, EndedAt: a.EndedAt,
		DeadlineMS: a.Policy.Deadline.Milliseconds(),
	}
	if a.Continues != nil {
		p.Continues = a.Continues.ExecutionID
	}
	after, until, err := attemptRange(ctx, conn, a)
	if err != nil {
		return p, err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id=? AND sequence>? AND sequence<=? AND event_type='tool_intent'`, a.Child.ID, after, until).Scan(&p.Actions); err != nil {
		return p, fmt.Errorf("count child actions: %w", err)
	}
	if p.Actions == 0 {
		return p, nil
	}
	var name, args string
	var done bool
	err = conn.QueryRowContext(ctx, `SELECT json_extract(i.payload_json,'$.call.name'), json_extract(i.payload_json,'$.call.arguments'),
 EXISTS(SELECT 1 FROM events o WHERE o.session_id=i.session_id AND o.execution_id=i.execution_id AND o.event_type IN ('tool_succeeded','tool_failed','tool_cancelled'))
 FROM events i WHERE i.session_id=? AND i.sequence>? AND i.sequence<=? AND i.event_type='tool_intent' ORDER BY i.sequence DESC LIMIT 1`, a.Child.ID, after, until).Scan(&name, &args, &done)
	if err != nil {
		return p, fmt.Errorf("read latest child action: %w", err)
	}
	p.Latest = &SubagentAction{Name: name, Args: BoundToolArguments(args, subagentActionArgsBytes), Done: done}
	return p, nil
}

// BoundToolArguments shortens a tool call's JSON arguments for display while
// keeping them JSON, so a row still finds its subject (a path, query or
// command): each long string value is cut first. Arguments that are not a
// JSON object, or still too long, are cut as text.
func BoundToolArguments(args string, n int) string {
	if len(args) <= n {
		return args
	}
	var fields map[string]any
	if json.Unmarshal([]byte(args), &fields) == nil {
		for limit := n / 2; limit >= 32; limit /= 2 {
			for key, value := range fields {
				if text, ok := value.(string); ok && len(text) > limit {
					fields[key] = utf8Prefix(text, limit) + "…"
				}
			}
			if b, err := json.Marshal(fields); err == nil && len(b) <= n {
				return string(b)
			}
		}
	}
	return utf8Prefix(args, n) + "…"
}
