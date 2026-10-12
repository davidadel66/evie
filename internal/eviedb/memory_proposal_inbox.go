package eviedb

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/google/uuid"
)

const memoryProposalInboxTable = `
CREATE TABLE IF NOT EXISTS memory_proposal_inbox (
    proposal_id                 TEXT PRIMARY KEY,
    session_id                  TEXT NOT NULL REFERENCES sessions(id),
    scope_key                   TEXT NOT NULL,
    tool_name                   TEXT NOT NULL CHECK (tool_name IN ('memory_remember_literal', 'memory_remember_entity', 'memory_correct_claim', 'memory_retire')),
    reason                      TEXT NOT NULL CHECK (reason IN ('not_requested', 'no_explicit_cue', 'background')),
    idempotency_key             TEXT NOT NULL UNIQUE,
    source_event_id             TEXT NOT NULL,
    intent_event_id             TEXT,
    target_claim_id             TEXT,
    dedupe_key                  TEXT NOT NULL,
    proposal_json               TEXT NOT NULL,
    proposal_sha256             TEXT NOT NULL,
    prepared_sha256             TEXT NOT NULL,
    alternative_proposal_sha256 TEXT,
    alternative_prepared_sha256 TEXT,
    revision                    INTEGER NOT NULL CHECK (revision > 0),
    queued_at                   TEXT NOT NULL,
    refreshed_at                TEXT,
    CHECK ((reason = 'background') = (intent_event_id IS NULL)),
    CHECK (reason = 'background' OR tool_name IN ('memory_remember_literal', 'memory_remember_entity'))
);`

// The memory proposal inbox (docs/memory-architecture.md, Stages 2 and 4). A
// remember proposal the owner did not explicitly ask for, and every change
// the background pass finds in the owner's conversations (a remember, a
// correction or a retirement), is stored here exactly as prepared, with its
// "Always applies" alternative, instead of interrupting the conversation
// with an approval card. It changes no memory until the owner approves it
// from the inbox, which applies it through the same path as an approval
// card: a durable approval event in the source conversation, then the
// ordinary Apply under that conversation's turn lease. A proposal is pending
// until it is declined or an operation with its idempotency key is accepted;
// nothing else marks it resolved.
const memoryProposalInboxSchema = memoryProposalInboxTable + `
CREATE INDEX IF NOT EXISTS memory_proposal_inbox_dedupe_idx ON memory_proposal_inbox(dedupe_key);
CREATE INDEX IF NOT EXISTS memory_proposal_inbox_scope_idx ON memory_proposal_inbox(scope_key);
CREATE INDEX IF NOT EXISTS memory_proposal_inbox_target_idx ON memory_proposal_inbox(target_claim_id) WHERE target_claim_id IS NOT NULL;
CREATE TABLE IF NOT EXISTS memory_proposal_inbox_declines (
    proposal_id TEXT PRIMARY KEY REFERENCES memory_proposal_inbox(proposal_id),
    declined_at TEXT NOT NULL
);
CREATE TRIGGER IF NOT EXISTS memory_proposal_inbox_declines_append_only_update BEFORE UPDATE ON memory_proposal_inbox_declines BEGIN SELECT RAISE(ABORT, 'memory proposal declines are append-only'); END;
CREATE TRIGGER IF NOT EXISTS memory_proposal_inbox_declines_append_only_delete BEFORE DELETE ON memory_proposal_inbox_declines BEGIN SELECT RAISE(ABORT, 'memory proposal declines are append-only'); END;
`

// memoryProposalInboxBackgroundMarker identifies the Stage 4 table
// definition, which admits background proposals.
const memoryProposalInboxBackgroundMarker = "'background'"

func ensureMemoryProposalInbox(ctx context.Context, db *sql.DB) error {
	if err := migrateMemoryProposalInboxForBackground(ctx, db); err != nil {
		return fmt.Errorf("migrate memory proposal inbox: %w", err)
	}
	if _, err := db.ExecContext(ctx, memoryProposalInboxSchema); err != nil {
		return fmt.Errorf("create memory proposal inbox: %w", err)
	}
	return nil
}

