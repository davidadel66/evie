package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// ReconcileCompilerHistory does one ancestry discovery and one sealed root
// decision in separate transactions. Neither can exceed 128 source inspections;
// at most one job is materialized or resumed in a step. Range cursors only aid
// discovery: unresolved roots keep their separate, restartable obligations.
func (s *Store) ReconcileCompilerHistory(ctx context.Context, config CompilerSupervisorConfig) (result memory.CompilerReconciliation, err error) {
	started := time.Now()
	defer func() { result.DurationNanos = time.Since(started).Nanoseconds() }()
	if len(config.Extractors) == 0 || len(config.Extractors) > 32 {
		return result, ErrCompilerNotConfigured
	}
	if err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error { return discoverCompilerHistory(ctx, conn, &result) }); err != nil {
		return
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var request, session, root, generationID, destination string
		var ordinal, first, last, order int64
		var manifest []byte
		err := conn.QueryRowContext(ctx, historyNextRootRequest).Scan(&request)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		check, err := nextHistoryCheck(ctx, conn)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_requests SET root_checked=? WHERE request_id=?`, check, request); err != nil {
			return err
		}
		err = conn.QueryRowContext(ctx, historyNextRootRange, request).Scan(&ordinal)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_ranges SET checked_order=? WHERE request_id=? AND ordinal=?`, check, request, ordinal); err != nil {
			return err
		}
		err = conn.QueryRowContext(ctx, `SELECT x.session_id,r.root_id,q.generation_id,x.destination,r.first_sequence,r.last_sequence,q.selection_order,g.manifest FROM memory_compiler_history_roots r JOIN memory_compiler_history_ranges x ON x.request_id=r.request_id AND x.ordinal=r.range_ordinal JOIN memory_compiler_history_requests q ON q.request_id=r.request_id JOIN memory_compiler_generations g USING(generation_id) WHERE r.request_id=? AND r.range_ordinal=? AND r.state IN ('selected_unmaterialized','deferred_live') ORDER BY r.checked_order,r.first_sequence,r.root_id LIMIT 1`, request, ordinal).Scan(&session, &root, &generationID, &destination, &first, &last, &order, &manifest)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_roots SET checked_order=? WHERE request_id=? AND range_ordinal=? AND root_id=?`, check, request, ordinal, root); err != nil {
			return err
		}
		extractor := config.Extractors[generationID]
		if extractor == nil || extractor.ServerIdentity() == "" {
			result.State = "configuration_paused"
			_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_roots SET reason='configuration_unavailable' WHERE request_id=? AND range_ordinal=? AND root_id=?`, request, ordinal, root)
			return err
		}
		var generation memory.CompilerGeneration
		if err := json.Unmarshal(manifest, &generation); err != nil {
			return err
		}
		owner, err := reviewSourceContext(ctx, conn, memory.SessionID(session))
		available := false
		if err == nil {
			available, err = compilerHistorySourceAvailable(ctx, conn, owner, destination)
		} else if errors.Is(err, sql.ErrNoRows) {
			// Missing retained registry identity is unavailable, not authority
			// to replace it or evidence that this interval completed.
			err = nil
		}
		if err != nil {
			return err
		}
		if !available {
			// The source is still rejected. Commit only safe scheduling metadata
			// so its retained obligation cannot monopolize later requests.
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_roots SET reason='source_scope_unavailable' WHERE request_id=? AND range_ordinal=? AND root_id=?`, request, ordinal, root); err != nil {
				return err
			}
			result.State = "authorization_paused"
			return nil
		}
		cutoff := last
		// A different overlapping request may own a queue-full earlier prefix.
		// Revisit that owner before considering the final already-owned suffix.
		var pendingLast int64
		err = conn.QueryRowContext(ctx, historyPendingOwnership, generationID, destination, session, root, last, first).Scan(&pendingLast)
		if err == nil && pendingLast < cutoff {
			cutoff = pendingLast
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		selectionID, _, err := selectCompilerUnitInTransaction(ctx, conn, owner, memory.CompilationSelection{SessionID: owner.SessionID, RootID: memory.EventID(root), Cutoff: cutoff, Destination: destination}, generationID, manifest, generation, compilerScheduling{Lane: "historical", HistoricalOrder: order, FirstSequence: first, AwaitClosure: true, HistorySelected: true})
		if err != nil {
			return err
		}
		state, reason := "deferred_live", "unfinished_live_turn"
		var sealedLast int64
		if selectionID != "" {
			if err := conn.QueryRowContext(ctx, `SELECT state,reason,last_sequence FROM memory_compiler_selections WHERE selection_id=?`, selectionID).Scan(&state, &reason, &sealedLast); err != nil {
				return err
			}
			pendingErr := conn.QueryRowContext(ctx, historyPendingOwnership, generationID, destination, session, root, last, first).Scan(&pendingLast)
			if pendingErr != nil && !errors.Is(pendingErr, sql.ErrNoRows) {
				return pendingErr
			}
			if (sealedLast < last || pendingErr == nil) && state != "selected_unmaterialized" && state != "deferred_live" {
				state = "selected_unmaterialized"
				reason = ""
			}
		}
		result.SelectionID = selectionID
		result.State = state
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_history_roots SET state=?,reason=?,selection_id=? WHERE request_id=? AND range_ordinal=? AND root_id=?`, state, reason, selectionID, request, ordinal, root)
		return err
	})
	if err != nil {
		return
	}
	err = s.resumeCompilerHistoryStep(ctx, config)
	return
}

