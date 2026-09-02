package eviedb

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

type canonicalGraphEndpoint struct {
	Kind memory.SemanticObjectKind `json:"kind"`
	ID   memory.SemanticID         `json:"id"`
}

type canonicalGraphLink struct {
	ID        memory.SemanticID         `json:"graph_link_id"`
	ScopeKey  string                    `json:"scope_key"`
	Relation  memory.GraphRelation      `json:"relation"`
	Source    canonicalGraphEndpoint    `json:"source"`
	Target    canonicalGraphEndpoint    `json:"target"`
	Lifecycle memory.SemanticStateValue `json:"lifecycle"`
}

type canonicalGraphEffect struct {
	Scopes      []string              `json:"scopes"`
	Predicates  []struct{}            `json:"predicates"`
	Entities    []struct{}            `json:"entities"`
	Aliases     []struct{}            `json:"aliases"`
	Claims      []struct{}            `json:"claims"`
	SourceLinks []struct{}            `json:"source_links"`
	GraphLinks  []canonicalGraphLink  `json:"graph_links"`
	Transitions []canonicalTransition `json:"transitions"`
}

type canonicalGraphProposal struct {
	Kind           string                 `json:"kind"`
	IdempotencyKey string                 `json:"idempotency_key"`
	Actor          memory.SemanticActor   `json:"actor"`
	SessionID      memory.SessionID       `json:"session_id"`
	PriorRevisions []memory.ScopeRevision `json:"prior_revisions"`
	SourceEventIDs []memory.EventID       `json:"source_event_ids"`
	Effect         canonicalGraphEffect   `json:"effect"`
}

func canonicalCreateGraphLinkProposal(proposal memory.CreateGraphLinkProposal) canonicalGraphProposal {
	return canonicalGraphProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Evidence.EventID},
		Effect: canonicalGraphEffect{
			Scopes: []string{proposal.Scope.Key}, Predicates: []struct{}{}, Entities: []struct{}{},
			Aliases: []struct{}{}, Claims: []struct{}{}, SourceLinks: []struct{}{},
			GraphLinks: []canonicalGraphLink{{
				ID: proposal.Link.ID, ScopeKey: proposal.Link.ScopeKey, Relation: proposal.Link.Relation,
				Source: canonicalGraphEndpoint(proposal.Link.Source), Target: canonicalGraphEndpoint(proposal.Link.Target),
				Lifecycle: memory.SemanticStateActive,
			}},
			Transitions: []canonicalTransition{{ObjectKind: "graph_link", ObjectID: proposal.Link.ID, State: memory.SemanticStateActive}},
		},
	}
}

func validateGraphRelation(relation memory.GraphRelation, source, target memory.GraphEndpoint) error {
	if source.ID == target.ID && source.Kind == target.Kind {
		return errors.New("Graph Link endpoints must be distinct")
	}
	supported := func(kind memory.SemanticObjectKind) bool {
		return kind == memory.SemanticObjectEntity || kind == memory.SemanticObjectAlias ||
			kind == memory.SemanticObjectClaim || kind == memory.SemanticObjectSourceLink
	}
	if !supported(source.Kind) || !supported(target.Kind) {
		return errors.New("Graph Link endpoint kind is not structural")
	}
	switch relation {
	case memory.GraphRelationContradiction:
		if source.Kind != memory.SemanticObjectClaim || target.Kind != memory.SemanticObjectClaim {
			return errors.New("contradiction Graph Links require two Claims")
		}
	case memory.GraphRelationDerivation:
		if target.Kind != memory.SemanticObjectClaim ||
			(source.Kind != memory.SemanticObjectClaim && source.Kind != memory.SemanticObjectSourceLink) {
			return errors.New("derivation Graph Links require a Claim target and Claim or Source Link source")
		}
	case memory.GraphRelationGeneralization:
		if source.Kind != target.Kind || (source.Kind != memory.SemanticObjectClaim && source.Kind != memory.SemanticObjectEntity) {
			return errors.New("generalization Graph Links require two Claims or two Entities")
		}
	default:
		return fmt.Errorf("unsupported Graph Link relation %q", relation)
	}
	return nil
}

func graphLinkRequestsEqual(left, right memory.CreateGraphLinkRequest) bool { return left == right }

func endpointAllowedInTarget(target, contextKey, sessionKey, endpointScope string) bool {
	if target == "global" {
		return endpointScope == "global"
	}
	if target == sessionKey {
		return endpointScope == "global" || endpointScope == contextKey || endpointScope == sessionKey
	}
	return endpointScope == "global" || endpointScope == target
}

