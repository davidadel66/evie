package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/workflow"
)

// The effect protocol (Stage 14 sub-stage 6). Every external effect follows
// the durable intent-call-receipt-checkpoint sequence: the node attempt and
// its Effect Intent (stable idempotency key, Resource Conflict Keys and the
// authority evaluation) are recorded under the fenced lease before the
// lease is checked and the provider is called, and the provider's response
// is recorded as an Effect Receipt with the normalized output and the next
// checkpoint. An intent without a receipt is Outcome Unknown: the run needs
// reconciliation, dependent nodes are blocked, nothing resumes it by itself
// and nothing retries it by policy. Fake providers behind the focused
// effect role model success, deterministic failure, timeouts before and
// after acceptance, idempotency replay, lookup, an unavailable dependency,
// conflicting resources and malformed responses. Every test uses a real
// temporary SQLite file; ordering is observed through a second connection.

var postShareDir = filepath.Join(testdata, "post-share")

const (
	postID       = "ledger.post"
	householdKey = "conn-ledger/account:household"
)

// postContract is the test ledger.post contract.
func postContract(idempotent, reconciles bool) workflow.CapabilityContract {
	return workflow.CapabilityContract{ID: postID, Contract: "1.0.0", Effect: true, Idempotent: idempotent, Reconciles: reconciles, ConflictKeys: "resource",
		Input: workflow.ObjectType(
			workflow.Field{Name: "bill_id", Type: workflow.StringType()},
			workflow.Field{Name: "amount_cents", Type: workflow.IntegerType()},
		),
		Output: workflow.ObjectType(
			workflow.Field{Name: "entry_id", Type: workflow.StringType()},
			workflow.Field{Name: "balance_cents", Type: workflow.IntegerType()},
		),
		Connections: []string{"conn-ledger"}, Resources: []string{"account:household", "account:travel"},
	}
}

// fakeLedger is the external system behind the fake ledger.post provider:
// it applies each idempotency key at most once, as an idempotent provider
// promises.
type fakeLedger struct {
	mu       sync.Mutex
	balance  map[string]int64
	entries  map[string]map[string]any
	performs []plugins.WorkflowEffectRequest
	lookups  []plugins.WorkflowEffectRequest
	// answers answer performs in turn (the last repeats); a nil answer
	// applies the posting and returns its entry.
	answers []effectAnswer
	// inFlight and maxInFlight count performs running at once.
	inFlight, maxInFlight int
}

type effectAnswer func(l *fakeLedger, ctx context.Context, req plugins.WorkflowEffectRequest) (any, error)

func newFakeLedger(answers ...effectAnswer) *fakeLedger {
	return &fakeLedger{balance: map[string]int64{}, entries: map[string]map[string]any{}, answers: answers}
}

// apply posts the request once per idempotency key and returns its entry.
func (l *fakeLedger) apply(req plugins.WorkflowEffectRequest) map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[req.IdempotencyKey]; ok {
		return e
	}
	l.balance[req.Resource] += req.Input["amount_cents"].(int64)
	e := map[string]any{"entry_id": fmt.Sprintf("entry-%d", len(l.entries)+1), "balance_cents": l.balance[req.Resource]}
	l.entries[req.IdempotencyKey] = e
	return e
}

func (l *fakeLedger) perform(ctx context.Context, req plugins.WorkflowEffectRequest) (any, error) {
	l.mu.Lock()
	l.performs = append(l.performs, req)
	l.inFlight++
	l.maxInFlight = max(l.maxInFlight, l.inFlight)
	var answer effectAnswer
	if len(l.answers) > 0 {
		answer = l.answers[min(len(l.performs), len(l.answers))-1]
	}
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		l.inFlight--
		l.mu.Unlock()
	}()
	if answer == nil {
		return l.apply(req), nil
	}
	return answer(l, ctx, req)
}

// lookupByKey is reconciliation by lookup: whether the ledger has the key.
func (l *fakeLedger) lookupByKey(_ context.Context, req plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lookups = append(l.lookups, req)
	if e, ok := l.entries[req.IdempotencyKey]; ok {
		return plugins.WorkflowEffectLookup{Outcome: plugins.WorkflowEffectHappened, Output: e, Evidence: "ledger entry " + e["entry_id"].(string) + " carries the key"}, nil
	}
	return plugins.WorkflowEffectLookup{Outcome: plugins.WorkflowEffectDidNotHappen, Evidence: "no ledger entry carries the key"}, nil
}

// replay is reconciliation by idempotency replay: the same request is sent
// again with the same key, which the ledger applies at most once.
func (l *fakeLedger) replay(_ context.Context, req plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
	l.mu.Lock()
	l.lookups = append(l.lookups, req)
	l.mu.Unlock()
	e := l.apply(req)
	return plugins.WorkflowEffectLookup{Outcome: plugins.WorkflowEffectHappened, Output: e, Evidence: "replayed with the same idempotency key"}, nil
}

func (l *fakeLedger) counts() (performs, lookups int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.performs), len(l.lookups)
}

func (l *fakeLedger) balanceOf(resource string) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.balance[resource]
}

// refuse is a provider that proves nothing happened.
func refuse(message string) effectAnswer {
	return func(_ *fakeLedger, _ context.Context, _ plugins.WorkflowEffectRequest) (any, error) {
		return nil, plugins.EffectNotPerformed(errors.New(message))
	}
}

// timeoutBeforeAcceptance waits out the call and reports, after the
// deadline, that the request was never accepted.
func timeoutBeforeAcceptance(_ *fakeLedger, ctx context.Context, _ plugins.WorkflowEffectRequest) (any, error) {
	<-ctx.Done()
	return nil, plugins.EffectNotPerformed(fmt.Errorf("never connected: %w", ctx.Err()))
}

// timeoutAfterAcceptance applies the posting and never answers in time.
func timeoutAfterAcceptance(l *fakeLedger, ctx context.Context, req plugins.WorkflowEffectRequest) (any, error) {
	l.apply(req)
	<-ctx.Done()
	return nil, ctx.Err()
}

