package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

const reviewTemporalSchema = `
CREATE TABLE IF NOT EXISTS memory_review_temporal_revisions (
 candidate_id TEXT NOT NULL REFERENCES memory_compiler_candidates(candidate_id),
 revision INTEGER NOT NULL CHECK(revision>0), envelope BLOB NOT NULL CHECK(length(envelope)<=131072),
 PRIMARY KEY(candidate_id,revision)
);
CREATE TRIGGER IF NOT EXISTS memory_review_temporal_no_update BEFORE UPDATE ON memory_review_temporal_revisions BEGIN SELECT RAISE(ABORT,'temporal revisions are immutable'); END;
CREATE TRIGGER IF NOT EXISTS memory_review_temporal_no_delete BEFORE DELETE ON memory_review_temporal_revisions BEGIN SELECT RAISE(ABORT,'temporal revisions are immutable'); END;
`

func loadReviewTemporalRevision(ctx context.Context, q reviewQuery, item *memory.OwnerCandidate) error {
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT envelope FROM memory_review_temporal_revisions WHERE candidate_id=? ORDER BY revision DESC LIMIT 1`, item.Ref.ID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var revision memory.ReviewTemporalRevision
	if json.Unmarshal(raw, &revision) != nil || revision.Revision < 1 || revision.ReviewRevision > item.Ref.ReviewRevision || revision.Options.Candidate.ID != item.Ref.ID || revision.Revision > item.Ref.InterpretationRevision && item.Identity != nil {
		return errors.New("invalid stored temporal revision")
	}
	if revision.Revision <= item.Ref.InterpretationRevision {
		return nil
	}
	item.Ref.InterpretationRevision = revision.Revision
	if !item.Redacted {
		item.Temporal = &revision
	}
	return nil
}

func temporalOptionsHash(options memory.ReviewTemporalOptions) string {
	options.SHA256 = ""
	return memory.CompilerHash(compilerJSON(struct {
		Domain  string                       `json:"domain"`
		Options memory.ReviewTemporalOptions `json:"options"`
	}{"owner-temporal-options-v1", options}))
}

func reviewTemporalOptions(ctx context.Context, q reviewQuery, a OwnerReviewContext, item memory.OwnerCandidate) (memory.ReviewTemporalOptions, error) {
	out := memory.ReviewTemporalOptions{Candidate: item.Ref, ScopeKey: a.scope, ScopeRevisions: []memory.ScopeRevision{}, Alternatives: []memory.ReviewCorrectionAlternative{}}
	temporal := item.Candidate.Proposal.Temporal
	if temporal == nil || temporal.Correction == nil || item.Candidate.Proposal.Identity != nil {
		return out, errors.New("candidate has no existing-identity correction choice")
	}
	out.Modes = temporal.Correction.Modes
	out.EffectiveTime = temporal.Correction.EffectiveTime
	keys, err := reviewScopeKeys(ctx, q, a.scope)
	if err != nil {
		return out, err
	}
	for _, key := range keys {
		scope, err := loadSemanticScope(ctx, q, key)
		if err != nil {
			return out, err
		}
		out.ScopeRevisions = append(out.ScopeRevisions, memory.ScopeRevision{ScopeKey: key, Revision: scope.Revision})
	}
	prop := item.Candidate.Proposal.Proposition
	rows, err := q.QueryContext(ctx, `SELECT c.claim_id FROM semantic_claims c JOIN semantic_scopes s ON s.scope_id=c.scope_id WHERE s.scope_key=? AND c.subject_entity_id=? AND c.predicate_id=? ORDER BY c.claim_id LIMIT 33`, a.scope, prop.SubjectEntityID, prop.PredicateID)
	if err != nil {
		return out, err
	}
	ids := []memory.SemanticID{}
	for rows.Next() {
		var id memory.SemanticID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > 32 {
		return out, errors.New("correction alternatives exceed bound")
	}
	for _, id := range ids {
		var size int
		if err = q.QueryRowContext(ctx, `SELECT COALESCE(length(CAST(literal_value AS BLOB)),0) FROM semantic_claims WHERE claim_id=?`, id).Scan(&size); err != nil {
			return out, err
		}
		if size > 8192 {
			return out, errors.New("prior Claim text exceeds review bound")
		}
		claim, err := loadSemanticClaim(ctx, q, id)
		if err != nil {
			return out, err
		}
		state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, id)
		if err != nil {
			return out, err
		}
		if state.State != memory.SemanticStateActive {
			continue
		}
		if compilerHasSecret(string(compilerJSON(claim))) {
			return out, ErrReviewInvalidSource
		}
		var count int
		if err = q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_claim_corrections WHERE old_claim_id=?`, id).Scan(&count); err != nil {
			return out, err
		}
		if count == 0 {
			out.Alternatives = append(out.Alternatives, memory.ReviewCorrectionAlternative{Claim: claim, State: state})
		}
	}
	out.SHA256 = temporalOptionsHash(out)
	if len(compilerJSON(out)) > 128*1024 {
		return memory.ReviewTemporalOptions{}, errors.New("correction alternatives exceed bound")
	}
	return out, nil
}

