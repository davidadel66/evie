package eviedb

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/localembedding"
	"github.com/davidadel66/evie/internal/memory"
)

const denseConfiguration = `{"version":"evidence-embedding-v1","model":"all-minilm:22m","manifest_sha256":"1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef","model_layer_sha256":"797b70c4edf85907fe0a49eb85811256f65fa0f7bf52166b147fd16be2be4662","dimensions":384,"dtype":"float32","distance":"cosine","min_cosine":0.25,"candidates":8,"batch_size":16,"truncate":false,"num_thread":4,"keep_alive":"30s","embedding_deadline_seconds":10,"projection_chunk_bytes":240,"projection_overlap_bytes":48}`

const denseChunkBytes, denseOverlapBytes = 240, 48

const denseRetrievalSchema = `
CREATE TABLE IF NOT EXISTS memory_dense_generations (
 generation TEXT PRIMARY KEY, configuration TEXT NOT NULL, configuration_hash TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('building','active')), claim_checkpoint INTEGER NOT NULL DEFAULT 0, event_checkpoint INTEGER NOT NULL DEFAULT 0
);
CREATE TRIGGER IF NOT EXISTS memory_dense_configuration_immutable
 BEFORE UPDATE OF generation,configuration,configuration_hash ON memory_dense_generations
 BEGIN SELECT RAISE(ABORT,'embedding configuration is immutable'); END;
CREATE TABLE IF NOT EXISTS memory_dense_selection (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), generation TEXT NOT NULL REFERENCES memory_dense_generations(generation), enabled INTEGER NOT NULL CHECK(enabled IN (0,1))
);
CREATE TABLE IF NOT EXISTS memory_dense_dirty (
 generation TEXT NOT NULL REFERENCES memory_dense_generations(generation), claim_id TEXT NOT NULL,
 PRIMARY KEY(generation,claim_id)
);
CREATE TABLE IF NOT EXISTS memory_dense_vectors (
 generation TEXT NOT NULL REFERENCES memory_dense_generations(generation), claim_id TEXT NOT NULL, scope_key TEXT NOT NULL,
 byte_start INTEGER NOT NULL,byte_end INTEGER NOT NULL,source_hash TEXT NOT NULL, revision_hash TEXT NOT NULL, content_hash TEXT NOT NULL,document_hash TEXT NOT NULL, vector BLOB NOT NULL,
 PRIMARY KEY(generation,claim_id,byte_start,byte_end)
);
CREATE TABLE IF NOT EXISTS memory_dense_event_dirty (
 generation TEXT NOT NULL REFERENCES memory_dense_generations(generation),event_id TEXT NOT NULL,PRIMARY KEY(generation,event_id)
);
CREATE TABLE IF NOT EXISTS memory_dense_event_vectors (
 generation TEXT NOT NULL REFERENCES memory_dense_generations(generation),event_id TEXT NOT NULL,scope_key TEXT NOT NULL,
 byte_start INTEGER NOT NULL,byte_end INTEGER NOT NULL,source_hash TEXT NOT NULL,revision_hash TEXT NOT NULL,content_hash TEXT NOT NULL,vector BLOB NOT NULL,
 PRIMARY KEY(generation,event_id,byte_start,byte_end)
);
CREATE TRIGGER IF NOT EXISTS memory_dense_event_added AFTER INSERT ON events BEGIN
 INSERT OR IGNORE INTO memory_dense_event_dirty SELECT generation,NEW.id FROM memory_dense_selection WHERE enabled=1;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_event_source_added AFTER INSERT ON semantic_source_links BEGIN
 INSERT OR IGNORE INTO memory_dense_event_dirty SELECT generation,NEW.event_id FROM memory_dense_selection WHERE enabled=1;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_event_state_added AFTER INSERT ON semantic_state_events BEGIN
 INSERT OR IGNORE INTO memory_dense_event_dirty SELECT d.generation,sl.event_id FROM memory_dense_selection d JOIN semantic_source_links sl
 WHERE d.enabled=1 AND ((NEW.object_kind='claim' AND sl.claim_id=NEW.object_id) OR (NEW.object_kind='source_link' AND sl.source_link_id=NEW.object_id));
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_event_workspace_changed AFTER UPDATE OF lifecycle_state ON workspaces BEGIN
 INSERT OR IGNORE INTO memory_dense_event_dirty SELECT d.generation,e.id FROM memory_dense_selection d JOIN events e WHERE d.enabled=1 AND e.workspace_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_event_project_changed AFTER UPDATE OF archived ON projects BEGIN
 INSERT OR IGNORE INTO memory_dense_event_dirty SELECT d.generation,e.id FROM memory_dense_selection d JOIN events e WHERE d.enabled=1 AND e.project_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_claim_added AFTER INSERT ON semantic_claims BEGIN
 INSERT OR IGNORE INTO memory_dense_dirty SELECT generation,NEW.claim_id FROM memory_dense_selection WHERE enabled=1;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_source_added AFTER INSERT ON semantic_source_links BEGIN
 INSERT OR IGNORE INTO memory_dense_dirty SELECT generation,NEW.claim_id FROM memory_dense_selection WHERE enabled=1;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_state_added AFTER INSERT ON semantic_state_events BEGIN
 INSERT OR IGNORE INTO memory_dense_dirty SELECT s.generation,c.claim_id FROM memory_dense_selection s JOIN semantic_claims c
 WHERE s.enabled=1 AND ((NEW.object_kind='claim' AND c.claim_id=NEW.object_id)
 OR (NEW.object_kind='entity' AND (c.subject_entity_id=NEW.object_id OR c.object_entity_id=NEW.object_id))
 OR (NEW.object_kind='source_link' AND c.claim_id=(SELECT claim_id FROM semantic_source_links WHERE source_link_id=NEW.object_id)));
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_workspace_changed AFTER UPDATE OF lifecycle_state ON workspaces BEGIN
 INSERT OR IGNORE INTO memory_dense_dirty SELECT d.generation,sl.claim_id FROM memory_dense_selection d JOIN semantic_source_links sl
 JOIN sessions s ON s.id=sl.source_session_id WHERE d.enabled=1 AND s.workspace_id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS memory_dense_project_changed AFTER UPDATE OF archived ON projects BEGIN
 INSERT OR IGNORE INTO memory_dense_dirty SELECT d.generation,sl.claim_id FROM memory_dense_selection d JOIN semantic_source_links sl
 JOIN sessions s ON s.id=sl.source_session_id WHERE d.enabled=1 AND s.project_id=NEW.id;
END;
`

