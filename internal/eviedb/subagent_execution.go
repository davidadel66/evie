package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// StartSubagent reserves execution capacity durably. Competing callers can
// observe or join the attempt, but only one wins its admitted -> running edge.
func (s *Store) StartSubagent(ctx context.Context, id string) (delegation.Attempt, bool, error) {
	var a delegation.Attempt
	started := false
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.State != "admitted" {
			return nil
		}
		if err = s.authorizeSubagentParent(ctx, conn, a.Parent, a.Receipt); err != nil {
			return err
		}
		var runtime, parent int
		if err = conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(parent_session_id=?),0) FROM subagent_executions WHERE state='running'`, a.Parent.Scope.SessionID).Scan(&runtime, &parent); err != nil {
			return err
		}
		if runtime >= a.Policy.Runtime || parent >= a.Policy.PerParent {
			return delegation.ErrCapacity
		}
		now := s.now().UTC()
		a.StartedAt = &now
		a.State = "running"
		if err = writeSubagent(ctx, conn, a); err != nil {
			return err
		}
		if a.Continues != nil {
			if err = reopenSubagentChild(ctx, conn, a.Child.ID, now); err != nil {
				return err
			}
		}
		started = true
		return nil
	})
	return a, started, err
}

// reopenSubagentChild makes a continued child's closed session runnable for
// its new attempt. Any child lease still recorded belongs to an earlier,
// finished attempt (for example one that crashed before releasing it); it is
// released so the continuation acquires the session with a new fencing token
// and the earlier holder can never write again.
func reopenSubagentChild(ctx context.Context, conn *sql.Conn, child memory.SessionID, now time.Time) error {
	if _, err := conn.ExecContext(ctx, `UPDATE sessions SET status=?,updated_at=? WHERE id=?`, memory.SessionActive, now.Format(time.RFC3339Nano), child); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, `UPDATE session_turn_leases SET holder_id=NULL,expires_at=NULL WHERE session_id=?`, child)
	return err
}

// authorizeSubagentChild is part of every fenced child mutation, including
// final assistant acceptance. Parent and child fences share one transaction.
func (s *Store) authorizeSubagentChild(ctx context.Context, conn *sql.Conn, child memory.SessionID) error {
	return s.authorizeSubagentChildWith(ctx, conn, child, fenceTurnLeaseWrite)
}

func (s *Store) authorizeSubagentChildWith(ctx context.Context, conn *sql.Conn, child memory.SessionID, fence subagentLeaseFence) error {
	var parent sql.NullString
	if err := conn.QueryRowContext(ctx, `SELECT parent_session_id FROM sessions WHERE id=?`, child).Scan(&parent); err != nil {
		return err
	}
	if !parent.Valid {
		return nil
	}
	// The child's latest attempt is the only one that can be running: a
	// continuation is admitted only after every earlier attempt finished.
	a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE child_session_id=? ORDER BY rowid DESC LIMIT 1`, child))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if a.State != "running" || a.StartedAt == nil || !s.now().Before(a.StartedAt.Add(a.Policy.Deadline)) {
		return delegation.ErrAuthority
	}
	return s.authorizeSubagentParentWith(ctx, conn, a.Parent, a.Receipt, fence)
}

// AuthorizeSubagent reports whether a running child still holds authority.
// It gates external work and the supervisor's watchdog but writes nothing, so
// it evaluates the same checks on one read snapshot without the write lock.
// Every child mutation still re-authorizes under BEGIN IMMEDIATE.
func (s *Store) AuthorizeSubagent(ctx context.Context, id string) error {
	return s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.State != "running" {
			return delegation.ErrAuthority
		}
		return s.authorizeSubagentChildWith(ctx, conn, a.Child.ID, checkTurnLeaseLive)
	})
}

