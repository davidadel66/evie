package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrCompilerActivationConflict = errors.New("compiler activation revision or selector conflict")

// Activation verifies operational metadata before capturing its frontier, never
// by dispatching inference. Scripted adapters explicitly implement this seam.
type CompilerConfigurationVerifier interface {
	VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error
}

func verifyCompilerActivation(ctx context.Context, g memory.CompilerGeneration, e CompilerExtractor) error {
	v, ok := e.(CompilerConfigurationVerifier)
	if !ok {
		return ErrCompilerConfiguration
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return v.VerifyCompilerConfiguration(ctx, g)
}

// ActivateCompiler captures the append frontier together with owner authority,
// the exact selector and pinned manifest. Extract is never called here.
func (s *Store) ActivateCompiler(ctx context.Context, owner memory.ScopeContext, req memory.CompilerActivationRequest, generation memory.CompilerGeneration, extractor CompilerExtractor) (memory.CompilerActivation, error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return memory.CompilerActivation{}, ErrCompilerNotConfigured
	}
	id, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return memory.CompilerActivation{}, err
	}
	generation = memory.CompilerGeneration{}
	if err := json.Unmarshal(manifest, &generation); err != nil {
		return memory.CompilerActivation{}, err
	}
	if prior, found, err := s.priorCompilerActivation(ctx, owner, "activate", req, id); found || err != nil {
		return prior, err
	}
	if err := verifyCompilerActivation(ctx, generation, extractor); err != nil {
		return memory.CompilerActivation{}, err
	}
	return s.changeCompilerActivation(ctx, owner, "activate", req, id, manifest)
}

func (s *Store) DisableCompilerActivation(ctx context.Context, owner memory.ScopeContext, req memory.CompilerActivationRequest) (memory.CompilerActivation, error) {
	return s.changeCompilerActivation(ctx, owner, "disable", req, "", nil)
}

// ResumeCompilerActivation resumes an exact prior segment's selected work. It
// does not reopen live selection or include events from the disabled interval.
func (s *Store) ResumeCompilerActivation(ctx context.Context, owner memory.ScopeContext, req memory.CompilerActivationRequest, generation memory.CompilerGeneration, extractor CompilerExtractor) (memory.CompilerActivation, error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return memory.CompilerActivation{}, ErrCompilerNotConfigured
	}
	id, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return memory.CompilerActivation{}, err
	}
	generation = memory.CompilerGeneration{}
	if err := json.Unmarshal(manifest, &generation); err != nil {
		return memory.CompilerActivation{}, err
	}
	if prior, found, err := s.priorCompilerActivation(ctx, owner, "resume", req, id); found || err != nil {
		return prior, err
	}
	if err := verifyCompilerActivation(ctx, generation, extractor); err != nil {
		return memory.CompilerActivation{}, err
	}
	return s.changeCompilerActivation(ctx, owner, "resume", req, id, manifest)
}

func authorizeCompilerActivation(ctx context.Context, conn *sql.Conn, owner memory.ScopeContext, selector memory.CompilerLiveSelector) error {
	if selector.SourceScope != scopeKeyForContext(owner) || (selector.SessionID != "" && selector.SessionID != owner.SessionID) {
		return errors.New("compiler activation source lineage mismatch")
	}
	if selector.Destination == "session:"+string(owner.SessionID) && selector.SessionID != owner.SessionID {
		return errors.New("session destination requires exact source session")
	}
	return compilerAuthorize(ctx, conn, owner, memory.CompilationSelection{SessionID: owner.SessionID, Destination: selector.Destination})
}

func compilerActivationRequestHash(action string, req memory.CompilerActivationRequest, generationID string) string {
	return memory.CompilerHash(compilerJSON(struct {
		Action     string
		Request    memory.CompilerActivationRequest
		Generation string
	}{action, req, generationID}))
}

func (s *Store) priorCompilerActivation(ctx context.Context, owner memory.ScopeContext, action string, req memory.CompilerActivationRequest, generationID string) (result memory.CompilerActivation, found bool, err error) {
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := authorizeCompilerActivation(ctx, conn, owner, req.Selector); err != nil {
			return err
		}
		var hash string
		var data []byte
		err := conn.QueryRowContext(ctx, `SELECT request_hash,response FROM memory_compiler_activation_requests WHERE request_id=?`, req.RequestID).Scan(&hash, &data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		if hash != compilerActivationRequestHash(action, req, generationID) {
			return ErrCompilerActivationConflict
		}
		return json.Unmarshal(data, &result)
	})
	return
}

