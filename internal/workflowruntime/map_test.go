package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

// Map steps (Stage 14 sub-stage 9): an AI step plans a list of topics and
// a map runs one research worker per topic, at most max_parallel at once
// (never more than the operator's worker caps), with the item failure
// policy, max_items checked before any item starts, input-ordered output,
// per-item recovery after a crash, the owner's cancellation reaching every
// item in flight, and the run's token limit counting every worker.

type fanOut struct {
	maxItems, parallel int
	policy             string
	minSucceeded       int
	retries            int
	budget, maxTokens  int64
}

func (f fanOut) yaml() string {
	failure := "on_item_failure: fail"
	if f.policy == "partial" {
		failure = fmt.Sprintf("on_item_failure: partial\n    min_succeeded: %d", f.minSucceeded)
	}
	return fmt.Sprintf(`schema_version: 1
name: fan-out
description: Research each planned topic with its own research worker.
inputs:
  question:
    type: string
    required: true
run_key:
  inputs: [question]
model_policies:
  planner:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    temperature: 0
    max_output_tokens: 200
  researcher:
    version: 1
    provider: openrouter
    model: deepseek/deepseek-v4.1-flash
    temperature: 0.3
    max_output_tokens: 4000
limits:
  max_duration: 1h
  max_attempts: 40
  max_parallel: 4
  max_tokens: %d
retention:
  classification: ordinary
start: plan
steps:
  - id: plan
    kind: ai
    model_policy: planner
    prompt: prompts/plan.md
    input:
      question: $inputs.question
    output_schema: schemas/topics.schema.json
    next: research
  - id: research
    kind: map
    over: $steps.plan.output.topics
    max_items: %d
    max_parallel: %d
    %s
    item:
      kind: agent
      worker: research
      model_policy: researcher
      objective: prompts/research.md
      input:
        topic: $item
      output_schema: schemas/finding.schema.json
      budget:
        time: 5m
        tokens: %d
      retry:
        max_attempts: %d
outputs:
  findings: $steps.research.output.results
  failures: $steps.research.output.failures
  succeeded: $steps.research.output.succeeded
`, f.maxTokens, f.maxItems, f.parallel, failure, f.budget, f.retries)
}

const topicsSchema = `{"type":"object","additionalProperties":false,"required":["topics"],"properties":{"topics":{"type":"array","items":{"type":"string","minLength":1,"maxLength":40}}}}`

func (f fanOut) definition(t *testing.T) string {
	if f.budget == 0 {
		f.budget = 100000
	}
	if f.maxTokens == 0 {
		f.maxTokens = 2000000
	}
	if f.retries == 0 {
		f.retries = 1
	}
	return writeDefinition(t, "fan-out", f.yaml(), map[string]string{"prompts/plan.md": "Plan research topics for the question.\n",
		"prompts/research.md":        "Research the topic in the assignment data. Cite what you fetch.\n",
		"schemas/topics.schema.json": topicsSchema, "schemas/finding.schema.json": findingSchema})
}

const fanOutFixtures = `{"inputs": {"question": "Q"}, "steps": {"plan": {"output": {"topics": ["a", "b", "c"]}, "tokens": 100},
 "research": {"item_default": {"output": {"status": "succeeded", "summary": "s", "sources": [], "result": {"answer": "x"}}, "tokens": 1000}}}}`

// planning is a model for the plan step that answers with topics.
func planning(topics ...string) *fakeChat {
	b, _ := json.Marshal(map[string]any{"topics": topics})
	return newFakeChat(reply(string(b), 100, 20))
}

func topicURL(topic string) string { return "https://example.org/" + topic }

// topicGate plays every topic's research worker: its first call waits
// until the test releases that topic (or the worker is stopped), counting
// how many workers are inside at once; then it fetches the topic's page
// and reports. A topic in fail always ends in a provider error.
type topicGate struct {
	mu      sync.Mutex
	active  int
	peak    int
	calls   map[string]int
	entered chan string
	release map[string]chan struct{}
	fail    map[string]bool
	open    bool
}

func newTopicGate(topics ...string) *topicGate {
	g := &topicGate{calls: map[string]int{}, entered: make(chan string, 64), release: map[string]chan struct{}{}, fail: map[string]bool{}}
	for _, topic := range topics {
		g.release[topic] = make(chan struct{})
	}
	return g
}

