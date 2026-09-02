package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

var (
	ErrStaleScopeRevision  = errors.New("semantic memory: stale scope revision")
	ErrIdempotencyConflict = errors.New("semantic memory: idempotency conflict")
	predicateTokenPattern  = regexp.MustCompile(`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`)
	integerPattern         = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)
	decimalPattern         = regexp.MustCompile(`^(0|-?[1-9][0-9]*|-?(0|[1-9][0-9]*)\.[0-9]*[1-9])$`)
)

const semanticTimestampLayout = "2006-01-02T15:04:05.000000000Z"

const semanticSchema = `
CREATE TABLE IF NOT EXISTS semantic_scopes (
    scope_id    TEXT PRIMARY KEY NOT NULL,
    scope_key   TEXT NOT NULL UNIQUE,
    scope_kind  TEXT NOT NULL CHECK (scope_kind IN ('global', 'workspace', 'project', 'session')),
    registry_id TEXT,
    revision    INTEGER NOT NULL DEFAULT 0 CHECK (typeof(revision) = 'integer' AND revision >= 0),
    CHECK ((scope_kind = 'global' AND scope_key = 'global' AND registry_id IS NULL)
        OR (scope_kind != 'global' AND registry_id IS NOT NULL AND scope_key = scope_kind || ':' || registry_id))
);

CREATE TABLE IF NOT EXISTS semantic_operations (
    operation_id       TEXT PRIMARY KEY NOT NULL,
    schema_version     INTEGER NOT NULL CHECK (schema_version = 1),
    operation_kind     TEXT NOT NULL,
    idempotency_key    TEXT NOT NULL UNIQUE,
    actor               TEXT NOT NULL,
    session_id          TEXT NOT NULL REFERENCES sessions(id),
    target_scope_id     TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    source_event_id     TEXT NOT NULL REFERENCES events(id),
    proposal_sha256     TEXT NOT NULL,
    effect_sha256       TEXT NOT NULL,
    proposal_json       TEXT NOT NULL CHECK (json_valid(proposal_json) AND json_type(proposal_json) = 'object'),
    prepared_proposal_json TEXT NOT NULL CHECK (json_valid(prepared_proposal_json) AND json_type(prepared_proposal_json) = 'object'),
    result_json         TEXT NOT NULL CHECK (json_valid(result_json) AND json_type(result_json) = 'object'),
    transaction_time    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS semantic_operation_scopes (
    operation_id       TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    scope_id            TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    prior_revision      INTEGER NOT NULL CHECK (prior_revision >= 0),
    resulting_revision  INTEGER NOT NULL CHECK (resulting_revision >= prior_revision),
    written             INTEGER NOT NULL CHECK (written IN (0, 1)),
    PRIMARY KEY (operation_id, scope_id)
);

CREATE TABLE IF NOT EXISTS semantic_predicates (
    predicate_id       TEXT PRIMARY KEY NOT NULL,
    token              TEXT NOT NULL,
    version            INTEGER NOT NULL CHECK (version > 0),
    label              TEXT NOT NULL,
    object_constraint  TEXT NOT NULL CHECK (object_constraint IN ('entity', 'text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
    cardinality        TEXT NOT NULL CHECK (cardinality IN ('one', 'many')),
    created_operation_id TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    UNIQUE (token, version)
);

CREATE TABLE IF NOT EXISTS semantic_entities (
    entity_id            TEXT PRIMARY KEY NOT NULL,
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    canonical_name        TEXT NOT NULL,
    entity_type           TEXT NOT NULL,
    anchor_kind           TEXT CHECK (anchor_kind IS NULL OR anchor_kind IN ('owner', 'evie', 'context')),
    lifecycle             TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired')),
    created_operation_id  TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    UNIQUE (scope_id, anchor_kind)
);

CREATE TABLE IF NOT EXISTS semantic_claims (
    claim_id              TEXT PRIMARY KEY NOT NULL,
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    subject_entity_id     TEXT NOT NULL REFERENCES semantic_entities(entity_id),
    predicate_id          TEXT NOT NULL REFERENCES semantic_predicates(predicate_id),
    predicate_token       TEXT NOT NULL,
    predicate_version     INTEGER NOT NULL CHECK (predicate_version > 0),
    literal_kind          TEXT NOT NULL CHECK (literal_kind IN ('text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
    literal_value         TEXT NOT NULL,
    polarity              TEXT NOT NULL CHECK (polarity IN ('affirmed', 'denied')),
    valid_from            TEXT,
    valid_to              TEXT,
    lifecycle             TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired', 'superseded')),
    created_operation_id  TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    transaction_time      TEXT NOT NULL,
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to)
);

CREATE TABLE IF NOT EXISTS semantic_source_links (
    source_link_id       TEXT PRIMARY KEY NOT NULL,
    claim_id             TEXT NOT NULL REFERENCES semantic_claims(claim_id),
    event_id             TEXT NOT NULL REFERENCES events(id),
    source_session_id    TEXT NOT NULL REFERENCES sessions(id),
    source_scope_key     TEXT NOT NULL,
    event_part           TEXT NOT NULL CHECK (event_part IN ('content', 'payload')),
    locator_kind         TEXT NOT NULL CHECK (locator_kind IN ('whole', 'utf8_byte_range', 'json_pointer')),
    locator_value        TEXT NOT NULL,
    evidence_sha256      TEXT NOT NULL,
    source_actor         TEXT NOT NULL,
    source_type          TEXT NOT NULL,
    authority            TEXT NOT NULL,
    observed_at          TEXT NOT NULL,
    eligibility          TEXT NOT NULL CHECK (eligibility IN ('eligible', 'retracted')),
    created_operation_id TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    UNIQUE (claim_id, event_id, event_part, locator_kind, locator_value, evidence_sha256)
);

CREATE TABLE IF NOT EXISTS semantic_state_events (
    scope_id             TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    object_kind          TEXT NOT NULL CHECK (object_kind IN ('entity', 'alias', 'claim', 'source_link', 'graph_link')),
    object_id            TEXT NOT NULL,
    state                TEXT NOT NULL,
    operation_id         TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    scope_revision       INTEGER NOT NULL CHECK (scope_revision > 0),
    transaction_time     TEXT NOT NULL,
    PRIMARY KEY (operation_id, object_kind, object_id, state)
);

CREATE INDEX IF NOT EXISTS semantic_claims_scope_idx ON semantic_claims(scope_id, lifecycle, claim_id);
CREATE INDEX IF NOT EXISTS semantic_source_links_claim_idx ON semantic_source_links(claim_id, eligibility, source_link_id);
CREATE INDEX IF NOT EXISTS semantic_state_events_object_idx ON semantic_state_events(object_kind, object_id, scope_revision);

CREATE TRIGGER IF NOT EXISTS semantic_operations_append_only_update BEFORE UPDATE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_operations_append_only_delete BEFORE DELETE ON semantic_operations BEGIN SELECT RAISE(ABORT, 'semantic operations are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_operation_scopes_append_only_update BEFORE UPDATE ON semantic_operation_scopes BEGIN SELECT RAISE(ABORT, 'semantic operation scopes are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_operation_scopes_append_only_delete BEFORE DELETE ON semantic_operation_scopes BEGIN SELECT RAISE(ABORT, 'semantic operation scopes are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_predicates_append_only_update BEFORE UPDATE ON semantic_predicates BEGIN SELECT RAISE(ABORT, 'semantic predicates are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_predicates_append_only_delete BEFORE DELETE ON semantic_predicates BEGIN SELECT RAISE(ABORT, 'semantic predicates are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_entities_append_only_update BEFORE UPDATE ON semantic_entities BEGIN SELECT RAISE(ABORT, 'semantic entities are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_entities_append_only_delete BEFORE DELETE ON semantic_entities BEGIN SELECT RAISE(ABORT, 'semantic entities are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_claims_append_only_delete BEFORE DELETE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_source_links_append_only_update BEFORE UPDATE ON semantic_source_links BEGIN SELECT RAISE(ABORT, 'semantic source links are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_source_links_append_only_delete BEFORE DELETE ON semantic_source_links BEGIN SELECT RAISE(ABORT, 'semantic source links are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_state_events_append_only_update BEFORE UPDATE ON semantic_state_events BEGIN SELECT RAISE(ABORT, 'semantic state events are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_state_events_append_only_delete BEFORE DELETE ON semantic_state_events BEGIN SELECT RAISE(ABORT, 'semantic state events are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_scopes_identity_immutable
BEFORE UPDATE OF scope_id, scope_key, scope_kind, registry_id ON semantic_scopes
BEGIN SELECT RAISE(ABORT, 'semantic scope identity is immutable'); END;
`

func ensureSemanticSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, semanticSchema)
	return err
}

func newSemanticID() (memory.SemanticID, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return memory.SemanticID(id.String()), nil
}

func formatSemanticTime(value time.Time) string {
	return value.UTC().Format(semanticTimestampLayout)
}

func parseSemanticTime(value string) (time.Time, error) {
	return time.Parse(semanticTimestampLayout, value)
}

func validateSemanticUUID(value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed.Version() != 4 || parsed.String() != value {
		return fmt.Errorf("%q is not a canonical UUIDv4", value)
	}
	return nil
}

func validateLiteral(literal memory.TypedLiteral) error {
	switch literal.Kind {
	case memory.LiteralText:
		return nil
	case memory.LiteralInteger:
		if integerPattern.MatchString(literal.Value) {
			return nil
		}
	case memory.LiteralDecimal:
		if decimalPattern.MatchString(literal.Value) {
			return nil
		}
	case memory.LiteralBoolean:
		if literal.Value == "true" || literal.Value == "false" {
			return nil
		}
	case memory.LiteralDate:
		parsed, err := time.Parse("2006-01-02", literal.Value)
		if err == nil && parsed.Format("2006-01-02") == literal.Value {
			return nil
		}
	case memory.LiteralDatetime:
		if parsed, err := parseSemanticTime(literal.Value); err == nil && formatSemanticTime(parsed) == literal.Value {
			return nil
		}
	default:
		return fmt.Errorf("unsupported literal kind %q", literal.Kind)
	}
	return fmt.Errorf("literal %q is not canonical for kind %q", literal.Value, literal.Kind)
}