// checkTurnLeaseLive is the read-only form of fenceTurnLeaseWrite, with the
// same predicate. It proves ownership at the snapshot without fencing writes.
func checkTurnLeaseLive(ctx context.Context, conn *sql.Conn, sessionID memory.SessionID, holderID memory.LeaseHolderID, token memory.FencingToken, nowText string) error {
	var live bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM session_turn_leases l JOIN sessions s ON s.id=l.session_id
 WHERE l.session_id=? AND l.holder_id=? AND l.fencing_token=? AND l.expires_at>? AND s.status=?)`, sessionID, holderID, token, nowText, memory.SessionActive).Scan(&live); err != nil {
		return fmt.Errorf("check turn lease: %w", err)
	}
	if !live {
		return fmt.Errorf("%w: session %q", ErrTurnLeaseLost, sessionID)
	}
	return nil
}

// withSubagentReadTransaction runs read-only subagent checks on one WAL
// snapshot without taking SQLite's write lock. It always rolls back, so a
// check can never persist a change.
func (s *Store) withSubagentReadTransaction(ctx context.Context, operation func(*sql.Conn) error) (err error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("open subagent read connection: %w", err)
	}
	defer func() {
		if closeErr := conn.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close subagent read connection: %w", closeErr)
		}
	}()
	if _, err := conn.ExecContext(ctx, `BEGIN`); err != nil {
		return fmt.Errorf("begin subagent read transaction: %w", err)
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if _, rollbackErr := conn.ExecContext(rollbackCtx, `ROLLBACK`); rollbackErr != nil {
			discardImmediateTransactionConnection(conn)
			if err == nil {
				err = fmt.Errorf("end subagent read transaction: %w", rollbackErr)
			}
		}
	}()
	return operation(conn)
}

// FinishSubagent arbitrates against accepted child evidence under the write
// lock. A later cancellation can never replace an already accepted final answer.
func (s *Store) FinishSubagent(ctx context.Context, id, state, reason string) (delegation.Attempt, error) {
	if state != "failed" && state != "cancelled" && state != "interrupted" && state != "succeeded" && state != delegation.StatePartial {
		return delegation.Attempt{}, errors.New("invalid terminal subagent state")
	}
	var a delegation.Attempt
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		a, err = readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if err != nil {
			return err
		}
		if a.Terminal() {
			return nil
		}
		return s.finishSubagent(ctx, conn, &a, state, reason)
	})
	return a, err
}

func (s *Store) finishSubagent(ctx context.Context, conn *sql.Conn, a *delegation.Attempt, state, reason string) error {
	// An attempt owns the child events after the earlier attempts' turns:
	// all of them for an original assignment, its own turn for a
	// continuation. An earlier report never settles a continuation.
	var after int64
	next := subagentNextSteps{continuable: subagentParentCanContinue(ctx, conn, a.Parent.Scope.SessionID)}
	result := delegation.Result{ExecutionID: a.ID, ChildSessionID: a.Child.ID, Status: state, Reason: reason}
	if a.Continues != nil {
		after = a.Continues.AfterSequence
		next.continues = a.Continues.ExecutionID
		result.ContinuesExecutionID = a.Continues.ExecutionID
	}
	var finalID, report string
	err := conn.QueryRowContext(ctx, `SELECT id,content FROM events WHERE session_id=? AND sequence>? AND event_type='assistant_message'
 AND COALESCE(json_array_length(payload_json,'$.tool_calls'),0)=0 ORDER BY sequence DESC LIMIT 1`, a.Child.ID, after).Scan(&finalID, &report)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	hasReport := err == nil
	switch {
	case hasReport:
		// An accepted final answer is the authoritative completion. It is a
		// partial report when the child was wrapping up at a budget.
		a.FinalEventID = memory.EventID(finalID)
		result.Status, result.Reason = "succeeded", ""
		if a.WrapUp != nil {
			result.Status, result.Reason = delegation.StatePartial, a.WrapUp.Reason
		} else if state == delegation.StatePartial {
			result.Status, result.Reason = delegation.StatePartial, reason
		}
	case state == "succeeded" || state == delegation.StatePartial:
		result.Status = "interrupted"
		result.Reason = "no_accepted_final_answer"
	}
	if result.Status == "failed" && result.Reason == "infrastructure_failure" {
		// The normal agent loop records transport and response-validation failures.
		// Reuse that safe classification, including invalid nil-error responses,
		// without copying raw provider errors into the delegation result.
		var terminalPayload []byte
		err = conn.QueryRowContext(ctx, `SELECT payload_json FROM events WHERE session_id=? AND sequence>? AND event_type='turn_failed' ORDER BY sequence DESC LIMIT 1`, a.Child.ID, after).Scan(&terminalPayload)
		if err == nil {
			var terminal memory.TurnTerminalPayload
			if err = json.Unmarshal(terminalPayload, &terminal); err != nil {
				return err
			}
			switch terminal.Classification {
			case memory.ClassificationProviderError:
				result.Reason = "provider_failure"
			case memory.ClassificationProviderResponseInvalid:
				result.Reason = "provider_response_invalid"
			case memory.ClassificationContextOverflow:
				result.Reason = delegation.ReasonContextLimit
			case memory.ClassificationStepLimitExceeded:
				result.Reason = delegation.ReasonWrapUpFailed
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	// Usage and the Web evidence are reported for every outcome.
	if result.Usage, err = subagentUsage(ctx, conn, a.Child.ID, after); err != nil {
		return err
	}
	evidence, err := subagentWebEvidence(ctx, conn, a.Child.ID, after)
	if err != nil {
		return err
	}
	if !hasReport {
		report = ""
	}
	buildSubagentResult(&result, hasReport, report, evidence, a.Policy, next)
	now := s.now().UTC()
	a.EndedAt = &now
	a.State = result.Status
	a.Result = &result
	if err = writeSubagent(ctx, conn, *a); err != nil {
		return err
	}
	// Terminal execution never leaves a runnable worker or an outstanding
	// claim; only a started continuation reopens the session (StartSubagent).
	_, err = conn.ExecContext(ctx, `UPDATE sessions SET status='closed',updated_at=? WHERE id=?`, now.Format(time.RFC3339Nano), a.Child.ID)
	return err
}

// subagentParentCanContinue reports whether the parent's pinned composition
// includes continue_research, so results only suggest a tool the parent has.
func subagentParentCanContinue(ctx context.Context, conn *sql.Conn, parent memory.SessionID) bool {
	receipt, err := getCompositionReceipt(ctx, conn, parent)
	if err != nil {
		return false
	}
	for _, c := range receipt.Capabilities {
		if c.ID == delegation.ContinueCapabilityID {
			return true
		}
	}
	return false
}

// subagentUsage sums the usage of the child's committed responses after the
// sequence where the attempt began, including compactions. A response without
// usage, or a turn that ended inside a provider or compaction call, makes the
// total a lower bound (Incomplete). With no reported usage at all it stays
// unknown (nil).
func subagentUsage(ctx context.Context, conn *sql.Conn, child memory.SessionID, after int64) (*delegation.Usage, error) {
	rows, err := conn.QueryContext(ctx, `SELECT event_type,payload_json FROM events WHERE session_id=? AND sequence>?
 AND event_type IN ('assistant_message','context_compacted','turn_failed','turn_interrupted') ORDER BY sequence`, child, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var total *delegation.Usage
	incomplete := false
	for rows.Next() {
		var kind string
		var b []byte
		if err = rows.Scan(&kind, &b); err != nil {
			return nil, err
		}
		if kind == string(memory.EventTurnFailed) || kind == string(memory.EventTurnInterrupted) {
			var terminal memory.TurnTerminalPayload
			if err = json.Unmarshal(b, &terminal); err != nil {
				return nil, err
			}
			if terminal.Stage == memory.StageProvider || terminal.Stage == memory.StageContextCompaction {
				incomplete = true
			}
			continue
		}
		var p struct {
			Usage *memory.TokenUsage `json:"usage"`
		}
		if err = json.Unmarshal(b, &p); err != nil {
			return nil, err
		}
		if p.Usage == nil {
			incomplete = true
			continue
		}
		if total == nil {
			total = &delegation.Usage{TokenUsage: *p.Usage}
			continue
		}
		// A field is reported only when every measured response reported it.
		addUsage := func(dst **int64, src *int64) {
			if src == nil || *dst == nil {
				*dst = nil
				return
			}
			sum := **dst + *src
			*dst = &sum
		}
		addUsage(&total.InputTokens, p.Usage.InputTokens)
		addUsage(&total.OutputTokens, p.Usage.OutputTokens)
		addUsage(&total.TotalTokens, p.Usage.TotalTokens)
		addUsage(&total.ReasoningOutputTokens, p.Usage.ReasoningOutputTokens)
		addUsage(&total.CachedInputTokens, p.Usage.CachedInputTokens)
		addUsage(&total.CacheWriteInputTokens, p.Usage.CacheWriteInputTokens)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if total != nil {
		total.Incomplete = incomplete
	}
	return total, nil
}

// RecoverSubagents never starts execution or fabricates conversational events.
// Only attempts whose original parent lease is no longer live are reconciled.
// Each attempt is reconciled in its own transaction: a record that cannot be
// reconciled is reported in the returned error and does not block the rest.
func (s *Store) RecoverSubagents(ctx context.Context) (int, error) {
	candidates, err := s.subagentRecoveryCandidates(ctx)
	if err != nil {
		return 0, fmt.Errorf("list unfinished subagent executions: %w", err)
	}
	count := 0
	var failures []error
	for _, id := range candidates {
		if err := ctx.Err(); err != nil {
			return count, errors.Join(append(failures, err)...)
		}
		recovered, err := s.recoverSubagent(ctx, id)
		if err != nil {
			failures = append(failures, fmt.Errorf("recover subagent execution %q: %w", id, err))
			continue
		}
		if recovered {
			count++
		}
	}
	return count, errors.Join(failures...)
}

// subagentRecoveryCandidates selects unfinished attempts whose parent lease is
// not live on one read snapshot, so live work costs no write transaction. An
// unreadable record is still selected so its own transaction reports it.
func (s *Store) subagentRecoveryCandidates(ctx context.Context) ([]string, error) {
	var candidates []string
	err := s.withSubagentReadTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `SELECT id,record_json FROM subagent_executions WHERE state IN ('admitted','running') ORDER BY id`)
		if err != nil {
			return err
		}
		type unfinished struct{ id, record string }
		var found []unfinished
		for rows.Next() {
			var u unfinished
			if err = rows.Scan(&u.id, &u.record); err != nil {
				rows.Close()
				return err
			}
			found = append(found, u)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		now := s.now().UTC().Format(turnLeaseTimeFormat)
		for _, u := range found {
			var a delegation.Attempt
			if json.Unmarshal([]byte(u.record), &a) != nil {
				candidates = append(candidates, u.id)
				continue
			}
			err = checkTurnLeaseLive(ctx, conn, a.Parent.Scope.SessionID, a.Parent.Lease.HolderID, a.Parent.Lease.FencingToken, now)
			if errors.Is(err, ErrTurnLeaseLost) {
				candidates = append(candidates, u.id)
			} else if err != nil {
				return err
			}
		}
		return nil
	})
	return candidates, err
}

// recoverSubagent re-proves abandonment under the write lock before finishing.
func (s *Store) recoverSubagent(ctx context.Context, id string) (bool, error) {
	recovered := false
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		a, err := readSubagent(conn.QueryRowContext(ctx, `SELECT record_json FROM subagent_executions WHERE id=?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if a.Terminal() {
			return nil
		}
		err = fenceTurnLeaseWrite(ctx, conn, a.Parent.Scope.SessionID, a.Parent.Lease.HolderID, a.Parent.Lease.FencingToken, s.now().UTC().Format(turnLeaseTimeFormat))
		if err == nil {
			return nil
		}
		if !errors.Is(err, ErrTurnLeaseLost) {
			return err
		}
		if err = s.finishSubagent(ctx, conn, &a, "interrupted", "original_parent_ownership_ended"); err != nil {
			return err
		}
		recovered = true
		return nil
	})
	return recovered, err
}
