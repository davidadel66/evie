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

// The always-on background pass (docs/memory-architecture.md, Stage 4). Its
// durable state is three small tables: one activation row (also the
// cross-process runner lease), one cursor per root conversation (the last
// event sequence whose owner messages a pass covered), and one row per pass
// (outcome, counts and spend). Proposals land in memory_proposal_inbox.
// Nothing here is memory: a cursor or run never becomes evidence, and
// generic SQL and file tools cannot read these tables.
const memoryBackgroundSchema = `
CREATE TABLE IF NOT EXISTS memory_background_state (
    singleton         INTEGER PRIMARY KEY CHECK (singleton = 1),
    activated_at      TEXT NOT NULL,
    runner_holder     TEXT,
    runner_expires_at TEXT
);
CREATE TABLE IF NOT EXISTS memory_background_cursors (
    session_id      TEXT PRIMARY KEY REFERENCES sessions(id),
    cursor_sequence INTEGER NOT NULL CHECK (cursor_sequence >= 0),
    failures        INTEGER NOT NULL DEFAULT 0 CHECK (failures >= 0),
    malformed       INTEGER NOT NULL DEFAULT 0 CHECK (malformed >= 0),
    next_attempt_at TEXT,
    updated_at      TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS memory_background_runs (
    run_id           TEXT PRIMARY KEY,
    session_id       TEXT NOT NULL REFERENCES sessions(id),
    trigger          TEXT NOT NULL CHECK (trigger IN ('idle', 'turns', 'compaction', 'manual')),
    status           TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'skipped', 'deferred', 'superseded')),
    from_sequence    INTEGER NOT NULL CHECK (from_sequence >= 0),
    through_sequence INTEGER NOT NULL CHECK (through_sequence >= from_sequence),
    model            TEXT NOT NULL,
    input_tokens     INTEGER NOT NULL CHECK (input_tokens >= 0),
    output_tokens    INTEGER NOT NULL CHECK (output_tokens >= 0),
    cost_micro_usd   INTEGER NOT NULL CHECK (cost_micro_usd >= 0),
    messages         INTEGER NOT NULL CHECK (messages >= 0),
    candidates       INTEGER NOT NULL CHECK (candidates >= 0),
    queued           INTEGER NOT NULL CHECK (queued >= 0),
    duplicates       INTEGER NOT NULL CHECK (duplicates >= 0),
    noops            INTEGER NOT NULL CHECK (noops >= 0),
    skipped          INTEGER NOT NULL CHECK (skipped >= 0),
    malformed        INTEGER NOT NULL CHECK (malformed IN (0, 1)),
    error            TEXT NOT NULL,
    started_at       TEXT NOT NULL,
    finished_at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS memory_background_runs_finished_idx ON memory_background_runs(finished_at);
CREATE INDEX IF NOT EXISTS memory_background_runs_session_idx ON memory_background_runs(session_id, finished_at);
`

func ensureMemoryBackgroundSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, memoryBackgroundSchema); err != nil {
		return fmt.Errorf("create memory background schema: %w", err)
	}
	return nil
}

const (
	// memoryBackgroundPoolLimit bounds the current memories one pass compares
	// its candidates with.
	memoryBackgroundPoolLimit = 400
	// memoryBackgroundPredicateLimit bounds the Predicate vocabulary shown
	// to the extractor.
	memoryBackgroundPredicateLimit = 60
	memoryBackgroundErrorBytes     = 300
)

// EnsureMemoryBackgroundActivation records when the background pass was
// first enabled and captures its frontier: every root conversation that
// exists now starts its cursor at its latest event, so only owner messages
// sent after activation are read. Conversations created later start at the
// beginning. Activation happens once; turning the pass off and on later
// resumes each cursor where it stopped. Historical backfill is a separate
// explicit choice, never implied.
func (s *Store) EnsureMemoryBackgroundActivation(ctx context.Context) (time.Time, error) {
	var activated time.Time
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		var text string
		err := conn.QueryRowContext(ctx, `SELECT activated_at FROM memory_background_state WHERE singleton = 1`).Scan(&text)
		if err == nil {
			activated, err = parseSemanticTime(text)
			return err
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		activated = s.now().UTC()
		at := formatSemanticTime(activated)
		if _, err := conn.ExecContext(ctx, `INSERT INTO memory_background_state (singleton, activated_at) VALUES (1, ?)`, at); err != nil {
			return err
		}
		_, err = conn.ExecContext(ctx, `
			INSERT INTO memory_background_cursors (session_id, cursor_sequence, updated_at)
			SELECT sessions.id, COALESCE((SELECT MAX(events.sequence) FROM events WHERE events.session_id = sessions.id), 0), ?
			FROM sessions
			WHERE sessions.parent_session_id IS NULL
			  AND NOT EXISTS (SELECT 1 FROM memory_background_cursors AS cursors WHERE cursors.session_id = sessions.id)
		`, at)
		return err
	})
	return activated, err
}

