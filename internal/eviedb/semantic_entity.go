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
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrAmbiguousAlias = errors.New("semantic memory: ambiguous Alias requires a stable Entity ID")

func normalizeAlias(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func targetScopeKey(scope memory.ScopeContext, useSession bool) string {
	if useSession {
		return "session:" + string(scope.SessionID)
	}
	return scopeKeyForContext(scope)
}

func normalizeEntityRequest(request memory.RememberEntityRequest) (memory.RememberEntityRequest, error) {
	cardinality, polarity, validTime, err := normalizeClaimSemantics(
		request.PredicateCardinality, memory.CardinalityMany, request.Polarity, request.ValidTime,
	)
	if err != nil {
		return memory.RememberEntityRequest{}, err
	}
	request.PredicateCardinality, request.Polarity, request.ValidTime = cardinality, polarity, validTime
	return request, nil
}

func entityRequestsEqual(left, right memory.RememberEntityRequest) bool {
	return left.IdempotencyKey == right.IdempotencyKey && left.SourceEventID == right.SourceEventID &&
		left.Predicate == right.Predicate && left.PredicateLabel == right.PredicateLabel &&
		left.PredicateCardinality == right.PredicateCardinality && left.Polarity == right.Polarity &&
		validTimesEqual(left.ValidTime, right.ValidTime) && left.Subject == right.Subject && left.Object == right.Object &&
		left.UseSessionScope == right.UseSessionScope
}

func loadOwnerSource(row rowScanner, sessionID memory.SessionID, eventID memory.EventID, scopeKey string) (memory.SemanticSource, error) {
	var source memory.SemanticSource
	var eventSession, eventType, role, content, recordedAt string
	if err := row.Scan(&eventSession, &eventType, &role, &content, &recordedAt); err != nil {
		return source, fmt.Errorf("load source event: %w", err)
	}
	if eventSession != string(sessionID) || eventType != string(memory.EventUserMessage) || role != string(memory.RoleUser) {
		return source, errors.New("source event must be an owner user message in the bound session")
	}
	observed, err := time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return source, fmt.Errorf("parse source event time: %w", err)
	}
	digest := sha256.Sum256([]byte(content))
	return memory.SemanticSource{
		EventID: eventID, SessionID: sessionID, ScopeKey: scopeKey,
		EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole,
		EvidenceSHA256: fmt.Sprintf("sha256:%x", digest), Actor: memory.SemanticActorOwner,
		SourceType: memory.SourceTypeUserMessage, Authority: memory.AuthorityOwnerStatement,
		ObservedAt: formatSemanticTime(observed), Evidence: content, Eligibility: memory.EligibilityEligible, Create: true,
	}, nil
}

func loadAnchor(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, scopeKey, canonicalName, entityType, anchorKind string) (memory.SemanticEntity, error) {
	entity := memory.SemanticEntity{ScopeKey: scopeKey, CanonicalName: canonicalName, EntityType: entityType, AnchorKind: anchorKind}
	err := query.QueryRowContext(ctx, `
		SELECT entities.entity_id, entities.canonical_name, entities.entity_type, COALESCE(entities.anchor_kind, '')
		FROM semantic_entities AS entities
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.anchor_kind = ? AND scopes.scope_key = ?
	`, anchorKind, scopeKey).Scan(&entity.ID, &entity.CanonicalName, &entity.EntityType, &entity.AnchorKind)
	if errors.Is(err, sql.ErrNoRows) {
		entity.ID, err = newSemanticID()
		entity.Create = true
	}
	return entity, err
}

func contextEntityName(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, scopeKey string) (string, error) {
	kind, registryID, err := splitScopeKey(scopeKey)
	if err != nil {
		return "", err
	}
	var name string
	switch kind {
	case "workspace":
		err = query.QueryRowContext(ctx, `SELECT display_name FROM workspaces WHERE id = ?`, registryID).Scan(&name)
	case "project":
		err = query.QueryRowContext(ctx, `SELECT display_name FROM projects WHERE id = ?`, registryID).Scan(&name)
	case "session":
		var exists int
		err = query.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, registryID).Scan(&exists)
		if err == nil && exists != 1 {
			err = sql.ErrNoRows
		}
		name = "session:" + registryID
	default:
		return "", errors.New("global scope has no Context Entity")
	}
	if err != nil {
		return "", fmt.Errorf("load Context Entity registry identity: %w", err)
	}
	return name, nil
}

func loadEntityByID(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id memory.SemanticID, targetScope, contextScope string) (memory.SemanticEntity, error) {
	var entity memory.SemanticEntity
	err := query.QueryRowContext(ctx, `
		SELECT entities.entity_id, scopes.scope_key, entities.canonical_name, entities.entity_type,
		       COALESCE(entities.anchor_kind, '')
		FROM semantic_entities AS entities
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.entity_id = ? AND entities.lifecycle = 'active'
		  AND (scopes.scope_key = 'global' OR scopes.scope_key = ? OR scopes.scope_key = ?)
	`, id, targetScope, contextScope).Scan(&entity.ID, &entity.ScopeKey, &entity.CanonicalName, &entity.EntityType, &entity.AnchorKind)
	if errors.Is(err, sql.ErrNoRows) {
		return entity, fmt.Errorf("semantic Entity %s is not eligible in scope %s", id, targetScope)
	}
	return entity, err
}

