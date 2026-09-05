package eviedb

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// Scheduling metadata is installed in the same transaction that creates the
// sealed job. Activation and backfill supply it through the internal selector.
// Neither retries nor a different supervisor can rewrite its original lane.
type compilerScheduling struct {
	FirstSequence   int64
	AwaitClosure    bool
	HistorySelected bool
	Lane            string
	Position        int64
	HistoricalOrder int64
}

const compilerWorkerSchema = `
-- Queueing seals the evidence; attempt one seals the accepted-state snapshot.
DROP TRIGGER IF EXISTS memory_compiler_request_immutable;
CREATE TRIGGER memory_compiler_request_immutable BEFORE UPDATE OF generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash ON memory_compiler_jobs BEGIN SELECT RAISE(ABORT,'immutable compiler request'); END;
DROP TRIGGER IF EXISTS memory_compiler_request_snapshot_immutable;
CREATE TRIGGER memory_compiler_request_snapshot_immutable BEFORE UPDATE OF request ON memory_compiler_jobs
 WHEN NEW.request!=OLD.request AND NOT COALESCE((OLD.attempts=0 AND NEW.attempts=1 AND OLD.state='queued' AND NEW.state='running' AND json_extract(NEW.request,'$.window')=json_extract(OLD.request,'$.window') AND json_extract(NEW.request,'$.window_sha256')=json_extract(OLD.request,'$.window_sha256') AND json_extract(NEW.request,'$.generation_id')=json_extract(OLD.request,'$.generation_id')),0)
 BEGIN SELECT RAISE(ABORT,'immutable compiler accepted snapshot'); END;
CREATE TABLE IF NOT EXISTS memory_compiler_job_schedule (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id),
 lane TEXT NOT NULL DEFAULT 'historical' CHECK(lane IN ('new','historical')),
 position INTEGER NOT NULL DEFAULT 0, historical_order INTEGER NOT NULL DEFAULT 0,
 pause_reason TEXT NOT NULL DEFAULT ''
);
CREATE TRIGGER IF NOT EXISTS memory_compiler_job_scheduling AFTER INSERT ON memory_compiler_jobs BEGIN
 INSERT INTO memory_compiler_job_schedule(job_id) VALUES(NEW.job_id);
END;
INSERT OR IGNORE INTO memory_compiler_job_schedule(job_id) SELECT job_id FROM memory_compiler_jobs;
CREATE TABLE IF NOT EXISTS memory_compiler_scheduler (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), new_attempts INTEGER NOT NULL DEFAULT 0 CHECK(new_attempts BETWEEN 0 AND 8)
);
INSERT OR IGNORE INTO memory_compiler_scheduler VALUES(1,0);
CREATE TABLE IF NOT EXISTS memory_compiler_resources (
 job_id TEXT PRIMARY KEY REFERENCES memory_compiler_jobs(job_id), fence INTEGER NOT NULL,
 stage_bytes INTEGER NOT NULL CHECK(stage_bytes=131072), candidate_positions INTEGER NOT NULL CHECK(candidate_positions=16)
);
CREATE TABLE IF NOT EXISTS memory_compiler_worker_installation (singleton INTEGER PRIMARY KEY CHECK(singleton=1));
INSERT OR IGNORE INTO memory_compiler_resources
 SELECT job_id,fence,131072,16 FROM memory_compiler_jobs WHERE state IN ('running','staged') AND NOT EXISTS (SELECT 1 FROM memory_compiler_worker_installation);
INSERT OR IGNORE INTO memory_compiler_worker_installation VALUES(1);
CREATE TABLE IF NOT EXISTS memory_compiler_release_receipts (
 request_id TEXT PRIMARY KEY, job_id TEXT NOT NULL, fence INTEGER NOT NULL, holder TEXT NOT NULL,
 server_identity TEXT NOT NULL, kind TEXT NOT NULL, recorded_at INTEGER NOT NULL
);
`

