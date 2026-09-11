package eviedb

import (
	"context"
	"encoding/binary"
	"math"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/localembedding"
	"github.com/davidadel66/evie/internal/memory"
)

const denseVectorScanLimit = 4096

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
	rows, err := c.tx.QueryContext(ctx, `SELECT claim_id,byte_start,byte_end,source_hash,revision_hash,content_hash,document_hash,vector FROM memory_dense_vectors v
 WHERE generation=? AND scope_key IN (?,?,?) AND NOT EXISTS(SELECT 1 FROM memory_dense_dirty d WHERE d.generation=v.generation AND d.claim_id=v.claim_id)
 ORDER BY claim_id LIMIT ?`, coverage.Generation, keys[0], keys[1], keys[2], denseVectorScanLimit+1)
	if err != nil {
		return err
	}
	type hit struct {
		id                                                  memory.SemanticID
		start, end                                          int
		sourceHash, revisionHash, contentHash, documentHash string
		score                                               float64
	}
	var hits []hit
	scanned := 0
	for rows.Next() {
		var item hit
		var encoded []byte
		if err := rows.Scan(&item.id, &item.start, &item.end, &item.sourceHash, &item.revisionHash, &item.contentHash, &item.documentHash, &encoded); err != nil {
			rows.Close()
			return err
		}
		if scanned == denseVectorScanLimit {
			c.truncated = true
			break
		}
		scanned++
		vector, ok := decodeDenseVector(encoded)
		if !ok {
			coverage.State = "unavailable"
			c.denseIncomplete = true
			continue
		}
		for i, value := range vector {
			item.score += float64(value) * float64(vectors[0][i])
		}
		if item.score >= 0.25 {
			hits = append(hits, item)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].id < hits[j].id
	})
	rank := 0
	ranked := map[memory.SemanticID]bool{}
	for _, item := range hits {
		if rank == 8 {
			break
		}
		if ranked[item.id] {
			continue
		}
		candidate, err := c.read(ctx, item.id)
		if err != nil {
			return err
		}
		if candidate == nil {
			continue
		}
		current, eligible, err := c.store.denseClaimDocument(ctx, c.tx, item.id)
		if err != nil {
			return err
		}
		if !eligible || current.sourceHash != item.sourceHash || current.revisionHash != item.revisionHash || current.contentHash != item.documentHash || item.start < 0 || item.end <= item.start || item.end > len(current.text) || memory.CompilerHash([]byte(current.text[item.start:item.end])) != item.contentHash {
			c.denseIncomplete = true
			continue
		}
		rank++
		ranked[item.id] = true
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
