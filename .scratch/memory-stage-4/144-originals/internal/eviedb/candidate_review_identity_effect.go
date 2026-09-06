package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func prepareReviewIdentityEffects(ctx context.Context, q reviewQuery, a OwnerReviewContext, candidate memory.OwnerCandidate, effect *memory.ReviewEffect, item *memory.ReviewClaimEffect) (memory.ClaimProposition, error) {
	prop := candidate.Candidate.Proposal.Proposition
	identity := candidate.Candidate.Proposal.Identity
	if identity == nil {
		return prop, nil
	}
	revision := candidate.Identity
	if revision == nil {
		return prop, errors.New("needs_choice: inspect alternatives and record owner identity choices")
	}
	if err := validateReviewIdentityChoices(identity, revision.Options, revision.Choices); err != nil {
		return prop, err
	}
	if string(compilerJSON(revision.Options.ScopeRevisions)) != string(compilerJSON(effect.PriorRevisions)) {
		return prop, ErrReviewStale
	}
	effect.Version = "owner-review-effect-v2"
	effect.Identity = &memory.ReviewIdentityEffect{Revision: *revision, Aliases: []memory.SemanticAlias{}}
	for _, entry := range []struct {
		mention *memory.EntityMention
		choice  *memory.ReviewEntityChoice
		target  **memory.SemanticEntity
		subject bool
	}{{identity.Subject, revision.Choices.Subject, nil, true}, {identity.Object, revision.Choices.Object, &item.ObjectEntity, false}} {
		if entry.mention == nil {
			continue
		}
		var entity memory.SemanticEntity
		if entry.choice.Create {
			id, err := newSemanticID()
			if err != nil {
				return prop, err
			}
			entity = memory.SemanticEntity{ID: id, ScopeKey: a.scope, CanonicalName: entry.mention.Name, EntityType: entry.mention.EntityType, Create: true}
			if err := validateEntityIdentity(entity); err != nil {
				return prop, err
			}
			aliasID, err := newSemanticID()
			if err != nil {
				return prop, err
			}
			effect.Identity.Aliases = append(effect.Identity.Aliases, memory.SemanticAlias{ID: aliasID, EntityID: id, ScopeKey: a.scope, Value: entry.mention.Name, NormalizedValue: normalizeAlias(entry.mention.Name), OperationID: effect.OperationID, SourceEventID: entry.mention.Support.EventID, Create: true})
		} else {
			keys, err := reviewScopeKeys(ctx, q, a.scope)
			if err != nil {
				return prop, err
			}
			entity, err = reviewEntity(ctx, q, keys, entry.choice.EntityID)
			if err != nil {
				return prop, err
			}
		}
		if entry.subject {
			item.Subject = entity
			prop.SubjectEntityID = entity.ID
		} else {
			*entry.target = &entity
			prop.Object = memory.ClaimObject{EntityID: entity.ID}
		}
	}
	if identity.Predicate != nil {
		definition := identity.Predicate
		if revision.Choices.Predicate.Create {
			var count int
			if err := q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_predicates WHERE token=?`, definition.Token).Scan(&count); err != nil {
				return prop, err
			}
			if count != 0 {
				return prop, ErrReviewStale
			}
			id, err := newSemanticID()
			if err != nil {
				return prop, err
			}
			item.Predicate = memory.SemanticPredicate{ID: id, Token: definition.Token, Version: 1, Label: definition.Label, ObjectConstraint: definition.ObjectConstraint, Cardinality: definition.Cardinality, Create: true}
		} else {
			for _, p := range revision.Options.Predicates {
				if p.ID == revision.Choices.Predicate.PredicateID {
					item.Predicate = p
				}
			}
		}
		prop.PredicateID = item.Predicate.ID
	}
	return prop, nil
}

// Validate every new identity as an explicit dependent effect. Existing graph
// IDs stay subject to the same scoped read and revision checks as v1.
func validateReviewIdentityEffect(p memory.ReviewPreview) error {
	effect := p.Effect
	if p.Version == "owner-review-preview-v1" {
		for _, candidate := range p.Candidates {
			if candidate.Identity != nil || candidate.Candidate.Proposal.Identity != nil {
				return errors.New("v1 review cannot carry identity interpretation")
			}
		}
		if effect != nil && effect.Identity != nil {
			return errors.New("v1 review cannot carry identity effects")
		}
		return nil
	}
	if p.Version != "owner-review-preview-v2" && p.Version != "owner-review-preview-v3" && p.Version != "owner-review-preview-v4" {
		return errors.New("unsupported identity preview")
	}
	candidate := p.Candidates[0]
	if candidate.Candidate.Proposal.Identity == nil {
		if (p.Version == "owner-review-preview-v3" || p.Version == "owner-review-preview-v4") && candidate.Identity == nil && (effect == nil || effect.Identity == nil) {
			return nil
		}
		return errors.New("v2 preview requires identity proposal")
	}
	if p.Action == "reject" {
		return nil
	}
	if effect == nil || effect.Identity == nil || candidate.Identity == nil {
		return errors.New("missing identity interpretation")
	}
	revision := effect.Identity.Revision
	if string(compilerJSON(candidate.Identity)) != string(compilerJSON(revision)) || revision.Revision != candidate.Ref.InterpretationRevision || revision.ReviewRevision != candidate.Ref.ReviewRevision || revision.OwnerID != memory.LocalOwnerID || revision.ParentRevision != revision.Revision-1 || revision.Options.Candidate.ID != candidate.Ref.ID || revision.Options.Candidate.InterpretationRevision != revision.ParentRevision || revision.Options.Candidate.ReviewRevision != revision.ReviewRevision-1 || revision.Options.ScopeKey != p.ScopeKey || revision.Options.SHA256 != reviewIdentityOptionsHash(revision.Options) || revision.AuthorizationRevision < 1 || validateSemanticUUID(revision.AuditID) != nil {
		return errors.New("invalid owner identity revision")
	}
	if string(compilerJSON(revision.Options.ScopeRevisions)) != string(compilerJSON(effect.PriorRevisions)) {
		return errors.New("identity resolution revision differs from effect")
	}
	if err := validateCandidateIdentity(candidate.Candidate.Proposal); err != nil {
		return err
	}
	if err := validateCompilerIdentitySupport(candidate.Candidate); err != nil {
		return err
	}
	if err := validateReviewIdentityChoices(candidate.Candidate.Proposal.Identity, revision.Options, revision.Choices); err != nil {
		return err
	}
	item := effect.Claims[0]
	identity := candidate.Candidate.Proposal.Identity
	newEntities := map[memory.SemanticID]*memory.EntityMention{}
	for _, entry := range []struct {
		mention      *memory.EntityMention
		choice       *memory.ReviewEntityChoice
		entity       *memory.SemanticEntity
		alternatives []memory.ReviewEntityAlternative
	}{{identity.Subject, revision.Choices.Subject, &item.Subject, revision.Options.Subject}, {identity.Object, revision.Choices.Object, item.ObjectEntity, revision.Options.Object}} {
		if entry.mention == nil {
			if entry.entity != nil && entry.entity.Create {
				return errors.New("unproposed Entity creation")
			}
			continue
		}
		if entry.entity == nil || entry.entity.Create != entry.choice.Create {
			return errors.New("identity effect differs from owner choice")
		}
		if entry.choice.Create {
			e := entry.entity
			if e.ScopeKey != effect.Scope.Key || e.CanonicalName != entry.mention.Name || e.EntityType != entry.mention.EntityType || e.AnchorKind != "" || validateEntityIdentity(*e) != nil {
				return errors.New("invalid new sourced Entity")
			}
			if _, exists := newEntities[e.ID]; exists {
				return errors.New("distinct creations share an identity")
			}
			newEntities[e.ID] = entry.mention
		} else {
			matched := false
			for _, alternative := range entry.alternatives {
				if alternative.Entity == *entry.entity && entry.entity.ID == entry.choice.EntityID {
					matched = true
				}
			}
			if !matched {
				return errors.New("resolved Entity differs from reviewed alternative")
			}
		}
	}
	if len(effect.Identity.Aliases) != len(newEntities) {
		return errors.New("incomplete identity aliases")
	}
	seenAliases := map[memory.SemanticID]bool{}
	for _, alias := range effect.Identity.Aliases {
		mention, ok := newEntities[alias.EntityID]
		if !ok || seenAliases[alias.EntityID] || !alias.Create || alias.ScopeKey != effect.Scope.Key || alias.Value != mention.Name || alias.NormalizedValue != normalizeAlias(mention.Name) || alias.SourceEventID != mention.Support.EventID || alias.OperationID != effect.OperationID || validateSemanticUUID(string(alias.ID)) != nil {
			return errors.New("invalid sourced Alias effect")
		}
		seenAliases[alias.EntityID] = true
	}
	if identity.Predicate == nil {
		if item.Predicate.Create {
			return errors.New("unproposed Predicate creation")
		}
	} else {
		choice := revision.Choices.Predicate
		if item.Predicate.Create != choice.Create || !sameReviewPredicate(*identity.Predicate, item.Predicate) || choice.Create && item.Predicate.Version != 1 || !choice.Create && item.Predicate.ID != choice.PredicateID {
			return errors.New("Predicate effect differs from owner definition choice")
		}
	}
	return nil
}

func reviewResolvedProposition(p memory.ReviewPreview, index int) (memory.ClaimProposition, error) {
	proposal := p.Candidates[index].Candidate.Proposal.Proposition
	identity := p.Candidates[index].Candidate.Proposal.Identity
	if identity == nil {
		return proposal, nil
	}
	if err := validateReviewIdentityEffect(p); err != nil {
		return proposal, err
	}
	item := p.Effect.Claims[index]
	if identity.Subject != nil {
		proposal.SubjectEntityID = item.Subject.ID
	}
	if identity.Object != nil {
		proposal.Object = memory.ClaimObject{EntityID: item.ObjectEntity.ID}
	}
	if identity.Predicate != nil {
		proposal.PredicateID = item.Predicate.ID
	}
	return proposal, nil
}

func reviewWritesGlobal(effect *memory.ReviewEffect) bool {
	return effect.Identity != nil && effect.Claims[0].Predicate.Create
}

func validateNewReviewEntity(ctx context.Context, q reviewQuery, effect *memory.ReviewEffect, entity memory.SemanticEntity) error {
	if effect.Identity == nil || entity.ScopeKey != effect.Scope.Key || !entity.Create {
		return errors.New("invalid new review Entity")
	}
	var count int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_entities WHERE entity_id=?`, entity.ID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrReviewStale
	}
	return nil
}

func applyReviewIdentityEffects(ctx context.Context, conn *sql.Conn, effect *memory.ReviewEffect, byKey map[string]memory.SemanticScope, at time.Time) error {
	if effect.Identity == nil {
		return nil
	}
	item := effect.Claims[0]
	if item.Predicate.Create {
		p := item.Predicate
		if _, err := conn.ExecContext(ctx, `INSERT INTO semantic_predicates(predicate_id,scope_id,token,version,label,object_constraint,cardinality,created_operation_id) VALUES(?,?,?,?,?,?,?,?)`, p.ID, byKey["global"].ID, p.Token, p.Version, p.Label, p.ObjectConstraint, p.Cardinality, effect.OperationID); err != nil {
			return err
		}
	}
	for _, entity := range []*memory.SemanticEntity{&item.Subject, item.ObjectEntity} {
		if entity == nil || !entity.Create {
			continue
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO semantic_entities(entity_id,scope_id,canonical_name,entity_type,anchor_kind,lifecycle,created_operation_id) VALUES(?,?,?,?,NULL,'active',?)`, entity.ID, effect.Scope.ID, entity.CanonicalName, entity.EntityType, effect.OperationID); err != nil {
			return err
		}
		if err := reviewInitialState(ctx, conn, effect.Scope, entity.ID, "entity", "active", effect.OperationID, at); err != nil {
			return err
		}
	}
	for _, alias := range effect.Identity.Aliases {
		if _, err := conn.ExecContext(ctx, `INSERT INTO semantic_aliases(alias_id,entity_id,scope_id,value,normalized_value,lifecycle,source_event_id,created_operation_id) VALUES(?,?,?,?,?,'active',?,?)`, alias.ID, alias.EntityID, effect.Scope.ID, alias.Value, alias.NormalizedValue, alias.SourceEventID, effect.OperationID); err != nil {
			return err
		}
		if err := reviewInitialState(ctx, conn, effect.Scope, alias.ID, "alias", "active", effect.OperationID, at); err != nil {
			return err
		}
	}
	return nil
}

func reviewIdentityEncodingDomain(p memory.ReviewPreview, kind string) string {
	version := "v1"
	if p.Version == "owner-review-preview-v2" {
		version = "v2"
	}
	if p.Version == "owner-review-preview-v3" {
		version = "v3"
	}
	if p.Version == "owner-review-preview-v4" {
		version = "v4"
	}
	return "evie-owner-review-" + kind + "-" + version
}

func validateReviewNewPredicate(ctx context.Context, q reviewQuery, p memory.SemanticPredicate) error {
	if !p.Create {
		return errors.New("expected new Predicate")
	}
	var count int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM semantic_predicates WHERE token=? OR predicate_id=?`, p.Token, p.ID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrReviewStale
	}
	if p.Version != 1 || len(p.Token) > 64 || !predicateTokenPattern.MatchString(p.Token) || strings.TrimSpace(p.Label) == "" {
		return errors.New("invalid new Predicate")
	}
	return nil
}