// AcquireMemoryBackgroundRunner takes or renews the single cross-process
// runner lease, so cooperating processes run at most one pass at a time.
// It reports false while another holder's lease is unexpired.
func (s *Store) AcquireMemoryBackgroundRunner(ctx context.Context, holder string, ttl time.Duration) (bool, error) {
	if holder == "" || ttl <= 0 {
		return false, errors.New("background runner lease requires a holder and a duration")
	}
	acquired := false
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		now := s.now().UTC()
		result, err := conn.ExecContext(ctx, `
			UPDATE memory_background_state SET runner_holder = ?, runner_expires_at = ?
			WHERE singleton = 1 AND (runner_holder IS NULL OR runner_holder = ? OR runner_expires_at <= ?)
		`, holder, formatSemanticTime(now.Add(ttl)), holder, formatSemanticTime(now))
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		acquired = changed == 1
		return err
	})
	return acquired, err
}

// ReleaseMemoryBackgroundRunner gives up the runner lease if still held.
func (s *Store) ReleaseMemoryBackgroundRunner(ctx context.Context, holder string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE memory_background_state SET runner_holder = NULL, runner_expires_at = NULL
		WHERE singleton = 1 AND runner_holder = ?`, holder)
	return err
}

// ownerMessageCondition selects owner messages: the only events a pass reads.
const ownerMessageCondition = `events.event_type = 'user_message' AND events.role = 'user'`

// compactedThroughSQL is the last event a committed compaction of the
// conversation summarized (memory architecture Stage 10): whole-turn and
// in-turn compactions both record it as covered_last_sequence. Only
// compactions after the cursor are read, since only they can cover an owner
// message the pass has not. Zero when there is none. session and cursor are
// SQL expressions for the conversation ID and its cursor.
func compactedThroughSQL(session, cursor string) string {
	return `COALESCE((SELECT MAX(CAST(json_extract(compactions.payload_json, '$.covered_last_sequence') AS INTEGER))
		FROM events AS compactions
		WHERE compactions.session_id = ` + session + `
		  AND compactions.event_type IN ('` + string(memory.EventContextCompacted) + `', '` + string(memory.EventContextTurnCompacted) + `')
		  AND compactions.sequence > ` + cursor + `), 0)`
}

// compactedPendingSQL counts the owner messages after cursor up to through:
// the memory flush a committed compaction still owes.
func compactedPendingSQL(session, cursor, through string) string {
	return `(SELECT COUNT(*) FROM events WHERE events.session_id = ` + session + ` AND ` + ownerMessageCondition + `
		AND events.sequence > ` + cursor + ` AND events.sequence <= ` + through + `)`
}

// DueMemoryBackgroundSessions lists root conversations whose pass is due:
// owner messages past the cursor that a committed compaction summarized
// (the compaction flush, Stage 10, regardless of the other triggers), at
// least Turns new owner messages since the cursor, or at least one and no
// activity for Idle. A conversation whose turn lease is held by a live turn,
// that is archived or in an archived Workspace, delegated, or waiting out a
// failure backoff is not due; its cursor holds until it is restored.
func (s *Store) DueMemoryBackgroundSessions(ctx context.Context, q memory.BackgroundDueQuery) ([]memory.BackgroundDue, error) {
	if q.Turns < 1 || q.Idle <= 0 || q.Limit < 1 {
		return nil, errors.New("background due query needs positive triggers and limit")
	}
	now := q.Now.UTC()
	cursor := `COALESCE(cursors.cursor_sequence, 0)`
	rows, err := s.db.QueryContext(ctx, `
		SELECT due.id, due.cursor_sequence, due.pending, due.last_recorded_at, due.compacted_through,
		       `+compactedPendingSQL("due.id", "due.cursor_sequence", "due.compacted_through")+`
		FROM (
			SELECT sessions.id AS id, `+cursor+` AS cursor_sequence,
			       (SELECT COUNT(*) FROM events WHERE events.session_id = sessions.id AND `+ownerMessageCondition+`
			          AND events.sequence > `+cursor+`) AS pending,
			       (SELECT events.recorded_at FROM events WHERE events.session_id = sessions.id ORDER BY events.sequence DESC LIMIT 1) AS last_recorded_at,
			       `+compactedThroughSQL("sessions.id", cursor)+` AS compacted_through
			FROM sessions
			LEFT JOIN memory_background_cursors AS cursors ON cursors.session_id = sessions.id
			WHERE sessions.parent_session_id IS NULL AND sessions.status = ?
			  AND NOT EXISTS (
			      SELECT 1 FROM workspaces
			      WHERE workspaces.id = sessions.workspace_id AND workspaces.lifecycle_state = ?)
			  AND (cursors.next_attempt_at IS NULL OR cursors.next_attempt_at <= ?)
			  AND NOT EXISTS (
			      SELECT 1 FROM session_turn_leases AS leases
			      WHERE leases.session_id = sessions.id AND leases.holder_id IS NOT NULL AND leases.expires_at > ?)
			  AND EXISTS (
			      SELECT 1 FROM events WHERE events.session_id = sessions.id AND `+ownerMessageCondition+`
			        AND events.sequence > `+cursor+`)
		) AS due
		ORDER BY due.id
	`, memory.SessionActive, memory.WorkspaceArchived, formatSemanticTime(now), now.Format(turnLeaseTimeFormat))
	if err != nil {
		return nil, fmt.Errorf("list due background passes: %w", err)
	}
	defer rows.Close()
	var due []memory.BackgroundDue
	for rows.Next() {
		var item memory.BackgroundDue
		var last sql.NullString
		var through int64
		if err := rows.Scan(&item.SessionID, &item.Cursor, &item.PendingOwner, &last, &through, &item.CompactedPending); err != nil {
			return nil, err
		}
		if last.Valid {
			if at, err := time.Parse(time.RFC3339Nano, last.String); err == nil {
				item.LastActivityAt = at
			}
		}
		switch {
		case item.CompactedPending > 0:
			item.Trigger, item.Through = memory.BackgroundTriggerCompaction, through
		case item.PendingOwner >= q.Turns:
			item.Trigger = memory.BackgroundTriggerTurns
		case item.PendingOwner > 0 && !item.LastActivityAt.IsZero() && !now.Before(item.LastActivityAt.Add(q.Idle)):
			item.Trigger = memory.BackgroundTriggerIdle
		default:
			continue
		}
		due = append(due, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Owed compaction flushes go first, since their owner messages already
	// left the model's context; then the conversations waiting longest.
	// Each tick serves a few.
	first := func(a, b memory.BackgroundDue) bool {
		aFlush, bFlush := a.Trigger == memory.BackgroundTriggerCompaction, b.Trigger == memory.BackgroundTriggerCompaction
		if aFlush != bFlush {
			return aFlush
		}
		return a.LastActivityAt.Before(b.LastActivityAt)
	}
	for i := 1; i < len(due); i++ {
		for j := i; j > 0 && first(due[j], due[j-1]); j-- {
			due[j], due[j-1] = due[j-1], due[j]
		}
	}
	if len(due) > q.Limit {
		due = due[:q.Limit]
	}
	return due, nil
}

// MemoryBackgroundBatch reads one bounded batch of owner messages new since
// the conversation's cursor. Only owner user messages of a root conversation
// are read: assistant, tool, web and compiler text never are. A message with
// a detected secret, and a local slash command, is covered but not returned.
// The batch stops at the first message past its bounds; a single message
// longer than MaxBytes is returned alone so no message is ever skipped for
// size (the pass truncates what it sends visibly).
func (s *Store) MemoryBackgroundBatch(ctx context.Context, sessionID memory.SessionID, limits memory.BackgroundBatchLimits) (memory.BackgroundBatch, error) {
	var batch memory.BackgroundBatch
	if limits.MaxMessages < 1 || limits.MaxBytes < 1 {
		return batch, errors.New("background batch needs positive bounds")
	}
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return batch, err
	}
	if session.ParentSessionID != "" {
		return batch, errors.New("the background pass reads only root conversations")
	}
	if session.Status != memory.SessionActive {
		return batch, errors.New("the conversation is archived")
	}
	batch.Session = session
	err = s.db.QueryRowContext(ctx, `SELECT cursor_sequence, failures, malformed FROM memory_background_cursors WHERE session_id = ?`, sessionID).
		Scan(&batch.FromSequence, &batch.Failures, &batch.Malformed)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return batch, err
	}
	batch.ThroughSequence = batch.FromSequence
	query := `SELECT id, sequence, content, recorded_at FROM events
		WHERE session_id = ? AND ` + ownerMessageCondition + ` AND sequence > ?`
	args := []any{sessionID, batch.FromSequence}
	if limits.Through > 0 {
		query += ` AND sequence <= ?`
		args = append(args, limits.Through)
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY sequence`, args...)
	if err != nil {
		return batch, fmt.Errorf("read owner messages: %w", err)
	}
	defer rows.Close()
	bytes := 0
	for rows.Next() {
		var message memory.BackgroundOwnerMessage
		var recorded string
		if err := rows.Scan(&message.EventID, &message.Sequence, &message.Content, &recorded); err != nil {
			return batch, err
		}
		if message.RecordedAt, err = time.Parse(time.RFC3339Nano, recorded); err != nil {
			return batch, fmt.Errorf("parse owner message time: %w", err)
		}
		sendable := !backgroundCommand(message.Content) && !memory.HasRetrievalSecret([]byte(message.Content)) && strings.TrimSpace(message.Content) != ""
		if sendable && (len(batch.Messages) == limits.MaxMessages || len(batch.Messages) > 0 && bytes+len(message.Content) > limits.MaxBytes) {
			batch.More = true
			break
		}
		batch.ThroughSequence = message.Sequence
		switch {
		case backgroundCommand(message.Content):
			batch.Commands++
		case !sendable:
			batch.Withheld++
		default:
			bytes += len(message.Content)
			batch.Messages = append(batch.Messages, message)
		}
	}
	if err := rows.Err(); err != nil {
		return batch, err
	}
	return batch, nil
}