func discoverCompilerHistory(ctx context.Context, conn compilerActivationTransaction, result *memory.CompilerReconciliation) error {
	var request, session string
	var ordinal, scanned, last int64
	err := conn.QueryRowContext(ctx, historyNextDiscoveryRequest).Scan(&request)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	check, err := nextHistoryCheck(ctx, conn)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_requests SET range_checked=? WHERE request_id=?`, check, request); err != nil {
		return err
	}
	err = conn.QueryRowContext(ctx, historyNextDiscoveryRange, request).Scan(&ordinal, &session, &scanned, &last)
	if err != nil {
		return err
	}
	var event, kind, role, payload string
	var parent sql.NullString
	var seq int64
	err = conn.QueryRowContext(ctx, `SELECT id,sequence,event_type,parent_id,COALESCE(role,''),CASE WHEN length(CAST(payload_json AS BLOB))<=8192 THEN COALESCE(payload_json,'') ELSE '' END FROM events WHERE session_id=? AND sequence>? AND sequence<=? ORDER BY sequence LIMIT 1`, session, scanned, last).Scan(&event, &seq, &kind, &parent, &role, &payload)
	if err != nil {
		return err
	}
	ancestry, err := resolveCompilerAncestry(ctx, conn, session, compilerAncestryEvent{ID: event, Parent: parent, Sequence: seq, Kind: kind, Role: role, Payload: payload})
	if err != nil {
		return err
	}
	root := ancestry.ID
	state, reason := "selected_unmaterialized", ""
	if ancestry.Kind != "user_message" || ancestry.Role != "user" || ancestry.Parent.Valid {
		root = event
		state = "failed"
		reason = "invalid_source_ancestry"
		if ancestry.Depth == 128 {
			reason = "source_inspection_limit"
		}
		if kind == string(memory.EventContextCompacted) || kind == string(memory.EventContextSnapshot) {
			state = "excluded"
			reason = "prohibited_source"
		}
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_roots(request_id,range_ordinal,root_id,first_sequence,last_sequence,state,reason) VALUES(?,?,?,?,?,?,?) ON CONFLICT(request_id,range_ordinal,root_id) DO UPDATE SET last_sequence=MAX(last_sequence,excluded.last_sequence)`, request, ordinal, root, seq, seq, state, reason); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_events VALUES(?,?,?,?,?)`, request, ordinal, event, seq, root); err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_history_ranges SET scanned_sequence=?,checked_order=? WHERE request_id=? AND ordinal=?`, seq, check, request, ordinal)
	result.Discovered = true
	return err
}

// Pending-request indexes exclude all retained completed and cancelled history.
// Child range choices visit at most the receipt's100 ranges; root choices use
// the exact request/range pending index, never all historical roots.
const historyNextDiscoveryRequest = `SELECT request_id FROM memory_compiler_history_requests WHERE cancelled=0 AND pending_ranges>0 ORDER BY range_checked,selection_order,request_id LIMIT 1`
const historyNextDiscoveryRange = `SELECT ordinal,session_id,scanned_sequence,last_sequence FROM memory_compiler_history_ranges WHERE request_id=? AND scanned_sequence<last_sequence ORDER BY checked_order,ordinal LIMIT 1`
const historyNextRootRequest = `SELECT request_id FROM memory_compiler_history_requests WHERE cancelled=0 AND pending_roots>0 ORDER BY root_checked,selection_order,request_id LIMIT 1`
const historyNextRootRange = `SELECT ordinal FROM memory_compiler_history_ranges x WHERE request_id=? AND scanned_sequence=last_sequence AND EXISTS(SELECT 1 FROM memory_compiler_history_roots r WHERE r.request_id=x.request_id AND r.range_ordinal=x.ordinal AND r.state IN ('selected_unmaterialized','deferred_live')) ORDER BY checked_order,ordinal LIMIT 1`
const historyPendingOwnership = `SELECT last_sequence FROM memory_compiler_selections WHERE generation_id=? AND destination=? AND session_id=? AND root_id=? AND first_sequence<=? AND last_sequence>=? AND job_id IS NULL AND state IN ('selected_unmaterialized','deferred_live') ORDER BY first_sequence LIMIT 1`

func nextHistoryCheck(ctx context.Context, conn compilerActivationTransaction) (int64, error) {
	var check int64
	err := conn.QueryRowContext(ctx, `UPDATE memory_compiler_history_schedule_counter SET value=value+1 WHERE singleton=1 RETURNING value`).Scan(&check)
	return check, err
}

// This preflight recognizes only proven unavailable registry states and the
// typed quarantine outcome. It does not grant authorization: available work
// still passes the shared compilerAuthorize inside the selector. Unexpected
// query/cancellation/transaction failures are never converted to a pause.
func compilerHistorySourceAvailable(ctx context.Context, conn *sql.Conn, owner memory.ScopeContext, destination string) (bool, error) {
	if owner.WorkspaceID != "" {
		var state string
		err := conn.QueryRowContext(ctx, `SELECT lifecycle_state FROM workspaces WHERE id=?`, owner.WorkspaceID).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if state != "active" {
			return false, nil
		}
	}
	if owner.ProjectID != "" {
		var archived int
		err := conn.QueryRowContext(ctx, `SELECT archived FROM projects WHERE id=?`, owner.ProjectID).Scan(&archived)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if archived != 0 {
			return false, nil
		}
	}
	err := requireSemanticScopeKeysAvailable(ctx, conn, []string{"global", scopeKeyForContext(owner), destination})
	if errors.Is(err, ErrSemanticScopeQuarantined) {
		return false, nil
	}
	return err == nil, err
}
