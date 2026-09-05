package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func promotionRequestsEqual(left, right memory.PromotionRequest) bool {
	return left == right
}

func (s *Store) PreparePromotion(
	ctx context.Context,
	scope memory.ScopeContext,
	request memory.PromotionRequest,
) (memory.PromotionProposal, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.PromotionProposal{}, err
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.PromotionProposal{}, errors.New("Promotion idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if err := validateSemanticUUID(string(request.SourceClaimID)); err != nil {
		return memory.PromotionProposal{}, err
	}

	var acceptedJSON, acceptedHash string
	err := s.db.QueryRowContext(ctx, `
		SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?
	`, request.IdempotencyKey).Scan(&acceptedJSON, &acceptedHash)
	if err == nil {
		var proposal memory.PromotionProposal
		if err := json.Unmarshal([]byte(acceptedJSON), &proposal); err != nil {
			return proposal, err
		}
		if proposal.SessionID != scope.SessionID || !promotionRequestsEqual(proposal.Request, request) {
			return memory.PromotionProposal{}, ErrIdempotencyConflict
		}
		if err := requirePromotionReviewDisclosure(ctx, s.db, proposal.Sources); err != nil {
			return memory.PromotionProposal{}, err
		}
		proposal.ProposalSHA256 = acceptedHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		return proposal, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.PromotionProposal{}, err
	}
	contextKey := scopeKeyForContext(scope)
	sourceClaim, err := loadSemanticClaim(ctx, s.db, request.SourceClaimID)
	if err != nil {
		return memory.PromotionProposal{}, fmt.Errorf("load Promotion source Claim: %w", err)
	}
	sourceKey := sourceClaim.ScopeKey
	if err := requireSemanticScopeKeysAvailable(ctx, s.db, []string{sourceKey, request.DestinationScopeKey}); err != nil {
		return memory.PromotionProposal{}, err
	}
	if !promotionPathAllowed(scope, sourceKey, request.DestinationScopeKey) {
		return memory.PromotionProposal{}, errors.New("Promotion source and destination are outside the session-bound broader-scope path")
	}
	sourceScope, err := loadSemanticScope(ctx, s.db, sourceKey)
	if err != nil || sourceScope.Create {
		return memory.PromotionProposal{}, errors.New("Promotion source scope is not registered")
	}
	destinationScope, err := loadSemanticScope(ctx, s.db, request.DestinationScopeKey)
	if err != nil {
		return memory.PromotionProposal{}, err
	}
	if destinationScope.Create && request.DestinationScopeKey != "global" && request.DestinationScopeKey != contextKey {
		return memory.PromotionProposal{}, errors.New("Promotion destination scope is not registered")
	}

	at := s.now().UTC()
	var latest sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(transaction_time) FROM semantic_operations`).Scan(&latest); err != nil {
		return memory.PromotionProposal{}, err
	}
	if latest.Valid {
		acceptedAt, err := parseSemanticTime(latest.String)
		if err != nil {
			return memory.PromotionProposal{}, err
		}
		if acceptedAt.After(at) {
			at = acceptedAt
		}
	}
	lifecycle, err := loadSemanticLifecycleAt(ctx, s.db, "claim", sourceClaim.ID, at)
	if err != nil || len(lifecycle) == 0 || lifecycle[len(lifecycle)-1].State != memory.SemanticStateActive {
		return memory.PromotionProposal{}, errors.New("Promotion source Claim is not active")
	}
	sources, err := loadEligibleSourcesAt(ctx, s.db, sourceClaim.ID, at)
	if err != nil || len(sources) == 0 {
		return memory.PromotionProposal{}, errors.New("Promotion source Claim has no eligible source")
	}
	allowedSourceScopes := make(map[string]struct{})
	for _, key := range allowedSemanticReadScopeKeys(scope) {
		allowedSourceScopes[key] = struct{}{}
	}
	for index := range sources {
		if _, allowed := allowedSourceScopes[sources[index].ScopeKey]; !allowed {
			sources[index].Evidence = ""
		}
	}

	if err := requirePromotionReviewDisclosure(ctx, s.db, sources); err != nil {
		return memory.PromotionProposal{}, err
	}

	operationID, err := newSemanticID()
	if err != nil {
		return memory.PromotionProposal{}, err
	}
	promoted, mapped, err := s.preparePromotedEntities(ctx, sourceClaim, destinationScope.SemanticScope)
	if err != nil {
		return memory.PromotionProposal{}, err
	}
	destinationClaim := sourceClaim
	destinationClaim.ID = ""
	destinationClaim.ScopeKey = destinationScope.Key
	destinationClaim.CreatedOperationID = operationID
	destinationClaim.TransactionTime = time.Time{}
	destinationClaim.SubjectEntityID = mapped[sourceClaim.SubjectEntityID]
	if sourceClaim.Object.EntityID != "" {
		destinationClaim.Object.EntityID = mapped[sourceClaim.Object.EntityID]
	}
	destinationClaimCreate := false
	err = s.db.QueryRowContext(ctx, semanticClaimByPropositionQuery,
		destinationScope.ID, destinationClaim.SubjectEntityID, destinationClaim.Predicate.ID,
		claimObjectKind(destinationClaim.Object), nullableSemanticID(destinationClaim.Object.EntityID),
		literalKindArgument(destinationClaim.Object.Literal), literalValueArgument(destinationClaim.Object.Literal),
		destinationClaim.Polarity, semanticTimeArgument(destinationClaim.ValidTime.From), semanticTimeArgument(destinationClaim.ValidTime.To),
	).Scan(&destinationClaim.ID)
	if errors.Is(err, sql.ErrNoRows) {
		destinationClaim.ID, err = newSemanticID()
		destinationClaimCreate = true
	} else if err == nil {
		destinationClaim, err = loadSemanticClaim(ctx, s.db, destinationClaim.ID)
	}
	if err != nil {
		return memory.PromotionProposal{}, err
	}

	promotedSources := make([]memory.SemanticSource, 0, len(sources))
	for _, source := range sources {
		var existing memory.SemanticSource
		var existingState memory.SemanticStateValue
		err := s.db.QueryRowContext(ctx, `
			SELECT source_link_id, created_operation_id,
			       (SELECT state FROM semantic_state_events
			        WHERE object_kind = 'source_link' AND object_id = semantic_source_links.source_link_id
			        ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1)
			FROM semantic_source_links
			WHERE claim_id = ? AND event_id = ? AND event_part = ? AND locator_kind = ?
			  AND locator_value = ? AND evidence_sha256 = ?
		`, destinationClaim.ID, source.EventID, source.EventPart, source.LocatorKind,
			source.LocatorValue, source.EvidenceSHA256).Scan(&existing.ID, &existing.OperationID, &existingState)
		if errors.Is(err, sql.ErrNoRows) {
			source.ID, err = newSemanticID()
			source.OperationID = operationID
			source.Create = true
		} else if err == nil {
			if existingState != memory.SemanticStateEligible {
				return memory.PromotionProposal{}, errors.New("Promotion destination Source Link is retracted; restore it explicitly")
			}
			source.ID, source.OperationID, source.Create = existing.ID, existing.OperationID, false
		}
		if err != nil {
			return memory.PromotionProposal{}, err
		}
		promotedSources = append(promotedSources, source)
	}
	evidence, err := loadLifecycleEvidence(s.db.QueryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
	`, request.SourceEventID), scope.SessionID, request.SourceEventID, contextKey)
	if err != nil {
		return memory.PromotionProposal{}, err
	}
	scopes := []memory.SemanticScope{sourceScope.SemanticScope, destinationScope.SemanticScope}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Key < scopes[j].Key })
	priors := make([]memory.ScopeRevision, len(scopes))
	for i := range scopes {
		priors[i] = memory.ScopeRevision{ScopeKey: scopes[i].Key, Revision: scopes[i].Revision}
	}
	proposal := memory.PromotionProposal{
		SchemaVersion: 4, Kind: "promote_claim", OperationID: operationID,
		IdempotencyKey: request.IdempotencyKey, Actor: memory.SemanticActorOwner, SessionID: scope.SessionID,
		SourceScope: sourceScope.SemanticScope, DestinationScope: destinationScope.SemanticScope,
		Scopes: scopes, PriorRevisions: priors, SourceClaim: sourceClaim, DestinationClaim: destinationClaim,
		DestinationClaimCreate: destinationClaimCreate, PromotedEntities: promoted,
		Sources: promotedSources, Evidence: evidence, Request: request,
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalPromoteClaimProposal(proposal))
	if err != nil {
		return proposal, err
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	return proposal, err
}