// lostRequest never reaches the ledger and never answers in time.
func lostRequest(_ *fakeLedger, ctx context.Context, _ plugins.WorkflowEffectRequest) (any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// malformed applies the posting and answers outside the contract.
func malformed(l *fakeLedger, _ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
	e := l.apply(req)
	return map[string]any{"entry_id": e["entry_id"], "balance_cents": "a lot"}, nil
}

// newEffectHarness is a lease harness whose runtimes reach ledger.post
// through a fake provider over ledger, reconciling with reconcile (nil:
// only the owner reconciles), with a short effect timeout.
func newEffectHarness(t *testing.T, ledger *fakeLedger, idempotent bool, reconcile plugins.WorkflowEffectReconcile) (*runHarness, *fakeProviders) {
	t.Helper()
	fp := &fakeProviders{contracts: map[string]workflow.CapabilityContract{}, live: map[string]*fakeProvider{
		postID: {plugin: "ledger", version: "1.0.0", contract: postContract(idempotent, reconcile != nil), perform: ledger.perform, reconcile: reconcile},
	}}
	h := newLeaseHarness(t)
	h.providers = fp
	h.scheduler.EffectTimeout = 40 * time.Millisecond
	h.reopen()
	h.advanceOnSleep(h.rt)
	return h, fp
}

// ledgerWorkspace is a Workspace whose access allows the ledger accounts.
func (h *harness) ledgerWorkspace(resources ...string) memory.Workspace {
	h.t.Helper()
	w, err := h.store.RegisterWorkspace(context.Background(), "Household")
	if err != nil {
		h.t.Fatal(err)
	}
	if len(resources) == 0 {
		resources = []string{"account:household"}
	}
	access := workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: "conn-ledger", Resources: resources}}}
	if _, err := h.store.SetWorkspaceAccess(context.Background(), w.ID, 0, access); err != nil {
		h.t.Fatal(err)
	}
	return w
}

func postShareFixtures(t *testing.T) *workflow.Fixtures {
	t.Helper()
	return fixtures(t, readFile(t, filepath.Join(testdata, "post-share.fixtures.json")))
}

// approvePostShare proposes and approves the definition in dir in a new
// Workspace with the given access.
func (h *runHarness) approvePostShare(dir string, resources ...string) memory.Workspace {
	h.t.Helper()
	w := h.ledgerWorkspace(resources...)
	h.approve(h.propose(w.ID, dir, postShareFixtures(h.t)))
	return w
}

// postShareVariant is a copy of post-share with each replacement applied
// to its workflow.yaml.
func postShareVariant(t *testing.T, replacements ...string) string {
	t.Helper()
	dir := copyDefinition(t, "post-share")
	yaml := filepath.Join(dir, "workflow.yaml")
	text := readFile(t, yaml)
	for i := 0; i+1 < len(replacements); i += 2 {
		if !strings.Contains(text, replacements[i]) {
			t.Fatalf("post-share has no %q", replacements[i])
		}
		text = strings.ReplaceAll(text, replacements[i], replacements[i+1])
	}
	writeFile(t, yaml, text)
	return dir
}

// withoutRetry is post-share whose effect allows one attempt.
func withoutRetry(t *testing.T) string {
	return postShareVariant(t, "    retry:\n      max_attempts: 2\n      backoff: 1m\n    next: record", "    next: record")
}

func postInputs(bill string, total, people int64) map[string]json.RawMessage {
	return billInputs(bill, total, people)
}

func (h *runHarness) startPost(rt *Runtime, ws memory.WorkspaceID, bill string, detach bool) (RunResult, error) {
	return rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "post-share", Inputs: postInputs(bill, 1000, 4), Surface: "cli", Detach: detach})
}

func effectStates(d eviedb.WorkflowRunDetail) []eviedb.WorkflowEffectState {
	out := []eviedb.WorkflowEffectState{}
	for _, e := range d.Effects {
		out = append(out, e.State())
	}
	return out
}

func attemptOf(d eviedb.WorkflowRunDetail, seq int64) eviedb.WorkflowNodeAttempt {
	for _, a := range d.Attempts {
		if a.Seq == seq {
			return a
		}
	}
	return eviedb.WorkflowNodeAttempt{}
}

func checkpointAfter(d eviedb.WorkflowRunDetail, seq int64) *eviedb.WorkflowCheckpoint {
	for i := range d.Checkpoints {
		if d.Checkpoints[i].AfterAttempt == seq {
			return &d.Checkpoints[i]
		}
	}
	return nil
}

var postedOutputs = map[string]any{"entry_id": "entry-1", "balance_cents": int64(250)}

