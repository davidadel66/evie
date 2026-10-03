package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

// Session episodes keep one searchable summary per primary session: its
// latest accepted rolling compaction summary. They are a rebuildable
// projection of context_compacted events, maintained in the fenced event
// transaction and reconciled at startup; no model is called to create one.
const sessionEpisodeSchema = `
CREATE INDEX IF NOT EXISTS events_context_compacted_idx ON events(session_id, sequence) WHERE event_type='context_compacted';
CREATE TABLE IF NOT EXISTS memory_session_episodes (
    episode_rowid          INTEGER PRIMARY KEY,
    session_id             TEXT NOT NULL UNIQUE REFERENCES sessions(id),
    scope_key              TEXT NOT NULL CHECK (length(scope_key) > 0),
    summary_event_id       TEXT NOT NULL UNIQUE REFERENCES events(id),
    generation             INTEGER NOT NULL CHECK (generation > 0),
    covered_first_event_id TEXT NOT NULL REFERENCES events(id),
    covered_first_sequence INTEGER NOT NULL CHECK (covered_first_sequence > 0),
    covered_last_event_id  TEXT NOT NULL REFERENCES events(id),
    covered_last_sequence  INTEGER NOT NULL CHECK (covered_last_sequence >= covered_first_sequence),
    started_at             TEXT NOT NULL,
    ended_at               TEXT NOT NULL,
    summarized_at          TEXT NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_session_episode_fts USING fts5(body, tokenize='unicode61');
CREATE TRIGGER IF NOT EXISTS memory_session_episodes_replaced AFTER UPDATE ON memory_session_episodes BEGIN
 DELETE FROM memory_session_episode_fts WHERE rowid=OLD.episode_rowid;
END;
`

const (
	sessionEpisodeCandidateLimit   = 32
	sessionEpisodeResultBytes      = 24 * 1024
	sessionEpisodeAssociationLimit = 256
)

// staleSessionEpisodesSQL lists the latest context_compacted event of every
// primary session whose episode is missing or does not reflect it.
const staleSessionEpisodesSQL = `SELECT latest.id FROM sessions s
 JOIN events latest ON latest.id=(SELECT c.id FROM events c WHERE c.session_id=s.id AND c.event_type='context_compacted' ORDER BY c.sequence DESC LIMIT 1)
 LEFT JOIN memory_session_episodes ep ON ep.session_id=s.id
 WHERE s.parent_session_id IS NULL AND (ep.summary_event_id IS NULL OR ep.summary_event_id<>latest.id)
 ORDER BY latest.rowid`

// sessionEpisodeWriter is the event transaction's executor. Every episode
// write returns a row, so the projection needs no other capability.
type sessionEpisodeWriter = eventQueryExecutor

type sessionEpisodeConn struct{ conn *sql.Conn }

func (c sessionEpisodeConn) queryRowContext(ctx context.Context, statement string, args ...any) rowScanner {
	return c.conn.QueryRowContext(ctx, statement, args...)
}

// sessionEpisodeSource is everything one episode records. Compaction supplies
// it today. A later background summary pass can supply the same fields for an
// idle session through upsertSessionEpisode without a model call here.
type sessionEpisodeSource struct {
	session                          memory.SessionID
	scopeKey                         string
	source                           memory.SessionEpisodeSource
	startedAt, endedAt, summarizedAt string
	summary                          string
}

func ensureSessionEpisodes(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, sessionEpisodeSchema); err != nil {
		return err
	}
	return reconcileSessionEpisodes(ctx, db)
}