func denseEndpoint() string {
	if os.Getenv("EVIE_REMOTE_MEMORY") != "on" {
		return ""
	}
	return strings.TrimSpace(os.Getenv("EVIE_MEMORY_EMBEDDING_ENDPOINT"))
}

func denseIndexCoverage(ctx context.Context, q semanticInspectionQueryer) (memory.RetrievalCoverage, error) {
	coverage := memory.RetrievalCoverage{State: "disabled"}
	if denseEndpoint() == "" {
		return coverage, nil
	}
	err := q.QueryRowContext(ctx, `SELECT g.generation,CASE WHEN s.enabled=1 AND g.configuration=? AND g.configuration_hash=? THEN g.state ELSE 'building' END,
 (SELECT count(*) FROM semantic_claims c WHERE c.rowid>g.claim_checkpoint)
 +(SELECT count(*) FROM memory_dense_dirty d WHERE d.generation=g.generation)
 +(SELECT count(*) FROM events e WHERE e.rowid>g.event_checkpoint)
 +(SELECT count(*) FROM memory_dense_event_dirty d WHERE d.generation=g.generation),
 (SELECT count(*) FROM memory_dense_vectors v WHERE v.generation=g.generation)
 +(SELECT count(*) FROM memory_dense_event_vectors v WHERE v.generation=g.generation)
 FROM memory_dense_selection s JOIN memory_dense_generations g ON g.generation=s.generation WHERE s.singleton=1`, denseConfiguration, memory.CompilerHash([]byte(denseConfiguration))).
		Scan(&coverage.Generation, &coverage.State, &coverage.Pending, &coverage.Indexed)
	if errors.Is(err, sql.ErrNoRows) {
		coverage.State = "building"
		return coverage, nil
	}
	return coverage, err
}

