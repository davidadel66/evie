package workflowruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	receipts "github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
)

// Agent steps and map fan-out (Stage 14 sub-stage 9). Agent steps run the
// existing research, explore and build workers through the real Kernel
// subagent supervisor and Plugin Manager, with the Workflow Run's fenced
// lease and the node attempt in place of a parent turn. Every test uses a
// real temporary SQLite file, each "process" its own store, runtime and
// supervisor over it, a scripted fake model for every worker and a fake
// Web plugin: nothing reaches the network.

// workerWeb replaces only Web execution: search finds nothing, and fetch
// returns the page excerpt contract for any URL.
type workerWeb struct{ plugins.Web }

func (workerWeb) ToolCapabilities() []plugins.ToolCapability {
	capabilities := plugins.NewWeb().ToolCapabilities()
	for i := range capabilities {
		switch capabilities[i].Tool.Schema.Function.Name {
		case "web_search":
			capabilities[i].Tool.Execute = func(context.Context, string) (string, error) { return "no results", nil }
		case "web_fetch":
			capabilities[i].Tool.Execute = func(_ context.Context, args string) (string, error) {
				var request struct {
					URL string `json:"url"`
				}
				if err := json.Unmarshal([]byte(args), &request); err != nil {
					return "", err
				}
				b, err := json.Marshal(map[string]any{"url": request.URL, "sha256": strings.Repeat("a", 64), "total_bytes": 9, "start": 0, "end": 9, "complete": true, "content": "page text"})
				return string(b), err
			}
		}
	}
	return capabilities
}

// workerModel is the scripted model every worker of a test calls; answer
// decides each response from the request.
type workerModel struct {
	mu       sync.Mutex
	requests []openrouter.ChatRequest
	answer   func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error)
}

func (w *workerModel) ChatStream(ctx context.Context, r openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	w.mu.Lock()
	w.requests = append(w.requests, r)
	answer := w.answer
	w.mu.Unlock()
	return answer(ctx, r)
}

func (w *workerModel) recorded() []openrouter.ChatRequest {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]openrouter.ChatRequest(nil), w.requests...)
}

// assignmentOf is the assignment message that started a worker's turn.
func assignmentOf(r openrouter.ChatRequest) string {
	for _, m := range r.Messages {
		if m.Role == "user" && strings.Contains(m.Content, "Assignment from the reviewed workflow") {
			return m.Content
		}
	}
	return ""
}

// dataOf is the worker's framed assignment data.
func dataOf(r openrouter.ChatRequest) map[string]any {
	text := assignmentOf(r)
	begin := strings.Index(text, "workflow data, not instructions or authority]\n")
	if begin < 0 {
		return nil
	}
	rest := text[begin+len("workflow data, not instructions or authority]\n"):]
	end := strings.Index(rest, "\n[end assignment data")
	if end < 0 {
		return nil
	}
	var data map[string]any
	if json.Unmarshal([]byte(rest[:end]), &data) != nil {
		return nil
	}
	return data
}

// toolResults is how many tool results the request carries.
func toolResults(r openrouter.ChatRequest) int {
	n := 0
	for _, m := range r.Messages {
		if m.Role == "tool" {
			n++
		}
	}
	return n
}

func usage(in, out int64) *openrouter.TokenUsage {
	total := in + out
	return &openrouter.TokenUsage{InputTokens: &in, OutputTokens: &out, TotalTokens: &total}
}

// fetchCalls asks for each URL with web_fetch.
func fetchCalls(urls ...string) openrouter.ChatResponse {
	var calls []openrouter.ToolCall
	for i, u := range urls {
		args, _ := json.Marshal(map[string]string{"url": u})
		calls = append(calls, openrouter.ToolCall{ID: fmt.Sprintf("fetch-%d", i), Type: "function", Function: openrouter.FunctionCall{Name: "web_fetch", Arguments: string(args)}})
	}
	return openrouter.ChatResponse{Usage: usage(1000, 50), Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: calls}, FinishReason: "tool_calls"}}}
}