type semanticScopeDraft struct {
	memory.SemanticScope
	Kind   string
	Create bool
}

func scopeKeyForContext(scope memory.ScopeContext) string {
	switch {
	case scope.WorkspaceID != "":
		return "workspace:" + string(scope.WorkspaceID)
	case scope.ProjectID != "":
		return "project:" + string(scope.ProjectID)
	default:
		return "global"
	}
}

func splitScopeKey(key string) (kind, registryID string, err error) {
	if key == "global" {
		return "global", "", nil
	}
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 || (parts[0] != "workspace" && parts[0] != "project" && parts[0] != "session") {
		return "", "", fmt.Errorf("invalid semantic scope key %q", key)
	}
	if err := validateSemanticUUID(parts[1]); err != nil {
		return "", "", fmt.Errorf("invalid semantic scope registry identity: %w", err)
	}
	return parts[0], parts[1], nil
}

func loadSemanticScope(ctx context.Context, query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (semanticScopeDraft, error) {
	var draft semanticScopeDraft
	var registry sql.NullString
	err := query.QueryRowContext(ctx, `
		SELECT scope_id, scope_key, scope_kind, registry_id, revision
		FROM semantic_scopes WHERE scope_key = ?
	`, key).Scan(&draft.ID, &draft.Key, &draft.Kind, &registry, &draft.Revision)
	if err == nil {
		if registry.Valid {
			draft.RegistryID = registry.String
		}
		return draft, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return semanticScopeDraft{}, err
	}
	kind, registryID, err := splitScopeKey(key)
	if err != nil {
		return semanticScopeDraft{}, err
	}
	id, err := newSemanticID()
	if err != nil {
		return semanticScopeDraft{}, err
	}
	return semanticScopeDraft{SemanticScope: memory.SemanticScope{
		ID: id, Key: key, RegistryID: registryID, Revision: 0,
	}, Kind: kind, Create: true}, nil
}

func validateSessionScope(ctx context.Context, db *sql.DB, scope memory.ScopeContext) error {
	var workspaceID, projectID sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = ?`,
		scope.SessionID, memory.SessionActive).Scan(&workspaceID, &projectID); err != nil {
		return fmt.Errorf("load active source session: %w", err)
	}
	if workspaceID.String != string(scope.WorkspaceID) || workspaceID.Valid != (scope.WorkspaceID != "") ||
		projectID.String != string(scope.ProjectID) || projectID.Valid != (scope.ProjectID != "") {
		return errors.New("semantic request scope does not match its session")
	}
	return nil
}

// PrepareRememberLiteral resolves a complete, immutable proposal without
// changing Semantic Memory. The exact cited owner event is loaded from SQLite;
// callers cannot supply source attributes or scope.
func (s *Store) PrepareRememberLiteral(
	ctx context.Context,
	scope memory.ScopeContext,
	request memory.RememberLiteralRequest,
) (memory.RememberLiteralProposal, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.RememberLiteralProposal{}, err
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") ||
		validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.RememberLiteralProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if len(request.Predicate) > 64 || !predicateTokenPattern.MatchString(request.Predicate) {
		return memory.RememberLiteralProposal{}, fmt.Errorf("invalid Predicate token %q", request.Predicate)
	}
	if strings.TrimSpace(request.PredicateLabel) == "" {
		return memory.RememberLiteralProposal{}, errors.New("Predicate label must not be blank")
	}
	if err := validateLiteral(request.Literal); err != nil {
		return memory.RememberLiteralProposal{}, err
	}

	var priorProposalJSON, priorHash string
	err := s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`,
		request.IdempotencyKey).Scan(&priorProposalJSON, &priorHash)
	if err == nil {
		var proposal memory.RememberLiteralProposal
		if err := json.Unmarshal([]byte(priorProposalJSON), &proposal); err != nil {
			return memory.RememberLiteralProposal{}, fmt.Errorf("decode accepted proposal: %w", err)
		}
		if proposal.SessionID != scope.SessionID || proposal.Source.EventID != request.SourceEventID ||
			proposal.Predicate.Token != request.Predicate || proposal.Predicate.Label != request.PredicateLabel ||
			proposal.Literal != request.Literal {
			return memory.RememberLiteralProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		return proposal, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.RememberLiteralProposal{}, fmt.Errorf("check idempotency key: %w", err)
	}

	var eventSession string
	var eventType, role, content, recordedAt string
	if err := s.db.QueryRowContext(ctx, `
		SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at
		FROM events WHERE id = ?
	`, request.SourceEventID).Scan(&eventSession, &eventType, &role, &content, &recordedAt); err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("load source event: %w", err)
	}
	if eventSession != string(scope.SessionID) || eventType != string(memory.EventUserMessage) || role != string(memory.RoleUser) {
		return memory.RememberLiteralProposal{}, errors.New("source event must be an owner user message in the bound session")
	}
	observed, err := time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("parse source event time: %w", err)
	}

	targetKey := scopeKeyForContext(scope)
	target, err := loadSemanticScope(ctx, s.db, targetKey)
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("resolve target scope: %w", err)
	}
	global := target
	if targetKey != "global" {
		global, err = loadSemanticScope(ctx, s.db, "global")
		if err != nil {
			return memory.RememberLiteralProposal{}, fmt.Errorf("resolve global scope: %w", err)
		}
	}

	predicate := memory.SemanticPredicate{Token: request.Predicate, Version: 1, Label: request.PredicateLabel,
		ObjectConstraint: request.Literal.Kind, Cardinality: "one"}
	err = s.db.QueryRowContext(ctx, `
		SELECT predicate_id, label, object_constraint, cardinality
		FROM semantic_predicates WHERE token = ? AND version = 1
	`, predicate.Token).Scan(&predicate.ID, &predicate.Label, &predicate.ObjectConstraint, &predicate.Cardinality)
	if errors.Is(err, sql.ErrNoRows) {
		predicate.ID, err = newSemanticID()
		predicate.Create = true
	} else if err == nil && (predicate.ObjectConstraint != request.Literal.Kind || predicate.Cardinality != "one") {
		return memory.RememberLiteralProposal{}, errors.New("Predicate definition does not accept this literal")
	}
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("resolve Predicate: %w", err)
	}

	subject := memory.SemanticEntity{ScopeKey: "global", CanonicalName: "owner", EntityType: "person", AnchorKind: "owner"}
	err = s.db.QueryRowContext(ctx, `
		SELECT entities.entity_id
		FROM semantic_entities AS entities
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.anchor_kind = 'owner' AND scopes.scope_key = 'global'
	`).Scan(&subject.ID)
	if errors.Is(err, sql.ErrNoRows) {
		subject.ID, err = newSemanticID()
		subject.Create = true
	}
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("resolve owner anchor: %w", err)
	}
	evie := memory.SemanticEntity{ScopeKey: "global", CanonicalName: "Evie", EntityType: "agent", AnchorKind: "evie"}
	err = s.db.QueryRowContext(ctx, `
		SELECT entities.entity_id
		FROM semantic_entities AS entities
		JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
		WHERE entities.anchor_kind = 'evie' AND scopes.scope_key = 'global'
	`).Scan(&evie.ID)
	if errors.Is(err, sql.ErrNoRows) {
		evie.ID, err = newSemanticID()
		evie.Create = true
	}
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("resolve Evie anchor: %w", err)
	}

	operationID, err := newSemanticID()
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}
	claimID, err := newSemanticID()
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}
	sourceLinkID, err := newSemanticID()
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}
	digest := sha256.Sum256([]byte(content))

	scopes := []memory.SemanticScope{global.SemanticScope}
	if targetKey != "global" {
		scopes = append(scopes, target.SemanticScope)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].Key < scopes[j].Key })
	prior := make([]memory.ScopeRevision, len(scopes))
	for i := range scopes {
		prior[i] = memory.ScopeRevision{ScopeKey: scopes[i].Key, Revision: scopes[i].Revision}
	}
	proposal := memory.RememberLiteralProposal{
		SchemaVersion: 1, Kind: "remember_literal_claim", OperationID: operationID,
		IdempotencyKey: request.IdempotencyKey, Actor: "owner", SessionID: scope.SessionID,
		Scope: target.SemanticScope, Scopes: scopes, PriorRevisions: prior, ExpectedRevision: target.Revision,
		Predicate: predicate, Subject: subject, Evie: evie, ClaimID: claimID, SourceLinkID: sourceLinkID,
		Literal: request.Literal, Polarity: "affirmed", ValidTime: memory.ValidTime{},
		Source: memory.SemanticSource{
			EventID: request.SourceEventID, SessionID: scope.SessionID, ScopeKey: targetKey,
			EventPart: "content", LocatorKind: "whole", LocatorValue: "",
			EvidenceSHA256: fmt.Sprintf("sha256:%x", digest), Actor: "owner", SourceType: "user_message",
			Authority: "owner_statement", ObservedAt: formatSemanticTime(observed), Evidence: content,
		},
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberLiteralProposal(proposal))
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("hash proposal: %w", err)
	}
	return proposal, nil
}

