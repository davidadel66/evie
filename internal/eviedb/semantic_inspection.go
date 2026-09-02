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
		       sources.source_link_id, sources.event_id, sources.source_session_id, sources.source_scope_key,
		       sources.event_part, sources.locator_kind, sources.locator_value, sources.evidence_sha256,
		       sources.source_actor, sources.source_type, sources.authority, sources.observed_at, events.content
		FROM semantic_claims AS claims
		JOIN semantic_entities AS entities ON entities.entity_id = claims.subject_entity_id
		JOIN semantic_scopes AS entity_scopes ON entity_scopes.scope_id = entities.scope_id
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_source_links AS sources ON sources.claim_id = claims.claim_id AND sources.eligibility = 'eligible'
		JOIN events ON events.id = sources.event_id
		WHERE claims.scope_id = ? AND claims.lifecycle = 'active'
		  AND (claims.valid_from IS NULL OR claims.valid_from <= ?)
		  AND (claims.valid_to IS NULL OR claims.valid_to > ?)
		ORDER BY claims.claim_id, sources.source_link_id
	`, result.Scope.ID, formatSemanticTime(effectiveAt), formatSemanticTime(effectiveAt))
	if err != nil {
		return result, fmt.Errorf("query semantic Claims: %w", err)
	}
	defer rows.Close()
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
			&claim.Source.ID, &claim.Source.EventID, &claim.Source.SessionID, &claim.Source.ScopeKey,
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
		result.Claims = append(result.Claims, claim)
	}
	return result, rows.Err()
}