func (g *topicGate) answer(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
	topic, _ := dataOf(r)["topic"].(string)
	if toolResults(r) > 0 {
		g.mu.Lock()
		g.active--
		g.mu.Unlock()
		return finalReport("Found "+topic+".", map[string]any{"answer": "about " + topic}, topicURL(topic)), nil
	}
	g.mu.Lock()
	g.calls[topic]++
	failing, gate, open := g.fail[topic], g.release[topic], g.open
	g.active++
	g.peak = max(g.peak, g.active)
	g.mu.Unlock()
	g.entered <- topic
	if failing {
		g.mu.Lock()
		g.active--
		g.mu.Unlock()
		return openrouter.ChatResponse{}, errors.New("provider unavailable")
	}
	if gate != nil && !open {
		select {
		case <-gate:
		case <-ctx.Done():
			g.mu.Lock()
			g.active--
			g.mu.Unlock()
			return openrouter.ChatResponse{}, ctx.Err()
		}
	}
	return fetchCalls(topicURL(topic)), nil
}

func (g *topicGate) let(topic string) { close(g.release[topic]) }

func (g *topicGate) wait(t *testing.T, n int) []string {
	t.Helper()
	var got []string
	for range n {
		select {
		case topic := <-g.entered:
			got = append(got, topic)
		case <-time.After(10 * time.Second):
			t.Fatalf("waited for %d workers to start; %v started", n, got)
		}
	}
	return got
}

func (g *topicGate) snapshot() (peak int, calls map[string]int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	copied := map[string]int{}
	for k, v := range g.calls {
		copied[k] = v
	}
	return g.peak, copied
}

func (h *runHarness) approveFanOut(f fanOut) memory.Workspace {
	h.t.Helper()
	raw := fanOutFixtures
	if f.maxItems < 3 {
		raw = strings.Replace(raw, `["a", "b", "c"]`, `["a"]`, 1)
	}
	w := h.researchWorkspace()
	h.approve(h.propose(w.ID, f.definition(h.t), fixtures(h.t, raw)))
	return w
}

// runAsync starts a foreground run in rt on another goroutine.
func (h *runHarness) runAsync(rt *Runtime, ws memory.WorkspaceID) <-chan RunResult {
	done := make(chan RunResult, 1)
	go func() {
		res, err := h.startRun(rt, ws, "fan-out", map[string]string{"question": "Q"})
		if err != nil {
			res.Run.Run.Error = "start error: " + err.Error()
		}
		done <- res
	}()
	return done
}

func waitRun(t *testing.T, done <-chan RunResult) RunResult {
	t.Helper()
	select {
	case res := <-done:
		return res
	case <-time.After(20 * time.Second):
		t.Fatal("the run did not finish")
	}
	return RunResult{}
}

func findings(t *testing.T, d eviedb.WorkflowRunDetail) []string {
	t.Helper()
	outputs, err := decodeObject(d.Run.Outputs)
	if err != nil {
		t.Fatalf("outputs %s: %v", d.Run.Outputs, err)
	}
	var out []string
	list, _ := outputs["findings"].([]any)
	for _, f := range list {
		m := f.(map[string]any)
		out = append(out, fmt.Sprintf("%v:%v:%v", m["index"], m["item"], m["output"].(map[string]any)["result"].(map[string]any)["answer"]))
	}
	return out
}

