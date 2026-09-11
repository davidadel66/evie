package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

func denseTurnEvidence(t *testing.T, request openrouter.ChatRequest) []memory.RetrievalEvidence {
	t.Helper()
	for _, message := range request.Messages {
		if !strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
			continue
		}
		var data struct {
			Evidence []memory.RetrievalEvidence `json:"evidence"`
		}
		if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content, "EVIE_MEMORY_DATA\n")), &data); err != nil {
			t.Fatal(err)
		}
		return data.Evidence
	}
	return nil
}

func TestDenseGenerationRebuildReconcilesBeforeServingAndSurvivesRestart(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	client, _ := f.search(f.global(), "jogging at dawn")
	var original memory.RetrievalEvidence
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if item.ClaimID == target.ClaimID {
			original = item
		}
	}
	if original.RetrievalGeneration == "" {
		t.Fatal("baseline dense result has no immutable generation")
	}
	rebuilder, ok := any(f.store).(interface {
		RebuildMemoryEmbeddings(context.Context) (memory.RetrievalCoverage, error)
	})
	if !ok {
		t.Fatal("local maintenance cannot replace a generation for a retained-state rebuild")
	}
	coverage, err := rebuilder.RebuildMemoryEmbeddings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if coverage.State != "building" || coverage.Generation == original.RetrievalGeneration || coverage.Pending == 0 {
		t.Fatalf("rebuild did not create a distinct incomplete generation: %+v", coverage)
	}
	before, _ := f.search(f.global(), "jogging at dawn")
	if len(denseTurnEvidence(t, before.reqs[1])) != 0 {
		t.Fatal("unreconciled replacement generation served a paraphrase")
	}
	if result := expansionBoundaryToolResult(t, before.reqs[1], "lookup"); !strings.Contains(result, `"status":"partial"`) {
		t.Fatalf("incomplete dense coverage was not explicit: %s", result)
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	f.refresh()
	after, _ := f.search(f.global(), "jogging at dawn")
	for _, item := range denseTurnEvidence(t, after.reqs[1]) {
		if item.ClaimID == target.ClaimID && item.RetrievalGeneration == coverage.Generation && item.Sources[0].EventID == target.Source.EventID {
			return
		}
	}
	t.Fatal("restarted replacement did not recover the same original source under its new generation")
}

func TestDenseDisabledGenerationReconcilesAcceptedChangesBeforeReenable(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	old := f.remember(source, memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	f.refresh()
	endpoint.mu.Lock()
	calls := len(endpoint.inputs)
	endpoint.mu.Unlock()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, old.ClaimID)
	replacement := f.remember(source, memory.MemoryEverywhere, "running group runs before sunrise")
	f.refresh()
	disabled, _ := f.search(f.global(), "jogging at dawn")
	if len(denseTurnEvidence(t, disabled.reqs[1])) != 0 {
		t.Fatal("disabled generation served dense evidence")
	}
	endpoint.mu.Lock()
	nowCalls := len(endpoint.inputs)
	endpoint.mu.Unlock()
	if nowCalls != calls {
		t.Fatal("disabled maintenance or query sent embedding input")
	}
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	before, _ := f.search(f.global(), "jogging at dawn")
	if len(denseTurnEvidence(t, before.reqs[1])) != 0 {
		t.Fatal("re-enabled generation served before reconciling disabled-period changes")
	}
	f.refresh()
	after, _ := f.search(f.global(), "jogging at dawn")
	found := false
	for _, item := range denseTurnEvidence(t, after.reqs[1]) {
		if item.ClaimID == old.ClaimID {
			t.Fatal("retired old vector bypassed current eligibility")
		}
		found = found || item.ClaimID == replacement.ClaimID
	}
	if !found {
		t.Fatal("re-enabled generation missed an accepted change made while disabled")
	}
}

