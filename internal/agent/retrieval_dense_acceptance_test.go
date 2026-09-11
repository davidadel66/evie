package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/memory"
)

func TestMemorySearchTurnFindsParaphraseThroughLocalEmbeddingWithExactSources(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	target := f.remember(source, memory.MemoryEverywhere, "runs before sunrise")
	distractor := f.remember(source, memory.MemoryEverywhere, "watches distant galaxies")
	f.refresh()
	client, _ := f.search(f.global(), "jogging at dawn")
	evidence := expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1])
	found := false
	for _, item := range evidence {
		if item.ClaimID == distractor.ClaimID {
			t.Fatal("orthogonal fixture evidence filled a semantic result")
		}
		if item.ClaimID == target.ClaimID {
			found = true
			if len(item.Sources) != 1 || item.Sources[0].EventID != target.Source.EventID || item.Sources[0].Evidence != target.Source.Evidence || item.Status != memory.SemanticStatusActive {
				t.Fatalf("dense suggestion lost accepted authority or its exact original source: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("complete turn did not recover the paraphrased accepted fact through the configured local embedding endpoint")
	}
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	querySeen, sourceSeen := false, false
	for _, batch := range endpoint.inputs {
		for _, text := range batch {
			querySeen = querySeen || text == "jogging at dawn"
			sourceSeen = sourceSeen || strings.Contains(text, "runs before sunrise")
		}
	}
	if !querySeen || !sourceSeen {
		t.Fatalf("fixture embedding I/O was not exercised: query=%v source=%v", querySeen, sourceSeen)
	}
}

func TestConversationSearchTurnFindsParaphraseWithoutInventingAnAcceptedClaim(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	statement := f.converse(source, "The running club runs before sunrise.")
	f.refresh()
	client := f.searchConversations(f.global(), "jogging at dawn")
	evidence := expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1])
	found := false
	for _, item := range evidence {
		if len(item.Sources) != 1 || item.Sources[0].EventID != statement.ID {
			continue
		}
		found = true
		if item.Kind != memory.RetrievalConversationExcerpt || item.ClaimID != "" || item.Claim != nil || item.Text != statement.Content || item.Sources[0].Authority != memory.AuthorityOwnerStatement {
			t.Fatalf("dense conversation suggestion lost its original authority or locator: %+v", item)
		}
	}
	if !found {
		t.Fatal("complete conversation-search turn did not recover the original paraphrased statement through local dense evidence")
	}
}

func TestConversationDenseTurnMarksPartialOriginalPassage(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	statement := f.converse(f.global(), strings.Repeat("Café astronomy details. ", 65)+"The running club runs before sunrise.")
	f.refresh()
	client := f.searchConversations(f.global(), "jogging at dawn")
	request := client.reqs[len(client.reqs)-1]
	evidence := expansionBoundaryEvidence(t, request)
	found := false
	for _, item := range evidence {
		if len(item.Sources) != 1 || item.Sources[0].EventID != statement.ID {
			continue
		}
		found = true
		source := item.Sources[0]
		if !utf8.ValidString(item.Text) || len(item.Text) > 800 || !strings.Contains(item.Text, "runs before sunrise") || source.EvidenceSHA256 != memory.CompilerHash([]byte(item.Text)) || source.Evidence != item.Text {
			t.Fatalf("dense snippet lost its exact original UTF-8 passage: %+v", item)
		}
	}
	if !found {
		t.Fatal("the matching later passage was not supplied")
	}
	if result := expansionBoundaryToolResult(t, request, "conversation-lookup"); !strings.Contains(result, `"truncated":true`) {
		t.Fatalf("partial dense excerpt was presented as complete: %s", result)
	}
}