func (s *Store) ensureDenseGeneration(ctx context.Context) (memory.RetrievalCoverage, error) {
	if denseEndpoint() == "" {
		_, err := s.db.ExecContext(ctx, `UPDATE memory_dense_selection SET enabled=0 WHERE enabled=1`)
		return memory.RetrievalCoverage{State: "disabled"}, err
	}
	client, err := localembedding.New(denseEndpoint())
	if err != nil {
		return memory.RetrievalCoverage{State: "unavailable"}, err
	}
	client.Close()
	err = s.withImmediateTransaction(ctx, func(q *sql.Conn) error {
		var id, configuration, configurationHash string
		var enabled int
		err := q.QueryRowContext(ctx, `SELECT g.generation,g.configuration,g.configuration_hash,s.enabled FROM memory_dense_selection s JOIN memory_dense_generations g ON g.generation=s.generation WHERE s.singleton=1`).Scan(&id, &configuration, &configurationHash, &enabled)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if configuration != denseConfiguration || configurationHash != memory.CompilerHash([]byte(denseConfiguration)) {
			generation, err := newSemanticID()
			if err != nil {
				return err
			}
			id = "dense:" + string(generation)
			if _, err := q.ExecContext(ctx, `INSERT INTO memory_dense_generations(generation,configuration,configuration_hash,state) VALUES(?,?,?,'building')`, id, denseConfiguration, memory.CompilerHash([]byte(denseConfiguration))); err != nil {
				return err
			}
			_, err = q.ExecContext(ctx, `INSERT INTO memory_dense_selection(singleton,generation,enabled) VALUES(1,?,1) ON CONFLICT(singleton) DO UPDATE SET generation=excluded.generation,enabled=1`, id)
			return err
		}
		if enabled == 0 {
			// Disabled generations do not accumulate work. Re-enabling requires
			// a full retained-state reconciliation before they can serve again.
			if _, err := q.ExecContext(ctx, `UPDATE memory_dense_generations SET state='building',claim_checkpoint=0,event_checkpoint=0 WHERE generation=?`, id); err != nil {
				return err
			}
			_, err = q.ExecContext(ctx, `UPDATE memory_dense_selection SET enabled=1 WHERE singleton=1`)
			return err
		}
		return nil
	})
	if err != nil {
		return memory.RetrievalCoverage{}, err
	}
	return denseIndexCoverage(ctx, s.db)
}

// RebuildMemoryEmbeddings selects a new immutable local projection. It performs
// no inference or retained-state backfill; bounded maintenance must reconcile
// the replacement before any dense query can use it.
func (s *Store) RebuildMemoryEmbeddings(ctx context.Context) (memory.RetrievalCoverage, error) {
	if denseEndpoint() == "" {
		return s.ensureDenseGeneration(ctx)
	}
	client, err := localembedding.New(denseEndpoint())
	if err != nil {
		return memory.RetrievalCoverage{State: "unavailable"}, err
	}
	client.Close()
	err = s.withImmediateTransaction(ctx, func(q *sql.Conn) error {
		if denseEndpoint() == "" {
			return localembedding.ErrDisabled
		}
		generation, err := newSemanticID()
		if err != nil {
			return err
		}
		id := "dense:" + string(generation)
		if _, err := q.ExecContext(ctx, `INSERT INTO memory_dense_generations(generation,configuration,configuration_hash,state) VALUES(?,?,?,'building')`, id, denseConfiguration, memory.CompilerHash([]byte(denseConfiguration))); err != nil {
			return err
		}
		_, err = q.ExecContext(ctx, `INSERT INTO memory_dense_selection(singleton,generation,enabled) VALUES(1,?,1) ON CONFLICT(singleton) DO UPDATE SET generation=excluded.generation,enabled=1`, id)
		return err
	})
	if err != nil {
		return memory.RetrievalCoverage{State: "unavailable"}, err
	}
	return denseIndexCoverage(ctx, s.db)
}

type denseClaimDocument struct {
	id                                                 memory.SemanticID
	scope, text, sourceHash, revisionHash, contentHash string
}

