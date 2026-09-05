package eviedb_test

import (
	"context"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestOwnerReviewTerminalResultPrecedesFreshAcceptanceChecks(t *testing.T) {
	for _, action := range []string{"accept", "reject"} {
		for _, change := range []string{"policy", "source", "authorization_revision"} {
			t.Run(action+"/"+change, func(t *testing.T) {
				ctx := context.Background()
				f, compiled, authority := reviewCandidateFixture(t)
				preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), action)
				if err != nil {
					t.Fatal(err)
				}
				winning, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "90000000-0000-4000-8000-000000000150"))
				if err != nil {
					t.Fatal(err)
				}
				counts := func() [4]int {
					t.Helper()
					var got [4]int
					if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_audits), (SELECT count(*) FROM memory_review_deliveries), (SELECT count(*) FROM semantic_operations), (SELECT review_revision FROM memory_compiler_candidates WHERE candidate_id=?)`, compiled.Candidates[0].ID).Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
						t.Fatal(err)
					}
					return got
				}
				before := counts()
				switch change {
				case "policy":
					_, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-detector-v2'`)
				case "source":
					// Simulate damaged retained evidence without weakening the
					// production append-only boundary.
					if _, err = f.db.Exec(`DROP TRIGGER events_append_only_update`); err != nil {
						t.Fatal(err)
					}
					_, err = f.db.Exec(`UPDATE events SET content='api_key=synthetic_secret_not_a_real_key' WHERE id=?`, compiled.Candidates[0].Support[0].Locator.EventID)
				case "authorization_revision":
					_, err = f.db.Exec(`UPDATE memory_review_authorization SET revision=revision+1`)
				}
				if err != nil {
					t.Fatal(err)
				}
				decision := decisionFor(preview, "90000000-0000-4000-8000-000000000151")
				if change == "authorization_revision" {
					if _, err = f.store.ResolveOwnerCandidateReview(ctx, authority, decision); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
						t.Fatalf("expired authority returned terminal information: %v", err)
					}
					authority, err = f.store.LocalOwnerReviewContext(ctx, "global")
					if err != nil {
						t.Fatal(err)
					}
				}
				got, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decision)
				if !errors.Is(err, eviedb.ErrReviewResolved) || string(mustJSON(t, got)) != string(mustJSON(t, winning)) {
					t.Fatalf("terminal outcome lost after %s: got %+v, error %v; want %+v", change, got, err, winning)
				}
				if after := counts(); after != before {
					t.Fatalf("terminal lookup changed durable state: before %v after %v", before, after)
				}
				var forged eviedb.OwnerReviewContext
				if _, err = f.store.ResolveOwnerCandidateReview(ctx, forged, decision); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
					t.Fatalf("forged authority returned terminal information: %v", err)
				}
				wrongScope, err := f.store.LocalOwnerReviewContext(ctx, "session:"+string(f.session.ID))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.store.ResolveOwnerCandidateReview(ctx, wrongScope, decision); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
					t.Fatalf("another scope returned terminal information: %v", err)
				}
				decision.PreviewSHA256 = memory.CompilerHash([]byte("forged preview"))
				if _, err = f.store.ResolveOwnerCandidateReview(ctx, authority, decision); !errors.Is(err, eviedb.ErrReviewStale) {
					t.Fatalf("forged preview returned terminal information: %v", err)
				}
			})
		}
	}
}
