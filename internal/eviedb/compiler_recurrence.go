package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerRecurrenceRecord struct {
	Exact, Related               []byte
	Primary, Relationship, State string
	Suppressed                   bool
	Ref                          memory.CandidateRef
	Epoch                        int64
}

func compilerRecurrenceRef(ctx context.Context, q reviewQuery, id string) (memory.CandidateRef, string, error) {
	ref := memory.CandidateRef{ID: id}
	var state string
	err := q.QueryRowContext(ctx, `SELECT review_revision,review_state,MAX(COALESCE((SELECT revision FROM memory_review_edit_revisions WHERE candidate_id=c.candidate_id ORDER BY revision DESC LIMIT 1),0),COALESCE((SELECT revision FROM memory_review_identity_revisions WHERE candidate_id=c.candidate_id ORDER BY revision DESC LIMIT 1),0),COALESCE((SELECT revision FROM memory_review_temporal_revisions WHERE candidate_id=c.candidate_id ORDER BY revision DESC LIMIT 1),0)) FROM memory_compiler_candidates c WHERE candidate_id=?`, id).Scan(&ref.ReviewRevision, &state, &ref.InterpretationRevision)
	return ref, state, err
}

func classifyCompilerRecurrence(ctx context.Context, conn *sql.Conn, g memory.CompilerGeneration, r memory.CompilerRequest, c memory.MemoryCandidate) (compilerRecurrenceRecord, error) {
	exact, related, err := compilerRecurrenceCanonical(g, r, c)
	out := compilerRecurrenceRecord{Exact: exact, Related: related, Relationship: "primary", State: "unresolved"}
	if err != nil {
		return out, err
	}
	var found, stored string
	err = conn.QueryRowContext(ctx, `SELECT candidate_id,exact_encoding,presentation_epoch FROM memory_compiler_recurrence WHERE exact_hash=? AND suppressed=0 ORDER BY presentation_epoch DESC,publication_order LIMIT 1`, memory.CompilerHash(exact)).Scan(&found, &stored, &out.Epoch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err == nil && stored != string(exact) {
		found = ""
	} // Do not turn a hash collision into equality.
	if found == "" {
		// The bounded legacy fallback never scans history or guesses from names.
		// The side migration improves recall incrementally without startup sweeps.
		var raw, requestRaw, generationRaw []byte
		err = conn.QueryRowContext(ctx, `SELECT c.candidate_id,c.envelope,j.request,g.manifest FROM memory_compiler_candidates c JOIN memory_compiler_jobs j ON j.job_id=c.job_id JOIN memory_compiler_generations g ON g.generation_id=j.generation_id WHERE c.equivalence_hash=? AND c.equivalent_to IS NULL ORDER BY c.candidate_id LIMIT 1`, memory.CompilerHash(compilerEquivalenceEncoding(r.Window.Selection, c))).Scan(&found, &raw, &requestRaw, &generationRaw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if err == nil {
			var prior memory.MemoryCandidate
			var pr memory.CompilerRequest
			var pg memory.CompilerGeneration
			if json.Unmarshal(raw, &prior) != nil || json.Unmarshal(requestRaw, &pr) != nil || json.Unmarshal(generationRaw, &pg) != nil {
				return out, ErrReviewInvalidSource
			}
			value, _, e := compilerRecurrenceCanonical(pg, pr, prior)
			if e != nil {
				return out, e
			}
			if string(value) != string(exact) {
				found = ""
			}
		}
	}
	if found != "" {
		out.Primary = found
		out.Ref, out.State, err = compilerRecurrenceRef(ctx, conn, found)
		if err != nil {
			return out, err
		}
		out.Relationship = "exact_original"
		out.Suppressed = true
		if out.State == "accepted" {
			current, e := compilerAcceptedRecurrenceCurrent(ctx, conn, found)
			if e != nil {
				return out, e
			}
			if !current {
				out.Relationship = "current_effect_changed"
				out.Epoch++
				out.Suppressed = false
			}
		}
		return out, nil
	}
	err = conn.QueryRowContext(ctx, `SELECT candidate_id,related_encoding FROM memory_compiler_recurrence WHERE related_hash=? AND suppressed=0 ORDER BY publication_order DESC LIMIT 1`, memory.CompilerHash(related)).Scan(&found, &stored)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err == nil && stored == string(related) {
		out.Primary = found
		out.Relationship = "different_support"
		out.Ref, out.State, err = compilerRecurrenceRef(ctx, conn, found)
		return out, err
	}
	return out, nil
}

func insertCompilerRecurrence(ctx context.Context, conn *sql.Conn, id string, r compilerRecurrenceRecord) error {
	_, err := conn.ExecContext(ctx, `INSERT INTO memory_compiler_recurrence(publication_order,candidate_id,encoding_version,exact_hash,related_hash,exact_encoding,related_encoding,primary_id,relationship,suppressed,checked_interpretation,checked_review,checked_state,presentation_epoch) VALUES((SELECT rowid FROM memory_compiler_candidates WHERE candidate_id=?),?,'compiler-recurrence-v2',?,?,?,?,NULLIF(?,''),?,?,?,?,?,?)`, id, id, memory.CompilerHash(r.Exact), memory.CompilerHash(r.Related), r.Exact, r.Related, r.Primary, r.Relationship, r.Suppressed, r.Ref.InterpretationRevision, r.Ref.ReviewRevision, r.State, r.Epoch)
	return err
}

type compilerReviewAudit struct {
	Preview  memory.ReviewPreview  `json:"preview"`
	Decision memory.ReviewDecision `json:"decision"`
	Result   memory.ReviewResult   `json:"result"`
}

func compilerRecurrenceAudit(ctx context.Context, q reviewQuery, id string) (compilerReviewAudit, error) {
	var out compilerReviewAudit
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT a.envelope FROM memory_review_resolutions r JOIN memory_review_audits a ON a.audit_id=r.audit_id WHERE r.candidate_id=?`, id).Scan(&raw)
	if err != nil {
		return out, err
	}
	if len(raw) > 2*1024*1024 || json.Unmarshal(raw, &out) != nil {
		return out, ErrReviewInvalidSource
	}
	return out, nil
}

// Check the actual accepted interpretation (including an owner edit and every
// member of its dependent group), not the unaccepted original model proposal.
// All reads are exact primary-key probes over the bounded frozen effect.
func compilerAcceptedRecurrenceCurrent(ctx context.Context, q reviewQuery, id string) (bool, error) {
	audit, err := compilerRecurrenceAudit(ctx, q, id)
	if err != nil {
		return false, err
	}
	effect := audit.Preview.Effect
	if audit.Result.Operation == nil || effect == nil || len(effect.Claims) > 64 {
		return false, ErrReviewInvalidSource
	}
	checkState := func(kind memory.SemanticObjectKind, id memory.SemanticID, want memory.SemanticStateValue) (bool, error) {
		state, e := loadLatestState(ctx, inspectionLifecycleQueryer{q}, kind, id)
		if errors.Is(e, sql.ErrNoRows) {
			return false, nil
		}
		return state.State == want, e
	}
	for _, item := range effect.Claims {
		claim, e := loadSemanticClaim(ctx, q, item.Claim.ID)
		if errors.Is(e, sql.ErrNoRows) {
			return false, nil
		}
		if e != nil {
			return false, e
		}
		expected := item.Claim
		expected.TransactionTime = claim.TransactionTime
		// Create is a preview flag, not a persisted Predicate definition.
		expected.Predicate.Create = false
		claim.Predicate.Create = false
		if string(compilerJSON(expected)) != string(compilerJSON(claim)) {
			return false, nil
		}
		active, e := checkState(memory.SemanticObjectClaim, claim.ID, memory.SemanticStateActive)
		if e != nil || !active {
			return false, e
		}
		for _, entity := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
			if entity == nil {
				continue
			}
			current, e := loadSemanticEntityForInspection(ctx, q, entity.ID)
			if errors.Is(e, sql.ErrNoRows) {
				return false, nil
			}
			if e != nil {
				return false, e
			}
			expected := *entity
			expected.Create = false
			current.Create = false
			if string(compilerJSON(expected)) != string(compilerJSON(current)) {
				return false, nil
			}
			var lifecycle string
			if e = q.QueryRowContext(ctx, `SELECT lifecycle FROM semantic_entities WHERE entity_id=?`, entity.ID).Scan(&lifecycle); e != nil {
				return false, e
			}
			if lifecycle != "active" {
				return false, nil
			}
			state, e := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectEntity, entity.ID)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return false, e
			}
			if e == nil && state.State != memory.SemanticStateActive {
				return false, nil
			}
		}
		for _, source := range item.Sources {
			active, e := checkState(memory.SemanticObjectSourceLink, source.ID, memory.SemanticStateEligible)
			if e != nil || !active {
				return false, e
			}
		}
	}
	members := effect.Members
	if len(members) == 0 {
		members = []memory.ReviewEffect{*effect}
	}
	for _, member := range members {
		if member.Correction != nil {
			old, e := loadSemanticClaim(ctx, q, member.Correction.OldClaim.ID)
			if e != nil {
				return false, e
			}
			// Claim rows retain their original interval. The correction ledger
			// projects OldAfter; compare that exact accepted projection instead
			// of treating immutable OldBefore as the current effective interval.
			if !validTimesEqual(old.ValidTime, member.Correction.ValidTimeEffect.OldBefore) {
				return false, nil
			}
			current, e := compilerRecurrenceCorrectionCurrent(ctx, q, member)
			if e != nil || !current {
				return false, e
			}
			active, e := checkState(memory.SemanticObjectClaim, old.ID, member.Correction.Transition.State)
			if e != nil || !active {
				return false, e
			}
		}
		if member.Identity != nil {
			for _, alias := range member.Identity.Aliases {
				active, e := checkState(memory.SemanticObjectAlias, alias.ID, memory.SemanticStateActive)
				if e != nil || !active {
					return false, e
				}
			}
		}
	}
	return true, nil
}

// old_claim_id is unique and (operation_id, old_claim_id) is the primary key.
// This exact probe also distinguishes corrections sharing one compound operation.
func compilerRecurrenceCorrectionCurrent(ctx context.Context, q reviewQuery, effect memory.ReviewEffect) (bool, error) {
	correction := effect.Correction
	if correction == nil || len(effect.Claims) != 1 {
		return false, ErrReviewInvalidSource
	}
	valid := correction.ValidTimeEffect
	var matches int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_claim_corrections
 WHERE operation_id=? AND old_claim_id=? AND scope_id=? AND replacement_claim_id=? AND mode=?
 AND effective_time IS ? AND old_valid_from IS ? AND old_valid_to IS ?
 AND old_effective_from IS ? AND old_effective_to IS ? AND replacement_from IS ? AND replacement_to IS ?`,
		effect.OperationID, correction.OldClaim.ID, effect.Scope.ID, effect.Claims[0].Claim.ID, correction.Mode,
		semanticTimeArgument(correction.EffectiveTime), semanticTimeArgument(valid.OldBefore.From), semanticTimeArgument(valid.OldBefore.To),
		semanticTimeArgument(valid.OldAfter.From), semanticTimeArgument(valid.OldAfter.To), semanticTimeArgument(valid.Replacement.From), semanticTimeArgument(valid.Replacement.To)).Scan(&matches)
	return matches == 1, err
}
