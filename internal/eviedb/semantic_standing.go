package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// The standing tier (docs/memory-architecture.md, Stage 1). A Claim's kind
// and standing flag are append-only rows written by the accepted operation
// that set them: a remember, correction or promotion carrying a
// classification, or an explicit classify_claim operation. The latest row per
// Claim is current; a Claim with no row is an unclassified fact that is not
// standing, so Claims accepted before this tier are never rewritten.
const semanticClaimClassificationsSchema = `
CREATE TABLE IF NOT EXISTS semantic_claim_classifications (
    claim_id         TEXT NOT NULL REFERENCES semantic_claims(claim_id),
    scope_id         TEXT NOT NULL REFERENCES semantic_scopes(scope_id),
    kind             TEXT NOT NULL CHECK (kind IN ('preference', 'instruction', 'profile_fact', 'workspace_brief', 'fact')),
    standing         INTEGER NOT NULL CHECK (standing IN (0, 1)),
    operation_id     TEXT NOT NULL REFERENCES semantic_operations(operation_id),
    scope_revision   INTEGER NOT NULL CHECK (scope_revision > 0),
    transaction_time TEXT NOT NULL,
    PRIMARY KEY (operation_id, claim_id)
);
CREATE INDEX IF NOT EXISTS semantic_claim_classifications_claim_idx ON semantic_claim_classifications(claim_id, scope_revision);
CREATE TRIGGER IF NOT EXISTS semantic_claim_classifications_append_only_update BEFORE UPDATE ON semantic_claim_classifications BEGIN SELECT RAISE(ABORT, 'semantic claim classifications are append-only'); END;
CREATE TRIGGER IF NOT EXISTS semantic_claim_classifications_append_only_delete BEFORE DELETE ON semantic_claim_classifications BEGIN SELECT RAISE(ABORT, 'semantic claim classifications are append-only'); END;
`

func ensureSemanticClaimClassifications(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, semanticClaimClassificationsSchema); err != nil {
		return fmt.Errorf("create semantic claim classifications: %w", err)
	}
	return nil
}

// semanticOperationsV7Constraint admits schema version 7, the classify_claim
// operation. Earlier migrations recognize it as already newer.
const semanticOperationsV7Constraint = "schema_version IN (1, 2, 3, 4, 5, 6, 7)"

func ensureSemanticOperationSchemaV7(ctx context.Context, db *sql.DB) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='semantic_operations'`).Scan(&definition); err != nil {
		return err
	}
	if strings.Contains(definition, semanticOperationsV7Constraint) {
		return nil
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys=ON`)
	if _, err = conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()
	migration := strings.ReplaceAll(semanticOperationsV5Migration, "semantic_operations_v5", "semantic_operations_v7")
	migration = strings.ReplaceAll(migration, "schema_version IN (1, 2, 3, 4, 5)", semanticOperationsV7Constraint)
	if _, err = conn.ExecContext(ctx, migration); err != nil {
		return fmt.Errorf("migrate semantic operation v7: %w", err)
	}
	rows, err := conn.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	invalid := rows.Next()
	scanErr := rows.Err()
	rows.Close()
	if scanErr != nil {
		return scanErr
	}
	if invalid {
		return errors.New("semantic v7 migration foreign key violation")
	}
	if _, err = conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

type semanticReplayContextKey struct{}

// withSemanticReplay marks an Apply that re-executes accepted history. Replay
// re-verifies structure only, so approval-time policy such as the standing
// budget is not re-run, exactly as source matching is not.
func withSemanticReplay(ctx context.Context) context.Context {
	return context.WithValue(ctx, semanticReplayContextKey{}, true)
}

func isSemanticReplay(ctx context.Context) bool {
	replay, _ := ctx.Value(semanticReplayContextKey{}).(bool)
	return replay
}

// loadClaimClassification returns a Claim's current classification and the
// operation that set it. An unclassified Claim reads as a fact that is not
// standing, with present false.
func loadClaimClassification(ctx context.Context, query lifecycleQueryer, claimID memory.SemanticID) (memory.ClaimClassification, memory.SemanticID, bool, error) {
	classification := memory.ClaimClassification{Kind: memory.ClaimKindFact}
	var operationID memory.SemanticID
	var standing int
	err := query.queryRowContext(ctx, `
		SELECT kind, standing, operation_id FROM semantic_claim_classifications
		WHERE claim_id = ? ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC LIMIT 1
	`, claimID).Scan(&classification.Kind, &standing, &operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return classification, "", false, nil
	}
	if err != nil {
		return classification, "", false, err
	}
	classification.Standing = standing == 1
	return classification, operationID, true, nil
}

