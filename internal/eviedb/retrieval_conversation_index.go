package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

const conversationIndexGeneration = "conversation-fts-unicode61-v1"

const conversationRetrievalSchema = `
INSERT OR IGNORE INTO memory_retrieval_generations(generation,configuration,state)
 VALUES ('conversation-fts-unicode61-v1','{"tokenizer":"unicode61","document_version":1,"fields":["user_message.content","assistant_message.content"]}','building');
CREATE TABLE IF NOT EXISTS memory_retrieval_event_dirty(event_id TEXT PRIMARY KEY);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_retrieval_event_fts USING fts5(
 generation UNINDEXED,event_id UNINDEXED,scope_key UNINDEXED,body,tokenize='unicode61');
CREATE TRIGGER IF NOT EXISTS memory_retrieval_event_source_added AFTER INSERT ON semantic_source_links BEGIN
 INSERT OR IGNORE INTO memory_retrieval_event_dirty VALUES(NEW.event_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_event_state_added AFTER INSERT ON semantic_state_events BEGIN
 INSERT OR IGNORE INTO memory_retrieval_event_dirty SELECT event_id FROM semantic_source_links
 WHERE (NEW.object_kind='claim' AND claim_id=NEW.object_id)
 OR (NEW.object_kind='source_link' AND source_link_id=NEW.object_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_event_workspace_changed AFTER UPDATE OF lifecycle_state ON workspaces BEGIN
 INSERT OR IGNORE INTO memory_retrieval_event_dirty SELECT id FROM events WHERE workspace_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_event_project_changed AFTER UPDATE OF archived ON projects BEGIN
 INSERT OR IGNORE INTO memory_retrieval_event_dirty SELECT id FROM events WHERE project_id=NEW.id;
END;
`

func conversationIndexCoverage(ctx context.Context, q semanticInspectionQueryer) (memory.RetrievalCoverage, error) {
	c := memory.RetrievalCoverage{Generation: conversationIndexGeneration}
	err := q.QueryRowContext(ctx, `SELECT state,
 (SELECT count(*) FROM events WHERE rowid>g.checkpoint)
 +(SELECT count(*) FROM memory_retrieval_event_dirty d JOIN events e ON e.id=d.event_id WHERE e.rowid<=g.checkpoint),
 (SELECT count(*) FROM memory_retrieval_event_fts WHERE generation=g.generation)
 FROM memory_retrieval_generations g WHERE generation=?`, conversationIndexGeneration).Scan(&c.State, &c.Pending, &c.Indexed)
	return c, err
}

func combinedRetrievalCoverage(ctx context.Context, q semanticInspectionQueryer) (memory.RetrievalCoverage, error) {
	accepted, err := memoryIndexCoverage(ctx, q)
	if err != nil {
		return accepted, err
	}
	events, err := conversationIndexCoverage(ctx, q)
	if err != nil {
		return accepted, err
	}
	accepted.Generation += "+" + events.Generation
	accepted.Pending += events.Pending
	accepted.Indexed += events.Indexed
	if events.State != "active" {
		accepted.State = "building"
	}
	return accepted, nil
}

// appendConversationProjection participates in the already fenced event
// transaction. Only allowlisted public content is copied; no payload is read.
func appendConversationProjection(ctx context.Context, q eventQueryExecutor, event memory.Event) error {
	var rowid int64
	if err := q.queryRowContext(ctx, `SELECT rowid FROM events WHERE id=?`, event.ID).Scan(&rowid); err != nil {
		return err
	}
	eligible := (event.Type == memory.EventUserMessage && event.Role == memory.RoleUser) || (event.Type == memory.EventAssistantMessage && event.Role == memory.RoleAssistant)
	if eligible && event.Content != "" && len(event.Content) <= 32768 && utf8.ValidString(event.Content) && !memory.HasRetrievalSecret([]byte(event.Content)) {
		var generation string
		if err := q.queryRowContext(ctx, `INSERT INTO memory_retrieval_event_fts(generation,event_id,scope_key,body) VALUES(?,?,?,?) RETURNING generation`,
			conversationIndexGeneration, event.ID, scopeKeyForContext(memory.ScopeContext{SessionID: event.SessionID, WorkspaceID: event.WorkspaceID, ProjectID: event.ProjectID}), event.Content).Scan(&generation); err != nil {
			return err
		}
	}
	var checkpoint int64
	return q.queryRowContext(ctx, `UPDATE memory_retrieval_generations
 SET checkpoint=CASE WHEN state='active' AND checkpoint=?-1 THEN ? ELSE checkpoint END
 WHERE generation=? RETURNING checkpoint`, rowid, rowid, conversationIndexGeneration).Scan(&checkpoint)
}