// The intent-call-receipt-checkpoint sequence, observed from inside the
// provider call through a second database connection: before the call the
// attempt and its Effect Intent are durable (idempotency key, Resource
// Conflict Key, the authority evaluation counting this call, the provider
// called) with no receipt and no checkpoint after the attempt; afterwards
// the receipt, the output and the next checkpoint are recorded together,
// and the dependent step runs only after them.
func TestWorkflowEffectRecordsIntentBeforeTheCallAndReceiptWithTheCheckpoint(t *testing.T) {
	ctx := context.Background()
	var h *runHarness
	var observer *process
	var seen []string
	ledger := newFakeLedger(func(l *fakeLedger, _ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		runs, err := observer.store.ListWorkflowRuns(ctx, eviedb.WorkflowRunFilter{})
		if err != nil || len(runs) != 1 {
			return nil, fmt.Errorf("runs = %+v, %v", runs, err)
		}
		d, err := observer.store.WorkflowRun(ctx, runs[0].ID)
		if err != nil {
			return nil, err
		}
		if len(d.Attempts) != 2 || d.Attempts[1].Step != "post" || d.Attempts[1].State != eviedb.WorkflowAttemptRunning || len(d.Checkpoints) != 2 {
			return nil, fmt.Errorf("called before the attempt was durable: %+v", summarize(d))
		}
		if len(d.Effects) != 1 {
			return nil, fmt.Errorf("called before the Effect Intent was durable: %+v", d.Effects)
		}
		i := d.Effects[0]
		var authority map[string]any
		_ = json.Unmarshal(i.Authority, &authority)
		if i.State() != eviedb.WorkflowEffectPending || i.Receipt != nil || i.Unknown != nil || i.AttemptSeq != d.Attempts[1].Seq || i.Step != "post" ||
			i.IdempotencyKey != req.IdempotencyKey || !strings.HasPrefix(i.IdempotencyKey, "sha256:") ||
			!reflect.DeepEqual(i.ConflictKeys, []string{householdKey}) || i.Plugin != "ledger" || i.Version != "1.0.0" ||
			i.Capability != postID || i.Connection != "conn-ledger" || i.Resource != "account:household" ||
			string(i.Input) != `{"amount_cents":250,"bill_id":"dinner-1"}` || i.LeaseToken != d.Attempts[1].LeaseToken ||
			authority["calls"] != float64(1) || authority["max_calls_per_run"] != float64(2) || authority["version"] != float64(1) {
			return nil, fmt.Errorf("intent before the call = %+v (authority %s)", i, i.Authority)
		}
		if i.CreatedAt.Before(d.Attempts[1].StartedAt) {
			return nil, errors.New("the intent predates its attempt")
		}
		seen = append(seen, fmt.Sprintf("%s %s %v", req.Connection, req.Resource, req.Input["amount_cents"]))
		return l.apply(req), nil
	})
	h, _ = newEffectHarness(t, ledger, true, nil)
	observer = h.process()
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	h.reopen()
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, d.Run.Outputs), postedOutputs) || !reflect.DeepEqual(seen, []string{"conn-ledger account:household 250"}) {
		t.Fatalf("run = %+v outputs %s seen %v", d.Run, d.Run.Outputs, seen)
	}
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "post", 0},
		{"post", 1, eviedb.WorkflowAttemptSucceeded, "record", 1},
		{"record", 1, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("attempts = %+v", got)
	}
	checkOrdering(t, d)
	i := d.Effects[0]
	r := i.Receipt
	if r == nil || r.Outcome != eviedb.WorkflowEffectSucceeded || r.Source != eviedb.WorkflowEffectFromProvider || r.Actor != "runtime" ||
		string(r.Response) != `{"balance_cents":250,"entry_id":"entry-1"}` || string(d.Attempts[1].Output) != string(r.Response) {
		t.Fatalf("receipt = %+v", r)
	}
	if c := checkpointAfter(d, d.Attempts[1].Seq); c == nil || !c.CreatedAt.Equal(r.RecordedAt) || r.RecordedAt.Before(i.CreatedAt) ||
		d.Attempts[2].StartedAt.Before(r.RecordedAt) {
		t.Fatalf("receipt %v, checkpoint %+v, dependent step %v", r.RecordedAt, c, d.Attempts[2].StartedAt)
	}
	text := RunText(d)
	for _, fragment := range []string{"Effects", "Intent #1 for steps.post attempt 1", "key " + i.IdempotencyKey, "conflict keys " + householdKey,
		"call 1 of 2", "receipt: succeeded from the provider"} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("inspection lacks %q:\n%s", fragment, text)
		}
	}
	var pins []struct {
		Capability string `json:"capability"`
		Effect     bool   `json:"effect"`
		Idempotent bool   `json:"idempotent"`
	}
	var comp struct {
		Runtime   string          `json:"runtime"`
		Providers json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(d.Run.Composition, &comp); err != nil || json.Unmarshal(comp.Providers, &pins) != nil {
		t.Fatalf("composition %s: %v", d.Run.Composition, err)
	}
	if comp.Runtime != RuntimeVersion || len(pins) != 1 || pins[0].Capability != postID || !pins[0].Effect || !pins[0].Idempotent {
		t.Fatalf("pinned providers = %s", d.Run.Composition)
	}
}

// A provider that proves nothing happened (a deterministic failure, or a
// timeout it reports before accepting the request) records a failed
// receipt; the step retries within its Retry Policy after its durable
// backoff, re-sending the same Effect Intent identity, and fails the run
// after its last attempt.
func TestWorkflowEffectProvenFailuresRetryWithTheSameIdempotencyKey(t *testing.T) {
	for _, tc := range []struct {
		name      string
		first     effectAnswer
		wantError string
	}{
		{"deterministic failure", refuse("account frozen"), "account frozen"},
		{"timeout before acceptance", timeoutBeforeAcceptance, "never connected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := newFakeLedger(tc.first, nil)
			h, _ := newEffectHarness(t, ledger, true, nil)
			w := h.approvePostShare(postShareDir)
			res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
			if err != nil {
				t.Fatal(err)
			}
			d := h.inspect(res.Run.Run.ID)
			if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Effects) != 2 ||
				!reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectFailed, eviedb.WorkflowEffectSucceeded}) {
				t.Fatalf("run = %+v effects %+v", d.Run, d.Effects)
			}
			first, second := d.Effects[0], d.Effects[1]
			if first.IdempotencyKey != second.IdempotencyKey || !strings.Contains(first.Receipt.Error, tc.wantError) || first.Receipt.Response != nil ||
				attemptOf(d, first.AttemptSeq).State != eviedb.WorkflowAttemptFailed || second.Attempt != 2 {
				t.Fatalf("effects = %+v / %+v", first, second)
			}
			if gap := attemptOf(d, second.AttemptSeq).StartedAt.Sub(attemptOf(d, first.AttemptSeq).StartedAt); gap < time.Minute {
				t.Fatalf("retried %s after the failure, before the step's backoff", gap)
			}
			if performs, _ := ledger.counts(); performs != 2 || ledger.balanceOf("account:household") != 250 {
				t.Fatalf("performs %d, balance %d", performs, ledger.balanceOf("account:household"))
			}
			checkOrdering(t, d)
		})
	}

	ledger := newFakeLedger(refuse("account frozen"))
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunFailed {
		t.Fatalf("run = %+v, %v", res.Run.Run, err)
	}
	if d := h.inspect(res.Run.Run.ID); !strings.HasPrefix(d.Run.Error, "steps.post failed after 2 attempts: ") || len(d.Effects) != 2 ||
		len(d.Attempts) != 3 || d.Attempts[2].Step != "post" {
		t.Fatalf("failed run = %+v attempts %+v", d.Run, summarize(d))
	}
}

// The idempotency key is the run's logical execution (its Run Key), the
// node and its input: a run started again after a failure sends the same
// key, so an idempotent provider does not post the same share twice; a
// different logical execution sends another.
func TestWorkflowEffectIdempotencyKeyIsStableAcrossRunsOfOneLogicalExecution(t *testing.T) {
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	failRecord := true
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "record" && failRecord {
			return nil, errors.New("the ledger report is unavailable")
		}
		return v.CallFunction(step, input)
	}
	first, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil || first.Run.Run.State != eviedb.WorkflowRunFailed {
		t.Fatalf("first run = %+v, %v", first.Run.Run, err)
	}
	failRecord = false
	again, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil || again.Existing || again.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("second run = %+v, %v", again.Run.Run, err)
	}
	other, err := h.startPost(h.rt, w.ID, "dinner-2", false)
	if err != nil || other.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("other run = %+v, %v", other.Run.Run, err)
	}
	keys := []string{h.inspect(first.Run.Run.ID).Effects[0].IdempotencyKey, h.inspect(again.Run.Run.ID).Effects[0].IdempotencyKey,
		h.inspect(other.Run.Run.ID).Effects[0].IdempotencyKey}
	if keys[0] != keys[1] || keys[0] == keys[2] || ledger.balanceOf("account:household") != 500 {
		t.Fatalf("keys %v, balance %d", keys, ledger.balanceOf("account:household"))
	}
	if performs, _ := ledger.counts(); performs != 3 {
		t.Fatalf("performs %d", performs)
	}
}

