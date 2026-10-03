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

// The memory proposal inbox (docs/memory-architecture.md, Stage 2). A
// remember proposal the owner did not explicitly ask for is stored here
// exactly as prepared, with its "Always applies" alternative, instead of
// interrupting the conversation with an approval card. It changes no memory
// until the owner approves it from the inbox, which applies it through the
// same path as an approval card: a durable approval event in the source
// conversation, then the ordinary Apply under that conversation's turn
// lease. A proposal is pending until it is declined or an operation with its
// idempotency key is accepted; nothing else marks it resolved.
const memoryProposalInboxSchema = `
CREATE TABLE IF NOT EXISTS memory_proposal_inbox (
    proposal_id                 TEXT PRIMARY KEY,
    session_id                  TEXT NOT NULL REFERENCES sessions(id),
    scope_key                   TEXT NOT NULL,
    tool_name                   TEXT NOT NULL CHECK (tool_name IN ('memory_remember_literal', 'memory_remember_entity')),
    reason                      TEXT NOT NULL CHECK (reason IN ('not_requested', 'no_explicit_cue')),
    idempotency_key             TEXT NOT NULL UNIQUE,
    source_event_id             TEXT NOT NULL,
    intent_event_id             TEXT NOT NULL,
    dedupe_key                  TEXT NOT NULL,
    proposal_json               TEXT NOT NULL,
    proposal_sha256             TEXT NOT NULL,
    prepared_sha256             TEXT NOT NULL,
    alternative_proposal_sha256 TEXT,
    alternative_prepared_sha256 TEXT,
    revision                    INTEGER NOT NULL CHECK (revision > 0),
    queued_at                   TEXT NOT NULL,
    refreshed_at                TEXT
);
CREATE INDEX IF NOT EXISTS memory_proposal_inbox_dedupe_idx ON memory_proposal_inbox(dedupe_key);
CREATE INDEX IF NOT EXISTS memory_proposal_inbox_scope_idx ON memory_proposal_inbox(scope_key);
CREATE TABLE IF NOT EXISTS memory_proposal_inbox_declines (
    proposal_id TEXT PRIMARY KEY REFERENCES memory_proposal_inbox(proposal_id),
    declined_at TEXT NOT NULL
);
CREATE TRIGGER IF NOT EXISTS memory_proposal_inbox_declines_append_only_update BEFORE UPDATE ON memory_proposal_inbox_declines BEGIN SELECT RAISE(ABORT, 'memory proposal declines are append-only'); END;
CREATE TRIGGER IF NOT EXISTS memory_proposal_inbox_declines_append_only_delete BEFORE DELETE ON memory_proposal_inbox_declines BEGIN SELECT RAISE(ABORT, 'memory proposal declines are append-only'); END;
`

func ensureMemoryProposalInbox(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, memoryProposalInboxSchema); err != nil {
		return fmt.Errorf("create memory proposal inbox: %w", err)
	}
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
	memoryProposalListLimit     = 50
	memoryProposalLeaseDuration = 30 * time.Second
	// inboxPending is the pending condition for the alias inbox.
	inboxPending = `NOT EXISTS (SELECT 1 FROM memory_proposal_inbox_declines AS declines WHERE declines.proposal_id = inbox.proposal_id)
	  AND NOT EXISTS (SELECT 1 FROM semantic_operations AS operations WHERE operations.idempotency_key = inbox.idempotency_key)`
)

// inboxProposal is one exact prepared remember proposal of either shape.
type inboxProposal struct {
	literal *memory.RememberLiteralProposal
	entity  *memory.RememberEntityProposal
}

func inboxProposalOf(toolName string, literal *memory.RememberLiteralProposal, entity *memory.RememberEntityProposal) (inboxProposal, error) {
	switch {
	case toolName == memory.RememberLiteralToolName && literal != nil && entity == nil:
		return inboxProposal{literal: literal}, nil
	case toolName == memory.RememberEntityToolName && entity != nil && literal == nil:
		return inboxProposal{entity: entity}, nil
	default:
		return inboxProposal{}, errors.New("an inbox proposal is exactly one remember proposal of its tool")
	}
}

func (p inboxProposal) toolName() string {
	if p.literal != nil {
		return memory.RememberLiteralToolName
	}
	return memory.RememberEntityToolName
}

