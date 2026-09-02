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
	"unicode/utf8"

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

CREATE TABLE IF NOT EXISTS semantic_aliases (
    alias_id              TEXT PRIMARY KEY NOT NULL,
    entity_id             TEXT NOT NULL REFERENCES semantic_entities(entity_id),
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    value                 TEXT NOT NULL,
    normalized_value      TEXT NOT NULL,
    lifecycle             TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired')),
    source_event_id       TEXT NOT NULL REFERENCES events(id),
    created_operation_id  TEXT NOT NULL REFERENCES semantic_operations(operation_id)
);

CREATE TABLE IF NOT EXISTS semantic_claims (
    claim_id              TEXT PRIMARY KEY NOT NULL,
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    subject_entity_id     TEXT NOT NULL REFERENCES semantic_entities(entity_id),
    predicate_id          TEXT NOT NULL REFERENCES semantic_predicates(predicate_id),
    predicate_token       TEXT NOT NULL,
    predicate_version     INTEGER NOT NULL CHECK (predicate_version > 0),
	object_kind           TEXT NOT NULL CHECK (object_kind IN ('entity', 'literal')),
	object_entity_id      TEXT REFERENCES semantic_entities(entity_id),
	literal_kind          TEXT CHECK (literal_kind IN ('text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
	literal_value         TEXT,
    polarity              TEXT NOT NULL CHECK (polarity IN ('affirmed', 'denied')),
    valid_from            TEXT,
    valid_to              TEXT,
    lifecycle             TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired', 'superseded')),
    created_operation_id  TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    transaction_time      TEXT NOT NULL,
	CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to),
	CHECK ((object_kind = 'entity' AND object_entity_id IS NOT NULL AND literal_kind IS NULL AND literal_value IS NULL)
	    OR (object_kind = 'literal' AND object_entity_id IS NULL AND literal_kind IS NOT NULL AND literal_value IS NOT NULL))
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
CREATE INDEX IF NOT EXISTS semantic_aliases_exact_idx ON semantic_aliases(scope_id, normalized_value, lifecycle, entity_id, alias_id);
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
CREATE TRIGGER IF NOT EXISTS semantic_aliases_append_only_update BEFORE UPDATE ON semantic_aliases BEGIN SELECT RAISE(ABORT, 'semantic aliases are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_aliases_append_only_delete BEFORE DELETE ON semantic_aliases BEGIN SELECT RAISE(ABORT, 'semantic aliases are append-only'); END;
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
	if _, err := db.ExecContext(ctx, semanticSchema); err != nil {
		return err
	}
	present, err := tableHasColumn(ctx, db, "semantic_claims", "object_kind")
	if err != nil || present {
		return err
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		var count int
		if err := conn.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM pragma_table_info('semantic_claims') WHERE name = 'object_kind'
		`).Scan(&count); err != nil {
			return err
		}
		if count == 1 {
			return nil
		}
		if _, err := conn.ExecContext(ctx, semanticClaimsObjectMigration); err != nil {
			return fmt.Errorf("migrate Semantic Memory Claim objects: %w", err)
		}
		return nil
	})
}

const semanticClaimsObjectMigration = `
DROP TRIGGER IF EXISTS semantic_source_links_append_only_update;
DROP TRIGGER IF EXISTS semantic_source_links_append_only_delete;
DROP TRIGGER IF EXISTS semantic_claims_append_only_update;
DROP TRIGGER IF EXISTS semantic_claims_append_only_delete;

CREATE TABLE semantic_claims_v1 (
    claim_id              TEXT PRIMARY KEY NOT NULL,
    scope_id              TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    subject_entity_id     TEXT NOT NULL REFERENCES semantic_entities(entity_id),
    predicate_id          TEXT NOT NULL REFERENCES semantic_predicates(predicate_id),
    predicate_token       TEXT NOT NULL,
    predicate_version     INTEGER NOT NULL CHECK (predicate_version > 0),
    object_kind           TEXT NOT NULL CHECK (object_kind IN ('entity', 'literal')),
    object_entity_id      TEXT REFERENCES semantic_entities(entity_id),
    literal_kind          TEXT CHECK (literal_kind IN ('text', 'integer', 'decimal', 'boolean', 'date', 'datetime')),
    literal_value         TEXT,
    polarity              TEXT NOT NULL CHECK (polarity IN ('affirmed', 'denied')),
    valid_from            TEXT,
    valid_to              TEXT,
    lifecycle             TEXT NOT NULL CHECK (lifecycle IN ('active', 'retired', 'superseded')),
    created_operation_id  TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    transaction_time      TEXT NOT NULL,
    CHECK (valid_from IS NULL OR valid_to IS NULL OR valid_from < valid_to),
    CHECK ((object_kind = 'entity' AND object_entity_id IS NOT NULL AND literal_kind IS NULL AND literal_value IS NULL)
        OR (object_kind = 'literal' AND object_entity_id IS NULL AND literal_kind IS NOT NULL AND literal_value IS NOT NULL))
);

INSERT INTO semantic_claims_v1 (
    claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
    object_kind, object_entity_id, literal_kind, literal_value, polarity, valid_from, valid_to,
    lifecycle, created_operation_id, transaction_time
)
SELECT claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
       'literal', NULL, literal_kind, literal_value, polarity, valid_from, valid_to,
       lifecycle, created_operation_id, transaction_time
FROM semantic_claims;

CREATE TABLE semantic_source_links_v1 (
    source_link_id       TEXT PRIMARY KEY NOT NULL,
    claim_id             TEXT NOT NULL REFERENCES semantic_claims_v1(claim_id),
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

INSERT INTO semantic_source_links_v1 SELECT * FROM semantic_source_links;
DROP TABLE semantic_source_links;
DROP TABLE semantic_claims;
ALTER TABLE semantic_claims_v1 RENAME TO semantic_claims;
ALTER TABLE semantic_source_links_v1 RENAME TO semantic_source_links;

CREATE INDEX semantic_claims_scope_idx ON semantic_claims(scope_id, lifecycle, claim_id);
CREATE INDEX semantic_source_links_claim_idx ON semantic_source_links(claim_id, eligibility, source_link_id);
CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END;
CREATE TRIGGER semantic_claims_append_only_delete BEFORE DELETE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END;
CREATE TRIGGER semantic_source_links_append_only_update BEFORE UPDATE ON semantic_source_links BEGIN SELECT RAISE(ABORT, 'semantic source links are append-only'); END;
CREATE TRIGGER semantic_source_links_append_only_delete BEFORE DELETE ON semantic_source_links BEGIN SELECT RAISE(ABORT, 'semantic source links are append-only'); END;
`

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
		if utf8.ValidString(literal.Value) {
			return nil
		}
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

func normalizeValidTime(value memory.ValidTime) (memory.ValidTime, error) {
	normalizeBound := func(source *time.Time) (*time.Time, error) {
		if source == nil {
			return nil, nil
		}
		utc := source.UTC()
		encoded := formatSemanticTime(utc)
		parsed, err := parseSemanticTime(encoded)
		if err != nil || !parsed.Equal(utc) {
			return nil, errors.New("Valid Time bound is outside the canonical UTC datetime encoding")
		}
		return &utc, nil
	}
	from, err := normalizeBound(value.From)
	if err != nil {
		return memory.ValidTime{}, err
	}
	to, err := normalizeBound(value.To)
	if err != nil {
		return memory.ValidTime{}, err
	}
	normalized := memory.ValidTime{From: from, To: to}
	if normalized.From != nil && normalized.To != nil && !normalized.From.Before(*normalized.To) {
		return memory.ValidTime{}, errors.New("Valid Time must be a non-empty half-open interval")
	}
	return normalized, nil
}

func semanticTimeArgument(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatSemanticTime(*value)
}

func validTimesEqual(left, right memory.ValidTime) bool {
	return nullableTimesEqual(left.From, right.From) && nullableTimesEqual(left.To, right.To)
}

func nullableTimesEqual(left, right *time.Time) bool {
	return left == nil && right == nil || left != nil && right != nil && left.Equal(*right)
}

func normalizeClaimSemantics(
	cardinality, defaultCardinality memory.PredicateCardinality,
	polarity memory.ClaimPolarity,
	validTime memory.ValidTime,
) (memory.PredicateCardinality, memory.ClaimPolarity, memory.ValidTime, error) {
	if cardinality == "" {
		cardinality = defaultCardinality
	}
	if cardinality != memory.CardinalityOne && cardinality != memory.CardinalityMany {
		return "", "", memory.ValidTime{}, fmt.Errorf("unsupported Predicate cardinality %q", cardinality)
	}
	if polarity == "" {
		polarity = memory.PolarityAffirmed
	}
	if polarity != memory.PolarityAffirmed && polarity != memory.PolarityDenied {
		return "", "", memory.ValidTime{}, fmt.Errorf("unsupported Claim polarity %q", polarity)
	}
	normalizedTime, err := normalizeValidTime(validTime)
	if err != nil {
		return "", "", memory.ValidTime{}, err
	}
	return cardinality, polarity, normalizedTime, nil
}

func normalizeLiteralRequest(request memory.RememberLiteralRequest) (memory.RememberLiteralRequest, error) {
	cardinality, polarity, validTime, err := normalizeClaimSemantics(
		request.PredicateCardinality, memory.CardinalityOne, request.Polarity, request.ValidTime,
	)
	if err != nil {
		return memory.RememberLiteralRequest{}, err
	}
	request.PredicateCardinality, request.Polarity, request.ValidTime = cardinality, polarity, validTime
	return request, nil
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
	var err error
	request, err = normalizeLiteralRequest(request)
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}
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
	err = s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`,
		request.IdempotencyKey).Scan(&priorProposalJSON, &priorHash)
	if err == nil {
		var proposal memory.RememberLiteralProposal
		if err := json.Unmarshal([]byte(priorProposalJSON), &proposal); err != nil {
			return memory.RememberLiteralProposal{}, fmt.Errorf("decode accepted proposal: %w", err)
		}
		var acceptedShape struct {
			ClaimCreate *bool `json:"claim_create"`
			Source      struct {
				Create *bool `json:"create"`
			} `json:"source"`
		}
		if err := json.Unmarshal([]byte(priorProposalJSON), &acceptedShape); err != nil {
			return memory.RememberLiteralProposal{}, fmt.Errorf("decode accepted proposal shape: %w", err)
		}
		// Stage 2 always created both rows and predates the explicit preview flags.
		// Recover that accepted meaning when reopening its stored proposal JSON.
		if acceptedShape.ClaimCreate == nil {
			proposal.ClaimCreate = true
		}
		if acceptedShape.Source.Create == nil {
			proposal.Source.Create = true
		}
		if proposal.Source.OperationID == "" {
			proposal.Source.OperationID = proposal.OperationID
		}
		if proposal.Source.Eligibility == "" {
			proposal.Source.Eligibility = memory.EligibilityEligible
		}
		proposal.Request = request
		if proposal.SessionID != scope.SessionID || proposal.Source.EventID != request.SourceEventID ||
			proposal.Predicate.Token != request.Predicate || proposal.Predicate.Label != request.PredicateLabel ||
			proposal.Predicate.Cardinality != request.PredicateCardinality || proposal.Literal != request.Literal ||
			proposal.Polarity != request.Polarity || !validTimesEqual(proposal.ValidTime, request.ValidTime) {
			return memory.RememberLiteralProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		if err != nil {
			return memory.RememberLiteralProposal{}, fmt.Errorf("hash accepted prepared proposal: %w", err)
		}
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

	predicate := memory.SemanticPredicate{Token: request.Predicate, Label: request.PredicateLabel,
		ObjectConstraint: memory.PredicateObjectConstraint(request.Literal.Kind), Cardinality: request.PredicateCardinality}
	err = s.db.QueryRowContext(ctx, `
		SELECT predicate_id, version, label, object_constraint, cardinality
		FROM semantic_predicates WHERE token = ? ORDER BY version DESC LIMIT 1
	`, predicate.Token).Scan(&predicate.ID, &predicate.Version, &predicate.Label, &predicate.ObjectConstraint, &predicate.Cardinality)
	if errors.Is(err, sql.ErrNoRows) {
		predicate.ID, err = newSemanticID()
		predicate.Version = 1
		predicate.Create = true
	} else if err == nil && (predicate.Label != request.PredicateLabel ||
		predicate.ObjectConstraint != memory.PredicateObjectConstraint(request.Literal.Kind) ||
		predicate.Cardinality != request.PredicateCardinality) {
		predicate.ID, err = newSemanticID()
		predicate.Version++
		predicate.Label = request.PredicateLabel
		predicate.ObjectConstraint = memory.PredicateObjectConstraint(request.Literal.Kind)
		predicate.Cardinality = request.PredicateCardinality
		predicate.Create = true
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
	var claimID memory.SemanticID
	claimCreate := false
	validFrom, validTo := semanticTimeArgument(request.ValidTime.From), semanticTimeArgument(request.ValidTime.To)
	err = s.db.QueryRowContext(ctx, `
		SELECT claim_id FROM semantic_claims
		WHERE scope_id = ? AND subject_entity_id = ? AND predicate_id = ? AND object_kind = 'literal'
		  AND literal_kind = ? AND literal_value = ? AND polarity = ?
		  AND valid_from IS ? AND valid_to IS ?
	`, target.ID, subject.ID, predicate.ID, request.Literal.Kind, request.Literal.Value, request.Polarity,
		validFrom, validTo).Scan(&claimID)
	if errors.Is(err, sql.ErrNoRows) {
		claimID, err = newSemanticID()
		claimCreate = true
	}
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}
	digest := sha256.Sum256([]byte(content))
	evidenceHash := fmt.Sprintf("sha256:%x", digest)
	var sourceLinkID memory.SemanticID
	var sourceOperationID memory.SemanticID
	sourceCreate := false
	err = s.db.QueryRowContext(ctx, `
		SELECT source_link_id, created_operation_id FROM semantic_source_links
		WHERE claim_id = ? AND event_id = ? AND event_part = 'content' AND locator_kind = 'whole'
		  AND locator_value = '' AND evidence_sha256 = ?
	`, claimID, request.SourceEventID, evidenceHash).Scan(&sourceLinkID, &sourceOperationID)
	if errors.Is(err, sql.ErrNoRows) {
		sourceLinkID, err = newSemanticID()
		sourceOperationID = operationID
		sourceCreate = true
	}
	if err != nil {
		return memory.RememberLiteralProposal{}, err
	}

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
		Predicate: predicate, Subject: subject, Evie: evie, ClaimID: claimID, ClaimCreate: claimCreate, SourceLinkID: sourceLinkID,
		Literal: request.Literal, Polarity: request.Polarity, ValidTime: request.ValidTime,
		Source: memory.SemanticSource{
			OperationID: sourceOperationID, EventID: request.SourceEventID, SessionID: scope.SessionID, ScopeKey: targetKey,
			EventPart: "content", LocatorKind: "whole", LocatorValue: "",
			EvidenceSHA256: evidenceHash, Actor: "owner", SourceType: "user_message",
			Authority: "owner_statement", ObservedAt: formatSemanticTime(observed), Evidence: content,
			Eligibility: memory.EligibilityEligible, Create: sourceCreate,
		}, Request: request,
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalRememberLiteralProposal(proposal))
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("hash proposal: %w", err)
	}
	proposal.PreparedSHA256, _, err = semanticHash(proposal)
	if err != nil {
		return memory.RememberLiteralProposal{}, fmt.Errorf("hash prepared proposal: %w", err)
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
	expectedTarget, expectedScopes, err := authorizedSemanticScopes(ctx, writer, proposal.SessionID, false)
	if err != nil {
		return err
	}
	return validateAuthorizedSemanticScopes(expectedTarget, expectedScopes, proposal.SessionID, proposal.Source.SessionID,
		proposal.Source.ScopeKey, proposal.Scopes)
}

func authorizedSemanticScopes(
	ctx context.Context,
	writer turnLeaseWriteExecutor,
	sessionID memory.SessionID,
	useSessionScope bool,
) (string, []string, error) {
	var workspaceID, projectID sql.NullString
	if err := writer.queryRowContext(ctx, `
		SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = ?
	`, sessionID, memory.SessionActive).Scan(&workspaceID, &projectID); err != nil {
		return "", nil, fmt.Errorf("load active proposal session: %w", err)
	}
	contextScope := "global"
	if workspaceID.Valid {
		contextScope = "workspace:" + workspaceID.String
	} else if projectID.Valid {
		contextScope = "project:" + projectID.String
	}
	expectedTarget := contextScope
	if useSessionScope {
		expectedTarget = "session:" + string(sessionID)
	}
	expectedScopes := []string{"global"}
	if contextScope != "global" {
		expectedScopes = append(expectedScopes, contextScope)
	}
	if expectedTarget != contextScope {
		expectedScopes = append(expectedScopes, expectedTarget)
	}
	sort.Strings(expectedScopes)
	return expectedTarget, expectedScopes, nil
}

func validateAuthorizedSemanticScopes(
	expectedTarget string,
	expectedScopes []string,
	proposalSessionID, sourceSessionID memory.SessionID,
	sourceScopeKey string,
	scopes []memory.SemanticScope,
) error {
	if sourceScopeKey != expectedTarget || sourceSessionID != proposalSessionID {
		return errors.New("semantic proposal is outside its immutable session Context Scope")
	}
	if len(scopes) != len(expectedScopes) {
		return errors.New("semantic proposal contains an unauthorized scope")
	}
	for index, scope := range scopes {
		if scope.Key != expectedScopes[index] {
			return errors.New("semantic proposal contains an unauthorized scope")
		}
	}
	return nil
}

func validateSemanticScopeVector(
	ctx context.Context,
	writer turnLeaseWriteExecutor,
	scopes []memory.SemanticScope,
	priors []memory.ScopeRevision,
) (map[string]memory.SemanticScope, error) {
	byKey := make(map[string]memory.SemanticScope, len(scopes))
	for index, scope := range scopes {
		if index > 0 && scopes[index-1].Key >= scope.Key {
			return nil, errors.New("semantic proposal scopes are not in canonical order")
		}
		if _, duplicate := byKey[scope.Key]; duplicate {
			return nil, errors.New("semantic proposal contains a duplicate scope")
		}
		byKey[scope.Key] = scope
		if err := validateScopeBacking(ctx, writer, scope); err != nil {
			return nil, err
		}
		var storedID string
		var storedRevision int64
		err := writer.queryRowContext(ctx, `SELECT scope_id, revision FROM semantic_scopes WHERE scope_key = ?`, scope.Key).Scan(&storedID, &storedRevision)
		switch {
		case errors.Is(err, sql.ErrNoRows) && scope.Revision == 0:
			kind, registryID, splitErr := splitScopeKey(scope.Key)
			if splitErr != nil {
				return nil, splitErr
			}
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_scopes (scope_id, scope_key, scope_kind, registry_id, revision)
				VALUES (?, ?, ?, NULLIF(?, ''), 0)
			`, scope.ID, scope.Key, kind, registryID); err != nil {
				return nil, fmt.Errorf("create semantic scope: %w", err)
			}
		case err != nil:
			return nil, err
		case storedID != string(scope.ID) || storedRevision != scope.Revision:
			return nil, ErrStaleScopeRevision
		}
	}
	if len(priors) != len(byKey) {
		return nil, errors.New("semantic proposal revision vector is incomplete")
	}
	seenPriors := make(map[string]struct{}, len(priors))
	for index, prior := range priors {
		if index > 0 && priors[index-1].ScopeKey >= prior.ScopeKey {
			return nil, errors.New("semantic proposal revision vector is not in canonical order")
		}
		if _, duplicate := seenPriors[prior.ScopeKey]; duplicate {
			return nil, errors.New("semantic proposal revision vector contains a duplicate scope")
		}
		seenPriors[prior.ScopeKey] = struct{}{}
		scope, ok := byKey[prior.ScopeKey]
		if !ok || scope.Revision != prior.Revision {
			return nil, errors.New("semantic proposal revision vector does not match its scopes")
		}
	}
	return byKey, nil
}

func nextSemanticTransactionTime(ctx context.Context, writer turnLeaseWriteExecutor, clock time.Time) (time.Time, error) {
	now := clock.UTC()
	var latest sql.NullString
	if err := writer.queryRowContext(ctx, `SELECT MAX(transaction_time) FROM semantic_operations`).Scan(&latest); err != nil {
		return time.Time{}, err
	}
	if latest.Valid {
		priorTime, err := parseSemanticTime(latest.String)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse latest semantic transaction time: %w", err)
		}
		if priorTime.After(now) {
			now = priorTime
		}
	}
	return now, nil
}

type acceptedSemanticOperation struct {
	OperationID     memory.SemanticID
	Kind            string
	IdempotencyKey  string
	Actor           memory.SemanticActor
	SessionID       memory.SessionID
	TargetScopeID   memory.SemanticID
	SourceEventID   memory.EventID
	ProposalHash    string
	EffectHash      string
	ProposalJSON    []byte
	PreparedJSON    []byte
	ResultJSON      []byte
	TransactionTime time.Time
	ResultRevisions []memory.ScopeRevision
	ScopesByKey     map[string]memory.SemanticScope
}

func recordAcceptedSemanticOperation(ctx context.Context, writer turnLeaseWriteExecutor, operation acceptedSemanticOperation) error {
	if _, err := writer.execContext(ctx, `
		INSERT INTO semantic_operations (
			operation_id, schema_version, operation_kind, idempotency_key, actor, session_id,
			target_scope_id, source_event_id, proposal_sha256, effect_sha256,
			proposal_json, prepared_proposal_json, result_json, transaction_time
		) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, operation.OperationID, operation.Kind, operation.IdempotencyKey, operation.Actor, operation.SessionID,
		operation.TargetScopeID, operation.SourceEventID, operation.ProposalHash, operation.EffectHash,
		string(operation.ProposalJSON), string(operation.PreparedJSON), string(operation.ResultJSON),
		formatSemanticTime(operation.TransactionTime)); err != nil {
		return fmt.Errorf("record semantic operation: %w", err)
	}
	for _, revision := range operation.ResultRevisions {
		scope, ok := operation.ScopesByKey[revision.ScopeKey]
		if !ok {
			return errors.New("semantic result contains a scope outside its proposal")
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO semantic_operation_scopes (operation_id, scope_id, prior_revision, resulting_revision, written)
			VALUES (?, ?, ?, ?, ?)
		`, operation.OperationID, scope.ID, scope.Revision, revision.Revision, revision.Revision != scope.Revision); err != nil {
			return fmt.Errorf("record semantic operation scope: %w", err)
		}
	}
	return nil
}

func validateRememberLiteralProposal(proposal memory.RememberLiteralProposal) error {
	if proposal.SchemaVersion != 1 || proposal.Kind != "remember_literal_claim" || proposal.Actor != "owner" {
		return errors.New("unsupported semantic proposal")
	}
	if proposal.Polarity != memory.PolarityAffirmed && proposal.Polarity != memory.PolarityDenied {
		return errors.New("remember literal proposal Claim polarity is invalid")
	}
	normalizedValidTime, err := normalizeValidTime(proposal.ValidTime)
	if err != nil || !validTimesEqual(normalizedValidTime, proposal.ValidTime) {
		return errors.New("remember literal proposal Valid Time is invalid")
	}
	if proposal.ExpectedRevision != proposal.Scope.Revision || proposal.Predicate.Version < 1 ||
		proposal.Predicate.ObjectConstraint != memory.PredicateObjectConstraint(proposal.Literal.Kind) ||
		(proposal.Predicate.Cardinality != memory.CardinalityOne && proposal.Predicate.Cardinality != memory.CardinalityMany) ||
		len(proposal.Predicate.Token) > 64 || !predicateTokenPattern.MatchString(proposal.Predicate.Token) ||
		strings.TrimSpace(proposal.Predicate.Label) == "" || !utf8.ValidString(proposal.Predicate.Label) {
		return errors.New("remember literal proposal Predicate or revision is invalid")
	}
	if proposal.Request.IdempotencyKey != "" && !literalRequestMatchesProposal(proposal.Request, proposal) {
		return fmt.Errorf("%w: remember literal proposal differs from its prepared request", ErrIdempotencyConflict)
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
		proposal.Source.SourceType != "user_message" || proposal.Source.Authority != "owner_statement" ||
		proposal.Source.Eligibility != memory.EligibilityEligible {
		return errors.New("remember literal proposal source attributes are invalid")
	}
	if proposal.Source.Create {
		if proposal.Source.OperationID != proposal.OperationID {
			return errors.New("new literal Source Link does not name its creating operation")
		}
	} else if proposal.Source.OperationID == "" {
		return errors.New("reused literal Source Link omits its creating operation")
	}
	return validateLiteral(proposal.Literal)
}

func literalRequestMatchesProposal(request memory.RememberLiteralRequest, proposal memory.RememberLiteralProposal) bool {
	return request.IdempotencyKey == proposal.IdempotencyKey && request.SourceEventID == proposal.Source.EventID &&
		request.Predicate == proposal.Predicate.Token && request.PredicateLabel == proposal.Predicate.Label &&
		request.PredicateCardinality == proposal.Predicate.Cardinality && request.Literal == proposal.Literal &&
		request.Polarity == proposal.Polarity && validTimesEqual(request.ValidTime, proposal.ValidTime)
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
	preparedHash, preparedProposalJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if err := validateRememberLiteralProposal(proposal); err != nil {
		return result, err
	}
	if proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != proposalHash ||
		proposal.PreparedSHA256 == "" || proposal.PreparedSHA256 != preparedHash {
		return result, errors.New("semantic proposal hash changed")
	}
	for _, id := range []string{string(proposal.OperationID), string(proposal.Predicate.ID), string(proposal.Subject.ID),
		string(proposal.Evie.ID), string(proposal.ClaimID), string(proposal.SourceLinkID), string(proposal.Source.OperationID), string(proposal.Scope.ID)} {
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

		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions)
		if err != nil {
			return err
		}
		targetScope, ok := byKey[proposal.Scope.Key]
		if !ok || targetScope != proposal.Scope {
			return errors.New("semantic proposal target scope does not match its revision vector")
		}

		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
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
		if err := recordAcceptedSemanticOperation(ctx, writer, acceptedSemanticOperation{
			OperationID: proposal.OperationID, Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey,
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
		if proposal.ClaimCreate {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_claims (
					claim_id, scope_id, subject_entity_id, predicate_id, predicate_token, predicate_version,
					object_kind, literal_kind, literal_value, polarity, valid_from, valid_to, lifecycle,
					created_operation_id, transaction_time
				) VALUES (?, ?, ?, ?, ?, ?, 'literal', ?, ?, ?, ?, ?, 'active', ?, ?)
			`, proposal.ClaimID, proposal.Scope.ID, proposal.Subject.ID, proposal.Predicate.ID,
				proposal.Predicate.Token, proposal.Predicate.Version, proposal.Literal.Kind, proposal.Literal.Value,
				proposal.Polarity, semanticTimeArgument(proposal.ValidTime.From), semanticTimeArgument(proposal.ValidTime.To),
				proposal.OperationID, transactionText); err != nil {
				return fmt.Errorf("create semantic Claim: %w", err)
			}
		} else {
			var id string
			if err := writer.queryRowContext(ctx, `
				SELECT claim_id FROM semantic_claims
				WHERE claim_id = ? AND scope_id = ? AND subject_entity_id = ? AND predicate_id = ?
				  AND object_kind = 'literal' AND literal_kind = ? AND literal_value = ?
				  AND polarity = ? AND valid_from IS ? AND valid_to IS ?
			`, proposal.ClaimID, proposal.Scope.ID, proposal.Subject.ID, proposal.Predicate.ID,
				proposal.Literal.Kind, proposal.Literal.Value, proposal.Polarity,
				semanticTimeArgument(proposal.ValidTime.From), semanticTimeArgument(proposal.ValidTime.To)).Scan(&id); err != nil {
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
			`, proposal.SourceLinkID, proposal.ClaimID, proposal.Source.EventID, proposal.SessionID, proposal.Source.ScopeKey,
				proposal.Source.EventPart, proposal.Source.LocatorKind, proposal.Source.LocatorValue,
				proposal.Source.EvidenceSHA256, proposal.Source.Actor, proposal.Source.SourceType,
				proposal.Source.Authority, proposal.Source.ObservedAt, proposal.OperationID); err != nil {
				return fmt.Errorf("create semantic Source Link: %w", err)
			}
		} else {
			var id, claimID, eventID, sessionID, scopeKey, eventPart, locatorKind, locatorValue string
			var evidenceHash, actor, sourceType, authority, observedAt, eligibility, operationID string
			if err := writer.queryRowContext(ctx, `
				SELECT source_link_id, claim_id, event_id, source_session_id, source_scope_key,
				       event_part, locator_kind, locator_value, evidence_sha256, source_actor,
				       source_type, authority, observed_at, eligibility, created_operation_id
				FROM semantic_source_links WHERE source_link_id = ?
			`, proposal.SourceLinkID).Scan(&id, &claimID, &eventID, &sessionID, &scopeKey, &eventPart,
				&locatorKind, &locatorValue, &evidenceHash, &actor, &sourceType, &authority,
				&observedAt, &eligibility, &operationID); err != nil || id != string(proposal.SourceLinkID) ||
				claimID != string(proposal.ClaimID) || eventID != string(proposal.Source.EventID) ||
				sessionID != string(proposal.Source.SessionID) || scopeKey != proposal.Source.ScopeKey ||
				eventPart != string(proposal.Source.EventPart) || locatorKind != string(proposal.Source.LocatorKind) ||
				locatorValue != proposal.Source.LocatorValue || evidenceHash != proposal.Source.EvidenceSHA256 ||
				actor != string(proposal.Source.Actor) || sourceType != string(proposal.Source.SourceType) ||
				authority != string(proposal.Source.Authority) || observedAt != proposal.Source.ObservedAt ||
				eligibility != "eligible" || operationID != string(proposal.Source.OperationID) {
				return errors.New("semantic Source Link changed after preparation")
			}
		}
		if proposal.ClaimCreate {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (
					scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time
				) VALUES (?, 'claim', ?, 'active', ?, ?, ?)
			`, proposal.Scope.ID, proposal.ClaimID, proposal.OperationID, result.ScopeRevision, transactionText); err != nil {
				return fmt.Errorf("create semantic state event: %w", err)
			}
		}
		if proposal.Source.Create {
			if _, err := writer.execContext(ctx, `
				INSERT INTO semantic_state_events (
					scope_id, object_kind, object_id, state, operation_id, scope_revision, transaction_time
				) VALUES (?, 'source_link', ?, 'eligible', ?, ?, ?)
			`, proposal.Scope.ID, proposal.SourceLinkID, proposal.OperationID, result.ScopeRevision, transactionText); err != nil {
				return fmt.Errorf("create semantic source state event: %w", err)
			}
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