func TestDenseEndpointDeadlinePreservesEligibleLexicalEvidenceAsPartial(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	endpoint.mu.Lock()
	endpoint.onEmbed = func(_ http.ResponseWriter, request *http.Request, texts []string) bool {
		if len(texts) == 1 && texts[0] == "sunrise" {
			<-request.Context().Done()
			return true
		}
		return false
	}
	endpoint.mu.Unlock()
	client, _ := f.search(f.global(), "sunrise")
	found := false
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		found = found || item.ClaimID == target.ClaimID
	}
	if !found {
		t.Fatal("unavailable dense generator discarded already eligible lexical evidence")
	}
	if result := expansionBoundaryToolResult(t, client.reqs[1], "lookup"); !strings.Contains(result, `"status":"partial"`) {
		t.Fatalf("dense endpoint deadline was presented as complete or empty: %s", result)
	}
}

func TestDenseBackfillRechecksConcurrentRetirementAndAcceptedAppend(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	old := f.remember(source, memory.MemoryEverywhere, "runs before sunrise")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	endpoint.mu.Lock()
	endpoint.onEmbed = func(_ http.ResponseWriter, request *http.Request, texts []string) bool {
		for _, text := range texts {
			if strings.Contains(text, "runs before sunrise") {
				once.Do(func() {
					close(entered)
					select {
					case <-release:
					case <-request.Context().Done():
					}
				})
				break
			}
		}
		return false
	}
	endpoint.mu.Unlock()
	done := make(chan error, 1)
	go func() { _, err := f.store.RefreshMemoryIndex(context.Background(), 256); done <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("fixture never reached unlocked embedding I/O")
	}
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, old.ClaimID)
	added := f.remember(source, memory.MemoryEverywhere, "new group runs before sunrise")
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	f.refresh()
	client, _ := f.search(f.global(), "jogging at dawn")
	found := false
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if item.ClaimID == old.ClaimID {
			t.Fatal("in-flight embedding revived the concurrently retired Claim")
		}
		found = found || item.ClaimID == added.ClaimID
	}
	if !found {
		t.Fatal("retained checkpoint skipped the concurrent accepted append")
	}
	historical := f.historicalSearch(f.global(), "memory_search", "jogging at dawn", nil)
	retired := false
	for _, item := range denseTurnEvidence(t, historical.reqs[1]) {
		retired = retired || item.ClaimID == old.ClaimID && item.CurrentStatus == memory.SemanticStatusRetired
	}
	if !retired {
		t.Fatal("historical dense read lost explicitly marked retired evidence")
	}
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, old.SourceLinkID)
	restricted := f.historicalSearch(f.global(), "memory_search", "jogging at dawn", nil)
	for _, item := range denseTurnEvidence(t, restricted.reqs[1]) {
		if item.ClaimID == old.ClaimID {
			t.Fatal("stale vector bypassed a newly restricted source")
		}
	}
}

func TestDenseMaintenanceBoundsChunksAndResumesOneLongOriginal(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, strings.Repeat("a! ", 1300)+"the club runs before sunrise")
	coverage, err := f.store.RefreshMemoryIndex(context.Background(), 256)
	if err != nil {
		t.Fatal(err)
	}
	endpoint.mu.Lock()
	inputs := 0
	for _, batch := range endpoint.inputs {
		inputs += len(batch)
	}
	endpoint.mu.Unlock()
	if inputs > 16 {
		t.Fatalf("one maintenance batch exceeded its selected sixteen-input work bound: %d", inputs)
	}
	if coverage.State == "active" {
		t.Fatal("partially indexed long source was marked active")
	}
	f.refresh()
	client, _ := f.search(f.global(), "jogging at dawn")
	if result := expansionBoundaryToolResult(t, client.reqs[1], "lookup"); !strings.Contains(result, `"status":"exhausted"`) {
		t.Fatalf("oversized complete Claim must retain the existing model-context gate: %s", result)
	}
	original := f.searchConversations(f.global(), "jogging at dawn")
	for _, item := range denseTurnEvidence(t, original.reqs[1]) {
		if len(item.Sources) == 1 && item.Sources[0].EventID == target.Source.EventID && strings.Contains(item.Text, "runs before sunrise") {
			return
		}
	}
	t.Fatal("bounded resumed chunks never recovered the original source passage")
}