func (p inboxProposal) value() any {
	if p.literal != nil {
		return *p.literal
	}
	return *p.entity
}

func (p inboxProposal) sessionID() memory.SessionID {
	if p.literal != nil {
		return p.literal.SessionID
	}
	return p.entity.SessionID
}

func (p inboxProposal) operationID() memory.SemanticID {
	if p.literal != nil {
		return p.literal.OperationID
	}
	return p.entity.OperationID
}

func (p inboxProposal) idempotencyKey() string {
	if p.literal != nil {
		return p.literal.IdempotencyKey
	}
	return p.entity.IdempotencyKey
}

func (p inboxProposal) sourceEventID() memory.EventID {
	if p.literal != nil {
		return p.literal.Source.EventID
	}
	return p.entity.Source.EventID
}

func (p inboxProposal) scopeKey() string {
	if p.literal != nil {
		return p.literal.Scope.Key
	}
	return p.entity.Scope.Key
}

func (p inboxProposal) scopes() []memory.SemanticScope {
	if p.literal != nil {
		return p.literal.Scopes
	}
	return p.entity.Scopes
}

func (p inboxProposal) claimID() memory.SemanticID {
	if p.literal != nil {
		return p.literal.ClaimID
	}
	return p.entity.Claim.ID
}

func (p inboxProposal) classification() *memory.ClaimClassification {
	if p.literal != nil {
		return p.literal.Classification
	}
	return p.entity.Classification
}

func (p inboxProposal) hashes() (string, string) {
	if p.literal != nil {
		return p.literal.ProposalSHA256, p.literal.PreparedSHA256
	}
	return p.entity.ProposalSHA256, p.entity.PreparedSHA256
}

func (p inboxProposal) alternative() *memory.ProposalHashes {
	if p.literal != nil {
		return p.literal.StandingAlternative
	}
	return p.entity.StandingAlternative
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
	if p.literal != nil {
		literal := *p.literal
		literal.Classification, literal.StandingAlternative = &flipped, nil
		literal.ProposalSHA256, literal.PreparedSHA256 = alternative.ProposalSHA256, alternative.PreparedSHA256
		return inboxProposal{literal: &literal}, nil
	}
	entity := *p.entity
	entity.Classification, entity.StandingAlternative = &flipped, nil
	entity.ProposalSHA256, entity.PreparedSHA256 = alternative.ProposalSHA256, alternative.PreparedSHA256
	return inboxProposal{entity: &entity}, nil
}