// max_calls_per_run is counted by Effect Intents: a retry past it is
// refused before the intent, and the provider is never called again.
func TestWorkflowEffectMaxCallsPerRunCountsIntents(t *testing.T) {
	ledger := newFakeLedger(refuse("account frozen"), nil)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareVariant(t, "max_calls_per_run: 2", "max_calls_per_run: 1"))
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(d.Run.Error, "max_calls_per_run 1") || len(d.Effects) != 1 || len(d.Attempts) != 2 {
		t.Fatalf("run = %+v effects %+v attempts %+v", d.Run, d.Effects, summarize(d))
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("the provider was called %d times", performs)
	}
}

// An effect whose outcome is unknown (a timeout after the provider
// accepted it, a malformed response) is never retried by policy: the run
// needs reconciliation, the dependent step never starts, and no scheduler
// resumes it, however often it polls. Without provider reconciliation the
// owner reconciles it with evidence; the run then continues from the
// receipt without calling the provider again.
func TestWorkflowEffectUnknownOutcomeWaitsForTheOwner(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation || d.Run.NotBefore != nil ||
		!strings.Contains(d.Run.Error, "unknown") || !strings.Contains(d.Run.Error, "evie workflows reconcile "+id+" post --outcome happened|did-not-happen") ||
		!reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectUnknown}) || !strings.Contains(d.Effects[0].Unknown.Reason, "40ms") {
		t.Fatalf("run = %+v effects %+v", d.Run, d.Effects)
	}
	if got := summarize(d); len(got) != 2 || got[1].Step != "post" || got[1].State != eviedb.WorkflowAttemptFailed {
		t.Fatalf("attempts = %+v", got)
	}

	// No scheduler resumes or retries it, however often it polls (the
	// run's limits.max_duration keeps counting meanwhile).
	p := h.process()
	stop := p.rt.Start(ctx)
	for range 3 {
		h.clock.Advance(5 * time.Minute)
		time.Sleep(20 * time.Millisecond)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	due, err := h.store.DueWorkflowRuns(ctx, 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("due = %v, %v", due, err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunNeedsReconciliation || len(d.Attempts) != 2 || len(d.Events) != 0 {
		t.Fatalf("after polling = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("an unknown effect was retried: %d performs", performs)
	}

	// Owner reconciliation: only the owner, only the unknown step, and
	// an outcome that happened needs output within the contract.
	refused := []Reconciliation{
		{RunID: id, Step: "post", Outcome: "happened", Output: json.RawMessage(`{"entry_id":"entry-1","balance_cents":250}`), Surface: "chat"},
		{RunID: id, Step: "record", Outcome: "happened", Output: json.RawMessage(`{"share":1}`), Surface: "cli"},
		{RunID: id, Step: "post", Outcome: "maybe", Surface: "cli"},
		{RunID: id, Step: "post", Outcome: "happened", Output: json.RawMessage(`{"entry_id":"entry-1"}`), Surface: "cli"},
		{RunID: id, Step: "post", Outcome: "happened", Surface: "cli"},
	}
	for i, r := range refused {
		if _, err := h.rt.Reconcile(ctx, r); err == nil {
			t.Fatalf("reconciliation %d was accepted: %+v", i, r)
		}
	}
	if d := h.inspect(id); d.Effects[0].Receipt != nil || d.Run.State != eviedb.WorkflowRunNeedsReconciliation {
		t.Fatalf("a refused reconciliation changed the run: %+v", d.Effects[0])
	}
	reconciled, err := h.rt.Reconcile(ctx, Reconciliation{RunID: id, Step: "post", Outcome: "happened",
		Evidence: "the ledger shows entry-1 for 2.50 on account:household", Output: json.RawMessage(`{"entry_id":"entry-1","balance_cents":250}`), Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, reconciled.Run.Outputs), postedOutputs) {
		t.Fatalf("reconciled run = %+v", reconciled.Run)
	}
	d = h.inspect(id)
	r := d.Effects[0].Receipt
	if r == nil || r.Outcome != eviedb.WorkflowEffectSucceeded || r.Source != eviedb.WorkflowEffectFromOwner || r.Actor != "owner" || r.Surface != "cli" ||
		r.Evidence != "the ledger shows entry-1 for 2.50 on account:household" || string(r.Response) != `{"balance_cents":250,"entry_id":"entry-1"}` {
		t.Fatalf("owner receipt = %+v", r)
	}
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "post", 0},
		{"post", 1, eviedb.WorkflowAttemptFailed, "", 1},
		{"post", 2, eviedb.WorkflowAttemptSucceeded, "record", 1},
		{"record", 1, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) || len(d.Effects) != 1 || !strings.Contains(d.Attempts[2].Note, "Effect Intent #1") {
		t.Fatalf("attempts = %+v effects %+v note %q", got, d.Effects, d.Attempts[2].Note)
	}
	checkOrdering(t, d)
	if performs, _ := ledger.counts(); performs != 1 || ledger.balanceOf("account:household") != 250 {
		t.Fatalf("performs %d", performs)
	}
	if _, err := h.rt.Reconcile(ctx, Reconciliation{RunID: id, Step: "post", Outcome: "did-not-happen", Surface: "cli"}); !errors.Is(err, ErrReconcileRefused) {
		t.Fatalf("reconciling a finished run again: %v", err)
	}
	if text := RunText(d); !strings.Contains(text, "receipt: succeeded from the owner via cli") || !strings.Contains(text, "outcome unknown") {
		t.Fatalf("inspection:\n%s", text)
	}
}

// An owner's "did not happen" lets the step retry only within its Retry
// Policy, re-sending the same Effect Intent identity, which an idempotent
// provider applies at most once; with no attempt left the run fails.
func TestWorkflowEffectOwnerDidNotHappenRetriesOnlyWithinThePolicy(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(malformed, nil)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunNeedsReconciliation || !strings.Contains(d.Effects[0].Unknown.Reason, "malformed response") ||
		string(d.Effects[0].Unknown.Response) != `{"balance_cents":"a lot","entry_id":"entry-1"}` {
		t.Fatalf("malformed response = %+v effects %+v", d.Run, d.Effects)
	}
	reconciled, err := h.rt.Reconcile(ctx, Reconciliation{RunID: id, Step: "post", Outcome: "did-not-happen", Evidence: "not on the statement yet", Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if reconciled.Run.State != eviedb.WorkflowRunSucceeded || len(d.Effects) != 2 ||
		!reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectFailed, eviedb.WorkflowEffectSucceeded}) ||
		d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromOwner || d.Effects[0].IdempotencyKey != d.Effects[1].IdempotencyKey {
		t.Fatalf("run = %+v effects %+v", reconciled.Run, d.Effects)
	}
	if performs, _ := ledger.counts(); performs != 2 || ledger.balanceOf("account:household") != 250 {
		t.Fatalf("performs %d, balance %d: the same key was applied twice", performs, ledger.balanceOf("account:household"))
	}

	once := newFakeLedger(lostRequest)
	h2, _ := newEffectHarness(t, once, false, nil)
	w2 := h2.approvePostShare(withoutRetry(t))
	res, err = h2.startPost(h2.rt, w2.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := h2.rt.Reconcile(ctx, Reconciliation{RunID: res.Run.Run.ID, Step: "post", Outcome: "did-not-happen", Evidence: "no transfer", Surface: "cli"})
	if err != nil || failed.Run.State != eviedb.WorkflowRunFailed || !strings.Contains(failed.Run.Error, "did not happen") || !strings.Contains(failed.Run.Error, "Retry Policy") {
		t.Fatalf("run = %+v, %v", failed.Run, err)
	}
	if performs, _ := once.counts(); performs != 1 {
		t.Fatalf("performs %d", performs)
	}
}

// With provider reconciliation, an unknown outcome is reconciled at once
// by lookup: one that happened resolves to a receipt and the run continues
// without calling the provider again; one that did not happen retries
// within the Retry Policy; one the provider cannot prove waits for the
// owner.
func TestWorkflowEffectAutomaticReconciliationByLookup(t *testing.T) {
	ledger := newFakeLedger(timeoutAfterAcceptance)
	h, _ := newEffectHarness(t, ledger, true, ledger.lookupByKey)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Effects) != 1 || d.Effects[0].Unknown == nil ||
		d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile || d.Effects[0].Receipt.Evidence != "ledger entry entry-1 carries the key" ||
		!reflect.DeepEqual(decoded(t, d.Run.Outputs), postedOutputs) {
		t.Fatalf("run = %+v effects %+v", d.Run, d.Effects)
	}
	if performs, lookups := ledger.counts(); performs != 1 || lookups != 1 || ledger.lookups[0].IdempotencyKey != d.Effects[0].IdempotencyKey {
		t.Fatalf("performs %d lookups %d", performs, lookups)
	}
	// A request that timed out may still be landing: the lookup waits one
	// effect timeout after the outcome became unknown.
	if settled := d.Effects[0].Receipt.RecordedAt.Sub(d.Effects[0].Unknown.RecordedAt); settled < 40*time.Millisecond {
		t.Fatalf("looked up %s after the outcome became unknown", settled)
	}
	checkOrdering(t, d)

	lost := newFakeLedger(lostRequest, nil)
	h, _ = newEffectHarness(t, lost, true, lost.lookupByKey)
	w = h.approvePostShare(postShareDir)
	res, err = h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d = h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded ||
		!reflect.DeepEqual(effectStates(d), []eviedb.WorkflowEffectState{eviedb.WorkflowEffectFailed, eviedb.WorkflowEffectSucceeded}) ||
		d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile || lost.balanceOf("account:household") != 250 {
		t.Fatalf("lost request run = %+v effects %+v", d.Run, d.Effects)
	}

	unsure := newFakeLedger(lostRequest)
	h, _ = newEffectHarness(t, unsure, true, func(context.Context, plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
		return plugins.WorkflowEffectLookup{Outcome: plugins.WorkflowEffectUnknown, Evidence: "the bank's log is not available yet"}, nil
	})
	w = h.approvePostShare(postShareDir)
	res, err = h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d = h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation || d.Run.NotBefore != nil || !strings.Contains(d.Run.Error, "the bank's log is not available yet") ||
		!strings.Contains(d.Run.Error, "evie workflows reconcile") {
		t.Fatalf("unproven run = %+v", d.Run)
	}
}

// Provider text of any length is recorded bounded: a long proven failure
// still records its failed receipt and retries, and a long reconciliation
// error still defers the next lookup by the recheck time instead of asking
// at every poll.
func TestWorkflowEffectLongProviderTextIsBounded(t *testing.T) {
	ctx := context.Background()
	long := strings.Repeat("é", 6000)
	ledger := newFakeLedger(refuse(long), nil)
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Effects[0].State() != eviedb.WorkflowEffectFailed || len(d.Effects[0].Receipt.Error) > 1100 {
		t.Fatalf("run = %+v effects %+v", d.Run, d.Effects)
	}

	failing := newFakeLedger(lostRequest)
	var lookups int
	var mu sync.Mutex
	h, _ = newEffectHarness(t, failing, true, func(context.Context, plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
		mu.Lock()
		lookups++
		mu.Unlock()
		return plugins.WorkflowEffectLookup{}, errors.New(long)
	})
	w = h.approvePostShare(postShareDir)
	res, err = h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id := res.Run.Run.ID
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunNeedsReconciliation || d.Run.NotBefore == nil || !d.Run.NotBefore.After(h.clock.Now()) {
		t.Fatalf("after a long reconciliation error = %+v", d.Run)
	}
	p := h.process()
	stop := p.rt.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if lookups != 1 {
		t.Fatalf("the provider was asked %d times before the recheck time", lookups)
	}
}

// A holder that loses its lease after recording the Effect Intent never
// calls the provider: the lease is checked right before the call. The
// next claim finds the intent without a receipt and records its outcome
// unknown; the owner, who knows it was never sent, lets it retry.
func TestWorkflowEffectLeaseLostAfterTheIntentCallsNothing(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	rt, err := New(Config{Store: expiringStore{Store: h.store, after: func() { h.clock.Advance(testLease + time.Second) }}, Repo: h.repo,
		Capabilities: h.providers, Workers: ceiling, Scheduler: h.scheduler})
	if err != nil {
		t.Fatal(err)
	}
	rt.now = h.clock.Now
	res, err := h.startPost(rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation || len(d.Effects) != 1 || d.Effects[0].State() != eviedb.WorkflowEffectUnknown ||
		d.Attempts[1].State != eviedb.WorkflowAttemptInterrupted {
		t.Fatalf("run = %+v attempts %+v effects %+v", d.Run, summarize(d), d.Effects)
	}
	if performs, _ := ledger.counts(); performs != 0 {
		t.Fatalf("a holder without its lease called the provider %d times", performs)
	}
	done, err := h.rt.Reconcile(ctx, Reconciliation{RunID: d.Run.ID, Step: "post", Outcome: OutcomeDidNotHappen, Evidence: "never sent", Surface: "cli"})
	if err != nil || done.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("reconciled = %+v, %v", done.Run, err)
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("performs %d", performs)
	}
}

// expiringStore lets the run's lease expire right after each Effect
// Intent is recorded.
type expiringStore struct {
	*eviedb.Store
	after func()
}

func (s expiringStore) BeginWorkflowEffect(ctx context.Context, lease eviedb.WorkflowRunLease, e eviedb.WorkflowEffectStart) (eviedb.WorkflowEffectIntent, error) {
	i, err := s.Store.BeginWorkflowEffect(ctx, lease, e)
	if err == nil {
		s.after()
	}
	return i, err
}

// Reconciliation by idempotency replay: the provider re-sends the same
// request with the same key, which it applies at most once, so the effect
// happens exactly once whether or not the first request arrived.
func TestWorkflowEffectAutomaticReconciliationByIdempotentReplay(t *testing.T) {
	for _, first := range []effectAnswer{malformed, lostRequest} {
		ledger := newFakeLedger(first)
		h, _ := newEffectHarness(t, ledger, true, ledger.replay)
		w := h.approvePostShare(postShareDir)
		res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
		if err != nil {
			t.Fatal(err)
		}
		d := h.inspect(res.Run.Run.ID)
		if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Effects) != 1 || d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile ||
			!reflect.DeepEqual(decoded(t, d.Run.Outputs), postedOutputs) || ledger.balanceOf("account:household") != 250 {
			t.Fatalf("run = %+v effects %+v balance %d", d.Run, d.Effects, ledger.balanceOf("account:household"))
		}
	}
}

// A process that crashes between the Effect Intent and its receipt leaves
// the outcome unknown: the next holder records the attempt interrupted and
// the intent unknown in the claim's transaction, never calls the provider
// again, and reconciles before anything else; whether or not the request
// reached the provider, it happens exactly once.
func TestWorkflowEffectCrashBetweenIntentAndReceiptNeedsReconciliation(t *testing.T) {
	ctx := context.Background()
	for _, accepted := range []bool{true, false} {
		ledger := newFakeLedger()
		h, fp := newEffectHarness(t, ledger, true, nil)
		w := h.approvePostShare(postShareDir)
		a := h.process()
		dead, forever := make(chan struct{}), make(chan struct{})
		t.Cleanup(func() { close(forever) })
		var once sync.Once
		fp.change(postID, func(p *fakeProvider) *fakeProvider {
			p.perform = func(_ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
				once.Do(func() {
					if accepted {
						ledger.apply(req)
					}
					_ = a.db.Close()
					close(dead)
				})
				<-forever
				return nil, errors.New("unreachable")
			}
			return p
		})
		go func() { _, _ = h.startPost(a.rt, w.ID, "dinner-1", false) }()
		<-dead
		id := h.runs(w.ID)[0].ID
		fp.change(postID, func(p *fakeProvider) *fakeProvider { p.perform = ledger.perform; return p })
		h.clock.Advance(testLease)
		b := h.process()
		stop := b.rt.Start(ctx)
		eventually(t, "the claim to find the unknown effect", func() bool { return h.state(id) == eviedb.WorkflowRunNeedsReconciliation })
		time.Sleep(20 * time.Millisecond)
		if err := stop(); err != nil {
			t.Fatal(err)
		}
		d := h.inspect(id)
		if len(d.Attempts) != 2 || d.Attempts[1].State != eviedb.WorkflowAttemptInterrupted || len(d.Effects) != 1 ||
			d.Effects[0].State() != eviedb.WorkflowEffectUnknown || !strings.Contains(d.Effects[0].Unknown.Reason, a.rt.Holder()) ||
			!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseTakenOver, eviedb.WorkflowEventAttemptInterrupted}) {
			t.Fatalf("after the crash = %+v attempts %+v effects %+v events %+v", d.Run, summarize(d), d.Effects, d.Events)
		}
		if performs, _ := ledger.counts(); performs != 0 {
			t.Fatalf("the new holder called the provider %d times", performs)
		}
		// Recovery of the provider is not evidence: the owner reconciles.
		outcome := "did-not-happen"
		if accepted {
			outcome = "happened"
		}
		r := Reconciliation{RunID: id, Step: "post", Outcome: outcome, Evidence: "checked the ledger", Surface: "cli"}
		if accepted {
			r.Output = json.RawMessage(`{"entry_id":"entry-1","balance_cents":250}`)
		}
		done, err := h.rt.Reconcile(ctx, r)
		if err != nil || done.Run.State != eviedb.WorkflowRunSucceeded || ledger.balanceOf("account:household") != 250 {
			t.Fatalf("accepted=%v: reconciled = %+v, %v (balance %d)", accepted, done.Run, err, ledger.balanceOf("account:household"))
		}
		if performs, _ := ledger.counts(); accepted && performs != 0 || !accepted && performs != 1 {
			t.Fatalf("accepted=%v: performs %d", accepted, performs)
		}
	}
}