// reconcileSessionEpisodes derives missing or stale episodes from retained
// context_compacted events: the backfill for databases written before
// episodes existed, and repair after a process that did not maintain them.
// The common no-op case takes no write lock.
func reconcileSessionEpisodes(ctx context.Context, db *sql.DB) error {
	stale, err := sessionEpisodeEventIDs(ctx, db)
	if err != nil || len(stale) == 0 {
		return err
	}
	return withImmediateTransaction(ctx, db, func(conn *sql.Conn) error {
		stale, err := sessionEpisodeEventIDs(ctx, conn)
		if err != nil {
			return err
		}
		writer := sessionEpisodeConn{conn: conn}
		for _, id := range stale {
			event, err := scanEvent(conn.QueryRowContext(ctx, `SELECT id, session_id, sequence, workspace_id, project_id, parent_id, event_type, role, execution_id, content, payload_json, recorded_at, format_version FROM events WHERE id=?`, id))
			if err != nil {
				return err
			}
			// Retained history that no longer validates yields no episode; it
			// must not block startup.
			payload, canonical, err := decodeCanonicalContextCompactedPayload(event.Payload)
			if err != nil || string(canonical) != string(event.Payload) || payload.Validate(event.Content) != nil {
				continue
			}
			if err := appendSessionEpisode(ctx, writer, event); err != nil {
				return err
			}
		}
		return nil
	})
}

func sessionEpisodeEventIDs(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) ([]memory.EventID, error) {
	rows, err := q.QueryContext(ctx, staleSessionEpisodesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []memory.EventID
	for rows.Next() {
		var id memory.EventID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// appendSessionEpisode participates in the fenced event transaction. An
// accepted context_compacted event of a primary session becomes that
// session's episode; delegated child sessions have none.
func appendSessionEpisode(ctx context.Context, w sessionEpisodeWriter, event memory.Event) error {
	if event.Type != memory.EventContextCompacted {
		return nil
	}
	var primary bool
	var workspace, project string
	if err := w.queryRowContext(ctx, `SELECT parent_session_id IS NULL,COALESCE(workspace_id,''),COALESCE(project_id,'') FROM sessions WHERE id=?`, event.SessionID).
		Scan(&primary, &workspace, &project); err != nil {
		return err
	}
	if !primary {
		return nil
	}
	var payload memory.ContextCompactedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return err
	}
	source := sessionEpisodeSource{
		session:  event.SessionID,
		scopeKey: scopeKeyForContext(memory.ScopeContext{SessionID: event.SessionID, WorkspaceID: memory.WorkspaceID(workspace), ProjectID: memory.ProjectID(project)}),
		summary:  event.Content,
		source: memory.SessionEpisodeSource{SummaryEventID: event.ID, Generation: payload.Generation,
			CoveredFirstEventID: payload.CoveredFirstEventID, CoveredLastEventID: payload.CoveredLastEventID},
	}
	if payload.Generation > 1 {
		// Each generation re-summarizes the prior one, so the rolling summary
		// covers the chain from the first generation's covered frontier.
		if err := w.queryRowContext(ctx, `SELECT json_extract(payload_json,'$.covered_first_event_id') FROM events
 WHERE session_id=? AND event_type='context_compacted' ORDER BY sequence LIMIT 1`, event.SessionID).Scan(&source.source.CoveredFirstEventID); err != nil {
			return err
		}
	}
	if err := w.queryRowContext(ctx, `SELECT sequence,recorded_at FROM events WHERE session_id=? AND id=?`, event.SessionID, source.source.CoveredFirstEventID).
		Scan(&source.source.CoveredFirstSequence, &source.startedAt); err != nil {
		return err
	}
	if err := w.queryRowContext(ctx, `SELECT sequence,recorded_at FROM events WHERE session_id=? AND id=?`, event.SessionID, source.source.CoveredLastEventID).
		Scan(&source.source.CoveredLastSequence, &source.endedAt); err != nil {
		return err
	}
	if err := w.queryRowContext(ctx, `SELECT recorded_at FROM events WHERE session_id=? AND id=?`, event.SessionID, event.ID).Scan(&source.summarizedAt); err != nil {
		return err
	}
	return upsertSessionEpisode(ctx, w, source)
}

// upsertSessionEpisode keeps exactly one episode per session and only moves
// it forward: an equal or older generation never replaces a newer one, so a
// racing startup reconciliation cannot regress it.
func upsertSessionEpisode(ctx context.Context, w sessionEpisodeWriter, e sessionEpisodeSource) error {
	var rowid, generation int64
	err := w.queryRowContext(ctx, `SELECT episode_rowid,generation FROM memory_session_episodes WHERE session_id=?`, e.session).Scan(&rowid, &generation)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err = w.queryRowContext(ctx, `INSERT INTO memory_session_episodes(session_id,scope_key,summary_event_id,generation,
 covered_first_event_id,covered_first_sequence,covered_last_event_id,covered_last_sequence,started_at,ended_at,summarized_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING episode_rowid`, e.session, e.scopeKey, e.source.SummaryEventID, e.source.Generation,
			e.source.CoveredFirstEventID, e.source.CoveredFirstSequence, e.source.CoveredLastEventID, e.source.CoveredLastSequence,
			e.startedAt, e.endedAt, e.summarizedAt).Scan(&rowid); err != nil {
			return err
		}
	case err != nil:
		return err
	case generation >= e.source.Generation:
		return nil
	default:
		if err = w.queryRowContext(ctx, `UPDATE memory_session_episodes SET scope_key=?,summary_event_id=?,generation=?,
 covered_first_event_id=?,covered_first_sequence=?,covered_last_event_id=?,covered_last_sequence=?,started_at=?,ended_at=?,summarized_at=?
 WHERE episode_rowid=? RETURNING episode_rowid`, e.scopeKey, e.source.SummaryEventID, e.source.Generation, e.source.CoveredFirstEventID, e.source.CoveredFirstSequence,
			e.source.CoveredLastEventID, e.source.CoveredLastSequence, e.startedAt, e.endedAt, e.summarizedAt, rowid).Scan(&rowid); err != nil {
			return err
		}
		// The replaced trigger removed the prior summary's index row.
	}
	body, ok := sessionEpisodeIndexBody(e.summary)
	if !ok {
		return nil
	}
	return w.queryRowContext(ctx, `INSERT INTO memory_session_episode_fts(rowid,body) VALUES(?,?) RETURNING rowid`, rowid, body).Scan(&rowid)
}

// sessionEpisodeIndexBody is the searchable text of a summary. A summary with
// a detected secret is never indexed. The fixed section headings are
// structure shared by every summary, so they never match a query.
func sessionEpisodeIndexBody(summary string) (string, bool) {
	if !sessionEpisodeSummaryEligible(summary) {
		return "", false
	}
	headings := make(map[string]bool)
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		headings["## "+heading] = true
	}
	var kept []string
	for _, line := range strings.Split(summary, "\n") {
		if !headings[strings.TrimSpace(line)] {
			kept = append(kept, line)
		}
	}
	body := strings.Join(kept, "\n")
	return body, strings.TrimSpace(body) != ""
}

