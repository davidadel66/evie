package eviedb

import (
	"context"
	"database/sql"
	"encoding/binary"
	"math"
	"time"

	"github.com/davidadel66/evie/internal/localembedding"
	"github.com/davidadel66/evie/internal/memory"
)

// Dense recall pages through every vector of a generation in primary-key
// order and keeps a running top-k (harness review M7). The page bounds one
// read; the budget bounds one search's comparisons. A search that stops at the
// budget, or that runs within denseScanReserve of its deadline, reports
// memory.RetrievalGapDenseScan instead of presenting the cut as complete.
// Variables so tests can exercise the bounds at small sizes.
var denseVectorScanBatch, denseVectorScanBudget = 1024, 65536

// Leave a quarter of the shared query deadline for the authoritative re-reads
// and rendering that follow the scan.
const denseScanReserve = retrievalDeadline / 4

const denseMinimumCosine = 0.25

// Reserve half of the shared query deadline for authoritative reads and
// rendering. A stalled optional generator cannot consume the baseline's entire
// budget; caller cancellation still stops both paths.
func denseQueryContext(ctx context.Context) (context.Context, context.CancelFunc) {
	budget := retrievalDeadline / 2
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, time.Until(deadline)/2)
	}
	return context.WithTimeout(ctx, budget)
}

type denseScanHit struct {
	id                                                  string
	start, end                                          int
	sourceHash, revisionHash, contentHash, documentHash string
	score                                               float64
}

// before orders hits by score, then by key, so ties resolve as a full sort.
func (h denseScanHit) before(other denseScanHit) bool {
	if h.score != other.score {
		return h.score > other.score
	}
	if h.id != other.id {
		return h.id < other.id
	}
	if h.start != other.start {
		return h.start < other.start
	}
	return h.end < other.end
}

type denseScanResult struct {
	hits    []denseScanHit // best `keep` hits at or above the minimum cosine
	invalid bool           // a stored vector failed validation
	cut     bool           // the budget stopped the scan with vectors left
}

// denseScan runs page with an exclusive (id, byte_start, byte_end) cursor and
// a row limit until the table is exhausted or the budget is spent. page must
// select id, byte_start, byte_end, source_hash, revision_hash, content_hash,
// document_hash and vector, ordered by that key.
func denseScan(ctx context.Context, target []float32, keep int, page func(id string, start, end, limit int) (*sql.Rows, error)) (denseScanResult, error) {
	var result denseScanResult
	cursor := denseScanHit{start: -1, end: -1}
	scanned := 0
	for {
		limit := min(denseVectorScanBatch, denseVectorScanBudget-scanned)
		rows, err := page(cursor.id, cursor.start, cursor.end, limit+1)
		if err != nil {
			return result, err
		}
		read, more := 0, false
		for rows.Next() {
			if read == limit {
				more = true
				break
			}
			var item denseScanHit
			var encoded []byte
			if err := rows.Scan(&item.id, &item.start, &item.end, &item.sourceHash, &item.revisionHash, &item.contentHash, &item.documentHash, &encoded); err != nil {
				rows.Close()
				return result, err
			}
			read++
			cursor = item
			vector, ok := decodeDenseVector(encoded)
			if !ok {
				result.invalid = true
				continue
			}
			for i, value := range vector {
				item.score += float64(value) * float64(target[i])
			}
			if item.score >= denseMinimumCosine {
				result.hits = keepDenseHit(result.hits, item, keep)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return result, err
		}
		rows.Close()
		scanned += read
		if !more {
			return result, nil
		}
		deadline, bounded := ctx.Deadline()
		if scanned >= denseVectorScanBudget || bounded && time.Until(deadline) < denseScanReserve {
			result.cut = true
			return result, nil
		}
	}
}

func keepDenseHit(hits []denseScanHit, item denseScanHit, keep int) []denseScanHit {
	if keep <= 0 || len(hits) == keep && !item.before(hits[len(hits)-1]) {
		return hits
	}
	at := len(hits)
	for at > 0 && item.before(hits[at-1]) {
		at--
	}
	if len(hits) < keep {
		hits = append(hits, denseScanHit{})
	}
	copy(hits[at+1:], hits[at:])
	hits[at] = item
	return hits
}

func (c *retrievalCandidates) denseGenerator(ctx context.Context) error {
	prepared := c.densePreparation
	if prepared == nil {
		return nil
	}
	vectors, err := prepared.wait(ctx)
	c.denseCoverage = &prepared.coverage
	if err != nil {
		return err
	}
	coverage := &prepared.coverage
	c.denseIncomplete = coverage.State != "active" || coverage.Pending > 0
	if len(vectors) == 0 {
		return nil
	}
	keys := c.plan.scopes
	// Candidate reads stop at retrievalCandidateLimit distinct Claims; chunks
	// of one Claim share its ID, so keep a margin of hits beyond that.
	scan, err := denseScan(ctx, vectors[0], 4*retrievalCandidateLimit, func(id string, start, end, limit int) (*sql.Rows, error) {
		return c.tx.QueryContext(ctx, `SELECT claim_id,byte_start,byte_end,source_hash,revision_hash,content_hash,document_hash,vector FROM memory_dense_vectors v
 WHERE generation=? AND scope_key IN (?,?,?) AND NOT EXISTS(SELECT 1 FROM memory_dense_dirty d WHERE d.generation=v.generation AND d.claim_id=v.claim_id)
 AND (claim_id,byte_start,byte_end)>(?,?,?) ORDER BY claim_id,byte_start,byte_end LIMIT ?`, coverage.Generation, keys[0], keys[1], keys[2], id, start, end, limit)
	})
	if err != nil {
		return err
	}
	if scan.invalid {
		coverage.State = "unavailable"
		c.denseIncomplete = true
	}
	if scan.cut {
		c.truncated, c.denseIncomplete, c.denseCut = true, true, true
	}
	rank := 0
	ranked := map[memory.SemanticID]bool{}
	for _, item := range scan.hits {
		if rank == 8 {
			break
		}
		id := memory.SemanticID(item.id)
		if ranked[id] {
			continue
		}
		candidate, err := c.read(ctx, id)
		if err != nil {
			return err
		}
		if candidate == nil {
			continue
		}
		current, eligible, err := c.store.denseClaimDocument(ctx, c.tx, id)
		if err != nil {
			return err
		}
		if !eligible || current.sourceHash != item.sourceHash || current.revisionHash != item.revisionHash || current.contentHash != item.documentHash || item.start < 0 || item.end <= item.start || item.end > len(current.text) || memory.CompilerHash([]byte(current.text[item.start:item.end])) != item.contentHash {
			c.denseIncomplete = true
			continue
		}
		rank++
		ranked[id] = true
		candidate.direct = true
		candidate.evidence.RetrievalGeneration = coverage.Generation
		addRetrievalRank(candidate, "dense", rank)
	}
	return nil
}

func decodeDenseVector(data []byte) ([]float32, bool) {
	if len(data) != localembedding.Dimensions*4 {
		return nil, false
	}
	vector := make([]float32, localembedding.Dimensions)
	norm := 0.0
	for i := range vector {
		value := math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, false
		}
		vector[i] = value
		norm += float64(value) * float64(value)
	}
	return vector, norm > 0.999 && norm < 1.001
}