func TestDenseSingleRowBackfillEventuallyServesCompleteTurn(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	var coverage memory.RetrievalCoverage
	for i := 0; i < 500; i++ {
		var err error
		coverage, err = f.store.RefreshMemoryIndex(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
	}
	if coverage.State != "active" || coverage.Pending != 0 {
		t.Fatalf("bounded single-row maintenance never reconciled retained coverage: %+v", coverage)
	}
	client, _ := f.search(f.global(), "jogging at dawn")
	for _, item := range expansionBoundaryEvidence(t, client.reqs[len(client.reqs)-1]) {
		if item.ClaimID == target.ClaimID {
			return
		}
	}
	t.Fatal("complete turn missed the paraphrase after single-row retained reconciliation")
}

func TestDenseLongAcceptedProjectionReconcilesWithoutSilentInputTruncation(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	value := strings.Repeat("A café detail; ", 85) + "the group runs before sunrise"
	target := f.remember(f.global(), memory.MemoryEverywhere, value)
	f.refresh()
	client, _ := f.search(f.global(), "jogging at dawn")
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if item.ClaimID == target.ClaimID && strings.Contains(item.Text, value) && len(item.Sources) == 1 && item.Sources[0].EventID == target.Source.EventID {
			return
		}
	}
	t.Fatal("a long accepted projection lost its later paraphrase or exact full Claim")
}

func TestDenseAutomaticRecallUsesTheSameLocalCandidateAndSourcePath(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	target := f.remember(f.global(), memory.MemoryEverywhere, "runs before sunrise")
	f.refresh()
	client := &fakeClient{steps: []step{assistantStep("The original schedule is available.", nil)}}
	if err := f.automaticSession(f.global(), client).Send(context.Background(), "jogging at dawn", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	for _, item := range denseTurnEvidence(t, client.reqs[0]) {
		if item.ClaimID == target.ClaimID && item.RetrievalGeneration != "" && item.Sources[0].EventID == target.Source.EventID {
			return
		}
	}
	endpoint.mu.Lock()
	inputs := append([][]string(nil), endpoint.inputs...)
	endpoint.mu.Unlock()
	t.Fatalf("first automatic request omitted the same source-bearing dense evidence; embedding inputs: %+v; data: %s", inputs, retrievalData(t, client.reqs[0]))
}

func TestDenseConversationCandidateWorkSurvivesCommonLexicalNoise(t *testing.T) {
	endpoint := newDenseFixtureEndpoint(t)
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", endpoint.server.URL)
	f := newRetrievalFixture(t)
	source := f.global()
	target := f.converse(source, "The running club runs before sunrise.")
	for i := 0; i < 70; i++ {
		f.converse(source, fmt.Sprintf("Inventory item %d remains at the distant observatory.", i))
	}
	f.refresh()
	client := f.searchConversations(f.global(), "jogging at dawn")
	for _, item := range denseTurnEvidence(t, client.reqs[1]) {
		if len(item.Sources) == 1 && item.Sources[0].EventID == target.ID && item.Text == target.Content {
			return
		}
	}
	t.Fatal("common lexical suggestions consumed the independent dense generator's bounded work allowance")
}

// This is deterministic external embedding I/O, not a quality measurement.
// The production turn, SQLite authority, index lifecycle and HTTP client remain
// real. Actual MiniLM inference has a separate frozen experiment.
type denseFixtureEndpoint struct {
	server  *httptest.Server
	mu      sync.Mutex
	inputs  [][]string
	onEmbed func(http.ResponseWriter, *http.Request, []string) bool
}

func newDenseFixtureEndpoint(t *testing.T) *denseFixtureEndpoint {
	t.Helper()
	endpoint := &denseFixtureEndpoint{}
	endpoint.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "all-minilm:22m", "model": "all-minilm:22m", "digest": "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}}})
		case "/api/embed":
			var request struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
				http.Error(w, "invalid bounded request", http.StatusBadRequest)
				return
			}
			endpoint.mu.Lock()
			endpoint.inputs = append(endpoint.inputs, append([]string(nil), request.Input...))
			hook := endpoint.onEmbed
			endpoint.mu.Unlock()
			if hook != nil && hook(w, r, request.Input) {
				return
			}
			var embeddings [][]float32
			for _, text := range request.Input {
				vector := make([]float32, 384)
				if strings.Contains(text, "runs before sunrise") || text == "jogging at dawn" || text == "jogging dawn" {
					vector[0] = 1
				} else {
					vector[1] = 1
				}
				embeddings = append(embeddings, vector)
			}
			json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "embeddings": embeddings})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(endpoint.server.Close)
	return endpoint
}
