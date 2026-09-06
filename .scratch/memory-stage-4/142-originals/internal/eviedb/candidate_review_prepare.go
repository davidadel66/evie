package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

func (s *Store) PrepareOwnerCandidateReview(ctx context.Context, a OwnerReviewContext, ref memory.CandidateRef, action string) (memory.ReviewPreview, error) {
	var p memory.ReviewPreview
	if action != "accept" && action != "reject" {
		return p, errors.New("explicit review action required")
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if err := checkReviewAuthority(ctx, conn, a); err != nil {
			return err
		}
		candidate, err := loadReviewCandidate(ctx, conn, a, ref.ID, action == "accept")
		if err != nil {
			return err
		}
		if candidate.Candidate.ReviewState != "unresolved" {
			return ErrReviewResolved
		}
		if candidate.Ref != ref {
			return ErrReviewStale
		}
		if candidate.Candidate.EquivalentTo != "" {
			return errors.New("equivalent candidate has no independent review")
		}
		candidates := []memory.OwnerCandidate{candidate}
		id, err := newSemanticID()
		if err != nil {
			return err
		}
		p = memory.ReviewPreview{Version: "owner-review-preview-v1", ID: string(id), OwnerID: memory.LocalOwnerID, AuthenticationBinding: a.binding, AuthorizationRevision: a.revision, ScopeKey: a.scope, JobID: candidate.JobID, GenerationID: candidates[0].GenerationID, Action: action, Candidates: candidates}
		if candidate.Candidate.Proposal.Identity != nil {
			p.Version = "owner-review-preview-v2"
		}
		if err = conn.QueryRowContext(ctx, `SELECT source_policy FROM memory_review_authorization WHERE singleton=1`).Scan(&p.SourcePolicy); err != nil {
			return err
		}
		if action == "accept" {
			p.Effect, err = prepareReviewEffects(ctx, conn, a, candidates)
			if err != nil {
				return err
			}
		}
		p.EffectSHA256, _, err = ownerReviewEffectHash(p.Effect)
		if err != nil {
			return err
		}
		p.SHA256, _, err = ownerReviewPreviewHash(p)
		if err != nil {
			return err
		}
		if err = validateOwnerReviewEncoding(p); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `INSERT INTO memory_review_previews VALUES(?,?,?,?)`, p.ID, p.ScopeKey, p.SHA256, compilerJSON(p))
		return err
	})
	if err != nil {
		return memory.ReviewPreview{}, err
	}
	return p, nil
}