// The receipt, the normalized output and the next checkpoint commit
// together, so a crash after them (inside the dependent step) recovers
// from that checkpoint: the dependent step runs again and the effect is
// never sent again.
func TestWorkflowEffectCrashAfterTheReceiptNeverRepeatsTheEffect(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	a := h.process()
	dead := h.crashInside(a, "record")
	go func() { _, _ = h.startPost(a.rt, w.ID, "dinner-1", false) }()
	<-dead
	id := h.runs(w.ID)[0].ID
	h.clock.Advance(testLease)
	b := h.process()
	stop := b.rt.Start(ctx)
	eventually(t, "recovery to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	want := []attemptSummary{
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "post", 0},
		{"post", 1, eviedb.WorkflowAttemptSucceeded, "record", 1},
		{"record", 1, eviedb.WorkflowAttemptInterrupted, "", 2},
		{"record", 2, eviedb.WorkflowAttemptSucceeded, "", 2},
	}
	if got := summarize(d); d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(got, want) || len(d.Effects) != 1 ||
		!reflect.DeepEqual(decoded(t, d.Run.Outputs), postedOutputs) {
		t.Fatalf("run = %+v attempts %+v effects %+v", d.Run, got, d.Effects)
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("the effect was sent %d times", performs)
	}
}

// Stopping the runtime does not cancel an effect in flight: the call
// completes and its receipt is recorded before the run is released, and
// the next process continues from the checkpoint after it.
func TestWorkflowEffectInFlightCompletesWhenTheRuntimeStops(t *testing.T) {
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	ledger := newFakeLedger(func(l *fakeLedger, ctx context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		close(entered)
		<-release
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return l.apply(req), nil
	}, nil)
	h, _ := newEffectHarness(t, ledger, true, nil)
	h.scheduler.EffectTimeout = 10 * time.Second
	h.reopen()
	w := h.approvePostShare(postShareDir)
	run, err := h.startPost(h.rt, w.ID, "dinner-1", true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	p := h.process()
	stop := p.rt.Start(ctx)
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- stop() }()
	time.Sleep(20 * time.Millisecond)
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State.Terminal() || len(d.Effects) != 1 || d.Effects[0].State() != eviedb.WorkflowEffectSucceeded || d.Run.NextStep != "record" ||
		d.Lease == nil || d.Lease.ReleasedAt == nil || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventLeaseReleased}) {
		t.Fatalf("after stopping = %+v effects %+v events %+v", d.Run, d.Effects, d.Events)
	}
	next := h.process()
	stop = next.rt.Start(ctx)
	eventually(t, "the next process to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("continued run = %+v", d.Run)
	}
	if performs, _ := ledger.counts(); performs != 1 {
		t.Fatalf("performs %d", performs)
	}
}