func (s *Store) denseClaimDocument(ctx context.Context, q semanticInspectionQueryer, id memory.SemanticID) (denseClaimDocument, bool, error) {
	doc := denseClaimDocument{id: id}
	claim, err := loadSemanticClaim(ctx, q, id)
	if errors.Is(err, sql.ErrNoRows) {
		return doc, false, nil
	}
	if err != nil {
		return doc, false, err
	}
	if err := requireSemanticScopeKeysAvailable(ctx, q, []string{claim.ScopeKey}); err != nil {
		if errors.Is(err, ErrSemanticScopeQuarantined) {
			return doc, false, nil
		}
		return doc, false, err
	}
	valid, known, err := s.semanticQueryTimes(ctx, q, memory.ClaimQuery{})
	if err != nil {
		return doc, false, err
	}
	evidence, eligible, err := s.retrievalClaim(ctx, q, memory.ExactReadMetadata{ValidAt: valid, AsKnownAt: known, AllowedScopes: []string{claim.ScopeKey}}, id, memory.RetrievalHistorical, false)
	if err != nil || !eligible {
		return doc, false, err
	}
	sources, _ := json.Marshal(evidence.Reference().Sources)
	revision, _ := json.Marshal(struct {
		Claim                         memory.SemanticClaim
		Status, CurrentStatus         memory.SemanticObjectStatus
		Correction, CurrentCorrection memory.CorrectionMode
		Effective                     *memory.ValidTime
	}{*evidence.Claim, evidence.Status, evidence.CurrentStatus, evidence.CorrectionMode, evidence.CurrentCorrectionMode, evidence.EffectiveValidTime})
	doc.scope, doc.text = claim.ScopeKey, evidence.Text
	doc.sourceHash, doc.revisionHash, doc.contentHash = memory.CompilerHash(sources), memory.CompilerHash(revision), memory.CompilerHash([]byte(doc.text))
	return doc, true, nil
}

func denseTextSpans(text string) []evidenceSpan {
	var spans []evidenceSpan
	for start := 0; start < len(text); {
		end := min(len(text), start+denseChunkBytes)
		for end > start && !utf8.ValidString(text[start:end]) {
			end--
		}
		spans = append(spans, evidenceSpan{start, end})
		if end == len(text) {
			break
		}
		start = end - denseOverlapBytes
		for !utf8.RuneStart(text[start]) {
			start++
		}
	}
	return spans
}