// QueueCandidateUnit records selection without waiting for extraction. The
// supervisor only dispatches generations supplied in its explicit configuration.
func (s *Store) QueueCandidateUnit(ctx context.Context, owner memory.ScopeContext, selection memory.CompilationSelection, generation memory.CompilerGeneration, extractor CompilerExtractor) (memory.Compilation, error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return memory.Compilation{}, ErrCompilerNotConfigured
	}
	id, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return memory.Compilation{}, err
	}
	generation = memory.CompilerGeneration{}
	if err := json.Unmarshal(manifest, &generation); err != nil {
		return memory.Compilation{}, err
	}
	selectionID, _, err := s.selectCompilerUnit(ctx, owner, selection, id, manifest, generation)
	if err != nil {
		return memory.Compilation{}, err
	}
	return s.InspectCompilation(ctx, owner, selectionID)
}

var errCompilerInvalidSource = errors.New("invalid sealed compiler source")

type compilerClaim struct {
	Supervised               bool
	JobID, Holder, AttemptID string
	Fence                    int64
	Request                  memory.CompilerRequest
	Generation               memory.CompilerGeneration
}

func (s *Store) claimCompilerJob(ctx context.Context, owner memory.ScopeContext, jobID string, extractor CompilerExtractor) (compilerClaim, error) {
	var claim compilerClaim
	var rejected error
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		claim, err = claimCompilerJob(ctx, conn, owner, jobID, extractor)
		if errors.Is(err, errCompilerInvalidSource) || errors.Is(err, ErrCompilerConfiguration) {
			rejected = err
			return rejectCompilerClaim(ctx, conn, jobID, err)
		}
		return err
	})
	return claim, errors.Join(err, rejected)
}

func claimCompilerJob(ctx context.Context, conn *sql.Conn, owner memory.ScopeContext, jobID string, extractor CompilerExtractor) (compilerClaim, error) {
	var c compilerClaim
	if extractor == nil || extractor.ServerIdentity() == "" {
		return c, ErrCompilerNotConfigured
	}
	inspected, err := loadCompilerInspection(ctx, conn, owner, jobID)
	if err != nil {
		if compilerDataFailure(err) {
			return c, errors.Join(errCompilerInvalidSource, err)
		}
		return c, err
	}
	var ready int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q ON q.job_id=j.job_id WHERE j.job_id=? AND q.pause_reason='' AND NOT EXISTS (SELECT 1 FROM memory_compiler_paused_jobs p WHERE p.job_id=j.job_id) AND j.attempts<5 AND (j.state='queued' OR (j.state='retry_wait' AND j.retry_at<=unixepoch('now')))`, jobID).Scan(&ready); err != nil {
		return c, err
	}
	if ready != 1 {
		return c, ErrCompilerFence
	}
	var capacity int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_capacity`).Scan(&capacity); err != nil {
		return c, err
	}
	if capacity != 0 {
		return c, ErrCompilerCapacityBlocked
	}
	holder, err := newSemanticID()
	if err != nil {
		return c, err
	}
	attemptID, err := newSemanticID()
	if err != nil {
		return c, err
	}
	c.JobID, c.Holder, c.AttemptID, c.Generation = jobID, string(holder), string(attemptID), inspected.Generation
	var request []byte
	if err := conn.QueryRowContext(ctx, `SELECT request FROM memory_compiler_jobs WHERE job_id=?`, jobID).Scan(&request); err != nil {
		return c, err
	}
	if err := json.Unmarshal(request, &c.Request); err != nil {
		return c, err
	}
	if inspected.Attempts == 0 {
		c.Request.AcceptedContextOmitted = false
		if err := compilerAcceptedContext(ctx, conn, owner, &c.Request); err != nil {
			if compilerDataFailure(err) {
				return c, errors.Join(errCompilerInvalidSource, err)
			}
			return c, err
		}
		c.Request.ID = ""
		c.Request.ID = memory.CompilerHash(compilerJSON(c.Request))
	}
	if err := memory.CompilerInputBudget(c.Generation, c.Request); err != nil {
		return c, fmt.Errorf("%w: input budget", ErrCompilerConfiguration)
	}
	if err := conn.QueryRowContext(ctx, `UPDATE memory_compiler_jobs SET state='running',attempts=attempts+1,fence=fence+1,holder=?,lease_until=unixepoch('now')+30,retry_at=NULL,reason='',request=? WHERE job_id=? RETURNING fence`, c.Holder, compilerJSON(c.Request), jobID).Scan(&c.Fence); err != nil {
		return c, err
	}
	if err := reserveCompilerResources(ctx, conn, jobID, c.Fence); err != nil {
		return c, err
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_capacity VALUES(1,?,?,?,?,?,'reserved')`, c.AttemptID, jobID, c.Fence, c.Holder, extractor.ServerIdentity()); err != nil {
		return c, err
	}
	// This counter changes with the reservation, never with a process-local poll.
	_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_scheduler SET new_attempts=CASE WHEN (SELECT lane FROM memory_compiler_job_schedule WHERE job_id=?)='new' THEN MIN(8,new_attempts+1) ELSE 0 END WHERE singleton=1`, jobID)
	return c, err
}