func requireActiveGraphEndpoint(ctx context.Context, query lifecycleQueryer, endpoint memory.GraphEndpoint) (memory.SemanticScope, error) {
	scope, err := loadLifecycleTargetScope(ctx, query, endpoint.Kind, endpoint.ID)
	if err != nil {
		return scope, err
	}
	state, err := loadLatestState(ctx, query, endpoint.Kind, endpoint.ID)
	if err != nil {
		return scope, err
	}
	want := memory.SemanticStateActive
	if endpoint.Kind == memory.SemanticObjectSourceLink {
		want = memory.SemanticStateEligible
	}
	if state.State != want {
		return scope, errors.New("Graph Link endpoint is not currently eligible")
	}
	if endpoint.Kind == memory.SemanticObjectClaim {
		var eligible int
		if err := query.queryRowContext(ctx, `
			SELECT COUNT(*) FROM semantic_source_links AS sources
			WHERE sources.claim_id = ? AND (SELECT state FROM semantic_state_events
			 WHERE object_kind = 'source_link' AND object_id = sources.source_link_id
			 ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1) = 'eligible'
		`, endpoint.ID).Scan(&eligible); err != nil {
			return scope, err
		}
		if eligible == 0 {
			return scope, errors.New("Graph Link Claim endpoint is unsupported")
		}
	}
	return scope, nil
}