func promotionPathAllowed(scope memory.ScopeContext, source, destination string) bool {
	if source == "global" || source == destination {
		return false
	}
	contextKey := scopeKeyForContext(scope)
	sessionKey := "session:" + string(scope.SessionID)
	if source == sessionKey {
		return destination == "global" || (contextKey != "global" && destination == contextKey)
	}
	return source == contextKey && contextKey != "global" && destination == "global"
}

func (s *Store) preparePromotedEntities(
	ctx context.Context,
	claim memory.SemanticClaim,
	destination memory.SemanticScope,
) ([]memory.PromotedEntity, map[memory.SemanticID]memory.SemanticID, error) {
	ids := []memory.SemanticID{claim.SubjectEntityID}
	if claim.Object.EntityID != "" && claim.Object.EntityID != claim.SubjectEntityID {
		ids = append(ids, claim.Object.EntityID)
	}
	promoted := make([]memory.PromotedEntity, 0, len(ids))
	mapped := make(map[memory.SemanticID]memory.SemanticID, len(ids))
	for _, sourceID := range ids {
		entity, err := loadSemanticEntityForInspection(ctx, s.db, sourceID)
		if err != nil {
			return nil, nil, err
		}
		if entity.ScopeKey == "global" {
			mapped[sourceID] = sourceID
			continue
		}
		var destinationEntity memory.SemanticEntity
		err = s.db.QueryRowContext(ctx, `
			SELECT destination.entity_id, scopes.scope_key, destination.canonical_name,
			       destination.entity_type, COALESCE(destination.anchor_kind, '')
			FROM semantic_promotion_entities AS mappings
			JOIN semantic_entities AS destination ON destination.entity_id = mappings.destination_entity_id
			JOIN semantic_scopes AS scopes ON scopes.scope_id = destination.scope_id
			WHERE mappings.source_entity_id = ? AND destination.scope_id = ?
			  AND COALESCE((SELECT state FROM semantic_state_events
			      WHERE object_kind = 'entity' AND object_id = destination.entity_id
			      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), destination.lifecycle) = 'active'
			ORDER BY mappings.operation_id LIMIT 1
		`, sourceID, destination.ID).Scan(&destinationEntity.ID, &destinationEntity.ScopeKey,
			&destinationEntity.CanonicalName, &destinationEntity.EntityType, &destinationEntity.AnchorKind)
		if errors.Is(err, sql.ErrNoRows) {
			destinationEntity = memory.SemanticEntity{
				ScopeKey: destination.Key, CanonicalName: entity.CanonicalName, EntityType: entity.EntityType, Create: true,
			}
			destinationEntity.ID, err = newSemanticID()
		}
		if err != nil {
			return nil, nil, err
		}
		mapped[sourceID] = destinationEntity.ID
		promoted = append(promoted, memory.PromotedEntity{SourceEntityID: sourceID, DestinationEntity: destinationEntity})
	}
	return promoted, mapped, nil
}

