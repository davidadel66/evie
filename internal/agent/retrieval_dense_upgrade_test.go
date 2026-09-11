package agent

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func TestRetrievalUpgradeIsolatesObsoleteFTSStatisticsAndReconcilesRetainedSources(t *testing.T) {
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "saffron keepsake")
	f.refresh()
	// Only disposable derived-index state is authored here to reproduce a
	// previous installation. Accepted memory and original events come from the
	// public owner-approval path above and are untouched by the upgrade fixture.
	_, err := f.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_retrieval_fts USING fts5(generation UNINDEXED,claim_id UNINDEXED,scope_key UNINDEXED,body,tokenize='unicode61');
INSERT INTO memory_retrieval_fts(generation,claim_id,scope_key,body) VALUES('accepted-fts-unicode61-v1','obsolete-derived-id','global','saffron saffron saffron saffron');
DROP TABLE IF EXISTS memory_retrieval_fts_v3;
DROP TABLE IF EXISTS memory_retrieval_event_fts_v3;
DELETE FROM memory_retrieval_generations WHERE generation IN ('accepted-fts-unicode61-v3','conversation-fts-unicode61-v3');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	coverage, err := f.store.MemoryIndexCoverage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(coverage.Generation, "accepted-fts-unicode61-v3+conversation-fts-unicode61-v3") || coverage.State != "building" {
		t.Fatalf("upgrade reused statistics/coverage from obsolete projections: %+v", coverage)
	}
	f.refresh()
	client, _ := f.search(f.global(), "saffron")
	found := false
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if item.ClaimID == "obsolete-derived-id" {
			t.Fatal("obsolete index row became authoritative evidence")
		}
		found = found || item.ClaimID == target.ClaimID && item.Sources[0].EventID == target.Source.EventID
	}
	if !found {
		t.Fatal("bounded upgrade failed to recover original accepted source")
	}
}

func TestDenseConfigurationUpgradeNeverServesAnObsoleteActiveGeneration(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	// Simulate an older installed embedding configuration using derived state
	// only. The immutable configuration trigger remains enabled throughout.
	_, err := f.db.Exec(`INSERT INTO memory_dense_generations(generation,configuration,configuration_hash,state,claim_checkpoint,event_checkpoint)
 SELECT generation||':legacy','{"version":"legacy"}','legacy-configuration-hash','active',claim_checkpoint,event_checkpoint FROM memory_dense_generations WHERE generation=(SELECT generation FROM memory_dense_selection);
INSERT INTO memory_dense_vectors(generation,claim_id,scope_key,byte_start,byte_end,source_hash,revision_hash,content_hash,document_hash,vector)
 SELECT generation||':legacy',claim_id,scope_key,byte_start,byte_end,source_hash,revision_hash,content_hash,document_hash,vector FROM memory_dense_vectors WHERE generation=(SELECT generation FROM memory_dense_selection);
UPDATE memory_dense_selection SET generation=generation||':legacy';`)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.mu.Lock()
	calls := len(endpoint.inputs)
	endpoint.mu.Unlock()
	before, _ := f.search(f.global(), "jogging at dawn")
	if len(denseTurnEvidence(t, before.reqs[1])) != 0 {
		t.Fatal("active but obsolete embedding configuration served authoritative evidence")
	}
	endpoint.mu.Lock()
	afterCalls := len(endpoint.inputs)
	endpoint.mu.Unlock()
	if calls != afterCalls {
		t.Fatal("query embedded against an obsolete generation before replacement reconciliation")
	}
	f.refresh()
	after, _ := f.search(f.global(), "jogging at dawn")
	for _, item := range denseTurnEvidence(t, after.reqs[1]) {
		if item.ClaimID == target.ClaimID && !strings.HasSuffix(item.RetrievalGeneration, ":legacy") {
			return
		}
	}
	t.Fatal("configuration upgrade did not reconcile and recover the original source")
}

func TestDenseActiveGenerationRemainsUsableDuringIndependentLexicalRebuild(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	_, err := f.db.Exec(`DROP TABLE memory_retrieval_fts_v3; DROP TABLE memory_retrieval_event_fts_v3; DELETE FROM memory_retrieval_generations WHERE generation IN ('accepted-fts-unicode61-v3','conversation-fts-unicode61-v3');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	accepted, _ := f.search(f.global(), "jogging at dawn")
	found := false
	for _, item := range denseTurnEvidence(t, accepted.reqs[1]) {
		found = found || item.ClaimID == target.ClaimID && item.RetrievalGeneration != ""
	}
	if !found {
		t.Fatal("independent lexical backfill hid fully reconciled dense accepted evidence")
	}
	if result := expansionBoundaryToolResult(t, accepted.reqs[1], "lookup"); !strings.Contains(result, `"status":"partial"`) {
		t.Fatalf("incomplete lexical coverage was not explicit: %s", result)
	}
	conversation := f.searchConversations(f.global(), "jogging at dawn")
	found = false
	for _, item := range denseTurnEvidence(t, conversation.reqs[1]) {
		found = found || len(item.Sources) == 1 && item.Sources[0].EventID == target.Source.EventID
	}
	if !found {
		t.Fatal("independent lexical backfill hid fully reconciled dense conversation evidence")
	}
	endpoint.mu.Lock()
	endpoint.onEmbed = func(w http.ResponseWriter, _ *http.Request, _ []string) bool {
		http.Error(w, "offline", http.StatusServiceUnavailable)
		return true
	}
	endpoint.mu.Unlock()
	offline, _ := f.search(f.global(), "jogging at dawn")
	if result := expansionBoundaryToolResult(t, offline.reqs[1], "lookup"); !strings.Contains(result, `"status":"unavailable"`) {
		t.Fatalf("all unavailable generators were reported as a completed empty baseline: %s", result)
	}
}
