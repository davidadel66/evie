package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type canonicalLifecycleChange struct {
	Action        memory.MemoryLifecycleAction `json:"action"`
	ObjectKind    memory.SemanticObjectKind    `json:"object_kind"`
	ObjectID      memory.SemanticID            `json:"object_id"`
	ExpectedState memory.SemanticStateValue    `json:"expected_state"`
}

type canonicalLifecycleEffect struct {
	Scopes          []string                   `json:"scopes"`
	Predicates      []struct{}                 `json:"predicates"`
	Entities        []struct{}                 `json:"entities"`
	Aliases         []struct{}                 `json:"aliases"`
	Claims          []struct{}                 `json:"claims"`
	SourceLinks     []struct{}                 `json:"source_links"`
	GraphLinks      []struct{}                 `json:"graph_links"`
	Transitions     []canonicalTransition      `json:"transitions"`
	LifecycleChange []canonicalLifecycleChange `json:"lifecycle_changes"`
}

type canonicalLifecycleProposal struct {
	Kind           string                   `json:"kind"`
	IdempotencyKey string                   `json:"idempotency_key"`
	Actor          memory.SemanticActor     `json:"actor"`
	SessionID      memory.SessionID         `json:"session_id"`
	PriorRevisions []memory.ScopeRevision   `json:"prior_revisions"`
	SourceEventIDs []memory.EventID         `json:"source_event_ids"`
	Effect         canonicalLifecycleEffect `json:"effect"`
}

func canonicalMemoryLifecycleProposal(proposal memory.MemoryLifecycleProposal) canonicalLifecycleProposal {
	transitions := make([]canonicalTransition, len(proposal.Transitions))
	for i, transition := range proposal.Transitions {
		transitions[i] = canonicalTransition(transition)
	}
	writtenScopes := append([]string(nil), proposal.EffectScopes...)
	return canonicalLifecycleProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Evidence.EventID},
		Effect: canonicalLifecycleEffect{
			Scopes: writtenScopes, Predicates: []struct{}{}, Entities: []struct{}{},
			Aliases: []struct{}{}, Claims: []struct{}{}, SourceLinks: []struct{}{}, GraphLinks: []struct{}{},
			Transitions: transitions,
			LifecycleChange: []canonicalLifecycleChange{{
				Action: proposal.Request.Action, ObjectKind: proposal.ObjectKind,
				ObjectID: proposal.ObjectID, ExpectedState: proposal.ExpectedState,
			}},
		},
	}
}

type lifecycleQueryer interface {
	queryContext(context.Context, string, ...any) (*sql.Rows, error)
	queryRowContext(context.Context, string, ...any) rowScanner
}

type dbLifecycleQueryer struct{ db *sql.DB }

func (q dbLifecycleQueryer) queryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	return q.db.QueryContext(ctx, statement, args...)
}

func (q dbLifecycleQueryer) queryRowContext(ctx context.Context, statement string, args ...any) rowScanner {
	return q.db.QueryRowContext(ctx, statement, args...)
}

func lifecycleKind(action memory.MemoryLifecycleAction) (string, error) {
	switch action {
	case memory.LifecycleRetire:
		return "retire_memory", nil
	case memory.LifecycleRestore:
		return "restore_memory", nil
	case memory.LifecycleRetractSource:
		return "retract_source", nil
	case memory.LifecycleRestoreSource:
		return "restore_source", nil
	default:
		return "", fmt.Errorf("unsupported memory lifecycle action %q", action)
	}
}

func validateLifecycleActionTarget(request memory.MemoryLifecycleRequest) error {
	if err := validateSemanticUUID(string(request.ObjectID)); err != nil {
		return err
	}
	switch request.Action {
	case memory.LifecycleRetire, memory.LifecycleRestore:
		if request.ObjectKind != memory.SemanticObjectEntity && request.ObjectKind != memory.SemanticObjectAlias &&
			request.ObjectKind != memory.SemanticObjectClaim && request.ObjectKind != memory.SemanticObjectGraphLink {
			return errors.New("retirement and restoration require an Entity, Alias, Claim, or Graph Link")
		}
	case memory.LifecycleRetractSource, memory.LifecycleRestoreSource:
		if request.ObjectKind != memory.SemanticObjectSourceLink {
			return errors.New("source lifecycle requires a Source Link")
		}
	default:
		return fmt.Errorf("unsupported memory lifecycle action %q", request.Action)
	}
	return nil
}

func loadLifecycleTargetScope(ctx context.Context, query lifecycleQueryer, kind memory.SemanticObjectKind, id memory.SemanticID) (memory.SemanticScope, error) {
	var scope memory.SemanticScope
	var registry sql.NullString
	var err error
	switch kind {
	case memory.SemanticObjectEntity:
		err = query.queryRowContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, scopes.registry_id, scopes.revision
			FROM semantic_entities AS objects JOIN semantic_scopes AS scopes ON scopes.scope_id = objects.scope_id
			WHERE objects.entity_id = ?
		`, id).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision)
	case memory.SemanticObjectAlias:
		err = query.queryRowContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, scopes.registry_id, scopes.revision
			FROM semantic_aliases AS objects JOIN semantic_scopes AS scopes ON scopes.scope_id = objects.scope_id
			WHERE objects.alias_id = ?
		`, id).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision)
	case memory.SemanticObjectClaim:
		err = query.queryRowContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, scopes.registry_id, scopes.revision
			FROM semantic_claims AS objects JOIN semantic_scopes AS scopes ON scopes.scope_id = objects.scope_id
			WHERE objects.claim_id = ?
		`, id).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision)
	case memory.SemanticObjectSourceLink:
		err = query.queryRowContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, scopes.registry_id, scopes.revision
			FROM semantic_source_links AS sources
			JOIN semantic_claims AS claims ON claims.claim_id = sources.claim_id
			JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
			WHERE sources.source_link_id = ?
		`, id).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision)
	case memory.SemanticObjectGraphLink:
		err = query.queryRowContext(ctx, `
			SELECT scopes.scope_id, scopes.scope_key, scopes.registry_id, scopes.revision
			FROM semantic_graph_links AS objects JOIN semantic_scopes AS scopes ON scopes.scope_id = objects.scope_id
			WHERE objects.graph_link_id = ?
		`, id).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision)
	default:
		return scope, fmt.Errorf("unsupported semantic object kind %q", kind)
	}
	if err != nil {
		return scope, fmt.Errorf("load %s lifecycle target: %w", kind, err)
	}
	if registry.Valid {
		scope.RegistryID = registry.String
	}
	return scope, nil
}

