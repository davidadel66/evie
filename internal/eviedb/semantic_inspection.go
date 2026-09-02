package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/memory"
)

// InspectLiteralClaims is an eventless exact read of current accepted literal
// Claims in the session's default Context Scope.
func (s *Store) InspectLiteralClaims(ctx context.Context, scope memory.ScopeContext) (memory.LiteralClaimsInspection, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.LiteralClaimsInspection{}, err
	}
	effectiveAt := s.now().UTC()
	key := scopeKeyForContext(scope)
	var result memory.LiteralClaimsInspection
	result.EffectiveAt = effectiveAt
	var registry sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT scope_id, scope_key, registry_id, revision FROM semantic_scopes WHERE scope_key = ?
	`, key).Scan(&result.Scope.ID, &result.Scope.Key, &registry, &result.Scope.Revision); errors.Is(err, sql.ErrNoRows) {
		result.Scope.Key = key
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("load semantic inspection scope: %w", err)
	}
	if registry.Valid {
		result.Scope.RegistryID = registry.String
	}
	result.ScopeRevision = result.Scope.Revision
	rows, err := s.db.QueryContext(ctx, `
		SELECT claims.claim_id,
		       entities.entity_id, entity_scopes.scope_key, entities.canonical_name, entities.entity_type, COALESCE(entities.anchor_kind, ''),
		       predicates.predicate_id, predicates.token, predicates.version, predicates.label, predicates.object_constraint, predicates.cardinality,
		       claims.literal_kind, claims.literal_value, claims.polarity, claims.valid_from, claims.valid_to,
		       claims.created_operation_id, claims.transaction_time,
		       sources.source_link_id, sources.created_operation_id, sources.event_id, sources.source_session_id, sources.source_scope_key,
		       sources.event_part, sources.locator_kind, sources.locator_value, sources.evidence_sha256,
		       sources.source_actor, sources.source_type, sources.authority, sources.observed_at, events.content
		FROM semantic_claims AS claims
		JOIN semantic_entities AS entities ON entities.entity_id = claims.subject_entity_id
		JOIN semantic_scopes AS entity_scopes ON entity_scopes.scope_id = entities.scope_id
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_source_links AS sources ON sources.claim_id = claims.claim_id AND sources.eligibility = 'eligible'
		JOIN events ON events.id = sources.event_id
		WHERE claims.scope_id = ? AND claims.object_kind = 'literal' AND claims.lifecycle = 'active'
		  AND (claims.valid_from IS NULL OR claims.valid_from <= ?)
		  AND (claims.valid_to IS NULL OR claims.valid_to > ?)
		ORDER BY claims.claim_id, sources.source_link_id
	`, result.Scope.ID, formatSemanticTime(effectiveAt), formatSemanticTime(effectiveAt))
	if err != nil {
		return result, fmt.Errorf("query semantic Claims: %w", err)
	}
	defer rows.Close()
	claimIndexes := make(map[memory.SemanticID]int)
	for rows.Next() {
		var claim memory.LiteralClaimInspection
		var validFrom, validTo sql.NullString
		var transactionText string
		claim.Scope = result.Scope
		claim.EffectiveAt = effectiveAt
		if err := rows.Scan(
			&claim.ID,
			&claim.Subject.ID, &claim.Subject.ScopeKey, &claim.Subject.CanonicalName, &claim.Subject.EntityType, &claim.Subject.AnchorKind,
			&claim.Predicate.ID, &claim.Predicate.Token, &claim.Predicate.Version, &claim.Predicate.Label,
			&claim.Predicate.ObjectConstraint, &claim.Predicate.Cardinality,
			&claim.Literal.Kind, &claim.Literal.Value, &claim.Polarity, &validFrom, &validTo,
			&claim.OperationID, &transactionText,
			&claim.Source.ID, &claim.Source.OperationID, &claim.Source.EventID, &claim.Source.SessionID, &claim.Source.ScopeKey,
			&claim.Source.EventPart, &claim.Source.LocatorKind, &claim.Source.LocatorValue, &claim.Source.EvidenceSHA256,
			&claim.Source.Actor, &claim.Source.SourceType, &claim.Source.Authority, &claim.Source.ObservedAt, &claim.Source.Evidence,
		); err != nil {
			return result, fmt.Errorf("scan semantic Claim: %w", err)
		}
		claim.TransactionTime, err = parseSemanticTime(transactionText)
		if err != nil {
			return result, err
		}
		if validFrom.Valid {
			value, err := parseSemanticTime(validFrom.String)
			if err != nil {
				return result, err
			}
			claim.ValidTime.From = &value
		}
		if validTo.Valid {
			value, err := parseSemanticTime(validTo.String)
			if err != nil {
				return result, err
			}
			claim.ValidTime.To = &value
		}
		if index, ok := claimIndexes[claim.ID]; ok {
			result.Claims[index].Sources = append(result.Claims[index].Sources, claim.Source)
			continue
		}
		claim.Sources = []memory.SemanticSource{claim.Source}
		claimIndexes[claim.ID] = len(result.Claims)
		result.Claims = append(result.Claims, claim)
	}
	return result, rows.Err()
}

// InspectEntityClaims is the exact current read for accepted Entity-object Claims.
func (s *Store) InspectEntityClaims(ctx context.Context, scope memory.ScopeContext) (memory.EntityClaimsInspection, error) {
	return s.InspectEntityClaimsAtScope(ctx, scope, false)
}

// InspectEntityClaimsAtScope selects the default Context Scope or the current
// session Scope without widening the exact claim read to other scopes.
func (s *Store) InspectEntityClaimsAtScope(ctx context.Context, scope memory.ScopeContext, useSessionScope bool) (memory.EntityClaimsInspection, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.EntityClaimsInspection{}, err
	}
	effectiveAt := s.now().UTC()
	key := targetScopeKey(scope, useSessionScope)
	result := memory.EntityClaimsInspection{EffectiveAt: effectiveAt}
	var registry sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT scope_id, scope_key, registry_id, revision FROM semantic_scopes WHERE scope_key = ?
	`, key).Scan(&result.Scope.ID, &result.Scope.Key, &registry, &result.Scope.Revision); errors.Is(err, sql.ErrNoRows) {
		result.Scope.Key = key
		return result, nil
	} else if err != nil {
		return result, fmt.Errorf("load semantic inspection scope: %w", err)
	}
	if registry.Valid {
		result.Scope.RegistryID = registry.String
	}
	result.ScopeRevision = result.Scope.Revision
	rows, err := s.db.QueryContext(ctx, `
		SELECT claims.claim_id, claims.subject_entity_id, claims.object_entity_id, claims.polarity,
		       claims.created_operation_id, claims.transaction_time,
		       subjects.canonical_name, subjects.entity_type, COALESCE(subjects.anchor_kind, ''), subject_scopes.scope_key,
		       objects.canonical_name, objects.entity_type, COALESCE(objects.anchor_kind, ''), object_scopes.scope_key,
		       predicates.predicate_id, predicates.token, predicates.version, predicates.label, predicates.cardinality,
		       sources.source_link_id, sources.created_operation_id, sources.event_id, sources.source_session_id, sources.source_scope_key,
		       sources.event_part, sources.locator_kind, sources.locator_value, sources.evidence_sha256,
		       sources.source_actor, sources.source_type, sources.authority, sources.observed_at, events.content
		FROM semantic_claims AS claims
		JOIN semantic_entities AS subjects ON subjects.entity_id = claims.subject_entity_id
		JOIN semantic_scopes AS subject_scopes ON subject_scopes.scope_id = subjects.scope_id
		JOIN semantic_entities AS objects ON objects.entity_id = claims.object_entity_id
		JOIN semantic_scopes AS object_scopes ON object_scopes.scope_id = objects.scope_id
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_source_links AS sources ON sources.claim_id = claims.claim_id AND sources.eligibility = 'eligible'
		JOIN events ON events.id = sources.event_id
		WHERE claims.scope_id = ? AND claims.object_kind = 'entity' AND claims.lifecycle = 'active'
		  AND (claims.valid_from IS NULL OR claims.valid_from <= ?)
		  AND (claims.valid_to IS NULL OR claims.valid_to > ?)
		ORDER BY claims.claim_id, sources.source_link_id
	`, result.Scope.ID, formatSemanticTime(effectiveAt), formatSemanticTime(effectiveAt))
	if err != nil {
		return result, err
	}
	defer rows.Close()
	byID := make(map[memory.SemanticID]int)
	for rows.Next() {
		var claim memory.EntityClaimInspection
		var source memory.SemanticSource
		var transactionText string
		claim.Scope = result.Scope
		if err := rows.Scan(
			&claim.Claim.ID, &claim.Claim.SubjectEntityID, &claim.Claim.ObjectEntityID, &claim.Claim.Polarity,
			&claim.OperationID, &transactionText,
			&claim.Subject.CanonicalName, &claim.Subject.EntityType, &claim.Subject.AnchorKind, &claim.Subject.ScopeKey,
			&claim.Object.CanonicalName, &claim.Object.EntityType, &claim.Object.AnchorKind, &claim.Object.ScopeKey,
			&claim.Predicate.ID, &claim.Predicate.Token, &claim.Predicate.Version, &claim.Predicate.Label, &claim.Predicate.Cardinality,
			&source.ID, &source.OperationID, &source.EventID, &source.SessionID, &source.ScopeKey,
			&source.EventPart, &source.LocatorKind, &source.LocatorValue, &source.EvidenceSHA256,
			&source.Actor, &source.SourceType, &source.Authority, &source.ObservedAt, &source.Evidence,
		); err != nil {
			return result, err
		}
		claim.Subject.ID = claim.Claim.SubjectEntityID
		claim.Object.ID = claim.Claim.ObjectEntityID
		claim.Predicate.ObjectConstraint = memory.ConstraintEntity
		claim.Claim.ScopeKey = result.Scope.Key
		claim.Claim.PredicateID = claim.Predicate.ID
		claim.Claim.PredicateToken = claim.Predicate.Token
		claim.Claim.PredicateVersion = claim.Predicate.Version
		claim.Claim.ValidTime = memory.ValidTime{}
		claim.TransactionTime, err = parseSemanticTime(transactionText)
		if err != nil {
			return result, err
		}
		if index, ok := byID[claim.Claim.ID]; ok {
			result.Claims[index].Sources = append(result.Claims[index].Sources, source)
			continue
		}
		claim.Sources = []memory.SemanticSource{source}
		byID[claim.Claim.ID] = len(result.Claims)
		result.Claims = append(result.Claims, claim)
	}
	return result, rows.Err()
}
