package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectOwnerCandidateLineage follows at most one primary/related link. Links
// retain publication's checked revisions while the separately loaded origin
// shows later owner choices and resolution under current source authorization.
func (s *Store) InspectOwnerCandidateLineage(ctx context.Context, a OwnerReviewContext, id string) (memory.CandidateLineage, error) {
	var out memory.CandidateLineage
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	out.Candidate, err = loadReviewCandidate(ctx, tx, a, id, false)
	if err != nil {
		return memory.CandidateLineage{}, err
	}
	if out.Candidate.Redacted {
		out.OriginRedacted = true
		return out, tx.Commit()
	}
	var manifest, requestRaw []byte
	err = tx.QueryRowContext(ctx, `SELECT g.manifest,j.request FROM memory_compiler_jobs j JOIN memory_compiler_generations g ON g.generation_id=j.generation_id WHERE j.job_id=?`, out.Candidate.JobID).Scan(&manifest, &requestRaw)
	if err != nil {
		return out, err
	}
	var request memory.CompilerRequest
	if json.Unmarshal(manifest, &out.Generation) != nil || json.Unmarshal(requestRaw, &request) != nil {
		return out, ErrReviewInvalidSource
	}
	out.Selection = request.Window.Selection
	var primary string
	var checked memory.CandidateRef
	err = tx.QueryRowContext(ctx, `SELECT encoding_version,relationship,suppressed,COALESCE(primary_id,''),checked_interpretation,checked_review,checked_state FROM memory_compiler_recurrence WHERE candidate_id=?`, id).Scan(&out.ComparisonVersion, &out.Relationship, &out.Suppressed, &primary, &checked.InterpretationRevision, &checked.ReviewRevision, &out.CheckedState)
	if errors.Is(err, sql.ErrNoRows) {
		out.ComparisonVersion = "compiler-equivalence-v1"
		out.Relationship = "legacy"
		primary = out.Candidate.Candidate.EquivalentTo
		out.Suppressed = primary != ""
	} else if err != nil {
		return out, err
	}
	if primary == "" {
		out.CheckedState = ""
		return out, tx.Commit()
	}
	checked.ID = primary
	out.Checked = &checked
	origin, err := loadReviewCandidate(ctx, tx, a, primary, false)
	if errors.Is(err, ErrOwnerReviewUnauthorized) || errors.Is(err, ErrReviewInvalidSource) {
		out.OriginRedacted = true
		return out, tx.Commit()
	}
	if err != nil {
		return out, err
	}
	if origin.Redacted {
		out.OriginRedacted = true
		return out, tx.Commit()
	}
	out.Origin = &origin
	if origin.Candidate.ReviewState != "unresolved" {
		audit, e := compilerRecurrenceAudit(ctx, tx, primary)
		if e != nil {
			return out, e
		}
		// A compound decision may disclose other members. Check their sources too;
		// retained authority is not permission to disclose a formerly visible group.
		if len(audit.Preview.Candidates) > 64 {
			return out, ErrReviewTooLarge
		}
		for _, member := range audit.Preview.Candidates {
			if member.Ref.ID == primary {
				continue
			}
			visible, e := loadReviewCandidate(ctx, tx, a, member.Ref.ID, false)
			if e != nil {
				return out, e
			}
			if visible.Redacted {
				out.OriginRedacted = true
				out.Origin = nil
				return out, tx.Commit()
			}
		}
		if !reviewReasonValid(audit.Decision.Reason) {
			out.OriginRedacted = true
			out.Origin = nil
			return out, tx.Commit()
		}
		out.Decision = &audit.Decision
		out.Resolution = &audit.Result
	}
	return out, tx.Commit()
}
