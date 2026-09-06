package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrCompilerHistoryConflict = errors.New("compiler history request or revision conflict")

func authorizeCompilerHistory(ctx context.Context, conn *sql.Conn, owners []memory.ScopeContext, ranges []memory.CompilerHistoryRange) error {
	if len(owners) < 1 || len(owners) > 100 {
		return errors.New("history requires 1 through 100 exact source authorizations")
	}
	for _, r := range ranges {
		var owner *memory.ScopeContext
		for i := range owners {
			if owners[i].SessionID == r.SessionID {
				if owner != nil {
					return errors.New("duplicate source authorization")
				}
				owner = &owners[i]
			}
		}
		if owner == nil || scopeKeyForContext(*owner) != r.SourceScope {
			return errors.New("history source scope not authorized")
		}
		if err := compilerAuthorize(ctx, conn, *owner, memory.CompilationSelection{SessionID: r.SessionID, Destination: r.Destination}); err != nil {
			return err
		}
	}
	return nil
}

// SelectCompilerHistory seals finite source coordinates, never source contents.
// No inference runs here and no live activation is created or changed.
func (s *Store) SelectCompilerHistory(ctx context.Context, owners []memory.ScopeContext, req memory.CompilerHistoryRequest, generation memory.CompilerGeneration, extractor CompilerExtractor) (result memory.CompilerHistoryReceipt, err error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return result, ErrCompilerNotConfigured
	}
	if req.RequestID == "" || len(req.RequestID) > 256 || len(req.Ranges) < 1 || len(req.Ranges) > 100 {
		return result, errors.New("history requires a bounded request ID and 1 through 100 ranges")
	}
	generationID, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return result, err
	}
	generation = memory.CompilerGeneration{}
	if err = json.Unmarshal(manifest, &generation); err != nil {
		return result, err
	}
	// Freeze caller storage before preflight or transaction use.
	var frozen memory.CompilerHistoryRequest
	if err = json.Unmarshal(compilerJSON(req), &frozen); err != nil {
		return result, err
	}
	req = frozen
	requestHash := memory.CompilerHash(compilerJSON(struct {
		Request    memory.CompilerHistoryRequest
		Generation string
	}{req, generationID}))
	prior := false
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := authorizeCompilerHistory(ctx, conn, owners, req.Ranges); err != nil {
			return err
		}
		var hash string
		var raw []byte
		err := conn.QueryRowContext(ctx, `SELECT request_hash,receipt FROM memory_compiler_history_requests WHERE request_id=?`, req.RequestID).Scan(&hash, &raw)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		prior = true
		if hash != requestHash {
			return ErrCompilerHistoryConflict
		}
		return json.Unmarshal(raw, &result)
	})
	if err != nil || prior {
		return result, err
	}
	if err = verifyCompilerActivation(ctx, generation, extractor); err != nil {
		return result, err
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := authorizeCompilerHistory(ctx, conn, owners, req.Ranges); err != nil {
			return err
		}
		var hash string
		var raw []byte
		err := conn.QueryRowContext(ctx, `SELECT request_hash,receipt FROM memory_compiler_history_requests WHERE request_id=?`, req.RequestID).Scan(&hash, &raw)
		if err == nil {
			if hash != requestHash {
				return ErrCompilerHistoryConflict
			}
			return json.Unmarshal(raw, &result)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var total int64
		// Count each distinct event only once even if this receipt overlaps itself.
		// These bounded index-coordinate queries do not inspect evidence fields.
		for i, r := range req.Ranges {
			if r.FirstSequence <= 0 || r.LastSequence < r.FirstSequence || r.FirstEventID == "" || r.LastEventID == "" {
				return errors.New("history requires exact inclusive boundaries")
			}
			var first, last string
			if err := conn.QueryRowContext(ctx, `SELECT id FROM events WHERE session_id=? AND sequence=?`, r.SessionID, r.FirstSequence).Scan(&first); err != nil {
				return err
			}
			if err := conn.QueryRowContext(ctx, `SELECT id FROM events WHERE session_id=? AND sequence=?`, r.SessionID, r.LastSequence).Scan(&last); err != nil {
				return err
			}
			if first != string(r.FirstEventID) || last != string(r.LastEventID) {
				return errors.New("history boundary event mismatch")
			}
			// Sum unique coordinate intervals after subtracting previous same-session
			// ranges using JSON parameters; the original order/receipt is retained.
			var n int64
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT 1 FROM events e WHERE e.session_id=? AND e.sequence BETWEEN ? AND ? AND NOT EXISTS(SELECT 1 FROM json_each(?) p WHERE json_extract(p.value,'$.session_id')=e.session_id AND e.sequence BETWEEN json_extract(p.value,'$.first_sequence') AND json_extract(p.value,'$.last_sequence')) LIMIT 10001)`, r.SessionID, r.FirstSequence, r.LastSequence, compilerJSON(req.Ranges[:i])).Scan(&n); err != nil {
				return err
			}
			total += n
			if total > 10000 {
				return errors.New("history exceeds 10000 selected events")
			}
		}
		var order int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(selection_order),0)+1 FROM memory_compiler_history_requests`).Scan(&order); err != nil {
			return err
		}
		result = memory.CompilerHistoryReceipt{Request: req, GenerationID: generationID, SelectedEvents: total, SelectionOrder: order}
		if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_generations VALUES(?,?)`, generationID, manifest); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_requests(request_id,request_hash,owner_id,generation_id,selection_order,receipt,pending_ranges) VALUES(?,?,'local',?,?,?,?)`, req.RequestID, requestHash, generationID, order, compilerJSON(result), len(req.Ranges)); err != nil {
			return err
		}
		if err := updateHistorySelectionRefs(ctx, conn, result, 1); err != nil {
			return err
		}
		for i, r := range req.Ranges {
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_ranges(request_id,ordinal,source_scope,destination,session_id,first_sequence,last_sequence,first_event_id,last_event_id,scanned_sequence) VALUES(?,?,?,?,?,?,?,?,?,?)`, req.RequestID, i, r.SourceScope, r.Destination, r.SessionID, r.FirstSequence, r.LastSequence, r.FirstEventID, r.LastEventID, r.FirstSequence-1); err != nil {
				return err
			}
		}
		return nil
	})
	return
}

func loadCompilerHistory(ctx context.Context, conn *sql.Conn, owners []memory.ScopeContext, id string) (receipt memory.CompilerHistoryReceipt, state memory.CompilerHistoryState, err error) {
	var raw []byte
	state.RequestID = id
	err = conn.QueryRowContext(ctx, `SELECT receipt,revision,cancelled FROM memory_compiler_history_requests WHERE request_id=? AND owner_id='local'`, id).Scan(&raw, &state.Revision, &state.Cancelled)
	if err != nil {
		return
	}
	if err = json.Unmarshal(raw, &receipt); err != nil {
		return
	}
	err = authorizeCompilerHistory(ctx, conn, owners, receipt.Request.Ranges)
	return
}

// Cancellation is an explicit owner transaction, bounded by the global 1024
// unfinished-job cap. Dynamic union membership is checked after the request
// changes state; an overlapping active request keeps its existing job alive.
func (s *Store) CancelCompilerHistory(ctx context.Context, owners []memory.ScopeContext, change memory.CompilerHistoryChange) (memory.CompilerHistoryState, error) {
	return s.changeCompilerHistory(ctx, owners, change, true, "", nil)
}
func (s *Store) ResumeCompilerHistory(ctx context.Context, owners []memory.ScopeContext, change memory.CompilerHistoryChange, generation memory.CompilerGeneration, extractor CompilerExtractor) (memory.CompilerHistoryState, error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return memory.CompilerHistoryState{}, ErrCompilerNotConfigured
	}
	id, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return memory.CompilerHistoryState{}, err
	}
	generation = memory.CompilerGeneration{}
	if err = json.Unmarshal(manifest, &generation); err != nil {
		return memory.CompilerHistoryState{}, err
	}
	return s.changeCompilerHistory(ctx, owners, change, false, id, func() error { return verifyCompilerActivation(ctx, generation, extractor) })
}
func (s *Store) changeCompilerHistory(ctx context.Context, owners []memory.ScopeContext, change memory.CompilerHistoryChange, cancelled bool, generationID string, verify func() error) (result memory.CompilerHistoryState, err error) {
	if change.OperationID == "" || len(change.OperationID) > 256 || change.ExpectedRevision < 1 {
		return result, errors.New("history change requires bounded operation ID and exact revision")
	}
	hash := memory.CompilerHash(compilerJSON(struct {
		Change     memory.CompilerHistoryChange
		Cancel     bool
		Generation string
	}{change, cancelled, generationID}))
	// Preflight stays outside SQLite. An exact prior delivery needs no network.
	prior := false
	check := func(conn *sql.Conn) error {
		receipt, state, err := loadCompilerHistory(ctx, conn, owners, change.RequestID)
		if err != nil {
			return err
		}
		var oldHash string
		var raw []byte
		err = conn.QueryRowContext(ctx, `SELECT request_hash,response FROM memory_compiler_history_changes WHERE operation_id=?`, change.OperationID).Scan(&oldHash, &raw)
		if err == nil {
			prior = true
			if oldHash != hash {
				return ErrCompilerHistoryConflict
			}
			return json.Unmarshal(raw, &result)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if state.Revision != change.ExpectedRevision || state.Cancelled == cancelled {
			return ErrCompilerHistoryConflict
		}
		if !cancelled && receipt.GenerationID != generationID {
			return ErrCompilerConfiguration
		}
		result = state
		result.Revision++
		result.Cancelled = cancelled
		return nil
	}
	if err = s.withImmediateTransaction(ctx, check); err != nil || prior {
		return result, err
	}
	if verify != nil {
		if err = verify(); err != nil {
			return result, err
		}
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := check(conn); err != nil || prior {
			return err
		}
		var changeOrder int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(change_order),0)+1 FROM memory_compiler_history_changes`).Scan(&changeOrder); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_requests SET revision=?,cancelled=?,resume_order=CASE WHEN ?=0 THEN ? ELSE resume_order END WHERE request_id=?`, result.Revision, result.Cancelled, result.Cancelled, changeOrder, change.RequestID); err != nil {
			return err
		}
		receipt, _, err := loadCompilerHistory(ctx, conn, owners, change.RequestID)
		if err != nil {
			return err
		}
		delta := 1
		if cancelled {
			delta = -1
		}
		if err := updateHistorySelectionRefs(ctx, conn, receipt, delta); err != nil {
			return err
		}
		if cancelled {
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_jobs SET cancel_order=? WHERE job_id IN (SELECT job_id FROM memory_compiler_history_paused_jobs) AND job_id IN (SELECT job_id FROM memory_compiler_jobs WHERE state IN ('queued','running','retry_wait','staged'))`, changeOrder); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_capacity SET state='release_pending' WHERE job_id IN (SELECT job_id FROM memory_compiler_history_paused_jobs)`); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id IN (SELECT job_id FROM memory_compiler_history_paused_jobs)`); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state='cancelled',reason='history_cancelled',fence=fence+1,holder=NULL,lease_until=NULL WHERE state IN ('queued','running','retry_wait','staged') AND job_id IN (SELECT job_id FROM memory_compiler_history_paused_jobs)`); err != nil {
				return err
			}
		}
		if cancelled {
			if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_history_resume_queue WHERE request_id=?`, change.RequestID); err != nil {
				return err
			}
		} else {
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_resume_queue(request_id,change_order) VALUES(?,?) ON CONFLICT(request_id) DO UPDATE SET change_order=excluded.change_order`, change.RequestID, changeOrder); err != nil {
				return err
			}
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_changes VALUES(?,?,?,?)`, change.OperationID, hash, compilerJSON(result), changeOrder)
		return err
	})
	return
}