// After a crash, a provider that reconciles resolves the unknown intent
// automatically in the next holder's first pass.
func TestWorkflowEffectCrashIsReconciledAutomaticallyByTheNextHolder(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger()
	h, fp := newEffectHarness(t, ledger, true, ledger.lookupByKey)
	w := h.approvePostShare(postShareDir)
	a := h.process()
	dead, forever := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(forever) })
	var once sync.Once
	fp.change(postID, func(p *fakeProvider) *fakeProvider {
		p.perform = func(_ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
			once.Do(func() {
				ledger.apply(req)
				_ = a.db.Close()
				close(dead)
			})
			<-forever
			return nil, errors.New("unreachable")
		}
		return p
	})
	go func() { _, _ = h.startPost(a.rt, w.ID, "dinner-1", false) }()
	<-dead
	id := h.runs(w.ID)[0].ID
	fp.change(postID, func(p *fakeProvider) *fakeProvider { p.perform = ledger.perform; return p })
	h.clock.Advance(testLease)
	b := h.process()
	stop := b.rt.Start(ctx)
	eventually(t, "automatic reconciliation to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Effects) != 1 || d.Effects[0].Unknown == nil ||
		d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile || !reflect.DeepEqual(decoded(t, d.Run.Outputs), postedOutputs) {
		t.Fatalf("run = %+v effects %+v", d.Run, d.Effects)
	}
	if performs, lookups := ledger.counts(); performs != 0 || lookups != 1 {
		t.Fatalf("performs %d lookups %d", performs, lookups)
	}
	checkOrdering(t, d)
}