func (s *Store) PrepareCreateGraphLink(ctx context.Context, scope memory.ScopeContext, request memory.CreateGraphLinkRequest) (memory.CreateGraphLinkProposal, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	if request.UseSessionScope {
		if err := requireSemanticScopeKeysAvailable(ctx, s.db, []string{"session:" + string(scope.SessionID)}); err != nil {
			return memory.CreateGraphLinkProposal{}, err
		}
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") || validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.CreateGraphLinkProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if err := validateGraphRelation(request.Relation, request.Source, request.Target); err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	var priorJSON, priorHash string
	err := s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&priorJSON, &priorHash)
	if err == nil {
		var proposal memory.CreateGraphLinkProposal
		if err := json.Unmarshal([]byte(priorJSON), &proposal); err != nil {
			return proposal, err
		}
		if !graphLinkRequestsEqual(proposal.Request, request) || proposal.SessionID != scope.SessionID {
			return proposal, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		return proposal, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.CreateGraphLinkProposal{}, err
	}
	targetKey := targetScopeKey(scope, request.UseSessionScope)
	target, err := loadSemanticScope(ctx, s.db, targetKey)
	if err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	query := dbLifecycleQueryer{s.db}
	sourceScope, err := requireActiveGraphEndpoint(ctx, query, request.Source)
	if err != nil {
		return memory.CreateGraphLinkProposal{}, fmt.Errorf("load Graph Link source: %w", err)
	}
	targetScope, err := requireActiveGraphEndpoint(ctx, query, request.Target)
	if err != nil {
		return memory.CreateGraphLinkProposal{}, fmt.Errorf("load Graph Link target: %w", err)
	}
	contextKey, sessionKey := scopeKeyForContext(scope), "session:"+string(scope.SessionID)
	if !endpointAllowedInTarget(targetKey, contextKey, sessionKey, sourceScope.Key) || !endpointAllowedInTarget(targetKey, contextKey, sessionKey, targetScope.Key) {
		return memory.CreateGraphLinkProposal{}, errors.New("Graph Link endpoint is outside the accepted scope-reference matrix")
	}
	var existing memory.SemanticID
	err = s.db.QueryRowContext(ctx, `SELECT graph_link_id FROM semantic_graph_links WHERE scope_id = ? AND relation = ? AND source_kind = ? AND source_id = ? AND target_kind = ? AND target_id = ?`,
		target.ID, request.Relation, request.Source.Kind, request.Source.ID, request.Target.Kind, request.Target.ID).Scan(&existing)
	if err == nil {
		return memory.CreateGraphLinkProposal{}, errors.New("Graph Link already exists; restore it explicitly if retired")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.CreateGraphLinkProposal{}, err
	}
	evidence, err := loadLifecycleEvidence(s.db.QueryRowContext(ctx, `SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?`, request.SourceEventID), scope.SessionID, request.SourceEventID, targetKey)
	if err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	operationID, err := newSemanticID()
	if err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	linkID, err := newSemanticID()
	if err != nil {
		return memory.CreateGraphLinkProposal{}, err
	}
	base := map[string]struct{}{"global": {}, contextKey: {}, targetKey: {}}
	keys := sortedStringSet(base)
	scopes := make([]memory.SemanticScope, len(keys))
	priors := make([]memory.ScopeRevision, len(keys))
	for i, key := range keys {
		loaded, err := loadSemanticScope(ctx, s.db, key)
		if err != nil {
			return memory.CreateGraphLinkProposal{}, err
		}
		scopes[i] = loaded.SemanticScope
		priors[i] = memory.ScopeRevision{ScopeKey: key, Revision: loaded.Revision}
	}
	proposal := memory.CreateGraphLinkProposal{
		SchemaVersion: 5, Kind: "create_graph_link", OperationID: operationID, IdempotencyKey: request.IdempotencyKey,
		Actor: memory.SemanticActorOwner, SessionID: scope.SessionID, Scope: target.SemanticScope, Scopes: scopes,
		PriorRevisions: priors, Evidence: evidence, Request: request,
		Link: memory.SemanticGraphLink{ID: linkID, ScopeKey: targetKey, Relation: request.Relation, Source: request.Source, Target: request.Target, CreatedOperationID: operationID},
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalCreateGraphLinkProposal(proposal))
	if err == nil {
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
	}
	return proposal, err
}

func validateExactSemanticApproval(ctx context.Context, writer turnLeaseWriteExecutor, sessionID memory.SessionID, operationID memory.SemanticID, parentID memory.EventID, proposalHash, preparedHash string) error {
	var raw []byte
	if err := writer.queryRowContext(ctx, `SELECT payload_json FROM events WHERE session_id = ? AND event_type = 'approval' AND execution_id = ? AND parent_id = ? ORDER BY sequence DESC LIMIT 1`, sessionID, operationID, parentID).Scan(&raw); err != nil {
		return fmt.Errorf("semantic operation has no trusted approval event: %w", err)
	}
	var approval memory.ApprovalPayload
	if err := json.Unmarshal(raw, &approval); err != nil || approval.Decision != memory.ApprovalApproved || approval.ProposalSHA256 != proposalHash || approval.PreparedSHA256 != preparedHash {
		return errors.New("semantic operation changed after approval")
	}
	return nil
}

func (s *Store) ApplyCreateGraphLink(ctx context.Context, lease memory.TurnLease, proposal memory.CreateGraphLinkProposal) (result memory.CreateGraphLinkResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("Graph Link proposal does not match its turn lease")
	}
	canonical := canonicalCreateGraphLinkProposal(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if proposal.SchemaVersion != 5 || proposal.Kind != "create_graph_link" || proposal.Actor != memory.SemanticActorOwner ||
		proposal.ProposalSHA256 != proposalHash || proposal.PreparedSHA256 != preparedHash || proposal.Link.ScopeKey != proposal.Scope.Key ||
		proposal.Link.CreatedOperationID != proposal.OperationID || !graphLinkRequestsEqual(proposal.Request, memory.CreateGraphLinkRequest{IdempotencyKey: proposal.IdempotencyKey, SourceEventID: proposal.Evidence.EventID, Relation: proposal.Link.Relation, Source: proposal.Link.Source, Target: proposal.Link.Target, UseSessionScope: proposal.Request.UseSessionScope}) {
		return result, errors.New("invalid Graph Link proposal")
	}
	if !proposal.Link.TransactionTime.IsZero() {
		return result, errors.New("unaccepted Graph Link proposal cannot assign Transaction Time")
	}
	for _, id := range []memory.SemanticID{proposal.OperationID, proposal.Link.ID, proposal.Link.Source.ID, proposal.Link.Target.ID, proposal.Scope.ID} {
		if err := validateSemanticUUID(string(id)); err != nil {
			return result, err
		}
	}
	if err := validateGraphRelation(proposal.Link.Relation, proposal.Link.Source, proposal.Link.Target); err != nil {
		return result, err
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
		if err := validateExactSemanticApproval(ctx, writer, proposal.SessionID, proposal.OperationID, proposal.Evidence.EventID, proposalHash, preparedHash); err != nil {
			return err
		}
		expected, baseKeys, err := authorizedSemanticScopes(ctx, writer, proposal.SessionID, proposal.Request.UseSessionScope)
		if err != nil || expected != proposal.Scope.Key {
			return errors.New("Graph Link proposal is outside its immutable session Context Scope")
		}
		if !stringSlicesEqual(baseKeys, semanticScopeKeys(proposal.Scopes)) {
			return errors.New("Graph Link proposal contains an unauthorized scope")
		}
		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions, s.now())
		if err != nil {
			return err
		}
		sourceScope, err := requireActiveGraphEndpoint(ctx, writer, proposal.Link.Source)
		if err != nil {
			return err
		}
		targetScope, err := requireActiveGraphEndpoint(ctx, writer, proposal.Link.Target)
		if err != nil {
			return err
		}
		contextKey, sessionKey := scopeKeyForContext(memory.ScopeContext{SessionID: proposal.SessionID}), "session:"+string(proposal.SessionID)
		// The authoritative context key is already present in the exact authorization vector.
		for _, key := range baseKeys {
			if key != "global" && key != sessionKey {
				contextKey = key
			}
		}
		if !endpointAllowedInTarget(expected, contextKey, sessionKey, sourceScope.Key) || !endpointAllowedInTarget(expected, contextKey, sessionKey, targetScope.Key) {
			return errors.New("Graph Link endpoint scope changed after approval")
		}
		evidence, err := loadLifecycleEvidence(writer.queryRowContext(ctx, `SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?`, proposal.Evidence.EventID), proposal.SessionID, proposal.Evidence.EventID, proposal.Scope.Key)
		if err != nil || evidence != proposal.Evidence {
			return errors.New("Graph Link evidence changed")
		}
		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		result = memory.CreateGraphLinkResult{OperationID: proposal.OperationID, GraphLinkID: proposal.Link.ID, TransactionTime: now, ScopeRevision: proposal.Scope.Revision + 1}
		for _, semanticScope := range proposal.Scopes {
			revision := semanticScope.Revision
			if semanticScope.Key == proposal.Scope.Key {
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
		if err := recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{SchemaVersion: 5, OperationID: proposal.OperationID, Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor, SessionID: proposal.SessionID, TargetScopeID: proposal.Scope.ID, SourceEventID: proposal.Evidence.EventID, ProposalHash: proposalHash, EffectHash: effectHash, ProposalJSON: proposalJSON, PreparedJSON: preparedJSON, ResultJSON: resultJSON, TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey}); err != nil {
			return err
		}
		if _, err := writer.execContext(ctx, `INSERT INTO semantic_graph_links (graph_link_id, scope_id, relation, source_kind, source_id, target_kind, target_id, lifecycle, created_operation_id, transaction_time) VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`, proposal.Link.ID, proposal.Scope.ID, proposal.Link.Relation, proposal.Link.Source.Kind, proposal.Link.Source.ID, proposal.Link.Target.Kind, proposal.Link.Target.ID, proposal.OperationID, formatSemanticTime(now)); err != nil {
			return err
		}
		if _, err := writer.execContext(ctx, `INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time) VALUES (?, 'graph_link', ?, 'active', ?, ?, ?)`, proposal.Scope.ID, proposal.Link.ID, proposal.OperationID, proposal.Scope.Revision+1, formatSemanticTime(now)); err != nil {
			return err
		}
		updated, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`, proposal.Scope.Revision+1, proposal.Scope.ID, proposal.Scope.Revision)
		if err != nil {
			return err
		}
		if changed, _ := updated.RowsAffected(); changed != 1 {
			return ErrStaleScopeRevision
		}
		return nil
	})
	return result, err
}

func loadGraphLink(ctx context.Context, queryer semanticInspectionQueryer, id memory.SemanticID) (memory.SemanticGraphLink, error) {
	var link memory.SemanticGraphLink
	var transactionTime string
	err := queryer.QueryRowContext(ctx, `SELECT links.graph_link_id, scopes.scope_key, links.relation, links.source_kind, links.source_id, links.target_kind, links.target_id, links.created_operation_id, links.transaction_time FROM semantic_graph_links AS links JOIN semantic_scopes AS scopes ON scopes.scope_id = links.scope_id WHERE links.graph_link_id = ?`, id).Scan(&link.ID, &link.ScopeKey, &link.Relation, &link.Source.Kind, &link.Source.ID, &link.Target.Kind, &link.Target.ID, &link.CreatedOperationID, &transactionTime)
	if err != nil {
		return link, err
	}
	link.TransactionTime, err = parseSemanticTime(transactionTime)
	return link, err
}

type semanticCursorPayload struct {
	Version        int                    `json:"v"`
	Kind           string                 `json:"kind"`
	QueryHash      string                 `json:"query_hash"`
	ValidAt        string                 `json:"valid_at"`
	AsKnownAt      string                 `json:"as_known_at"`
	AllowedScopes  []string               `json:"allowed_scopes"`
	ScopeRevisions []memory.ScopeRevision `json:"scope_revisions"`
	LastKey        string                 `json:"last_key"`
}

type semanticCursorEnvelope struct {
	Payload   semanticCursorPayload `json:"payload"`
	MACSHA256 string                `json:"mac_sha256"`
}

func semanticCursorMAC(key, payload []byte) string {
	digest := hmac.New(sha256.New, key)
	_, _ = digest.Write(payload)
	return fmt.Sprintf("hmac-sha256:%x", digest.Sum(nil))
}

func loadSemanticCursorKey(ctx context.Context, queryer semanticInspectionQueryer) ([]byte, error) {
	var key []byte
	if err := queryer.QueryRowContext(ctx, `SELECT hmac_key FROM semantic_cursor_auth WHERE singleton = 1`).Scan(&key); err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("semantic cursor authentication key is invalid")
	}
	return key, nil
}

func encodeSemanticCursor(key []byte, payload semanticCursorPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	envelope, err := json.Marshal(semanticCursorEnvelope{Payload: payload, MACSHA256: semanticCursorMAC(key, raw)})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

func decodeSemanticCursor(key []byte, value string) (semanticCursorPayload, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return semanticCursorPayload{}, ErrInvalidCursor
	}
	var envelope semanticCursorEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Payload.Version != 1 {
		return semanticCursorPayload{}, ErrInvalidCursor
	}
	payloadRaw, _ := json.Marshal(envelope.Payload)
	if !hmac.Equal([]byte(envelope.MACSHA256), []byte(semanticCursorMAC(key, payloadRaw))) {
		return semanticCursorPayload{}, ErrInvalidCursor
	}
	return envelope.Payload, nil
}

func exactQueryHash(kind string, query any) (string, error) {
	raw, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Query any    `json:"query"`
	}{kind, query})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", digest), nil
}

func (s *Store) exactReadMetadata(ctx context.Context, queryer semanticInspectionQueryer, scope memory.ScopeContext, query memory.ClaimQuery, cursor *semanticCursorPayload) (memory.ExactReadMetadata, error) {
	if err := validateSessionScope(ctx, queryer, scope); err != nil {
		return memory.ExactReadMetadata{}, err
	}
	validAt, asKnownAt, err := s.semanticQueryTimes(ctx, queryer, query)
	if err != nil {
		return memory.ExactReadMetadata{}, err
	}
	keys := allowedSemanticReadScopeKeys(scope)
	if err := requireSemanticScopeKeysAvailable(ctx, queryer, keys); err != nil {
		return memory.ExactReadMetadata{}, err
	}
	metadata := memory.ExactReadMetadata{ValidAt: validAt, AsKnownAt: asKnownAt, AllowedScopes: keys}
	if cursor != nil {
		parsedValid, err := parseSemanticTime(cursor.ValidAt)
		if err != nil {
			return metadata, ErrInvalidCursor
		}
		parsedKnown, err := parseSemanticTime(cursor.AsKnownAt)
		if err != nil {
			return metadata, ErrInvalidCursor
		}
		metadata.ValidAt, metadata.AsKnownAt, metadata.AllowedScopes, metadata.ScopeRevisions = parsedValid, parsedKnown, cursor.AllowedScopes, cursor.ScopeRevisions
		if !stringSlicesEqual(keys, metadata.AllowedScopes) {
			return metadata, ErrInvalidCursor
		}
		if len(metadata.ScopeRevisions) != len(keys) {
			return metadata, ErrInvalidCursor
		}
		for index, key := range keys {
			if metadata.ScopeRevisions[index].ScopeKey != key {
				return metadata, ErrInvalidCursor
			}
		}
	}
	if cursor == nil {
		for _, key := range keys {
			var revision int64
			err := queryer.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = ?`, key).Scan(&revision)
			if errors.Is(err, sql.ErrNoRows) {
				revision = 0
			} else if err != nil {
				return metadata, err
			}
			metadata.ScopeRevisions = append(metadata.ScopeRevisions, memory.ScopeRevision{ScopeKey: key, Revision: revision})
		}
	} else {
		for _, captured := range metadata.ScopeRevisions {
			var current int64
			err := queryer.QueryRowContext(ctx, `SELECT revision FROM semantic_scopes WHERE scope_key = ?`, captured.ScopeKey).Scan(&current)
			if errors.Is(err, sql.ErrNoRows) {
				current = 0
			} else if err != nil {
				return metadata, err
			}
			if current != captured.Revision {
				return metadata, ErrStaleCursor
			}
		}
	}
	return metadata, nil
}

