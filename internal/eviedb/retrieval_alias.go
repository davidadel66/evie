package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"reflect"

	"github.com/davidadel66/evie/internal/memory"
)

// Only an exact query can introduce identity matches. Two mappings per selected
// Claim and at most three candidate origin links keep the extra work bounded.
func (c *retrievalCandidates) exactIdentityMatches(ctx context.Context, ids []memory.SemanticID) error {
	for _, id := range ids {
		candidate := c.eligible[id]
		if candidate == nil || candidate.evidence.Claim == nil {
			continue
		}
		claim := candidate.evidence.Claim
		var refs []memory.RetrievalIdentityReference
		for _, entity := range []memory.SemanticID{claim.SubjectEntityID, claim.Object.EntityID} {
			if entity != "" && string(entity) == c.plan.query.Text {
				refs = append(refs, memory.RetrievalIdentityReference{Kind: "entity", EntityID: entity})
			}
		}
		rows, err := c.tx.QueryContext(ctx, `SELECT a.alias_id,a.entity_id FROM semantic_aliases a JOIN semantic_scopes sc ON sc.scope_id=a.scope_id WHERE a.entity_id IN (?,?) AND a.normalized_value=? AND sc.scope_key IN (?,?,?) ORDER BY a.alias_id LIMIT 2`, claim.SubjectEntityID, claim.Object.EntityID, normalizeAlias(c.plan.query.Text), c.plan.scopes[0], c.plan.scopes[1], c.plan.scopes[2])
		if err != nil {
			return err
		}
		for rows.Next() {
			ref := memory.RetrievalIdentityReference{Kind: "alias"}
			if err = rows.Scan(&ref.AliasID, &ref.EntityID); err != nil {
				rows.Close()
				return err
			}
			refs = append(refs, ref)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		candidate.evidence.IdentityMatches, err = resolveRetrievalIdentityMatches(ctx, c.tx, c.plan.metadata, candidate.evidence, refs)
		if err != nil {
			return err
		}
	}
	return nil
}

func resolveRetrievalIdentityMatches(ctx context.Context, q semanticInspectionQueryer, metadata memory.ExactReadMetadata, evidence memory.RetrievalEvidence, refs []memory.RetrievalIdentityReference) ([]memory.RetrievalIdentityMatch, error) {
	var matches []memory.RetrievalIdentityMatch
	if evidence.Claim == nil {
		return matches, nil
	}
	for _, ref := range refs {
		if len(matches) == 2 {
			break
		}
		if ref.EntityID != evidence.Claim.SubjectEntityID && ref.EntityID != evidence.Claim.Object.EntityID {
			continue
		}
		active, err := retrievalIdentityActive(ctx, q, memory.SemanticObjectEntity, ref.EntityID, metadata)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		if ref.Kind == "entity" && ref.AliasID == "" && ref.Source == nil {
			matches = append(matches, memory.RetrievalIdentityMatch{RetrievalIdentityReference: ref})
			continue
		}
		if ref.Kind != "alias" || ref.AliasID == "" {
			continue
		}
		var entity memory.SemanticID
		var scope, value string
		var event memory.EventID
		var operation memory.SemanticID
		err = q.QueryRowContext(ctx, `SELECT a.entity_id,sc.scope_key,CASE WHEN length(CAST(a.value AS BLOB))<=128 THEN a.value ELSE '' END,a.source_event_id,a.created_operation_id FROM semantic_aliases a JOIN semantic_scopes sc ON sc.scope_id=a.scope_id WHERE a.alias_id=?`, ref.AliasID).Scan(&entity, &scope, &value, &event, &operation)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if entity != ref.EntityID || !containsString(metadata.AllowedScopes, scope) || value == "" || compilerHasSecret(value) {
			continue
		}
		active, err = retrievalIdentityActive(ctx, q, memory.SemanticObjectAlias, ref.AliasID, metadata)
		if err != nil {
			return nil, err
		}
		if !active {
			continue
		}
		var sourceIDs []memory.SemanticID
		if ref.Source != nil {
			sourceIDs = append(sourceIDs, ref.Source.SourceLinkID)
		} else {
			rows, err := q.QueryContext(ctx, `SELECT source_link_id FROM semantic_source_links WHERE event_id=? AND created_operation_id=? ORDER BY source_link_id LIMIT 3`, event, operation)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var id memory.SemanticID
				if err = rows.Scan(&id); err != nil {
					rows.Close()
					return nil, err
				}
				sourceIDs = append(sourceIDs, id)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return nil, err
			}
		}
		for _, id := range sourceIDs {
			state, err := latestStateAt(ctx, q, memory.SemanticObjectSourceLink, id, formatSemanticTime(metadata.AsKnownAt))
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if state != memory.SemanticStateEligible {
				continue
			}
			source, err := loadSourceForInspection(ctx, q, id)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if source.EventID != event || source.OperationID != operation {
				continue
			}
			source, eligible, err := retrievalSource(ctx, q, source, metadata.AllowedScopes)
			if err != nil {
				return nil, err
			}
			if !eligible || source.Evidence == "" {
				continue
			}
			sourceRef := (memory.RetrievalEvidence{Sources: []memory.SemanticSource{source}}).Reference().Sources[0]
			if ref.Source != nil && !reflect.DeepEqual(*ref.Source, sourceRef) {
				continue
			}
			ref.Source = &sourceRef
			matches = append(matches, memory.RetrievalIdentityMatch{RetrievalIdentityReference: ref, AliasValue: value})
			break
		}
	}
	return matches, nil
}

func retrievalIdentityActive(ctx context.Context, q semanticInspectionQueryer, kind memory.SemanticObjectKind, id memory.SemanticID, metadata memory.ExactReadMetadata) (bool, error) {
	state, err := latestStateAt(ctx, q, kind, id, formatSemanticTime(metadata.AsKnownAt))
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil || state != memory.SemanticStateActive {
		return false, err
	}
	var current memory.SemanticStateValue
	err = q.QueryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind=? AND object_id=? ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1`, kind, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return current == memory.SemanticStateActive, err
}