// migrateMemoryProposalInboxForBackground rebuilds a Stage 2 inbox, whose
// checks admit only in-turn remember proposals, into the Stage 4 table: the
// same rows, a nullable intent (background proposals cite none) and the
// target Claim of a correction or retirement. Every existing row and decline
// is kept exactly; no proposal is rewritten.
func migrateMemoryProposalInboxForBackground(ctx context.Context, db *sql.DB) error {
	var definition string
	err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'memory_proposal_inbox'`).Scan(&definition)
	if errors.Is(err, sql.ErrNoRows) || err == nil && strings.Contains(definition, memoryProposalInboxBackgroundMarker) {
		return nil
	}
	if err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `PRAGMA foreign_keys = ON`)
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()
	if err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'memory_proposal_inbox'`).Scan(&definition); err != nil {
		return err
	}
	if strings.Contains(definition, memoryProposalInboxBackgroundMarker) {
		return nil
	}
	rebuilt := strings.Replace(memoryProposalInboxTable, "CREATE TABLE IF NOT EXISTS memory_proposal_inbox (", "CREATE TABLE memory_proposal_inbox_stage4 (", 1)
	columns := `proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id, intent_event_id,
		dedupe_key, proposal_json, proposal_sha256, prepared_sha256, alternative_proposal_sha256, alternative_prepared_sha256,
		revision, queued_at, refreshed_at`
	for _, statement := range []string{
		rebuilt,
		`INSERT INTO memory_proposal_inbox_stage4 (` + columns + `) SELECT ` + columns + ` FROM memory_proposal_inbox`,
		`DROP INDEX IF EXISTS memory_proposal_inbox_dedupe_idx`,
		`DROP INDEX IF EXISTS memory_proposal_inbox_scope_idx`,
		`DROP TABLE memory_proposal_inbox`,
		`ALTER TABLE memory_proposal_inbox_stage4 RENAME TO memory_proposal_inbox`,
	} {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
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
		return errors.New("memory proposal inbox migration foreign key violation")
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

var (
	ErrMemoryProposalNotFound    = errors.New("memory proposal not found")
	ErrMemoryProposalResolved    = errors.New("memory proposal was already approved or declined")
	ErrMemoryProposalChanged     = errors.New("memory proposal changed since it was shown; review the updated proposal")
	ErrMemoryProposalBusy        = errors.New("the proposal's conversation is running a turn; approve it when the turn finishes")
	ErrMemoryProposalUnavailable = errors.New("memory proposal can no longer apply")
)

const (
	memoryProposalListLimit = 50
	// inboxSourceMessageLimit bounds the source message an inbox item shows.
	inboxSourceMessageLimit     = 2000
	memoryProposalLeaseDuration = 30 * time.Second
	// inboxPending is the pending condition for the alias inbox.
	inboxPending = `NOT EXISTS (SELECT 1 FROM memory_proposal_inbox_declines AS declines WHERE declines.proposal_id = inbox.proposal_id)
	  AND NOT EXISTS (SELECT 1 FROM semantic_operations AS operations WHERE operations.idempotency_key = inbox.idempotency_key)`
)

// inboxProposal is one exact prepared proposal: a remember of either shape,
// or (from the background pass) a correction or a retirement.
type inboxProposal struct {
	literal    *memory.RememberLiteralProposal
	entity     *memory.RememberEntityProposal
	correction *memory.CorrectClaimProposal
	lifecycle  *memory.MemoryLifecycleProposal
}

func inboxProposalOf(queued memory.QueuedMemoryProposal) (inboxProposal, error) {
	set := 0
	for _, present := range []bool{queued.Literal != nil, queued.Entity != nil, queued.Correction != nil, queued.Lifecycle != nil} {
		if present {
			set++
		}
	}
	p := inboxProposal{literal: queued.Literal, entity: queued.Entity, correction: queued.Correction, lifecycle: queued.Lifecycle}
	switch {
	case set != 1:
	case queued.ToolName == memory.RememberLiteralToolName && p.literal != nil,
		queued.ToolName == memory.RememberEntityToolName && p.entity != nil,
		queued.ToolName == memory.CorrectClaimToolName && p.correction != nil:
		return p, nil
	case queued.ToolName == memory.RetireToolName && p.lifecycle != nil:
		if p.lifecycle.Request.Action != memory.LifecycleRetire || p.lifecycle.ObjectKind != memory.SemanticObjectClaim {
			return inboxProposal{}, errors.New("an inbox retirement retires exactly one Claim")
		}
		return p, nil
	}
	return inboxProposal{}, errors.New("an inbox proposal is exactly one prepared proposal of its tool")
}

func (p inboxProposal) remember() bool { return p.literal != nil || p.entity != nil }

func (p inboxProposal) toolName() string {
	switch {
	case p.literal != nil:
		return memory.RememberLiteralToolName
	case p.entity != nil:
		return memory.RememberEntityToolName
	case p.correction != nil:
		return memory.CorrectClaimToolName
	default:
		return memory.RetireToolName
	}
}

func (p inboxProposal) value() any {
	switch {
	case p.literal != nil:
		return *p.literal
	case p.entity != nil:
		return *p.entity
	case p.correction != nil:
		return *p.correction
	default:
		return *p.lifecycle
	}
}

func (p inboxProposal) sessionID() memory.SessionID {
	switch {
	case p.literal != nil:
		return p.literal.SessionID
	case p.entity != nil:
		return p.entity.SessionID
	case p.correction != nil:
		return p.correction.SessionID
	default:
		return p.lifecycle.SessionID
	}
}

func (p inboxProposal) operationID() memory.SemanticID {
	switch {
	case p.literal != nil:
		return p.literal.OperationID
	case p.entity != nil:
		return p.entity.OperationID
	case p.correction != nil:
		return p.correction.OperationID
	default:
		return p.lifecycle.OperationID
	}
}

func (p inboxProposal) idempotencyKey() string {
	switch {
	case p.literal != nil:
		return p.literal.IdempotencyKey
	case p.entity != nil:
		return p.entity.IdempotencyKey
	case p.correction != nil:
		return p.correction.IdempotencyKey
	default:
		return p.lifecycle.IdempotencyKey
	}
}

// sourceEventID is the owner message the proposal cites, which is also the
// parent of the approval event an inbox approval records.
func (p inboxProposal) sourceEventID() memory.EventID {
	switch {
	case p.literal != nil:
		return p.literal.Source.EventID
	case p.entity != nil:
		return p.entity.Source.EventID
	case p.correction != nil:
		return p.correction.Source.EventID
	default:
		return p.lifecycle.Evidence.EventID
	}
}

func (p inboxProposal) scopeKey() string {
	switch {
	case p.literal != nil:
		return p.literal.Scope.Key
	case p.entity != nil:
		return p.entity.Scope.Key
	case p.correction != nil:
		return p.correction.Scope.Key
	default:
		return p.lifecycle.Scope.Key
	}
}

func (p inboxProposal) scopes() []memory.SemanticScope {
	switch {
	case p.literal != nil:
		return p.literal.Scopes
	case p.entity != nil:
		return p.entity.Scopes
	case p.correction != nil:
		return p.correction.Scopes
	default:
		return p.lifecycle.Scopes
	}
}

// claimID is the Claim the approval creates or changes: the remembered
// Claim, a correction's replacement, or the retired Claim.
func (p inboxProposal) claimID() memory.SemanticID {
	switch {
	case p.literal != nil:
		return p.literal.ClaimID
	case p.entity != nil:
		return p.entity.Claim.ID
	case p.correction != nil:
		return p.correction.ReplacementClaim.ID
	default:
		return p.lifecycle.ObjectID
	}
}

// targetClaimID is the existing Claim a correction or retirement changes.
func (p inboxProposal) targetClaimID() memory.SemanticID {
	switch {
	case p.correction != nil:
		return p.correction.OldClaim.ID
	case p.lifecycle != nil:
		return p.lifecycle.ObjectID
	default:
		return ""
	}
}

func (p inboxProposal) classification() *memory.ClaimClassification {
	switch {
	case p.literal != nil:
		return p.literal.Classification
	case p.entity != nil:
		return p.entity.Classification
	case p.correction != nil:
		return p.correction.Classification
	default:
		return nil
	}
}

func (p inboxProposal) hashes() (string, string) {
	switch {
	case p.literal != nil:
		return p.literal.ProposalSHA256, p.literal.PreparedSHA256
	case p.entity != nil:
		return p.entity.ProposalSHA256, p.entity.PreparedSHA256
	case p.correction != nil:
		return p.correction.ProposalSHA256, p.correction.PreparedSHA256
	default:
		return p.lifecycle.ProposalSHA256, p.lifecycle.PreparedSHA256
	}
}

func (p inboxProposal) alternative() *memory.ProposalHashes {
	switch {
	case p.literal != nil:
		return p.literal.StandingAlternative
	case p.entity != nil:
		return p.entity.StandingAlternative
	case p.correction != nil:
		return p.correction.StandingAlternative
	default:
		return nil
	}
}

// switched is the prepared "Always applies" alternative: the identical
// proposal with standing switched and the hashes Prepare computed for it.
func (p inboxProposal) switched() (inboxProposal, error) {
	classification, alternative := p.classification(), p.alternative()
	if classification == nil || alternative == nil {
		return inboxProposal{}, errors.New("this memory has no Always applies alternative")
	}
	flipped := *classification
	flipped.Standing = !flipped.Standing
	switch {
	case p.literal != nil:
		literal := *p.literal
		literal.Classification, literal.StandingAlternative = &flipped, nil
		literal.ProposalSHA256, literal.PreparedSHA256 = alternative.ProposalSHA256, alternative.PreparedSHA256
		return inboxProposal{literal: &literal}, nil
	case p.entity != nil:
		entity := *p.entity
		entity.Classification, entity.StandingAlternative = &flipped, nil
		entity.ProposalSHA256, entity.PreparedSHA256 = alternative.ProposalSHA256, alternative.PreparedSHA256
		return inboxProposal{entity: &entity}, nil
	default:
		correction := *p.correction
		correction.Classification, correction.StandingAlternative = &flipped, nil
		correction.ProposalSHA256, correction.PreparedSHA256 = alternative.ProposalSHA256, alternative.PreparedSHA256
		return inboxProposal{correction: &correction}, nil
	}
}

// dedupeKey identifies the memory a proposal states, independent of the
// identifiers each preparation mints, so a repeated proposal is not queued
// twice. A correction is identified by the Claim it replaces and the
// replacement it states; a retirement by the Claim it retires.
func (p inboxProposal) dedupeKey() string {
	selector := func(selected memory.EntitySelector) string {
		if selected.EntityID != "" {
			return "id:" + string(selected.EntityID)
		}
		return "name:" + strings.ToLower(strings.TrimSpace(selected.CanonicalName)) + "|" + strings.ToLower(strings.TrimSpace(selected.EntityType))
	}
	var key any
	switch {
	case p.literal != nil:
		key = []any{"literal", p.literal.Scope.Key, p.literal.Predicate.Token, p.literal.Literal, p.literal.Polarity, p.literal.ValidTime}
	case p.entity != nil:
		key = []any{"entity", p.entity.Scope.Key, p.entity.Predicate.Token, selector(p.entity.Request.Subject), selector(p.entity.Request.Object),
			p.entity.Claim.Polarity, p.entity.Claim.ValidTime}
	case p.correction != nil:
		key = []any{"correct", p.correction.OldClaim.ID, p.correction.Request.Replacement.Object, p.correction.Request.Replacement.Polarity}
	default:
		key = []any{"retire", p.lifecycle.ObjectKind, p.lifecycle.ObjectID}
	}
	encoded, _ := json.Marshal(key)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// decodeInboxProposal restores a stored proposal and proves the round trip
// reproduces exactly the prepared document whose hash was stored.
func decodeInboxProposal(toolName, proposalJSON, proposalHash, preparedHash string, alternativeProposal, alternativePrepared sql.NullString) (inboxProposal, error) {
	var alternative *memory.ProposalHashes
	if alternativeProposal.Valid && alternativePrepared.Valid {
		alternative = &memory.ProposalHashes{ProposalSHA256: alternativeProposal.String, PreparedSHA256: alternativePrepared.String}
	}
	var p inboxProposal
	switch toolName {
	case memory.RememberLiteralToolName:
		var literal memory.RememberLiteralProposal
		if err := json.Unmarshal([]byte(proposalJSON), &literal); err != nil {
			return p, fmt.Errorf("decode stored proposal: %w", err)
		}
		literal.ProposalSHA256, literal.PreparedSHA256, literal.StandingAlternative = proposalHash, preparedHash, alternative
		p.literal = &literal
	case memory.RememberEntityToolName:
		var entity memory.RememberEntityProposal
		if err := json.Unmarshal([]byte(proposalJSON), &entity); err != nil {
			return p, fmt.Errorf("decode stored proposal: %w", err)
		}
		entity.ProposalSHA256, entity.PreparedSHA256, entity.StandingAlternative = proposalHash, preparedHash, alternative
		p.entity = &entity
	case memory.CorrectClaimToolName:
		var correction memory.CorrectClaimProposal
		if err := json.Unmarshal([]byte(proposalJSON), &correction); err != nil {
			return p, fmt.Errorf("decode stored proposal: %w", err)
		}
		correction.ProposalSHA256, correction.PreparedSHA256, correction.StandingAlternative = proposalHash, preparedHash, alternative
		p.correction = &correction
	case memory.RetireToolName:
		var lifecycle memory.MemoryLifecycleProposal
		if err := json.Unmarshal([]byte(proposalJSON), &lifecycle); err != nil {
			return p, fmt.Errorf("decode stored proposal: %w", err)
		}
		if alternative != nil {
			return p, errors.New("a retirement has no Always applies alternative")
		}
		lifecycle.ProposalSHA256, lifecycle.PreparedSHA256 = proposalHash, preparedHash
		p.lifecycle = &lifecycle
	default:
		return p, fmt.Errorf("unsupported inbox tool %q", toolName)
	}
	if hash, _, err := semanticHash(p.value()); err != nil || hash != preparedHash {
		return inboxProposal{}, errors.New("stored proposal does not match its prepared hash")
	}
	return p, nil
}

// encodeInboxProposal is the exact prepared document the approval card would
// show, checked for secrets: a memory that may hold one is never queued.
func encodeInboxProposal(p inboxProposal) ([]byte, error) {
	encoded, err := json.Marshal(p.value())
	if err != nil {
		return nil, err
	}
	if memory.HasRetrievalSecret(encoded) {
		return nil, errInboxProposalSecret
	}
	proposalHash, preparedHash := p.hashes()
	if proposalHash == "" || preparedHash == "" {
		return nil, errors.New("inbox proposal lacks its exact hashes")
	}
	var alternativeProposal, alternativePrepared sql.NullString
	if alternative := p.alternative(); alternative != nil {
		alternativeProposal = sql.NullString{String: alternative.ProposalSHA256, Valid: true}
		alternativePrepared = sql.NullString{String: alternative.PreparedSHA256, Valid: true}
	}
	if _, err := decodeInboxProposal(p.toolName(), string(encoded), proposalHash, preparedHash, alternativeProposal, alternativePrepared); err != nil {
		return nil, err
	}
	return encoded, nil
}

var errInboxProposalSecret = errors.New("this memory may contain a secret, so it was not queued; never propose secrets as memories")

func alternativeColumns(p inboxProposal) (any, any) {
	if alternative := p.alternative(); alternative != nil {
		return alternative.ProposalSHA256, alternative.PreparedSHA256
	}
	return nil, nil
}

func nullableClaimID(id memory.SemanticID) any {
	if id == "" {
		return nil
	}
	return string(id)
}

// QueueMemoryProposal stores one exact prepared remember proposal in the
// owner's review inbox, fenced by the proposing turn's lease. It changes no
// memory. An identical pending memory is not queued twice; the existing
// proposal is reported as a duplicate.
func (s *Store) QueueMemoryProposal(ctx context.Context, lease memory.TurnLease, queued memory.QueuedMemoryProposal) (memory.MemoryProposalQueued, error) {
	var result memory.MemoryProposalQueued
	p, err := inboxProposalOf(queued)
	if err != nil {
		return result, err
	}
	if !p.remember() {
		return result, errors.New("only a remember proposal is queued from a turn")
	}
	if !queued.Reason.Valid() || queued.Reason == memory.MemoryProposalBackground {
		return result, fmt.Errorf("unsupported inbox reason %q", queued.Reason)
	}
	if lease.SessionID != p.sessionID() {
		return result, errors.New("inbox proposal does not match its turn lease")
	}
	encoded, err := encodeInboxProposal(p)
	if err != nil {
		return result, err
	}
	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var intentType string
		if err := writer.queryRowContext(ctx, `SELECT event_type FROM events WHERE id = ? AND session_id = ?`, queued.IntentEventID, lease.SessionID).Scan(&intentType); err != nil ||
			intentType != string(memory.EventToolIntent) {
			return errors.New("inbox proposal must cite its tool intent in the proposing conversation")
		}
		outcome, err := s.insertInboxProposal(ctx, writer, p, encoded, queued.Reason, string(queued.IntentEventID))
		if err != nil {
			return err
		}
		if outcome.alreadyApplied {
			return fmt.Errorf("%w: this idempotency key already saved a memory", ErrIdempotencyConflict)
		}
		result = memory.MemoryProposalQueued{ProposalID: outcome.proposalID, Duplicate: outcome.duplicate}
		return nil
	})
	return result, err
}