func proposalWritesGlobal(proposal memory.RememberLiteralProposal) bool {
	return proposal.Scope.Key == "global" || proposal.Predicate.Create || proposal.Subject.Create || proposal.Evie.Create
}

func validateScopeBacking(ctx context.Context, writer turnLeaseWriteExecutor, scope memory.SemanticScope) error {
	kind, registryID, err := splitScopeKey(scope.Key)
	if err != nil {
		return err
	}
	if registryID != scope.RegistryID {
		return errors.New("semantic scope registry identity changed")
	}
	var exists int
	switch kind {
	case "global":
		return nil
	case "workspace":
		err = writer.queryRowContext(ctx, `SELECT COUNT(*) FROM workspaces WHERE id = ?`, registryID).Scan(&exists)
	case "project":
		err = writer.queryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE id = ?`, registryID).Scan(&exists)
	case "session":
		err = writer.queryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id = ?`, registryID).Scan(&exists)
	}
	if err != nil {
		return err
	}
	if exists != 1 {
		return fmt.Errorf("semantic %s scope backing identity is missing", kind)
	}
	return nil
}

func validateProposalSessionScope(
	ctx context.Context,
	writer turnLeaseWriteExecutor,
	proposal memory.RememberLiteralProposal,
) error {
	var workspaceID, projectID sql.NullString
	if err := writer.queryRowContext(ctx, `
		SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = ?
	`, proposal.SessionID, memory.SessionActive).Scan(&workspaceID, &projectID); err != nil {
		return fmt.Errorf("load active proposal session: %w", err)
	}
	expectedTarget := "global"
	if workspaceID.Valid {
		expectedTarget = "workspace:" + workspaceID.String
	} else if projectID.Valid {
		expectedTarget = "project:" + projectID.String
	}
	if proposal.Scope.Key != expectedTarget || proposal.Source.ScopeKey != expectedTarget ||
		proposal.Source.SessionID != proposal.SessionID {
		return errors.New("semantic proposal is outside its immutable session Context Scope")
	}
	expectedScopeCount := 1
	if expectedTarget != "global" {
		expectedScopeCount = 2
	}
	if len(proposal.Scopes) != expectedScopeCount {
		return errors.New("semantic proposal contains an unauthorized scope")
	}
	seenScopes := make(map[string]struct{}, len(proposal.Scopes))
	for index, scope := range proposal.Scopes {
		if scope.Key != "global" && scope.Key != expectedTarget {
			return errors.New("semantic proposal contains an unauthorized scope")
		}
		if index > 0 && proposal.Scopes[index-1].Key >= scope.Key {
			return errors.New("semantic proposal scopes are not in canonical order")
		}
		if _, duplicate := seenScopes[scope.Key]; duplicate {
			return errors.New("semantic proposal contains a duplicate scope")
		}
		seenScopes[scope.Key] = struct{}{}
	}
	if _, ok := seenScopes["global"]; !ok {
		return errors.New("semantic proposal omits the global scope")
	}
	if _, ok := seenScopes[expectedTarget]; !ok {
		return errors.New("semantic proposal omits its target scope")
	}
	return nil
}

