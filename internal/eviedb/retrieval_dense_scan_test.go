package eviedb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

const denseScanTarget = "The running club runs before sunrise."

// newDenseScanFixture builds real Global history with dense vectors from a
// loopback embedder: the target statement and the query share one direction,
// every filler another. Returns the target event IDs and a reader session.
func newDenseScanFixture(t *testing.T, fillers int, targetsAt ...int) (*Store, []memory.EventID, memory.Session) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "all-minilm:22m", "model": "all-minilm:22m", "digest": "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}}})
		case "/api/embed":
			var request struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			var embeddings [][]float32
			for _, text := range request.Input {
				vector := make([]float32, 384)
				if strings.Contains(text, "runs before sunrise") || text == "jogging at dawn" {
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
	t.Cleanup(server.Close)
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", server.URL)
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	source, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var targets []memory.EventID
	for i := 0; i < fillers; i++ {
		text := fmt.Sprintf("Filler note %d about the checksum parser.", i)
		if slices.Contains(targetsAt, i) {
			text = denseScanTarget
		}
		event, err := store.appendEventForTest(ctx, source.ID, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
		if err != nil {
			t.Fatal(err)
		}
		if text == denseScanTarget {
			targets = append(targets, event.ID)
		}
	}
	for i := 0; ; i++ {
		coverage, err := store.RefreshMemoryIndex(ctx, 256)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
		if i == 100 {
			t.Fatal("dense backfill did not complete")
		}
	}
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return store, targets, reader
}

func withDenseScan(t *testing.T, batch, budget int) {
	t.Helper()
	oldBatch, oldBudget := denseVectorScanBatch, denseVectorScanBudget
	denseVectorScanBatch, denseVectorScanBudget = batch, budget
	t.Cleanup(func() { denseVectorScanBatch, denseVectorScanBudget = oldBatch, oldBudget })
}

func denseScanFound(result memory.RetrievalResult, id memory.EventID) bool {
	for _, item := range result.Evidence {
		if slices.Contains(item.Paths, "conversation_dense") && len(item.Sources) == 1 && item.Sources[0].EventID == id {
			return true
		}
	}
	return false
}

// M7: dense recall pages through the whole vector table instead of stopping
// at the first page, so targets anywhere in random-UUID order are reachable.
func TestDenseConversationScanCoversEveryVectorAcrossPages(t *testing.T) {
	store, targets, reader := newDenseScanFixture(t, 24, 3, 20)
	withDenseScan(t, 1, 1<<16)
	result, err := store.SearchMemory(context.Background(), reader.ScopeContext(), memory.RetrievalQuery{Kind: memory.RetrievalConversationExcerpt, Text: "jogging at dawn"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range targets {
		if !denseScanFound(result, id) {
			t.Errorf("dense target %s beyond the first page was not found: %+v", id, result)
		}
	}
	if len(result.Gaps) != 0 || result.Status != memory.RetrievalSuccess {
		t.Errorf("complete scan reported incomplete coverage: status=%s gaps=%v", result.Status, result.Gaps)
	}
}

// M7: when the scan budget genuinely cuts coverage, the result says so with a
// distinct gap, not only "partial" (which pending index work also produces).
func TestDenseConversationScanBudgetReportsDistinctGap(t *testing.T) {
	store, _, reader := newDenseScanFixture(t, 24, 3, 20)
	withDenseScan(t, 1, 2)
	result, err := store.SearchMemory(context.Background(), reader.ScopeContext(), memory.RetrievalQuery{Kind: memory.RetrievalConversationExcerpt, Text: "jogging at dawn"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != memory.RetrievalPartial || !slices.Contains(result.Gaps, memory.RetrievalGapDenseScan) {
		t.Fatalf("budget-cut dense scan looked complete: status=%s gaps=%v", result.Status, result.Gaps)
	}
	outcome, err := memory.RenderRetrievalOutcome(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outcome, `"gaps":["`+memory.RetrievalGapDenseScan+`"]`) {
		t.Fatalf("model-visible outcome hides the scan gap: %s", outcome)
	}
	// A budget that reaches every vector reports no gap.
	withDenseScan(t, 1, 24)
	result, err = store.SearchMemory(context.Background(), reader.ScopeContext(), memory.RetrievalQuery{Kind: memory.RetrievalConversationExcerpt, Text: "jogging at dawn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Gaps) != 0 {
		t.Fatalf("exact-fit budget reported a gap: %v", result.Gaps)
	}
}