func (s *Store) refreshDenseIndex(ctx context.Context, limit int) (memory.RetrievalCoverage, error) {
	coverage, err := s.ensureDenseGeneration(ctx)
	if err != nil || coverage.State == "disabled" || coverage.State == "active" && coverage.Pending == 0 {
		return coverage, err
	}
	limit = min(limit, localembedding.BatchSize)
	if limit < 1 {
		return coverage, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return coverage, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT c.rowid,c.claim_id FROM semantic_claims c
 WHERE c.rowid>(SELECT claim_checkpoint FROM memory_dense_generations WHERE generation=?)
 ORDER BY c.rowid LIMIT ?`, coverage.Generation, limit)
	if err != nil {
		return coverage, err
	}
	type entry struct {
		row      int64
		id       memory.SemanticID
		doc      denseClaimDocument
		eligible bool
		vectors  [][]float32
		missing  []evidenceSpan
	}
	var entries []entry
	for rows.Next() {
		var item entry
		if err := rows.Scan(&item.row, &item.id); err != nil {
			rows.Close()
			return coverage, err
		}
		entries = append(entries, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return coverage, err
	}
	rows.Close()
	if len(entries) == 0 {
		rows, err = tx.QueryContext(ctx, `SELECT 0,claim_id FROM memory_dense_dirty WHERE generation=? ORDER BY claim_id LIMIT ?`, coverage.Generation, limit)
		if err != nil {
			return coverage, err
		}
		for rows.Next() {
			var item entry
			if err := rows.Scan(&item.row, &item.id); err != nil {
				rows.Close()
				return coverage, err
			}
			entries = append(entries, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return coverage, err
		}
		rows.Close()
	}
	var inputs []string
	for i := range entries {
		entries[i].doc, entries[i].eligible, err = s.denseClaimDocument(ctx, tx, entries[i].id)
		if err != nil {
			return coverage, err
		}
		if entries[i].eligible {
			doc := entries[i].doc
			for _, span := range denseTextSpans(entries[i].doc.text) {
				if len(inputs) == localembedding.BatchSize {
					break
				}
				var present int
				if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM memory_dense_vectors WHERE generation=? AND claim_id=? AND byte_start=? AND byte_end=? AND source_hash=? AND revision_hash=? AND document_hash=? AND content_hash=?`, coverage.Generation, doc.id, span.start, span.end, doc.sourceHash, doc.revisionHash, doc.contentHash, memory.CompilerHash([]byte(doc.text[span.start:span.end]))).Scan(&present); err != nil {
					return coverage, err
				}
				if present > 0 {
					continue
				}
				entries[i].missing = append(entries[i].missing, span)
				inputs = append(inputs, entries[i].doc.text[span.start:span.end])
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return coverage, err
	}
	var vectors [][]float32
	if len(inputs) > 0 {
		client, err := localembedding.New(denseEndpoint())
		if err != nil {
			return coverage, err
		}
		defer client.Close()
		for start := 0; start < len(inputs); start += localembedding.BatchSize {
			batch, err := client.Embed(ctx, inputs[start:min(len(inputs), start+localembedding.BatchSize)])
			if err != nil {
				return coverage, err
			}
			vectors = append(vectors, batch...)
		}
	}
	offset := 0
	for i := range entries {
		if entries[i].eligible {
			count := len(entries[i].missing)
			entries[i].vectors = vectors[offset : offset+count]
			offset += count
		}
	}
	err = s.withImmediateTransaction(ctx, func(q *sql.Conn) error {
		var selected string
		if err := q.QueryRowContext(ctx, `SELECT generation FROM memory_dense_selection WHERE singleton=1 AND enabled=1`).Scan(&selected); err != nil {
			return err
		}
		if selected != coverage.Generation || denseEndpoint() == "" {
			return localembedding.ErrDisabled
		}
		for _, item := range entries {
			current, eligible, err := s.denseClaimDocument(ctx, q, item.id)
			if err != nil {
				return err
			}
			if eligible != item.eligible || eligible && (current.sourceHash != item.doc.sourceHash || current.revisionHash != item.doc.revisionHash || current.contentHash != item.doc.contentHash) {
				if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO memory_dense_dirty(generation,claim_id) VALUES(?,?)`, coverage.Generation, item.id); err != nil {
					return err
				}
			} else {
				if _, err := q.ExecContext(ctx, `DELETE FROM memory_dense_vectors WHERE generation=? AND claim_id=? AND (?=0 OR source_hash!=? OR revision_hash!=? OR document_hash!=?)`, coverage.Generation, item.id, eligible, current.sourceHash, current.revisionHash, current.contentHash); err != nil {
					return err
				}
				if eligible {
					for i, span := range item.missing {
						if _, err := q.ExecContext(ctx, `INSERT OR REPLACE INTO memory_dense_vectors(generation,claim_id,scope_key,byte_start,byte_end,source_hash,revision_hash,content_hash,document_hash,vector) VALUES(?,?,?,?,?,?,?,?,?,?)`, coverage.Generation, item.id, current.scope, span.start, span.end, current.sourceHash, current.revisionHash, memory.CompilerHash([]byte(current.text[span.start:span.end])), current.contentHash, encodeDenseVector(item.vectors[i])); err != nil {
							return err
						}
					}
				}
				var indexed int
				if err := q.QueryRowContext(ctx, `SELECT count(*) FROM memory_dense_vectors WHERE generation=? AND claim_id=?`, coverage.Generation, item.id).Scan(&indexed); err != nil {
					return err
				}
				if !eligible || indexed == len(denseTextSpans(current.text)) {
					if _, err := q.ExecContext(ctx, `DELETE FROM memory_dense_dirty WHERE generation=? AND claim_id=?`, coverage.Generation, item.id); err != nil {
						return err
					}
				} else {
					if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO memory_dense_dirty(generation,claim_id) VALUES(?,?)`, coverage.Generation, item.id); err != nil {
						return err
					}
				}
			}
			if _, err := q.ExecContext(ctx, `UPDATE memory_dense_generations SET claim_checkpoint=MAX(claim_checkpoint,?) WHERE generation=?`, item.row, coverage.Generation); err != nil {
				return err
			}
		}
		coverage, err = denseIndexCoverage(ctx, q)
		if err != nil {
			return err
		}
		if coverage.Pending == 0 {
			_, err = q.ExecContext(ctx, `UPDATE memory_dense_generations SET state='active' WHERE generation=?`, coverage.Generation)
			coverage.State = "active"
		}
		return err
	})
	if err == nil && limit > len(entries) {
		return s.refreshDenseEvents(ctx, coverage, limit-len(entries), localembedding.BatchSize-len(inputs))
	}
	return coverage, err
}

func encodeDenseVector(vector []float32) []byte {
	data := make([]byte, 4*len(vector))
	for i, value := range vector {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}