func reserveCompilerResources(ctx context.Context, conn compilerStageTransaction, jobID string, fence int64) error {
	var groups, bytes, presentation int64
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(stage_bytes),0) FROM memory_compiler_resources WHERE job_id<>?`, jobID).Scan(&groups, &bytes); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_candidates WHERE review_state='unresolved' AND equivalent_to IS NULL`).Scan(&presentation); err != nil {
		return err
	}
	if groups+1 > 128 || bytes+memory.CompilerMaxBytes > 16*1024*1024 || presentation+16*(groups+1) > 2048 {
		return ErrCompilerCapacityBlocked
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_resources VALUES(?,?,131072,16) ON CONFLICT(job_id) DO UPDATE SET fence=excluded.fence`, jobID, fence)
	return err
}

func (s *Store) renewCompilerClaim(ctx context.Context, claim compilerClaim) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET lease_until=unixepoch('now')+30 WHERE job_id=? AND holder=? AND fence=? AND lease_until>unixepoch('now') AND state IN ('running','staged') AND NOT EXISTS (SELECT 1 FROM memory_compiler_invalid_claims p WHERE p.job_id=memory_compiler_jobs.job_id)`, claim.JobID, claim.Holder, claim.Fence)
		return compilerChanged(result, err)
	})
}

// RecoverCompilerWork fences a bounded batch of expired attempts. It never
// guesses whether the server stopped. Valid stages remain adoptable; a missing
// stage is a consumed failed attempt with its original retry budget.
func (s *Store) RecoverCompilerWork(ctx context.Context) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `SELECT job_id,fence,holder,state FROM memory_compiler_jobs WHERE state IN ('running','staged') AND lease_until<=unixepoch('now') ORDER BY job_id LIMIT 16`)
		if err != nil {
			return err
		}
		type expired struct {
			job, holder, state string
			fence              int64
		}
		var jobs []expired
		for rows.Next() {
			var j expired
			if err := rows.Scan(&j.job, &j.fence, &j.holder, &j.state); err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, j := range jobs {
			if j.state == "running" {
				// One request plus its reservations and job: <=64 mutations per batch.
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_capacity SET state='release_pending' WHERE job_id=? AND fence=? AND holder=?`, j.job, j.fence, j.holder); err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=? AND fence=?`, j.job, j.fence); err != nil {
					return err
				}
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state=CASE WHEN attempts=5 THEN 'failed' ELSE 'retry_wait' END,reason=CASE WHEN attempts=5 THEN 'attempts_exhausted' ELSE 'worker_interrupted' END,retry_at=CASE WHEN attempts<5 THEN unixepoch('now')+(5 << (attempts-1)) END,fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, j.job); err != nil {
					return err
				}
			} else {
				if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, j.job); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// CompilerCapacityReservation identifies one dispatch, not a mutable model
// alias or an assumed server boot. It contains no source or candidate content.
type CompilerCapacityReservation struct {
	RequestID      string `json:"request_id"`
	JobID          string `json:"job_id"`
	Fence          int64  `json:"fence"`
	Holder         string `json:"holder"`
	ServerIdentity string `json:"server_identity"`
}

type CompilerReleaseAcknowledgement struct {
	Reservation CompilerCapacityReservation
	Kind        string // request_completed, request_cancelled, or verified_server_restart
}

// CompilerReleaseVerifier is a trusted runtime adapter. Implementations must
// authenticate a request-specific completion/idle acknowledgement or controlled
// restart that covers this exact reservation. Ollama deliberately does not
// implement this interface: endpoint/version/model do not prove a server boot.
type CompilerReleaseVerifier interface {
	VerifyCompilerRelease(context.Context, CompilerCapacityReservation) (CompilerReleaseAcknowledgement, error)
}