// dedupeKey identifies the memory a proposal states, independent of the
// identifiers each preparation mints, so a repeated proposal is not queued
// twice.
func (p inboxProposal) dedupeKey() string {
	selector := func(selected memory.EntitySelector) string {
		if selected.EntityID != "" {
			return "id:" + string(selected.EntityID)
		}
		return "name:" + strings.ToLower(strings.TrimSpace(selected.CanonicalName)) + "|" + strings.ToLower(strings.TrimSpace(selected.EntityType))
	}
	var key any
	if p.literal != nil {
		key = []any{"literal", p.literal.Scope.Key, p.literal.Predicate.Token, p.literal.Literal, p.literal.Polarity, p.literal.ValidTime}
	} else {
		key = []any{"entity", p.entity.Scope.Key, p.entity.Predicate.Token, selector(p.entity.Request.Subject), selector(p.entity.Request.Object),
			p.entity.Claim.Polarity, p.entity.Claim.ValidTime}
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
		return nil, errors.New("this memory may contain a secret, so it was not queued; never propose secrets as memories")
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

func alternativeColumns(p inboxProposal) (any, any) {
	if alternative := p.alternative(); alternative != nil {
		return alternative.ProposalSHA256, alternative.PreparedSHA256
	}
	return nil, nil
}

// QueueMemoryProposal stores one exact prepared remember proposal in the
// owner's review inbox, fenced by the proposing turn's lease. It changes no
// memory. An identical pending memory is not queued twice; the existing
// proposal is reported as a duplicate.
func (s *Store) QueueMemoryProposal(ctx context.Context, lease memory.TurnLease, queued memory.QueuedMemoryProposal) (memory.MemoryProposalQueued, error) {
	var result memory.MemoryProposalQueued
	p, err := inboxProposalOf(queued.ToolName, queued.Literal, queued.Entity)
	if err != nil {
		return result, err
	}
	if !queued.Reason.Valid() {
		return result, fmt.Errorf("unsupported inbox reason %q", queued.Reason)
	}
	if lease.SessionID != p.sessionID() {
		return result, errors.New("inbox proposal does not match its turn lease")
	}
	encoded, err := encodeInboxProposal(p)
	if err != nil {
		return result, err
	}
	proposalHash, preparedHash := p.hashes()
	alternativeProposal, alternativePrepared := alternativeColumns(p)
	id, err := uuid.NewRandom()
	if err != nil {
		return result, err
	}
	dedupe := p.dedupeKey()
	queuedAt := formatSemanticTime(s.now())
	err = s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		var intentType string
		if err := writer.queryRowContext(ctx, `SELECT event_type FROM events WHERE id = ? AND session_id = ?`, queued.IntentEventID, lease.SessionID).Scan(&intentType); err != nil ||
			intentType != string(memory.EventToolIntent) {
			return errors.New("inbox proposal must cite its tool intent in the proposing conversation")
		}
		var accepted int
		if err := writer.queryRowContext(ctx, `SELECT COUNT(*) FROM semantic_operations WHERE idempotency_key = ?`, p.idempotencyKey()).Scan(&accepted); err != nil {
			return err
		}
		if accepted != 0 {
			return fmt.Errorf("%w: this idempotency key already saved a memory", ErrIdempotencyConflict)
		}
		var existingID, existingDedupe string
		err := writer.queryRowContext(ctx, `SELECT proposal_id, dedupe_key FROM memory_proposal_inbox WHERE idempotency_key = ?`, p.idempotencyKey()).Scan(&existingID, &existingDedupe)
		if err == nil {
			if existingDedupe != dedupe {
				return ErrIdempotencyConflict
			}
			result = memory.MemoryProposalQueued{ProposalID: existingID, Duplicate: true}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		err = writer.queryRowContext(ctx, `SELECT inbox.proposal_id FROM memory_proposal_inbox AS inbox
			WHERE inbox.dedupe_key = ? AND `+inboxPending+` ORDER BY inbox.queued_at, inbox.proposal_id LIMIT 1`, dedupe).Scan(&existingID)
		if err == nil {
			result = memory.MemoryProposalQueued{ProposalID: existingID, Duplicate: true}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := writer.execContext(ctx, `
			INSERT INTO memory_proposal_inbox (
				proposal_id, session_id, scope_key, tool_name, reason, idempotency_key, source_event_id, intent_event_id,
				dedupe_key, proposal_json, proposal_sha256, prepared_sha256, alternative_proposal_sha256,
				alternative_prepared_sha256, revision, queued_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
		`, id.String(), p.sessionID(), p.scopeKey(), p.toolName(), queued.Reason, p.idempotencyKey(), p.sourceEventID(), queued.IntentEventID,
			dedupe, string(encoded), proposalHash, preparedHash, alternativeProposal, alternativePrepared, queuedAt); err != nil {
			return fmt.Errorf("queue memory proposal: %w", err)
		}
		result = memory.MemoryProposalQueued{ProposalID: id.String()}
		return nil
	})
	return result, err
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
	if stale {
		refreshed, err := s.refreshInboxProposal(ctx, row, p)
		if err != nil {
			item.Problem = "it no longer applies to current memory: " + err.Error()
			return item, nil
		}
		item.Revision, item.Proposal, item.PreparedSHA256 = refreshed.revision, refreshed.proposalJSON, refreshed.preparedHash
	}
	item.Approvable = true
	return item, nil
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
	if p.literal != nil {
		literal, err := s.PrepareRememberLiteral(ctx, session.ScopeContext(), p.literal.Request)
		if err != nil {
			return row, err
		}
		fresh.literal = &literal
	} else {
		entity, err := s.PrepareRememberEntity(ctx, session.ScopeContext(), p.entity.Request)
		if err != nil {
			return row, err
		}
		fresh.entity = &entity
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
	if p.literal != nil {
		_, err = s.ApplyRememberLiteral(ctx, lease, *chosen.literal)
	} else {
		_, err = s.ApplyRememberEntity(ctx, lease, *chosen.entity)
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