func lookupAliases(ctx context.Context, query interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, targetScope, contextScope, normalized string) ([]memory.AliasEntityMatch, error) {
	rows, err := query.QueryContext(ctx, `
		SELECT entities.entity_id, scopes.scope_key, entities.canonical_name, entities.entity_type,
		       COALESCE(entities.anchor_kind, ''), aliases.alias_id, aliases.value,
		       aliases.normalized_value, aliases.created_operation_id, aliases.source_event_id
		FROM semantic_aliases AS aliases
		JOIN semantic_entities AS entities ON entities.entity_id = aliases.entity_id
		JOIN semantic_scopes AS scopes ON scopes.scope_id = aliases.scope_id
		WHERE (scopes.scope_key = ? OR scopes.scope_key = ? OR scopes.scope_key = 'global') AND aliases.normalized_value = ?
		  AND aliases.lifecycle = 'active' AND entities.lifecycle = 'active'
		ORDER BY scopes.scope_key, entities.entity_id, aliases.alias_id
	`, targetScope, contextScope, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matches []memory.AliasEntityMatch
	for rows.Next() {
		var match memory.AliasEntityMatch
		if err := rows.Scan(&match.Entity.ID, &match.Entity.ScopeKey, &match.Entity.CanonicalName,
			&match.Entity.EntityType, &match.Entity.AnchorKind, &match.Alias.ID, &match.Alias.Value,
			&match.Alias.NormalizedValue, &match.Alias.OperationID, &match.Alias.SourceEventID); err != nil {
			return nil, err
		}
		match.Alias.EntityID = match.Entity.ID
		match.Alias.ScopeKey = match.Entity.ScopeKey
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func resolveEntitySelector(
	ctx context.Context,
	db *sql.DB,
	targetScope string,
	contextScope string,
	selector memory.EntitySelector,
	sourceEventID memory.EventID,
) (memory.SemanticEntity, *memory.SemanticAlias, error) {
	if !utf8.ValidString(selector.CanonicalName) || !utf8.ValidString(selector.EntityType) || !utf8.ValidString(selector.Alias) {
		return memory.SemanticEntity{}, nil, errors.New("Entity selector text must be valid UTF-8")
	}
	if selector.Create {
		if selector.EntityID != "" || strings.TrimSpace(selector.CanonicalName) == "" ||
			strings.TrimSpace(selector.EntityType) == "" || normalizeAlias(selector.Alias) == "" {
			return memory.SemanticEntity{}, nil, errors.New("new Entity requires canonical name, type, and Alias without a preselected ID")
		}
		entityID, err := newSemanticID()
		if err != nil {
			return memory.SemanticEntity{}, nil, err
		}
		aliasID, err := newSemanticID()
		if err != nil {
			return memory.SemanticEntity{}, nil, err
		}
		entity := memory.SemanticEntity{ID: entityID, ScopeKey: targetScope, CanonicalName: selector.CanonicalName,
			EntityType: selector.EntityType, Create: true}
		alias := &memory.SemanticAlias{ID: aliasID, EntityID: entityID, ScopeKey: targetScope, Value: selector.Alias,
			NormalizedValue: normalizeAlias(selector.Alias), SourceEventID: sourceEventID, Create: true}
		return entity, alias, nil
	}
	if selector.EntityID != "" {
		if selector.Alias != "" || selector.CanonicalName != "" || selector.EntityType != "" {
			return memory.SemanticEntity{}, nil, errors.New("stable Entity selection cannot also specify mutable identity fields")
		}
		entity, err := loadEntityByID(ctx, db, selector.EntityID, targetScope, contextScope)
		return entity, nil, err
	}
	normalized := normalizeAlias(selector.Alias)
	if normalized == "" || selector.CanonicalName != "" || selector.EntityType != "" {
		return memory.SemanticEntity{}, nil, errors.New("existing Entity selection requires one stable ID or exact Alias")
	}
	matches, err := lookupAliases(ctx, db, targetScope, contextScope, normalized)
	if err != nil {
		return memory.SemanticEntity{}, nil, err
	}
	if len(matches) > 1 {
		return memory.SemanticEntity{}, nil, ErrAmbiguousAlias
	}
	if len(matches) == 0 {
		return memory.SemanticEntity{}, nil, fmt.Errorf("no Entity has exact Alias %q in scope %s", selector.Alias, targetScope)
	}
	alias := matches[0].Alias
	alias.Create = false
	return matches[0].Entity, &alias, nil
}

func appendUniqueEntity(entities []memory.SemanticEntity, entity memory.SemanticEntity) []memory.SemanticEntity {
	for _, existing := range entities {
		if existing.ID == entity.ID {
			return entities
		}
	}
	return append(entities, entity)
}

func validateEntityIdentity(entity memory.SemanticEntity) error {
	switch entity.AnchorKind {
	case "owner":
		if entity.ScopeKey != "global" || entity.CanonicalName != "owner" || entity.EntityType != "person" {
			return errors.New("invalid canonical owner Entity")
		}
	case "evie":
		if entity.ScopeKey != "global" || entity.CanonicalName != "Evie" || entity.EntityType != "agent" {
			return errors.New("invalid canonical Evie Entity")
		}
	case "context":
		if entity.ScopeKey == "global" || entity.EntityType != "context" {
			return errors.New("invalid Context Entity")
		}
	case "":
		if entity.EntityType == "context" ||
			(entity.ScopeKey == "global" && entity.CanonicalName == "owner" && entity.EntityType == "person") ||
			(entity.ScopeKey == "global" && entity.CanonicalName == "Evie" && entity.EntityType == "agent") {
			return errors.New("ordinary Entity uses a reserved canonical anchor identity")
		}
	default:
		return fmt.Errorf("unknown Entity anchor kind %q", entity.AnchorKind)
	}
	return nil
}

// PrepareRememberEntity resolves all canonical anchors, ordinary Entities,
// Aliases, the Entity-object Claim, and its Source Link for one approval preview.
func (s *Store) PrepareRememberEntity(ctx context.Context, scope memory.ScopeContext, request memory.RememberEntityRequest) (memory.RememberEntityProposal, error) {
	var err error
	request, err = normalizeEntityRequest(request)
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.RememberEntityProposal{}, err
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.RememberEntityProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if len(request.Predicate) > 64 || !predicateTokenPattern.MatchString(request.Predicate) ||
		!utf8.ValidString(request.PredicateLabel) || strings.TrimSpace(request.PredicateLabel) == "" {
		return memory.RememberEntityProposal{}, errors.New("entity Claim requires a valid Predicate token and label")
	}
	var priorJSON, priorHash string
	err = s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&priorJSON, &priorHash)
	if err == nil {
		var proposal memory.RememberEntityProposal
		if decodeErr := json.Unmarshal([]byte(priorJSON), &proposal); decodeErr != nil {
			return proposal, decodeErr
		}
		if proposal.Source.Eligibility == "" {
			proposal.Source.Eligibility = memory.EligibilityEligible
		}
		if !entityRequestsEqual(proposal.Request, request) || proposal.SessionID != scope.SessionID {
			return memory.RememberEntityProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		if err != nil {
			return memory.RememberEntityProposal{}, err
		}
		return proposal, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.RememberEntityProposal{}, err
	}

	targetKey := targetScopeKey(scope, request.UseSessionScope)
	contextKey := scopeKeyForContext(scope)
	source, err := loadOwnerSource(s.db.QueryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
	`, request.SourceEventID), scope.SessionID, request.SourceEventID, targetKey)
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	target, err := loadSemanticScope(ctx, s.db, targetKey)
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	global := target
	if targetKey != "global" {
		global, err = loadSemanticScope(ctx, s.db, "global")
		if err != nil {
			return memory.RememberEntityProposal{}, err
		}
	}
	contextSemanticScope := global
	if contextKey == targetKey {
		contextSemanticScope = target
	} else if contextKey != "global" {
		contextSemanticScope, err = loadSemanticScope(ctx, s.db, contextKey)
		if err != nil {
			return memory.RememberEntityProposal{}, err
		}
	}
	predicate := memory.SemanticPredicate{Token: request.Predicate, Label: request.PredicateLabel,
		ObjectConstraint: memory.ConstraintEntity, Cardinality: request.PredicateCardinality}
	err = s.db.QueryRowContext(ctx, `
		SELECT predicate_id, version, label, object_constraint, cardinality
		FROM semantic_predicates WHERE token = ? ORDER BY version DESC LIMIT 1
	`, predicate.Token).Scan(&predicate.ID, &predicate.Version, &predicate.Label, &predicate.ObjectConstraint, &predicate.Cardinality)
	if errors.Is(err, sql.ErrNoRows) {
		predicate.ID, err = newSemanticID()
		predicate.Version = 1
		predicate.Create = true
	} else if err == nil && (predicate.Label != request.PredicateLabel || predicate.ObjectConstraint != memory.ConstraintEntity ||
		predicate.Cardinality != request.PredicateCardinality) {
		predicate.ID, err = newSemanticID()
		predicate.Version++
		predicate.Label = request.PredicateLabel
		predicate.ObjectConstraint = memory.ConstraintEntity
		predicate.Cardinality = request.PredicateCardinality
		predicate.Create = true
	}
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	owner, err := loadAnchor(ctx, s.db, "global", "owner", "person", "owner")
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	evie, err := loadAnchor(ctx, s.db, "global", "Evie", "agent", "evie")
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	entities := appendUniqueEntity(nil, owner)
	entities = appendUniqueEntity(entities, evie)
	if targetKey != "global" {
		name, err := contextEntityName(ctx, s.db, targetKey)
		if err != nil {
			return memory.RememberEntityProposal{}, err
		}
		contextEntity, err := loadAnchor(ctx, s.db, targetKey, name, "context", "context")
		if err != nil {
			return memory.RememberEntityProposal{}, err
		}
		entities = appendUniqueEntity(entities, contextEntity)
	}
	subject, subjectAlias, err := resolveEntitySelector(ctx, s.db, targetKey, contextKey, request.Subject, request.SourceEventID)
	if err != nil {
		return memory.RememberEntityProposal{}, fmt.Errorf("resolve subject Entity: %w", err)
	}
	object, objectAlias, err := resolveEntitySelector(ctx, s.db, targetKey, contextKey, request.Object, request.SourceEventID)
	if err != nil {
		return memory.RememberEntityProposal{}, fmt.Errorf("resolve object Entity: %w", err)
	}
	entities = appendUniqueEntity(entities, subject)
	entities = appendUniqueEntity(entities, object)
	aliases := make([]memory.SemanticAlias, 0, 2)
	if subjectAlias != nil {
		aliases = append(aliases, *subjectAlias)
	}
	if objectAlias != nil && (subjectAlias == nil || objectAlias.ID != subjectAlias.ID) {
		aliases = append(aliases, *objectAlias)
	}

	claim := memory.SemanticEntityClaim{ScopeKey: targetKey, SubjectEntityID: subject.ID, PredicateID: predicate.ID,
		PredicateToken: predicate.Token, PredicateVersion: predicate.Version, ObjectEntityID: object.ID,
		Polarity: request.Polarity, ValidTime: request.ValidTime}
	err = s.db.QueryRowContext(ctx, `
		SELECT claim_id FROM semantic_claims
		WHERE scope_id = ? AND subject_entity_id = ? AND predicate_id = ? AND object_kind = 'entity'
		  AND object_entity_id = ? AND polarity = ? AND valid_from IS ? AND valid_to IS ?
	`, target.ID, subject.ID, predicate.ID, object.ID, request.Polarity,
		semanticTimeArgument(request.ValidTime.From), semanticTimeArgument(request.ValidTime.To)).Scan(&claim.ID)
	if errors.Is(err, sql.ErrNoRows) {
		claim.ID, err = newSemanticID()
		claim.Create = true
	}
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT source_link_id, created_operation_id FROM semantic_source_links
		WHERE claim_id = ? AND event_id = ? AND event_part = ? AND locator_kind = ?
		  AND locator_value = ? AND evidence_sha256 = ?
	`, claim.ID, source.EventID, source.EventPart, source.LocatorKind,
		source.LocatorValue, source.EvidenceSHA256).Scan(&source.ID, &source.OperationID)
	if errors.Is(err, sql.ErrNoRows) {
		source.ID, err = newSemanticID()
		source.Create = true
	} else if err == nil {
		source.Create = false
	}
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	operationID, err := newSemanticID()
	if err != nil {
		return memory.RememberEntityProposal{}, err
	}
	if source.Create {
		source.OperationID = operationID
	}
	scopes := []memory.SemanticScope{global.SemanticScope}
	if contextKey != "global" {
		scopes = append(scopes, contextSemanticScope.SemanticScope)
	}
	if targetKey != "global" && targetKey != contextKey {
		scopes = append(scopes, target.SemanticScope)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Key < scopes[j].Key })
	priors := make([]memory.ScopeRevision, len(scopes))
	for i := range scopes {
		priors[i] = memory.ScopeRevision{ScopeKey: scopes[i].Key, Revision: scopes[i].Revision}
	}
	proposal := memory.RememberEntityProposal{
		SchemaVersion: 1, Kind: "remember_entity_claim", OperationID: operationID,
		IdempotencyKey: request.IdempotencyKey, Actor: memory.SemanticActorOwner, SessionID: scope.SessionID,
		Scope: target.SemanticScope, Scopes: scopes, PriorRevisions: priors, Predicate: predicate,
		Entities: entities, Aliases: aliases, Claim: claim, Source: source,
		ResultingRevision: target.Revision + 1, Request: request,
	}
	writeGlobal := entityProposalWritesGlobal(proposal)
	for _, semanticScope := range proposal.Scopes {
		revision := semanticScope.Revision
		if semanticScope.Key == proposal.Scope.Key || (semanticScope.Key == "global" && writeGlobal) {
			revision++
		}
		proposal.ResultingRevisions = append(proposal.ResultingRevisions, memory.ScopeRevision{
			ScopeKey: semanticScope.Key, Revision: revision,
		})
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberEntityProposal(proposal))
	if err == nil {
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
	}
	return proposal, err
}

func validateEntityProposalSession(ctx context.Context, writer turnLeaseWriteExecutor, proposal memory.RememberEntityProposal) error {
	expected, expectedScopes, err := authorizedSemanticScopes(ctx, writer, proposal.SessionID, proposal.Request.UseSessionScope)
	if err != nil {
		return err
	}
	if proposal.Scope.Key != expected {
		return errors.New("semantic proposal is outside its immutable session Context Scope")
	}
	return validateAuthorizedSemanticScopes(expected, expectedScopes, proposal.SessionID, proposal.Source.SessionID,
		proposal.Source.ScopeKey, proposal.Scopes)
}

func validateEntityProposalWriteScopes(proposal memory.RememberEntityProposal) error {
	for _, entity := range proposal.Entities {
		if !entity.Create {
			continue
		}
		switch entity.AnchorKind {
		case "owner", "evie":
			if entity.ScopeKey != "global" {
				return errors.New("canonical global anchor moved outside global scope")
			}
		case "context":
			if entity.ScopeKey != proposal.Scope.Key {
				return errors.New("Context Entity moved outside the operation target scope")
			}
		case "":
			if entity.ScopeKey != proposal.Scope.Key {
				return errors.New("ordinary Entity creation outside the target scope requires Promotion")
			}
		}
	}
	for _, alias := range proposal.Aliases {
		if alias.Create && alias.ScopeKey != proposal.Scope.Key {
			return errors.New("Alias creation outside the target scope requires Promotion")
		}
	}
	return nil
}

func validateEntityProposalRelations(proposal memory.RememberEntityProposal) error {
	if proposal.IdempotencyKey != proposal.Request.IdempotencyKey ||
		!strings.HasPrefix(proposal.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(proposal.IdempotencyKey, "idem:v1:")) != nil {
		return errors.New("Entity proposal idempotency key is invalid")
	}
	if proposal.Predicate.Version < 1 ||
		(proposal.Predicate.Cardinality != memory.CardinalityOne && proposal.Predicate.Cardinality != memory.CardinalityMany) ||
		len(proposal.Predicate.Token) > 64 || !predicateTokenPattern.MatchString(proposal.Predicate.Token) ||
		strings.TrimSpace(proposal.Predicate.Label) == "" || !utf8.ValidString(proposal.Predicate.Label) ||
		!utf8.ValidString(proposal.Request.PredicateLabel) || strings.TrimSpace(proposal.Request.PredicateLabel) == "" {
		return errors.New("Entity proposal Predicate is invalid")
	}
	entities := make(map[memory.SemanticID]memory.SemanticEntity, len(proposal.Entities))
	allowedScopes := make(map[string]struct{}, len(proposal.Scopes))
	for _, scope := range proposal.Scopes {
		allowedScopes[scope.Key] = struct{}{}
	}
	for _, entity := range proposal.Entities {
		if !utf8.ValidString(entity.CanonicalName) || !utf8.ValidString(entity.EntityType) ||
			!utf8.ValidString(entity.AnchorKind) {
			return errors.New("Entity proposal text must be valid UTF-8")
		}
		if _, ok := allowedScopes[entity.ScopeKey]; !ok {
			return errors.New("Entity belongs to a scope outside the proposal")
		}
		if strings.TrimSpace(entity.CanonicalName) == "" || strings.TrimSpace(entity.EntityType) == "" {
			return errors.New("Entity requires a canonical name and type")
		}
		if _, duplicate := entities[entity.ID]; duplicate {
			return errors.New("Entity proposal enumerates a duplicate stable ID")
		}
		entities[entity.ID] = entity
	}
	if proposal.Claim.ScopeKey != proposal.Scope.Key ||
		proposal.Claim.PredicateID != proposal.Predicate.ID ||
		proposal.Claim.PredicateToken != proposal.Predicate.Token ||
		proposal.Claim.PredicateVersion != proposal.Predicate.Version ||
		proposal.Claim.Polarity != proposal.Request.Polarity ||
		!validTimesEqual(proposal.Claim.ValidTime, proposal.Request.ValidTime) ||
		proposal.Predicate.Cardinality != proposal.Request.PredicateCardinality ||
		proposal.Predicate.Label != proposal.Request.PredicateLabel {
		return errors.New("Entity Claim does not match its prepared scope or Predicate")
	}
	if proposal.Source.EventID != proposal.Request.SourceEventID || proposal.Source.SessionID != proposal.SessionID ||
		proposal.Source.ScopeKey != proposal.Scope.Key || proposal.Source.EventPart != memory.EvidenceContent ||
		proposal.Source.LocatorKind != memory.LocatorWhole || proposal.Source.LocatorValue != "" ||
		proposal.Source.Actor != memory.SemanticActorOwner || proposal.Source.SourceType != memory.SourceTypeUserMessage ||
		proposal.Source.Authority != memory.AuthorityOwnerStatement || proposal.Source.Eligibility != memory.EligibilityEligible {
		return errors.New("Entity proposal Source is not canonical owner-message provenance")
	}
	if proposal.Source.Create {
		if proposal.Source.OperationID != proposal.OperationID {
			return errors.New("new Source Link does not name its creating operation")
		}
	} else if proposal.Source.OperationID == "" {
		return errors.New("reused Source Link omits its creating operation")
	}
	if _, ok := entities[proposal.Claim.SubjectEntityID]; !ok {
		return errors.New("Entity Claim subject is not enumerated in its proposal")
	}
	if _, ok := entities[proposal.Claim.ObjectEntityID]; !ok {
		return errors.New("Entity Claim object is not enumerated in its proposal")
	}
	for _, alias := range proposal.Aliases {
		if !utf8.ValidString(alias.Value) || !utf8.ValidString(alias.NormalizedValue) {
			return errors.New("Alias text must be valid UTF-8")
		}
		entity, ok := entities[alias.EntityID]
		if !ok {
			return errors.New("Alias target is not enumerated in its proposal")
		}
		if alias.ScopeKey != entity.ScopeKey {
			return errors.New("Alias and Entity scopes do not match")
		}
		if alias.Create && (normalizeAlias(alias.Value) == "" || normalizeAlias(alias.Value) != alias.NormalizedValue ||
			alias.SourceEventID != proposal.Source.EventID) {
			return errors.New("new Alias text or provenance is not canonical")
		}
	}
	for _, selector := range []memory.EntitySelector{proposal.Request.Subject, proposal.Request.Object} {
		if !utf8.ValidString(selector.CanonicalName) || !utf8.ValidString(selector.EntityType) || !utf8.ValidString(selector.Alias) {
			return errors.New("Entity request text must be valid UTF-8")
		}
	}
	return nil
}

func entityProposalWritesGlobal(proposal memory.RememberEntityProposal) bool {
	if proposal.Scope.Key == "global" || proposal.Predicate.Create {
		return true
	}
	for _, entity := range proposal.Entities {
		if entity.Create && entity.ScopeKey == "global" {
			return true
		}
	}
	return false
}

// ApplyRememberEntity revalidates and atomically accepts the prepared compound effect.
func (s *Store) ApplyRememberEntity(ctx context.Context, lease memory.TurnLease, proposal memory.RememberEntityProposal) (result memory.RememberEntityResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("semantic proposal does not match its turn lease")
	}
	canonical := canonicalRememberEntityProposal(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedProposalJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != proposalHash ||
		proposal.PreparedSHA256 == "" || proposal.PreparedSHA256 != preparedHash {
		return result, errors.New("semantic proposal hash changed")
	}
	if proposal.SchemaVersion != 1 || proposal.Kind != "remember_entity_claim" || proposal.Actor != memory.SemanticActorOwner ||
		(proposal.Claim.Polarity != memory.PolarityAffirmed && proposal.Claim.Polarity != memory.PolarityDenied) ||
		proposal.Predicate.ObjectConstraint != memory.ConstraintEntity || proposal.ResultingRevision != proposal.Scope.Revision+1 ||
		len(proposal.ResultingRevisions) != len(proposal.Scopes) {
		return result, errors.New("invalid Entity-object Claim proposal")
	}
	if normalized, validTimeErr := normalizeValidTime(proposal.Claim.ValidTime); validTimeErr != nil ||
		!validTimesEqual(normalized, proposal.Claim.ValidTime) {
		return result, errors.New("invalid Entity-object Claim Valid Time")
	}
	ids := []memory.SemanticID{proposal.OperationID, proposal.Predicate.ID, proposal.Claim.ID, proposal.Source.ID, proposal.Scope.ID}
	for _, entity := range proposal.Entities {
		if err := validateEntityIdentity(entity); err != nil {
			return result, err
		}
		ids = append(ids, entity.ID)
	}
	for _, alias := range proposal.Aliases {
		ids = append(ids, alias.ID, alias.EntityID)
	}
	ids = append(ids, proposal.Source.OperationID)
	for _, id := range ids {
		if err := validateSemanticUUID(string(id)); err != nil {
			return result, err
		}
	}
	if err := validateEntityProposalWriteScopes(proposal); err != nil {
		return result, err
	}
	if err := validateEntityProposalRelations(proposal); err != nil {
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
		if err := validateEntityProposalSession(ctx, writer, proposal); err != nil {
			return err
		}
		source, err := loadOwnerSource(writer.queryRowContext(ctx, `
			SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
		`, proposal.Source.EventID), proposal.SessionID, proposal.Source.EventID, proposal.Scope.Key)
		if err != nil || source.EventID != proposal.Source.EventID || source.SessionID != proposal.Source.SessionID ||
			source.ScopeKey != proposal.Source.ScopeKey || source.EventPart != proposal.Source.EventPart ||
			source.LocatorKind != proposal.Source.LocatorKind || source.LocatorValue != proposal.Source.LocatorValue ||
			source.Evidence != proposal.Source.Evidence || source.EvidenceSHA256 != proposal.Source.EvidenceSHA256 ||
			source.Actor != proposal.Source.Actor || source.SourceType != proposal.Source.SourceType ||
			source.Authority != proposal.Source.Authority || source.ObservedAt != proposal.Source.ObservedAt ||
			source.Eligibility != proposal.Source.Eligibility {
			return errors.New("semantic source evidence changed")
		}

		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions)
		if err != nil {
			return err
		}
		if target, ok := byKey[proposal.Scope.Key]; !ok || target != proposal.Scope {
			return errors.New("semantic proposal target scope does not match its revision vector")
		}

		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		transactionText := formatSemanticTime(now)
		writeGlobal := entityProposalWritesGlobal(proposal)
		for _, scope := range proposal.Scopes {
			revision := scope.Revision
			if scope.Key == proposal.Scope.Key || (scope.Key == "global" && writeGlobal) {
				revision++
			}
			result.ResultingRevisions = append(result.ResultingRevisions, memory.ScopeRevision{ScopeKey: scope.Key, Revision: revision})
			if scope.Key == proposal.Scope.Key {
				result.ScopeRevision = revision
			}
		}
		for index := range result.ResultingRevisions {
			if result.ResultingRevisions[index] != proposal.ResultingRevisions[index] {
				return errors.New("semantic proposal resulting revision vector changed")
			}
		}
		result.OperationID, result.ClaimID, result.SourceLinkID, result.TransactionTime = proposal.OperationID, proposal.Claim.ID, proposal.Source.ID, now
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		effectHash, _, err := semanticHash(canonical.Effect)
		if err != nil {
			return err
		}
		if err := recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{
			SchemaVersion: proposal.SchemaVersion,
			OperationID:   proposal.OperationID, Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey,
			Actor: proposal.Actor, SessionID: proposal.SessionID, TargetScopeID: proposal.Scope.ID,
			SourceEventID: proposal.Source.EventID, ProposalHash: proposalHash, EffectHash: effectHash,
			ProposalJSON: proposalJSON, PreparedJSON: preparedProposalJSON, ResultJSON: resultJSON,
			TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey,
		}); err != nil {
			return err
		}
		if proposal.Predicate.Create {
			var latest int64
			err := writer.queryRowContext(ctx, `SELECT version FROM semantic_predicates WHERE token = ? ORDER BY version DESC LIMIT 1`,
				proposal.Predicate.Token).Scan(&latest)
			if errors.Is(err, sql.ErrNoRows) {
				latest = 0
			} else if err != nil {
				return err
			}
			if proposal.Predicate.Version != latest+1 {
				return errors.New("semantic Predicate version changed after preparation")
			}
			if _, err := writer.execContext(ctx, `INSERT INTO semantic_predicates (predicate_id, token, version, label, object_constraint, cardinality, created_operation_id) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				proposal.Predicate.ID, proposal.Predicate.Token, proposal.Predicate.Version, proposal.Predicate.Label,
				proposal.Predicate.ObjectConstraint, proposal.Predicate.Cardinality, proposal.OperationID); err != nil {
				return err
			}
		} else {
			var id, label, objectConstraint, cardinality string
			if err := writer.queryRowContext(ctx, `
				SELECT predicate_id, label, object_constraint, cardinality
				FROM semantic_predicates WHERE token = ? AND version = ?
			`, proposal.Predicate.Token, proposal.Predicate.Version).Scan(&id, &label, &objectConstraint, &cardinality); err != nil ||
				id != string(proposal.Predicate.ID) || label != proposal.Predicate.Label ||
				objectConstraint != string(proposal.Predicate.ObjectConstraint) || cardinality != string(proposal.Predicate.Cardinality) {
				return errors.New("semantic Predicate changed after preparation")
			}
		}
		for _, entity := range proposal.Entities {
			scope, ok := byKey[entity.ScopeKey]
			if !ok {
				return errors.New("Entity belongs to a scope outside the proposal")
			}
			if entity.Create {
				if _, err := writer.execContext(ctx, `INSERT INTO semantic_entities (entity_id, scope_id, canonical_name, entity_type, anchor_kind, lifecycle, created_operation_id) VALUES (?, ?, ?, ?, NULLIF(?, ''), 'active', ?)`,
					entity.ID, scope.ID, entity.CanonicalName, entity.EntityType, entity.AnchorKind, proposal.OperationID); err != nil {
					return err
				}
			} else {
				var name, entityType, anchor string
				if err := writer.queryRowContext(ctx, `SELECT canonical_name, entity_type, COALESCE(anchor_kind, '') FROM semantic_entities WHERE entity_id = ? AND scope_id = ? AND lifecycle = 'active'`,
					entity.ID, scope.ID).Scan(&name, &entityType, &anchor); err != nil || name != entity.CanonicalName || entityType != entity.EntityType || anchor != entity.AnchorKind {
					return errors.New("semantic Entity changed after preparation")
				}
			}
		}
		for _, alias := range proposal.Aliases {
			if alias.Create {
				scope := byKey[alias.ScopeKey]
				if normalizeAlias(alias.Value) != alias.NormalizedValue || alias.SourceEventID != proposal.Source.EventID {
					return errors.New("semantic Alias changed after preparation")
				}
				if _, err := writer.execContext(ctx, `INSERT INTO semantic_aliases (alias_id, entity_id, scope_id, value, normalized_value, lifecycle, source_event_id, created_operation_id) VALUES (?, ?, ?, ?, ?, 'active', ?, ?)`,
					alias.ID, alias.EntityID, scope.ID, alias.Value, alias.NormalizedValue, alias.SourceEventID, proposal.OperationID); err != nil {
					return err
				}
			} else {
				var entityID, scopeKey, value, normalizedValue, operationID, sourceEventID string
				if err := writer.queryRowContext(ctx, `
					SELECT aliases.entity_id, scopes.scope_key, aliases.value, aliases.normalized_value,
					       aliases.created_operation_id, aliases.source_event_id
					FROM semantic_aliases AS aliases
					JOIN semantic_scopes AS scopes ON scopes.scope_id = aliases.scope_id
					WHERE aliases.alias_id = ? AND aliases.lifecycle = 'active'
				`, alias.ID).Scan(&entityID, &scopeKey, &value, &normalizedValue, &operationID, &sourceEventID); err != nil ||
					entityID != string(alias.EntityID) || scopeKey != alias.ScopeKey || value != alias.Value ||
					normalizedValue != alias.NormalizedValue || operationID != string(alias.OperationID) ||
					sourceEventID != string(alias.SourceEventID) {
					return errors.New("semantic Alias changed after preparation")
				}
			}
		}
		if proposal.Claim.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_claims (
					claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
					object_kind, object_entity_id, polarity, valid_from, valid_to, lifecycle, created_operation_id, transaction_time
				) VALUES (?, ?, ?, ?, ?, ?, 'entity', ?, ?, ?, ?, 'active', ?, ?)
			`, proposal.Claim.ID, proposal.Scope.ID, proposal.Claim.SubjectEntityID, proposal.Predicate.ID,
				proposal.Predicate.Token, proposal.Predicate.Version, proposal.Claim.ObjectEntityID, proposal.Claim.Polarity,
				semanticTimeArgument(proposal.Claim.ValidTime.From), semanticTimeArgument(proposal.Claim.ValidTime.To),
				proposal.OperationID, transactionText); err != nil {
				return err
			}
		} else {
			var id string
			if err := writer.queryRowContext(ctx, `SELECT claim_id FROM semantic_claims WHERE claim_id = ? AND scope_id = ? AND subject_entity_id = ? AND predicate_id = ? AND object_kind = 'entity' AND object_entity_id = ? AND polarity = ? AND valid_from IS ? AND valid_to IS ?`,
				proposal.Claim.ID, proposal.Scope.ID, proposal.Claim.SubjectEntityID, proposal.Predicate.ID,
				proposal.Claim.ObjectEntityID, proposal.Claim.Polarity,
				semanticTimeArgument(proposal.Claim.ValidTime.From), semanticTimeArgument(proposal.Claim.ValidTime.To)).Scan(&id); err != nil {
				return errors.New("semantic Claim changed after preparation")
			}
		}
		if proposal.Source.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_source_links (
					source_link_id, claim_id, event_id, source_session_id, source_scope_key,
					event_part, locator_kind, locator_value, evidence_sha256, source_actor,
					source_type, authority, observed_at, eligibility, created_operation_id
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'eligible', ?)
			`, proposal.Source.ID, proposal.Claim.ID, proposal.Source.EventID, proposal.SessionID, proposal.Source.ScopeKey,
				proposal.Source.EventPart, proposal.Source.LocatorKind, proposal.Source.LocatorValue,
				proposal.Source.EvidenceSHA256, proposal.Source.Actor, proposal.Source.SourceType,
				proposal.Source.Authority, proposal.Source.ObservedAt, proposal.OperationID); err != nil {
				return err
			}
		} else {
			var id, claimID, eventID, sessionID, scopeKey, eventPart, locatorKind, locatorValue string
			var evidenceHash, actor, sourceType, authority, observedAt, eligibility, operationID string
			if err := writer.queryRowContext(ctx, `
				SELECT source_link_id, claim_id, event_id, source_session_id, source_scope_key,
				       event_part, locator_kind, locator_value, evidence_sha256, source_actor,
				       source_type, authority, observed_at, eligibility, created_operation_id
				FROM semantic_source_links WHERE source_link_id = ?
			`, proposal.Source.ID).Scan(&id, &claimID, &eventID, &sessionID, &scopeKey, &eventPart,
				&locatorKind, &locatorValue, &evidenceHash, &actor, &sourceType, &authority,
				&observedAt, &eligibility, &operationID); err != nil || id != string(proposal.Source.ID) ||
				claimID != string(proposal.Claim.ID) || eventID != string(proposal.Source.EventID) ||
				sessionID != string(proposal.Source.SessionID) || scopeKey != proposal.Source.ScopeKey ||
				eventPart != string(proposal.Source.EventPart) || locatorKind != string(proposal.Source.LocatorKind) ||
				locatorValue != proposal.Source.LocatorValue || evidenceHash != proposal.Source.EvidenceSHA256 ||
				actor != string(proposal.Source.Actor) || sourceType != string(proposal.Source.SourceType) ||
				authority != string(proposal.Source.Authority) || observedAt != proposal.Source.ObservedAt ||
				eligibility != "eligible" || operationID != string(proposal.Source.OperationID) {
				return errors.New("semantic Source Link changed after preparation")
			}
		}
		stateRevision := result.ScopeRevision
		for _, entity := range proposal.Entities {
			if entity.Create {
				scope := byKey[entity.ScopeKey]
				revision := stateRevision
				for _, item := range result.ResultingRevisions {
					if item.ScopeKey == entity.ScopeKey {
						revision = item.Revision
					}
				}
				if _, err := writer.execContext(ctx, `INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time) VALUES (?, 'entity', ?, 'active', ?, ?, ?)`,
					scope.ID, entity.ID, proposal.OperationID, revision, transactionText); err != nil {
					return err
				}
			}
		}
		for _, alias := range proposal.Aliases {
			if alias.Create {
				if _, err := writer.execContext(ctx, `INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time) VALUES (?, 'alias', ?, 'active', ?, ?, ?)`,
					byKey[alias.ScopeKey].ID, alias.ID, proposal.OperationID, stateRevision, transactionText); err != nil {
					return err
				}
			}
		}
		if proposal.Claim.Create {
			if _, err := writer.execContext(ctx, `INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time) VALUES (?, 'claim', ?, 'active', ?, ?, ?)`,
				proposal.Scope.ID, proposal.Claim.ID, proposal.OperationID, stateRevision, transactionText); err != nil {
				return err
			}
		}
		if proposal.Source.Create {
			if _, err := writer.execContext(ctx, `INSERT INTO semantic_state_events (scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time) VALUES (?, 'source_link', ?, 'eligible', ?, ?, ?)`,
				proposal.Scope.ID, proposal.Source.ID, proposal.OperationID, stateRevision, transactionText); err != nil {
				return err
			}
		}
		for _, revision := range result.ResultingRevisions {
			scope := byKey[revision.ScopeKey]
			if revision.Revision != scope.Revision {
				result, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`, revision.Revision, scope.ID, scope.Revision)
				if err != nil {
					return err
				}
				if changed, _ := result.RowsAffected(); changed != 1 {
					return ErrStaleScopeRevision
				}
			}
		}
		return nil
	})
	return result, err
}