func (s *Store) changeCompilerActivation(ctx context.Context, owner memory.ScopeContext, action string, req memory.CompilerActivationRequest, generationID string, manifest []byte) (result memory.CompilerActivation, err error) {
	if req.RequestID == "" || len(req.RequestID) > 256 || req.ExpectedRevision < 0 {
		return result, errors.New("activation requires bounded request ID and nonnegative revision")
	}
	requestHash := compilerActivationRequestHash(action, req, generationID)
	selectorKey := memory.CompilerHash(compilerJSON(req.Selector))
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := authorizeCompilerActivation(ctx, conn, owner, req.Selector); err != nil {
			return err
		}
		var priorHash string
		var response []byte
		err := conn.QueryRowContext(ctx, `SELECT request_hash,response FROM memory_compiler_activation_requests WHERE request_id=?`, req.RequestID).Scan(&priorHash, &response)
		if err == nil {
			if priorHash != requestHash {
				return ErrCompilerActivationConflict
			}
			return json.Unmarshal(response, &result)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var revision int64
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM memory_compiler_activations WHERE selector_key=?`, selectorKey).Scan(&revision); err != nil {
			return err
		}
		if revision != req.ExpectedRevision {
			return ErrCompilerActivationConflict
		}
		var frontier int64
		if err := conn.QueryRowContext(ctx, `SELECT value FROM memory_compiler_position_counter WHERE singleton=1`).Scan(&frontier); err != nil {
			return err
		}
		if action == "activate" {
			if req.ActivationID != "" {
				return errors.New("new activation does not accept a prior activation ID")
			}
			var conflict int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_activations WHERE through_position IS NULL AND destination=? AND source_scope=? AND selector_key<>? AND (source_session='' OR ?='' OR source_session=?)`, req.Selector.Destination, req.Selector.SourceScope, selectorKey, req.Selector.SessionID, req.Selector.SessionID).Scan(&conflict); err != nil {
				return err
			}
			if conflict != 0 {
				return ErrCompilerActivationConflict
			}
			if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_generations VALUES(?,?)`, generationID, manifest); err != nil {
				return err
			}
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activations SET through_position=? WHERE selector_key=? AND through_position IS NULL`, frontier, selectorKey); err != nil {
				return err
			}
			result = memory.CompilerActivation{ID: memory.CompilerHash([]byte(req.RequestID)), Selector: req.Selector, GenerationID: generationID, Revision: revision + 1, AfterPosition: frontier}
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_activations(activation_id,selector_key,source_scope,source_session,destination,generation_id,revision,after_position) VALUES(?,?,?,?,?,?,?,?)`, result.ID, selectorKey, req.Selector.SourceScope, req.Selector.SessionID, req.Selector.Destination, generationID, result.Revision, frontier); err != nil {
				return err
			}
		} else {
			if req.ActivationID == "" {
				return errors.New("disable/resume requires the exact activation ID")
			}
			row := conn.QueryRowContext(ctx, `SELECT activation_id,source_scope,source_session,destination,generation_id,revision,after_position,through_position,work_paused FROM memory_compiler_activations WHERE selector_key=? AND activation_id=?`, selectorKey, req.ActivationID)
			if err := scanCompilerActivation(row, &result); err != nil {
				return err
			}
			if action == "disable" {
				if result.WorkPaused {
					return ErrCompilerActivationConflict
				}
				if result.ThroughPosition == nil {
					result.ThroughPosition = &frontier
				}
				result.WorkPaused = true
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activations SET through_position=COALESCE(through_position,?),work_paused=1,work_epoch=work_epoch+1,revision=? WHERE activation_id=?`, frontier, revision+1, result.ID); err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_capacity SET state='release_pending' WHERE job_id IN (SELECT job_id FROM memory_compiler_activation_jobs WHERE activation_id=?)`, result.ID); err != nil {
					return err
				}
			} else {
				if !result.WorkPaused || result.GenerationID != generationID {
					return fmt.Errorf("%w: resume requires the paused generation", ErrCompilerActivationConflict)
				}
				result.WorkPaused = false
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_activations SET work_paused=0,revision=? WHERE activation_id=?`, revision+1, result.ID); err != nil {
					return err
				}
			}
			result.Revision = revision + 1
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO memory_compiler_activation_requests VALUES(?,?,?)`, req.RequestID, requestHash, compilerJSON(result))
		return err
	})
	return
}

func scanCompilerActivation(row interface{ Scan(...any) error }, result *memory.CompilerActivation) error {
	return row.Scan(&result.ID, &result.Selector.SourceScope, &result.Selector.SessionID, &result.Selector.Destination, &result.GenerationID, &result.Revision, &result.AfterPosition, &result.ThroughPosition, &result.WorkPaused)
}

// Status returns content-free selection counts for this exact owner lineage.
func (s *Store) InspectCompilerActivations(ctx context.Context, owner memory.ScopeContext) (result memory.CompilerActivationStatus, err error) {
	result.Activations = []memory.CompilerActivation{}
	result.Roots = []memory.CompilerActivationRootStatus{}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := compilerAuthorize(ctx, conn, owner, memory.CompilationSelection{SessionID: owner.SessionID, Destination: scopeKeyForContext(owner)}); err != nil {
			return err
		}
		rows, err := conn.QueryContext(ctx, `SELECT activation_id,source_scope,source_session,destination,generation_id,revision,after_position,through_position,work_paused FROM memory_compiler_activations WHERE source_scope=? AND (source_session='' OR source_session=?) ORDER BY revision DESC,activation_id LIMIT 129`, scopeKeyForContext(owner), owner.SessionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a memory.CompilerActivation
			if err := scanCompilerActivation(rows, &a); err != nil {
				return err
			}
			result.Activations = append(result.Activations, a)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		rows.Close()
		if len(result.Activations) > 128 {
			result.ActivationsTruncated = true
			result.Activations = result.Activations[:128]
		}
		roots, err := conn.QueryContext(ctx, `SELECT r.activation_id,r.root_id,r.first_sequence,r.last_sequence,r.state,r.reason,r.selection_id,COALESCE(j.state,''),COALESCE(j.reason,'') FROM memory_compiler_activation_roots r LEFT JOIN memory_compiler_selections s ON s.selection_id=r.selection_id LEFT JOIN memory_compiler_jobs j ON j.job_id=s.job_id WHERE r.session_id=? ORDER BY r.position DESC,r.activation_id LIMIT 129`, owner.SessionID)
		if err != nil {
			return err
		}
		defer roots.Close()
		for roots.Next() {
			var r memory.CompilerActivationRootStatus
			var jobState, jobReason string
			if err := roots.Scan(&r.ActivationID, &r.RootID, &r.FirstSequence, &r.LastSequence, &r.State, &r.Reason, &r.SelectionID, &jobState, &jobReason); err != nil {
				return err
			}
			// A pending extension owns a fresh discovery obligation even when its
			// preceding selected prefix already finished. Otherwise show the live
			// worker state, not the queue state captured at materialization time.
			if r.State != "selected_unmaterialized" && r.State != "deferred_live" && jobState != "" {
				r.State = jobState
				r.Reason = safeCompilerReason(jobReason)
			}
			result.Roots = append(result.Roots, r)
		}
		if err := roots.Err(); err != nil {
			return err
		}
		roots.Close()
		if len(result.Roots) > 128 {
			result.RootsTruncated = true
			result.Roots = result.Roots[:128]
		}
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events e JOIN memory_compiler_event_positions p ON p.event_id=e.id WHERE e.session_id=? AND EXISTS(SELECT 1 FROM memory_compiler_activations a WHERE a.source_scope=? AND (a.source_session='' OR a.source_session=e.session_id) AND p.commit_position>a.after_position AND (a.through_position IS NULL OR p.commit_position<=a.through_position))`, owner.SessionID, scopeKeyForContext(owner)).Scan(&result.SelectedEvents); err != nil {
			return err
		}
		var total int64
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE session_id=?`, owner.SessionID).Scan(&total); err != nil {
			return err
		}
		result.OutsideSelectionEvents = total - result.SelectedEvents
		return conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(state IN ('selected_unmaterialized','deferred_live')),0),COALESCE(SUM(state='failed'),0) FROM memory_compiler_activation_roots WHERE session_id=?`, owner.SessionID).Scan(&result.PendingRoots, &result.SourceErrors)
	})
	return
}