// inboxInsertion reports what inserting one proposal did.
type inboxInsertion struct {
	proposalID         string
	duplicate          bool
	previouslyDeclined bool
	alreadyApplied     bool
}

// insertInboxProposal queues one encoded proposal inside a lease-fenced
// transaction. A proposal whose idempotency key already saved memory is
// reported as already applied; one whose key or memory is already pending is
// a duplicate. Background proposals are also suppressed when the owner
// declined the same memory before, and when another change to the same
// Claim is already pending, so the owner never reviews two competing
// changes to one memory.
func (s *Store) insertInboxProposal(ctx context.Context, writer turnLeaseWriteExecutor, p inboxProposal, encoded []byte,
	reason memory.MemoryProposalReason, intentEventID string,
) (inboxInsertion, error) {
	var outcome inboxInsertion
	var accepted int
	if err := writer.queryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE idempotency_key = ?`, p.idempotencyKey()).Scan(&accepted); err != nil {
		return outcome, err
	}
	if accepted != 0 {
		outcome.alreadyApplied = true
		return outcome, nil
	}
	dedupe := p.dedupeKey()
	var existingID, existingDedupe string
	err := writer.queryRowContext(ctx, `SELECT proposal_id, dedupe_key FROM memory_proposal_inbox WHERE idempotency_key = ?`, p.idempotencyKey()).Scan(&existingID, &existingDedupe)
	if err == nil {
		if existingDedupe != dedupe {
			return outcome, ErrIdempotencyConflict
		}
		outcome.proposalID, outcome.duplicate = existingID, true
		return outcome, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return outcome, err
	}
	err = writer.queryRowContext(ctx, `SELECT inbox.proposal_id FROM memory_proposal_inbox AS inbox
		WHERE inbox.dedupe_key = ? AND `+inboxPending+` ORDER BY inbox.queued_at, inbox.proposal_id LIMIT 1`, dedupe).Scan(&existingID)
	if err == nil {
		outcome.proposalID, outcome.duplicate = existingID, true
		return outcome, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return outcome, err
	}
	if reason == memory.MemoryProposalBackground {
		err = writer.queryRowContext(ctx, `SELECT inbox.proposal_id FROM memory_proposal_inbox AS inbox
			JOIN memory_proposal_inbox_declines AS declines ON declines.proposal_id = inbox.proposal_id
			WHERE inbox.dedupe_key = ? ORDER BY inbox.queued_at, inbox.proposal_id LIMIT 1`, dedupe).Scan(&existingID)
		if err == nil {
			outcome.proposalID, outcome.previouslyDeclined = existingID, true
			return outcome, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return outcome, err
		}
		if target := p.targetClaimID(); target != "" {
			err = writer.queryRowContext(ctx, `SELECT inbox.proposal_id FROM memory_proposal_inbox AS inbox
				WHERE inbox.target_claim_id = ? AND `+inboxPending+` ORDER BY inbox.queued_at, inbox.proposal_id LIMIT 1`, target).Scan(&existingID)
			if err == nil {
				outcome.proposalID, outcome.duplicate = existingID, true
				return outcome, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return outcome, err
			}
		}
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return outcome, err
	}
	var intent any
	if intentEventID != "" {
		intent = intentEventID
	}
	proposalHash, preparedHash := p.hashes()
	alternativeProposal, alternativePrepared := alternativeColumns(p)
	if _, err := writer.execContext(ctx, `
		INSERT INTO memory_proposal_inbox (
			proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id, intent_event_id,
			target_claim_id, dedupe_key, proposal_json, proposal_sha256, prepared_sha256, alternative_proposal_sha256,
			alternative_prepared_sha256, revision, queued_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
	`, id.String(), p.sessionID(), p.scopeKey(), p.toolName(), reason, p.idempotencyKey(), p.sourceEventID(), intent,
		nullableClaimID(p.targetClaimID()), dedupe, string(encoded), proposalHash, preparedHash, alternativeProposal, alternativePrepared,
		formatSemanticTime(s.now())); err != nil {
		return outcome, fmt.Errorf("queue memory proposal: %w", err)
	}
	outcome.proposalID = id.String()
	return outcome, nil
}

// OwnerRequestsMemory reports whether the bound conversation's owner message
// carries an explicit memory cue. It is the harness's half of the explicit
// request rule; a message outside the bound conversation is no request.
func (s *Store) OwnerRequestsMemory(ctx context.Context, scope memory.ScopeContext, eventID memory.EventID) (bool, error) {
	var content string
	err := s.db.QueryRowContext(ctx, `
		SELECT content FROM events
		WHERE id = ? AND session_id = ? AND event_type = ? AND role = ?
		  AND session_id IN (SELECT id FROM sessions WHERE parent_session_id IS NULL)
	`, eventID, scope.SessionID, memory.EventUserMessage, memory.RoleUser).Scan(&content)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load owner request: %w", err)
	}
	return memory.HasExplicitMemoryCue(content), nil
}

type inboxRow struct {
	id, sessionID, scopeKey, toolName, reason, idempotencyKey, dedupe string
	proposalJSON, proposalHash, preparedHash                          string
	alternativeProposal, alternativePrepared                          sql.NullString
	revision                                                          int64
	queuedAt, sessionTitle, sessionStatus                             string
	accepted, declined                                                bool
}

const inboxRowColumns = `inbox.proposal_id, inbox.session_id, inbox.scope_key, inbox.tool_name, inbox.reason, inbox.idempotency_key,
	inbox.dedupe_key, inbox.proposal_json, inbox.proposal_sha256, inbox.prepared_sha256, inbox.alternative_proposal_sha256,
	inbox.alternative_prepared_sha256, inbox.revision, inbox.queued_at, COALESCE(sessions.title, ''), sessions.status,
	EXISTS (SELECT 1 FROM semantic_operations AS operations WHERE operations.idempotency_key = inbox.idempotency_key),
	EXISTS (SELECT 1 FROM memory_proposal_inbox_declines AS declines WHERE declines.proposal_id = inbox.proposal_id)`

func scanInboxRow(scanner rowScanner) (inboxRow, error) {
	var row inboxRow
	err := scanner.Scan(&row.id, &row.sessionID, &row.scopeKey, &row.toolName, &row.reason, &row.idempotencyKey,
		&row.dedupe, &row.proposalJSON, &row.proposalHash, &row.preparedHash, &row.alternativeProposal,
		&row.alternativePrepared, &row.revision, &row.queuedAt, &row.sessionTitle, &row.sessionStatus, &row.accepted, &row.declined)
	return row, err
}

func (s *Store) loadInboxRow(ctx context.Context, id string) (inboxRow, error) {
	row, err := scanInboxRow(s.db.QueryRowContext(ctx, `SELECT `+inboxRowColumns+`
		FROM memory_proposal_inbox AS inbox JOIN sessions ON sessions.id = inbox.session_id WHERE inbox.proposal_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrMemoryProposalNotFound
	}
	return row, err
}