func validateRememberLiteralProposal(proposal memory.RememberLiteralProposal) error {
	if proposal.SchemaVersion != 1 || proposal.Kind != "remember_literal_claim" || proposal.Actor != "owner" {
		return errors.New("unsupported semantic proposal")
	}
	if proposal.Polarity != "affirmed" || proposal.ValidTime.From != nil || proposal.ValidTime.To != nil {
		return errors.New("remember literal proposals must be timeless affirmed Claims")
	}
	if proposal.ExpectedRevision != proposal.Scope.Revision || proposal.Predicate.Version != 1 ||
		proposal.Predicate.ObjectConstraint != proposal.Literal.Kind || proposal.Predicate.Cardinality != "one" ||
		len(proposal.Predicate.Token) > 64 || !predicateTokenPattern.MatchString(proposal.Predicate.Token) ||
		strings.TrimSpace(proposal.Predicate.Label) == "" {
		return errors.New("remember literal proposal Predicate or revision is invalid")
	}
	if proposal.Subject.ScopeKey != "global" || proposal.Subject.CanonicalName != "owner" ||
		proposal.Subject.EntityType != "person" || proposal.Subject.AnchorKind != "owner" {
		return errors.New("remember literal proposal does not use the canonical owner anchor")
	}
	if proposal.Evie.ScopeKey != "global" || proposal.Evie.CanonicalName != "Evie" ||
		proposal.Evie.EntityType != "agent" || proposal.Evie.AnchorKind != "evie" {
		return errors.New("remember literal proposal does not use the canonical Evie anchor")
	}
	if proposal.Source.EventPart != "content" || proposal.Source.LocatorKind != "whole" ||
		proposal.Source.LocatorValue != "" || proposal.Source.Actor != "owner" ||
		proposal.Source.SourceType != "user_message" || proposal.Source.Authority != "owner_statement" {
		return errors.New("remember literal proposal source attributes are invalid")
	}
	return validateLiteral(proposal.Literal)
}