// deriveClaimClassification is the classification a remember, correction or
// promotion proposes. A proposed kind keeps a same-kind prior standing flag
// (an owner's earlier switch is not undone) and otherwise takes the kind's
// default; with no kind a classified prior carries over and an unclassified
// one stays unclassified, as before the standing tier.
func deriveClaimClassification(kind memory.ClaimKind, prior memory.ClaimClassification, priorPresent bool, ratio int64) (*memory.ClaimClassification, error) {
	if ratio < 0 {
		return nil, errors.New("standing budget ratio must not be negative")
	}
	switch {
	case kind != "":
		if !kind.Valid() {
			return nil, fmt.Errorf("unsupported memory kind %q; use preference, instruction, profile_fact, workspace_brief or fact", kind)
		}
		standing := memory.DefaultStanding(kind)
		if priorPresent && prior.Kind == kind {
			standing = prior.Standing
		}
		return &memory.ClaimClassification{Kind: kind, Standing: standing, BytesPerTokenMilli: ratio}, nil
	case priorPresent:
		return &memory.ClaimClassification{Kind: prior.Kind, Standing: prior.Standing, BytesPerTokenMilli: ratio}, nil
	default:
		return nil, nil
	}
}

// validateProposalClassification checks a proposal's classification against
// its request: a requested kind must be the proposed kind at the requested
// ratio. Without a kind, a remember (inherit false) stays unclassified, while
// a correction or promotion may carry its Claim's classification over. The
// standing flag is the owner's approved choice.
func validateProposalClassification(classification *memory.ClaimClassification, kind memory.ClaimKind, ratio int64, inherit bool) error {
	if classification == nil {
		if kind != "" {
			return errors.New("classified memory request has no classification")
		}
		return nil
	}
	if kind == "" && !inherit {
		return errors.New("unclassified memory request carries a classification")
	}
	if err := classification.Validate(); err != nil {
		return err
	}
	if (kind != "" && classification.Kind != kind) || classification.BytesPerTokenMilli != ratio {
		return errors.New("memory classification differs from its request")
	}
	return nil
}

// standingAlternative hashes proposal with its standing flag switched. The
// approval card offers that exact alternative, so switching "Always applies"
// before approving approves a fully prepared proposal rather than an edit.
func standingAlternative[T any](proposal T, classification func(*T) **memory.ClaimClassification, canonical func(T) any) (*memory.ProposalHashes, error) {
	current := *classification(&proposal)
	if current == nil {
		return nil, nil
	}
	switched := *current
	switched.Standing = !switched.Standing
	*classification(&proposal) = &switched
	proposalHash, _, err := semanticHash(canonical(proposal))
	if err != nil {
		return nil, err
	}
	preparedHash, _, err := semanticHash(proposal)
	if err != nil {
		return nil, err
	}
	return &memory.ProposalHashes{ProposalSHA256: proposalHash, PreparedSHA256: preparedHash}, nil
}

type canonicalClassification struct {
	ClaimID          memory.SemanticID `json:"claim_id"`
	ScopeKey         string            `json:"scope_key"`
	Kind             memory.ClaimKind  `json:"kind"`
	Standing         bool              `json:"standing"`
	PriorOperationID memory.SemanticID `json:"prior_operation_id,omitempty"`
}

// canonicalClassifications is the effect entry for a proposal's
// classification; nil (and so omitted) for a pre-standing proposal, which
// keeps every earlier proposal and effect hash unchanged.
func canonicalClassifications(classification *memory.ClaimClassification, claimID memory.SemanticID, scopeKey string) []canonicalClassification {
	if classification == nil {
		return nil
	}
	return []canonicalClassification{{ClaimID: claimID, ScopeKey: scopeKey, Kind: classification.Kind, Standing: classification.Standing}}
}

