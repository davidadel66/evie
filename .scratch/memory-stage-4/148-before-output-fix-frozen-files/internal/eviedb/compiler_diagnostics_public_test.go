package eviedb_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerDiagnosticsPartialBatchOutcomesAndExactRetry(t *testing.T) {
	ctx := context.Background()
	f := newCompilerFixture(t)
	refs := compileBatchCandidates(t, f, "coffee", "juice")
	a := temporalAuthority(t, f)
	read := func() memory.CompilerDiagnostics {
		t.Helper()
		r, e := f.store.InspectOwnerCompilerDiagnostics(ctx, a, memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "candidates"})
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	before := read()
	if before.Counts["candidates_unresolved"] != 2 || len(before.Candidates) != 2 {
		t.Fatalf("backlog %+v", before)
	}
	p, err := f.store.PrepareOwnerCandidateBatch(ctx, a, independentBatch(refs))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(fmt.Sprintf(`CREATE TRIGGER diagnostics_fail BEFORE INSERT ON semantic_claims WHEN NEW.claim_id='%s' BEGIN SELECT RAISE(ABORT,'bounded group failure'); END`, p.Groups[0].Preview.Effect.Claims[0].Claim.ID)); err != nil {
		t.Fatal(err)
	}
	d := batchDecision(p, "90000000-0000-4000-8000-000000001148")
	r, err := f.store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err != nil || r.Groups[0].Outcome != "failed" || r.Groups[1].Outcome != "accepted" {
		t.Fatalf("partial %+v %v", r, err)
	}
	after := read()
	if after.Counts["candidates_unresolved"] != 1 || after.Counts["candidates_accepted"] != 1 {
		t.Fatalf("counters %+v", after)
	}
	for _, c := range after.Candidates {
		if c.Ref.ID == refs[0].ID && c.DecidedAtUnixMS != nil {
			t.Fatal("failed group gained decision timestamp")
		}
		if c.Ref.ID == refs[1].ID && (c.DecidedAtUnixMS == nil || c.ReviewState != "accepted") {
			t.Fatalf("accepted %+v", c)
		}
	}
	if _, err = f.store.ResolveOwnerCandidateBatch(ctx, a, d); err != nil {
		t.Fatal(err)
	}
	again := read()
	if again.Revision != after.Revision {
		t.Fatal("delivery retry counted another outcome")
	}
	if _, err = f.db.Exec(`DROP TRIGGER diagnostics_fail`); err != nil {
		t.Fatal(err)
	}
	reject, err := f.store.PrepareOwnerCandidateReview(ctx, a, refs[0], "reject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ResolveOwnerCandidateReview(ctx, a, decisionFor(reject, "90000000-0000-4000-8000-000000002148")); err != nil {
		t.Fatal(err)
	}
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	a, err = f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	final := read()
	if final.Counts["candidates_unresolved"] != 0 || final.Counts["candidates_accepted"] != 1 || final.Counts["candidates_rejected"] != 1 {
		t.Fatalf("reopen %+v", final)
	}
}