// Effect nodes sharing a Resource Conflict Key are serialized across runs
// and runtime instances:
// while one run's posting to the household account is in flight, another
// run in the Workspace computes its own split but waits, visibly, before
// its posting; it continues once the first receipt is recorded. A posting
// to another account proceeds meanwhile.
func TestWorkflowEffectConflictKeysSerializeEffectsAcrossRuns(t *testing.T) {
	ctx := context.Background()
	entered, release := make(chan string, 4), make(chan struct{})
	ledger := newFakeLedger(func(l *fakeLedger, _ context.Context, req plugins.WorkflowEffectRequest) (any, error) {
		entered <- req.Input["bill_id"].(string)
		if req.Resource == "account:household" && req.Input["bill_id"] == "first" {
			<-release
		}
		return l.apply(req), nil
	})
	h, _ := newEffectHarness(t, ledger, true, nil)
	h.scheduler.EffectTimeout = 10 * time.Second
	h.scheduler.Workers = 1
	h.reopen()
	w := h.approvePostShare(postShareDir)
	travel := h.approvePostShare(postShareVariant(t, "account:household", "account:travel"), "account:travel")
	// Two runtime instances over one database, one worker each: the first
	// is busy with the first run's posting while the second takes the
	// others.
	p1 := h.process()
	stop1 := p1.rt.Start(ctx)
	first, err := h.startPost(h.rt, w.ID, "first", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-entered; got != "first" {
		t.Fatalf("entered %s", got)
	}
	second, err := h.startPost(h.rt, w.ID, "second", true)
	if err != nil {
		t.Fatal(err)
	}
	p2 := h.process()
	stop2 := p2.rt.Start(ctx)
	stop := func() error { return errors.Join(stop1(), stop2()) }
	eventually(t, "the second run to wait for the conflict key", func() bool {
		d := h.inspect(second.Run.Run.ID)
		return d.Run.State == eviedb.WorkflowRunWaitingForDependency && strings.Contains(d.Run.Error, householdKey)
	})
	d := h.inspect(second.Run.Run.ID)
	if got := summarize(d); len(got) != 1 || got[0].Step != "split" || got[0].State != eviedb.WorkflowAttemptSucceeded || len(d.Effects) != 0 ||
		!strings.Contains(d.Run.Error, first.Run.Run.ID) {
		t.Fatalf("waiting run = %+v attempts %+v effects %+v", d.Run, got, d.Effects)
	}
	other, err := h.startPost(h.rt, travel.ID, "other", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := <-entered; got != "other" {
		t.Fatalf("entered %s", got)
	}
	eventually(t, "the travel posting to finish while the household one is in flight", func() bool { return h.state(other.Run.Run.ID).Terminal() })
	close(release)
	eventually(t, "the first run to finish", func() bool { return h.state(first.Run.Run.ID).Terminal() })
	h.clock.Advance(time.Second)
	eventually(t, "the second run to post after the first", func() bool { return h.state(second.Run.Run.ID).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if got := <-entered; got != "second" {
		t.Fatalf("entered %s", got)
	}
	for _, id := range []string{first.Run.Run.ID, second.Run.Run.ID, other.Run.Run.ID} {
		if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded {
			t.Fatalf("run %s = %+v", id, d.Run)
		}
	}
	firstReceipt := h.inspect(first.Run.Run.ID).Effects[0].Receipt.RecordedAt
	secondIntent := h.inspect(second.Run.Run.ID).Effects[0].CreatedAt
	if secondIntent.Before(firstReceipt) || ledger.balanceOf("account:household") != 500 || ledger.balanceOf("account:travel") != 250 {
		t.Fatalf("second intent %v before first receipt %v; balances %v", secondIntent, firstReceipt, ledger.balance)
	}
	if k := kinds(h.inspect(second.Run.Run.ID).Events); !reflect.DeepEqual(k, []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused, eviedb.WorkflowEventResumed}) {
		t.Fatalf("second run events = %v", k)
	}
}

// Per-node admission runs before every Effect Intent: access narrowed
// during a run pauses it before the effect, with no attempt, no intent and
// no provider call.
func TestWorkflowEffectOutsideCurrentAuthorityIsRefusedBeforeTheIntent(t *testing.T) {
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, nil)
	w := h.approvePostShare(postShareDir)
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "split" {
			access := workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: "conn-ledger", Resources: []string{"account:travel"}}}}
			if _, err := h.store.SetWorkspaceAccess(context.Background(), w.ID, 1, access); err != nil {
				return nil, err
			}
		}
		return v.CallFunction(step, input)
	}
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, "revoked part of its authority") || len(d.Attempts) != 1 || len(d.Effects) != 0 {
		t.Fatalf("run = %+v attempts %+v effects %+v", d.Run, summarize(d), d.Effects)
	}
	if performs, _ := ledger.counts(); performs != 0 {
		t.Fatalf("the provider was called %d times", performs)
	}
}

