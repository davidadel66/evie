package eviedb

import (
	"context"

	"github.com/davidadel66/evie/internal/localembedding"
	"github.com/davidadel66/evie/internal/memory"
)

type denseQueryOutcome struct {
	vectors [][]float32
	err     error
}

type denseQueryPreparation struct {
	coverage memory.RetrievalCoverage
	result   <-chan denseQueryOutcome
	cancel   context.CancelFunc
}

// Only HTTP inference runs concurrently. SQLite generators retain one ordered
// read transaction; the worker cannot access or mutate that transaction.
func prepareDenseQuery(ctx context.Context, q semanticInspectionQueryer, text string) (*denseQueryPreparation, error) {
	if denseEndpoint() == "" {
		return nil, nil
	}
	coverage, err := denseIndexCoverage(ctx, q)
	if err != nil {
		return nil, err
	}
	p := &denseQueryPreparation{coverage: coverage}
	if coverage.State != "active" {
		return p, nil
	}
	if memory.HasRetrievalSecret([]byte(text)) {
		p.coverage.State = "unavailable"
		return p, nil
	}
	client, err := localembedding.New(denseEndpoint())
	if err != nil {
		p.coverage.State = "unavailable"
		return p, nil
	}
	denseCtx, cancel := denseQueryContext(ctx)
	p.cancel = cancel
	result := make(chan denseQueryOutcome, 1)
	p.result = result
	go func() {
		defer cancel()
		defer client.Close()
		vectors, err := client.Embed(denseCtx, []string{text})
		result <- denseQueryOutcome{vectors, err}
	}()
	return p, nil
}

func (p *denseQueryPreparation) close() {
	if p != nil && p.cancel != nil {
		p.cancel()
	}
}

func (p *denseQueryPreparation) wait(ctx context.Context) ([][]float32, error) {
	if p.result == nil {
		return nil, nil
	}
	select {
	case result := <-p.result:
		if result.err != nil {
			p.coverage.State = "unavailable"
			return nil, ctx.Err()
		}
		return result.vectors, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
