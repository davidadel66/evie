package eviedb

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func prepareReviewTemporalEffect(ctx context.Context, q reviewQuery, a OwnerReviewContext, candidate memory.OwnerCandidate, effect *memory.ReviewEffect, item memory.ReviewClaimEffect) error {
	proposal := candidate.Candidate.Proposal
	if proposal.Temporal == nil {
		return nil
	}
	if err := validateTemporalMeaning(proposal, item.Predicate); err != nil {
		return err
	}
	effect.Version = "owner-review-effect-v3"
	if proposal.Temporal.Correction == nil {
		return nil
	}
	revision := candidate.Temporal
	if revision == nil {
		return errors.New("needs_choice: choose correction mode and exact prior Claim")
	}
	current, err := reviewTemporalOptions(ctx, q, a, candidate)
	if err != nil {
		return err
	}
	// Choice creation advances only candidate revisions, so compare the same
	// original option reference while rechecking every semantic value/revision.
	current.Candidate = revision.Options.Candidate
	current.SHA256 = temporalOptionsHash(current)
	if current.SHA256 != revision.Options.SHA256 {
		return ErrReviewStale
	}
	old, valid, err := validateTemporalChoice(proposal, revision.Options, revision.Choice)
	if err != nil {
		return err
	}
	if old.Claim.ID == item.Claim.ID {
		return errors.New("correction cannot supersede itself")
	}
	effective := (*time.Time)(nil)
	if revision.Choice.Mode == memory.CorrectionChanged {
		effective = proposal.Temporal.Correction.EffectiveTime
	}
	effect.Correction = &memory.ReviewCorrectionEffect{Revision: *revision, OldClaim: old.Claim, OldState: old.State, Mode: revision.Choice.Mode, EffectiveTime: effective, ValidTimeEffect: valid, Transition: memory.SemanticTransition{ObjectKind: "claim", ObjectID: old.Claim.ID, State: memory.SemanticStateSuperseded}}
	return nil
}

func validateReviewTemporalEncoding(p memory.ReviewPreview) error {
	candidate := p.Candidates[0]
	proposal := candidate.Candidate.Proposal
	if p.Version != "owner-review-preview-v3" && p.Version != "owner-review-preview-v4" {
		if proposal.Temporal != nil || candidate.Temporal != nil || p.Effect != nil && p.Effect.Correction != nil {
			return errors.New("older review version cannot carry temporal effects")
		}
		return nil
	}
	if proposal.Temporal == nil {
		if p.Version == "owner-review-preview-v4" && candidate.Temporal == nil && (p.Effect == nil || p.Effect.Correction == nil) {
			return nil
		}
		return errors.New("v3 preview requires typed temporal interpretation")
	}
	if p.Action == "reject" {
		return nil
	}
	if err := validateTemporalObserved(candidate.Candidate); err != nil {
		return err
	}
	item := p.Effect.Claims[0]
	if err := validateTemporalMeaning(proposal, item.Predicate); err != nil {
		return err
	}
	if proposal.Temporal.Correction == nil {
		if p.Effect.Correction != nil || candidate.Temporal != nil {
			return errors.New("unproposed correction effect")
		}
		return nil
	}
	effect := p.Effect.Correction
	if effect == nil || candidate.Temporal == nil {
		return errors.New("missing reviewed correction")
	}
	revision := effect.Revision
	if string(compilerJSON(candidate.Temporal)) != string(compilerJSON(revision)) || revision.Revision != candidate.Ref.InterpretationRevision || revision.ParentRevision != revision.Revision-1 || revision.ReviewRevision != candidate.Ref.ReviewRevision || revision.Options.Candidate.ID != candidate.Ref.ID || revision.Options.Candidate.InterpretationRevision != revision.ParentRevision || revision.Options.Candidate.ReviewRevision != revision.ReviewRevision-1 || revision.Options.ScopeKey != p.ScopeKey || revision.Options.SHA256 != temporalOptionsHash(revision.Options) || revision.OwnerID != memory.LocalOwnerID || revision.AuthorizationRevision < 1 || validateSemanticUUID(revision.AuditID) != nil || len(revision.AuthenticationBinding) != 64 {
		return errors.New("invalid temporal choice revision")
	}
	if string(compilerJSON(revision.Options.ScopeRevisions)) != string(compilerJSON(p.Effect.PriorRevisions)) || string(compilerJSON(revision.Options.Modes)) != string(compilerJSON(proposal.Temporal.Correction.Modes)) || string(compilerJSON(revision.Options.EffectiveTime)) != string(compilerJSON(proposal.Temporal.Correction.EffectiveTime)) {
		return errors.New("temporal options differ from original proposal")
	}
	binding, bindingErr := hex.DecodeString(revision.AuthenticationBinding)
	if bindingErr != nil || len(binding) != 32 || strings.ToLower(revision.AuthenticationBinding) != revision.AuthenticationBinding {
		return errors.New("invalid temporal authorization binding")
	}
	if len(revision.Options.Alternatives) > 32 {
		return errors.New("correction alternatives exceed bound")
	}
	for n, alternative := range revision.Options.Alternatives {
		prior := alternative.Claim
		if n > 0 && revision.Options.Alternatives[n-1].Claim.ID >= prior.ID || prior.ScopeKey != p.ScopeKey || prior.SubjectEntityID != item.Claim.SubjectEntityID || prior.Predicate != item.Predicate || alternative.State.State != memory.SemanticStateActive || alternative.State.ScopeRevision < 1 || alternative.State.ScopeRevision > p.Effect.Scope.Revision {
			return errors.New("invalid scoped correction alternative")
		}
		for _, id := range []memory.SemanticID{prior.ID, prior.CreatedOperationID, alternative.State.OperationID} {
			if validateSemanticUUID(string(id)) != nil {
				return errors.New("invalid prior Claim identity")
			}
		}
		if err := validateClaimObject(prior.Object); err != nil {
			return err
		}
		valid, err := normalizeValidTime(prior.ValidTime)
		if err != nil || !validTimesEqual(valid, prior.ValidTime) {
			return errors.New("noncanonical prior Claim time")
		}
		if prior.Polarity != memory.PolarityAffirmed && prior.Polarity != memory.PolarityDenied {
			return errors.New("invalid prior Claim polarity")
		}
	}
	old, valid, err := validateTemporalChoice(proposal, revision.Options, revision.Choice)
	if err != nil {
		return err
	}
	expected := memory.ReviewCorrectionEffect{Revision: revision, OldClaim: old.Claim, OldState: old.State, Mode: revision.Choice.Mode, ValidTimeEffect: valid, Transition: memory.SemanticTransition{ObjectKind: "claim", ObjectID: old.Claim.ID, State: memory.SemanticStateSuperseded}}
	if expected.Mode == memory.CorrectionChanged {
		expected.EffectiveTime = proposal.Temporal.Correction.EffectiveTime
	}
	if string(compilerJSON(effect)) != string(compilerJSON(expected)) || old.Claim.ScopeKey != p.ScopeKey || old.Claim.ID == item.Claim.ID || old.State.State != memory.SemanticStateActive || old.Claim.Predicate != item.Predicate {
		return errors.New("correction differs from exact reviewed lifecycle effect")
	}
	return nil
}