// applyClaimClassification writes an accepted classification and, when it
// makes the Claim standing, enforces the standing budget inside the same
// transaction; "memory full" rolls the whole operation back.
func (s *Store) applyClaimClassification(ctx context.Context, writer turnLeaseWriteExecutor, sessionID memory.SessionID,
	classification *memory.ClaimClassification, claimID, scopeID, operationID memory.SemanticID, scopeRevision int64, transactionText string,
) error {
	if classification == nil {
		return nil
	}
	if _, err := writer.execContext(ctx, `
		INSERT INTO semantic_claim_classifications (claim_id, scope_id, kind, standing, operation_id, scope_revision, transaction_time)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, claimID, scopeID, classification.Kind, classification.Standing, operationID, scopeRevision, transactionText); err != nil {
		return fmt.Errorf("record memory classification: %w", err)
	}
	if !classification.Standing || isSemanticReplay(ctx) {
		return nil
	}
	return s.enforceStandingBudget(ctx, writer, sessionID, claimID, classification.BytesPerTokenMilli)
}

// enforceStandingBudget measures the approving session's standing set,
// candidate included, against the candidate's tier budget.
func (s *Store) enforceStandingBudget(ctx context.Context, writer turnLeaseWriteExecutor, sessionID memory.SessionID, claimID memory.SemanticID, ratio int64) error {
	var workspaceID, projectID sql.NullString
	if err := writer.queryRowContext(ctx, `SELECT workspace_id, project_id FROM sessions WHERE id = ?`, sessionID).Scan(&workspaceID, &projectID); err != nil {
		return fmt.Errorf("load standing budget session: %w", err)
	}
	scope := memory.ScopeContext{SessionID: sessionID}
	if workspaceID.Valid {
		scope.WorkspaceID = memory.WorkspaceID(workspaceID.String)
	} else if projectID.Valid {
		scope.ProjectID = memory.ProjectID(projectID.String)
	}
	items, err := loadStandingMemory(ctx, writer, allowedSemanticReadScopeKeys(scope), s.now())
	if err != nil {
		return err
	}
	others := make([]memory.StandingMemoryItem, 0, len(items))
	var candidate *memory.StandingMemoryItem
	for i := range items {
		if items[i].ClaimID == claimID {
			candidate = &items[i]
			continue
		}
		others = append(others, items[i])
	}
	if candidate == nil {
		// Not current now (a future Valid Time): nothing renders yet.
		return nil
	}
	return memory.CheckStandingBudget(others, *candidate, ratio)
}

// loadStandingMemory returns the active, approved, standing Claims in the
// given scopes that are valid at now and keep an eligible source, ordered
// Global, then Workspace or project, then session, and within a scope by when
// each became standing. Quarantined scopes contribute nothing.
func loadStandingMemory(ctx context.Context, query lifecycleQueryer, scopeKeys []string, now time.Time) ([]memory.StandingMemoryItem, error) {
	if len(scopeKeys) == 0 {
		return nil, nil
	}
	at := formatSemanticTime(now.UTC())
	args := make([]any, 0, len(scopeKeys)+2)
	for _, key := range scopeKeys {
		args = append(args, key)
	}
	args = append(args, at, at)
	rows, err := query.queryContext(ctx, `
		SELECT claims.claim_id, scopes.scope_key, classes.kind, subject.canonical_name, predicates.label,
		       claims.object_kind, COALESCE(claims.literal_value, ''), COALESCE(object.canonical_name, ''), claims.polarity
		FROM semantic_claim_classifications AS classes
		JOIN semantic_claims AS claims ON claims.claim_id = classes.claim_id
		JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_entities AS subject ON subject.entity_id = claims.subject_entity_id
		LEFT JOIN semantic_entities AS object ON object.entity_id = claims.object_entity_id
		WHERE scopes.scope_key IN (`+strings.TrimSuffix(strings.Repeat("?,", len(scopeKeys)), ",")+`)
		  AND scopes.scope_id NOT IN (SELECT scope_id FROM semantic_projection_quarantine)
		  AND classes.standing = 1
		  AND NOT EXISTS (
		      SELECT 1 FROM semantic_claim_classifications AS newer
		      WHERE newer.claim_id = classes.claim_id AND newer.scope_revision > classes.scope_revision)
		  AND COALESCE((SELECT state FROM semantic_state_events
		      WHERE object_kind = 'claim' AND object_id = claims.claim_id
		      ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), claims.lifecycle) = 'active'
		  AND EXISTS (
		      SELECT 1 FROM semantic_source_links AS sources
		      WHERE sources.claim_id = claims.claim_id
		        AND COALESCE((SELECT state FROM semantic_state_events
		            WHERE object_kind = 'source_link' AND object_id = sources.source_link_id
		            ORDER BY scope_revision DESC, transaction_time DESC, operation_id DESC, state DESC LIMIT 1), sources.eligibility) = 'eligible')
		  AND (claims.valid_from IS NULL OR claims.valid_from <= ?)
		  AND (claims.valid_to IS NULL OR claims.valid_to > ?)
		ORDER BY classes.transaction_time, claims.claim_id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load standing memory: %w", err)
	}
	defer rows.Close()
	var items []memory.StandingMemoryItem
	for rows.Next() {
		var item memory.StandingMemoryItem
		var subject, label, objectKind, literal, objectName string
		var polarity memory.ClaimPolarity
		if err := rows.Scan(&item.ClaimID, &item.ScopeKey, &item.Kind, &subject, &label, &objectKind, &literal, &objectName, &polarity); err != nil {
			return nil, err
		}
		item.Text = standingClaimText(subject, label, objectKind, literal, objectName, polarity)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memory.SortStandingMemory(items), nil
}

// standingClaimText is renderClaimText's wording: the Claim, never its source.
func standingClaimText(subject, label, objectKind, literal, objectName string, polarity memory.ClaimPolarity) string {
	object := literal
	if objectKind == "entity" {
		object = objectName
	}
	text := fmt.Sprintf("%s — %s: %s", subject, label, object)
	if polarity == memory.PolarityDenied {
		text = "Denied: " + text
	}
	return text
}

// StandingMemory returns the standing set visible in one session's scope
// context: Global, its Workspace or project, and the session itself.
func (s *Store) StandingMemory(ctx context.Context, scope memory.ScopeContext) ([]memory.StandingMemoryItem, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return nil, err
	}
	return loadStandingMemory(ctx, dbLifecycleQueryer{s.db}, allowedSemanticReadScopeKeys(scope), s.now())
}