// Items run at most max_parallel at once and their results are in input
// order whatever order they finish in; the operator's worker caps lower
// the parallelism further, never raise it.
func TestWorkflowMapRunsItemsBoundedAndKeepsInputOrder(t *testing.T) {
	for _, tc := range []struct {
		name                string
		parallel, perParent int
		want                int
	}{{"max_parallel bounds it", 2, 4, 2}, {"worker caps bound it", 4, 3, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			gate := newTopicGate("a", "b", "c", "d")
			h, env := newAgentHarness(t, planning("a", "b", "c", "d"), &workerModel{answer: gate.answer})
			env.policy.PerParent = tc.perParent
			h.reopen()
			h.advanceOnSleep(h.rt)
			w := h.approveFanOut(fanOut{maxItems: 4, parallel: tc.parallel, policy: "fail"})
			done := h.runAsync(h.rt, w.ID)
			first := gate.wait(t, tc.want)
			select {
			case extra := <-gate.entered:
				t.Fatalf("worker %s started beyond the bound of %d", extra, tc.want)
			case <-time.After(100 * time.Millisecond):
			}
			// Finish in reverse order of start.
			for i := len(first) - 1; i >= 0; i-- {
				gate.let(first[i])
			}
			for _, topic := range []string{"a", "b", "c", "d"} {
				if !contains(first, topic) {
					gate.wait(t, 1)
					gate.let(topic)
				}
			}
			res := waitRun(t, done)
			d := h.inspect(res.Run.Run.ID)
			if d.Run.State != eviedb.WorkflowRunSucceeded {
				t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
			}
			want := []string{"0:a:about a", "1:b:about b", "2:c:about c", "3:d:about d"}
			if got := findings(t, d); strings.Join(got, ",") != strings.Join(want, ",") {
				t.Fatalf("findings = %v", got)
			}
			if peak, _ := gate.snapshot(); peak != tc.want {
				t.Fatalf("peak = %d, want %d", peak, tc.want)
			}
			views := MapViews(d)
			if len(views) != 1 || len(views[0].Items) != 4 || views[0].Items[2].State != "succeeded" || views[0].Items[2].Execution == "" || len(d.Workers) != 4 {
				t.Fatalf("map views = %+v workers %d", views, len(d.Workers))
			}
			text := RunText(d)
			for _, want := range []string{"research item 0 attempt 1: succeeded", "Map steps.research from checkpoint 1", "item 3 \"d\": succeeded after 1 attempt, worker execution"} {
				if !strings.Contains(text, want) {
					t.Fatalf("inspection lacks %q:\n%s", want, text)
				}
			}
			checkOrdering(t, d)
		})
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// With on_item_failure: fail, the first item that fails after its retries
// fails the map and the run: the item running beside it is cancelled and
// recorded interrupted, and items not started never start.
func TestWorkflowMapFailPolicyStopsAtTheFirstFailedItem(t *testing.T) {
	gate := newTopicGate("a", "b", "c", "d")
	gate.fail["b"] = true
	h, _ := newAgentHarness(t, planning("a", "b", "c", "d"), &workerModel{answer: gate.answer})
	w := h.approveFanOut(fanOut{maxItems: 4, parallel: 2, policy: "fail", retries: 2})
	done := h.runAsync(h.rt, w.ID)
	res := waitRun(t, done)
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(d.Run.Error, "steps.research: item 1 failed: steps.research.item failed after 2 attempts") {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	_, calls := gate.snapshot()
	if calls["b"] != 2 || calls["c"] != 0 || calls["d"] != 0 {
		t.Fatalf("worker calls = %v", calls)
	}
	views := MapViews(d)[0].Items
	if views[0].State != string(eviedb.WorkflowAttemptInterrupted) || !strings.Contains(views[0].Error, "an item failed and the map stops at its first failed item") ||
		views[1].State != string(eviedb.WorkflowAttemptFailed) || views[1].Attempts != 2 || views[2].State != "not started" || views[3].State != "not started" {
		t.Fatalf("items = %+v", views)
	}
	for _, c := range d.Workers {
		if c.FinishedAt == nil {
			t.Fatalf("an open worker call after the run failed: %+v", c)
		}
	}
	checkOrdering(t, d)
}

// With partial, the map succeeds when at least min_succeeded items
// succeeded, listing failed items after the succeeded ones; with fewer it
// fails the run.
func TestWorkflowMapPartialPolicyNeedsMinSucceeded(t *testing.T) {
	for _, tc := range []struct {
		min   int
		state eviedb.WorkflowRunState
	}{{2, eviedb.WorkflowRunSucceeded}, {3, eviedb.WorkflowRunFailed}} {
		t.Run(fmt.Sprintf("min_succeeded %d", tc.min), func(t *testing.T) {
			gate := newTopicGate()
			gate.open = true
			gate.fail["b"] = true
			h, _ := newAgentHarness(t, planning("a", "b", "c"), &workerModel{answer: gate.answer})
			w := h.approveFanOut(fanOut{maxItems: 3, parallel: 3, policy: "partial", minSucceeded: tc.min})
			res := waitRun(t, h.runAsync(h.rt, w.ID))
			d := h.inspect(res.Run.Run.ID)
			if d.Run.State != tc.state {
				t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
			}
			if tc.state == eviedb.WorkflowRunFailed {
				if d.Run.Error != "steps.research: 2 of 3 items succeeded; min_succeeded 3" {
					t.Fatalf("error = %q", d.Run.Error)
				}
				return
			}
			if got := findings(t, d); strings.Join(got, ",") != "0:a:about a,2:c:about c" {
				t.Fatalf("findings = %v", got)
			}
			outputs, _ := decodeObject(d.Run.Outputs)
			failures := outputs["failures"].([]any)
			if len(failures) != 1 || failures[0].(map[string]any)["index"] != int64(1) || outputs["succeeded"] != int64(2) ||
				!strings.Contains(failures[0].(map[string]any)["error"].(string), "worker execution") {
				t.Fatalf("outputs = %s", d.Run.Outputs)
			}
		})
	}
}

// A list longer than max_items fails the map before any item starts; it is
// never truncated.
func TestWorkflowMapLongerListFailsBeforeAnyItemStarts(t *testing.T) {
	gate := newTopicGate()
	gate.open = true
	h, _ := newAgentHarness(t, planning("a", "b", "c"), &workerModel{answer: gate.answer})
	w := h.approveFanOut(fanOut{maxItems: 2, parallel: 2, policy: "fail"})
	res := waitRun(t, h.runAsync(h.rt, w.ID))
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunFailed || d.Run.Error != "steps.research: the list has 3 items; max_items 2, so no item started" ||
		len(d.Workers) != 0 || len(gate.calls) != 0 {
		t.Fatalf("run = %+v workers %d calls %v", d.Run, len(d.Workers), gate.calls)
	}
	last := d.Attempts[len(d.Attempts)-1]
	if last.Kind != "map" || last.State != eviedb.WorkflowAttemptFailed || last.Item != nil {
		t.Fatalf("map attempt = %+v", last)
	}
}

// A crash during a map recovers each item from its own record: items that
// finished keep their outputs and their workers never run again; the item
// whose worker the dead process held is recorded interrupted with its
// worker, and runs again under the new holder.
func TestWorkflowMapCrashRecoversEachItemFromItsOwnRecord(t *testing.T) {
	ctx := context.Background()
	gate := newTopicGate("c")
	var h *runHarness
	var dead *process
	var wsID memory.WorkspaceID
	died := make(chan struct{})
	var once sync.Once
	recorded := func() bool {
		runs := h.runs(wsID)
		if len(runs) == 0 {
			return false
		}
		attempts, err := h.store.WorkflowItemAttempts(ctx, runs[0].ID, 1, "research")
		finished := 0
		for _, a := range attempts {
			if a.State == eviedb.WorkflowAttemptSucceeded {
				finished++
			}
		}
		return err == nil && finished == 2
	}
	model := &workerModel{}
	model.answer = func(ctx context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		if topic, _ := dataOf(r)["topic"].(string); topic == "c" && toolResults(r) == 0 && dead != nil {
			first := false
			once.Do(func() { first = true })
			if first {
				// Once a and b are recorded, the process dies inside c's worker:
				// its database closes and the call never returns until the test
				// ends.
				eventually(t, "items a and b to be recorded", recorded)
				_ = dead.db.Close()
				close(died)
				<-ctx.Done()
				return openrouter.ChatResponse{}, ctx.Err()
			}
		}
		return gate.answer(ctx, r)
	}
	gate.open = true
	h, _ = newAgentHarness(t, planning("a", "b", "c"), model)
	w := h.approveFanOut(fanOut{maxItems: 3, parallel: 3, policy: "fail"})
	wsID = w.ID
	dead = h.process()
	h.advanceOnSleep(dead.rt)
	go func() { _, _ = h.startRun(dead.rt, w.ID, "fan-out", map[string]string{"question": "Q"}) }()
	<-died
	id := h.runs(w.ID)[0].ID
	h.clock.Advance(testLease + time.Second)
	next := h.process()
	if end, err := next.rt.execute(ctx, id); err != nil {
		t.Fatalf("pass = %v, %v", end, err)
	}
	stop := next.rt.Start(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for !h.state(id).Terminal() {
		if time.Now().After(deadline) {
			t.Fatalf("the run did not finish under the new holder:\n%s", RunText(h.inspect(id)))
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	if got := findings(t, d); strings.Join(got, ",") != "0:a:about a,1:b:about b,2:c:about c" {
		t.Fatalf("findings = %v", got)
	}
	_, calls := gate.snapshot()
	if calls["a"] != 1 || calls["b"] != 1 || calls["c"] != 1 {
		t.Fatalf("worker calls = %v (a and b must not run again; c ran once more after the crash)", calls)
	}
	views := MapViews(d)[0].Items
	if views[0].Attempts != 1 || views[1].Attempts != 1 || views[2].Attempts != 2 || views[2].State != "succeeded" {
		t.Fatalf("items = %+v", views)
	}
	var interrupted []eviedb.WorkflowNodeAttempt
	for _, a := range d.Attempts {
		if a.State == eviedb.WorkflowAttemptInterrupted {
			interrupted = append(interrupted, a)
		}
	}
	if len(interrupted) != 2 || interrupted[0].Kind != "map" || interrupted[1].Item == nil || *interrupted[1].Item != 2 {
		t.Fatalf("interrupted = %+v", interrupted)
	}
	// The dead holder's worker was settled with the claim.
	for _, c := range d.Workers {
		if c.FinishedAt == nil || c.ExecutionID == "" {
			t.Fatalf("worker call = %+v", c)
		}
		if c.AttemptSeq == interrupted[1].Seq {
			exec, _, err := h.store.RunWorkerExecution(ctx, id, c.ExecutionID)
			if err != nil || exec.State != "interrupted" || exec.Result.Reason != "run_ownership_ended" || c.Status != "interrupted" {
				t.Fatalf("the dead holder's worker = %+v %v call %+v", exec, err, c)
			}
		}
	}
	checkOrdering(t, d)
}

// The owner's cancellation reaches every item in flight: running workers
// are cancelled and their items recorded interrupted, items not started
// never start, and the run is cancelled only after every item ended.
func TestWorkflowMapCancellationReachesEveryItemInFlight(t *testing.T) {
	gate := newTopicGate("a", "b", "c")
	h, _ := newAgentHarness(t, planning("a", "b", "c"), &workerModel{answer: gate.answer})
	w := h.approveFanOut(fanOut{maxItems: 3, parallel: 2, policy: "fail"})
	done := h.runAsync(h.rt, w.ID)
	gate.wait(t, 2)
	id := h.runs(w.ID)[0].ID
	owner := h.process()
	if _, err := owner.rt.Cancel(context.Background(), CancelRequest{RunID: id, Reason: "enough", Surface: "cli", Detach: true}); err != nil {
		t.Fatal(err)
	}
	res := waitRun(t, done)
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunCancelled || d.Cancellation == nil {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	views := MapViews(d)[0].Items
	if views[0].State != string(eviedb.WorkflowAttemptInterrupted) || views[1].State != string(eviedb.WorkflowAttemptInterrupted) || views[2].State != "not started" ||
		!strings.Contains(views[0].Error, "the owner cancelled the run") {
		t.Fatalf("items = %+v", views)
	}
	_, calls := gate.snapshot()
	if calls["c"] != 0 {
		t.Fatalf("an item started after the cancellation: %v", calls)
	}
	for _, c := range d.Workers {
		exec, _, err := h.store.RunWorkerExecution(context.Background(), d.Run.ID, c.ExecutionID)
		if err != nil || c.FinishedAt == nil || !exec.Terminal() || (exec.State != "cancelled" && exec.State != "interrupted") {
			t.Fatalf("worker %+v execution %+v %v", c, exec, err)
		}
	}
	last := d.Attempts[len(d.Attempts)-1]
	mapAttempt := d.Attempts[1]
	if mapAttempt.Kind != "map" || mapAttempt.State != eviedb.WorkflowAttemptInterrupted || !strings.Contains(mapAttempt.Error, "the owner cancelled the run during the map") ||
		last.FinishedAt == nil {
		t.Fatalf("map attempt = %+v", mapAttempt)
	}
}

// Every worker counts against limits.max_tokens: a worker whose usage was
// not reported counts its whole budget, and an item whose budget would
// pass the limit is not started; with nothing else running the run fails
// visibly, and inspection shows the spend.
func TestWorkflowMapTokenLimitStopsItemsAndFailsVisibly(t *testing.T) {
	model := &workerModel{answer: func(_ context.Context, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		topic, _ := dataOf(r)["topic"].(string)
		report := finalReport("Found "+topic+".", map[string]any{"answer": "about " + topic})
		report.Usage = nil // no usage reported: the worker counts its budget
		return report, nil
	}}
	h, _ := newAgentHarness(t, planning("a", "b", "c"), model)
	w := h.approveFanOut(fanOut{maxItems: 3, parallel: 1, policy: "fail", budget: 100000, maxTokens: 250000})
	res := waitRun(t, h.runAsync(h.rt, w.ID))
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(d.Run.Error, "steps.research item 2: its next attempt could use up to 100,000 tokens") ||
		!strings.Contains(d.Run.Error, "limits.max_tokens 250,000; it was not started") {
		t.Fatalf("run = %+v\n%s", d.Run, RunText(d))
	}
	if len(d.Workers) != 2 || d.Workers[0].CountedTokens != 100000 || d.Workers[0].UsageComplete {
		t.Fatalf("workers = %+v", d.Workers)
	}
	if text := RunText(d); !strings.Contains(text, "Worker spend: 200,000 tokens") || !strings.Contains(text, "Run spend:") {
		t.Fatalf("inspection:\n%s", text)
	}
}
