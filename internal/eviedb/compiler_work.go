package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrCompilerNotConfigured = errors.New("memory compiler is not configured")
var ErrCompilerCapacityBlocked = errors.New("memory compiler capacity blocked")
var ErrCompilerConfiguration = errors.New("unsafe or unverifiable compiler configuration")
var ErrCompilerFence = errors.New("memory compiler attempt lost authority")

// CompilerExtractor consumes only sealed data. A completion marker proves
// server release; a timeout, disconnected socket, or client cancellation does
// not. Scripted and local HTTP extractors implement this same boundary.
type CompilerExtractor interface {
	ServerIdentity() string
	Extract(context.Context, memory.CompilerGeneration, memory.CompilerRequest) (CompilerExtraction, error)
}
type CompilerExtraction struct {
	Raw             []byte
	ReleaseEvidence string
}

// CompileCandidateUnit explicitly selects and attempts one bounded source unit.
// It does not activate scheduling or retry an earlier attempt. No SQLite
// transaction remains open while Extract runs.
func (s *Store) CompileCandidateUnit(ctx context.Context, owner memory.ScopeContext, selection memory.CompilationSelection, generation memory.CompilerGeneration, extractor CompilerExtractor) (memory.Compilation, error) {
	if extractor == nil || extractor.ServerIdentity() == "" {
		return memory.Compilation{}, ErrCompilerNotConfigured
	}
	generationID, manifest, err := memory.CompilerGenerationIdentity(generation)
	if err != nil {
		return memory.Compilation{}, err
	}
	// Detach caller-owned RawMessage storage before sealing or dispatch.
	generation = memory.CompilerGeneration{}
	if err := json.Unmarshal(manifest, &generation); err != nil {
		return memory.Compilation{}, err
	}
	selectionID, jobID, err := s.selectCompilerUnit(ctx, owner, selection, generationID, manifest, generation)
	if err != nil {
		return memory.Compilation{}, err
	}
	inspection, err := s.InspectCompilation(ctx, owner, selectionID)
	if err != nil {
		return inspection, err
	}
	if inspection.State != "queued" {
		return inspection, nil
	}
	claim, err := s.claimCompilerJob(ctx, owner, jobID, extractor)
	if err != nil {
		result, readErr := s.InspectCompilation(ctx, owner, selectionID)
		return result, errors.Join(err, readErr)
	}
	return s.runCompilerClaim(ctx, owner, selectionID, claim, extractor)
}