// ApplyRememberLiteral revalidates the complete proposal and commits its
// operation, projection, provenance, state event, and revisions atomically.
func (s *Store) ApplyRememberLiteral(
	ctx context.Context,
	lease memory.TurnLease,
	proposal memory.RememberLiteralProposal,
) (result memory.RememberLiteralResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("semantic proposal does not match its turn lease")
	}
	canonical := canonicalRememberLiteralProposal(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	_, preparedProposalJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if err := validateRememberLiteralProposal(proposal); err != nil {
		return result, err
	}
	for _, id := range []string{string(proposal.OperationID), string(proposal.Predicate.ID), string(proposal.Subject.ID),
		string(proposal.Evie.ID), string(proposal.ClaimID), string(proposal.SourceLinkID), string(proposal.Scope.ID)} {
		if err := validateSemanticUUID(id); err != nil {
			return result, err
		}
	}
	for _, scope := range proposal.Scopes {
		if err := validateSemanticUUID(string(scope.ID)); err != nil {
			return result, err
		}
	}

	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var acceptedHash, acceptedResult string
		existingErr := writer.queryRowContext(ctx, `
			SELECT proposal_sha256, result_json FROM semantic_operations WHERE idempotency_key = ?
		`, proposal.IdempotencyKey).Scan(&acceptedHash, &acceptedResult)
		if existingErr == nil {
			if acceptedHash != proposalHash {
				return ErrIdempotencyConflict
			}
			if err := json.Unmarshal([]byte(acceptedResult), &result); err != nil {
				return fmt.Errorf("decode original semantic result: %w", err)
			}
			return nil
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		if proposal.ProposalSHA256 != "" && proposal.ProposalSHA256 != proposalHash {
			return errors.New("semantic proposal hash changed")
		}
		if err := validateProposalSessionScope(ctx, writer, proposal); err != nil {
			return err
		}

		var eventSession, eventType, eventRole, eventContent, eventRecorded string
		if err := writer.queryRowContext(ctx, `
			SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ?
		`, proposal.Source.EventID).Scan(&eventSession, &eventType, &eventRole, &eventContent, &eventRecorded); err != nil {
			return fmt.Errorf("revalidate source event: %w", err)
		}
		digest := sha256.Sum256([]byte(eventContent))
		if eventSession != string(proposal.SessionID) || eventType != string(memory.EventUserMessage) ||
			eventRole != string(memory.RoleUser) || proposal.Source.Evidence != eventContent ||
			proposal.Source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", digest) {
			return errors.New("semantic source evidence changed")
		}
		observed, err := time.Parse(time.RFC3339Nano, eventRecorded)
		if err != nil || proposal.Source.ObservedAt != formatSemanticTime(observed) {
			return errors.New("semantic source observation time changed")
		}

		byKey := make(map[string]memory.SemanticScope, len(proposal.Scopes))
		for _, scope := range proposal.Scopes {
			byKey[scope.Key] = scope
			if err := validateScopeBacking(ctx, writer, scope); err != nil {
				return err
			}
			var storedID string
			var storedRevision int64
			err := writer.queryRowContext(ctx, `SELECT scope_id, revision FROM semantic_scopes WHERE scope_key = ?`, scope.Key).Scan(&storedID, &storedRevision)
			switch {
			case errors.Is(err, sql.ErrNoRows) && scope.Revision == 0:
				kind, registryID, splitErr := splitScopeKey(scope.Key)
				if splitErr != nil {
					return splitErr
				}
				if _, err := writer.execContext(ctx, `
					INSERT INTO semantic_scopes (scope_id, scope_key, scope_kind, registry_id, revision)
					VALUES (?, ?, ?, NULLIF(?, ''), 0)
				`, scope.ID, scope.Key, kind, registryID); err != nil {
					return fmt.Errorf("create semantic scope: %w", err)
				}
			case err != nil:
				return err
			case storedID != string(scope.ID) || storedRevision != scope.Revision:
				return ErrStaleScopeRevision
			}
		}
		targetScope, ok := byKey[proposal.Scope.Key]
		if !ok || targetScope != proposal.Scope {
			return errors.New("semantic proposal target scope does not match its revision vector")
		}
		seenPriors := make(map[string]struct{}, len(proposal.PriorRevisions))
		for index, prior := range proposal.PriorRevisions {
			if index > 0 && proposal.PriorRevisions[index-1].ScopeKey >= prior.ScopeKey {
				return errors.New("semantic proposal revision vector is not in canonical order")
			}
			if _, duplicate := seenPriors[prior.ScopeKey]; duplicate {
				return errors.New("semantic proposal revision vector contains a duplicate scope")
			}
			seenPriors[prior.ScopeKey] = struct{}{}
			scope, ok := byKey[prior.ScopeKey]
			if !ok || scope.Revision != prior.Revision {
				return errors.New("semantic proposal revision vector does not match its scopes")
			}
		}
		if len(seenPriors) != len(byKey) {
			return errors.New("semantic proposal revision vector is incomplete")
		}

		now := s.now().UTC()
		var latest sql.NullString
		if err := writer.queryRowContext(ctx, `SELECT MAX(transaction_time) FROM semantic_operations`).Scan(&latest); err != nil {
			return err
		}
		if latest.Valid {
			priorTime, err := parseSemanticTime(latest.String)
			if err != nil {
				return fmt.Errorf("parse latest semantic transaction time: %w", err)
			}
			if priorTime.After(now) {
				now = priorTime
			}
		}
		transactionText := formatSemanticTime(now)
		writeGlobal := proposalWritesGlobal(proposal)
		result.ResultingRevisions = make([]memory.ScopeRevision, 0, len(proposal.Scopes))
		for _, scope := range proposal.Scopes {
			written := scope.Key == proposal.Scope.Key || (scope.Key == "global" && writeGlobal)
			resulting := scope.Revision
			if written {
				resulting++
			}
			result.ResultingRevisions = append(result.ResultingRevisions, memory.ScopeRevision{ScopeKey: scope.Key, Revision: resulting})
			if scope.Key == proposal.Scope.Key {
				result.ScopeRevision = resulting
			}
		}
		result.OperationID, result.ClaimID, result.SourceLinkID = proposal.OperationID, proposal.ClaimID, proposal.SourceLinkID
		result.TransactionTime = now
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return err
		}
		effectHash, _, err := semanticHash(canonical.Effect)
		if err != nil {
			return err
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_operations (
				operation_id, schema_version, operation_kind, idempotency_key, actor, session_id,
				target_scope_id, source_event_id, proposal_sha256, effect_sha256,
				proposal_json, prepared_proposal_json, result_json, transaction_time
			) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, proposal.OperationID, proposal.Kind, proposal.IdempotencyKey, proposal.Actor, proposal.SessionID,
			proposal.Scope.ID, proposal.Source.EventID, proposalHash, effectHash, string(proposalJSON),
			string(preparedProposalJSON), string(resultJSON), transactionText); err != nil {
			return fmt.Errorf("record semantic operation: %w", err)
		}
		for _, revision := range result.ResultingRevisions {
			scope := byKey[revision.ScopeKey]
			written := revision.Revision != scope.Revision
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_operation_scopes (operation_id, scope_id, prior_revision, resulting_revision, written)
				VALUES (?, ?, ?, ?, ?)
			`, proposal.OperationID, scope.ID, scope.Revision, revision.Revision, written); err != nil {
				return err
			}
		}
		if proposal.Predicate.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_predicates (predicate_id, token, version, label, object_constraint, cardinality, created_operation_id)
				VALUES (?, ?, ?, ?, ?, ?, ?)
			`, proposal.Predicate.ID, proposal.Predicate.Token, proposal.Predicate.Version, proposal.Predicate.Label,
				proposal.Predicate.ObjectConstraint, proposal.Predicate.Cardinality, proposal.OperationID); err != nil {
				return fmt.Errorf("create semantic Predicate: %w", err)
			}
		} else {
			var id string
			if err := writer.queryRowContext(ctx, `
				SELECT predicate_id FROM semantic_predicates
				WHERE token = ? AND version = ? AND label = ? AND object_constraint = ? AND cardinality = ?
			`, proposal.Predicate.Token, proposal.Predicate.Version, proposal.Predicate.Label,
				proposal.Predicate.ObjectConstraint, proposal.Predicate.Cardinality).Scan(&id); err != nil || id != string(proposal.Predicate.ID) {
				return errors.New("semantic Predicate changed after preparation")
			}
		}
		globalScope := byKey["global"]
		if proposal.Subject.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_entities (entity_id, scope_id, canonical_name, entity_type, anchor_kind, lifecycle, created_operation_id)
				VALUES (?, ?, ?, ?, ?, 'active', ?)
			`, proposal.Subject.ID, globalScope.ID, proposal.Subject.CanonicalName, proposal.Subject.EntityType,
				proposal.Subject.AnchorKind, proposal.OperationID); err != nil {
				return fmt.Errorf("create semantic owner anchor: %w", err)
			}
		} else {
			var id string
			if err := writer.queryRowContext(ctx, `
				SELECT entities.entity_id
				FROM semantic_entities AS entities
				JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
				WHERE entities.anchor_kind = 'owner' AND scopes.scope_key = 'global'
			`).Scan(&id); err != nil || id != string(proposal.Subject.ID) {
				return errors.New("semantic owner anchor changed after preparation")
			}
		}
		if proposal.Evie.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_entities (entity_id, scope_id, canonical_name, entity_type, anchor_kind, lifecycle, created_operation_id)
				VALUES (?, ?, ?, ?, ?, 'active', ?)
			`, proposal.Evie.ID, globalScope.ID, proposal.Evie.CanonicalName, proposal.Evie.EntityType,
				proposal.Evie.AnchorKind, proposal.OperationID); err != nil {
				return fmt.Errorf("create semantic Evie anchor: %w", err)
			}
		} else {
			var id string
			if err := writer.queryRowContext(ctx, `
				SELECT entities.entity_id
				FROM semantic_entities AS entities
				JOIN semantic_scopes AS scopes ON scopes.scope_id = entities.scope_id
				WHERE entities.anchor_kind = 'evie' AND scopes.scope_key = 'global'
			`).Scan(&id); err != nil || id != string(proposal.Evie.ID) {
				return errors.New("semantic Evie anchor changed after preparation")
			}
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_claims (
				claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
				literal_kind, literal_value, polarity, valid_from, valid_to, lifecycle,
				created_operation_id, transaction_time
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, 'active', ?, ?)
		`, proposal.ClaimID, proposal.Scope.ID, proposal.Subject.ID, proposal.Predicate.ID,
			proposal.Predicate.Token, proposal.Predicate.Version, proposal.Literal.Kind, proposal.Literal.Value,
			proposal.Polarity, proposal.OperationID, transactionText); err != nil {
			return fmt.Errorf("create semantic Claim: %w", err)
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_source_links (
				source_link_id, claim_id, event_id, source_session_id, source_scope_key,
				event_part, locator_kind, locator_value, evidence_sha256, source_actor,
				source_type, authority, observed_at, eligibility, created_operation_id
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'eligible', ?)
		`, proposal.SourceLinkID, proposal.ClaimID, proposal.Source.EventID, proposal.SessionID, proposal.Source.ScopeKey,
			proposal.Source.EventPart, proposal.Source.LocatorKind, proposal.Source.LocatorValue,
			proposal.Source.EvidenceSHA256, proposal.Source.Actor, proposal.Source.SourceType,
			proposal.Source.Authority, proposal.Source.ObservedAt, proposal.OperationID); err != nil {
			return fmt.Errorf("create semantic Source Link: %w", err)
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_state_events (
				scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time
			) VALUES (?, 'claim', ?, 'active', ?, ?, ?)
		`, proposal.Scope.ID, proposal.ClaimID, proposal.OperationID, result.ScopeRevision, transactionText); err != nil {
			return fmt.Errorf("create semantic state event: %w", err)
		}
		for _, revision := range result.ResultingRevisions {
			if revision.Revision == byKey[revision.ScopeKey].Revision {
				continue
			}
			if _, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`,
				revision.Revision, byKey[revision.ScopeKey].ID, byKey[revision.ScopeKey].Revision); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}