func validateTemporalChoice(proposal memory.ExtractorCandidate, options memory.ReviewTemporalOptions, choice memory.ReviewTemporalChoice) (memory.ReviewCorrectionAlternative, memory.CorrectionValidTimeEffect, error) {
	var old memory.ReviewCorrectionAlternative
	if proposal.Temporal == nil || proposal.Temporal.Correction == nil {
		return old, memory.CorrectionValidTimeEffect{}, errors.New("no correction proposal")
	}
	found := false
	for _, alternative := range options.Alternatives {
		if alternative.Claim.ID == choice.OldClaimID {
			old = alternative
			found = true
		}
	}
	if !found {
		return old, memory.CorrectionValidTimeEffect{}, errors.New("needs_choice: select an exact eligible prior Claim")
	}
	modeFound := false
	for _, mode := range proposal.Temporal.Correction.Modes {
		if mode == choice.Mode {
			modeFound = true
		}
	}
	if !modeFound {
		return old, memory.CorrectionValidTimeEffect{}, errors.New("correction mode was not proposed")
	}
	request := memory.CorrectClaimRequest{OldClaimID: choice.OldClaimID, Replacement: proposal.Proposition, Mode: choice.Mode}
	if choice.Mode == memory.CorrectionChanged {
		request.EffectiveTime = proposal.Temporal.Correction.EffectiveTime
	} else {
		request.ReplacementValidTime = &proposal.ValidTime
	}
	normalized, err := normalizeCorrectClaimRequest(request)
	if err != nil {
		return old, memory.CorrectionValidTimeEffect{}, err
	}
	effect, err := correctionValidTimeEffect(old.Claim.ValidTime, normalized)
	if err != nil {
		return old, effect, err
	}
	if !validTimesEqual(effect.Replacement, proposal.ValidTime) {
		return old, effect, errors.New("needs_choice: proposed replacement interval differs from the correction's exact interval")
	}
	if old.Claim.SubjectEntityID != proposal.Proposition.SubjectEntityID || old.Claim.Predicate.ID != proposal.Proposition.PredicateID {
		return old, effect, errors.New("correction changes unrelated identity")
	}
	if string(compilerJSON(old.Claim.Object)) == string(compilerJSON(proposal.Proposition.Object)) && old.Claim.Polarity == proposal.Proposition.Polarity && validTimesEqual(old.Claim.ValidTime, effect.Replacement) {
		return old, effect, errors.New("duplicate proposition is additional support, not a correction")
	}
	return old, effect, nil
}