func (row inboxRow) proposal() (inboxProposal, error) {
	return decodeInboxProposal(row.toolName, row.proposalJSON, row.proposalHash, row.preparedHash, row.alternativeProposal, row.alternativePrepared)
}

// ListMemoryProposals returns the pending proposals, oldest first, each
// revalidated against current memory: a proposal prepared against older
// scope revisions is prepared again from its exact request, so what the
// owner sees is what an approval applies. One that can no longer be prepared,
// or whose conversation is archived, is listed as not approvable with why.
func (s *Store) ListMemoryProposals(ctx context.Context) (memory.MemoryProposalPage, error) {
	page := memory.MemoryProposalPage{Items: []memory.MemoryProposalItem{}}
	pending, err := s.PendingMemoryProposalCount(ctx)
	if err != nil {
		return page, err
	}
	page.Pending = pending
	rows, err := s.db.QueryContext(ctx, `SELECT `+inboxRowColumns+`
		FROM memory_proposal_inbox AS inbox JOIN sessions ON sessions.id = inbox.session_id
		WHERE `+inboxPending+` ORDER BY inbox.queued_at, inbox.proposal_id LIMIT ?`, memoryProposalListLimit)
	if err != nil {
		return page, fmt.Errorf("list memory proposals: %w", err)
	}
	var stored []inboxRow
	for rows.Next() {
		row, err := scanInboxRow(rows)
		if err != nil {
			rows.Close()
			return page, err
		}
		stored = append(stored, row)
	}
	if err := rows.Close(); err != nil {
		return page, err
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	for _, row := range stored {
		item, err := s.reviewInboxRow(ctx, row)
		if err != nil {
			return page, err
		}
		if item.SourceMessage, err = s.inboxSourceMessage(ctx, row.id); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// PendingMemoryProposalCount counts pending proposals, optionally only those
// targeting the given scope keys.
func (s *Store) PendingMemoryProposalCount(ctx context.Context, scopeKeys ...string) (int, error) {
	query := `SELECT COUNT(*) FROM memory_proposal_inbox AS inbox WHERE ` + inboxPending
	args := make([]any, 0, len(scopeKeys))
	if len(scopeKeys) > 0 {
		query += ` AND inbox.scope_key IN (` + strings.TrimSuffix(strings.Repeat("?,", len(scopeKeys)), ",") + `)`
		for _, key := range scopeKeys {
			args = append(args, key)
		}
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count memory proposals: %w", err)
	}
	return count, nil
}

// inboxSourceMessage is the owner's message a proposal was prepared from, so
// the reviewer can judge what Evie read. Only the owner's own message in the
// proposal's conversation is returned; anything else returns nothing.
func (s *Store) inboxSourceMessage(ctx context.Context, proposalID string) (string, error) {
	var message string
	err := s.db.QueryRowContext(ctx, `SELECT events.content FROM memory_proposal_inbox AS inbox
		JOIN events ON events.id = inbox.source_event_id AND events.session_id = inbox.session_id
		WHERE inbox.proposal_id = ? AND events.event_type = ? AND events.role = 'user'`, proposalID, memory.EventUserMessage).Scan(&message)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read memory proposal source message: %w", err)
	}
	return clipText(strings.TrimSpace(message), inboxSourceMessageLimit), nil
}

const inboxArchivedProblem = "its conversation is archived; restore the conversation to approve it"

func (s *Store) reviewInboxRow(ctx context.Context, row inboxRow) (memory.MemoryProposalItem, error) {
	item := memory.MemoryProposalItem{
		ProposalID: row.id, SessionID: memory.SessionID(row.sessionID), SessionTitle: row.sessionTitle, ScopeKey: row.scopeKey,
		ToolName: row.toolName, Reason: memory.MemoryProposalReason(row.reason), Revision: row.revision,
		Proposal: row.proposalJSON, PreparedSHA256: row.preparedHash,
	}
	if queuedAt, err := parseSemanticTime(row.queuedAt); err == nil {
		item.QueuedAt = queuedAt
	}
	p, err := row.proposal()
	if err != nil {
		item.Problem = "the stored proposal cannot be read: " + err.Error()
		return item, nil
	}
	if row.sessionStatus != string(memory.SessionActive) {
		item.Problem = inboxArchivedProblem
		return item, nil
	}
	stale, err := s.inboxProposalStale(ctx, p)
	if err != nil {
		return item, err
	}
	item.Change = s.inboxChange(ctx, p)
	if stale {
		refreshed, err := s.refreshInboxProposal(ctx, row, p)
		if err != nil {
			item.Problem = "it no longer applies to current memory: " + err.Error()
			return item, nil
		}
		item.Revision, item.Proposal, item.PreparedSHA256 = refreshed.revision, refreshed.proposalJSON, refreshed.preparedHash
		if fresh, err := refreshed.proposal(); err == nil {
			item.Change = s.inboxChange(ctx, fresh)
		}
	}
	item.Approvable = true
	return item, nil
}

// inboxChange summarizes what approving does, in the Claim text the standing
// tier and overview use: the existing memory a correction or retirement
// changes, a correction's replacement, and the owner's words it cites.
func (s *Store) inboxChange(ctx context.Context, p inboxProposal) *memory.MemoryProposalChange {
	switch {
	case p.literal != nil:
		return &memory.MemoryProposalChange{Action: memory.MemoryChangeAdd, Evidence: p.literal.Source.Evidence,
			EvieProposed: p.literal.Source.Authority == memory.AuthorityEvieProposed}
	case p.entity != nil:
		return &memory.MemoryProposalChange{Action: memory.MemoryChangeAdd, Evidence: p.entity.Source.Evidence,
			EvieProposed: p.entity.Source.Authority == memory.AuthorityEvieProposed}
	case p.correction != nil:
		change := &memory.MemoryProposalChange{Action: memory.MemoryChangeUpdate, Current: s.claimDisplayText(ctx, p.correction.OldClaim.ID),
			Evidence: p.correction.Source.Evidence, EvieProposed: p.correction.Source.Authority == memory.AuthorityEvieProposed}
		replacement := p.correction.ReplacementClaim
		subject := s.entityDisplayName(ctx, replacement.SubjectEntityID)
		kind, object := "literal", ""
		if replacement.Object.Literal != nil {
			object = replacement.Object.Literal.Value
		} else {
			kind, object = "entity", s.entityDisplayName(ctx, replacement.Object.EntityID)
		}
		change.Replacement = standingClaimText(subject, replacement.Predicate.Label, kind, object, object, replacement.Polarity)
		return change
	default:
		return &memory.MemoryProposalChange{Action: memory.MemoryChangeRetire, Current: s.claimDisplayText(ctx, p.lifecycle.ObjectID),
			Evidence: p.lifecycle.Evidence.Evidence}
	}
}

func (s *Store) entityDisplayName(ctx context.Context, id memory.SemanticID) string {
	var name string
	if err := s.db.QueryRowContext(ctx, `SELECT canonical_name FROM semantic_entities WHERE entity_id = ?`, id).Scan(&name); err != nil {
		return string(id)
	}
	return name
}

// claimDisplayText is a Claim's own text (subject — predicate: value), never
// source text; empty when the Claim cannot be read.
func (s *Store) claimDisplayText(ctx context.Context, id memory.SemanticID) string {
	var subject, label, objectKind, literal, objectName string
	var polarity memory.ClaimPolarity
	err := s.db.QueryRowContext(ctx, `
		SELECT subject.canonical_name, predicates.label, claims.object_kind, COALESCE(claims.literal_value, ''),
		       COALESCE(object.canonical_name, ''), claims.polarity
		FROM semantic_claims AS claims
		JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
		JOIN semantic_entities AS subject ON subject.entity_id = claims.subject_entity_id
		LEFT JOIN semantic_entities AS object ON object.entity_id = claims.object_entity_id
		WHERE claims.claim_id = ?`, id).Scan(&subject, &label, &objectKind, &literal, &objectName, &polarity)
	if err != nil {
		return ""
	}
	return standingClaimText(subject, label, objectKind, literal, objectName, polarity)
}

// inboxProposalStale reports whether any scope the proposal was prepared
// against has since changed, so applying it would meet newer memory.
func (s *Store) inboxProposalStale(ctx context.Context, p inboxProposal) (bool, error) {
	for _, scope := range p.scopes() {
		var id string
		var revision int64
		err := s.db.QueryRowContext(ctx, `SELECT scope_id, revision FROM semantic_scopes WHERE scope_key = ?`, scope.Key).Scan(&id, &revision)
		if errors.Is(err, sql.ErrNoRows) {
			if scope.Revision != 0 {
				return true, nil
			}
			continue
		}
		if err != nil {
			return false, err
		}
		if id != string(scope.ID) || revision != scope.Revision {
			return true, nil
		}
	}
	return false, nil
}

// refreshInboxProposal prepares the proposal again from its exact request in
// its own conversation's scope and stores the result as the proposal's next
// revision. It is revalidation, not a rewrite: the request, source message,
// kind and budget ratio are the ones Evie proposed.
func (s *Store) refreshInboxProposal(ctx context.Context, row inboxRow, p inboxProposal) (inboxRow, error) {
	session, err := s.GetSession(ctx, memory.SessionID(row.sessionID))
	if err != nil {
		return row, err
	}
	var fresh inboxProposal
	switch {
	case p.literal != nil:
		literal, err := s.PrepareRememberLiteral(ctx, session.ScopeContext(), p.literal.Request)
		if err != nil {
			return row, err
		}
		fresh.literal = &literal
	case p.entity != nil:
		entity, err := s.PrepareRememberEntity(ctx, session.ScopeContext(), p.entity.Request)
		if err != nil {
			return row, err
		}
		fresh.entity = &entity
	case p.correction != nil:
		correction, err := s.PrepareCorrectClaim(ctx, session.ScopeContext(), p.correction.Request)
		if err != nil {
			return row, err
		}
		fresh.correction = &correction
	default:
		lifecycle, err := s.PrepareMemoryLifecycle(ctx, session.ScopeContext(), p.lifecycle.Request)
		if err != nil {
			return row, err
		}
		fresh.lifecycle = &lifecycle
	}
	encoded, err := encodeInboxProposal(fresh)
	if err != nil {
		return row, err
	}
	proposalHash, preparedHash := fresh.hashes()
	alternativeProposal, alternativePrepared := alternativeColumns(fresh)
	refreshedAt := formatSemanticTime(s.now())
	var updated bool
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		result, err := conn.ExecContext(ctx, `
			UPDATE memory_proposal_inbox AS inbox
			SET proposal_json = ?, proposal_sha256 = ?, prepared_sha256 = ?, alternative_proposal_sha256 = ?,
			    alternative_prepared_sha256 = ?, scope_key = ?, revision = revision + 1, refreshed_at = ?
			WHERE inbox.proposal_id = ? AND inbox.revision = ? AND `+inboxPending,
			string(encoded), proposalHash, preparedHash, alternativeProposal, alternativePrepared, fresh.scopeKey(), refreshedAt, row.id, row.revision)
		if err != nil {
			return fmt.Errorf("refresh memory proposal: %w", err)
		}
		changed, err := result.RowsAffected()
		updated = changed == 1
		return err
	})
	if err != nil {
		return row, err
	}
	if !updated {
		// Another reader refreshed or resolved it first; report its state.
		current, err := s.loadInboxRow(ctx, row.id)
		if err != nil {
			return row, err
		}
		if current.accepted || current.declined {
			return current, ErrMemoryProposalResolved
		}
		return current, nil
	}
	return s.loadInboxRow(ctx, row.id)
}

