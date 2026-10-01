package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/localembedding"
	"github.com/davidadel66/evie/internal/memory"
)

type denseEventDocument struct {
	start, end                                  int
	text, sourceHash, revisionHash, contentHash string
}

func (s *Store) denseEventDocuments(ctx context.Context, q semanticInspectionQueryer, id memory.EventID) (conversationEvidence, []denseEventDocument, error) {
	e, eligible, err := loadConversationEvidence(ctx, q, id)
	if err != nil || !eligible {
		return e, nil, err
	}
	spans, err := conversationReadSpans(ctx, q, e, memory.RetrievalHistorical, s.now().UTC())
	if err != nil {
		return e, nil, err
	}
	var documents []denseEventDocument
	for _, span := range spans {
		for _, chunk := range denseTextSpans(e.content[span.start:span.end]) {
			start, end := span.start+chunk.start, span.start+chunk.end
			part := span
			part.evidenceSpan = evidenceSpan{start, end}
			evidence := conversationTypedExcerpt(e, part, time.Time{}, time.Time{}, memory.RetrievalHistorical)
			source, _ := json.Marshal(evidence.Reference().Sources)
			revision, _ := json.Marshal(struct{ Status, CurrentStatus memory.SemanticObjectStatus }{span.status, span.currentStatus})
			documents = append(documents, denseEventDocument{start, end, evidence.Text, memory.CompilerHash(source), memory.CompilerHash(revision), memory.CompilerHash([]byte(evidence.Text))})
		}
	}
	return e, documents, nil
}