// LookupEntitiesByAlias returns every current exact match in canonical ID order.
func (s *Store) LookupEntitiesByAlias(ctx context.Context, scope memory.ScopeContext, alias string) ([]memory.AliasEntityMatch, error) {
	return s.LookupEntitiesByAliasAtScope(ctx, scope, alias, false)
}

// LookupEntitiesByAliasAtScope selects either the default Context Scope or the
// current session Scope and returns matches from every scope eligible there.
func (s *Store) LookupEntitiesByAliasAtScope(ctx context.Context, scope memory.ScopeContext, alias string, useSessionScope bool) ([]memory.AliasEntityMatch, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return nil, err
	}
	normalized := normalizeAlias(alias)
	if normalized == "" {
		return nil, errors.New("Alias must not be blank")
	}
	return lookupAliases(ctx, s.db, targetScopeKey(scope, useSessionScope), scopeKeyForContext(scope), normalized)
}

// InspectSemanticEntity performs an exact stable-ID read inside the bound scope.
func (s *Store) InspectSemanticEntity(ctx context.Context, scope memory.ScopeContext, id memory.SemanticID) (memory.SemanticEntity, error) {
	return s.InspectSemanticEntityAtScope(ctx, scope, id, false)
}

// InspectSemanticEntityAtScope selects either the default Context Scope or the
// current session Scope while preserving the exact eligible-scope matrix.
func (s *Store) InspectSemanticEntityAtScope(ctx context.Context, scope memory.ScopeContext, id memory.SemanticID, useSessionScope bool) (memory.SemanticEntity, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.SemanticEntity{}, err
	}
	return loadEntityByID(ctx, s.db, id, targetScopeKey(scope, useSessionScope), scopeKeyForContext(scope))
}