// StandingMemory is the bound session's standing set. Delegated sessions
// have none: they do not read the owner's memory.
func (h *SessionHistory) StandingMemory(ctx context.Context) ([]memory.StandingMemoryItem, error) {
	session, err := h.store.GetActiveSession(ctx, h.sessionID)
	if err != nil {
		return nil, err
	}
	if session.ParentSessionID != "" {
		return nil, nil
	}
	return h.store.StandingMemory(ctx, session.ScopeContext())
}

type canonicalClaimStandingEffect struct {
	Scopes          []string                  `json:"scopes"`
	Classifications []canonicalClassification `json:"classifications"`
}

type canonicalClaimStandingProposal struct {
	Kind           string                       `json:"kind"`
	IdempotencyKey string                       `json:"idempotency_key"`
	Actor          memory.SemanticActor         `json:"actor"`
	SessionID      memory.SessionID             `json:"session_id"`
	PriorRevisions []memory.ScopeRevision       `json:"prior_revisions"`
	SourceEventIDs []memory.EventID             `json:"source_event_ids"`
	Effect         canonicalClaimStandingEffect `json:"effect"`
}

func canonicalClaimStanding(proposal memory.ClaimStandingProposal) canonicalClaimStandingProposal {
	return canonicalClaimStandingProposal{
		Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey, Actor: proposal.Actor,
		SessionID: proposal.SessionID, PriorRevisions: proposal.PriorRevisions,
		SourceEventIDs: []memory.EventID{proposal.Evidence.EventID},
		Effect: canonicalClaimStandingEffect{
			Scopes: []string{proposal.Scope.Key},
			Classifications: []canonicalClassification{{
				ClaimID: proposal.Claim.ID, ScopeKey: proposal.Scope.Key, Kind: proposal.Classification.Kind,
				Standing: proposal.Classification.Standing, PriorOperationID: proposal.PriorOperationID,
			}},
		},
	}
}