func sessionEpisodeSummaryEligible(summary string) bool {
	return summary != "" && len(summary) <= memory.ContextCompactedSummaryMaxBytes && utf8.ValidString(summary) &&
		!memory.HasRetrievalSecret([]byte(summary))
}

type sessionEpisodeState int

const (
	sessionEpisodeIneligible sessionEpisodeState = iota
	sessionEpisodeWithheld
	sessionEpisodeEligible
)

// SearchSessionEpisodes searches other sessions' episodes in exactly the area
// conversation search may read: the same Workspace, the same project, or
// Global to Global. Index hits are suggestions; every returned episode is
// re-read and re-verified in the same read transaction.
func (s *Store) SearchSessionEpisodes(ctx context.Context, scope memory.ScopeContext, query memory.SessionEpisodeQuery) (memory.SessionEpisodeResult, error) {
	result := memory.SessionEpisodeResult{Status: memory.RetrievalFailed, Episodes: []memory.SessionEpisode{}}
	normalized, fts, err := normalizeRetrievalQuery(memory.RetrievalQuery{Text: query.Text, Kind: memory.RetrievalConversationExcerpt, Intent: query.Intent})
	if err != nil {
		return result, err
	}
	limit := query.Limit
	if limit == 0 {
		limit = memory.SessionEpisodeDefaultLimit
	}
	if limit < 1 || limit > memory.SessionEpisodeMaxLimit {
		return result, ErrInvalidRetrievalQuery
	}
	result.Intent = normalized.Intent
	ctx, cancel := context.WithTimeout(ctx, retrievalDeadline)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	defer tx.Rollback()
	if err = validateSessionScopeIdentity(ctx, tx, scope); err != nil {
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	scopeKey := scopeKeyForContext(scope)
	if err = requireSemanticScopeKeysAvailable(ctx, tx, []string{scopeKey}); err != nil {
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	// The bound session's own summary is already in its request.
	rows, err := tx.QueryContext(ctx, `SELECT ep.session_id FROM memory_session_episode_fts f JOIN memory_session_episodes ep ON ep.episode_rowid=f.rowid
 WHERE memory_session_episode_fts MATCH ? AND ep.scope_key=? AND ep.session_id<>?
 ORDER BY bm25(memory_session_episode_fts),ep.summarized_at DESC,ep.session_id LIMIT ?`, fts, scopeKey, scope.SessionID, sessionEpisodeCandidateLimit+1)
	if err != nil {
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	var candidates []memory.SessionID
	for rows.Next() {
		var session memory.SessionID
		if err = rows.Scan(&session); err != nil {
			rows.Close()
			return sessionEpisodeReadFailure(ctx, result, err)
		}
		candidates = append(candidates, session)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	rows.Close()
	if len(candidates) > sessionEpisodeCandidateLimit {
		candidates, result.Truncated = candidates[:sessionEpisodeCandidateLimit], true
	}
	for _, session := range candidates {
		episode, state, err := loadSessionEpisode(ctx, tx, scope, session, normalized.Intent)
		if err != nil {
			return sessionEpisodeReadFailure(ctx, result, err)
		}
		if state == sessionEpisodeWithheld {
			result.Withheld++
			continue
		}
		if state != sessionEpisodeEligible {
			continue
		}
		if len(result.Episodes) == limit {
			result.Truncated = true
			break
		}
		result.Episodes = append(result.Episodes, episode)
	}
	if err = tx.Commit(); err != nil {
		return sessionEpisodeReadFailure(ctx, result, err)
	}
	result.Status = memory.RetrievalSuccess
	if len(result.Episodes) == 0 {
		result.Status = memory.RetrievalEmpty
	}
	if result.Withheld > 0 {
		result.Status = memory.RetrievalPartial
	}
	return boundSessionEpisodes(result)
}

func sessionEpisodeReadFailure(ctx context.Context, result memory.SessionEpisodeResult, err error) (memory.SessionEpisodeResult, error) {
	result.Status, result.Episodes = memory.RetrievalFailed, nil
	if ctx.Err() != nil {
		result.Status, err = memory.RetrievalCancelled, ctx.Err()
	}
	return result, err
}

// boundSessionEpisodes drops trailing episodes, then shortens a single
// remaining summary at a UTF-8 boundary, until the encoded result fits.
func boundSessionEpisodes(result memory.SessionEpisodeResult) (memory.SessionEpisodeResult, error) {
	for {
		encoded, err := json.Marshal(result)
		if err != nil {
			return result, err
		}
		excess := len(encoded) - sessionEpisodeResultBytes
		if excess <= 0 || len(result.Episodes) == 0 {
			return result, nil
		}
		result.Truncated = true
		if len(result.Episodes) > 1 {
			result.Episodes = result.Episodes[:len(result.Episodes)-1]
			continue
		}
		episode := &result.Episodes[0]
		cut := max(0, len(episode.Summary)-excess)
		for cut > 0 && !utf8.RuneStart(episode.Summary[cut]) {
			cut--
		}
		episode.Summary, episode.SummaryTruncated = episode.Summary[:cut], true
	}
}

func loadSessionEpisode(ctx context.Context, q semanticInspectionQueryer, reader memory.ScopeContext, session memory.SessionID, intent string) (memory.SessionEpisode, sessionEpisodeState, error) {
	episode := memory.SessionEpisode{EpisodeID: "episode:" + string(session), SessionID: session}
	var (
		eventSession                            memory.SessionID
		eventType, content, payloadJSON, latest string
		started, ended, summarized              string
		workspace, project, workspaceState      string
		archived                                int
		primary                                 bool
	)
	err := q.QueryRowContext(ctx, `SELECT ep.scope_key,ep.summary_event_id,ep.generation,ep.covered_first_event_id,ep.covered_first_sequence,
 ep.covered_last_event_id,ep.covered_last_sequence,ep.started_at,ep.ended_at,ep.summarized_at,
 e.session_id,e.event_type,e.content,e.payload_json,
 s.parent_session_id IS NULL,COALESCE(s.workspace_id,''),COALESCE(s.project_id,''),COALESCE(w.lifecycle_state,'active'),COALESCE(p.archived,0),
 COALESCE((SELECT c.id FROM events c WHERE c.session_id=ep.session_id AND c.event_type='context_compacted' ORDER BY c.sequence DESC LIMIT 1),'')
 FROM memory_session_episodes ep JOIN events e ON e.id=ep.summary_event_id JOIN sessions s ON s.id=ep.session_id
 LEFT JOIN workspaces w ON w.id=s.workspace_id LEFT JOIN projects p ON p.id=s.project_id
 WHERE ep.session_id=?`, session).Scan(&episode.ScopeKey, &episode.Source.SummaryEventID, &episode.Source.Generation,
		&episode.Source.CoveredFirstEventID, &episode.Source.CoveredFirstSequence, &episode.Source.CoveredLastEventID, &episode.Source.CoveredLastSequence,
		&started, &ended, &summarized, &eventSession, &eventType, &content, &payloadJSON,
		&primary, &workspace, &project, &workspaceState, &archived, &latest)
	if errors.Is(err, sql.ErrNoRows) {
		return episode, sessionEpisodeIneligible, nil
	}
	if err != nil {
		return episode, sessionEpisodeIneligible, err
	}
	// The same source eligibility as conversation evidence: a primary
	// session in the reader's exact area, an active Workspace or unarchived
	// project, and no detected secret.
	sessionKey := scopeKeyForContext(memory.ScopeContext{SessionID: session, WorkspaceID: memory.WorkspaceID(workspace), ProjectID: memory.ProjectID(project)})
	if !primary || workspaceState != "active" || archived != 0 || sessionKey != episode.ScopeKey || sessionKey != scopeKeyForContext(reader) ||
		!sessionEpisodeSummaryEligible(content) {
		return episode, sessionEpisodeIneligible, nil
	}
	// Only the session's latest accepted summary is its episode. A row that
	// is not, or that no longer matches its event, is withheld until startup
	// reconciliation repairs it.
	payload, canonical, err := decodeCanonicalContextCompactedPayload(json.RawMessage(payloadJSON))
	if latest != string(episode.Source.SummaryEventID) || eventSession != session || eventType != string(memory.EventContextCompacted) ||
		err != nil || string(canonical) != payloadJSON || payload.Validate(content) != nil ||
		payload.Generation != episode.Source.Generation || payload.CoveredLastEventID != episode.Source.CoveredLastEventID {
		return episode, sessionEpisodeWithheld, nil
	}
	for _, value := range []struct {
		text   string
		target *time.Time
	}{{started, &episode.StartedAt}, {ended, &episode.EndedAt}, {summarized, &episode.SummarizedAt}} {
		if *value.target, err = time.Parse(time.RFC3339Nano, value.text); err != nil {
			return episode, sessionEpisodeWithheld, nil
		}
	}
	status, historical, suppressed, err := sessionEpisodeSourceLifecycle(ctx, q, reader, session, episode.Source.CoveredFirstSequence, episode.Source.CoveredLastSequence)
	if errors.Is(err, ErrConversationAssociation) {
		return episode, sessionEpisodeWithheld, nil
	}
	if err != nil {
		return episode, sessionEpisodeIneligible, err
	}
	if suppressed || status == memory.SemanticStatusRetired && intent != memory.RetrievalHistorical {
		return episode, sessionEpisodeWithheld, nil
	}
	episode.CurrentStatus, episode.HistoricalClaims = status, historical
	episode.Summary, episode.SummaryBytes = content, len(content)
	return episode, sessionEpisodeEligible, nil
}

// sessionEpisodeSourceLifecycle applies the conversation-evidence retirement
// rules to the covered range. A summary is derived text that may restate any
// covered statement, so the rules apply to the whole episode: a retracted
// source suppresses it for every intent, a retired Claim's source suppresses
// it unless the read is historical, and a superseded Claim labels it. Claims
// are named only for scopes the reader may read.
func sessionEpisodeSourceLifecycle(ctx context.Context, q semanticInspectionQueryer, reader memory.ScopeContext, session memory.SessionID, first, last int64) (memory.SemanticObjectStatus, []memory.RetrievalHistoricalClaim, bool, error) {
	readable := []string{"global", scopeKeyForContext(reader), "session:" + string(reader.SessionID)}
	rows, err := q.QueryContext(ctx, `WITH associations AS (
 SELECT sl.claim_id,sc.scope_key,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='claim' AND object_id=sl.claim_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'active') AS current,
 COALESCE((SELECT state FROM semantic_state_events WHERE object_kind='source_link' AND object_id=sl.source_link_id
 ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1),'eligible') AS source_state
 FROM semantic_source_links sl JOIN events e ON e.id=sl.event_id
 JOIN semantic_claims c ON c.claim_id=sl.claim_id JOIN semantic_scopes sc ON sc.scope_id=c.scope_id
 WHERE sl.source_session_id=? AND e.session_id=? AND e.sequence BETWEEN ? AND ? AND sl.authority<>'evie_proposed')
 SELECT claim_id,scope_key,current,source_state FROM associations
 WHERE current IN ('retired','superseded') OR source_state='retracted' ORDER BY claim_id LIMIT ?`,
		session, session, first, last, sessionEpisodeAssociationLimit+1)
	if err != nil {
		return "", nil, false, err
	}
	type ended struct {
		claim memory.SemanticID
		state memory.SemanticStateValue
	}
	status := memory.SemanticStatusActive
	suppressed := false
	var named []ended
	seen := make(map[memory.SemanticID]bool)
	count := 0
	for rows.Next() {
		var claim memory.SemanticID
		var scopeKey string
		var current, source memory.SemanticStateValue
		if err = rows.Scan(&claim, &scopeKey, &current, &source); err != nil {
			rows.Close()
			return "", nil, false, err
		}
		if count++; count > sessionEpisodeAssociationLimit {
			rows.Close()
			return "", nil, false, ErrConversationAssociation
		}
		if source == memory.SemanticStateRetracted {
			suppressed = true
			continue
		}
		if current == memory.SemanticStateRetired {
			status = memory.SemanticStatusRetired
		} else if status == memory.SemanticStatusActive {
			status = memory.SemanticStatusSuperseded
		}
		if containsString(readable, scopeKey) && !seen[claim] {
			seen[claim] = true
			named = append(named, ended{claim: claim, state: current})
		}
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return "", nil, false, err
	}
	rows.Close()
	var historical []memory.RetrievalHistoricalClaim
	for _, item := range named {
		link, err := historicalClaimLink(ctx, q, memory.RetrievalHistoricalSource, item.claim, item.state)
		if err != nil {
			return "", nil, false, err
		}
		historical = append(historical, link)
	}
	sort.Slice(historical, func(i, j int) bool { return historical[i].ClaimID < historical[j].ClaimID })
	return status, historical, suppressed, nil
}