func denseEventDocumentsEqual(left, right []denseEventDocument) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *Store) refreshDenseEvents(ctx context.Context, coverage memory.RetrievalCoverage, limit, embeddingBudget int) (memory.RetrievalCoverage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return coverage, err
	}
	defer tx.Rollback()
	type entry struct {
		row       int64
		id        memory.EventID
		documents []denseEventDocument
		vectors   [][]float32
		missing   []int
	}
	rows, err := tx.QueryContext(ctx, `SELECT rowid,id FROM events WHERE rowid>(SELECT event_checkpoint FROM memory_dense_generations WHERE generation=?) ORDER BY rowid LIMIT ?`, coverage.Generation, limit)
	if err != nil {
		return coverage, err
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
		rows, err = tx.QueryContext(ctx, `SELECT 0,event_id FROM memory_dense_event_dirty WHERE generation=? ORDER BY event_id LIMIT ?`, coverage.Generation, limit)
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
	var texts []string
	for i := range entries {
		_, entries[i].documents, err = s.denseEventDocuments(ctx, tx, entries[i].id)
		if err != nil {
			return coverage, err
		}
		for j, document := range entries[i].documents {
			if len(texts) >= embeddingBudget {
				break
			}
			var present int
			if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM memory_dense_event_vectors WHERE generation=? AND event_id=? AND byte_start=? AND byte_end=? AND source_hash=? AND revision_hash=? AND content_hash=?`, coverage.Generation, entries[i].id, document.start, document.end, document.sourceHash, document.revisionHash, document.contentHash).Scan(&present); err != nil {
				return coverage, err
			}
			if present > 0 {
				continue
			}
			entries[i].missing = append(entries[i].missing, j)
			texts = append(texts, document.text)
		}
	}
	if err := tx.Commit(); err != nil {
		return coverage, err
	}
	var vectors [][]float32
	if len(texts) > 0 {
		client, err := localembedding.New(denseEndpoint())
		if err != nil {
			return coverage, err
		}
		defer client.Close()
		for start := 0; start < len(texts); start += localembedding.BatchSize {
			batch, err := client.Embed(ctx, texts[start:min(len(texts), start+localembedding.BatchSize)])
			if err != nil {
				return coverage, err
			}
			vectors = append(vectors, batch...)
		}
	}
	offset := 0
	for i := range entries {
		count := len(entries[i].missing)
		entries[i].vectors = vectors[offset : offset+count]
		offset += count
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
			e, current, err := s.denseEventDocuments(ctx, q, item.id)
			if err != nil {
				return err
			}
			if !denseEventDocumentsEqual(current, item.documents) {
				if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO memory_dense_event_dirty(generation,event_id) VALUES(?,?)`, coverage.Generation, item.id); err != nil {
					return err
				}
			} else {
				if err := pruneDenseEventParts(ctx, q, coverage.Generation, item.id, current); err != nil {
					return err
				}
				for i, index := range item.missing {
					document := current[index]
					if _, err := q.ExecContext(ctx, `INSERT OR REPLACE INTO memory_dense_event_vectors(generation,event_id,scope_key,byte_start,byte_end,source_hash,revision_hash,content_hash,vector) VALUES(?,?,?,?,?,?,?,?,?)`, coverage.Generation, item.id, e.scope, document.start, document.end, document.sourceHash, document.revisionHash, document.contentHash, encodeDenseVector(item.vectors[i])); err != nil {
						return err
					}
				}
				var indexed int
				if err := q.QueryRowContext(ctx, `SELECT count(*) FROM memory_dense_event_vectors WHERE generation=? AND event_id=?`, coverage.Generation, item.id).Scan(&indexed); err != nil {
					return err
				}
				if indexed == len(current) {
					if _, err := q.ExecContext(ctx, `DELETE FROM memory_dense_event_dirty WHERE generation=? AND event_id=?`, coverage.Generation, item.id); err != nil {
						return err
					}
				} else {
					if _, err := q.ExecContext(ctx, `INSERT OR IGNORE INTO memory_dense_event_dirty(generation,event_id) VALUES(?,?)`, coverage.Generation, item.id); err != nil {
						return err
					}
				}
			}
			if _, err := q.ExecContext(ctx, `UPDATE memory_dense_generations SET event_checkpoint=MAX(event_checkpoint,?) WHERE generation=?`, item.row, coverage.Generation); err != nil {
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
	return coverage, err
}

func pruneDenseEventParts(ctx context.Context, q *sql.Conn, generation string, id memory.EventID, current []denseEventDocument) error {
	wanted := make(map[evidenceSpan]denseEventDocument, len(current))
	for _, document := range current {
		wanted[evidenceSpan{document.start, document.end}] = document
	}
	rows, err := q.QueryContext(ctx, `SELECT byte_start,byte_end,source_hash,revision_hash,content_hash FROM memory_dense_event_vectors WHERE generation=? AND event_id=?`, generation, id)
	if err != nil {
		return err
	}
	var stale []evidenceSpan
	for rows.Next() {
		var part denseEventDocument
		if err := rows.Scan(&part.start, &part.end, &part.sourceHash, &part.revisionHash, &part.contentHash); err != nil {
			rows.Close()
			return err
		}
		key := evidenceSpan{part.start, part.end}
		if want, ok := wanted[key]; !ok || want.sourceHash != part.sourceHash || want.revisionHash != part.revisionHash || want.contentHash != part.contentHash {
			stale = append(stale, key)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, span := range stale {
		if _, err := q.ExecContext(ctx, `DELETE FROM memory_dense_event_vectors WHERE generation=? AND event_id=? AND byte_start=? AND byte_end=?`, generation, id, span.start, span.end); err != nil {
			return err
		}
	}
	return nil
}

type denseConversationResult struct {
	evidence              []memory.RetrievalEvidence
	coverage              *memory.RetrievalCoverage
	incomplete, truncated bool
	cut                   bool // the scan budget left vectors uncompared
}

func (s *Store) denseConversationCandidates(ctx context.Context, tx *sql.Tx, scope memory.ScopeContext, query memory.RetrievalQuery, known time.Time, remaining int, prepared *denseQueryPreparation, cutoff int64) (denseConversationResult, error) {
	if prepared == nil {
		return denseConversationResult{}, nil
	}
	vectors, err := prepared.wait(ctx)
	coverage := prepared.coverage
	result := denseConversationResult{coverage: &coverage, incomplete: true}
	if err != nil {
		return result, err
	}
	if len(vectors) == 0 {
		return result, nil
	}
	if remaining <= 0 {
		result.incomplete, result.truncated = coverage.Pending > 0, true
		return result, nil
	}
	// Keep one hit beyond the remaining candidate work so exhausting it is
	// still reported as truncation.
	scan, err := denseScan(ctx, vectors[0], remaining+1, func(id string, start, end, limit int) (*sql.Rows, error) {
		return tx.QueryContext(ctx, `SELECT v.event_id,v.byte_start,v.byte_end,v.source_hash,v.revision_hash,v.content_hash,'',v.vector FROM memory_dense_event_vectors v JOIN events e ON e.id=v.event_id
 WHERE v.generation=? AND v.scope_key=? AND NOT EXISTS(SELECT 1 FROM memory_dense_event_dirty d WHERE d.generation=v.generation AND d.event_id=v.event_id)
 AND `+conversationObservedTimeSQL+`<=?
 AND (e.session_id!=? OR e.sequence<?)
 AND (?=0 OR e.content!=COALESCE((SELECT content FROM events WHERE session_id=? AND event_type='user_message' ORDER BY sequence DESC LIMIT 1),''))
 AND (v.event_id,v.byte_start,v.byte_end)>(?,?,?)
 ORDER BY v.event_id,v.byte_start,v.byte_end LIMIT ?`, coverage.Generation, scopeKeyForContext(scope), formatSemanticTime(known), scope.SessionID, cutoff, query.ExcludeCurrentRequestCopies, scope.SessionID, id, start, end, limit)
	})
	if err != nil {
		return result, err
	}
	result.incomplete = coverage.Pending > 0
	if scan.invalid {
		result.incomplete = true
		coverage.State = "unavailable"
	}
	if scan.cut {
		result.incomplete, result.truncated, result.cut = true, true, true
	}
	for _, item := range scan.hits {
		if len(result.evidence) == 8 {
			break
		}
		if remaining <= 0 {
			result.truncated = true
			break
		}
		remaining--
		e, documents, err := s.denseEventDocuments(ctx, tx, memory.EventID(item.id))
		if errors.Is(err, ErrConversationAssociation) {
			result.incomplete = true
			continue
		}
		if err != nil {
			result.incomplete = true
			return result, err
		}
		var current *denseEventDocument
		for i := range documents {
			doc := &documents[i]
			if doc.start == item.start && doc.end == item.end && doc.sourceHash == item.sourceHash && doc.revisionHash == item.revisionHash && doc.contentHash == item.contentHash {
				current = doc
				break
			}
		}
		if current == nil {
			result.incomplete = true
			continue
		}
		candidate := conversationTypedExcerpt(e, conversationReadSpan{evidenceSpan{current.start, current.end}, memory.SemanticStatusActive, memory.SemanticStatusActive}, known, known, query.Intent)
		resolved, eligible, err := s.resolveConversationReference(ctx, tx, scope, candidate.Reference())
		if err != nil {
			result.incomplete = true
			return result, err
		}
		if !eligible {
			continue
		}
		resolved.Paths = []string{"conversation_dense"}
		resolved.RetrievalGeneration = coverage.Generation
		result.evidence = append(result.evidence, resolved)
		result.truncated = result.truncated || current.start > 0 || current.end < len(e.content)
	}
	return result, nil
}

func fuseConversationEvidence(lexical, dense []memory.RetrievalEvidence, limit int) ([]memory.RetrievalEvidence, bool) {
	type item struct {
		evidence memory.RetrievalEvidence
		score    float64
	}
	items := make(map[string]*item)
	for _, list := range [][]memory.RetrievalEvidence{lexical, dense} {
		for rank, evidence := range list {
			candidate := items[evidence.ID]
			if candidate == nil {
				candidate = &item{evidence: evidence}
				items[evidence.ID] = candidate
			}
			candidate.score += 1 / float64(61+rank)
			for _, path := range evidence.Paths {
				if !containsString(candidate.evidence.Paths, path) {
					candidate.evidence.Paths = append(candidate.evidence.Paths, path)
				}
			}
			if evidence.RetrievalGeneration != "" {
				candidate.evidence.RetrievalGeneration = evidence.RetrievalGeneration
			}
		}
	}
	var ordered []*item
	for _, candidate := range items {
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return ordered[i].evidence.ID < ordered[j].evidence.ID
	})
	var result []memory.RetrievalEvidence
	for _, candidate := range ordered[:min(limit, len(ordered))] {
		result = append(result, candidate.evidence)
	}
	return result, len(ordered) > limit
}