// backgroundCommand reports a local slash command ("/remember …",
// "/compact"): an instruction to Evie's harness, not a statement to learn
// from. A path such as "/Users/…" in prose is not one.
func backgroundCommand(content string) bool {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") || len(trimmed) < 2 {
		return false
	}
	first := strings.Fields(trimmed)[0]
	for _, r := range first[1:] {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// backgroundPoolClaims selects current Claims in the given scopes: active,
// valid now, with an eligible source, outside quarantined scopes. Standing
// Claims are included: a candidate is compared with everything the owner
// already has. Arguments: the scope keys, then now twice.
const backgroundPoolClaims = `
	FROM semantic_claims AS claims
	JOIN semantic_scopes AS scopes ON scopes.scope_id = claims.scope_id
	JOIN semantic_predicates AS predicates ON predicates.predicate_id = claims.predicate_id
	JOIN semantic_entities AS subject ON subject.entity_id = claims.subject_entity_id
	LEFT JOIN semantic_entities AS object ON object.entity_id = claims.object_entity_id
	LEFT JOIN semantic_claim_classifications AS classes ON classes.claim_id = claims.claim_id
	     AND NOT EXISTS (
	         SELECT 1 FROM semantic_claim_classifications AS newer
	         WHERE newer.claim_id = classes.claim_id AND newer.scope_revision > classes.scope_revision)
	WHERE scopes.scope_key IN (%s)
	  AND scopes.scope_id NOT IN (SELECT scope_id FROM semantic_projection_quarantine)
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
	  AND (claims.valid_to IS NULL OR claims.valid_to > ?)`

// MemoryBackgroundPool returns what a pass compares its candidates with:
// the current memories the conversation can read (Global, its Workspace or
// project, and itself), newest first and bounded, and the pending inbox
// remembers in those scopes, plus the Predicate vocabulary in use. Claim
// text is the Claim's own text, never source text; items the secret fence
// flags are left out.
func (s *Store) MemoryBackgroundPool(ctx context.Context, scope memory.ScopeContext) (memory.BackgroundPool, error) {
	pool := memory.BackgroundPool{ContextScopeKey: scopeKeyForContext(scope), Memories: []memory.BackgroundExistingMemory{}, Predicates: []memory.BackgroundPredicate{}}
	if err := validateSessionScope(ctx, s.db, scope); err != nil {
		return pool, err
	}
	keys := []string{"global"}
	if pool.ContextScopeKey != "global" {
		keys = append(keys, pool.ContextScopeKey)
	}
	keys = append(keys, "session:"+string(scope.SessionID))
	at := formatSemanticTime(s.now().UTC())
	args := make([]any, 0, len(keys)+3)
	for _, key := range keys {
		args = append(args, key)
	}
	args = append(args, at, at, memoryBackgroundPoolLimit+1)
	rows, err := s.db.QueryContext(ctx, `
		SELECT claims.claim_id, scopes.scope_key, COALESCE(classes.kind, ''), COALESCE(classes.standing, 0),
		       subject.canonical_name, claims.subject_entity_id, claims.predicate_id, predicates.token, predicates.label, claims.object_kind,
		       COALESCE(claims.literal_kind, ''), COALESCE(claims.literal_value, ''), COALESCE(object.canonical_name, ''), claims.polarity
		`+fmt.Sprintf(backgroundPoolClaims, strings.TrimSuffix(strings.Repeat("?,", len(keys)), ","))+`
		ORDER BY claims.transaction_time DESC, claims.claim_id LIMIT ?`, args...)
	if err != nil {
		return pool, fmt.Errorf("read background comparison pool: %w", err)
	}
	seenPredicate := map[string]bool{}
	for rows.Next() {
		var item memory.BackgroundExistingMemory
		var standing int
		var subject, objectKind, literalKind, literalValue, objectName string
		if err := rows.Scan(&item.ClaimID, &item.ScopeKey, &item.Kind, &standing, &subject, &item.SubjectEntityID, &item.PredicateID,
			&item.PredicateToken, &item.PredicateLabel, &objectKind, &literalKind, &literalValue, &objectName, &item.Polarity); err != nil {
			rows.Close()
			return pool, err
		}
		if len(pool.Memories) == memoryBackgroundPoolLimit {
			pool.Truncated = true
			break
		}
		if item.Kind == "" {
			item.Kind = memory.ClaimKindFact
		}
		item.Standing = standing == 1
		if objectKind != "entity" {
			item.Literal = &memory.TypedLiteral{Kind: memory.LiteralKind(literalKind), Value: literalValue}
		}
		item.Text = standingClaimText(subject, item.PredicateLabel, objectKind, literalValue, objectName, item.Polarity)
		// Corrections and retirements from this conversation may change only
		// its own Context Scope's Claims, as the approval-card tools may.
		item.Writable = item.ScopeKey == pool.ContextScopeKey
		if memory.HasRetrievalSecret([]byte(item.Text)) {
			continue
		}
		pool.Memories = append(pool.Memories, item)
		if !seenPredicate[item.PredicateToken] && len(pool.Predicates) < memoryBackgroundPredicateLimit {
			seenPredicate[item.PredicateToken] = true
			pool.Predicates = append(pool.Predicates, memory.BackgroundPredicate{Token: item.PredicateToken, Label: item.PredicateLabel})
		}
	}
	if err := rows.Close(); err != nil {
		return pool, err
	}
	if err := rows.Err(); err != nil {
		return pool, err
	}
	pending, err := s.db.QueryContext(ctx, `SELECT inbox.proposal_id, inbox.scope_key, inbox.proposal_json FROM memory_proposal_inbox AS inbox
		WHERE inbox.tool_name = ? AND inbox.scope_key IN (`+strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")+`) AND `+inboxPending+`
		ORDER BY inbox.queued_at DESC, inbox.proposal_id LIMIT 100`, append([]any{memory.RememberLiteralToolName}, args[:len(keys)]...)...)
	if err != nil {
		return pool, fmt.Errorf("read pending proposals: %w", err)
	}
	defer pending.Close()
	for pending.Next() {
		var id, scopeKey, proposalJSON string
		if err := pending.Scan(&id, &scopeKey, &proposalJSON); err != nil {
			return pool, err
		}
		var literal memory.RememberLiteralProposal
		if err := json.Unmarshal([]byte(proposalJSON), &literal); err != nil || literal.Subject.AnchorKind != "owner" {
			continue
		}
		value := literal.Literal
		item := memory.BackgroundExistingMemory{ProposalID: id, ScopeKey: scopeKey, PredicateToken: literal.Predicate.Token,
			PredicateLabel: literal.Predicate.Label, Literal: &value, Polarity: literal.Polarity, Kind: memory.ClaimKindFact, Pending: true,
			Text: standingClaimText("owner", literal.Predicate.Label, "literal", value.Value, "", literal.Polarity)}
		if literal.Classification != nil {
			item.Kind, item.Standing = literal.Classification.Kind, literal.Classification.Standing
		}
		if memory.HasRetrievalSecret([]byte(item.Text)) {
			continue
		}
		pool.Memories = append(pool.Memories, item)
	}
	return pool, pending.Err()
}

// CommitMemoryBackgroundPass queues a pass's exact prepared proposals and
// advances the conversation's cursor in one transaction fenced by the
// conversation's turn lease, so a pass never writes while a live turn holds
// the conversation, and never commits a batch another pass already covered
// (compare-and-set on the cursor). It changes no memory. A proposal that may
// hold a secret is refused; the others are queued, or reported as
// duplicates, previously declined, or already applied.
func (s *Store) CommitMemoryBackgroundPass(ctx context.Context, lease memory.TurnLease, commit memory.BackgroundCommit) (memory.BackgroundCommitResult, error) {
	result := memory.BackgroundCommitResult{ProposalIDs: []string{}}
	run := commit.Run
	run.Status = memory.BackgroundRunSucceeded
	if err := validateBackgroundRun(run); err != nil {
		return result, err
	}
	if lease.SessionID != run.SessionID {
		return result, errors.New("background commit does not match its turn lease")
	}
	type encodedProposal struct {
		proposal   inboxProposal
		encoded    []byte
		everywhere *inboxVersion
	}
	var proposals []encodedProposal
	for _, queued := range commit.Proposals {
		if queued.Reason != memory.MemoryProposalBackground || queued.IntentEventID != "" {
			return result, errors.New("a background proposal cites no tool intent and has the background reason")
		}
		p, err := inboxProposalOf(queued)
		if err != nil {
			return result, err
		}
		if p.sessionID() != run.SessionID {
			return result, errors.New("a background proposal belongs to its pass's conversation")
		}
		encoded, err := encodeInboxProposal(p)
		var everywhere *inboxVersion
		if err == nil && queued.Everywhere != nil {
			everywhere, err = newInboxVersion(p, inboxProposal{literal: queued.Everywhere})
		}
		if errors.Is(err, errInboxProposalSecret) {
			result.Refused++
			continue
		}
		if err != nil {
			return result, err
		}
		proposals = append(proposals, encodedProposal{proposal: p, encoded: encoded, everywhere: everywhere})
	}
	err := s.withTurnLeaseWrite(ctx, lease.SessionID, lease.HolderID, lease.FencingToken, func(writer turnLeaseWriteExecutor) error {
		if err := checkBackgroundCursor(ctx, writer, run); err != nil {
			return err
		}
		for _, item := range proposals {
			outcome, err := s.insertInboxProposal(ctx, writer, item.proposal, item.encoded, memory.MemoryProposalBackground, "", item.everywhere)
			if err != nil {
				return err
			}
			switch {
			case outcome.alreadyApplied:
				result.AlreadyApplied++
			case outcome.previouslyDeclined:
				result.PreviouslyDeclined++
			case outcome.duplicate:
				result.Duplicates++
			default:
				result.Queued++
				result.ProposalIDs = append(result.ProposalIDs, outcome.proposalID)
			}
		}
		run.Queued = result.Queued
		run.Duplicates += result.Duplicates + result.PreviouslyDeclined + result.AlreadyApplied
		run.Skipped += result.Refused
		if err := insertBackgroundRun(ctx, writer, run); err != nil {
			return err
		}
		return upsertBackgroundCursor(ctx, writer, run.SessionID, run.ThroughSequence, 0, 0, nil, run.FinishedAt)
	})
	return result, err
}

// FailMemoryBackgroundPass records a pass that did not commit, without the
// turn lease: it never touches the conversation or the inbox. A failed run
// keeps the cursor and sets the next attempt (backoff); a deferred run keeps
// the cursor and failure counts; a skipped batch moves the cursor past it so
// one malformed batch cannot stall the conversation, and the skipped run
// stays visible.
func (s *Store) FailMemoryBackgroundPass(ctx context.Context, failure memory.BackgroundFailure) error {
	run := failure.Run
	if run.Status == memory.BackgroundRunSucceeded {
		return errors.New("a succeeded pass commits through CommitMemoryBackgroundPass")
	}
	if err := validateBackgroundRun(run); err != nil {
		return err
	}
	if failure.SkipBatch != (run.Status == memory.BackgroundRunSkipped) {
		return errors.New("only a skipped run skips its batch")
	}
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		writer := &turnLeaseWriter{conn: conn}
		defer writer.close()
		if run.Status == memory.BackgroundRunSuperseded {
			// Spend only: another pass owns this batch and the cursor.
			return insertBackgroundRun(ctx, writer, run)
		}
		if err := checkBackgroundCursor(ctx, writer, run); err != nil {
			return err
		}
		if err := insertBackgroundRun(ctx, writer, run); err != nil {
			return err
		}
		if failure.SkipBatch {
			return upsertBackgroundCursor(ctx, writer, run.SessionID, run.ThroughSequence, 0, 0, nil, run.FinishedAt)
		}
		var failures, malformed int
		err := writer.queryRowContext(ctx, `SELECT failures, malformed FROM memory_background_cursors WHERE session_id = ?`, run.SessionID).Scan(&failures, &malformed)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if run.Status == memory.BackgroundRunFailed {
			failures++
			if run.Malformed {
				malformed++
			} else {
				malformed = 0
			}
		}
		next := failure.NextAttempt.UTC()
		return upsertBackgroundCursor(ctx, writer, run.SessionID, run.FromSequence, failures, malformed, &next, run.FinishedAt)
	})
}

func validateBackgroundRun(run memory.BackgroundRun) error {
	switch {
	case run.RunID == "" || run.SessionID == "" || run.Model == "":
		return errors.New("background run needs an ID, a conversation and a model")
	case !run.Trigger.Valid() || !run.Status.Valid():
		return fmt.Errorf("unsupported background run trigger %q or status %q", run.Trigger, run.Status)
	case run.FromSequence < 0 || run.ThroughSequence < run.FromSequence:
		return errors.New("background run covers an invalid range")
	case run.InputTokens < 0 || run.OutputTokens < 0 || run.CostMicroUSD < 0:
		return errors.New("background run spend must not be negative")
	case run.StartedAt.IsZero() || run.FinishedAt.Before(run.StartedAt):
		return errors.New("background run times are invalid")
	}
	return nil
}

// checkBackgroundCursor is the compare-and-set: a batch commits only from
// the cursor it was read at, and never past the conversation's last event.
func checkBackgroundCursor(ctx context.Context, writer turnLeaseWriteExecutor, run memory.BackgroundRun) error {
	var cursor int64
	err := writer.queryRowContext(ctx, `SELECT cursor_sequence FROM memory_background_cursors WHERE session_id = ?`, run.SessionID).Scan(&cursor)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if cursor != run.FromSequence {
		return memory.ErrBackgroundCursorMoved
	}
	var last int64
	if err := writer.queryRowContext(ctx, `SELECT COALESCE(MAX(sequence), 0) FROM events WHERE session_id = ?`, run.SessionID).Scan(&last); err != nil {
		return err
	}
	if run.ThroughSequence > last {
		return errors.New("background run covers events that do not exist")
	}
	return nil
}

func insertBackgroundRun(ctx context.Context, writer turnLeaseWriteExecutor, run memory.BackgroundRun) error {
	message := run.Error
	if len(message) > memoryBackgroundErrorBytes {
		message = strings.ToValidUTF8(message[:memoryBackgroundErrorBytes], "")
	}
	malformed := 0
	if run.Malformed {
		malformed = 1
	}
	_, err := writer.execContext(ctx, `
		INSERT INTO memory_background_runs (
			run_id, session_id, trigger, status, from_sequence, through_sequence, model, input_tokens, output_tokens,
			cost_micro_usd, messages, candidates, queued, duplicates, noops, skipped, malformed, error, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, run.RunID, run.SessionID, run.Trigger, run.Status, run.FromSequence, run.ThroughSequence, run.Model, run.InputTokens, run.OutputTokens,
		run.CostMicroUSD, run.Messages, run.Candidates, run.Queued, run.Duplicates, run.Noops, run.Skipped, malformed, message,
		formatSemanticTime(run.StartedAt.UTC()), formatSemanticTime(run.FinishedAt.UTC()))
	if err != nil {
		return fmt.Errorf("record background run: %w", err)
	}
	return nil
}

func upsertBackgroundCursor(ctx context.Context, writer turnLeaseWriteExecutor, sessionID memory.SessionID, cursor int64, failures, malformed int, next *time.Time, at time.Time) error {
	var nextText any
	if next != nil {
		nextText = formatSemanticTime(*next)
	}
	_, err := writer.execContext(ctx, `
		INSERT INTO memory_background_cursors (session_id, cursor_sequence, failures, malformed, next_attempt_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET cursor_sequence = excluded.cursor_sequence, failures = excluded.failures,
			malformed = excluded.malformed, next_attempt_at = excluded.next_attempt_at, updated_at = excluded.updated_at
	`, sessionID, cursor, failures, malformed, nextText, formatSemanticTime(at.UTC()))
	if err != nil {
		return fmt.Errorf("advance background cursor: %w", err)
	}
	return nil
}

// MemoryBackgroundSpend sums the pass's recorded spend since a time.
func (s *Store) MemoryBackgroundSpend(ctx context.Context, since time.Time) (memory.BackgroundSpend, error) {
	spend := memory.BackgroundSpend{Since: since.UTC()}
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cost_micro_usd), 0)
		FROM memory_background_runs WHERE finished_at >= ?`, formatSemanticTime(since.UTC())).Scan(&spend.InputTokens, &spend.OutputTokens, &spend.CostMicroUSD)
	if err != nil {
		return spend, fmt.Errorf("read background spend: %w", err)
	}
	return spend, nil
}

// MemoryBackgroundProgress is the durable half of the pass's status: when
// it was activated, the latest run and latest problem, today's spend (since
// UTC midnight of now), pending background proposals, and the cursors of
// the most recently active root conversations.
func (s *Store) MemoryBackgroundProgress(ctx context.Context, now time.Time, limit int) (memory.BackgroundProgress, error) {
	progress := memory.BackgroundProgress{Sessions: []memory.BackgroundSessionProgress{}}
	if limit < 1 || limit > 100 {
		return progress, errors.New("background progress limit must be 1 to 100")
	}
	var activated string
	err := s.db.QueryRowContext(ctx, `SELECT activated_at FROM memory_background_state WHERE singleton = 1`).Scan(&activated)
	if err == nil {
		at, parseErr := parseSemanticTime(activated)
		if parseErr != nil {
			return progress, parseErr
		}
		progress.ActivatedAt = &at
	} else if !errors.Is(err, sql.ErrNoRows) {
		return progress, err
	}
	if progress.LastRun, err = s.latestBackgroundRun(ctx, ``); err != nil {
		return progress, err
	}
	if progress.LastError, err = s.latestBackgroundRun(ctx, `WHERE status <> 'succeeded'`); err != nil {
		return progress, err
	}
	midnight := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if progress.Today, err = s.MemoryBackgroundSpend(ctx, midnight); err != nil {
		return progress, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(queued), 0) FROM memory_background_runs WHERE finished_at >= ?`,
		formatSemanticTime(midnight)).Scan(&progress.RunsToday, &progress.ProposalsQueuedDay); err != nil {
		return progress, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_background_runs WHERE status = 'skipped'`).Scan(&progress.SkippedBatches); err != nil {
		return progress, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_proposal_inbox AS inbox WHERE inbox.reason = 'background' AND `+inboxPending).
		Scan(&progress.PendingProposals); err != nil {
		return progress, err
	}
	cursor := `COALESCE(cursors.cursor_sequence, 0)`
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(owed.pending), 0) FROM (
			SELECT `+compactedPendingSQL("flush.id", "flush.cursor_sequence", "flush.compacted_through")+` AS pending
			FROM (
				SELECT sessions.id AS id, `+cursor+` AS cursor_sequence, `+compactedThroughSQL("sessions.id", cursor)+` AS compacted_through
				FROM sessions
				LEFT JOIN memory_background_cursors AS cursors ON cursors.session_id = sessions.id
				WHERE sessions.parent_session_id IS NULL AND sessions.status = ?
			) AS flush
		) AS owed WHERE owed.pending > 0`, memory.SessionActive).
		Scan(&progress.CompactionFlush.Conversations, &progress.CompactionFlush.PendingOwnerMessages); err != nil {
		return progress, fmt.Errorf("read owed compaction flushes: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT sessions.id, COALESCE(sessions.title, ''), sessions.workspace_id, sessions.project_id,
		       `+cursor+`, COALESCE(cursors.failures, 0), cursors.next_attempt_at,
		       (SELECT COUNT(*) FROM events WHERE events.session_id = sessions.id AND `+ownerMessageCondition+`
		          AND events.sequence > `+cursor+`),
		       `+compactedPendingSQL("sessions.id", cursor, compactedThroughSQL("sessions.id", cursor))+`,
		       (SELECT status FROM memory_background_runs AS runs WHERE runs.session_id = sessions.id ORDER BY runs.finished_at DESC, runs.run_id DESC LIMIT 1),
		       (SELECT finished_at FROM memory_background_runs AS runs WHERE runs.session_id = sessions.id ORDER BY runs.finished_at DESC, runs.run_id DESC LIMIT 1)
		FROM sessions
		LEFT JOIN memory_background_cursors AS cursors ON cursors.session_id = sessions.id
		WHERE sessions.parent_session_id IS NULL AND sessions.status = ?
		ORDER BY COALESCE((SELECT events.recorded_at FROM events WHERE events.session_id = sessions.id ORDER BY events.sequence DESC LIMIT 1),
		                  sessions.created_at) DESC, sessions.id LIMIT ?`, memory.SessionActive, limit)
	if err != nil {
		return progress, fmt.Errorf("read background cursors: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item memory.BackgroundSessionProgress
		var workspaceID, projectID, next, status, finished sql.NullString
		if err := rows.Scan(&item.SessionID, &item.Title, &workspaceID, &projectID, &item.Cursor, &item.Failures, &next,
			&item.PendingOwner, &item.CompactedPending, &status, &finished); err != nil {
			return progress, err
		}
		item.ScopeKey = scopeKeyForContext(memory.ScopeContext{WorkspaceID: memory.WorkspaceID(workspaceID.String), ProjectID: memory.ProjectID(projectID.String)})
		if next.Valid {
			if at, err := parseSemanticTime(next.String); err == nil {
				item.NextAttemptAt = &at
			}
		}
		if status.Valid {
			item.LastStatus = memory.BackgroundRunStatus(status.String)
		}
		if finished.Valid {
			if at, err := parseSemanticTime(finished.String); err == nil {
				item.LastRunAt = &at
			}
		}
		progress.Sessions = append(progress.Sessions, item)
	}
	return progress, rows.Err()
}

func (s *Store) latestBackgroundRun(ctx context.Context, where string) (*memory.BackgroundRun, error) {
	var run memory.BackgroundRun
	var malformed int
	var started, finished string
	err := s.db.QueryRowContext(ctx, `
		SELECT run_id, session_id, trigger, status, from_sequence, through_sequence, model, input_tokens, output_tokens, cost_micro_usd,
		       messages, candidates, queued, duplicates, noops, skipped, malformed, error, started_at, finished_at
		FROM memory_background_runs `+where+` ORDER BY finished_at DESC, run_id DESC LIMIT 1`).Scan(
		&run.RunID, &run.SessionID, &run.Trigger, &run.Status, &run.FromSequence, &run.ThroughSequence, &run.Model, &run.InputTokens,
		&run.OutputTokens, &run.CostMicroUSD, &run.Messages, &run.Candidates, &run.Queued, &run.Duplicates, &run.Noops, &run.Skipped,
		&malformed, &run.Error, &started, &finished)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	run.Malformed = malformed == 1
	if run.StartedAt, err = parseSemanticTime(started); err != nil {
		return nil, err
	}
	if run.FinishedAt, err = parseSemanticTime(finished); err != nil {
		return nil, err
	}
	return &run, nil
}