func (s *Store) runCompilerClaim(ctx context.Context, owner memory.ScopeContext, selectionID string, claim compilerClaim, extractor CompilerExtractor) (memory.Compilation, error) {
	jobID, holder, fence := claim.JobID, claim.Holder, claim.Fence
	request, generation := claim.Request, claim.Generation
	var err error
	if err := ctx.Err(); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		stopErr := s.stopCompilerClaim(cleanupCtx, claim, true)
		result, readErr := s.InspectCompilation(cleanupCtx, owner, selectionID)
		return result, errors.Join(err, stopErr, readErr)
	}
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 120*time.Second)
	callerWatchStop := make(chan struct{})
	callerWatchDone := make(chan struct{})
	var callerFenceErr error
	go func() {
		defer close(callerWatchDone)
		select {
		case <-ctx.Done():
		case <-callerWatchStop:
			if ctx.Err() == nil {
				return
			}
		}
		// Fence before cancelling the local client. If SQLite is unavailable,
		// cleanup remains bounded and the durable reservation still blocks it.
		fenceCtx, fenceCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		callerFenceErr = s.stopCompilerClaim(fenceCtx, claim, false)
		fenceCancel()
		cancel()
	}()
	heartbeats := make(chan struct{})
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(100 * time.Millisecond)
		lastRenewal := time.Now()
		defer ticker.Stop()
		for {
			select {
			case <-heartbeats:
				return
			case <-callCtx.Done():
				return
			case <-ticker.C:
				checkCtx, checkCancel := context.WithTimeout(callCtx, 500*time.Millisecond)
				var current int
				err := s.db.QueryRowContext(checkCtx, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE job_id=? AND holder=? AND fence=? AND lease_until>unixepoch('now') AND state='running' AND NOT EXISTS (SELECT 1 FROM memory_compiler_invalid_claims p WHERE p.job_id=memory_compiler_jobs.job_id)`, jobID, holder, fence).Scan(&current)
				if err == nil && current != 1 {
					err = ErrCompilerFence
				}
				if err == nil && time.Since(lastRenewal) >= 10*time.Second {
					err = s.renewCompilerClaim(checkCtx, claim)
					lastRenewal = time.Now()
				}
				checkCancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	var dispatched memory.CompilerRequest
	if err := json.Unmarshal(compilerJSON(request), &dispatched); err != nil {
		return memory.Compilation{}, err
	}
	dispatched.AttemptID = claim.AttemptID

	var extracted CompilerExtraction
	var extractErr error
	if callCtx.Err() != nil {
		extractErr = callCtx.Err()
		extracted.ReleaseEvidence = "not_dispatched"
	} else {
		extracted, extractErr = extractor.Extract(callCtx, generation, dispatched)
	}
	close(callerWatchStop)
	<-callerWatchDone
	close(heartbeats)
	cancel()
	<-heartbeatDone
	cancelledResult := func(cause error) (memory.Compilation, error) {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		fenceErr := s.stopCompilerClaim(cleanupCtx, claim, compilerReleaseKnown(extracted.ReleaseEvidence))
		result, readErr := s.InspectCompilation(cleanupCtx, owner, selectionID)
		return result, errors.Join(ctx.Err(), cause, callerFenceErr, fenceErr, readErr)
	}
	if ctx.Err() != nil {
		return cancelledResult(nil)
	}
	publicationCtx, publicationCancel := context.WithTimeout(ctx, 5*time.Second)
	defer publicationCancel()

	if extracted.ReleaseEvidence != "completed" {
		if extractErr == nil {
			extractErr = errors.New("missing server completion evidence")
		}
	}
	var candidates []memory.MemoryCandidate
	if extractErr == nil {
		candidates, extractErr = validateCompilerOutput(request, extracted.Raw)
	}
	if extractErr != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		reason := "invalid_or_missing_output"
		if errors.Is(extractErr, ErrCompilerConfiguration) {
			reason = "invalid_configuration"
		}
		if errors.Is(extractErr, ErrCompilerTerminalOutput) {
			reason = "invalid_source_or_effect"
		}
		err = s.failCompilerAttempt(cleanupCtx, jobID, string(holder), fence, compilerReleaseKnown(extracted.ReleaseEvidence), reason, errors.Is(extractErr, ErrCompilerConfiguration) || errors.Is(extractErr, ErrCompilerTerminalOutput))
		if errors.Is(err, ErrCompilerFence) {
			err = errors.Join(err, s.stopCompilerClaim(cleanupCtx, claim, compilerReleaseKnown(extracted.ReleaseEvidence)))
		}
		result, readErr := s.InspectCompilation(cleanupCtx, owner, selectionID)
		return result, errors.Join(extractErr, err, readErr)
	}
	if err = s.stageCompilerResult(publicationCtx, owner, jobID, string(holder), fence, request, candidates); err != nil {
		if ctx.Err() != nil {
			return cancelledResult(err)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		reason := "staging_failed"
		terminal := compilerDataFailure(err) && !errors.Is(err, ErrCompilerFence) && !errors.Is(err, ErrCompilerCapacityBlocked)
		if terminal {
			reason = "invalid_source"
		}
		failedErr := s.failCompilerAttempt(cleanupCtx, jobID, string(holder), fence, true, reason, terminal)
		if errors.Is(failedErr, ErrCompilerFence) {
			failedErr = errors.Join(failedErr, s.stopCompilerClaim(cleanupCtx, claim, true))
		}
		return memory.Compilation{}, errors.Join(err, failedErr)
	}
	if err = s.publishCompilerResult(publicationCtx, owner, jobID, string(holder), fence, request); err != nil {
		if ctx.Err() != nil {
			return cancelledResult(err)
		}
		return memory.Compilation{}, err
	}
	return s.InspectCompilation(ctx, owner, selectionID)
}

func (s *Store) selectCompilerUnit(ctx context.Context, owner memory.ScopeContext, selection memory.CompilationSelection, generationID string, manifest []byte, generation memory.CompilerGeneration) (selectionID, jobID string, err error) {
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var selectErr error
		selectionID, jobID, selectErr = selectCompilerUnitInTransaction(ctx, conn, owner, selection, generationID, manifest, generation, compilerScheduling{Lane: "historical"})
		return selectErr
	})
	return
}

func selectCompilerUnitInTransaction(ctx context.Context, conn *sql.Conn, owner memory.ScopeContext, selection memory.CompilationSelection, generationID string, manifest []byte, generation memory.CompilerGeneration, scheduling compilerScheduling) (selectionID, jobID string, err error) {
	if selection.Cutoff <= 0 || selection.RootID == "" {
		return "", "", errors.New("invalid finite source selection")
	}
	if scheduling.Lane != "new" && scheduling.Lane != "historical" {
		return "", "", errors.New("invalid scheduling lane")
	}
	err = func() error {
		if err := compilerAuthorize(ctx, conn, owner, selection); err != nil {
			return err
		}
		var requestedFirst int64
		if err := conn.QueryRowContext(ctx, `SELECT sequence FROM events WHERE id=? AND session_id=?`, selection.RootID, selection.SessionID).Scan(&requestedFirst); err != nil {
			return err
		}
		if scheduling.FirstSequence > requestedFirst {
			requestedFirst = scheduling.FirstSequence
		}
		interval, err := nextCompilerInterval(ctx, conn, generationID, selection, requestedFirst)
		if err != nil {
			return err
		}
		first := interval.First
		selection.Cutoff = interval.Last
		selectionID, jobID = interval.SelectionID, interval.JobID
		newSelection := selectionID == ""
		if !newSelection && interval.State != "deferred_live" && interval.State != "selected_unmaterialized" {
			return nil
		}
		if newSelection {
			selectionID = memory.CompilerHash(compilerJSON(struct {
				Generation string
				Selection  memory.CompilationSelection
			}{generationID, selection}))
		}

		window, state, reason, err := captureCompilerWindow(ctx, conn, owner, selection, first)
		if err != nil {
			return err
		}
		// Live activation owns its revisit obligation separately. Do not create
		// an immutable-looking partial cut before its exact root can be sealed.
		if state == "deferred_live" && scheduling.AwaitClosure {
			selectionID = ""
			return nil
		}
		if _, err := conn.ExecContext(ctx, `INSERT OR IGNORE INTO memory_compiler_generations VALUES(?,?)`, generationID, manifest); err != nil {
			return err
		}
		if newSelection {
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_selections(selection_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,state,reason,window) VALUES(?,?,?,?,?,?,?,?,?,?)`, selectionID, generationID, selection.Destination, selection.SessionID, selection.RootID, first, selection.Cutoff, state, reason, compilerJSON(window)); err != nil {
				return err
			}
		} else {
			if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_selections SET state=?,reason=?,window=? WHERE selection_id=? AND job_id IS NULL`, state, reason, compilerJSON(window), selectionID); err != nil {
				return err
			}
		}
		if state == "deferred_live" {
			return nil
		}
		var unfinished int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE state IN ('queued','running','retry_wait','staged')`).Scan(&unfinished); err != nil {
			return err
		}
		if unfinished >= 1024 {
			_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_selections SET state='selected_unmaterialized',reason='job_capacity' WHERE selection_id=?`, selectionID)
			return err
		}
		request := memory.CompilerRequest{GenerationID: generationID, WindowSHA256: memory.CompilerHash(compilerJSON(window)), Window: window, Entities: []memory.SemanticEntity{}, Predicates: []memory.SemanticPredicate{}, ScopeRevisions: []memory.ScopeRevision{}}
		jobID = memory.CompilerHash(compilerJSON(struct{ Generation, Window string }{generationID, request.WindowSHA256}))
		request.ID = memory.CompilerHash(compilerJSON(request))
		if state == "queued" {
			if err := memory.CompilerInputBudget(generation, request); err != nil {
				state = "failed"
				reason = err.Error()
			}
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_jobs(job_id,generation_id,destination,session_id,root_id,first_sequence,last_sequence,window_hash,request,state,reason) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, jobID, generationID, selection.Destination, selection.SessionID, selection.RootID, first, selection.Cutoff, request.WindowSHA256, compilerJSON(request), state, reason); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_job_schedule SET lane=?,position=?,historical_order=? WHERE job_id=?`, scheduling.Lane, scheduling.Position, scheduling.HistoricalOrder, jobID); err != nil {
			return err
		}
		if scheduling.HistorySelected {
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_history_jobs(job_id) VALUES(?)`, jobID); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_selections SET job_id=?,state=?,reason=? WHERE selection_id=?`, jobID, state, reason, selectionID); err != nil {
			return err
		}
		if state == "excluded" {
			_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_coverage VALUES(?,'excluded',?)`, jobID, compilerJSON(window.NewEventIDs))
			return err
		}
		return nil
	}()
	return
}

func compilerChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrCompilerFence
	}
	return nil
}
func compilerFence(ctx context.Context, conn *sql.Conn, job, holder string, fence int64, state string) error {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE job_id=? AND holder=? AND fence=? AND lease_until>unixepoch('now') AND state=? AND NOT EXISTS (SELECT 1 FROM memory_compiler_invalid_claims p WHERE p.job_id=memory_compiler_jobs.job_id)`, job, holder, fence, state).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return ErrCompilerFence
	}
	return nil
}
func compilerRelease(ctx context.Context, conn *sql.Conn, job, holder string, fence int64, known bool) error {
	query := `UPDATE memory_compiler_capacity SET state='release_pending' WHERE job_id=? AND holder=? AND fence=?`
	if known {
		query = `DELETE FROM memory_compiler_capacity WHERE job_id=? AND holder=? AND fence=?`
	}
	result, err := conn.ExecContext(ctx, query, job, holder, fence)
	return compilerChanged(result, err)
}
func (s *Store) cancelCompilerAttempt(ctx context.Context, job, holder string, fence int64, known bool) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var state string
		err := conn.QueryRowContext(ctx, `SELECT state FROM memory_compiler_jobs WHERE job_id=? AND holder=? AND fence=? AND lease_until>unixepoch('now') AND state IN ('running','staged')`, job, holder, fence).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			// The inference watcher may already have fenced this same attempt.
			// A late result cannot change its conservative release disposition.
			var cancelled int
			err = conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs WHERE job_id=? AND fence=? AND state='cancelled' AND reason='caller_cancelled'`, job, fence+1).Scan(&cancelled)
			if err != nil {
				return err
			}
			if cancelled == 1 {
				return nil
			}
			return ErrCompilerFence
		}
		if err != nil {
			return err
		}
		if state == "running" {
			if err := compilerRelease(ctx, conn, job, holder, fence, known); err != nil {
				return err
			}
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=? AND fence=?`, job, fence); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state='cancelled',reason='caller_cancelled',fence=fence+1,holder=NULL,lease_until=NULL WHERE job_id=?`, job)
		return err
	})
}

func (s *Store) failCompilerAttempt(ctx context.Context, job, holder string, fence int64, known bool, reason string, terminal bool) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := compilerFence(ctx, conn, job, holder, fence, "running"); err != nil {
			return err
		}
		if err := compilerRelease(ctx, conn, job, holder, fence, known); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=? AND fence=?`, job, fence); err != nil {
			return err
		}
		_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state=CASE WHEN attempts=5 OR ? THEN 'failed' ELSE 'retry_wait' END,reason=CASE WHEN attempts=5 THEN 'attempts_exhausted' ELSE ? END,retry_at=CASE WHEN attempts<5 AND NOT ? THEN unixepoch('now')+(5 << (attempts-1)) END,holder=NULL,lease_until=NULL WHERE job_id=?`, terminal, reason, terminal, job)
		return err
	})
}

func (s *Store) stageCompilerResult(ctx context.Context, owner memory.ScopeContext, job, holder string, fence int64, request memory.CompilerRequest, candidates []memory.MemoryCandidate) error {
	envelope := compilerJSON(candidates)
	if len(envelope) > memory.CompilerMaxBytes {
		return errors.New("staged envelope limit")
	}
	hash := memory.CompilerHash(envelope)
	return s.withCompilerPublicationTransaction(ctx, func(conn *sql.Conn) error {
		if err := compilerAuthorize(ctx, conn, owner, request.Window.Selection); err != nil {
			return err
		}
		var existingHash, state string
		var existingFence int64
		existingErr := conn.QueryRowContext(ctx, `SELECT s.envelope_hash,s.fence,j.state FROM memory_compiler_stages s JOIN memory_compiler_jobs j ON j.job_id=s.job_id WHERE s.job_id=?`, job).Scan(&existingHash, &existingFence, &state)
		if existingErr == nil {
			if existingHash != hash || existingFence != fence {
				return errors.New("staging receipt conflict")
			}
			if state == "completed_candidates" || state == "completed_empty" {
				return nil
			}
			return compilerFence(ctx, conn, job, holder, fence, "staged")
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}

		if err := compilerFence(ctx, conn, job, holder, fence, "running"); err != nil {
			return err
		}
		if err := compilerAuthorize(ctx, conn, owner, request.Window.Selection); err != nil {
			return err
		}
		for _, source := range request.Window.Sources {
			if _, err := resolveCompilerSource(ctx, conn, owner, request.Window.Selection, source); err != nil {
				return err
			}
		}

		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_stages VALUES(?,?,?,?,0)`, job, fence, envelope, hash); err != nil {
			return err
		}
		if err := compilerRelease(ctx, conn, job, holder, fence, true); err != nil {
			return err
		}
		_, err := conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state='staged' WHERE job_id=?`, job)
		return err
	})
}
func (s *Store) publishCompilerResult(ctx context.Context, owner memory.ScopeContext, job, holder string, fence int64, request memory.CompilerRequest) error {
	return s.withCompilerPublicationTransaction(ctx, func(conn *sql.Conn) error {
		if err := compilerAuthorize(ctx, conn, owner, request.Window.Selection); err != nil {
			return err
		}
		var completed int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_compiler_jobs j JOIN memory_compiler_candidate_groups g ON g.job_id=j.job_id JOIN memory_compiler_stages s ON s.job_id=j.job_id WHERE j.job_id=? AND j.state IN ('completed_candidates','completed_empty') AND s.consumed=1 AND s.envelope_hash=g.envelope_hash`, job).Scan(&completed); err != nil {
			return err
		}
		if completed == 1 {
			return nil
		}

		if err := compilerFence(ctx, conn, job, holder, fence, "staged"); err != nil {
			return err
		}
		if err := compilerAuthorize(ctx, conn, owner, request.Window.Selection); err != nil {
			return err
		}
		var envelope []byte
		var hash string
		if err := conn.QueryRowContext(ctx, `SELECT envelope,envelope_hash FROM memory_compiler_stages WHERE job_id=? AND fence=? AND consumed=0`, job, fence).Scan(&envelope, &hash); err != nil {
			return err
		}
		if memory.CompilerHash(envelope) != hash {
			return errors.New("staging seal mismatch")
		}
		// Adoption validated sources in its earlier transaction. Publication
		// independently rechecks current eligibility before committing coverage.
		for _, source := range request.Window.Sources {
			if _, err := resolveCompilerSource(ctx, conn, owner, request.Window.Selection, source); err != nil {
				return err
			}
		}
		candidates, err := validateCompilerStage(request, envelope, hash)
		if err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_candidate_groups VALUES(?,?)`, job, hash); err != nil {
			return err
		}
		for index, candidate := range candidates {
			candidate.ID = memory.CompilerHash([]byte(fmt.Sprintf("%s:%s:%d", job, hash, index)))
			equivalence := memory.CompilerHash(compilerEquivalenceEncoding(request.Window.Selection, candidate))
			var equivalent string
			err := conn.QueryRowContext(ctx, `SELECT candidate_id FROM memory_compiler_candidates WHERE equivalence_hash=? AND equivalent_to IS NULL ORDER BY candidate_id LIMIT 1`, equivalence).Scan(&equivalent)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			candidate.EquivalentTo = equivalent
			if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_candidates(candidate_id,job_id,ordinal,envelope,equivalence_hash,equivalent_to) VALUES(?,?,?,?,?,NULLIF(?,''))`, candidate.ID, job, index, compilerJSON(candidate), equivalence, equivalent); err != nil {
				return err
			}
		}
		state := "completed_candidates"
		if len(candidates) == 0 {
			state = "completed_empty"
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_coverage VALUES(?,?,?)`, job, state, compilerJSON(request.Window.NewEventIDs)); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `UPDATE memory_compiler_stages SET consumed=1 WHERE job_id=?`, job); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx, `DELETE FROM memory_compiler_resources WHERE job_id=? AND fence=?`, job, fence); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `UPDATE memory_compiler_jobs SET state=?,reason='',holder=NULL,lease_until=NULL WHERE job_id=?`, state, job)
		return err
	})
}

func compilerReleaseKnown(evidence string) bool {
	return evidence == "completed" || evidence == "not_dispatched"
}

// Publication is caller-authorized work. Its COMMIT must observe cancellation
// even after the operation callback finishes; only rollback may detach for
// bounded transaction cleanup.
func (s *Store) withCompilerPublicationTransaction(ctx context.Context, operation func(*sql.Conn) error) error {
	resolve := func(resolutionCtx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
		if statement == "COMMIT" {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return s.resolveImmediateTransaction(ctx, conn, statement)
		}
		return s.resolveImmediateTransaction(resolutionCtx, conn, statement)
	}
	return withImmediateTransactionResolver(ctx, s.db, resolve, s.newResolutionContext, operation)
}

// compilerEquivalenceEncoding v1 treats reference order as presentation, while
// retaining exact source/category/authority/effect values. Sorting copies keeps
// the original extraction and its sealed staging envelope unchanged.
func compilerEquivalenceEncoding(selection memory.CompilationSelection, candidate memory.MemoryCandidate) []byte {
	proposal := candidate.Proposal
	proposal.Support = append([]memory.EvidenceLocator{}, proposal.Support...)
	proposal.Context = append([]memory.EvidenceLocator{}, proposal.Context...)
	support := append([]memory.CompilerSource{}, candidate.Support...)
	contextSources := append([]memory.CompilerSource{}, candidate.Context...)
	sort.Slice(proposal.Support, func(i, j int) bool {
		return string(compilerJSON(proposal.Support[i])) < string(compilerJSON(proposal.Support[j]))
	})
	sort.Slice(proposal.Context, func(i, j int) bool {
		return string(compilerJSON(proposal.Context[i])) < string(compilerJSON(proposal.Context[j]))
	})
	sort.Slice(support, func(i, j int) bool { return string(compilerJSON(support[i])) < string(compilerJSON(support[j])) })
	sort.Slice(contextSources, func(i, j int) bool {
		return string(compilerJSON(contextSources[i])) < string(compilerJSON(contextSources[j]))
	})
	return compilerJSON(struct {
		Encoding, Policy, Destination string
		Session                       memory.SessionID
		Proposal                      memory.ExtractorCandidate
		Support, Context              []memory.CompilerSource
	}{"compiler-equivalence-v1", memory.CompilerPolicyVersion, selection.Destination, selection.SessionID, proposal, support, contextSources})
}