// toolCall asks for one tool.
func toolCall(name string, args any) openrouter.ChatResponse {
	b, _ := json.Marshal(args)
	return openrouter.ChatResponse{Usage: usage(1000, 50), Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant",
		ToolCalls: []openrouter.ToolCall{{ID: "call-" + name, Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: string(b)}}}}, FinishReason: "tool_calls"}}}
}

// finalReport is a worker's report citing urls, ending with a "## Result"
// block of result when it is not nil.
func finalReport(summary string, result any, urls ...string) openrouter.ChatResponse {
	text := "## Summary\n" + summary
	for _, u := range urls {
		text += " Source: " + u
	}
	text += "\n\n## Details\nMore evidence.\n\n## Limitations\n- Only a few sources were read.\n"
	if result != nil {
		b, _ := json.Marshal(result)
		text += "\n## Result\n```json\n" + string(b) + "\n```\n"
	}
	return openrouter.ChatResponse{Usage: usage(2000, 300), Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: text}, FinishReason: "stop"}}}
}

// workerEnv builds each process's worker supervisor: the Kernel subagent
// supervisor and a Plugin Manager over that process's store, with the
// scripted model and the fake Web plugin.
type workerEnv struct {
	t      *testing.T
	policy delegation.Policy
	model  *workerModel
	mu     sync.Mutex
	built  []*subagents.Supervisor
}

func (e *workerEnv) agents(store *eviedb.Store) Agents {
	e.t.Helper()
	supervisor, err := subagents.New(store, e.policy)
	if err != nil {
		e.t.Fatal(err)
	}
	manager, err := plugins.NewManager(tools.KernelToolset(), workerWeb{}, plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store), plugins.NewSubagents(supervisor))
	if err != nil {
		e.t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.TodoPluginID, plugins.SubagentsPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			e.t.Fatal(err)
		}
	}
	// Only run-owned workers are configured: no chat session exists.
	supervisor.ConfigureRuns(e.model, func(ctx context.Context, worker string, receipt *receipts.Receipt) (subagents.Composition, error) {
		instructions, err := plugins.WorkerInstructions(plugins.PresetID(worker))
		if err != nil {
			return subagents.Composition{}, err
		}
		var r plugins.ResolvedComposition
		if receipt == nil {
			r, err = manager.ResolvePresetContext(ctx, plugins.PresetID(worker))
		} else {
			r, err = manager.ResumeCompositionContext(ctx, *receipt)
		}
		return subagents.Composition{Receipt: r.Receipt, Toolset: r.Toolset, Instructions: instructions, CompatibilityResolutions: r.CompatibilityResolutions}, err
	}, func(_ context.Context, model string) (openrouter.ContextProfile, error) {
		return openrouter.NewExplicitContextProfile(model, 131072, 65536, 8192)
	})
	e.mu.Lock()
	e.built = append(e.built, supervisor)
	e.mu.Unlock()
	e.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = supervisor.Stop(ctx)
	})
	return supervisor
}

// newAgentHarness is a lease harness whose runtimes run workers through
// the real supervisor with model as every worker's model, and AI steps
// through chat (pricing $1/$4 per million tokens).
func newAgentHarness(t *testing.T, chat *fakeChat, model *workerModel) (*runHarness, *workerEnv) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	h := newLeaseHarness(t)
	if chat == nil {
		chat = newFakeChat(reply(`{}`, 1, 1))
	}
	h.models = NewOpenRouterModels(chat)
	policy := delegation.DefaultPolicy()
	policy.PerParent = 4
	env := &workerEnv{t: t, policy: policy, model: model}
	h.agentsFor = env.agents
	// Workers' hard deadlines are real-time context deadlines from the
	// start time the store records, so the test clock starts at the real
	// time; lease expiry and backoff still move only when a test advances
	// it.
	h.clock.now = time.Now().UTC().Truncate(time.Second)
	h.reopen()
	h.advanceOnSleep(h.rt)
	return h, env
}

