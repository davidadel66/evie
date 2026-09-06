package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// ReconcileCompilerEvidence performs two bounded transactions: discover one
// selected event's ancestry, then reconsider one root. Discovery cursors are
// independent of root obligations, so deferred roots cannot hide later work.
// Configuration controls materialization, never event commitment or selection.
func (s *Store) ReconcileCompilerEvidence(ctx context.Context, config CompilerSupervisorConfig) (result memory.CompilerReconciliation, err error) {
	started := time.Now()
	defer func() { result.DurationNanos = time.Since(started).Nanoseconds() }()
	if len(config.Extractors) == 0 || len(config.Extractors) > 32 {
		return result, ErrCompilerNotConfigured
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		return discoverCompilerEvidenceInTransaction(ctx, conn, &result)
	})
	if err != nil {
		return result, err
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var activation, session, root, generationID, destination, pendingSelection string
		var first, last, position, after, high int64
		var manifest []byte
		// Check one revisit candidate, even when another root remains live.
		err := conn.QueryRowContext(ctx, `SELECT r.activation_id,r.session_id,r.root_id,a.generation_id,a.destination,r.first_sequence,r.last_sequence,r.position,a.after_position,d.high_position,g.manifest,r.selection_id FROM memory_compiler_activation_roots r JOIN memory_compiler_activations a USING(activation_id) JOIN memory_compiler_activation_dirty d ON d.activation_id=r.activation_id AND d.session_id=r.session_id JOIN memory_compiler_generations g USING(generation_id) WHERE a.work_paused=0 AND r.state IN ('selected_unmaterialized','deferred_live') ORDER BY r.checked_order,r.position,r.activation_id LIMIT 1`).Scan(&activation, &session, &root, &generationID, &destination, &first, &last, &position, &after, &high, &manifest, &pendingSelection)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activation_roots SET checked_order=(SELECT COALESCE(MAX(checked_order),0)+1 FROM memory_compiler_activation_roots) WHERE activation_id=? AND root_id=?`, activation, root); err != nil {
			return err
		}
		extractor := config.Extractors[generationID]
		if extractor == nil || extractor.ServerIdentity() == "" {
			result.State = "configuration_paused"
			_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activation_roots SET reason='configuration_unavailable' WHERE activation_id=? AND root_id=?`, activation, root)
			return err
		}
		var generation memory.CompilerGeneration
		if err := json.Unmarshal(manifest, &generation); err != nil {
			return err
		}
		var workspace, project sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT workspace_id,project_id FROM sessions WHERE id=?`, session).Scan(&workspace, &project); err != nil {
			return err
		}
		owner := memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: memory.SessionID(session), WorkspaceID: memory.WorkspaceID(workspace.String), ProjectID: memory.ProjectID(project.String)}
		// Include a selected later root solely as the closure boundary. Earlier
		// roots own only their actual members; its content cannot become support.
		var cutoff int64
		err = conn.QueryRowContext(ctx, `SELECT e.sequence FROM events e JOIN memory_compiler_event_positions p ON p.event_id=e.id WHERE e.session_id=? AND p.commit_position>? AND p.commit_position<=? AND e.event_type='user_message' AND e.role='user' AND e.parent_id IS NULL AND e.sequence>(SELECT sequence FROM events WHERE id=?) ORDER BY e.sequence LIMIT 1`, session, after, high, root).Scan(&cutoff)
		if errors.Is(err, sql.ErrNoRows) {
			err = conn.QueryRowContext(ctx, `SELECT e.sequence FROM events e JOIN memory_compiler_event_positions p ON p.event_id=e.id WHERE e.session_id=? AND p.commit_position>? AND p.commit_position<=? ORDER BY e.sequence DESC LIMIT 1`, session, after, high).Scan(&cutoff)
		}
		if err != nil {
			return err
		}
		if cutoff < last {
			cutoff = last
		}
		desiredCutoff := cutoff
		if pendingSelection != "" {
			var pendingCutoff int64
			err := conn.QueryRowContext(ctx, `SELECT last_sequence FROM memory_compiler_selections WHERE selection_id=? AND state='selected_unmaterialized' AND job_id IS NULL`, pendingSelection).Scan(&pendingCutoff)
			if err == nil {
				cutoff = pendingCutoff
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		selectionID, jobID, err := selectCompilerUnitInTransaction(ctx, conn, owner, memory.CompilationSelection{SessionID: owner.SessionID, RootID: memory.EventID(root), Cutoff: cutoff, Destination: destination}, generationID, manifest, generation, compilerScheduling{Lane: "new", Position: position, FirstSequence: first, AwaitClosure: true})
		if err != nil {
			return err
		}
		if selectionID == "" {
			result.State = "deferred_live"
			_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activation_roots SET state='deferred_live',reason='unfinished_live_turn' WHERE activation_id=? AND root_id=?`, activation, root)
			return err
		}

		result.SelectionID = selectionID
		var state, reason string
		var sealedLast int64
		if err := conn.QueryRowContext(ctx, `SELECT state,reason,last_sequence FROM memory_compiler_selections WHERE selection_id=?`, selectionID).Scan(&state, &reason, &sealedLast); err != nil {
			return err
		}
		result.State = state
		if jobID != "" {
			// Explicit historical work keeps its original scheduling/authority.
			// Reuse cannot attach another activation to a different owned window.
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_activation_jobs SELECT ?,? WHERE EXISTS(SELECT 1 FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q USING(job_id) WHERE j.job_id=? AND q.lane='new' AND j.first_sequence>=? AND j.last_sequence<=?)`, jobID, activation, jobID, first, desiredCutoff); err != nil {
				return err
			}
		}
		rootState, rootReason := state, reason
		if desiredCutoff > sealedLast && state != "selected_unmaterialized" && state != "deferred_live" {
			rootState = "selected_unmaterialized"
			rootReason = ""
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_activation_roots SET state=?,reason=?,selection_id=? WHERE activation_id=? AND root_id=?`, rootState, rootReason, selectionID, activation, root)
		return err
	})
	return
}