func loadRegisteredLifecycleScope(ctx context.Context, query lifecycleQueryer, key string) (memory.SemanticScope, error) {
	var scope memory.SemanticScope
	var registry sql.NullString
	if err := query.queryRowContext(ctx, `
		SELECT scope_id, scope_key, registry_id, revision FROM semantic_scopes WHERE scope_key = ?
	`, key).Scan(&scope.ID, &scope.Key, &registry, &scope.Revision); err != nil {
		return scope, fmt.Errorf("load registered semantic scope %q: %w", key, err)
	}
	if registry.Valid {
		scope.RegistryID = registry.String
	}
	return scope, nil
}

func lifecycleTransitionScopes(ctx context.Context, query lifecycleQueryer, baseKeys []string, transitions []memory.SemanticTransition) ([]memory.SemanticScope, map[string]struct{}, error) {
	keys := make(map[string]struct{}, len(baseKeys)+len(transitions))
	written := make(map[string]struct{}, len(transitions))
	for _, key := range baseKeys {
		keys[key] = struct{}{}
	}
	for _, transition := range transitions {
		scope, err := loadLifecycleTargetScope(ctx, query, memory.SemanticObjectKind(transition.ObjectKind), transition.ObjectID)
		if err != nil {
			return nil, nil, err
		}
		keys[scope.Key] = struct{}{}
		written[scope.Key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	scopes := make([]memory.SemanticScope, len(ordered))
	for i, key := range ordered {
		scope, err := loadRegisteredLifecycleScope(ctx, query, key)
		if err != nil {
			return nil, nil, err
		}
		scopes[i] = scope
	}
	return scopes, written, nil
}

func semanticScopeKeys(scopes []memory.SemanticScope) []string {
	keys := make([]string, len(scopes))
	for i, scope := range scopes {
		keys[i] = scope.Key
	}
	return keys
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validateLifecycleTransitionAuthority(ctx context.Context, query lifecycleQueryer, allowed map[string]struct{}, transitions []memory.SemanticTransition) error {
	for _, transition := range transitions {
		scope, err := loadLifecycleTargetScope(ctx, query, memory.SemanticObjectKind(transition.ObjectKind), transition.ObjectID)
		if err != nil {
			return err
		}
		if _, ok := allowed[scope.Key]; !ok {
			return errors.New("lifecycle dependencies include an object outside the session's authorized scope")
		}
	}
	return nil
}

func loadLatestState(ctx context.Context, query lifecycleQueryer, kind memory.SemanticObjectKind, id memory.SemanticID) (memory.SemanticState, error) {
	var state memory.SemanticState
	var transactionTime string
	err := query.queryRowContext(ctx, `
		SELECT state, operation_id, scope_revision, transaction_time
		FROM semantic_state_events WHERE object_kind = ? AND object_id = ?
		ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1
	`, kind, id).Scan(&state.State, &state.OperationID, &state.ScopeRevision, &transactionTime)
	if err != nil {
		return state, fmt.Errorf("load latest %s lifecycle: %w", kind, err)
	}
	state.TransactionTime, err = parseSemanticTime(transactionTime)
	return state, err
}

func lifecycleIDs(ctx context.Context, query lifecycleQueryer, statement string, args ...any) ([]memory.SemanticID, error) {
	rows, err := query.queryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func currentEntityDependents(ctx context.Context, query lifecycleQueryer, entityID memory.SemanticID) ([]memory.SemanticTransition, error) {
	aliases, err := lifecycleIDs(ctx, query, `
		SELECT aliases.alias_id FROM semantic_aliases AS aliases
		WHERE aliases.entity_id = ?
		  AND (SELECT state FROM semantic_state_events
		       WHERE object_kind = 'alias' AND object_id = aliases.alias_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'active'
		ORDER BY aliases.alias_id
	`, entityID)
	if err != nil {
		return nil, err
	}
	claims, err := lifecycleIDs(ctx, query, `
		SELECT claims.claim_id FROM semantic_claims AS claims
		WHERE (claims.subject_entity_id = ? OR (claims.object_kind = 'entity' AND claims.object_entity_id = ?))
		  AND (SELECT state FROM semantic_state_events
		       WHERE object_kind = 'claim' AND object_id = claims.claim_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'active'
		ORDER BY claims.claim_id
	`, entityID, entityID)
	if err != nil {
		return nil, err
	}
	graphLinks, err := lifecycleIDs(ctx, query, `
		SELECT links.graph_link_id FROM semantic_graph_links AS links
		WHERE (
		  (links.source_kind = 'entity' AND links.source_id = ?) OR
		  (links.target_kind = 'entity' AND links.target_id = ?) OR
		  (links.source_kind = 'alias' AND EXISTS (SELECT 1 FROM semantic_aliases WHERE alias_id = links.source_id AND entity_id = ?)) OR
		  (links.target_kind = 'alias' AND EXISTS (SELECT 1 FROM semantic_aliases WHERE alias_id = links.target_id AND entity_id = ?)) OR
		  (links.source_kind = 'claim' AND EXISTS (SELECT 1 FROM semantic_claims WHERE claim_id = links.source_id AND (subject_entity_id = ? OR object_entity_id = ?))) OR
		  (links.target_kind = 'claim' AND EXISTS (SELECT 1 FROM semantic_claims WHERE claim_id = links.target_id AND (subject_entity_id = ? OR object_entity_id = ?))) OR
		  (links.source_kind = 'source_link' AND EXISTS (SELECT 1 FROM semantic_source_links AS sources JOIN semantic_claims AS claims ON claims.claim_id = sources.claim_id WHERE sources.source_link_id = links.source_id AND (claims.subject_entity_id = ? OR claims.object_entity_id = ?))) OR
		  (links.target_kind = 'source_link' AND EXISTS (SELECT 1 FROM semantic_source_links AS sources JOIN semantic_claims AS claims ON claims.claim_id = sources.claim_id WHERE sources.source_link_id = links.target_id AND (claims.subject_entity_id = ? OR claims.object_entity_id = ?)))
		)
		  AND (SELECT state FROM semantic_state_events
		       WHERE object_kind = 'graph_link' AND object_id = links.graph_link_id
		       ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'active'
		ORDER BY links.graph_link_id
	`, entityID, entityID, entityID, entityID, entityID, entityID, entityID, entityID, entityID, entityID, entityID, entityID)
	if err != nil {
		return nil, err
	}
	var transitions []memory.SemanticTransition
	for _, aliasID := range aliases {
		state, err := loadLatestState(ctx, query, memory.SemanticObjectAlias, aliasID)
		if err != nil {
			return nil, err
		}
		if state.State == memory.SemanticStateActive {
			transitions = append(transitions, memory.SemanticTransition{ObjectKind: "alias", ObjectID: aliasID, State: memory.SemanticStateRetired})
		}
	}
	for _, claimID := range claims {
		state, err := loadLatestState(ctx, query, memory.SemanticObjectClaim, claimID)
		if err != nil {
			return nil, err
		}
		if state.State == memory.SemanticStateActive {
			transitions = append(transitions, memory.SemanticTransition{ObjectKind: "claim", ObjectID: claimID, State: memory.SemanticStateRetired})
		}
	}
	for _, linkID := range graphLinks {
		transitions = append(transitions, memory.SemanticTransition{ObjectKind: "graph_link", ObjectID: linkID, State: memory.SemanticStateRetired})
	}
	return transitions, nil
}

func retiredEntityOperationTransitions(ctx context.Context, query lifecycleQueryer, target memory.SemanticID, retirement memory.SemanticState) ([]memory.SemanticTransition, error) {
	rows, err := query.queryContext(ctx, `
		SELECT object_kind, object_id FROM semantic_state_events
		WHERE operation_id = ? AND state = 'retired' AND object_kind IN ('entity', 'alias', 'claim', 'graph_link')
		ORDER BY CASE object_kind WHEN 'entity' THEN 0 WHEN 'alias' THEN 1 WHEN 'claim' THEN 2 ELSE 3 END, object_id
	`, retirement.OperationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var transitions []memory.SemanticTransition
	for rows.Next() {
		var kind string
		var id memory.SemanticID
		if err := rows.Scan(&kind, &id); err != nil {
			return nil, err
		}
		state, err := loadLatestState(ctx, query, memory.SemanticObjectKind(kind), id)
		if err != nil || state.State != memory.SemanticStateRetired || state.OperationID != retirement.OperationID {
			return nil, errors.New("Entity restoration no longer names the latest eligible retired state")
		}
		transitions = append(transitions, memory.SemanticTransition{ObjectKind: kind, ObjectID: id, State: memory.SemanticStateActive})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(transitions) == 0 || transitions[0].ObjectKind != "entity" || transitions[0].ObjectID != target {
		return nil, errors.New("Entity retirement operation does not identify the restoration target")
	}
	return transitions, nil
}

func requireActiveEntity(ctx context.Context, query lifecycleQueryer, id memory.SemanticID, restoring map[memory.SemanticID]struct{}) error {
	if _, ok := restoring[id]; ok {
		return nil
	}
	var state memory.SemanticStateValue
	err := query.queryRowContext(ctx, `
		SELECT COALESCE((SELECT state FROM semantic_state_events
		                 WHERE object_kind = 'entity' AND object_id = entities.entity_id
		                 ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1),
		                entities.lifecycle)
		FROM semantic_entities AS entities WHERE entities.entity_id = ?
	`, id).Scan(&state)
	if err != nil || state != memory.SemanticStateActive {
		return errors.New("lifecycle restoration has an incompatible Entity dependency")
	}
	return nil
}

func validateRestorationDependencies(ctx context.Context, query lifecycleQueryer, transitions []memory.SemanticTransition) error {
	restoringEntities := make(map[memory.SemanticID]struct{})
	restoringObjects := make(map[semanticNodeKey]struct{})
	for _, transition := range transitions {
		restoringObjects[semanticNodeKey{Kind: memory.SemanticObjectKind(transition.ObjectKind), ID: transition.ObjectID}] = struct{}{}
		if transition.ObjectKind == "entity" {
			restoringEntities[transition.ObjectID] = struct{}{}
		}
	}
	for _, transition := range transitions {
		switch transition.ObjectKind {
		case "alias":
			var entityID memory.SemanticID
			if err := query.queryRowContext(ctx, `SELECT entity_id FROM semantic_aliases WHERE alias_id = ?`, transition.ObjectID).Scan(&entityID); err != nil {
				return err
			}
			if err := requireActiveEntity(ctx, query, entityID, restoringEntities); err != nil {
				return err
			}
		case "claim":
			var subject memory.SemanticID
			var object sql.NullString
			if err := query.queryRowContext(ctx, `SELECT subject_entity_id, object_entity_id FROM semantic_claims WHERE claim_id = ?`, transition.ObjectID).Scan(&subject, &object); err != nil {
				return err
			}
			if err := requireActiveEntity(ctx, query, subject, restoringEntities); err != nil {
				return err
			}
			if object.Valid {
				if err := requireActiveEntity(ctx, query, memory.SemanticID(object.String), restoringEntities); err != nil {
					return err
				}
			}
		case "graph_link":
			var sourceKind, targetKind memory.SemanticObjectKind
			var sourceID, targetID memory.SemanticID
			if err := query.queryRowContext(ctx, `SELECT source_kind, source_id, target_kind, target_id FROM semantic_graph_links WHERE graph_link_id = ?`, transition.ObjectID).Scan(&sourceKind, &sourceID, &targetKind, &targetID); err != nil {
				return err
			}
			for _, endpoint := range []memory.GraphEndpoint{{Kind: sourceKind, ID: sourceID}, {Kind: targetKind, ID: targetID}} {
				if endpoint.Kind == memory.SemanticObjectEntity {
					if err := requireActiveEntity(ctx, query, endpoint.ID, restoringEntities); err != nil {
						return err
					}
					continue
				}
				state, err := loadLatestState(ctx, query, endpoint.Kind, endpoint.ID)
				if err != nil {
					return err
				}
				want := memory.SemanticStateActive
				if endpoint.Kind == memory.SemanticObjectSourceLink {
					want = memory.SemanticStateEligible
				}
				_, restoring := restoringObjects[semanticNodeKey(endpoint)]
				if state.State != want && !restoring {
					return errors.New("Graph Link restoration has an incompatible endpoint dependency")
				}
				if endpoint.Kind == memory.SemanticObjectClaim {
					var eligible int
					if err := query.queryRowContext(ctx, `
							SELECT COUNT(*) FROM semantic_source_links AS sources
							WHERE sources.claim_id = ? AND (SELECT state FROM semantic_state_events
							 WHERE object_kind = 'source_link' AND object_id = sources.source_link_id
							 ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'eligible'
						`, endpoint.ID).Scan(&eligible); err != nil {
						return err
					}
					if eligible == 0 {
						return errors.New("Graph Link restoration has an unsupported Claim endpoint")
					}
				}
			}
		}
	}
	return nil
}

func deriveLifecycleTransitions(ctx context.Context, query lifecycleQueryer, request memory.MemoryLifecycleRequest) (memory.SemanticStateValue, []memory.SemanticTransition, error) {
	latest, err := loadLatestState(ctx, query, request.ObjectKind, request.ObjectID)
	if err != nil {
		return "", nil, err
	}
	switch request.Action {
	case memory.LifecycleRetire:
		if latest.State != memory.SemanticStateActive {
			return "", nil, errors.New("only the latest active state can be retired")
		}
		if request.ObjectKind == memory.SemanticObjectEntity {
			var anchor sql.NullString
			if err := query.queryRowContext(ctx, `SELECT anchor_kind FROM semantic_entities WHERE entity_id = ?`, request.ObjectID).Scan(&anchor); err != nil {
				return "", nil, err
			}
			if anchor.Valid {
				return "", nil, errors.New("canonical anchor Entities are not eligible for retirement")
			}
		}
		transitions := []memory.SemanticTransition{{ObjectKind: string(request.ObjectKind), ObjectID: request.ObjectID, State: memory.SemanticStateRetired}}
		if request.ObjectKind == memory.SemanticObjectEntity {
			dependents, err := currentEntityDependents(ctx, query, request.ObjectID)
			if err != nil {
				return "", nil, err
			}
			transitions = append(transitions, dependents...)
		}
		return latest.State, transitions, nil
	case memory.LifecycleRestore:
		if latest.State != memory.SemanticStateRetired {
			if latest.State == memory.SemanticStateSuperseded {
				return "", nil, errors.New("superseded memory cannot be restored")
			}
			return "", nil, errors.New("only the latest retired state can be restored")
		}
		transitions := []memory.SemanticTransition{{ObjectKind: string(request.ObjectKind), ObjectID: request.ObjectID, State: memory.SemanticStateActive}}
		if request.ObjectKind == memory.SemanticObjectEntity {
			transitions, err = retiredEntityOperationTransitions(ctx, query, request.ObjectID, latest)
			if err != nil {
				return "", nil, err
			}
		}
		if err := validateRestorationDependencies(ctx, query, transitions); err != nil {
			return "", nil, err
		}
		return latest.State, transitions, nil
	case memory.LifecycleRetractSource:
		if latest.State != memory.SemanticStateEligible {
			return "", nil, errors.New("only the latest eligible Source Link can be retracted")
		}
		if err := requireSourceClaimActive(ctx, query, request.ObjectID); err != nil {
			return "", nil, err
		}
		return latest.State, []memory.SemanticTransition{{ObjectKind: "source_link", ObjectID: request.ObjectID, State: memory.SemanticStateRetracted}}, nil
	case memory.LifecycleRestoreSource:
		if latest.State != memory.SemanticStateRetracted {
			return "", nil, errors.New("only the latest retracted Source Link can be restored")
		}
		if err := requireSourceClaimActive(ctx, query, request.ObjectID); err != nil {
			return "", nil, err
		}
		return latest.State, []memory.SemanticTransition{{ObjectKind: "source_link", ObjectID: request.ObjectID, State: memory.SemanticStateEligible}}, nil
	default:
		return "", nil, fmt.Errorf("unsupported memory lifecycle action %q", request.Action)
	}
}

func requireSourceClaimActive(ctx context.Context, query lifecycleQueryer, sourceID memory.SemanticID) error {
	var claimID memory.SemanticID
	if err := query.queryRowContext(ctx, `SELECT claim_id FROM semantic_source_links WHERE source_link_id = ?`, sourceID).Scan(&claimID); err != nil {
		return err
	}
	state, err := loadLatestState(ctx, query, memory.SemanticObjectClaim, claimID)
	if err != nil || state.State != memory.SemanticStateActive {
		return errors.New("Source Link Claim is not active and eligible for source restoration")
	}
	return nil
}

func loadLifecycleEvidence(row rowScanner, sessionID memory.SessionID, eventID memory.EventID, scopeKey string) (memory.SemanticOperationEvidence, error) {
	var evidence memory.SemanticOperationEvidence
	var eventSession, eventType, role, content, recordedAt string
	if err := row.Scan(&eventSession, &eventType, &role, &content, &recordedAt); err != nil {
		return evidence, fmt.Errorf("load lifecycle evidence: %w", err)
	}
	if eventSession != string(sessionID) || eventType != string(memory.EventUserMessage) || role != string(memory.RoleUser) {
		return evidence, errors.New("lifecycle evidence must be an owner user message in the bound session")
	}
	observed, err := time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return evidence, err
	}
	digest := sha256.Sum256([]byte(content))
	return memory.SemanticOperationEvidence{
		EventID: eventID, SessionID: sessionID, ScopeKey: scopeKey, Actor: memory.SemanticActorOwner,
		SourceType: memory.SourceTypeUserMessage, ObservedAt: formatSemanticTime(observed),
		EvidenceSHA256: fmt.Sprintf("sha256:%x", digest), Evidence: content,
	}, nil
}

func lifecycleRequestsEqual(left, right memory.MemoryLifecycleRequest) bool { return left == right }

func lifecycleIncludesGraphLink(transitions []memory.SemanticTransition) bool {
	for _, transition := range transitions {
		if transition.ObjectKind == string(memory.SemanticObjectGraphLink) {
			return true
		}
	}
	return false
}

// PrepareMemoryLifecycle expands and freezes one complete lifecycle operation
// without changing Semantic Memory.
func (s *Store) PrepareMemoryLifecycle(ctx context.Context, scope memory.ScopeContext, request memory.MemoryLifecycleRequest) (memory.MemoryLifecycleProposal, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	if request.UseSessionScope {
		if err := requireSemanticScopeKeysAvailable(ctx, s.db, []string{"session:" + string(scope.SessionID)}); err != nil {
			return memory.MemoryLifecycleProposal{}, err
		}
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.MemoryLifecycleProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if err := validateLifecycleActionTarget(request); err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	var priorJSON, priorHash string
	err := s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&priorJSON, &priorHash)
	if err == nil {
		var proposal memory.MemoryLifecycleProposal
		if err := json.Unmarshal([]byte(priorJSON), &proposal); err != nil {
			return proposal, err
		}
		if !lifecycleRequestsEqual(proposal.Request, request) || proposal.SessionID != scope.SessionID {
			return memory.MemoryLifecycleProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		return proposal, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.MemoryLifecycleProposal{}, err
	}
	query := dbLifecycleQueryer{s.db}
	target, err := loadLifecycleTargetScope(ctx, query, request.ObjectKind, request.ObjectID)
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	targetKey := targetScopeKey(scope, request.UseSessionScope)
	if target.Key != targetKey {
		return memory.MemoryLifecycleProposal{}, errors.New("lifecycle target is outside the session-bound scope")
	}
	expectedState, transitions, err := deriveLifecycleTransitions(ctx, query, request)
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	contextKey := scopeKeyForContext(scope)
	allowedScopes := map[string]struct{}{
		"global":                             {},
		contextKey:                           {},
		"session:" + string(scope.SessionID): {},
	}
	if err := validateLifecycleTransitionAuthority(ctx, query, allowedScopes, transitions); err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	evidence, err := loadLifecycleEvidence(s.db.QueryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
	`, request.SourceEventID), scope.SessionID, request.SourceEventID, target.Key)
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	baseSet := map[string]struct{}{"global": {}}
	baseSet[contextKey] = struct{}{}
	baseSet[targetKey] = struct{}{}
	baseKeys := sortedStringSet(baseSet)
	scopes, writtenScopes, err := lifecycleTransitionScopes(ctx, query, baseKeys, transitions)
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	priors := make([]memory.ScopeRevision, len(scopes))
	for i, semanticScope := range scopes {
		priors[i] = memory.ScopeRevision{ScopeKey: semanticScope.Key, Revision: semanticScope.Revision}
	}
	operationID, err := newSemanticID()
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	kind, err := lifecycleKind(request.Action)
	if err != nil {
		return memory.MemoryLifecycleProposal{}, err
	}
	schemaVersion := 3
	if lifecycleIncludesGraphLink(transitions) {
		schemaVersion = 5
	}
	proposal := memory.MemoryLifecycleProposal{
		SchemaVersion: schemaVersion, Kind: kind, OperationID: operationID, IdempotencyKey: request.IdempotencyKey,
		Actor: memory.SemanticActorOwner, SessionID: scope.SessionID, Scope: target, Scopes: scopes,
		PriorRevisions: priors, ObjectKind: request.ObjectKind, ObjectID: request.ObjectID,
		ExpectedState: expectedState, Transitions: transitions, EffectScopes: sortedStringSet(writtenScopes),
		Evidence: evidence, Request: request,
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalMemoryLifecycleProposal(proposal))
	if err == nil {
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
	}
	return proposal, err
}

func validateLifecycleProposal(proposal memory.MemoryLifecycleProposal) error {
	kind, err := lifecycleKind(proposal.Request.Action)
	wantVersion := 3
	if lifecycleIncludesGraphLink(proposal.Transitions) {
		wantVersion = 5
	}
	if err != nil || proposal.SchemaVersion != wantVersion || proposal.Kind != kind || proposal.Actor != memory.SemanticActorOwner ||
		proposal.IdempotencyKey != proposal.Request.IdempotencyKey || proposal.ObjectKind != proposal.Request.ObjectKind ||
		proposal.ObjectID != proposal.Request.ObjectID || proposal.Evidence.EventID != proposal.Request.SourceEventID ||
		proposal.Evidence.SessionID != proposal.SessionID || proposal.Evidence.ScopeKey != proposal.Scope.Key ||
		proposal.Evidence.Actor != memory.SemanticActorOwner || proposal.Evidence.SourceType != memory.SourceTypeUserMessage {
		return errors.New("invalid memory lifecycle proposal")
	}
	if err := validateLifecycleActionTarget(proposal.Request); err != nil {
		return err
	}
	if len(proposal.Transitions) == 0 || proposal.Transitions[0].ObjectKind != string(proposal.ObjectKind) ||
		proposal.Transitions[0].ObjectID != proposal.ObjectID {
		return errors.New("memory lifecycle proposal omits its target transition")
	}
	if len(proposal.EffectScopes) == 0 {
		return errors.New("memory lifecycle proposal omits its written scopes")
	}
	for i, key := range proposal.EffectScopes {
		if key == "" || (i > 0 && proposal.EffectScopes[i-1] >= key) {
			return errors.New("memory lifecycle proposal written scopes are not canonical")
		}
	}
	return nil
}

func transitionsEqual(left, right []memory.SemanticTransition) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// ApplyMemoryLifecycle revalidates the complete approved effect and appends its
// operation, transitions, evidence reference, and Scope Revision atomically.
func (s *Store) ApplyMemoryLifecycle(ctx context.Context, lease memory.TurnLease, proposal memory.MemoryLifecycleProposal) (result memory.MemoryLifecycleResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("memory lifecycle proposal does not match its turn lease")
	}
	canonical := canonicalMemoryLifecycleProposal(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if err := validateLifecycleProposal(proposal); err != nil {
		return result, err
	}
	if proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != proposalHash ||
		proposal.PreparedSHA256 == "" || proposal.PreparedSHA256 != preparedHash {
		return result, errors.New("memory lifecycle proposal hash changed")
	}
	for _, id := range []memory.SemanticID{proposal.OperationID, proposal.ObjectID, proposal.Scope.ID} {
		if err := validateSemanticUUID(string(id)); err != nil {
			return result, err
		}
	}
	for _, semanticScope := range proposal.Scopes {
		if err := validateSemanticUUID(string(semanticScope.ID)); err != nil {
			return result, err
		}
	}
	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var acceptedHash, acceptedResult string
		existingErr := writer.queryRowContext(ctx, `SELECT proposal_sha256, result_json FROM semantic_operations WHERE idempotency_key = ?`, proposal.IdempotencyKey).Scan(&acceptedHash, &acceptedResult)
		if existingErr == nil {
			if acceptedHash != proposalHash {
				return ErrIdempotencyConflict
			}
			return json.Unmarshal([]byte(acceptedResult), &result)
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		if proposal.SchemaVersion == 5 {
			if err := validateExactSemanticApproval(ctx, writer, proposal.SessionID, proposal.OperationID, proposal.Evidence.EventID, proposalHash, preparedHash); err != nil {
				return err
			}
		}
		expectedTarget, baseScopeKeys, err := authorizedSemanticScopes(ctx, writer, proposal.SessionID, proposal.Request.UseSessionScope)
		if err != nil {
			return err
		}
		if proposal.Scope.Key != expectedTarget {
			return errors.New("memory lifecycle proposal is outside its immutable session Context Scope")
		}
		evidence, err := loadLifecycleEvidence(writer.queryRowContext(ctx, `
			SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
		`, proposal.Evidence.EventID), proposal.SessionID, proposal.Evidence.EventID, proposal.Scope.Key)
		if err != nil || evidence != proposal.Evidence {
			return errors.New("memory lifecycle evidence changed")
		}
		targetScope, err := loadLifecycleTargetScope(ctx, writer, proposal.ObjectKind, proposal.ObjectID)
		if err != nil || targetScope.ID != proposal.Scope.ID || targetScope.Key != proposal.Scope.Key ||
			targetScope.RegistryID != proposal.Scope.RegistryID {
			return errors.New("memory lifecycle target changed after preparation")
		}
		expectedState, transitions, err := deriveLifecycleTransitions(ctx, writer, proposal.Request)
		if err != nil || expectedState != proposal.ExpectedState || !transitionsEqual(transitions, proposal.Transitions) {
			return errors.New("memory lifecycle dependencies or latest state changed after preparation")
		}
		allowedScopes := make(map[string]struct{}, len(baseScopeKeys)+1)
		for _, key := range baseScopeKeys {
			allowedScopes[key] = struct{}{}
		}
		allowedScopes["session:"+string(proposal.SessionID)] = struct{}{}
		if err := validateLifecycleTransitionAuthority(ctx, writer, allowedScopes, transitions); err != nil {
			return err
		}
		expectedScopes, writtenScopes, err := lifecycleTransitionScopes(ctx, writer, baseScopeKeys, transitions)
		if err != nil {
			return err
		}
		if !stringSlicesEqual(semanticScopeKeys(expectedScopes), semanticScopeKeys(proposal.Scopes)) {
			return errors.New("memory lifecycle proposal contains an unauthorized scope")
		}
		if !stringSlicesEqual(sortedStringSet(writtenScopes), proposal.EffectScopes) {
			return errors.New("memory lifecycle proposal written scopes changed after preparation")
		}
		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions, s.now())
		if err != nil {
			return err
		}
		if target := byKey[proposal.Scope.Key]; target != proposal.Scope {
			return errors.New("memory lifecycle target scope changed")
		}
		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		result = memory.MemoryLifecycleResult{
			OperationID: proposal.OperationID, ObjectKind: proposal.ObjectKind, ObjectID: proposal.ObjectID,
			TransactionTime: now, ScopeRevision: proposal.Scope.Revision + 1,
		}
		result.ResultingRevisions = make([]memory.ScopeRevision, len(proposal.Scopes))
		for i, semanticScope := range proposal.Scopes {
			revision := semanticScope.Revision
			if _, written := writtenScopes[semanticScope.Key]; written {
				revision++
			}
			result.ResultingRevisions[i] = memory.ScopeRevision{ScopeKey: semanticScope.Key, Revision: revision}
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
			TargetScopeID: proposal.Scope.ID, SourceEventID: proposal.Evidence.EventID,
			ProposalHash: proposalHash, EffectHash: effectHash, ProposalJSON: proposalJSON,
			PreparedJSON: preparedJSON, ResultJSON: resultJSON, TransactionTime: now,
			ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey,
		}); err != nil {
			return err
		}
		for _, transition := range proposal.Transitions {
			transitionScope, err := loadLifecycleTargetScope(ctx, writer, memory.SemanticObjectKind(transition.ObjectKind), transition.ObjectID)
			if err != nil {
				return err
			}
			resultingRevision := byKey[transitionScope.Key].Revision + 1
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, transitionScope.ID, transition.ObjectKind, transition.ObjectID, transition.State,
				proposal.OperationID, resultingRevision, formatSemanticTime(now)); err != nil {
				return err
			}
		}
		for _, semanticScope := range proposal.Scopes {
			if _, written := writtenScopes[semanticScope.Key]; !written {
				continue
			}
			updated, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`,
				semanticScope.Revision+1, semanticScope.ID, semanticScope.Revision)
			if err != nil {
				return err
			}
			if changed, _ := updated.RowsAffected(); changed != 1 {
				return ErrStaleScopeRevision
			}
		}
		return nil
	})
	return result, err
}

func loadSourceForInspection(ctx context.Context, queryer semanticInspectionQueryer, id memory.SemanticID) (memory.SemanticSource, error) {
	var source memory.SemanticSource
	err := queryer.QueryRowContext(ctx, `
		SELECT sources.source_link_id, sources.created_operation_id, sources.event_id,
		       sources.source_session_id, sources.source_scope_key, sources.event_part,
		       sources.locator_kind, sources.locator_value, sources.evidence_sha256,
		       sources.source_actor, sources.source_type, sources.authority, sources.observed_at,
		       sources.eligibility, events.content
		FROM semantic_source_links AS sources JOIN events ON events.id = sources.event_id
		WHERE sources.source_link_id = ?
	`, id).Scan(&source.ID, &source.OperationID, &source.EventID, &source.SessionID, &source.ScopeKey,
		&source.EventPart, &source.LocatorKind, &source.LocatorValue, &source.EvidenceSHA256,
		&source.Actor, &source.SourceType, &source.Authority, &source.ObservedAt,
		&source.Eligibility, &source.Evidence)
	return source, err
}

func loadAllSourceInspections(ctx context.Context, queryer semanticInspectionQueryer, claimID memory.SemanticID, at time.Time) ([]memory.SemanticSourceInspection, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT source_link_id FROM semantic_source_links WHERE claim_id = ? ORDER BY source_link_id`, claimID)
	if err != nil {
		return nil, err
	}
	var ids []memory.SemanticID
	for rows.Next() {
		var id memory.SemanticID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]memory.SemanticSourceInspection, 0, len(ids))
	for _, id := range ids {
		source, err := loadSourceForInspection(ctx, queryer, id)
		if err != nil {
			return nil, err
		}
		lifecycle, err := loadSemanticLifecycleAt(ctx, queryer, "source_link", id, at)
		if err != nil {
			return nil, err
		}
		if len(lifecycle) == 0 {
			continue
		}
		if len(lifecycle) != 0 && lifecycle[len(lifecycle)-1].State == memory.SemanticStateRetracted {
			source.Eligibility = memory.EligibilityRetracted
		} else {
			source.Eligibility = memory.EligibilityEligible
		}
		result = append(result, memory.SemanticSourceInspection{Source: source, Lifecycle: lifecycle})
	}
	return result, nil
}

func redactSourceInspections(sources []memory.SemanticSourceInspection, allowedScopes map[string]struct{}) {
	for index := range sources {
		if _, allowed := allowedScopes[sources[index].Source.ScopeKey]; !allowed {
			sources[index].Source.Evidence = ""
		}
	}
}

func loadSemanticOperationInspection(ctx context.Context, queryer semanticInspectionQueryer, id memory.SemanticID) (memory.SemanticOperationInspection, error) {
	var operation memory.SemanticOperationInspection
	var transactionTime string
	err := queryer.QueryRowContext(ctx, `
		SELECT operation_id, schema_version, operation_kind, source_event_id,
		       proposal_sha256, effect_sha256, proposal_json, prepared_proposal_json,
		       result_json, transaction_time
		FROM semantic_operations WHERE operation_id = ?
	`, id).Scan(&operation.OperationID, &operation.SchemaVersion, &operation.Kind, &operation.SourceEventID,
		&operation.ProposalSHA256, &operation.EffectSHA256, &operation.ProposalJSON, &operation.PreparedJSON,
		&operation.ResultJSON, &transactionTime)
	if err != nil {
		return operation, err
	}
	operation.TransactionTime, err = parseSemanticTime(transactionTime)
	if err != nil {
		return operation, err
	}
	rows, err := queryer.QueryContext(ctx, `
		SELECT scopes.scope_key, operation_scopes.prior_revision, operation_scopes.resulting_revision
		FROM semantic_operation_scopes AS operation_scopes
		JOIN semantic_scopes AS scopes ON scopes.scope_id = operation_scopes.scope_id
		WHERE operation_scopes.operation_id = ? ORDER BY scopes.scope_key
	`, id)
	if err != nil {
		return operation, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var prior, resulting int64
		if err := rows.Scan(&key, &prior, &resulting); err != nil {
			return operation, err
		}
		operation.PriorRevisions = append(operation.PriorRevisions, memory.ScopeRevision{ScopeKey: key, Revision: prior})
		operation.ResultingRevisions = append(operation.ResultingRevisions, memory.ScopeRevision{ScopeKey: key, Revision: resulting})
	}
	return operation, rows.Err()
}

func semanticStatus(state memory.SemanticStateValue) memory.SemanticObjectStatus {
	switch state {
	case memory.SemanticStateActive:
		return memory.SemanticStatusActive
	case memory.SemanticStateRetired:
		return memory.SemanticStatusRetired
	case memory.SemanticStateSuperseded:
		return memory.SemanticStatusSuperseded
	case memory.SemanticStateEligible:
		return memory.SemanticStatusEligible
	case memory.SemanticStateRetracted:
		return memory.SemanticStatusSourceRetracted
	default:
		return memory.SemanticObjectStatus(state)
	}
}

// InspectSemanticObject returns one exact object even when it is retired,
// superseded, unsupported, or source-retracted, together with full provenance
// and accepted operation history.
func (s *Store) InspectSemanticObject(ctx context.Context, scope memory.ScopeContext, kind memory.SemanticObjectKind, id memory.SemanticID) (memory.SemanticObjectInspection, error) {
	return s.InspectSemanticObjectAt(ctx, scope, kind, id, memory.ClaimQuery{})
}

// InspectSemanticObjectAt preserves the caller-selected Valid and Transaction
// Time axes while retaining the same exact, scope-bound inspection contract.
func (s *Store) InspectSemanticObjectAt(ctx context.Context, scope memory.ScopeContext, kind memory.SemanticObjectKind, id memory.SemanticID, temporal memory.ClaimQuery) (memory.SemanticObjectInspection, error) {
	return s.InspectSemanticObjectAtScopeAndTime(ctx, scope, kind, id, false, temporal)
}

// InspectSemanticObjectAtScope performs exact inspection in the session's
// Context Scope or, when requested, its current-session scope. Global and
// Context objects remain eligible dependencies in a session-scope view.
func (s *Store) InspectSemanticObjectAtScope(ctx context.Context, scope memory.ScopeContext, kind memory.SemanticObjectKind, id memory.SemanticID, useSessionScope bool) (memory.SemanticObjectInspection, error) {
	return s.InspectSemanticObjectAtScopeAndTime(ctx, scope, kind, id, useSessionScope, memory.ClaimQuery{})
}

// InspectSemanticObjectAtScopeAndTime is the complete local exact-inspection
// primitive. Convenience wrappers keep current-time and Context-scope callers
// compact without hiding either temporal axis from callers that need history.
func (s *Store) InspectSemanticObjectAtScopeAndTime(ctx context.Context, scope memory.ScopeContext, kind memory.SemanticObjectKind, id memory.SemanticID, useSessionScope bool, temporal memory.ClaimQuery) (memory.SemanticObjectInspection, error) {
	if err := validateSessionScopeIdentity(ctx, s.db, scope); err != nil {
		return memory.SemanticObjectInspection{}, err
	}
	if err := validateLifecycleActionTarget(memory.MemoryLifecycleRequest{Action: actionForInspection(kind), ObjectKind: kind, ObjectID: id}); err != nil {
		return memory.SemanticObjectInspection{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.SemanticObjectInspection{}, err
	}
	defer tx.Rollback()
	query := semanticInspectionQueryer(tx)
	target, err := loadLifecycleTargetScope(ctx, inspectionLifecycleQueryer{query}, kind, id)
	if err != nil {
		return memory.SemanticObjectInspection{}, err
	}
	if err := requireSemanticScopeKeysAvailable(ctx, query, []string{target.Key}); err != nil {
		return memory.SemanticObjectInspection{}, err
	}
	allowedScopes := map[string]struct{}{
		"global":                               {},
		scopeKeyForContext(scope):              {},
		"session:" + string(scope.SessionID):   {},
		targetScopeKey(scope, useSessionScope): {},
	}
	if _, allowed := allowedScopes[target.Key]; !allowed {
		return memory.SemanticObjectInspection{}, errors.New("semantic inspection target is outside the session-bound scope")
	}
	if temporal.ScopeKey != "" && temporal.ScopeKey != target.Key {
		return memory.SemanticObjectInspection{}, errors.New("semantic inspection target is outside the explicitly selected scope")
	}
	result := memory.SemanticObjectInspection{ObjectKind: kind, ObjectID: id, Scope: target}
	result.Metadata, err = s.exactReadMetadata(ctx, query, scope, temporal, nil, true)
	if err != nil {
		return result, err
	}
	at := result.Metadata.AsKnownAt
	result.Lifecycle, err = loadSemanticLifecycleAt(ctx, query, string(kind), id, at)
	if err != nil || len(result.Lifecycle) == 0 {
		return result, errors.New("semantic object has no accepted lifecycle")
	}
	result.Status = semanticStatus(result.Lifecycle[len(result.Lifecycle)-1].State)
	switch kind {
	case memory.SemanticObjectEntity:
		entity, err := loadSemanticEntityForInspection(ctx, query, id)
		if err != nil {
			return result, err
		}
		result.Entity = &entity
	case memory.SemanticObjectAlias:
		var alias memory.SemanticAlias
		if err := query.QueryRowContext(ctx, `
			SELECT aliases.alias_id, aliases.entity_id, scopes.scope_key, aliases.value,
			       aliases.normalized_value, aliases.created_operation_id, aliases.source_event_id
			FROM semantic_aliases AS aliases JOIN semantic_scopes AS scopes ON scopes.scope_id = aliases.scope_id
			WHERE aliases.alias_id = ?
		`, id).Scan(&alias.ID, &alias.EntityID, &alias.ScopeKey, &alias.Value, &alias.NormalizedValue,
			&alias.OperationID, &alias.SourceEventID); err != nil {
			return result, err
		}
		result.Alias = &alias
	case memory.SemanticObjectClaim:
		claim, err := loadSemanticClaim(ctx, query, id)
		if err != nil {
			return result, err
		}
		result.Claim = &claim
		result.Sources, err = loadAllSourceInspections(ctx, query, id, at)
		if err != nil {
			return result, err
		}
		redactSourceInspections(result.Sources, allowedScopes)
		if result.Status == memory.SemanticStatusActive {
			supported := false
			for _, source := range result.Sources {
				if len(source.Lifecycle) != 0 && source.Lifecycle[len(source.Lifecycle)-1].State == memory.SemanticStateEligible {
					supported = true
				}
			}
			if !supported {
				result.Status = memory.SemanticStatusUnsupported
			}
		}
		diagnosticQuery := temporal
		diagnosticQuery.ScopeKey = target.Key
		diagnostics, err := s.inspectClaimsSnapshot(ctx, query, scope, false, diagnosticQuery, true)
		if err != nil {
			return result, err
		}
		for _, warning := range exactClaimConflictWarnings(diagnostics.Claims) {
			if containsSemanticID(warning.ClaimIDs, id) {
				result.Conflicts = append(result.Conflicts, warning)
			}
		}
	case memory.SemanticObjectSourceLink:
		source, err := loadSourceForInspection(ctx, query, id)
		if err != nil {
			return result, err
		}
		if result.Lifecycle[len(result.Lifecycle)-1].State == memory.SemanticStateRetracted {
			source.Eligibility = memory.EligibilityRetracted
		} else {
			source.Eligibility = memory.EligibilityEligible
		}
		result.Source = &source
		if _, allowed := allowedScopes[source.ScopeKey]; !allowed {
			result.Source.Evidence = ""
		}
	case memory.SemanticObjectGraphLink:
		link, err := loadGraphLink(ctx, query, id)
		if err != nil {
			return result, err
		}
		result.GraphLink = &link
	}
	operationIDs := make(map[memory.SemanticID]struct{})
	redactedOperationIDs := make(map[memory.SemanticID]struct{})
	for _, state := range result.Lifecycle {
		operationIDs[state.OperationID] = struct{}{}
	}
	if result.Source != nil {
		if _, allowed := allowedScopes[result.Source.ScopeKey]; !allowed {
			redactedOperationIDs[result.Source.OperationID] = struct{}{}
			for _, state := range result.Lifecycle {
				redactedOperationIDs[state.OperationID] = struct{}{}
			}
		}
	}
	for _, source := range result.Sources {
		if _, allowed := allowedScopes[source.Source.ScopeKey]; !allowed {
			redactedOperationIDs[source.Source.OperationID] = struct{}{}
			for _, state := range source.Lifecycle {
				redactedOperationIDs[state.OperationID] = struct{}{}
			}
		}
		for _, state := range source.Lifecycle {
			operationIDs[state.OperationID] = struct{}{}
		}
	}
	ordered := make([]memory.SemanticID, 0, len(operationIDs))
	for operationID := range operationIDs {
		ordered = append(ordered, operationID)
	}
	for _, operationID := range ordered {
		operation, err := loadSemanticOperationInspection(ctx, query, operationID)
		if err != nil {
			return result, err
		}
		var promotionSourceScope string
		err = query.QueryRowContext(ctx, `
			SELECT scopes.scope_key FROM semantic_promotions AS promotions
			JOIN semantic_scopes AS scopes ON scopes.scope_id = promotions.source_scope_id
			WHERE promotions.operation_id = ?
		`, operationID).Scan(&promotionSourceScope)
		if err == nil {
			if _, allowed := allowedScopes[promotionSourceScope]; !allowed {
				operation.ProposalJSON = ""
				operation.PreparedJSON = ""
				operation.ResultJSON = ""
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
		if _, redacted := redactedOperationIDs[operationID]; redacted {
			operation.ProposalJSON = ""
			operation.PreparedJSON = ""
			operation.ResultJSON = ""
		}
		result.Operations = append(result.Operations, operation)
	}
	sort.Slice(result.Operations, func(i, j int) bool {
		if !result.Operations[i].TransactionTime.Equal(result.Operations[j].TransactionTime) {
			return result.Operations[i].TransactionTime.Before(result.Operations[j].TransactionTime)
		}
		return result.Operations[i].OperationID < result.Operations[j].OperationID
	})
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func containsSemanticID(values []memory.SemanticID, wanted memory.SemanticID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

type inspectionLifecycleQueryer struct{ semanticInspectionQueryer }

func (q inspectionLifecycleQueryer) queryContext(ctx context.Context, statement string, args ...any) (*sql.Rows, error) {
	return q.QueryContext(ctx, statement, args...)
}

func (q inspectionLifecycleQueryer) queryRowContext(ctx context.Context, statement string, args ...any) rowScanner {
	return q.QueryRowContext(ctx, statement, args...)
}

func actionForInspection(kind memory.SemanticObjectKind) memory.MemoryLifecycleAction {
	if kind == memory.SemanticObjectSourceLink {
		return memory.LifecycleRetractSource
	}
	return memory.LifecycleRetire
}