func (s *Store) OwnerCandidateTemporalOptions(ctx context.Context, a OwnerReviewContext, ref memory.CandidateRef) (memory.ReviewTemporalOptions, error) {
	var out memory.ReviewTemporalOptions
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	if err := requireReviewUnresolved(ctx, tx, a, ref.ID); err != nil {
		return out, err
	}
	item, err := loadReviewCandidate(ctx, tx, a, ref.ID, true)
	if err != nil {
		return out, err
	}
	if item.Ref != ref {
		return out, ErrReviewStale
	}
	if item.Candidate.ReviewState != "unresolved" {
		return out, ErrReviewResolved
	}
	out, err = reviewTemporalOptions(ctx, tx, a, item)
	if err != nil {
		return memory.ReviewTemporalOptions{}, err
	}
	return out, tx.Commit()
}

func (s *Store) ChooseOwnerCandidateTemporal(ctx context.Context, a OwnerReviewContext, decision memory.ReviewTemporalDecision) (memory.OwnerCandidate, error) {
	var out memory.OwnerCandidate
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		if err := requireReviewUnresolved(ctx, conn, a, decision.Candidate.ID); err != nil {
			return err
		}
		item, err := loadReviewCandidate(ctx, conn, a, decision.Candidate.ID, true)
		if err != nil {
			return err
		}
		if item.Candidate.ReviewState != "unresolved" {
			return ErrReviewResolved
		}
		if item.Ref != decision.Candidate {
			return ErrReviewStale
		}
		if item.Candidate.EquivalentTo != "" {
			return errors.New("equivalent candidate has no independent review")
		}
		options, err := reviewTemporalOptions(ctx, conn, a, item)
		if err != nil {
			return err
		}
		if options.SHA256 != decision.OptionsSHA256 {
			return ErrReviewStale
		}
		if _, _, err = validateTemporalChoice(item.Candidate.Proposal, options, decision.Choice); err != nil {
			return err
		}
		id, err := newSemanticID()
		if err != nil {
			return err
		}
		revision := memory.ReviewTemporalRevision{Revision: item.Ref.InterpretationRevision + 1, ParentRevision: item.Ref.InterpretationRevision, ReviewRevision: item.Ref.ReviewRevision + 1, AuditID: string(id), OwnerID: memory.LocalOwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, Options: options, Choice: decision.Choice}
		if _, err = conn.ExecContext(ctx, `INSERT INTO memory_review_temporal_revisions VALUES(?,?,?)`, item.Ref.ID, revision.Revision, compilerJSON(revision)); err != nil {
			return err
		}
		update, err := conn.ExecContext(ctx, `UPDATE memory_compiler_candidates SET review_revision=review_revision+1 WHERE candidate_id=? AND review_revision=? AND review_state='unresolved'`, item.Ref.ID, item.Ref.ReviewRevision)
		if err != nil {
			return err
		}
		n, err := update.RowsAffected()
		if err != nil || n != 1 {
			return ErrReviewStale
		}
		out, err = loadReviewCandidate(ctx, conn, a, item.Ref.ID, true)
		return err
	})
	if err != nil {
		return memory.OwnerCandidate{}, err
	}
	return out, nil
}

func (s *Store) InspectOwnerCandidateTemporalRevision(ctx context.Context, a OwnerReviewContext, id string, revision int64) (memory.ReviewTemporalRevision, error) {
	var out memory.ReviewTemporalRevision
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = checkReviewAuthority(ctx, tx, a); err != nil {
		return out, err
	}
	if _, err = loadReviewCandidate(ctx, tx, a, id, true); err != nil {
		return out, err
	}
	var raw []byte
	if err = tx.QueryRowContext(ctx, `SELECT envelope FROM memory_review_temporal_revisions WHERE candidate_id=? AND revision=?`, id, revision).Scan(&raw); err != nil {
		return out, err
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		return out, err
	}
	if compilerHasSecret(string(raw)) {
		return memory.ReviewTemporalRevision{}, ErrReviewInvalidSource
	}
	return out, tx.Commit()
}
