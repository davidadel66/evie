package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// lowConfidenceChat answers every AI request with a low-confidence total.
type lowConfidenceChat struct{}

func (lowConfidenceChat) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	in, out := int64(100), int64(10)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: `{"total_cents":1150,"confidence":"low"}`}}},
		Usage: &openrouter.TokenUsage{InputTokens: &in, OutputTokens: &out}}, nil
}

func (lowConfidenceChat) ModelPricing(context.Context, string) (openrouter.ModelPricing, error) {
	return openrouter.ModelPricing{Prompt: 1e-6, Completion: 4e-6}, nil
}

// The owner answers a run's question from the web (Stage 14 sub-stage 7):
// the interruptions route lists it for the session that started the run,
// and the answer route records a typed answer from any session of the
// Workspace; malformed requests, answers outside the step's schema and
// answers to nothing are refused with their reason.
func TestWorkflowWebAnswersAnInterruption(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	rt, err := workflowruntime.New(workflowruntime.Config{Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")),
		Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}, Models: workflowruntime.NewOpenRouterModels(lowConfidenceChat{})})
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.RegisterWorkspace(ctx, "Receipts")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := os.ReadFile(filepath.Join(webTestdata, "receipt-split.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "receipt-split", fixtures)
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	receipt := composition.Receipt{FormatVersion: composition.FormatVersion, Preset: composition.PresetIdentity{ID: "standard", Version: "sha256:" + hash},
		EvieVersion: "1.0.0", Providers: []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities: []composition.Capability{{ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash}},
		ToolSchemas:  []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}}, Instructions: []composition.InstructionReference{{ID: "fixture-instructions", SHA256: hash}}}
	origin, err := store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	later, err := store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, receipt)
	if err != nil {
		t.Fatal(err)
	}
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	resp := workflowPost(t, h, "start", map[string]any{"workspace_id": w.ID, "name": "receipt-split", "session_id": origin.ID,
		"inputs": map[string]any{"receipt_id": "r-1", "receipt_text": "TOTAL 11.50?", "people": 3}})
	var started workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &started) != nil || started.Run.Run.State != eviedb.WorkflowRunWaitingForHuman {
		t.Fatalf("start = %d %s", resp.Code, resp.Body)
	}
	id := started.Run.Run.ID
	resp = workflowPost(t, h, "interruptions", map[string]any{"session_id": origin.ID})
	var listed struct {
		Interruptions []eviedb.WorkflowInterruption `json:"interruptions"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Interruptions) != 1 || listed.Interruptions[0].RunID != id ||
		listed.Interruptions[0].Step != "confirm" {
		t.Fatalf("interruptions = %d %s", resp.Code, resp.Body)
	}
	for name, body := range map[string]map[string]any{
		"no step":        {"run_id": id, "answer": map[string]any{"total_cents": 1300}},
		"no answer":      {"run_id": id, "step": "confirm"},
		"answer list":    {"run_id": id, "step": "confirm", "answer": []int{1300}},
		"surface chosen": {"run_id": id, "step": "confirm", "answer": map[string]any{"total_cents": 1300}, "surface": "chat"},
	} {
		if resp := workflowPost(t, h, "answer", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if resp := workflowPost(t, h, "answer", map[string]any{"run_id": id, "step": "confirm", "answer": map[string]any{"total_cents": "x"}}); resp.Code != http.StatusConflict ||
		!strings.Contains(resp.Body.String(), "answer_refused") {
		t.Fatalf("an answer outside the schema = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "answer", map[string]any{"run_id": id, "step": "split", "answer": map[string]any{"total_cents": 1}}); resp.Code != http.StatusConflict {
		t.Fatalf("a step that asks nothing = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "answer", map[string]any{"run_id": id, "step": "confirm", "session_id": later.ID, "answer": map[string]any{"total_cents": 1300}})
	var answered eviedb.WorkflowRunDetail
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &answered) != nil || answered.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("answer = %d %s", resp.Code, resp.Body)
	}
	res := answered.Interruptions[0].Resolution
	if res == nil || res.Surface != "web" || res.SessionID != later.ID || string(res.Answer) != `{"total_cents":1300}` {
		t.Fatalf("resolution = %+v", res)
	}
	if resp := workflowPost(t, h, "interruptions", map[string]any{"workspace_id": w.ID}); resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"interruptions":[]`) {
		t.Fatalf("after the answer = %d %s", resp.Code, resp.Body)
	}
}