// Explicit owner actions maintain union reference counts for only their bounded
// coordinate selection. Background worker gates probe at most the sealed job's
// 128 event IDs and never enumerate retained overlapping request receipts.
func updateHistorySelectionRefs(ctx context.Context, conn *sql.Conn, receipt memory.CompilerHistoryReceipt, delta int) error {
	ranges := compilerJSON(receipt.Request.Ranges)
	if delta == 1 {
		_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_selection_refs(generation_id,destination,session_id,event_id,active_requests)
 SELECT DISTINCT ?,json_extract(r.value,'$.destination'),e.session_id,e.id,1 FROM json_each(?) r JOIN events e ON e.session_id=json_extract(r.value,'$.session_id') AND e.sequence BETWEEN json_extract(r.value,'$.first_sequence') AND json_extract(r.value,'$.last_sequence') WHERE 1
 ON CONFLICT(generation_id,destination,session_id,event_id) DO UPDATE SET active_requests=active_requests+1`, receipt.GenerationID, ranges)
		return err
	}
	_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_history_selection_refs SET active_requests=active_requests-1 WHERE generation_id=? AND (destination,session_id,event_id) IN (
 SELECT DISTINCT json_extract(r.value,'$.destination'),e.session_id,e.id FROM json_each(?) r JOIN events e ON e.session_id=json_extract(r.value,'$.session_id') AND e.sequence BETWEEN json_extract(r.value,'$.first_sequence') AND json_extract(r.value,'$.last_sequence'))`, receipt.GenerationID, ranges)
	return err
}
