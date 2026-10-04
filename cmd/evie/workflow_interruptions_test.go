package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

var receiptSplitDir = filepath.Join("..", "..", "internal", "workflow", "testdata", "receipt-split")

// scriptedChat is a fake OpenRouter client answering every AI request
// with one text.
type scriptedChat struct {
	mu       sync.Mutex
	text     string
	requests int
}

func (s *scriptedChat) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	s.mu.Lock()
	s.requests++
	s.mu.Unlock()
	in, out := int64(100), int64(10)
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: s.text}}},
		Usage: &openrouter.TokenUsage{InputTokens: &in, OutputTokens: &out}}, nil
}

func (s *scriptedChat) ModelPricing(context.Context, string) (openrouter.ModelPricing, error) {
	return openrouter.ModelPricing{Prompt: 1e-6, Completion: 4e-6}, nil
}

var receiptRunLine = regexp.MustCompile(`Run (\S+): receipt-split version`)

// The owner answers a run's question from the CLI: `run` exits with the
// question and how to answer it, `interruptions` lists it for the
// Workspace and for the session that started the run, and `answer`
// records a typed answer and continues the run (Stage 14 sub-stage 7).
func TestWorkflowCLIAnswersAnInterruption(t *testing.T) {
	ctx := context.Background()
	store := backgroundTestStore(t)
	chat := &scriptedChat{text: `{"total_cents":1150,"confidence":"low"}`}
	rt, err := workflowruntime.New(workflowruntime.Config{
		Store: store, Repo: procedural.New(filepath.Join(t.TempDir(), "procedural")), Capabilities: workflowruntime.ContractsOnly(notesTestCatalog()),
		Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}, Models: workflowruntime.NewOpenRouterModels(chat),
	})
	if err != nil {
		t.Fatal(err)
	}
	c := &workflowCLI{t: t, store: store, rt: rt}
	w, err := store.RegisterWorkspace(ctx, "Receipts")
	if err != nil {
		t.Fatal(err)
	}
	card := c.mustRun("workflows", "propose", string(w.ID), receiptSplitDir, "--fixtures", filepath.Join("..", "..", "internal", "workflow", "testdata", "receipt-split.fixtures.json"))
	m := approveLine.FindStringSubmatch(card)
	c.mustRun("workflows", "approve", m[1], "--hash", m[2], "--authority", m[3], "--expect-version", m[4])

	out, err := c.run("workflows", "run", string(w.ID), "receipt-split", "--input", "receipt_id=r-9", "--input", "receipt_text=TOTAL 11.50?", "--input", "people=3")
	if err == nil || !strings.Contains(err.Error(), "waits for your answer") || !strings.Contains(err.Error(), "evie workflows answer") ||
		!strings.Contains(out, "Waiting for you: steps.confirm asks") || !strings.Contains(out, "AI spend: 110 tokens") {
		t.Fatalf("run = %v\n%s", err, out)
	}
	id := receiptRunLine.FindStringSubmatch(out)[1]
	listed := c.mustRun("workflows", "interruptions", string(w.ID))
	if !strings.Contains(listed, "Run "+id) || !strings.Contains(listed, `"proposed_total_cents":1150`) {
		t.Fatalf("interruptions:\n%s", listed)
	}
	raw := c.mustRun("workflows", "interruptions", "--json", "--run", id, "--all")
	var shown struct {
		Interruptions []eviedb.WorkflowInterruption `json:"interruptions"`
	}
	if err := json.Unmarshal([]byte(raw), &shown); err != nil || len(shown.Interruptions) != 1 || shown.Interruptions[0].Step != "confirm" {
		t.Fatalf("interruptions --json = %s (%v)", raw, err)
	}
	if _, err := c.run("workflows", "answer", id, "confirm", "--answer", `{"total_cents":"x"}`); err == nil || !strings.Contains(err.Error(), "does not fit") {
		t.Fatalf("an answer outside the schema: %v", err)
	}
	file := filepath.Join(t.TempDir(), "answer.json")
	if err := os.WriteFile(file, []byte(`{"total_cents": 1300}`), 0o600); err != nil {
		t.Fatal(err)
	}
	out = c.mustRun("workflows", "answer", id, "confirm", "--answer-file", file)
	if !strings.Contains(out, "Answered steps.confirm of run "+id) || !strings.Contains(out, "succeeded") || !strings.Contains(out, "confirmed_shares: [434,433,433]") ||
		!strings.Contains(out, "answered by the owner via cli") {
		t.Fatalf("answer output:\n%s", out)
	}
	if none := c.mustRun("workflows", "interruptions", string(w.ID)); !strings.Contains(none, "No open interruptions") {
		t.Fatalf("after the answer:\n%s", none)
	}
	for _, args := range [][]string{
		{"workflows", "answer", id},
		{"workflows", "answer", id, "confirm"},
		{"workflows", "answer", id, "confirm", "--answer", "{}", "--answer-file", file},
		{"workflows", "answer", id, "confirm", "--answer", "[1]"},
		{"workflows", "interruptions", "a", "b"},
	} {
		if _, err := c.run(args...); err == nil {
			t.Fatalf("%q was accepted", args)
		}
	}
}

// AI steps reach OpenRouter only when the owner turned remote workflow
// models on, with an API key; any other setting refuses to start.
func TestWorkflowRemoteModelsSetting(t *testing.T) {
	env := func(values map[string]string) func(string) string { return func(k string) string { return values[k] } }
	models, off, err := workflowModels(env(nil))
	if err != nil || models != nil || !strings.Contains(off, "EVIE_WORKFLOW_REMOTE_MODELS=on") {
		t.Fatalf("default = %v, %q, %v", models, off, err)
	}
	models, off, err = workflowModels(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "off", "OPENROUTER_API_KEY": "k"}))
	if err != nil || models != nil || off == "" {
		t.Fatalf("off = %v, %q, %v", models, off, err)
	}
	models, off, err = workflowModels(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "on"}))
	if err != nil || models != nil || !strings.Contains(off, "OPENROUTER_API_KEY") {
		t.Fatalf("on without a key = %v, %q, %v", models, off, err)
	}
	models, off, err = workflowModels(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "on", "OPENROUTER_API_KEY": "k"}))
	if err != nil || models == nil || off != "" {
		t.Fatalf("on = %v, %q, %v", models, off, err)
	}
	if _, _, err := workflowModels(env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "yes"})); err == nil || !strings.Contains(err.Error(), "on or off") {
		t.Fatalf("an unknown setting: %v", err)
	}
	if _, err := newWorkflowRuntime(backgroundTestStore(t), env(map[string]string{"EVIE_WORKFLOW_REMOTE_MODELS": "always", "HOME": t.TempDir()}), nil); err == nil {
		t.Fatal("the runtime started with an unknown remote model setting")
	}
}