// An unavailable provider pauses the run before the Effect Intent, and the
// run resumes by itself once it is back (nothing is uncertain). During
// reconciliation, though, an unavailable provider never pauses the run as a
// dependency: it keeps needing reconciliation, retried later, and resolves
// once the provider can reconcile.
func TestWorkflowEffectUnavailableProvider(t *testing.T) {
	ctx := context.Background()
	ledger := newFakeLedger(nil)
	h, fp := newEffectHarness(t, ledger, true, ledger.lookupByKey)
	w := h.approvePostShare(postShareDir)
	run, err := h.startPost(h.rt, w.ID, "dinner-1", true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	fp.change(postID, func(p *fakeProvider) *fakeProvider { p.down = "the bank is offline"; return p })
	p := h.process()
	stop := p.rt.Start(ctx)
	eventually(t, "the run to pause before the effect", func() bool { return h.state(id) == eviedb.WorkflowRunWaitingForDependency })
	if d := h.inspect(id); len(d.Effects) != 0 || !strings.Contains(d.Run.Error, "the bank is offline") {
		t.Fatalf("paused run = %+v effects %+v", d.Run, d.Effects)
	}
	fp.change(postID, func(p *fakeProvider) *fakeProvider { p.down = ""; return p })
	h.clock.Advance(time.Minute)
	eventually(t, "the run to resume and finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("resumed run = %+v", d.Run)
	}

	// Unknown, then the provider goes away before it can reconcile.
	unknown := newFakeLedger(timeoutAfterAcceptance)
	h, fp = newEffectHarness(t, unknown, true, func(ctx context.Context, req plugins.WorkflowEffectRequest) (plugins.WorkflowEffectLookup, error) {
		return unknown.lookupByKey(ctx, req)
	})
	w = h.approvePostShare(postShareDir)
	fp.change(postID, func(p *fakeProvider) *fakeProvider {
		perform := p.perform
		p.perform = func(ctx context.Context, req plugins.WorkflowEffectRequest) (any, error) {
			out, err := perform(ctx, req)
			fp.mu.Lock()
			fp.live[postID].down = "the bank is offline"
			fp.mu.Unlock()
			return out, err
		}
		return p
	})
	res, err := h.startPost(h.rt, w.ID, "dinner-1", false)
	if err != nil {
		t.Fatal(err)
	}
	id = res.Run.Run.ID
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunNeedsReconciliation || d.Run.NotBefore == nil || !strings.Contains(d.Run.Error, "the bank is offline") {
		t.Fatalf("unknown with the provider away = %+v", d.Run)
	}
	p = h.process()
	stop = p.rt.Start(ctx)
	h.clock.Advance(time.Minute)
	eventually(t, "a recheck while the provider is away", func() bool {
		d := h.inspect(id)
		return d.Run.NotBefore != nil && d.Run.NotBefore.After(h.clock.Now()) && d.Lease != nil && d.Lease.Token == 2 && d.Lease.ReleasedAt != nil
	})
	if d := h.inspect(id); d.Run.State != eviedb.WorkflowRunNeedsReconciliation || len(d.Events) != 0 || !strings.Contains(d.Run.Error, "the bank is offline") {
		t.Fatalf("a recheck changed the run while the provider is away: %+v events %+v", d.Run, d.Events)
	}
	fp.change(postID, func(p *fakeProvider) *fakeProvider { p.down = ""; return p })
	h.clock.Advance(time.Minute)
	eventually(t, "automatic reconciliation once the provider is back", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Effects[0].Receipt.Source != eviedb.WorkflowEffectFromReconcile || len(d.Events) != 0 {
		t.Fatalf("reconciled run = %+v effects %+v events %+v", d.Run, d.Effects, d.Events)
	}
	if performs, lookups := unknown.counts(); performs != 1 || lookups != 1 {
		t.Fatalf("performs %d lookups %d", performs, lookups)
	}
}

// Proposal and approval simulate the effect as a preview only: no
// provider is called, no run, intent or receipt is recorded, and the review
// shows the effect's idempotency, reconciliation and Resource Conflict Key
// rule.
func TestWorkflowEffectSimulationOnlyPreviews(t *testing.T) {
	ledger := newFakeLedger()
	h, _ := newEffectHarness(t, ledger, true, ledger.lookupByKey)
	w := h.ledgerWorkspace()
	card := h.propose(w.ID, postShareDir, postShareFixtures(t))
	sim := card.Proposal.Simulation
	if sim == nil || sim.Status != workflow.SimulationSucceeded || len(sim.Previews) != 1 || sim.Previews[0].Step != "post" || sim.Previews[0].Capability != postID ||
		!reflect.DeepEqual(sim.Previews[0].Input, map[string]any{"bill_id": "dinner-1", "amount_cents": int64(250)}) {
		t.Fatalf("simulation = %+v", sim)
	}
	h.approve(card)
	if performs, lookups := ledger.counts(); performs != 0 || lookups != 0 || len(h.runs(w.ID)) != 0 {
		t.Fatalf("simulation reached the provider: %d performs, %d lookups", performs, lookups)
	}
	report := card.Proposal.Report.Text()
	if !strings.Contains(report, "ledger.post 1.0.0: effect, idempotent, reconciled by the provider, conflict keys per resource") {
		t.Fatalf("report:\n%s", report)
	}
}