func prepareReviewEffects(ctx context.Context, q reviewQuery, a OwnerReviewContext, candidates []memory.OwnerCandidate) (*memory.ReviewEffect, error) {
	id, err := newSemanticID()
	if err != nil {
		return nil, err
	}
	effect := &memory.ReviewEffect{Version: "owner-review-effect-v1", OperationID: id, Scopes: []memory.SemanticScope{}, PriorRevisions: []memory.ScopeRevision{}, Claims: []memory.ReviewClaimEffect{}}
	keys, err := reviewScopeKeys(ctx, q, a.scope)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		scope, err := loadSemanticScope(ctx, q, key)
		if err != nil {
			return nil, err
		}
		effect.Scopes = append(effect.Scopes, scope.SemanticScope)
		effect.PriorRevisions = append(effect.PriorRevisions, memory.ScopeRevision{ScopeKey: key, Revision: scope.Revision})
		if key == a.scope {
			effect.Scope = scope.SemanticScope
		}
	}
	claimKeys := map[string]bool{}
	for _, candidate := range candidates {
		c := candidate.Candidate
		if len(c.Support) == 0 {
			return nil, ErrReviewInvalidSource
		}
		time, err := normalizeValidTime(c.Proposal.ValidTime)
		if err != nil {
			return nil, err
		}
		if !validTimesEqual(time, c.Proposal.ValidTime) {
			return nil, errors.New("noncanonical candidate time")
		}
		prop := c.Proposal.Proposition
		if prop.Polarity != memory.PolarityAffirmed && prop.Polarity != memory.PolarityDenied {
			return nil, errors.New("invalid candidate polarity")
		}
		item := memory.ReviewClaimEffect{Candidate: candidate.Ref, Context: c.Context, TemporalQualification: c.Proposal.TemporalQualification, Sources: []memory.SemanticSource{}, Conflicts: []memory.ClaimConflictWarning{}}
		prop, err = prepareReviewIdentityEffects(ctx, q, a, candidate, effect, &item)
		if err != nil {
			return nil, err
		}
		if err = validateClaimObject(prop.Object); err != nil {
			return nil, err
		}
		if item.Subject.ID == "" {
			item.Subject, err = reviewEntity(ctx, q, keys, prop.SubjectEntityID)
			if err != nil {
				return nil, err
			}
		}
		if item.Predicate.ID == "" {
			if err = q.QueryRowContext(ctx, `SELECT predicate_id,token,version,label,object_constraint,cardinality FROM semantic_predicates WHERE predicate_id=?`, prop.PredicateID).Scan(&item.Predicate.ID, &item.Predicate.Token, &item.Predicate.Version, &item.Predicate.Label, &item.Predicate.ObjectConstraint, &item.Predicate.Cardinality); err != nil {
				return nil, errors.New("needs_choice: Predicate")
			}
		}
		if prop.Object.EntityID != "" {
			if item.ObjectEntity == nil {
				entity, err := reviewEntity(ctx, q, keys, prop.Object.EntityID)
				if err != nil {
					return nil, err
				}
				item.ObjectEntity = &entity
			}
			if item.Predicate.ObjectConstraint != memory.ConstraintEntity {
				return nil, errors.New("Predicate object constraint changed")
			}
		} else if string(item.Predicate.ObjectConstraint) != string(prop.Object.Literal.Kind) {
			return nil, errors.New("Predicate object constraint changed")
		}
		item.Claim = memory.SemanticClaim{ScopeKey: a.scope, SubjectEntityID: prop.SubjectEntityID, Predicate: item.Predicate, Object: prop.Object, Polarity: prop.Polarity, ValidTime: time, CreatedOperationID: id}
		objectKind, objectEntity, literalKind, literalValue := "literal", any(nil), any(nil), any(nil)
		if prop.Object.EntityID != "" {
			objectKind = "entity"
			objectEntity = prop.Object.EntityID
		} else {
			literalKind = prop.Object.Literal.Kind
			literalValue = prop.Object.Literal.Value
		}
		var claimID memory.SemanticID
		err = q.QueryRowContext(ctx, `SELECT claim_id FROM semantic_claims WHERE scope_id=? AND subject_entity_id=? AND predicate_id=? AND object_kind=? AND object_entity_id IS ? AND literal_kind IS ? AND literal_value IS ? AND polarity=? AND valid_from IS ? AND valid_to IS ?`, effect.Scope.ID, prop.SubjectEntityID, prop.PredicateID, objectKind, objectEntity, literalKind, literalValue, prop.Polarity, semanticTimeArgument(time.From), semanticTimeArgument(time.To)).Scan(&claimID)
		if errors.Is(err, sql.ErrNoRows) {
			item.Claim.ID, err = newSemanticID()
			item.Create = true
		} else if err == nil {
			item.Claim, err = loadSemanticClaim(ctx, q, claimID)
		}
		if err != nil {
			return nil, err
		}
		if !item.Create {
			state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, item.Claim.ID)
			if err != nil || state.State != memory.SemanticStateActive {
				return nil, errors.New("needs_choice: equal Claim is inactive")
			}
		}
		meaning := string(compilerJSON(struct {
			P memory.ClaimProposition
			T memory.ValidTime
		}{prop, time}))
		if claimKeys[meaning] {
			return nil, errors.New("duplicate effects require a combined interpretation")
		}
		claimKeys[meaning] = true
		sources := append([]memory.CompilerSource(nil), c.Support...)
		sort.Slice(sources, func(i, j int) bool {
			return string(compilerJSON(sources[i].Locator)) < string(compilerJSON(sources[j].Locator))
		})
		for _, source := range sources {
			if source.Actor != memory.SemanticActorOwner || source.Authority != memory.AuthorityOwnerStatement {
				return nil, ErrReviewInvalidSource
			}
			src := memory.SemanticSource{EventID: source.Locator.EventID, SessionID: source.SessionID, ScopeKey: source.ScopeKey, EventPart: source.Locator.EventPart, LocatorKind: source.Locator.LocatorKind, LocatorValue: source.Locator.LocatorValue, EvidenceSHA256: "sha256:" + source.Locator.EvidenceSHA256, Actor: source.Actor, SourceType: memory.SourceTypeUserMessage, Authority: source.Authority, ObservedAt: source.ObservedAt, Evidence: source.Evidence, Eligibility: memory.EligibilityEligible, OperationID: id}
			err = q.QueryRowContext(ctx, `SELECT source_link_id,created_operation_id FROM semantic_source_links WHERE claim_id=? AND event_id=? AND event_part=? AND locator_kind=? AND locator_value=? AND evidence_sha256=?`, item.Claim.ID, src.EventID, src.EventPart, src.LocatorKind, src.LocatorValue, src.EvidenceSHA256).Scan(&src.ID, &src.OperationID)
			if errors.Is(err, sql.ErrNoRows) {
				src.ID, err = newSemanticID()
				src.Create = true
			} else if err == nil {
				state, stateErr := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectSourceLink, src.ID)
				if stateErr != nil || state.State != memory.SemanticStateEligible {
					return nil, errors.New("retracted Source Link requires explicit restoration")
				}
			}
			if err != nil {
				return nil, err
			}
			item.Sources = append(item.Sources, src)
		}
		conflicts, err := reviewClaimConflicts(ctx, q, effect.Scope.ID, item.Claim)
		if err != nil {
			return nil, err
		}
		item.Conflicts = conflicts
		effect.Claims = append(effect.Claims, item)
	}
	effects := 0
	for _, item := range effect.Claims {
		effects += 1 + len(item.Sources)
	}
	if effects > 256 {
		return nil, errors.New("review_too_large")
	}
	return effect, nil
}