// ReconcileCompilerCapacity only contacts a contracted verifier and bounds the
// contact to five seconds. No verifier, timeout, or stale acknowledgement can
// free a slot. There is intentionally no owner-supplied force-release flag.
func (s *Store) ReconcileCompilerCapacity(ctx context.Context, verifier CompilerReleaseVerifier) error {
	var r CompilerCapacityReservation
	err := s.db.QueryRowContext(ctx, `SELECT request_id,job_id,fence,holder,server_identity FROM memory_compiler_capacity WHERE state='release_pending'`).Scan(&r.RequestID, &r.JobID, &r.Fence, &r.Holder, &r.ServerIdentity)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if verifier == nil {
		return ErrCompilerCapacityBlocked
	}
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ack, err := verifier.VerifyCompilerRelease(verifyCtx, r)
	if err != nil {
		return errors.Join(ErrCompilerCapacityBlocked, err)
	}
	if verifyCtx.Err() != nil {
		return errors.Join(ErrCompilerCapacityBlocked, verifyCtx.Err())
	}
	if ack.Reservation != r || (ack.Kind != "request_completed" && ack.Kind != "request_cancelled" && ack.Kind != "verified_server_restart") {
		return ErrCompilerCapacityBlocked
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_capacity WHERE request_id=? AND job_id=? AND fence=? AND holder=? AND server_identity=? AND state='release_pending'`, r.RequestID, r.JobID, r.Fence, r.Holder, r.ServerIdentity)
		if err := compilerChanged(result, err); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO memory_compiler_release_receipts VALUES(?,?,?,?,?,?,unixepoch('now'))`, r.RequestID, r.JobID, r.Fence, r.Holder, r.ServerIdentity, ack.Kind)
		return err
	})
}

func validateCompilerStage(request memory.CompilerRequest, envelope []byte, hash string) ([]memory.MemoryCandidate, error) {
	if len(envelope) > memory.CompilerMaxBytes || memory.CompilerHash(envelope) != hash {
		return nil, errors.New("stage seal mismatch")
	}
	var candidates []memory.MemoryCandidate
	if err := memory.DecodeCompilerJSON(envelope, &candidates); err != nil {
		return nil, err
	}
	if candidates == nil || len(candidates) > 16 {
		return nil, errors.New("invalid saved stage")
	}
	response := memory.CompilerResponse{RequestID: request.ID, Candidates: make([]memory.ExtractorCandidate, 0, len(candidates))}
	for _, c := range candidates {
		response.Candidates = append(response.Candidates, c.Proposal)
	}
	validated, err := validateCompilerOutput(request, compilerJSON(response))
	if err != nil {
		return nil, err
	}
	if string(compilerJSON(validated)) != string(envelope) {
		return nil, errors.New("stage projection mismatch")
	}
	return validated, nil
}

// compilerStageTransaction keeps the complete adoption decision in one SQLite
// transaction and makes its source-inspection budget directly verifiable.
type compilerStageTransaction interface {
	compilerQueryer
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) adoptCompilerStage(ctx context.Context, owner memory.ScopeContext, jobID string) (compilerClaim, error) {
	var c compilerClaim
	err := s.withCompilerPublicationTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		c, err = adoptCompilerStageInTransaction(ctx, conn, owner, jobID)
		return err
	})
	return c, err
}

func adoptCompilerStageInTransaction(ctx context.Context, conn compilerStageTransaction, owner memory.ScopeContext, jobID string) (compilerClaim, error) {
	var c compilerClaim
	err := func() error {
		inspected, err := loadCompilerInspection(ctx, conn, owner, jobID)
		if err != nil {
			return err
		}
		var eligible int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q ON q.job_id=j.job_id WHERE j.job_id=? AND j.state='staged' AND (j.holder IS NULL OR j.lease_until<=unixepoch('now')) AND q.pause_reason='' AND NOT EXISTS (SELECT 1 FROM memory_compiler_paused_jobs p WHERE p.job_id=j.job_id)`, jobID).Scan(&eligible); err != nil {
			return err
		}
		if eligible != 1 {
			return ErrCompilerFence
		}
		var raw, envelope []byte
		var hash string
		if err := conn.QueryRowContext(ctx, `SELECT j.request,s.envelope,s.envelope_hash FROM memory_compiler_jobs j JOIN memory_compiler_stages s ON s.job_id=j.job_id WHERE j.job_id=? AND s.consumed=0`, jobID).Scan(&raw, &envelope, &hash); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &c.Request); err != nil {
			return err
		}
		// loadCompilerInspection already resolved this same sealed window in
		// this transaction. Validate the envelope without rereading its sources.
		if _, err := validateCompilerStage(c.Request, envelope, hash); err != nil {
			return err
		}
		holder, err := newSemanticID()
		if err != nil {
			return err
		}
		c.JobID, c.Holder, c.Generation = jobID, string(holder), inspected.Generation
		if err := conn.QueryRowContext(ctx, `UPDATE memory_compiler_jobs SET fence=fence+1,holder=?,lease_until=unixepoch('now')+30,reason='' WHERE job_id=? RETURNING fence`, c.Holder, jobID).Scan(&c.Fence); err != nil {
			return err
		}
		if err := reserveCompilerResources(ctx, conn, jobID, c.Fence); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_stages SET fence=? WHERE job_id=?`, c.Fence, jobID)
		return err
	}()
	return c, err
}