const claimStandingEvidenceQuery = `SELECT session_id, event_type, COALESCE(role, ''), content, recorded_at FROM events WHERE id = ? AND session_id IN (SELECT id FROM sessions WHERE parent_session_id IS NULL)`

// PrepareClaimStanding proposes marking one existing Claim standing or not,
// optionally with a new kind. The Claim must be active and in a scope this
// session reads; the approval card shows its text and both classifications.
func (s *Store) PrepareClaimStanding(ctx context.Context, scope memory.ScopeContext, request memory.ClaimStandingRequest) (memory.ClaimStandingProposal, error) {
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	if !strings.HasPrefix(request.IdempotencyKey, "idem:v1:") || validateSemanticUUID(strings.TrimPrefix(request.IdempotencyKey, "idem:v1:")) != nil {
		return memory.ClaimStandingProposal{}, errors.New("idempotency key must be idem:v1:<canonical-uuidv4>")
	}
	if err := validateSemanticUUID(string(request.ClaimID)); err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	if request.Kind != "" && !request.Kind.Valid() {
		return memory.ClaimStandingProposal{}, fmt.Errorf("unsupported memory kind %q; use preference, instruction, profile_fact, workspace_brief or fact", request.Kind)
	}
	if request.StandingBytesPerTokenMilli < 0 {
		return memory.ClaimStandingProposal{}, errors.New("standing budget ratio must not be negative")
	}
	var priorJSON, priorHash string
	err := s.db.QueryRowContext(ctx, `SELECT prepared_proposal_json, proposal_sha256 FROM semantic_operations WHERE idempotency_key = ?`, request.IdempotencyKey).Scan(&priorJSON, &priorHash)
	if err == nil {
		var proposal memory.ClaimStandingProposal
		if err := json.Unmarshal([]byte(priorJSON), &proposal); err != nil {
			return proposal, err
		}
		if proposal.Request != request || proposal.SessionID != scope.SessionID {
			return memory.ClaimStandingProposal{}, ErrIdempotencyConflict
		}
		proposal.ProposalSHA256 = priorHash
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
		return proposal, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return memory.ClaimStandingProposal{}, err
	}
	claim, err := loadSemanticClaim(ctx, s.db, request.ClaimID)
	if err != nil {
		return memory.ClaimStandingProposal{}, fmt.Errorf("load standing Claim: %w", err)
	}
	if !containsScopeKey(allowedSemanticReadScopeKeys(scope), claim.ScopeKey) {
		return memory.ClaimStandingProposal{}, errors.New("standing Claim is outside this conversation's scopes")
	}
	if err := requireSemanticScopeKeysAvailable(ctx, s.db, []string{claim.ScopeKey}); err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	query := dbLifecycleQueryer{s.db}
	state, err := loadLatestState(ctx, query, memory.SemanticObjectClaim, claim.ID)
	if err != nil || state.State != memory.SemanticStateActive {
		return memory.ClaimStandingProposal{}, errors.New("only an active Claim can change standing")
	}
	prior, priorOperation, _, err := loadClaimClassification(ctx, query, claim.ID)
	if err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	kind := request.Kind
	if kind == "" {
		kind = prior.Kind
	}
	if kind == prior.Kind && request.Standing == prior.Standing {
		return memory.ClaimStandingProposal{}, fmt.Errorf("Claim is already a %s that %s", kind, map[bool]string{true: "always applies", false: "does not always apply"}[prior.Standing])
	}
	text, err := loadStandingClaimText(ctx, query, claim.ID)
	if err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	evidence, err := loadLifecycleEvidence(s.db.QueryRowContext(ctx, claimStandingEvidenceQuery, request.SourceEventID), scope.SessionID, request.SourceEventID, scopeKeyForContext(scope))
	if err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	target, err := loadSemanticScope(ctx, s.db, claim.ScopeKey)
	if err != nil || target.Create {
		return memory.ClaimStandingProposal{}, errors.New("standing Claim scope is not registered")
	}
	operationID, err := newSemanticID()
	if err != nil {
		return memory.ClaimStandingProposal{}, err
	}
	proposal := memory.ClaimStandingProposal{
		SchemaVersion: 7, Kind: "classify_claim", OperationID: operationID, IdempotencyKey: request.IdempotencyKey,
		Actor: memory.SemanticActorOwner, SessionID: scope.SessionID, Scope: target.SemanticScope,
		Scopes: []memory.SemanticScope{target.SemanticScope}, PriorRevisions: []memory.ScopeRevision{{ScopeKey: target.Key, Revision: target.Revision}},
		Claim: claim, ClaimText: text, Prior: prior, PriorOperationID: priorOperation,
		Classification: memory.ClaimClassification{Kind: kind, Standing: request.Standing, BytesPerTokenMilli: request.StandingBytesPerTokenMilli},
		Evidence:       evidence, Request: request,
	}
	proposal.ProposalSHA256, _, err = semanticHash(canonicalClaimStanding(proposal))
	if err == nil {
		proposal.PreparedSHA256, _, err = semanticHash(proposal)
	}
	return proposal, err
}