func (s *Store) refreshConversationIndex(ctx context.Context, limit int) error {
	coverage, err := conversationIndexCoverage(ctx, s.db)
	if err != nil || coverage.State == "active" && coverage.Pending == 0 {
		return err
	}
	return s.withImmediateTransaction(ctx, func(q *sql.Conn) error {
		var checkpoint int64
		if err := q.QueryRowContext(ctx, `SELECT checkpoint FROM memory_retrieval_generations WHERE generation=?`, conversationIndexGeneration).Scan(&checkpoint); err != nil {
			return err
		}
		type entry struct {
			row int64
			id  memory.EventID
		}
		var entries []entry
		rows, err := q.QueryContext(ctx, `SELECT rowid,id FROM events WHERE rowid>? ORDER BY rowid LIMIT ?`, checkpoint, limit)
		if err != nil {
			return err
		}
		for rows.Next() {
			var e entry
			if err = rows.Scan(&e.row, &e.id); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, e)
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		if len(entries) == 0 {
			rows, err = q.QueryContext(ctx, `SELECT 0,event_id FROM memory_retrieval_event_dirty ORDER BY event_id LIMIT ?`, limit)
			if err != nil {
				return err
			}
			for rows.Next() {
				var e entry
				if err = rows.Scan(&e.row, &e.id); err != nil {
					rows.Close()
					return err
				}
				entries = append(entries, e)
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
		for _, e := range entries {
			if _, err = q.ExecContext(ctx, `DELETE FROM memory_retrieval_event_fts WHERE generation=? AND event_id=?`, conversationIndexGeneration, e.id); err != nil {
				return err
			}
			event, eligible, err := loadConversationEvidence(ctx, q, e.id)
			if err != nil {
				return err
			}
			if eligible {
				spans, err := eligibleConversationSpans(ctx, q, event)
				if err != nil {
					return err
				}
				var parts []string
				for _, span := range spans {
					parts = append(parts, event.content[span.start:span.end])
				}
				if body := strings.Join(parts, "\n"); strings.TrimSpace(body) != "" {
					if _, err = q.ExecContext(ctx, `INSERT INTO memory_retrieval_event_fts(generation,event_id,scope_key,body) VALUES(?,?,?,?)`, conversationIndexGeneration, e.id, event.scope, body); err != nil {
						return err
					}
				}
			}
			if _, err = q.ExecContext(ctx, `DELETE FROM memory_retrieval_event_dirty WHERE event_id=?`, e.id); err != nil {
				return err
			}
			checkpoint = max(checkpoint, e.row)
		}
		if _, err = q.ExecContext(ctx, `UPDATE memory_retrieval_generations SET checkpoint=? WHERE generation=?`, checkpoint, conversationIndexGeneration); err != nil {
			return err
		}
		coverage, err = conversationIndexCoverage(ctx, q)
		if err != nil {
			return err
		}
		if coverage.Pending == 0 {
			_, err = q.ExecContext(ctx, `UPDATE memory_retrieval_generations SET state='active' WHERE generation=?`, conversationIndexGeneration)
		}
		return err
	})
}

// RefreshMemoryIndex shares one row budget across both independent generations.
// The serving state of one generator cannot imply coverage of the other.
func (s *Store) RefreshMemoryIndex(ctx context.Context, limit int) (memory.RetrievalCoverage, error) {
	if limit < 1 || limit > 256 {
		return memory.RetrievalCoverage{}, errors.New("retrieval refresh batch must be between 1 and 256")
	}
	accepted, err := memoryIndexCoverage(ctx, s.db)
	if err != nil {
		return accepted, err
	}
	acceptedLimit := 0
	if accepted.State != "active" || accepted.Pending > 0 {
		acceptedLimit = max(1, limit/2)
	}
	if acceptedLimit > 0 {
		if _, err = s.refreshAcceptedMemoryIndex(ctx, acceptedLimit); err != nil {
			return memory.RetrievalCoverage{}, err
		}
	}
	if remaining := limit - acceptedLimit; remaining > 0 {
		if err = s.refreshConversationIndex(ctx, remaining); err != nil {
			return memory.RetrievalCoverage{}, err
		}
	}
	return combinedRetrievalCoverage(ctx, s.db)
}
