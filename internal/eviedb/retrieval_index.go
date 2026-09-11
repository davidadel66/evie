package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/memory"
)

const memoryIndexGeneration = "accepted-fts-unicode61-v3"

// The checkpoint and dirty queue belong to the projection, never accepted
// memory. Triggers enqueue inside the accepting transaction; foreground reads
// exclude dirty rows until a bounded maintenance batch refreshes them.
const retrievalSchema = `
CREATE TABLE IF NOT EXISTS memory_retrieval_generations (
 generation TEXT PRIMARY KEY, configuration TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('building','active')), checkpoint INTEGER NOT NULL DEFAULT 0
);
CREATE TRIGGER IF NOT EXISTS memory_retrieval_configuration_immutable
 BEFORE UPDATE OF generation,configuration ON memory_retrieval_generations
 BEGIN SELECT RAISE(ABORT,'retrieval configuration is immutable'); END;
INSERT OR IGNORE INTO memory_retrieval_generations(generation,configuration,state)
 VALUES ('accepted-fts-unicode61-v3','{"tokenizer":"unicode61","document_version":3,"lifecycle":"source-eligible-history"}', 'building');
CREATE TABLE IF NOT EXISTS memory_retrieval_dirty (claim_id TEXT PRIMARY KEY);
CREATE VIRTUAL TABLE IF NOT EXISTS memory_retrieval_fts_v3 USING fts5(
 generation UNINDEXED, claim_id UNINDEXED, scope_key UNINDEXED, body, tokenize='unicode61'
);
CREATE TRIGGER IF NOT EXISTS memory_retrieval_claim_added AFTER INSERT ON semantic_claims BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty VALUES(NEW.claim_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_source_added AFTER INSERT ON semantic_source_links BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty VALUES(NEW.claim_id);
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_alias_added AFTER INSERT ON semantic_aliases BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty SELECT claim_id FROM semantic_claims
 WHERE subject_entity_id=NEW.entity_id OR object_entity_id=NEW.entity_id;
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_workspace_changed AFTER UPDATE OF lifecycle_state ON workspaces BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty SELECT sl.claim_id FROM semantic_source_links sl
 JOIN sessions s ON s.id=sl.source_session_id WHERE s.workspace_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_project_changed AFTER UPDATE OF archived ON projects BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty SELECT sl.claim_id FROM semantic_source_links sl
 JOIN sessions s ON s.id=sl.source_session_id WHERE s.project_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_retrieval_state_added AFTER INSERT ON semantic_state_events BEGIN
 INSERT OR IGNORE INTO memory_retrieval_dirty SELECT claim_id FROM semantic_claims
 WHERE (NEW.object_kind='claim' AND claim_id=NEW.object_id)
 OR (NEW.object_kind='entity' AND (subject_entity_id=NEW.object_id OR object_entity_id=NEW.object_id))
 OR (NEW.object_kind='alias' AND (subject_entity_id=(SELECT entity_id FROM semantic_aliases WHERE alias_id=NEW.object_id)
 OR object_entity_id=(SELECT entity_id FROM semantic_aliases WHERE alias_id=NEW.object_id)))
 OR (NEW.object_kind='source_link' AND claim_id=(SELECT claim_id FROM semantic_source_links WHERE source_link_id=NEW.object_id));
END;
`

func ensureRetrievalSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, retrievalSchema+conversationRetrievalSchema+denseRetrievalSchema)
	return err
}

func memoryIndexCoverage(ctx context.Context, q semanticInspectionQueryer) (memory.RetrievalCoverage, error) {
	c := memory.RetrievalCoverage{Generation: memoryIndexGeneration}
	err := q.QueryRowContext(ctx, `SELECT state,
 (SELECT count(*) FROM semantic_claims WHERE rowid>g.checkpoint)
 +(SELECT count(*) FROM memory_retrieval_dirty d JOIN semantic_claims c ON c.claim_id=d.claim_id WHERE c.rowid<=g.checkpoint),
 (SELECT count(*) FROM memory_retrieval_fts_v3 WHERE generation=g.generation)
 FROM memory_retrieval_generations g WHERE generation=?`, memoryIndexGeneration).Scan(&c.State, &c.Pending, &c.Indexed)
	return c, err
}

func (s *Store) MemoryIndexCoverage(ctx context.Context) (memory.RetrievalCoverage, error) {
	return combinedRetrievalCoverage(ctx, s.db)
}