func containsScopeKey(keys []string, key string) bool {
	for _, candidate := range keys {
		if candidate == key {
			return true
		}
	}
	return false
}

func loadStandingClaimText(ctx context.Context, query lifecycleQueryer, claimID memory.SemanticID) (string, error) {
	var subject, label, objectKind, literal, objectName string
	var polarity memory.ClaimPolarity
	if err := query.queryRowContext(ctx, `
		SELECT subject.canonical_name, predicates.label, claims.object_kind, COALESCE(claims.literal_value, ''),
		       COALESCE(object.canonical_name, ''), claims.polarity
		FROM semantic_claims AS claims
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_entities AS subject ON subject.entity_id = claims.subject_entity_id
		LEFT JOIN semantic_entities AS object ON object.entity_id = claims.object_entity_id
		WHERE claims.claim_id = ?
	`, claimID).Scan(&subject, &label, &objectKind, &literal, &objectName, &polarity); err != nil {
		return "", fmt.Errorf("load standing Claim text: %w", err)
	}
	return standingClaimText(subject, label, objectKind, literal, objectName, polarity), nil
}

func validateClaimStandingProposal(proposal memory.ClaimStandingProposal) error {
	request := proposal.Request
	if proposal.SchemaVersion != 7 || proposal.Kind != "classify_claim" || proposal.Actor != memory.SemanticActorOwner ||
		proposal.IdempotencyKey != request.IdempotencyKey || proposal.Claim.ID != request.ClaimID ||
		proposal.Evidence.EventID != request.SourceEventID || proposal.Claim.ScopeKey != proposal.Scope.Key ||
		len(proposal.Scopes) != 1 || proposal.Scopes[0] != proposal.Scope ||
		len(proposal.PriorRevisions) != 1 || proposal.PriorRevisions[0] != (memory.ScopeRevision{ScopeKey: proposal.Scope.Key, Revision: proposal.Scope.Revision}) {
		return errors.New("invalid standing proposal")
	}
	if err := proposal.Classification.Validate(); err != nil {
		return err
	}
	if err := proposal.Prior.Validate(); err != nil {
		return err
	}
	if (request.Kind != "" && proposal.Classification.Kind != request.Kind) || (request.Kind == "" && proposal.Classification.Kind != proposal.Prior.Kind) ||
		proposal.Classification.Standing != request.Standing || proposal.Classification.BytesPerTokenMilli != request.StandingBytesPerTokenMilli {
		return errors.New("standing proposal differs from its request")
	}
	for _, id := range []memory.SemanticID{proposal.OperationID, proposal.Claim.ID, proposal.Scope.ID} {
		if err := validateSemanticUUID(string(id)); err != nil {
			return err
		}
	}
	return nil
}