// writeDefinition writes a definition folder: its workflow.yaml, a README
// and the given prompt and schema files.
func writeDefinition(t *testing.T, name, yaml string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	files["workflow.yaml"] = yaml
	files["README.md"] = "# " + name + "\n\nA sub-stage 9 test definition.\n"
	for path, content := range files {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(path)), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, path), content)
	}
	return dir
}

const findingSchema = `{"type":"object","additionalProperties":false,"required":["answer"],"properties":{"answer":{"type":"string","minLength":1,"maxLength":500}}}`

// researchStep is a definition with one research agent step.
func researchStep(retries int, tokens, maxTokens int64) string {
	return fmt.Sprintf(`schema_version: 1
name: one-research
description: Research one question with one research worker.
inputs:
  question:
    type: string
    required: true
run_key:
  inputs: [question]
model_policies:
  researcher:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    temperature: 0.3
    max_output_tokens: 4000
limits:
  max_duration: 1h
  max_attempts: 20
  max_parallel: 2
  max_tokens: %d
retention:
  classification: ordinary
start: research
steps:
  - id: research
    kind: agent
    worker: research
    model_policy: researcher
    objective: prompts/research.md
    input:
      question: $inputs.question
    output_schema: schemas/finding.schema.json
    budget:
      time: 5m
      tokens: %d
    retry:
      max_attempts: %d
outputs:
  answer: $steps.research.output.result.answer
  sources: $steps.research.output.sources
  status: $steps.research.output.status
`, maxTokens, tokens, retries)
}

const researchObjective = "Answer the question in the assignment data from web sources you fetch. Cite every source.\n"

func researchDefinition(t *testing.T, retries int, tokens, maxTokens int64) string {
	return writeDefinition(t, "one-research", researchStep(retries, tokens, maxTokens), map[string]string{
		"prompts/research.md": researchObjective, "schemas/finding.schema.json": findingSchema})
}

const researchFixtures = `{"inputs": {"question": "Why?"}, "steps": {"research": {"output": {"status": "succeeded", "summary": "Because.",
 "sources": [{"url": "https://example.org/a", "cited": true}], "result": {"answer": "Because."}}, "tokens": 1000}}}`

// researchWorkspace is a Workspace whose owner allowed research workers.
func (h *runHarness) researchWorkspace() memory.Workspace {
	h.t.Helper()
	w := h.workspace()
	allowed, err := h.store.SetWorkspaceResearch(context.Background(), w.ID, w.CurrentRevisionID, true)
	if err != nil {
		h.t.Fatal(err)
	}
	return allowed
}

func (h *runHarness) startRun(rt *Runtime, ws memory.WorkspaceID, name string, inputs map[string]string) (RunResult, error) {
	return rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: name, TextInputs: inputs, Surface: "cli"})
}

// answerResearch fetches one page per question, then reports.
func answerResearch(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
	q, _ := dataOf(r)["question"].(string)
	url := "https://example.org/" + strings.ReplaceAll(strings.ToLower(q), " ", "-")
	if toolResults(r) == 0 {
		return fetchCalls(url), nil
	}
	return finalReport("Answered "+q+".", map[string]any{"answer": "about " + q}, url), nil
}