func normalizedPageSize(size int) (int, error) {
	if size == 0 {
		return 50, nil
	}
	if size < 1 || size > 100 {
		return 0, errors.New("semantic exact-read page size must be between 1 and 100")
	}
	return size, nil
}

func objectListQueryWithoutCursor(query memory.SemanticObjectListQuery) memory.SemanticObjectListQuery {
	query.Cursor = ""
	return query
}

func scopeListQueryWithoutCursor(query memory.SemanticScopeListQuery) memory.SemanticScopeListQuery {
	query.Cursor = ""
	return query
}

func semanticObjectKey(object memory.SemanticObjectSummary) string {
	return object.ScopeKey + "\x00" + string(object.ObjectKind) + "\x00" + string(object.ObjectID)
}

func requestedObjectKinds(kinds []memory.SemanticObjectKind) (map[memory.SemanticObjectKind]struct{}, error) {
	if len(kinds) == 0 {
		kinds = []memory.SemanticObjectKind{memory.SemanticObjectEntity, memory.SemanticObjectAlias, memory.SemanticObjectClaim, memory.SemanticObjectSourceLink, memory.SemanticObjectGraphLink}
	}
	result := make(map[memory.SemanticObjectKind]struct{}, len(kinds))
	for _, kind := range kinds {
		switch kind {
		case memory.SemanticObjectEntity, memory.SemanticObjectAlias, memory.SemanticObjectClaim, memory.SemanticObjectSourceLink, memory.SemanticObjectGraphLink:
		default:
			return nil, fmt.Errorf("unsupported semantic object listing kind %q", kind)
		}
		result[kind] = struct{}{}
	}
	return result, nil
}