// ApplyClaimStanding commits an approved standing change: it requires the
// exact durable approval, the unchanged Claim and classification it was
// prepared against, and, when it makes the Claim standing, room in its tier.
func (s *Store) ApplyClaimStanding(ctx context.Context, lease memory.TurnLease, proposal memory.ClaimStandingProposal) (result memory.ClaimStandingResult, err error) {
	if lease.SessionID != proposal.SessionID {
		return result, errors.New("standing proposal does not match its turn lease")
	}
	canonical := canonicalClaimStanding(proposal)
	proposalHash, proposalJSON, err := semanticHash(canonical)
	if err != nil {
		return result, err
	}
	preparedHash, preparedJSON, err := semanticHash(proposal)
	if err != nil {
		return result, err
	}
	if proposal.ProposalSHA256 == "" || proposal.ProposalSHA256 != proposalHash || proposal.PreparedSHA256 != preparedHash {
		return result, errors.New("standing proposal hash changed")
	}
	if err := validateClaimStandingProposal(proposal); err != nil {
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
		var workspaceID, projectID sql.NullString
		if err := writer.queryRowContext(ctx, `SELECT workspace_id, project_id FROM sessions WHERE id = ? AND status = ?`, proposal.SessionID, memory.SessionActive).Scan(&workspaceID, &projectID); err != nil {
			return fmt.Errorf("load active standing session: %w", err)
		}
		bound := memory.ScopeContext{SessionID: proposal.SessionID}
		if workspaceID.Valid {
			bound.WorkspaceID = memory.WorkspaceID(workspaceID.String)
		} else if projectID.Valid {
			bound.ProjectID = memory.ProjectID(projectID.String)
		}
		if !containsScopeKey(allowedSemanticReadScopeKeys(bound), proposal.Scope.Key) {
			return errors.New("standing Claim is outside the session's scopes")
		}
		byKey, err := validateSemanticScopeVector(ctx, writer, proposal.Scopes, proposal.PriorRevisions, s.now())
		if err != nil {
			return err
		}
		if byKey[proposal.Scope.Key] != proposal.Scope {
			return errors.New("standing Claim scope changed")
		}
		claim, err := loadSemanticClaimFromWriter(ctx, writer, proposal.Claim.ID)
		if err != nil || !semanticClaimsEqual(claim, proposal.Claim) {
			return errors.New("standing Claim changed after preparation")
		}
		state, err := loadLatestState(ctx, writer, memory.SemanticObjectClaim, claim.ID)
		if err != nil || state.State != memory.SemanticStateActive {
			return errors.New("standing Claim is no longer active")
		}
		prior, priorOperation, _, err := loadClaimClassification(ctx, writer, claim.ID)
		if err != nil {
			return err
		}
		if prior != proposal.Prior || priorOperation != proposal.PriorOperationID {
			return errors.New("standing Claim classification changed after preparation")
		}
		evidence, err := loadLifecycleEvidence(writer.queryRowContext(ctx, claimStandingEvidenceQuery, proposal.Evidence.EventID), proposal.SessionID, proposal.Evidence.EventID, scopeKeyForContext(bound))
		if err != nil || evidence != proposal.Evidence {
			return errors.New("standing request evidence changed")
		}
		now, err := nextSemanticTransactionTime(ctx, writer, s.now())
		if err != nil {
			return err
		}
		result = memory.ClaimStandingResult{
			OperationID: proposal.OperationID, ClaimID: claim.ID, TransactionTime: now, ScopeRevision: proposal.Scope.Revision + 1,
			ResultingRevisions: []memory.ScopeRevision{{ScopeKey: proposal.Scope.Key, Revision: proposal.Scope.Revision + 1}},
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
			SchemaVersion: 7, OperationID: proposal.OperationID, Kind: proposal.Kind, IdempotencyKey: proposal.IdempotencyKey,
			Actor: proposal.Actor, SessionID: proposal.SessionID, TargetScopeID: proposal.Scope.ID, SourceEventID: proposal.Evidence.EventID,
			ProposalHash: proposalHash, EffectHash: effectHash, ProposalJSON: proposalJSON, PreparedJSON: preparedJSON, ResultJSON: resultJSON,
			TransactionTime: now, ResultRevisions: result.ResultingRevisions, ScopesByKey: byKey,
		}); err != nil {
			return err
		}
		classification := proposal.Classification
		if err := s.applyClaimClassification(ctx, writer, proposal.SessionID, &classification, claim.ID, proposal.Scope.ID,
			proposal.OperationID, result.ScopeRevision, formatSemanticTime(now)); err != nil {
			return err
		}
		updated, err := writer.execContext(ctx, `UPDATE semantic_scopes SET revision = ? WHERE scope_id = ? AND revision = ?`, result.ScopeRevision, proposal.Scope.ID, proposal.Scope.Revision)
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