// CancelCompilation records owner cancellation before running clients observe
// the new fence. An intact stage is retained as audit and needs explicit resume.
func (s *Store) CancelCompilation(ctx context.Context, owner memory.ScopeContext, id string) (memory.Compilation, error) {
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		inspection, err := loadCompilerInspection(ctx, conn, owner, id)
		if err != nil {
			return err
		}
		if inspection.JobID == "" {
			return errors.New("selection has no materialized job")
		}
		switch inspection.State {
		case "completed_candidates", "completed_empty", "excluded", "failed":
			return errors.New("terminal compilation cannot be cancelled")
		case "cancelled":
			return nil
		}
		// Owner cancellation revokes an expired worker too; worker operations still
		// require the unexpired holder/fence. The old request remains quarantined.
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_capacity SET state='release_pending' WHERE job_id=?`, inspection.JobID); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=?`, inspection.JobID); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state='cancelled',reason='owner_cancelled',fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, inspection.JobID)
		return err
	})
	if err != nil {
		return memory.Compilation{}, err
	}
	return s.InspectCompilation(ctx, owner, id)
}

// ResumeCompilation preserves attempts, due time and sealed input. A fifth
// attempt's intact saved stage can be adopted; a sixth inference cannot occur.
func (s *Store) ResumeCompilation(ctx context.Context, owner memory.ScopeContext, id string) (memory.Compilation, error) {
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		inspection, err := loadCompilerInspection(ctx, conn, owner, id)
		if err != nil {
			return err
		}
		if inspection.JobID == "" {
			return errors.New("selection has no materialized job")
		}
		switch inspection.State {
		case "completed_candidates", "completed_empty", "excluded", "failed":
			return errors.New("terminal compilation cannot be resumed")
		}
		var paused int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_paused_jobs WHERE job_id=?`, inspection.JobID).Scan(&paused); err != nil {
			return err
		}
		if paused != 0 {
			return ErrCompilerFence
		}

		if inspection.State == "cancelled" {
			var unfinished int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE state IN ('queued','running','retry_wait','staged')`).Scan(&unfinished); err != nil {
				return err
			}
			if unfinished >= 1024 {
				return ErrCompilerCapacityBlocked
			}
			var stage int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_stages WHERE job_id=? AND consumed=0`, inspection.JobID).Scan(&stage); err != nil {
				return err
			}
			state := "queued"
			if stage == 1 {
				state = "staged"
				var fence int64
				if err := conn.QueryRowContext(ctx, `SELECT fence FROM memory_compiler_jobs WHERE job_id=?`, inspection.JobID).Scan(&fence); err != nil {
					return err
				}
				if err := reserveCompilerResources(ctx, conn, inspection.JobID, fence); err != nil {
					return err
				}
			} else if inspection.Attempts >= 5 {
				return errors.New("attempts exhausted")
			} else if inspection.Attempts > 0 {
				state = "retry_wait"
			}
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state=?,reason='',retry_at=CASE WHEN ?='retry_wait' THEN COALESCE(retry_at,unixepoch('now')+(5 << (attempts-1))) ELSE retry_at END WHERE job_id=?`, state, state, inspection.JobID); err != nil {
				return err
			}
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_job_schedule SET pause_reason='' WHERE job_id=?`, inspection.JobID)
		return err
	})
	if err != nil {
		return memory.Compilation{}, err
	}
	return s.InspectCompilation(ctx, owner, id)
}

// CompilerSupervisorConfig supplies already validated generation-specific
// dependencies. Copying the map prevents a caller from changing it mid-run.
// Long-lived host lifecycle integration belongs to the activation adapter.
type CompilerSupervisorConfig struct {
	Extractors      map[string]CompilerExtractor
	ReleaseVerifier CompilerReleaseVerifier
}

// RunCompilerSupervisor returns on host shutdown. It polls durable work, keeps
// only one active local client, and never drains work on a short command's exit.
func (s *Store) RunCompilerSupervisor(ctx context.Context, config CompilerSupervisorConfig) error {
	if len(config.Extractors) == 0 || len(config.Extractors) > 32 {
		return ErrCompilerNotConfigured
	}
	pinned := make(map[string]CompilerExtractor, len(config.Extractors))
	for id, e := range config.Extractors {
		if e == nil || e.ServerIdentity() == "" {
			return ErrCompilerNotConfigured
		}
		pinned[id] = e
	}
	config.Extractors = pinned
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		worked, err := s.RunCompilerStep(ctx, config)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Per-job errors are durably visible and must not starve independent work.
		delay := time.Second
		if worked {
			delay = 10 * time.Millisecond
		}
		if err != nil && !worked {
			delay = time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// RunCompilerStep performs at most one inference/publication and a bounded
// recovery batch. It is useful for deterministic hosts/tests as well as the
// supervisor; selection and source authority still come from SQLite.
func (s *Store) RunCompilerStep(ctx context.Context, config CompilerSupervisorConfig) (bool, error) {
	if len(config.Extractors) == 0 || len(config.Extractors) > 32 {
		return false, ErrCompilerNotConfigured
	}
	if err := s.RecoverCompilerWork(ctx); err != nil {
		return false, err
	}
	capacityErr := s.ReconcileCompilerCapacity(ctx, config.ReleaseVerifier)
	// Publication can proceed even while an unrelated server request is blocked.
	keys := make([]string, 0, len(config.Extractors))
	for key, e := range config.Extractors {
		if e != nil && e.ServerIdentity() != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return false, ErrCompilerNotConfigured
	}
	var claim compilerClaim
	var owner memory.ScopeContext
	var jobID string
	var staged bool
	var rejected error
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		args := make([]any, 0, len(keys))
		for _, key := range keys {
			args = append(args, key)
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
		// The fairness predicate includes due/configured/unpaused work only.
		query := `SELECT j.job_id,j.session_id,j.state FROM memory_compiler_jobs j JOIN memory_compiler_job_schedule q ON q.job_id=j.job_id WHERE j.generation_id IN (` + marks + `) AND q.pause_reason='' AND NOT EXISTS (SELECT 1 FROM memory_compiler_paused_jobs p WHERE p.job_id=j.job_id) AND ((j.state='staged' AND (j.holder IS NULL OR j.lease_until<=unixepoch('now'))) OR (j.attempts<5 AND (j.state='queued' OR (j.state='retry_wait' AND j.retry_at<=unixepoch('now'))))) ORDER BY CASE WHEN j.state='staged' THEN 0 WHEN q.lane='historical' AND (SELECT new_attempts FROM memory_compiler_scheduler WHERE singleton=1)>=8 THEN 1 WHEN q.lane='new' THEN 2 ELSE 3 END,CASE WHEN q.lane='new' THEN q.position ELSE q.historical_order END,j.session_id,j.first_sequence,j.job_id LIMIT 1`
		var session memory.SessionID
		var state string
		if err := conn.QueryRowContext(ctx, query, args...).Scan(&jobID, &session, &state); err != nil {
			return err
		}
		owner = memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: session}
		var workspace, project sql.NullString
		if err := conn.QueryRowContext(ctx, `SELECT workspace_id,project_id FROM sessions WHERE id=?`, session).Scan(&workspace, &project); err != nil {
			return err
		}
		owner.WorkspaceID, owner.ProjectID = memory.WorkspaceID(workspace.String), memory.ProjectID(project.String)
		if state == "staged" {
			staged = true
			return nil
		}
		if capacityErr != nil {
			return capacityErr
		}
		var generationID string
		if err := conn.QueryRowContext(ctx, `SELECT generation_id FROM memory_compiler_jobs WHERE job_id=?`, jobID).Scan(&generationID); err != nil {
			return err
		}
		var err error
		claim, err = claimCompilerJob(ctx, conn, owner, jobID, config.Extractors[generationID])
		if errors.Is(err, errCompilerInvalidSource) || errors.Is(err, ErrCompilerConfiguration) {
			rejected = err
			err = rejectCompilerClaim(ctx, conn, jobID, err)
		}
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, capacityErr
	}
	if err != nil {
		return false, err
	}
	if rejected != nil {
		return true, rejected
	}
	if staged {
		claim, err = s.adoptCompilerStage(ctx, owner, jobID)
		if err != nil {
			if compilerDataFailure(err) && !errors.Is(err, ErrCompilerFence) && !errors.Is(err, ErrCompilerCapacityBlocked) {
				cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				defer cancel()
				failedErr := s.withImmediateTransaction(cleanupCtx, func(conn *sql.Conn) error {
					result, e := conn.ExecContext(cleanupCtx, `UPDATE memory_compiler_jobs SET state='failed',reason='invalid_stage',fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=? AND state='staged' AND (holder IS NULL OR lease_until<=unixepoch('now'))`, jobID)
					if e = compilerChanged(result, e); e != nil {
						return e
					}
					_, e = conn.ExecContext(cleanupCtx, `DELETE FROM memory_compiler_resources WHERE job_id=?`, jobID)
					return e
				})
				return true, errors.Join(err, failedErr)
			}
			return false, err
		}
		err = s.publishCompilerResult(ctx, owner, jobID, claim.Holder, claim.Fence, claim.Request)
		if ctx.Err() != nil {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			claim.Supervised = true
			err = errors.Join(err, s.stopCompilerClaim(cleanupCtx, claim, true))
		}
		return true, err
	}
	claim.Supervised = true
	_, err = s.runCompilerClaim(ctx, owner, jobID, claim, config.Extractors[claim.Request.GenerationID])
	return true, err
}

func (s *Store) stopCompilerClaim(ctx context.Context, c compilerClaim, known bool) error {
	if !c.Supervised {
		return s.cancelCompilerAttempt(ctx, c.JobID, c.Holder, c.Fence, known)
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var state string
		err := conn.QueryRowContext(ctx, `SELECT state FROM memory_compiler_jobs WHERE job_id=? AND holder=? AND fence=? AND lease_until>unixepoch('now')`, c.JobID, c.Holder, c.Fence).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			// Owner history cancellation already invalidated this job fence.
			// A subsequently proven transport completion may release only its
			// unchanged request reservation, never a replacement's slot.
			if known {
				_, err = conn.ExecContext(ctx, `DELETE FROM memory_compiler_capacity WHERE job_id=? AND holder=? AND fence=?`, c.JobID, c.Holder, c.Fence)
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		if state == "staged" {
			_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, c.JobID)
			return err
		}
		if state != "running" {
			return ErrCompilerFence
		}
		if err := compilerRelease(ctx, conn, c.JobID, c.Holder, c.Fence, known); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=? AND fence=?`, c.JobID, c.Fence); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state=CASE WHEN attempts=5 THEN 'failed' ELSE 'retry_wait' END,reason=CASE WHEN attempts=5 THEN 'attempts_exhausted' ELSE 'worker_shutdown' END,retry_at=CASE WHEN attempts<5 THEN unixepoch('now')+(5 << (attempts-1)) END,fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, c.JobID)
		return err
	})
}

// SQLite transport/lock errors and cancelled operations cannot prove bad source
// data. Only completed deterministic validation is a terminal source failure.
func compilerDataFailure(err error) bool {
	var coded interface{ Code() int }
	return err != nil && !errors.As(err, &coded) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, sql.ErrConnDone) && !errors.Is(err, driver.ErrBadConn)
}

func rejectCompilerClaim(ctx context.Context, conn *sql.Conn, jobID string, cause error) error {
	reason := "invalid_source"
	if errors.Is(cause, ErrCompilerConfiguration) {
		reason = "invalid_configuration"
	}
	_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state='failed',reason=?,holder=NULL,lease_until=NULL WHERE job_id=? AND state IN ('queued','retry_wait')`, reason, jobID)
	return err
}
