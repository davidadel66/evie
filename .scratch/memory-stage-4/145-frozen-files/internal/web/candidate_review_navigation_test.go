package web_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestCandidateReviewNavigationLegacyBoundedRestartAndPublication(t *testing.T) {
	f := newWebReviewFixture(t)
	ctx := context.Background()
	if _, err := f.db.Exec(`UPDATE sessions SET status='active' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	var err error
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "legacy-navigation", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 64; i++ {
		f.compile(t, "global")
	}
	if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	before := navigationCandidateBytes(t, f)
	// Simulate installation over a pre-review compiler database. Retained
	// candidates are actual Kernel publications; no source or envelope is forged.
	if _, err = f.db.Exec(`DELETE FROM memory_review_inbox_revisions; DROP TABLE memory_review_navigation_reconcile`); err != nil {
		t.Fatal(err)
	}
	reopen := func() {
		t.Helper()
		if err = f.db.Close(); err != nil {
			t.Fatal(err)
		}
		f.db, err = eviedb.OpenDBAt(f.path)
		if err != nil {
			t.Fatal(err)
		}
		f.store = eviedb.NewStore(f.db)
	}
	reopen()
	var cutoff string
	if err = f.db.QueryRow(`SELECT cutoff FROM memory_review_navigation_reconcile`).Scan(&cutoff); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = f.store.ListLocalOwnerCandidateScopes(canceled, memory.OwnerCandidateScopeQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled: %v", err)
	}
	var last string
	f.db.QueryRow(`SELECT last_candidate FROM memory_review_navigation_reconcile`).Scan(&last)
	if last != "" {
		t.Fatal("canceled page advanced cursor")
	}
	// A transactional write failure must also preserve the page cursor and ledger.
	if _, err = f.db.Exec(`CREATE TRIGGER navigation_test_abort BEFORE INSERT ON memory_review_inbox_revisions BEGIN SELECT RAISE(ABORT,'navigation fixture rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ListLocalOwnerCandidateScopes(ctx, memory.OwnerCandidateScopeQuery{}); err == nil {
		t.Fatal("expected rollback")
	}
	f.db.QueryRow(`SELECT last_candidate FROM memory_review_navigation_reconcile`).Scan(&last)
	if last != "" {
		t.Fatal("failed page advanced cursor")
	}
	if _, err = f.db.Exec(`DROP TRIGGER navigation_test_abort`); err != nil {
		t.Fatal(err)
	}
	first, err := f.store.ListLocalOwnerCandidateScopes(ctx, memory.OwnerCandidateScopeQuery{})
	if err != nil || !first.Indexing {
		t.Fatalf("first %+v %v", first, err)
	}
	f.db.QueryRow(`SELECT last_candidate FROM memory_review_navigation_reconcile`).Scan(&last)
	var inspected int
	f.db.QueryRow(`SELECT count(*) FROM memory_compiler_candidates WHERE candidate_id<=?`, last).Scan(&inspected)
	if inspected != 63 {
		t.Fatalf("inspected %d, want bounded63", inspected)
	}
	reopen()
	var resumed string
	f.db.QueryRow(`SELECT last_candidate FROM memory_review_navigation_reconcile`).Scan(&resumed)
	if resumed != last {
		t.Fatal("restart lost migration position")
	}
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "concurrent-navigation", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	f.compile(t, "session:"+string(f.session.ID))
	f.compile(t, "global")
	if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	var revision int64
	f.db.QueryRow(`SELECT revision FROM memory_review_inbox_revisions WHERE scope_key='global'`).Scan(&revision)
	var frozen string
	f.db.QueryRow(`SELECT cutoff FROM memory_review_navigation_reconcile`).Scan(&frozen)
	if frozen != cutoff {
		t.Fatal("new publications changed the frozen migration cutoff")
	}
	page, err := f.store.ListLocalOwnerCandidateScopes(ctx, memory.OwnerCandidateScopeQuery{})
	if err != nil || page.Indexing || len(page.Scopes) != 2 {
		t.Fatalf("complete %+v %v", page, err)
	}
	var afterRevision int64
	f.db.QueryRow(`SELECT revision FROM memory_review_inbox_revisions WHERE scope_key='global'`).Scan(&afterRevision)
	if revision != afterRevision {
		t.Fatal("migration reset the existing inbox revision")
	}
	after := navigationCandidateBytes(t, f)
	for id, b := range before {
		if !reflect.DeepEqual(b, after[id]) {
			t.Fatalf("migration changed candidate %s", id)
		}
	}
}

func navigationCandidateBytes(t *testing.T, f *webReviewFixture) map[string][]byte {
	t.Helper()
	rows, err := f.db.Query(`SELECT candidate_id,envelope FROM memory_compiler_candidates ORDER BY candidate_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string][]byte{}
	for rows.Next() {
		var id string
		var b []byte
		if err = rows.Scan(&id, &b); err != nil {
			t.Fatal(err)
		}
		out[id] = b
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCandidateReviewNavigationEmptyAndRevokedScopes(t *testing.T) {
	f := newWebReviewFixture(t)
	// Registry availability is checked after the bounded navigation page: an
	// old ledger entry does not grant access or disclose an archived scope name.
	if _, err := f.db.Exec(`INSERT INTO memory_review_inbox_revisions VALUES('workspace:missing',0)`); err != nil {
		t.Fatal(err)
	}
	page, err := f.store.ListLocalOwnerCandidateScopes(context.Background(), memory.OwnerCandidateScopeQuery{})
	if err != nil || page.Indexing || len(page.Scopes) != 1 || page.Scopes[0].ScopeKey != "global" {
		t.Fatalf("page %+v %v", page, err)
	}
	for _, q := range []memory.OwnerCandidateScopeQuery{{Limit: -1}, {Limit: 101}, {Cursor: "forged"}} {
		if _, err = f.store.ListLocalOwnerCandidateScopes(context.Background(), q); err == nil {
			t.Fatalf("invalid query %+v", q)
		}
	}
}
