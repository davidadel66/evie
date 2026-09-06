package web_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

type uncertainReviewKernel struct {
	web.CandidateReviewKernel
	failed bool
}

func (k *uncertainReviewKernel) ResolveOwnerCandidateReview(ctx context.Context, owner eviedb.OwnerReviewContext, decision memory.ReviewDecision) (memory.ReviewResult, error) {
	result, err := k.CandidateReviewKernel.ResolveOwnerCandidateReview(ctx, owner, decision)
	if err == nil && !k.failed {
		k.failed = true
		return memory.ReviewResult{}, errors.New("injected commit confirmation failure with protected details")
	}
	return result, err
}

func TestCandidateReviewHTTPUnknownCommitOutcomeRetainsExactRecovery(t *testing.T) {
	f := newWebReviewFixture(t)
	p := webPrepare(t, f, "accept")
	f.handler = web.WithCandidateReview(web.NewServer(nil), &uncertainReviewKernel{CandidateReviewKernel: f.store}).Handler()
	body := map[string]any{"scope_key": "global", "decision": webDecision(p)}
	first := reviewResponse[struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}](t, reviewPost(t, f.handler, "resolve", body), http.StatusServiceUnavailable)
	if first.Code != "review_retryable" || first.Error == "" {
		t.Fatalf("unknown outcome: %+v", first)
	}
	result := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", body), http.StatusOK)
	if result.Operation == nil {
		t.Fatal("recovery lost accepted operation")
	}
	retry := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", body), http.StatusOK)
	if !reflect.DeepEqual(result, retry) {
		t.Fatal("repeated recovery changed durable result")
	}
	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("claims=%d err=%v", count, err)
	}
	verified, err := f.store.VerifySemanticProjection(context.Background())
	if err != nil || !verified.Valid {
		t.Fatalf("replay: %+v %v", verified, err)
	}
}
