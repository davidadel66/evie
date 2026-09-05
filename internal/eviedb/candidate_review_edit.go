package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/davidadel66/evie/internal/memory"
	"unicode/utf8"
)

const reviewEditSchema = `
CREATE TABLE IF NOT EXISTS memory_review_edit_revisions (
 candidate_id TEXT NOT NULL REFERENCES memory_compiler_candidates(candidate_id),
 revision INTEGER NOT NULL CHECK(revision>0), envelope BLOB NOT NULL CHECK(length(envelope)<=262144),
 PRIMARY KEY(candidate_id,revision)
);
CREATE TRIGGER IF NOT EXISTS memory_review_edit_no_update BEFORE UPDATE ON memory_review_edit_revisions BEGIN SELECT RAISE(ABORT,'edit revisions are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_edit_no_delete BEFORE DELETE ON memory_review_edit_revisions BEGIN SELECT RAISE(ABORT,'edit revisions are immutable'); END;
`

func reviewReasonValid(reason string) bool {
	return len(reason) <= 4096 && utf8.ValidString(reason) && !compilerHasSecret(reason)
}
func editMeaning(item memory.OwnerCandidate) memory.ReviewEditMeaning {
	return memory.ReviewEditMeaning{Proposal: item.Candidate.Proposal, Support: item.Candidate.Support, Context: item.Candidate.Context, Identity: item.Identity, Temporal: item.Temporal}
}
func (s *Store) EditOwnerCandidate(ctx context.Context, a OwnerReviewContext, d memory.ReviewEditDecision) (memory.OwnerCandidate, error) {
	var result memory.OwnerCandidate
	if !reviewReasonValid(d.Reason) {
		return result, ErrReviewInvalidRequest
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		if err := requireReviewUnresolved(ctx, conn, a, d.Candidate.ID); err != nil {
			return err
		}
		item, err := loadReviewCandidate(ctx, conn, a, d.Candidate.ID, true)
		if err != nil {
			return err
		}
		if item.Candidate.ReviewState != "unresolved" {
			return ErrReviewResolved
		}
		if item.Ref != d.Candidate {
			return ErrReviewStale
		}
		if item.Candidate.EquivalentTo != "" {
			return ErrReviewStale
		}
		var raw []byte
		var request memory.CompilerRequest
		if err = conn.QueryRowContext(ctx, `SELECT request FROM memory_compiler_jobs WHERE job_id=?`, item.JobID).Scan(&raw); err != nil {
			return err
		}
		if json.Unmarshal(raw, &request) != nil {
			return ErrReviewInvalidSource
		}
		validated, err := validateOwnerEditedProposal(request, d.Proposal)
		if err != nil {
			return err
		}
		next := item
		next.Candidate = validated[0]
		next.Identity = nil
		next.Temporal = nil
		canonicalizeReviewCandidate(&next)
		owner, err := reviewSourceContext(ctx, conn, request.Window.Selection.SessionID)
		if err != nil {
			return err
		}
		for _, sources := range [][]memory.CompilerSource{next.Candidate.Support, next.Candidate.Context} {
			for _, source := range sources {
				if _, err = resolveCompilerSource(ctx, conn, owner, request.Window.Selection, source); err != nil {
					if !compilerDataFailure(err) {
						return err
					}
					return ErrReviewInvalidSource
				}
			}
		}
		id, err := newSemanticID()
		if err != nil {
			return err
		}
		revision := memory.ReviewEditRevision{Revision: item.Ref.InterpretationRevision + 1, ParentRevision: item.Ref.InterpretationRevision, ReviewRevision: item.Ref.ReviewRevision + 1, AuditID: string(id), OwnerID: memory.LocalOwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, CandidateID: item.Ref.ID, Before: editMeaning(item), After: editMeaning(next), Reason: d.Reason}
		if len(compilerJSON(revision)) > 256*1024 {
			return ErrReviewTooLarge
		}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_edit_revisions VALUES(?,?,?)`, item.Ref.ID, revision.Revision, compilerJSON(revision)); err != nil {
			return err
		}
		updated, err := conn.ExecContext(ctx, `UPDATE memory_compiler_candidates SET review_revision=review_revision+1 WHERE candidate_id=? AND review_revision=? AND review_state='unresolved'`, item.Ref.ID, item.Ref.ReviewRevision)
		if err != nil {
			return err
		}
		n, err := updated.RowsAffected()
		if err != nil || n != 1 {
			return ErrReviewStale
		}
		result, err = loadReviewCandidate(ctx, conn, a, item.Ref.ID, true)
		return err
	})
	if err != nil {
		return memory.OwnerCandidate{}, err
	}
	return result, nil
}
func loadReviewEditRevision(ctx context.Context, q reviewQuery, a OwnerReviewContext, item *memory.OwnerCandidate, request memory.CompilerRequest, requireSources bool) error {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT envelope FROM memory_review_edit_revisions WHERE candidate_id=? ORDER BY revision DESC LIMIT 1`, item.Ref.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var r memory.ReviewEditRevision
	if json.Unmarshal(raw, &r) != nil || r.CandidateID != item.Ref.ID || r.Revision < 1 || r.ParentRevision != r.Revision-1 || r.ReviewRevision > item.Ref.ReviewRevision {
		return errors.New("invalid stored edit revision")
	}
	item.Ref.InterpretationRevision = r.Revision
	if item.Redacted {
		return nil
	}
	original := item.Candidate
	item.Original = &original
	item.Candidate.Proposal = r.After.Proposal
	item.Candidate.Support = r.After.Support
	item.Candidate.Context = r.After.Context
	item.Edit = &r
	validated, err := validateOwnerEditedProposal(request, r.After.Proposal)
	if err == nil && len(validated) == 1 {
		v := memory.OwnerCandidate{Candidate: validated[0]}
		canonicalizeReviewCandidate(&v)
		if string(compilerJSON(editMeaning(v))) != string(compilerJSON(r.After)) {
			err = ErrReviewInvalidSource
		}
	}
	if err == nil {
		owner, ownerErr := reviewSourceContext(ctx, q, request.Window.Selection.SessionID)
		err = ownerErr
		if err == nil {
			for _, sources := range reviewCandidateDisclosureSources(*item) {
				for _, source := range sources {
					if _, err = resolveCompilerSource(ctx, q, owner, request.Window.Selection, source); err != nil {
						break
					}
				}
				if err != nil {
					break
				}
			}
		}
	}
	if err != nil || compilerHasSecret(string(raw)) {
		if err != nil && !compilerDataFailure(err) {
			return err
		}
		if requireSources {
			return ErrReviewInvalidSource
		}
		item.Redacted = true
		item.Original = nil
		item.Edit = nil
		item.Candidate = memory.MemoryCandidate{ID: item.Ref.ID, ReviewState: item.Candidate.ReviewState, ReviewRevision: item.Ref.ReviewRevision, EquivalentTo: item.Candidate.EquivalentTo}
	}
	return nil
}
func (s *Store) InspectOwnerCandidateEditRevision(ctx context.Context, a OwnerReviewContext, id string, revision int64) (memory.ReviewEditRevision, error) {
	var out memory.ReviewEditRevision
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	item, err := loadReviewCandidate(ctx, tx, a, id, true)
	if err != nil {
		return out, err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT envelope FROM memory_review_edit_revisions WHERE candidate_id=? AND revision=?`, id, revision).Scan(&raw); err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out) != nil || compilerHasSecret(string(raw)) {
		return memory.ReviewEditRevision{}, ErrReviewInvalidSource
	}
	// Old interpretations may cite different ranges from the frozen window; every
	// historical quotation is checked under today's disclosure policy as well.
	for _, meaning := range []memory.ReviewEditMeaning{out.Before, out.After} {
		candidate := item
		candidate.Candidate.Support = meaning.Support
		candidate.Candidate.Context = meaning.Context
		op := memory.OwnerReviewOperation{Preview: memory.ReviewPreview{Candidates: []memory.OwnerCandidate{candidate}}}
		if err = reviewOperationSourcesVisible(ctx, tx, op); err != nil {
			return memory.ReviewEditRevision{}, err
		}
	}
	return out, tx.Commit()
}

func validateReviewEditEnvelope(item memory.OwnerCandidate) error {
	if item.Edit == nil {
		if item.Original != nil {
			return errors.New("unbound original extraction")
		}
		return nil
	}
	r := item.Edit
	if item.Original == nil || r.CandidateID != item.Ref.ID || r.Revision < 1 || r.ParentRevision != r.Revision-1 || r.ReviewRevision > item.Ref.ReviewRevision || r.Revision > item.Ref.InterpretationRevision || r.OwnerID != memory.LocalOwnerID || r.AuthorizationRevision < 1 || validateSemanticUUID(r.AuditID) != nil || len(r.AuthenticationBinding) != 64 || !reviewReasonValid(r.Reason) || r.After.Identity != nil || r.After.Temporal != nil {
		return errors.New("invalid recorded owner edit")
	}
	current := editMeaning(item)
	current.Identity = nil
	current.Temporal = nil
	if string(compilerJSON(current)) != string(compilerJSON(r.After)) || item.Original.ID != item.Ref.ID {
		return errors.New("edit meaning differs from accepted candidate")
	}
	return nil
}

func reviewCandidateDisclosureSources(item memory.OwnerCandidate) [][]memory.CompilerSource {
	out := [][]memory.CompilerSource{item.Candidate.Support, item.Candidate.Context}
	if item.Original != nil {
		out = append(out, item.Original.Support, item.Original.Context)
	}
	if item.Edit != nil {
		out = append(out, item.Edit.Before.Support, item.Edit.Before.Context, item.Edit.After.Support, item.Edit.After.Context)
	}
	return out
}

// Owner interpretation uses the supported typed vocabulary, independently of
// the old extractor generation. The original window/evidence policy and offered
// semantic IDs stay fixed: this cannot import a source or upgrade its authority.
func validateOwnerEditedProposal(request memory.CompilerRequest, p memory.ExtractorCandidate) ([]memory.MemoryCandidate, error) {
	request.IdentityPolicy = memory.CompilerIdentityPolicyV2
	if p.Temporal != nil {
		request.IdentityPolicy = memory.CompilerTemporalPolicyV3
	}
	return validateCompilerOutput(request, compilerJSON(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{p}}))
}
