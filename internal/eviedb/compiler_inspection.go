package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// InspectCompilation returns a bounded selected unit. It reports gaps and
// exclusions separately from successful empty output and never mutates memory.
func (s *Store) InspectCompilation(ctx context.Context, owner memory.ScopeContext, id string) (memory.Compilation, error) {
	var result memory.Compilation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var err error
		result, err = loadCompilerInspection(ctx, conn, owner, id)
		return err
	})
	if err != nil {
		return memory.Compilation{}, err
	}
	return result, nil
}

// loadCompilerInspection is also the transactional owner-review read seam.
// Candidate original envelopes are immutable; review_revision begins at zero.
func loadCompilerInspection(ctx context.Context, q compilerQueryer, owner memory.ScopeContext, id string) (memory.Compilation, error) {
	var result memory.Compilation
	var manifest, window []byte
	var job sql.NullString
	err := q.QueryRowContext(ctx, `SELECT s.selection_id,s.job_id,s.generation_id,g.manifest,s.window,s.state,s.reason FROM memory_compiler_selections s JOIN memory_compiler_generations g ON g.generation_id=s.generation_id WHERE s.selection_id=? OR s.job_id=? LIMIT 1`, id, id).Scan(&result.SelectionID, &job, &result.GenerationID, &manifest, &window, &result.State, &result.Reason)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(manifest, &result.Generation); err != nil {
		return result, err
	}
	generationID, canonical, err := memory.CompilerGenerationIdentity(result.Generation)
	if err != nil || generationID != result.GenerationID || string(canonical) != string(manifest) {
		return result, errors.New("generation seal mismatch")
	}
	if err := json.Unmarshal(window, &result.Window); err != nil {
		return result, err
	}
	if err := compilerAuthorize(ctx, q, owner, result.Window.Selection); err != nil {
		return result, err
	}
	result.Candidates = []memory.MemoryCandidate{}
	if !job.Valid {
		return result, nil
	}
	result.JobID = job.String
	var sealed []byte
	var hash string
	if err := q.QueryRowContext(ctx, `SELECT state,reason,attempts,request,window_hash FROM memory_compiler_jobs WHERE job_id=?`, result.JobID).Scan(&result.State, &result.Reason, &result.Attempts, &sealed, &hash); err != nil {
		return result, err
	}
	var request memory.CompilerRequest
	if err := json.Unmarshal(sealed, &request); err != nil {
		return result, err
	}
	requestID := request.ID
	request.ID = ""
	if memory.CompilerHash(compilerJSON(request)) != requestID || memory.CompilerHash(compilerJSON(request.Window)) != hash || hash != request.WindowSHA256 || request.GenerationID != result.GenerationID || string(compilerJSON(request.Window)) != string(window) {
		return result, errors.New("request seal mismatch")
	}
	request.ID = requestID
	// All source fields are checked once, at most the bounded window event count.
	ctx = withCompilerSourceCache(ctx)
	for _, source := range result.Window.Sources {
		if _, err := resolveCompilerSource(ctx, q, owner, result.Window.Selection, source); err != nil {
			return result, err
		}
	}
	var capacity string
	err = q.QueryRowContext(ctx, `SELECT state FROM memory_compiler_capacity WHERE job_id=?`, result.JobID).Scan(&capacity)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	result.CapacityState = capacity
	rows, err := q.QueryContext(ctx, `SELECT envelope,review_state,review_revision,COALESCE(equivalent_to,'') FROM memory_compiler_candidates WHERE job_id=? ORDER BY ordinal LIMIT 17`, result.JobID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var raw []byte
		var state, equivalent string
		var revision int64
		if err := rows.Scan(&raw, &state, &revision, &equivalent); err != nil {
			rows.Close()
			return result, err
		}
		var candidate memory.MemoryCandidate
		if err := json.Unmarshal(raw, &candidate); err != nil {
			rows.Close()
			return result, err
		}
		if compilerHasSecret(string(raw)) {
			rows.Close()
			return result, errors.New("candidate inspection secret exclusion")
		}
		candidate.ReviewState = state
		candidate.ReviewRevision = revision
		candidate.EquivalentTo = equivalent
		// Recompute projections from the currently checked sealed fields; never
		// trust stored candidate quotes as a full-event fallback.
		for _, sources := range [][]memory.CompilerSource{candidate.Support, candidate.Context} {
			for _, source := range sources {
				found := false
				for _, offered := range request.Window.Sources {
					if offered.Locator.EventID != source.Locator.EventID {
						continue
					}
					projected, err := projectCompilerSource(offered, source.Locator)
					if err != nil || string(compilerJSON(projected)) != string(compilerJSON(source)) {
						rows.Close()
						return result, errors.New("candidate source seal mismatch")
					}
					found = true
					break
				}
				if !found {
					rows.Close()
					return result, errors.New("candidate source missing from window")
				}
			}
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(result.Candidates) > 16 {
		return result, errors.New("candidate group bound")
	}
	return result, nil
}