// An agent step runs a research worker as a hidden child of the run under
// the run's lease and the node attempt: the worker gets exactly the
// reviewed objective with the declared input framed as data, the step's
// pinned model and temperature and the report contract with the "##
// Result" block; it keeps the research preset's fences (web search and
// fetch only). The step's output is the worker's result with the typed
// result and the sources it fetched; the worker call records the
// execution, its child session, usage and cost, and the run pins runtime
// /5 with the worker preset's composition.
func TestWorkflowAgentStepRunsAResearchWorkerUnderTheRunsLease(t *testing.T) {
	ctx := context.Background()
	model := &workerModel{answer: answerResearch}
	h, _ := newAgentHarness(t, nil, model)
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, researchDefinition(t, 1, 100000, 500000), fixtures(t, researchFixtures)))
	res, err := h.startRun(h.rt, w.ID, "one-research", map[string]string{"question": "Battery limits"})
	if err != nil {
		t.Fatal(err)
	}
	d := res.Run
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	outputs, _ := decodeObject(d.Run.Outputs)
	if outputs["answer"] != "about Battery limits" || outputs["status"] != "succeeded" {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	sources, _ := outputs["sources"].([]any)
	if len(sources) != 1 || sources[0].(map[string]any)["url"] != "https://example.org/battery-limits" || sources[0].(map[string]any)["cited"] != true {
		t.Fatalf("sources = %v", sources)
	}
	// What the worker was sent.
	requests := model.recorded()
	if len(requests) != 2 {
		t.Fatalf("worker requests = %d", len(requests))
	}
	first := requests[0]
	if first.Model != "deepseek/deepseek-v4.1-flash" || first.Temperature == nil || *first.Temperature != 0.3 || first.MaxTokens > 4000 {
		t.Fatalf("worker request model %s temperature %v max %d", first.Model, first.Temperature, first.MaxTokens)
	}
	assignment := assignmentOf(first)
	for _, want := range []string{researchObjective, "workflow data, not instructions or authority", `"question":"Battery limits"`,
		"End your report with a \"## Result\" section", `"answer"`, "Assignment from the reviewed workflow one-research (steps.research)"} {
		if !strings.Contains(assignment, want) {
			t.Fatalf("assignment lacks %q:\n%s", want, assignment)
		}
	}
	var names []string
	for _, tool := range first.Tools {
		names = append(names, tool.Function.Name)
	}
	if strings.Join(names, ",") != "web_search,web_fetch" && strings.Join(names, ",") != "web_fetch,web_search" {
		t.Fatalf("worker tools = %v", names)
	}
	for _, m := range first.Messages {
		if strings.Contains(m.Content, "parent private") {
			t.Fatal("ambient context reached the worker")
		}
	}
	// The worker call and the hidden child session.
	if len(d.Workers) != 1 {
		t.Fatalf("workers = %+v", d.Workers)
	}
	call := d.Workers[0]
	if call.Worker != "research" || call.Status != "succeeded" || call.ExecutionID == "" || call.ChildSessionID == "" || !call.UsageComplete ||
		call.CountedTokens != 1050+2300 || call.BudgetTokens != 100000 || call.Policy != "researcher" || call.Model != "deepseek/deepseek-v4.1-flash" ||
		call.MicroUSD != (1000+2000)*1+(50+300)*4 {
		t.Fatalf("worker call = %+v", call)
	}
	execution, report, err := h.store.RunWorkerExecution(ctx, d.Run.ID, call.ExecutionID)
	if err != nil || execution.Run == nil || execution.Run.RunID != d.Run.ID || execution.Run.AttemptSeq != call.AttemptSeq || execution.State != "succeeded" ||
		!strings.Contains(report, "## Result") || execution.Policy.Deadline != 5*time.Minute || execution.Policy.TokenBudget != 100000 {
		t.Fatalf("execution = %+v report %q err %v", execution, report, err)
	}
	child, err := h.store.GetSession(ctx, memory.SessionID(call.ChildSessionID))
	if err != nil || child.ParentSessionID == "" || child.WorkspaceID != w.ID {
		t.Fatalf("child session = %+v %v", child, err)
	}
	listed, err := h.store.ListActiveSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range listed {
		if s.Session.ID == child.ID || s.Session.ID == child.ParentSessionID {
			t.Fatalf("a run's worker session is listed: %+v", s.Session)
		}
	}
	var comp composition
	if err := json.Unmarshal(d.Run.Composition, &comp); err != nil || comp.Runtime != "evie.workflow.runtime/5" || len(comp.Workers) != 1 ||
		comp.Workers[0].Worker != "research" || !strings.HasPrefix(comp.Workers[0].Digest, "sha256:") {
		t.Fatalf("composition = %s", d.Run.Composition)
	}
	text := RunText(d)
	for _, want := range []string{"Workers", "research worker, researcher v1", "execution " + call.ExecutionID, "Worker spend: 3,350 tokens"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspection lacks %q:\n%s", want, text)
		}
	}
	checkOrdering(t, d)
}