func requestedRelations(relations []memory.GraphRelation) (map[memory.GraphRelation]struct{}, error) {
	result := make(map[memory.GraphRelation]struct{}, len(relations))
	for _, relation := range relations {
		if relation != memory.GraphRelationDerivation && relation != memory.GraphRelationGeneralization && relation != memory.GraphRelationContradiction {
			return nil, fmt.Errorf("unsupported Graph Link relation filter %q", relation)
		}
		result[relation] = struct{}{}
	}
	return result, nil
}

func latestStateAt(ctx context.Context, queryer semanticInspectionQueryer, kind memory.SemanticObjectKind, id memory.SemanticID, at string) (memory.SemanticStateValue, error) {
	var state memory.SemanticStateValue
	err := queryer.QueryRowContext(ctx, `SELECT state FROM semantic_state_events WHERE object_kind = ? AND object_id = ? AND transaction_time <= ? ORDER BY transaction_time DESC, scope_revision DESC, operation_id DESC, state DESC LIMIT 1`, kind, id, at).Scan(&state)
	return state, err
}

func (s *Store) collectExactObjects(ctx context.Context, queryer semanticInspectionQueryer, scope memory.ScopeContext, metadata memory.ExactReadMetadata, query memory.SemanticObjectListQuery) ([]memory.SemanticObjectSummary, error) {
	kinds, err := requestedObjectKinds(query.Kinds)
	if err != nil {
		return nil, err
	}
	relations, err := requestedRelations(query.Relations)
	if err != nil {
		return nil, err
	}
	allowed := make(map[string]struct{}, len(metadata.AllowedScopes))
	for _, key := range metadata.AllowedScopes {
		allowed[key] = struct{}{}
	}
	known := formatSemanticTime(metadata.AsKnownAt)
	objects := make([]memory.SemanticObjectSummary, 0)
	visible := make(map[semanticNodeKey]memory.SemanticObjectSummary)
	exactClaimQuery := query.ClaimQuery
	exactClaimQuery.ValidAt, exactClaimQuery.AsKnownAt = &metadata.ValidAt, &metadata.AsKnownAt
	claims, err := s.inspectClaimsSnapshot(ctx, queryer, scope, false, exactClaimQuery, false)
	if err != nil {
		return nil, err
	}
	for _, claim := range claims.Claims {
		row := memory.SemanticObjectSummary{ObjectKind: memory.SemanticObjectClaim, ObjectID: claim.ID, ScopeKey: claim.Scope.Key, Status: memory.SemanticStatusActive, Claim: &claim.SemanticClaim}
		visible[semanticNodeKey{Kind: row.ObjectKind, ID: row.ObjectID}] = row
		if _, ok := kinds[row.ObjectKind]; ok {
			objects = append(objects, row)
		}
	}
	loadSimple := func(kind memory.SemanticObjectKind, table, idColumn string, eligible memory.SemanticStateValue) error {
		if _, wanted := kinds[kind]; !wanted && kind != memory.SemanticObjectEntity && kind != memory.SemanticObjectAlias && kind != memory.SemanticObjectSourceLink {
			return nil
		}
		rows, err := queryer.QueryContext(ctx, fmt.Sprintf(`SELECT objects.%s, scopes.scope_key FROM %s AS objects JOIN semantic_scopes AS scopes ON scopes.scope_id = objects.scope_id JOIN semantic_operations AS operations ON operations.operation_id = objects.created_operation_id WHERE operations.transaction_time <= ? ORDER BY scopes.scope_key, objects.%s`, idColumn, table, idColumn), known)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id memory.SemanticID
			var scopeKey string
			if err := rows.Scan(&id, &scopeKey); err != nil {
				return err
			}
			if _, ok := allowed[scopeKey]; !ok {
				continue
			}
			state, err := latestStateAt(ctx, queryer, kind, id, known)
			if err != nil {
				return err
			}
			if state != eligible {
				continue
			}
			status := semanticStatus(state)
			row := memory.SemanticObjectSummary{ObjectKind: kind, ObjectID: id, ScopeKey: scopeKey, Status: status}
			visible[semanticNodeKey{Kind: kind, ID: id}] = row
			if _, wanted := kinds[kind]; wanted {
				objects = append(objects, row)
			}
		}
		return rows.Err()
	}
	if err := loadSimple(memory.SemanticObjectEntity, "semantic_entities", "entity_id", memory.SemanticStateActive); err != nil {
		return nil, err
	}
	if err := loadSimple(memory.SemanticObjectAlias, "semantic_aliases", "alias_id", memory.SemanticStateActive); err != nil {
		return nil, err
	}
	if err := loadSimple(memory.SemanticObjectSourceLink, "semantic_source_links", "source_link_id", memory.SemanticStateEligible); err != nil {
		return nil, err
	}
	if _, wanted := kinds[memory.SemanticObjectGraphLink]; wanted {
		rows, err := queryer.QueryContext(ctx, `SELECT graph_link_id FROM semantic_graph_links ORDER BY graph_link_id`)
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
		for _, id := range ids {
			link, err := loadGraphLink(ctx, queryer, id)
			if err != nil {
				return nil, err
			}
			if _, ok := allowed[link.ScopeKey]; !ok || link.TransactionTime.After(metadata.AsKnownAt) {
				continue
			}
			state, err := latestStateAt(ctx, queryer, memory.SemanticObjectGraphLink, id, known)
			if err != nil {
				return nil, fmt.Errorf("load Graph Link lifecycle: %w", err)
			}
			if state != memory.SemanticStateActive {
				continue
			}
			if len(relations) != 0 {
				if _, ok := relations[link.Relation]; !ok {
					continue
				}
			}
			if _, ok := visible[semanticNodeKey(link.Source)]; !ok {
				continue
			}
			if _, ok := visible[semanticNodeKey(link.Target)]; !ok {
				continue
			}
			copyLink := link
			objects = append(objects, memory.SemanticObjectSummary{ObjectKind: memory.SemanticObjectGraphLink, ObjectID: id, ScopeKey: link.ScopeKey, Status: memory.SemanticStatusActive, GraphLink: &copyLink})
		}
	}
	sort.Slice(objects, func(i, j int) bool { return semanticObjectKey(objects[i]) < semanticObjectKey(objects[j]) })
	return objects, nil
}