// RefreshMemoryIndex is maintenance, deliberately absent from SearchMemory.
// Repeated bounded batches resume the same durable generation after restart.
func (s *Store) refreshAcceptedMemoryIndex(ctx context.Context, limit int) (memory.RetrievalCoverage, error) {
	if limit < 1 || limit > 256 {
		return memory.RetrievalCoverage{}, errors.New("retrieval refresh batch must be between 1 and 256")
	}
	coverage, err := memoryIndexCoverage(ctx, s.db)
	if err != nil || (coverage.State == "active" && coverage.Pending == 0) {
		return coverage, err
	}
	err = s.withImmediateTransaction(ctx, func(q *sql.Conn) error {
		var checkpoint int64
		if err := q.QueryRowContext(ctx, `SELECT checkpoint FROM memory_retrieval_generations WHERE generation=?`, memoryIndexGeneration).Scan(&checkpoint); err != nil {
			return err
		}
		rows, err := q.QueryContext(ctx, `SELECT rowid,claim_id FROM semantic_claims WHERE rowid>? ORDER BY rowid LIMIT ?`, checkpoint, limit)
		if err != nil {
			return err
		}
		type entry struct {
			row int64
			id  memory.SemanticID
		}
		var entries []entry
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
			rows, err = q.QueryContext(ctx, `SELECT 0,claim_id FROM memory_retrieval_dirty ORDER BY claim_id LIMIT ?`, limit)
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
			if err = s.refreshMemoryClaim(ctx, q, e.id); err != nil {
				return err
			}
			if _, err = q.ExecContext(ctx, `DELETE FROM memory_retrieval_dirty WHERE claim_id=?`, e.id); err != nil {
				return err
			}
			checkpoint = max(checkpoint, e.row)
		}
		if _, err = q.ExecContext(ctx, `UPDATE memory_retrieval_generations SET checkpoint=? WHERE generation=?`, checkpoint, memoryIndexGeneration); err != nil {
			return err
		}
		coverage, err = memoryIndexCoverage(ctx, q)
		if err != nil {
			return err
		}
		if coverage.Pending == 0 {
			_, err = q.ExecContext(ctx, `UPDATE memory_retrieval_generations SET state='active' WHERE generation=?`, memoryIndexGeneration)
			coverage.State = "active"
		}
		return err
	})
	return coverage, err
}

func (s *Store) refreshMemoryClaim(ctx context.Context, q *sql.Conn, id memory.SemanticID) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM memory_retrieval_fts_v3 WHERE generation=? AND claim_id=?`, memoryIndexGeneration, id); err != nil {
		return err
	}
	claim, err := loadSemanticClaim(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, known, err := s.semanticQueryTimes(ctx, q, memory.ClaimQuery{})
	if err != nil {
		return err
	}
	sources, err := loadEligibleSourcesAt(ctx, q, id, known)
	if err != nil {
		return err
	}
	eligible := false
	for _, source := range sources {
		if _, ok, err := retrievalSource(ctx, q, source, nil); err != nil {
			return err
		} else if ok {
			eligible = true
			break
		}
	}
	if !eligible {
		return nil
	}
	subject, err := loadSemanticEntityForInspection(ctx, q, claim.SubjectEntityID)
	if err != nil {
		return err
	}
	object := ""
	if claim.Object.Literal != nil {
		object = claim.Object.Literal.Value
	} else if claim.Object.EntityID != "" {
		entity, err := loadSemanticEntityForInspection(ctx, q, claim.Object.EntityID)
		if err != nil {
			return err
		}
		object = entity.CanonicalName
	}
	parts := []string{subject.CanonicalName, claim.Predicate.Token, claim.Predicate.Label, object}
	rows, err := q.QueryContext(ctx, `SELECT a.value FROM semantic_aliases a
 WHERE a.entity_id IN (?,?) AND (SELECT state FROM semantic_state_events se WHERE se.object_kind='alias' AND se.object_id=a.alias_id ORDER BY transaction_time DESC,scope_revision DESC LIMIT 1)='active'
 ORDER BY a.alias_id LIMIT 32`, claim.SubjectEntityID, claim.Object.EntityID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			rows.Close()
			return err
		}
		parts = append(parts, value)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	body := strings.Join(parts, " ")
	if len(body) > 16384 || compilerHasSecret(body) {
		return nil
	}
	_, err = q.ExecContext(ctx, `INSERT INTO memory_retrieval_fts_v3(generation,claim_id,scope_key,body) VALUES(?,?,?,?)`, memoryIndexGeneration, id, claim.ScopeKey, body)
	if err != nil {
		return fmt.Errorf("refresh accepted memory projection: %w", err)
	}
	return nil
}