// ApproveMemoryProposal applies the exact proposal the owner reviewed, with
// the "Always applies" switch as set. It holds the source conversation's turn
// lease, records the durable approval there, and applies through the
// ordinary Apply, which enforces scope, source binding and the standing
// budget. A proposal whose memory changed since it was shown is refreshed
// and refused; the owner reviews the refreshed proposal.
func (s *Store) ApproveMemoryProposal(ctx context.Context, approval memory.MemoryProposalApproval) (memory.MemoryProposalResult, error) {
	var result memory.MemoryProposalResult
	row, err := s.loadInboxRow(ctx, approval.ProposalID)
	if err != nil {
		return result, err
	}
	if row.accepted || row.declined {
		return result, ErrMemoryProposalResolved
	}
	if approval.PreparedSHA256 != row.preparedHash {
		return result, ErrMemoryProposalChanged
	}
	p, err := row.proposal()
	if err != nil {
		return result, fmt.Errorf("%w: %v", ErrMemoryProposalUnavailable, err)
	}
	if row.sessionStatus != string(memory.SessionActive) {
		return result, fmt.Errorf("%w: %s", ErrMemoryProposalUnavailable, inboxArchivedProblem)
	}
	chosen := p
	classification := p.classification()
	switch {
	case classification == nil && approval.AlwaysApplies:
		return result, errors.New("this memory has no kind, so it cannot always apply")
	case classification != nil && classification.Standing != approval.AlwaysApplies:
		if chosen, err = p.switched(); err != nil {
			return result, err
		}
	}

	holder := memory.LeaseHolderID("memory-inbox:" + uuid.NewString())
	lease, err := s.AcquireTurnLease(ctx, p.sessionID(), holder, memoryProposalLeaseDuration)
	switch {
	case errors.Is(err, ErrTurnLeaseHeld):
		return result, ErrMemoryProposalBusy
	case errors.Is(err, ErrTurnLeaseSessionInactive):
		return result, fmt.Errorf("%w: %s", ErrMemoryProposalUnavailable, inboxArchivedProblem)
	case err != nil:
		return result, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = s.ReleaseTurnLease(releaseCtx, lease.SessionID, lease.HolderID, lease.FencingToken)
	}()
	changed := func() error {
		if _, err := s.refreshInboxProposal(ctx, row, p); err != nil && !errors.Is(err, ErrMemoryProposalResolved) {
			return fmt.Errorf("%w (%v)", ErrMemoryProposalChanged, err)
		}
		return ErrMemoryProposalChanged
	}
	if stale, err := s.inboxProposalStale(ctx, p); err != nil {
		return result, err
	} else if stale {
		return result, changed()
	}

	proposalHash, preparedHash := chosen.hashes()
	payload, err := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: proposalHash, PreparedSHA256: preparedHash})
	if err != nil {
		return result, err
	}
	if _, err := s.AppendEventWithLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID: p.sourceEventID(), Type: memory.EventApproval, ExecutionID: memory.ExecutionID(p.operationID()), Payload: payload,
	}); err != nil {
		return result, fmt.Errorf("record inbox approval: %w", err)
	}
	switch {
	case chosen.literal != nil:
		_, err = s.ApplyRememberLiteral(ctx, lease, *chosen.literal)
	case chosen.entity != nil:
		_, err = s.ApplyRememberEntity(ctx, lease, *chosen.entity)
	case chosen.correction != nil:
		_, err = s.ApplyCorrectClaim(ctx, lease, *chosen.correction)
	default:
		_, err = s.ApplyMemoryLifecycle(ctx, lease, *chosen.lifecycle)
	}
	if err != nil {
		if stale, staleErr := s.inboxProposalStale(ctx, p); staleErr == nil && stale && !errors.Is(err, memory.ErrStandingMemoryFull) {
			return result, changed()
		}
		return result, err
	}
	standing := false
	if applied := chosen.classification(); applied != nil {
		standing = applied.Standing
	}
	return memory.MemoryProposalResult{ProposalID: row.id, OperationID: p.operationID(), ClaimID: p.claimID(), Standing: standing,
		ProposalSHA256: proposalHash, PreparedSHA256: preparedHash}, nil
}

// DeclineMemoryProposal resolves a pending proposal without changing memory.
// Declining twice is not an error.
func (s *Store) DeclineMemoryProposal(ctx context.Context, id string) error {
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		row, err := scanInboxRow(conn.QueryRowContext(ctx, `SELECT `+inboxRowColumns+`
			FROM memory_proposal_inbox AS inbox JOIN sessions ON sessions.id = inbox.session_id WHERE inbox.proposal_id = ?`, id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMemoryProposalNotFound
		}
		if err != nil {
			return err
		}
		if row.declined {
			return nil
		}
		if row.accepted {
			return ErrMemoryProposalResolved
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_proposal_inbox_declines (proposal_id, declined_at) VALUES (?, ?)`,
			id, formatSemanticTime(s.now())); err != nil {
			return fmt.Errorf("decline memory proposal: %w", err)
		}
		return nil
	})
}