func (s *Store) ListSemanticObjects(ctx context.Context, scope memory.ScopeContext, query memory.SemanticObjectListQuery) (memory.SemanticObjectPage, error) {
	pageSize, err := normalizedPageSize(query.PageSize)
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	query.PageSize = pageSize
	queryHash, err := exactQueryHash("objects", objectListQueryWithoutCursor(query))
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	defer tx.Rollback()
	cursorKey, err := loadSemanticCursorKey(ctx, tx)
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	var cursor *semanticCursorPayload
	if query.Cursor != "" {
		decoded, err := decodeSemanticCursor(cursorKey, query.Cursor)
		if err != nil {
			return memory.SemanticObjectPage{}, err
		}
		if decoded.Kind != "objects" || decoded.QueryHash != queryHash {
			return memory.SemanticObjectPage{}, ErrInvalidCursor
		}
		cursor = &decoded
	}
	metadata, err := s.exactReadMetadata(ctx, tx, scope, query.ClaimQuery, cursor)
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	objects, err := s.collectExactObjects(ctx, tx, scope, metadata, query)
	if err != nil {
		return memory.SemanticObjectPage{}, err
	}
	start := 0
	if cursor != nil {
		for start < len(objects) && semanticObjectKey(objects[start]) <= cursor.LastKey {
			start++
		}
	}
	end := start + pageSize
	if end > len(objects) {
		end = len(objects)
	}
	result := memory.SemanticObjectPage{Metadata: metadata, Objects: append([]memory.SemanticObjectSummary(nil), objects[start:end]...)}
	if end < len(objects) {
		result.NextCursor, err = encodeSemanticCursor(cursorKey, semanticCursorPayload{Version: 1, Kind: "objects", QueryHash: queryHash, ValidAt: formatSemanticTime(metadata.ValidAt), AsKnownAt: formatSemanticTime(metadata.AsKnownAt), AllowedScopes: metadata.AllowedScopes, ScopeRevisions: metadata.ScopeRevisions, LastKey: semanticObjectKey(objects[end-1])})
		if err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) ListSemanticScopes(ctx context.Context, scope memory.ScopeContext, query memory.SemanticScopeListQuery) (memory.SemanticScopePage, error) {
	pageSize, err := normalizedPageSize(query.PageSize)
	if err != nil {
		return memory.SemanticScopePage{}, err
	}
	query.PageSize = pageSize
	queryHash, err := exactQueryHash("scopes", scopeListQueryWithoutCursor(query))
	if err != nil {
		return memory.SemanticScopePage{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.SemanticScopePage{}, err
	}
	defer tx.Rollback()
	cursorKey, err := loadSemanticCursorKey(ctx, tx)
	if err != nil {
		return memory.SemanticScopePage{}, err
	}
	var cursor *semanticCursorPayload
	if query.Cursor != "" {
		decoded, err := decodeSemanticCursor(cursorKey, query.Cursor)
		if err != nil {
			return memory.SemanticScopePage{}, err
		}
		if decoded.Kind != "scopes" || decoded.QueryHash != queryHash {
			return memory.SemanticScopePage{}, ErrInvalidCursor
		}
		cursor = &decoded
	}
	metadata, err := s.exactReadMetadata(ctx, tx, scope, query.ClaimQuery, cursor)
	if err != nil {
		return memory.SemanticScopePage{}, err
	}
	var scopes []memory.SemanticScope
	for _, key := range metadata.AllowedScopes {
		var item memory.SemanticScope
		var registry sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT scope_id, scope_key, registry_id, revision FROM semantic_scopes WHERE scope_key = ?`, key).Scan(&item.ID, &item.Key, &registry, &item.Revision)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return memory.SemanticScopePage{}, err
		}
		if registry.Valid {
			item.RegistryID = registry.String
		}
		scopes = append(scopes, item)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Key < scopes[j].Key })
	start := 0
	if cursor != nil {
		for start < len(scopes) && scopes[start].Key <= cursor.LastKey {
			start++
		}
	}
	end := start + pageSize
	if end > len(scopes) {
		end = len(scopes)
	}
	result := memory.SemanticScopePage{Metadata: metadata, Scopes: append([]memory.SemanticScope(nil), scopes[start:end]...)}
	if end < len(scopes) {
		result.NextCursor, err = encodeSemanticCursor(cursorKey, semanticCursorPayload{Version: 1, Kind: "scopes", QueryHash: queryHash, ValidAt: formatSemanticTime(metadata.ValidAt), AsKnownAt: formatSemanticTime(metadata.AsKnownAt), AllowedScopes: metadata.AllowedScopes, ScopeRevisions: metadata.ScopeRevisions, LastKey: scopes[end-1].Key})
		if err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

type semanticNodeKey memory.GraphEndpoint

func pathKey(path memory.SemanticPath) string {
	parts := make([]string, 0, len(path.Nodes)+len(path.Links))
	for i, node := range path.Nodes {
		parts = append(parts, string(node.Kind)+":"+string(node.ID))
		if i < len(path.Links) {
			parts = append(parts, string(path.Links[i].Relation)+":"+string(path.Links[i].ID))
		}
	}
	return strings.Join(parts, "\x00")
}

func (s *Store) TraverseSemanticNeighborhood(ctx context.Context, scope memory.ScopeContext, query memory.SemanticTraversalQuery) (memory.SemanticNeighborhood, error) {
	if query.Depth != 1 && query.Depth != 2 {
		return memory.SemanticNeighborhood{}, errors.New("semantic traversal depth must be one or two")
	}
	if err := validateGraphRelationFilterOnly(query.Relations); err != nil {
		return memory.SemanticNeighborhood{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return memory.SemanticNeighborhood{}, err
	}
	defer tx.Rollback()
	metadata, err := s.exactReadMetadata(ctx, tx, scope, query.ClaimQuery, nil)
	if err != nil {
		return memory.SemanticNeighborhood{}, err
	}
	listQuery := memory.SemanticObjectListQuery{ClaimQuery: query.ClaimQuery, Relations: query.Relations}
	objects, err := s.collectExactObjects(ctx, tx, scope, metadata, listQuery)
	if err != nil {
		return memory.SemanticNeighborhood{}, err
	}
	nodes := make(map[semanticNodeKey]memory.SemanticObjectSummary)
	var links []memory.SemanticGraphLink
	for _, object := range objects {
		if object.GraphLink != nil {
			links = append(links, *object.GraphLink)
		} else {
			nodes[semanticNodeKey{Kind: object.ObjectKind, ID: object.ObjectID}] = object
		}
	}
	startKey := semanticNodeKey(query.Start)
	if _, ok := nodes[startKey]; !ok {
		return memory.SemanticNeighborhood{}, errors.New("semantic traversal start is not eligible in the exact read")
	}
	type partial struct {
		path memory.SemanticPath
		seen map[semanticNodeKey]struct{}
	}
	frontier := []partial{{path: memory.SemanticPath{Nodes: []memory.GraphEndpoint{query.Start}}, seen: map[semanticNodeKey]struct{}{startKey: {}}}}
	var paths []memory.SemanticPath
	reached := map[semanticNodeKey]struct{}{startKey: {}}
	for depth := 0; depth < query.Depth; depth++ {
		var next []partial
		for _, current := range frontier {
			last := semanticNodeKey(current.path.Nodes[len(current.path.Nodes)-1])
			for _, link := range links {
				var endpoint memory.GraphEndpoint
				if semanticNodeKey(link.Source) == last {
					endpoint = link.Target
				} else if semanticNodeKey(link.Target) == last {
					endpoint = link.Source
				} else {
					continue
				}
				key := semanticNodeKey(endpoint)
				if _, cycle := current.seen[key]; cycle {
					continue
				}
				if _, visible := nodes[key]; !visible {
					continue
				}
				path := memory.SemanticPath{Nodes: append(append([]memory.GraphEndpoint(nil), current.path.Nodes...), endpoint), Links: append(append([]memory.SemanticGraphLink(nil), current.path.Links...), link)}
				paths = append(paths, path)
				reached[key] = struct{}{}
				seen := make(map[semanticNodeKey]struct{}, len(current.seen)+1)
				for prior := range current.seen {
					seen[prior] = struct{}{}
				}
				seen[key] = struct{}{}
				next = append(next, partial{path: path, seen: seen})
			}
		}
		frontier = next
	}
	sort.Slice(paths, func(i, j int) bool { return pathKey(paths[i]) < pathKey(paths[j]) })
	result := memory.SemanticNeighborhood{Metadata: metadata, Paths: paths}
	for key := range reached {
		result.Objects = append(result.Objects, nodes[key])
	}
	sort.Slice(result.Objects, func(i, j int) bool {
		return semanticObjectKey(result.Objects[i]) < semanticObjectKey(result.Objects[j])
	})
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func validateGraphRelationFilterOnly(relations []memory.GraphRelation) error {
	_, err := requestedRelations(relations)
	return err
}