type compilerActivationTransaction interface {
	compilerQueryer
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Discovery reads the selected event once, then seeds the bounded ancestry walk
// with that cached row. At depth128 it has inspected1+127 events, not129.
func discoverCompilerEvidenceInTransaction(ctx context.Context, conn compilerActivationTransaction, result *memory.CompilerReconciliation) error {
	var activation, session string
	var scanned, high int64
	err := conn.QueryRowContext(ctx, `SELECT d.activation_id,d.session_id,d.scanned_position,d.high_position FROM memory_compiler_activation_dirty d JOIN memory_compiler_activations a USING(activation_id) WHERE d.scanned_position<d.high_position AND a.work_paused=0 ORDER BY d.scan_order,d.activation_id,d.session_id LIMIT 1`).Scan(&activation, &session, &scanned, &high)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var event, kind, role, payload string
	var parent sql.NullString
	var seq, position int64
	err = conn.QueryRowContext(ctx, `SELECT e.id,e.sequence,e.event_type,p.commit_position,e.parent_id,COALESCE(e.role,''),CASE WHEN length(CAST(e.payload_json AS BLOB))<=8192 THEN COALESCE(e.payload_json,'') ELSE '' END FROM events e JOIN memory_compiler_event_positions p ON p.event_id=e.id JOIN memory_compiler_activations a ON a.activation_id=? WHERE e.session_id=? AND p.commit_position>MAX(?,a.after_position) AND p.commit_position<=? ORDER BY p.commit_position LIMIT 1`, activation, session, scanned, high).Scan(&event, &seq, &kind, &position, &parent, &role, &payload)
	if err != nil {
		return err
	}
	// Follow parent/terminal turn identity, never "most recent root". Only
	// metadata is loaded; the separate sealing transaction bounds raw input.
	var root, rootKind, rootRole string
	var rootseq, depth int64
	var rootParent sql.NullString
	err = conn.QueryRowContext(ctx, `WITH RECURSIVE ancestry(id,parent_id,sequence,event_type,role,payload_json,depth) AS (
 VALUES(?,?,?,?,?,?,1)
 UNION ALL SELECT e.id,e.parent_id,e.sequence,e.event_type,COALESCE(e.role,''),CASE WHEN length(CAST(e.payload_json AS BLOB))<=8192 THEN COALESCE(e.payload_json,'') ELSE '' END,a.depth+1 FROM ancestry a JOIN events e ON e.id=CASE WHEN a.event_type IN ('turn_failed','turn_interrupted') AND json_valid(a.payload_json) THEN json_extract(a.payload_json,'$.turn_id') ELSE a.parent_id END
 WHERE e.session_id=? AND e.sequence<a.sequence AND a.depth<128
) SELECT id,sequence,event_type,role,parent_id,depth FROM ancestry ORDER BY depth DESC LIMIT 1`, event, parent, seq, kind, role, payload, session).Scan(&root, &rootseq, &rootKind, &rootRole, &rootParent, &depth)
	if err != nil {
		return err
	}
	state, reason := "selected_unmaterialized", ""
	if rootKind != "user_message" || rootRole != "user" || rootParent.Valid {
		root = event
		rootseq = seq
		state = "failed"
		reason = "invalid_source_ancestry"
		if depth == 128 {
			reason = "source_inspection_limit"
		}
		if kind == string(memory.EventContextCompacted) || kind == string(memory.EventContextSnapshot) {
			state = "excluded"
			reason = "prohibited_source"
		}
	}

	_, err = conn.ExecContext(ctx, `INSERT INTO memory_compiler_activation_roots(activation_id,session_id,root_id,first_sequence,last_sequence,position,state,reason) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(activation_id,root_id) DO UPDATE SET last_sequence=MAX(last_sequence,excluded.last_sequence),state=CASE WHEN excluded.last_sequence>last_sequence THEN excluded.state ELSE state END,reason=CASE WHEN excluded.last_sequence>last_sequence THEN excluded.reason ELSE reason END`, activation, session, root, seq, seq, position, state, reason)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_activation_dirty SET scanned_position=?,scan_order=(SELECT COALESCE(MAX(scan_order),0)+1 FROM memory_compiler_activation_dirty) WHERE activation_id=? AND session_id=?`, position, activation, session)
	result.Discovered = true
	return err
}

// RunCompilerHost owns independent selection and extraction loops. A blocked
// local request cannot prevent new obligations from being materialized.
func (s *Store) RunCompilerHost(ctx context.Context, config CompilerSupervisorConfig) error {
	if len(config.Extractors) == 0 || len(config.Extractors) > 32 {
		return ErrCompilerNotConfigured
	}
	pinned := make(map[string]CompilerExtractor, len(config.Extractors))
	for id, extractor := range config.Extractors {
		if extractor == nil || extractor.ServerIdentity() == "" {
			return ErrCompilerNotConfigured
		}
		pinned[id] = extractor
	}
	config.Extractors = pinned
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.RunCompilerSupervisor(ctx, config) }()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			cancel()
			timer := time.NewTimer(5 * time.Second)
			defer timer.Stop()
			select {
			case <-done:
				return ctx.Err()
			case <-timer.C:
				return errors.Join(ctx.Err(), errors.New("compiler host cleanup deadline"))
			}
		case <-ticker.C:
			_, err := s.ReconcileCompilerEvidence(ctx, config)
			if err != nil && ctx.Err() == nil {
				continue
			} // obligations remain durable
		}
	}
}