const semanticClaimByPropositionQuery = `
	SELECT claims.claim_id FROM semantic_claims AS claims
	WHERE claims.scope_id = ? AND claims.subject_entity_id = ? AND claims.predicate_id = ? AND claims.object_kind = ?
	  AND claims.object_entity_id IS ? AND claims.literal_kind IS ? AND claims.literal_value IS ?
	  AND claims.polarity = ? AND claims.valid_from IS ? AND claims.valid_to IS ?
	  AND COALESCE((SELECT state FROM semantic_state_events
	      WHERE object_kind = 'claim' AND object_id = claims.claim_id
	      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), claims.lifecycle) = 'active'
`

func claimObjectKind(object memory.ClaimObject) string {
	if object.EntityID != "" {
		return "entity"
	}
	return "literal"
}

func nullableSemanticID(id memory.SemanticID) any {
	if id == "" {
		return nil
	}
	return id
}

func literalKindArgument(literal *memory.TypedLiteral) any {
	if literal == nil {
		return nil
	}
	return literal.Kind
}

func literalValueArgument(literal *memory.TypedLiteral) any {
	if literal == nil {
		return nil
	}
	return literal.Value
}

func (s *Store) ApplyPromotion(
	ctx context.Context,
	lease memory.TurnLease,
	proposal memory.PromotionProposal,
) (result memory.PromotionResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("Promotion proposal does not match its turn lease")
	}
	canonical := canonicalPromoteClaimProposal(proposal)
	hash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if proposal.SchemaVersion != 4 || proposal.Kind != "promote_claim" || proposal.Actor != memory.SemanticActorOwner ||
		proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != hash || proposal.PreparedSHA256 != preparedHash ||
		proposal.SourceClaim.ID != proposal.Request.SourceClaimID || proposal.DestinationScope.Key != proposal.Request.DestinationScopeKey ||
		proposal.SourceClaim.ID == proposal.DestinationClaim.ID || len(proposal.Scopes) != 2 || len(proposal.PriorRevisions) != 2 {
		return result, errors.New("invalid Promotion proposal")
	}
	for _, id := range []memory.SemanticID{proposal.OperationID, proposal.SourceScope.ID, proposal.DestinationScope.ID,
		proposal.SourceClaim.ID, proposal.DestinationClaim.ID} {
		if err := validateSemanticUUID(string(id)); err != nil {
			return result, err
		}
	}
	for _, promoted := range proposal.PromotedEntities {
		if err := validateSemanticUUID(string(promoted.SourceEntityID)); err != nil {
			return result, err
		}
		if err := validateEntityIdentity(promoted.DestinationEntity); err != nil {
			return result, err
		}
	}
	for _, source := range proposal.Sources {
		if err := validateSemanticUUID(string(source.ID)); err != nil {
			return result, err
		}
	}

	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var acceptedHash, acceptedResult string
		existingErr := writer.queryRowContext(ctx, `
			SELECT proposal_sha256, result_json FROM semantic_operations WHERE idempotency_key = ?
		`, proposal.IdempotencyKey).Scan(&acceptedHash, &acceptedResult)
		if existingErr == nil {
			if acceptedHash != hash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal([]byte(acceptedResult), &result)
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		var approvalPayload []byte
		if err := writer.queryRowContext(ctx, `
			SELECT payload_json FROM events
			WHERE session_id = ? AND event_type = 'approval' AND execution_id = ? AND parent_id = ?
			ORDER BY sequence DESC LIMIT 1
		`, proposal.SessionID, proposal.OperationID, proposal.Evidence.EventID).Scan(&approvalPayload); err != nil {
			return errors.New("Promotion has no trusted approval event")
		}
		var approval memory.ApprovalPayload
		if err := json.Unmarshal(approvalPayload, &approval); err != nil || approval.Decision != memory.ApprovalApproved ||
			approval.ProposalSHA256 != hash || approval.PreparedSHA256 != preparedHash {
			return errors.New("Promotion changed after approval")
		}

		var workspaceID, projectID sql.NullString
		if err := writer.queryRowContext(ctx, `
			SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = 'active'
		`, proposal.SessionID).Scan(&workspaceID, &projectID); err != nil {
			return fmt.Errorf("load active Promotion session: %w", err)
		}
		bound := memory.ScopeContext{SessionID: proposal.SessionID}
		if workspaceID.Valid {
			bound.WorkspaceID = memory.WorkspaceID(workspaceID.String)
		}
		if projectID.Valid {
			bound.ProjectID = memory.ProjectID(projectID.String)
		}
		if !promotionPathAllowed(bound, proposal.SourceScope.Key, proposal.DestinationScope.Key) {
			return errors.New("Promotion scope path changed or is unauthorized")
		}
		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions, s.now())
		if err != nil {
			return err
		}
		if byKey[proposal.SourceScope.Key] != proposal.SourceScope || byKey[proposal.DestinationScope.Key] != proposal.DestinationScope {
			return errors.New("Promotion scope identity changed")
		}

		evidence, err := loadLifecycleEvidence(writer.queryRowContext(ctx, `
			SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
		`, proposal.Evidence.EventID), proposal.SessionID, proposal.Evidence.EventID, scopeKeyForContext(bound))
		if err != nil || evidence != proposal.Evidence {
			return errors.New("Promotion approval evidence changed")
		}
		sourceClaim, err := loadSemanticClaimFromWriter(ctx, writer, proposal.SourceClaim.ID)
		if err != nil || !semanticClaimsEqual(sourceClaim, proposal.SourceClaim) || sourceClaim.ScopeKey != proposal.SourceScope.Key {
			return errors.New("Promotion source Claim changed")
		}
		var sourceState memory.SemanticStateValue
		if err := writer.queryRowContext(ctx, `
			SELECT state FROM semantic_state_events WHERE object_kind = 'claim' AND object_id = ?
			ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1
		`, sourceClaim.ID).Scan(&sourceState); err != nil || sourceState != memory.SemanticStateActive {
			return errors.New("Promotion source Claim is no longer active")
		}
		if err := validatePromotionSources(ctx, writer, proposal); err != nil {
			return err
		}
		if err := validatePromotionEntities(ctx, writer, proposal); err != nil {
			return err
		}

		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		result = memory.PromotionResult{
			OperationID: proposal.OperationID, SourceClaimID: proposal.SourceClaim.ID,
			DestinationClaimID: proposal.DestinationClaim.ID, TransactionTime: now,
			DestinationRevision: proposal.DestinationScope.Revision + 1,
		}
		for _, semanticScope := range proposal.Scopes {
			revision := semanticScope.Revision
			if semanticScope.Key == proposal.DestinationScope.Key {
				revision++
			}
			result.ResultingRevisions = append(result.ResultingRevisions, memory.ScopeRevision{ScopeKey: semanticScope.Key, Revision: revision})
		}
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		effectHash, _, err := semanticHash(canonical.Effect)
		if err != nil {
			return err
		}
		if err := recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{
			SchemaVersion: proposal.SchemaVersion, OperationID: proposal.OperationID, Kind: proposal.Kind,
			IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor, SessionID: proposal.SessionID,
			TargetScopeID: proposal.DestinationScope.ID, SourceEventID: proposal.Evidence.EventID,
			ProposalHash: hash, EffectHash: effectHash, ProposalJSON: proposalJSON, PreparedJSON: preparedJSON,
			ResultJSON: resultJSON, TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey,
		}); err != nil {
			return err
		}
		transactionText := formatSemanticTime(now)
		for _, promoted := range proposal.PromotedEntities {
			entity := promoted.DestinationEntity
			if entity.Create {
				if _, err := writer.execContext(ctx, `
					INSERT INTO semantic_entities (entity_id, scope_id, canonical_name, entity_type, anchor_kind, lifecycle, created_operation_id)
					VALUES (?, ?, ?, ?, NULL, 'active', ?)
				`, entity.ID, proposal.DestinationScope.ID, entity.CanonicalName, entity.EntityType, proposal.OperationID); err != nil {
					return err
				}
				if _, err := writer.execContext(ctx, `
					INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time)
					VALUES (?, 'entity', ?, 'active', ?, ?, ?)
				`, proposal.DestinationScope.ID, entity.ID, proposal.OperationID, result.DestinationRevision, transactionText); err != nil {
					return err
				}
			}
		}
		if proposal.DestinationClaimCreate {
			claim := proposal.DestinationClaim
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_claims (
					claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
					object_kind, object_entity_id, literal_kind, literal_value, polarity, valid_from, valid_to,
					lifecycle, created_operation_id, transaction_time
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
			`, claim.ID, proposal.DestinationScope.ID, claim.SubjectEntityID, claim.Predicate.ID,
				claim.Predicate.Token, claim.Predicate.Version, claimObjectKind(claim.Object),
				nullableSemanticID(claim.Object.EntityID), literalKindArgument(claim.Object.Literal),
				literalValueArgument(claim.Object.Literal), claim.Polarity,
				semanticTimeArgument(claim.ValidTime.From), semanticTimeArgument(claim.ValidTime.To),
				proposal.OperationID, transactionText); err != nil {
				return err
			}
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time)
				VALUES (?, 'claim', ?, 'active', ?, ?, ?)
			`, proposal.DestinationScope.ID, claim.ID, proposal.OperationID, result.DestinationRevision, transactionText); err != nil {
				return err
			}
		}
		for _, source := range proposal.Sources {
			if !source.Create {
				continue
			}
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_source_links (
					source_link_id, scope_id, claim_id, event_id, source_session_id, source_scope_key, event_part,
					locator_kind, locator_value, evidence_sha256, source_actor, source_type, authority,
					observed_at, eligibility, created_operation_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'eligible', ?)
			`, source.ID, proposal.DestinationScope.ID, proposal.DestinationClaim.ID, source.EventID, source.SessionID, source.ScopeKey,
				source.EventPart, source.LocatorKind, source.LocatorValue, source.EvidenceSHA256,
				source.Actor, source.SourceType, source.Authority, source.ObservedAt, proposal.OperationID); err != nil {
				return err
			}
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time)
				VALUES (?, 'source_link', ?, 'eligible', ?, ?, ?)
			`, proposal.DestinationScope.ID, source.ID, proposal.OperationID, result.DestinationRevision, transactionText); err != nil {
				return err
			}
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_promotions (operation_id, source_scope_id, destination_scope_id, source_claim_id, destination_claim_id)
			VALUES (?, ?, ?, ?, ?)
		`, proposal.OperationID, proposal.SourceScope.ID, proposal.DestinationScope.ID,
			proposal.SourceClaim.ID, proposal.DestinationClaim.ID); err != nil {
			return err
		}
		for _, promoted := range proposal.PromotedEntities {
			if _, err := writer.execContext(ctx, `
				INSERT OR IGNORE INTO semantic_promotion_entities (operation_id, source_entity_id, destination_entity_id)
				VALUES (?, ?, ?)
			`, proposal.OperationID, promoted.SourceEntityID, promoted.DestinationEntity.ID); err != nil {
				return err
			}
		}
		updated, err := writer.execContext(ctx, `
			UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?
		`, result.DestinationRevision, proposal.DestinationScope.ID, proposal.DestinationScope.Revision)
		if err != nil {
			return err
		}
		if rows, _ := updated.RowsAffected(); rows != 1 {
			return ErrStaleScopeRevision
		}
		return nil
	})
	return result, err
}

func validatePromotionSources(
	ctx context.Context,
	writer turnLeaseWriteExecutor,
	proposal memory.PromotionProposal,
) error {
	sourceClaimID := proposal.SourceClaim.ID
	sources := proposal.Sources
	if len(sources) == 0 {
		return errors.New("Promotion source Claim has no eligible source")
	}
	rows, err := writer.queryContext(ctx, `
		SELECT links.source_link_id, links.event_id, links.source_session_id, links.source_scope_key,
		       links.event_part, links.locator_kind, links.locator_value, links.evidence_sha256,
		       links.source_actor, links.source_type, links.authority, links.observed_at, events.content
		FROM semantic_source_links AS links JOIN events ON events.id = links.event_id
		WHERE links.claim_id = ? AND
		  (SELECT state FROM semantic_state_events WHERE object_kind = 'source_link' AND object_id = links.source_link_id
		   ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'eligible'
		ORDER BY links.source_link_id
	`, sourceClaimID)
	if err != nil {
		return err
	}
	defer rows.Close()
	eligible := make(map[string]memory.SemanticSource)
	for rows.Next() {
		var source memory.SemanticSource
		if err := rows.Scan(&source.ID, &source.EventID, &source.SessionID, &source.ScopeKey,
			&source.EventPart, &source.LocatorKind, &source.LocatorValue, &source.EvidenceSHA256,
			&source.Actor, &source.SourceType, &source.Authority, &source.ObservedAt, &source.Evidence); err != nil {
			return err
		}
		eligible[promotionSourceKey(source)] = source
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(eligible) != len(sources) {
		return errors.New("Promotion source eligibility changed")
	}
	seen := make(map[string]struct{}, len(sources))
	for _, proposed := range sources {
		key := promotionSourceKey(proposed)
		if _, duplicate := seen[key]; duplicate {
			return errors.New("Promotion contains duplicate source provenance")
		}
		seen[key] = struct{}{}
		current, ok := eligible[key]
		if ok {
			var protected bool
			var err error
			current, protected, err = projectSourceWithReviewOrigin(ctx, promotionReviewQuery{writer}, current)
			if err != nil {
				return err
			}
			if protected && proposed.Evidence == "" {
				return ErrReviewInvalidSource
			}
		}
		if !ok || current.ID == "" || current.EventID != proposed.EventID || current.SessionID != proposed.SessionID ||
			current.ScopeKey != proposed.ScopeKey || current.EventPart != proposed.EventPart ||
			current.LocatorKind != proposed.LocatorKind || current.LocatorValue != proposed.LocatorValue ||
			current.EvidenceSHA256 != proposed.EvidenceSHA256 || current.Actor != proposed.Actor ||
			current.SourceType != proposed.SourceType || current.Authority != proposed.Authority ||
			current.ObservedAt != proposed.ObservedAt || (proposed.Evidence != "" && current.Evidence != proposed.Evidence) {
			return errors.New("Promotion source provenance changed")
		}
		if proposed.Eligibility != memory.EligibilityEligible {
			return errors.New("Promotion destination source eligibility changed")
		}
		if proposed.Create {
			if proposed.ID == current.ID || proposed.OperationID != proposal.OperationID {
				return errors.New("Promotion destination Source Link creation changed")
			}
			var existingIDCount, activeMatchCount int
			if err := writer.queryRowContext(ctx, `SELECT COUNT(*) FROM semantic_source_links WHERE source_link_id = ?`, proposed.ID).Scan(&existingIDCount); err != nil {
				return err
			}
			if err := writer.queryRowContext(ctx, `
				SELECT COUNT(*) FROM semantic_source_links AS links
				WHERE links.claim_id = ? AND links.event_id = ? AND links.event_part = ? AND links.locator_kind = ?
				  AND links.locator_value = ? AND links.evidence_sha256 = ?
				  AND (SELECT state FROM semantic_state_events
				       WHERE object_kind = 'source_link' AND object_id = links.source_link_id
				       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'eligible'
			`, proposal.DestinationClaim.ID, proposed.EventID, proposed.EventPart, proposed.LocatorKind,
				proposed.LocatorValue, proposed.EvidenceSHA256).Scan(&activeMatchCount); err != nil {
				return err
			}
			if existingIDCount != 0 || activeMatchCount != 0 {
				return errors.New("Promotion ignored an existing eligible destination Source Link")
			}
			continue
		}
		if proposal.DestinationClaimCreate {
			return errors.New("new promoted Claim requires new eligible provenance")
		}
		var destination memory.SemanticSource
		err := writer.queryRowContext(ctx, `
			SELECT links.source_link_id, links.created_operation_id, links.event_id, links.source_session_id,
			       links.source_scope_key, links.event_part, links.locator_kind, links.locator_value,
			       links.evidence_sha256, links.source_actor, links.source_type, links.authority,
			       links.observed_at, events.content
			FROM semantic_source_links AS links JOIN events ON events.id = links.event_id
			WHERE links.source_link_id = ? AND links.claim_id = ?
			  AND (SELECT state FROM semantic_state_events
			       WHERE object_kind = 'source_link' AND object_id = links.source_link_id
			       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'eligible'
		`, proposed.ID, proposal.DestinationClaim.ID).Scan(
			&destination.ID, &destination.OperationID, &destination.EventID, &destination.SessionID,
			&destination.ScopeKey, &destination.EventPart, &destination.LocatorKind, &destination.LocatorValue,
			&destination.EvidenceSHA256, &destination.Actor, &destination.SourceType, &destination.Authority,
			&destination.ObservedAt, &destination.Evidence,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("Promotion destination Source Link changed")
		}
		if err != nil {
			return err
		}
		destination, _, err = projectSourceWithReviewOrigin(ctx, promotionReviewQuery{writer}, destination)
		if err != nil {
			return err
		}
		if destination.ID != proposed.ID || destination.OperationID != proposed.OperationID ||
			promotionSourceKey(destination) != promotionSourceKey(proposed) ||
			destination.SessionID != proposed.SessionID || destination.ScopeKey != proposed.ScopeKey ||
			destination.Actor != proposed.Actor || destination.SourceType != proposed.SourceType ||
			destination.Authority != proposed.Authority || destination.ObservedAt != proposed.ObservedAt ||
			(proposed.Evidence != "" && destination.Evidence != proposed.Evidence) {
			return errors.New("Promotion destination Source Link changed")
		}
	}
	return nil
}

func promotionSourceKey(source memory.SemanticSource) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", source.EventID, source.EventPart,
		source.LocatorKind, source.LocatorValue, source.EvidenceSHA256)
}

func validatePromotionEntities(ctx context.Context, writer turnLeaseWriteExecutor, proposal memory.PromotionProposal) error {
	required := make(map[memory.SemanticID]struct{})
	for _, sourceID := range []memory.SemanticID{proposal.SourceClaim.SubjectEntityID, proposal.SourceClaim.Object.EntityID} {
		if sourceID == "" {
			continue
		}
		var scopeKey string
		if err := writer.queryRowContext(ctx, `
			SELECT scopes.scope_key FROM semantic_entities AS entities
			JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
			WHERE entities.entity_id = ?
		`, sourceID).Scan(&scopeKey); err != nil {
			return err
		}
		if scopeKey != "global" {
			required[sourceID] = struct{}{}
		}
	}
	if len(proposal.PromotedEntities) != len(required) {
		return errors.New("Promotion identity mappings do not match the source Claim")
	}
	mapped := make(map[memory.SemanticID]memory.SemanticID)
	for _, promoted := range proposal.PromotedEntities {
		if _, ok := required[promoted.SourceEntityID]; !ok {
			return errors.New("Promotion contains an unrelated identity mapping")
		}
		if _, duplicate := mapped[promoted.SourceEntityID]; duplicate {
			return errors.New("Promotion contains a duplicate identity mapping")
		}
		entity := promoted.DestinationEntity
		if entity.ScopeKey != proposal.DestinationScope.Key || entity.AnchorKind != "" {
			return errors.New("promoted Entity is outside the destination scope")
		}
		var sourceScope, sourceName, sourceType string
		if err := writer.queryRowContext(ctx, `
			SELECT scopes.scope_key, entities.canonical_name, entities.entity_type FROM semantic_entities AS entities
			JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
			WHERE entities.entity_id = ? AND entities.lifecycle = 'active'
			  AND COALESCE((SELECT state FROM semantic_state_events
			      WHERE object_kind = 'entity' AND object_id = entities.entity_id
			      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), entities.lifecycle) = 'active'
		`, promoted.SourceEntityID).Scan(&sourceScope, &sourceName, &sourceType); err != nil ||
			sourceScope != proposal.SourceScope.Key || entity.CanonicalName != sourceName || entity.EntityType != sourceType {
			return errors.New("Promotion source Entity changed")
		}
		if entity.Create {
			var existingMappings int
			if err := writer.queryRowContext(ctx, `
				SELECT COUNT(*) FROM semantic_promotion_entities AS mappings
				JOIN semantic_entities AS destination ON destination.entity_id = mappings.destination_entity_id
				WHERE mappings.source_entity_id = ? AND destination.scope_id = ?
				  AND COALESCE((SELECT state FROM semantic_state_events
				      WHERE object_kind = 'entity' AND object_id = destination.entity_id
				      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), destination.lifecycle) = 'active'
			`, promoted.SourceEntityID, proposal.DestinationScope.ID).Scan(&existingMappings); err != nil {
				return err
			}
			if existingMappings != 0 {
				return errors.New("Promotion ignored an existing broader identity mapping")
			}
		} else {
			var id memory.SemanticID
			var name, entityType string
			if err := writer.queryRowContext(ctx, `
				SELECT entities.entity_id, entities.canonical_name, entities.entity_type
				FROM semantic_promotion_entities AS mappings
				JOIN semantic_entities AS entities ON entities.entity_id = mappings.destination_entity_id
				JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
				WHERE mappings.source_entity_id = ? AND entities.entity_id = ? AND scopes.scope_key = ?
				  AND COALESCE((SELECT state FROM semantic_state_events
				      WHERE object_kind = 'entity' AND object_id = entities.entity_id
				      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), entities.lifecycle) = 'active'
			`, promoted.SourceEntityID, entity.ID, proposal.DestinationScope.Key).Scan(&id, &name, &entityType); err != nil ||
				id != entity.ID || name != sourceName || entityType != sourceType {
				return errors.New("Promotion destination Entity changed")
			}
		}
		mapped[promoted.SourceEntityID] = entity.ID
	}
	for _, sourceID := range []memory.SemanticID{proposal.SourceClaim.SubjectEntityID, proposal.SourceClaim.Object.EntityID} {
		if sourceID == "" {
			continue
		}
		var scopeKey string
		if err := writer.queryRowContext(ctx, `
			SELECT scopes.scope_key FROM semantic_entities AS entities JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
			WHERE entities.entity_id = ?
		`, sourceID).Scan(&scopeKey); err != nil {
			return err
		}
		want := sourceID
		if scopeKey != "global" {
			want = mapped[sourceID]
		}
		if sourceID == proposal.SourceClaim.SubjectEntityID && proposal.DestinationClaim.SubjectEntityID != want {
			return errors.New("Promotion subject identity mapping changed")
		}
		if sourceID == proposal.SourceClaim.Object.EntityID && proposal.DestinationClaim.Object.EntityID != want {
			return errors.New("Promotion object identity mapping changed")
		}
	}
	if proposal.DestinationClaim.ScopeKey != proposal.DestinationScope.Key ||
		proposal.DestinationClaim.Predicate != proposal.SourceClaim.Predicate ||
		proposal.DestinationClaim.Polarity != proposal.SourceClaim.Polarity ||
		!validTimesEqual(proposal.DestinationClaim.ValidTime, proposal.SourceClaim.ValidTime) ||
		!typedLiteralPointersEqual(proposal.DestinationClaim.Object.Literal, proposal.SourceClaim.Object.Literal) {
		return errors.New("Promotion destination proposition changed")
	}
	if !proposal.DestinationClaimCreate {
		current, err := loadSemanticClaimFromWriter(ctx, writer, proposal.DestinationClaim.ID)
		if err != nil || !semanticClaimsEqual(current, proposal.DestinationClaim) {
			return errors.New("Promotion destination Claim changed")
		}
		var state memory.SemanticStateValue
		if err := writer.queryRowContext(ctx, `
			SELECT state FROM semantic_state_events WHERE object_kind = 'claim' AND object_id = ?
			ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1
		`, proposal.DestinationClaim.ID).Scan(&state); err != nil || state != memory.SemanticStateActive {
			return errors.New("Promotion destination Claim is not active")
		}
	} else {
		var existing memory.SemanticID
		err := writer.queryRowContext(ctx, semanticClaimByPropositionQuery,
			proposal.DestinationScope.ID, proposal.DestinationClaim.SubjectEntityID, proposal.DestinationClaim.Predicate.ID,
			claimObjectKind(proposal.DestinationClaim.Object), nullableSemanticID(proposal.DestinationClaim.Object.EntityID),
			literalKindArgument(proposal.DestinationClaim.Object.Literal), literalValueArgument(proposal.DestinationClaim.Object.Literal),
			proposal.DestinationClaim.Polarity, semanticTimeArgument(proposal.DestinationClaim.ValidTime.From),
			semanticTimeArgument(proposal.DestinationClaim.ValidTime.To),
		).Scan(&existing)
		if err == nil {
			return errors.New("Promotion ignored an existing broader Claim")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}