func validateReviewCorrectionCurrent(ctx context.Context, q reviewQuery, effect *memory.ReviewEffect) error {
	correction := effect.Correction
	if correction == nil {
		return nil
	}
	old, err := loadSemanticClaim(ctx, q, correction.OldClaim.ID)
	if err != nil {
		return err
	}
	state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, old.ID)
	if err != nil {
		return err
	}
	if string(compilerJSON(old)) != string(compilerJSON(correction.OldClaim)) || state != correction.OldState {
		return ErrReviewStale
	}
	var count int
	if err = q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_claim_corrections WHERE old_claim_id=?`, old.ID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrReviewStale
	}
	return nil
}

func applyReviewCorrectionEffect(ctx context.Context, conn *sql.Conn, effect *memory.ReviewEffect, at time.Time) error {
	correction := effect.Correction
	if correction == nil {
		return nil
	}
	valid := correction.ValidTimeEffect
	_, err := conn.ExecContext(ctx, `INSERT INTO semantic_claim_corrections(operation_id,scope_id,old_claim_id,replacement_claim_id,mode,effective_time,old_valid_from,old_valid_to,old_effective_from,old_effective_to,replacement_from,replacement_to,scope_revision,transaction_time) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, effect.OperationID, effect.Scope.ID, correction.OldClaim.ID, effect.Claims[0].Claim.ID, correction.Mode, semanticTimeArgument(correction.EffectiveTime), semanticTimeArgument(valid.OldBefore.From), semanticTimeArgument(valid.OldBefore.To), semanticTimeArgument(valid.OldAfter.From), semanticTimeArgument(valid.OldAfter.To), semanticTimeArgument(valid.Replacement.From), semanticTimeArgument(valid.Replacement.To), effect.Scope.Revision+1, formatSemanticTime(at))
	if err != nil {
		return err
	}
	return reviewInitialState(ctx, conn, effect.Scope, correction.OldClaim.ID, "claim", "superseded", effect.OperationID, at)
}

// Stage 3 stores fixed nanosecond UTC source instants; compiler events retain
// their original timestamp spelling. V3 seals canonical Source Link time while
// retaining the original event spelling in the candidate evidence manifest.
func reviewObservedTimeMatches(version, accepted, original string) bool {
	if version != "owner-review-preview-v3" && version != "owner-review-preview-v4" {
		return accepted == original
	}
	at, err := time.Parse(time.RFC3339Nano, original)
	return err == nil && formatSemanticTime(at) == accepted
}