var errReviewInactiveEntity = errors.New("inactive Entity")

func reviewEntity(ctx context.Context, q reviewQuery, keys []string, id memory.SemanticID) (memory.SemanticEntity, error) {
	entity, err := loadSemanticEntityForInspection(ctx, q, id)
	if err != nil {
		return entity, errors.New("needs_choice: Entity")
	}
	allowed := false
	for _, key := range keys {
		if entity.ScopeKey == key {
			allowed = true
		}
	}
	if !allowed {
		return entity, ErrOwnerReviewUnauthorized
	}
	var lifecycle string
	if err = q.QueryRowContext(ctx, `SELECT lifecycle FROM semantic_entities WHERE entity_id=?`, id).Scan(&lifecycle); err != nil || lifecycle != "active" {
		return entity, errReviewInactiveEntity
	}
	state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectEntity, id)
	if err == nil && state.State != memory.SemanticStateActive {
		return entity, errReviewInactiveEntity
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return entity, err
	}
	return entity, nil
}

func reviewClaimConflicts(ctx context.Context, q reviewQuery, scope memory.SemanticID, claim memory.SemanticClaim) ([]memory.ClaimConflictWarning, error) {
	rows, err := q.QueryContext(ctx, `SELECT claim_id FROM semantic_claims WHERE scope_id=? AND subject_entity_id=? AND predicate_id=? AND claim_id<>? ORDER BY claim_id LIMIT 129`, scope, claim.SubjectEntityID, claim.Predicate.ID, claim.ID)
	if err != nil {
		return nil, err
	}
	ids := []memory.SemanticID{}
	for rows.Next() {
		var id memory.SemanticID
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) > 128 {
		return nil, errors.New("review conflict bound")
	}
	convert := func(c memory.SemanticClaim) claimConflictCandidate {
		object := string(compilerJSON(c.Object))
		return claimConflictCandidate{ID: c.ID, SubjectID: c.SubjectEntityID, PredicateID: c.Predicate.ID, PredicateToken: c.Predicate.Token, ObjectKey: object, Polarity: c.Polarity, ValidTime: c.ValidTime, Cardinality: c.Predicate.Cardinality}
	}
	candidates := []claimConflictCandidate{convert(claim)}
	for _, id := range ids {
		c, err := loadSemanticClaim(ctx, q, id)
		if err != nil {
			return nil, err
		}
		state, err := loadLatestState(ctx, inspectionLifecycleQueryer{q}, memory.SemanticObjectClaim, id)
		if err != nil {
			return nil, err
		}
		if state.State == memory.SemanticStateActive {
			candidates = append(candidates, convert(c))
		}
	}
	result := []memory.ClaimConflictWarning{}
	for _, warning := range classifyClaimConflicts(candidates) {
		if containsSemanticID(warning.ClaimIDs, claim.ID) {
			result = append(result, warning)
		}
	}
	return result, nil
}

func reviewDeliveryValid(key string) bool {
	return strings.HasPrefix(key, "idem:v1:") && validateSemanticUUID(strings.TrimPrefix(key, "idem:v1:")) == nil
}
