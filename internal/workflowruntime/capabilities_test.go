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
	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/workflow"
)

// Capability reads (Stage 14 sub-stage 5). Runs execute capability read
// steps as fenced attempts through providers resolved from the run's
// pinned Execution Composition. Fake providers behind the same focused
// seam model success, deterministic failure, timeouts, malformed output,
// an unavailable dependency and a provider that changes between runs; one
// test runs the real finance plugin over a real finance database.

var sharedSpendingDir = filepath.Join(testdata, "shared-spending")

const cashFlowID = string(plugins.FinanceMonthCashFlowCapabilityID)

// fakeProviders is a capability catalog whose live providers a test
// controls. A capability with a live provider is listed with that
// provider's contract, as a compiled plugin's is; the others are catalog
// entries no run can call.
type fakeProviders struct {
	mu        sync.Mutex
	contracts map[string]workflow.CapabilityContract
	live      map[string]*fakeProvider
}

type fakeProvider struct {
	plugin    plugins.PluginID
	version   string
	contract  workflow.CapabilityContract
	resumable map[string]plugins.CapabilityCompatibility
	// down, when set, makes the provider unavailable for that reason.
	down string
	read plugins.WorkflowRead
	// perform, reconcile and conflictKeys are an effect's behavior
	// (sub-stage 6).
	perform      plugins.WorkflowEffect
	reconcile    plugins.WorkflowEffectReconcile
	conflictKeys plugins.WorkflowConflictKeys
}

func (f *fakeProviders) WorkflowContract(id string) (workflow.CapabilityContract, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p, ok := f.live[id]; ok {
		return p.contract, true
	}
	c, ok := f.contracts[id]
	return c, ok
}

func (f *fakeProviders) ResolveWorkflowCapability(_ context.Context, id string) (plugins.WorkflowProvider, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.live[id]
	if !ok {
		return plugins.WorkflowProvider{}, fmt.Errorf("%w: %s", plugins.ErrWorkflowCapabilityUnknown, id)
	}
	out := plugins.WorkflowProvider{Contract: p.contract, SchemaSHA256: p.contract.SchemaSHA256(), Plugin: p.plugin, Version: p.version,
		ResumableFrom: p.resumable}
	if p.down != "" {
		return out, fmt.Errorf("%w: %s: %s", plugins.ErrWorkflowCapabilityUnavailable, id, p.down)
	}
	out.Read = p.read
	out.Perform, out.Reconcile, out.ConflictKeys = p.perform, p.reconcile, p.conflictKeys
	return out, nil
}

// change edits the live provider of id; a nil provider removes it.
func (f *fakeProviders) change(id string, edit func(p *fakeProvider) *fakeProvider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if p := edit(f.live[id]); p != nil {
		f.live[id] = p
	} else {
		delete(f.live, id)
	}
}

// financeCashFlowContract is the production finance.month_cash_flow
// contract.
func financeCashFlowContract(t *testing.T) workflow.CapabilityContract {
	t.Helper()
	for _, c := range plugins.NewFinance().WorkflowCapabilities() {
		if c.ID == plugins.FinanceMonthCashFlowCapabilityID {
			return workflow.CapabilityContract{ID: string(c.ID), Contract: c.ContractVersion, Input: c.Input, Output: c.Output,
				Connections: c.Connections, Resources: c.Resources}
		}
	}
	t.Fatal("finance does not provide finance.month_cash_flow")
	return workflow.CapabilityContract{}
}

func monthFlow(month string, outflow int64) map[string]any {
	return map[string]any{"month": month, "inflow_cents": int64(0), "outflow_cents": outflow, "net_cents": -outflow,
		"transactions": int64(2), "pending_transactions": int64(0), "categories": []any{}}
}

// readScript answers read calls in turn; the last answer repeats.
type readScript struct {
	mu      sync.Mutex
	calls   []plugins.WorkflowReadRequest
	ctxs    []context.Context
	answers []func(ctx context.Context, req plugins.WorkflowReadRequest) (any, error)
}

func (s *readScript) read(ctx context.Context, req plugins.WorkflowReadRequest) (any, error) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	s.ctxs = append(s.ctxs, ctx)
	answer := s.answers[min(len(s.calls), len(s.answers))-1]
	s.mu.Unlock()
	return answer(ctx, req)
}

func (s *readScript) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *readScript) ctx(i int) context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ctxs[i]
}

// flow answers with the month's cash flow for an outflow.
func flow(outflow int64) func(context.Context, plugins.WorkflowReadRequest) (any, error) {
	return func(_ context.Context, req plugins.WorkflowReadRequest) (any, error) {
		return monthFlow(req.Input["month"].(string), outflow), nil
	}
}

// blockUntilCancelled answers once the read's context ends.
func blockUntilCancelled(entered chan<- struct{}) func(context.Context, plugins.WorkflowReadRequest) (any, error) {
	var once sync.Once
	return func(ctx context.Context, _ plugins.WorkflowReadRequest) (any, error) {
		once.Do(func() { close(entered) })
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

// newCapabilityHarness is a lease harness whose runtimes reach the finance
// cash-flow read through a fake provider (finance 1.1.0) that answers
// with the script, and the notes contracts as catalog entries.
func newCapabilityHarness(t *testing.T, answers ...func(context.Context, plugins.WorkflowReadRequest) (any, error)) (*runHarness, *fakeProviders, *readScript) {
	t.Helper()
	script := &readScript{answers: answers}
	contracts := map[string]workflow.CapabilityContract{}
	for _, id := range []string{"notes.list", "notes.append"} {
		c, _ := notesCatalog().Capability(id)
		contracts[id] = c
	}
	fp := &fakeProviders{contracts: contracts, live: map[string]*fakeProvider{
		cashFlowID: {plugin: "finance", version: "1.1.0", contract: financeCashFlowContract(t), read: script.read},
	}}
	h := newLeaseHarness(t)
	h.providers = fp
	h.reopen()
	return h, fp, script
}

func sharedSpendingFixtures(t *testing.T) *workflow.Fixtures {
	t.Helper()
	return fixtures(t, readFile(t, filepath.Join(testdata, "shared-spending.fixtures.json")))
}

// financeWorkspace is a Workspace whose access allows the finance ledger.
func (h *harness) financeWorkspace(resources ...string) memory.Workspace {
	h.t.Helper()
	w, err := h.store.RegisterWorkspace(context.Background(), "Household")
	if err != nil {
		h.t.Fatal(err)
	}
	if len(resources) == 0 {
		resources = []string{plugins.FinanceLedgerResource}
	}
	h.setFinanceAccess(w.ID, 0, resources...)
	return w
}

func (h *harness) setFinanceAccess(ws memory.WorkspaceID, expected int64, resources ...string) {
	h.t.Helper()
	access := workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: plugins.FinanceConnectionID, Resources: resources}}}
	if _, err := h.store.SetWorkspaceAccess(context.Background(), ws, expected, access); err != nil {
		h.t.Fatal(err)
	}
}

func (h *runHarness) approveSharedSpending(dir string) (memory.Workspace, eviedb.WorkflowActivation) {
	h.t.Helper()
	w := h.financeWorkspace()
	return w, h.approve(h.propose(w.ID, dir, sharedSpendingFixtures(h.t)))
}

func spendingInputs(month string, people int64) map[string]json.RawMessage {
	return map[string]json.RawMessage{"month": json.RawMessage(fmt.Sprintf("%q", month)), "people": json.RawMessage(fmt.Sprint(people))}
}

func (h *runHarness) startSpending(rt *Runtime, ws memory.WorkspaceID, month string, people int64, detach bool) (RunResult, error) {
	return rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "shared-spending", Inputs: spendingInputs(month, people), Surface: "cli", Detach: detach})
}

// advanceOnSleep makes rt's foreground waits for a retry advance the test
// clock instead of sleeping.
func (h *runHarness) advanceOnSleep(rt *Runtime) {
	rt.sleep = func(_ context.Context, d time.Duration) error {
		h.clock.Advance(d)
		return nil
	}
}

type pinnedTestProvider struct {
	Capability   string   `json:"capability"`
	Contract     string   `json:"contract"`
	SchemaSHA256 string   `json:"schema_sha256"`
	Plugin       string   `json:"plugin"`
	Version      string   `json:"version"`
	Connections  []string `json:"connections"`
}

func pinnedProviders(t *testing.T, run eviedb.WorkflowRun) []pinnedTestProvider {
	t.Helper()
	var c struct {
		Runtime   string               `json:"runtime"`
		Kinds     []string             `json:"step_kinds"`
		Providers []pinnedTestProvider `json:"providers"`
	}
	if err := json.Unmarshal(run.Composition, &c); err != nil {
		t.Fatal(err)
	}
	if c.Runtime != RuntimeVersion || !reflect.DeepEqual(c.Kinds, []string{"branch", "capability", "deterministic"}) {
		t.Fatalf("composition = %s", run.Composition)
	}
	return c.Providers
}

// seedFinance creates an isolated finance database for September 2026.
func seedFinance(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	db, err := finance.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`INSERT INTO items (item_id, access_token, linked_at) VALUES ('item-1', 'access-token-SECRET', 'now')`,
		`INSERT INTO transactions (transaction_id, item_id, date, name, merchant_name, amount_cents, pending) VALUES
			('t1', 'item-1', '2026-09-03', 'MARKET', 'Market', 4520, 0),
			('t2', 'item-1', '2026-09-10', 'CAFE', 'Cafe', 1250, 0),
			('t3', 'item-1', '2026-09-15', 'PAYROLL', '', -300000, 0)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
}

// A definition with deterministic, capability read and branch steps runs
// end to end through the real finance plugin and Plugin Manager: the run
// pins the provider (Capability ID, contract and schema hash, plugin
// version, connection), reads the month from the finance database as one
// recorded attempt, and splits it by code.
func TestWorkflowCapabilityReadRunsEndToEndThroughTheFinancePlugin(t *testing.T) {
	seedFinance(t)
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled(plugins.FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	h := newRunHarness(t)
	h.providers = manager
	h.reopen()
	w, a := h.approveSharedSpending(sharedSpendingDir)
	res, err := h.startSpending(h.rt, w.ID, "2026-09", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	h.reopen()
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Run.Version != a.Version {
		t.Fatalf("run = %+v", d.Run)
	}
	wantOutputs := map[string]any{"spent_cents": int64(5770), "shares": []any{int64(1924), int64(1923), int64(1923)}, "leftover_cents": int64(1)}
	if !reflect.DeepEqual(decoded(t, d.Run.Outputs), wantOutputs) {
		t.Fatalf("outputs = %s", d.Run.Outputs)
	}
	want := []attemptSummary{
		{"read", 1, eviedb.WorkflowAttemptSucceeded, "split", 0},
		{"split", 1, eviedb.WorkflowAttemptSucceeded, "check", 1},
		{"check", 1, eviedb.WorkflowAttemptSucceeded, "leftover", 2},
		{"leftover", 1, eviedb.WorkflowAttemptSucceeded, "", 3},
	}
	if got := summarize(d); !reflect.DeepEqual(got, want) {
		t.Fatalf("attempts = %+v", got)
	}
	checkOrdering(t, d)
	read := d.Attempts[0]
	if read.Kind != workflow.KindCapability || string(read.Input) != `{"month":"2026-09"}` ||
		!strings.Contains(string(read.Output), `"outflow_cents":5770`) || strings.Contains(string(read.Output), "SECRET") {
		t.Fatalf("read attempt = %+v", read)
	}
	contract, _ := manager.WorkflowContract(cashFlowID)
	pins := pinnedProviders(t, d.Run)
	wantPins := []pinnedTestProvider{{Capability: cashFlowID, Contract: "1.0.0", SchemaSHA256: contract.SchemaSHA256(), Plugin: "finance", Version: plugins.NewFinance().Manifest().ImplementationVersion,
		Connections: []string{plugins.FinanceConnectionID}}}
	if !reflect.DeepEqual(pins, wantPins) {
		t.Fatalf("pinned providers = %+v", pins)
	}
	text := RunText(d)
	for _, want := range []string{"Providers: finance.month_cash_flow 1.0.0 from finance 1.3.0 on finance-local", "read attempt 1: succeeded -> split", "leftover_cents: 1"} {
		if !strings.Contains(text, want) {
			t.Fatalf("inspection lacks %q:\n%s", want, text)
		}
	}
	even, err := h.startSpending(h.rt, w.ID, "2026-09", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded(t, even.Run.Run.Outputs); !reflect.DeepEqual(got, map[string]any{"spent_cents": int64(5770), "shares": []any{int64(2885), int64(2885)}, "per_person": int64(2885)}) {
		t.Fatalf("even outputs = %v", got)
	}
}

// Before a provider is called, its attempt is durable with the resolved
// input and the lease it runs under; the provider gets the step's
// connection and resource and a context bounded by the read timeout.
func TestWorkflowCapabilityReadIsRecordedBeforeTheProviderIsCalled(t *testing.T) {
	ctx := context.Background()
	var h *runHarness
	var observed []string
	h, _, _ = newCapabilityHarness(t, func(ctx context.Context, req plugins.WorkflowReadRequest) (any, error) {
		runs, err := h.store.ListWorkflowRuns(context.Background(), eviedb.WorkflowRunFilter{})
		if err != nil || len(runs) != 1 {
			return nil, fmt.Errorf("runs = %+v, %v", runs, err)
		}
		p := h.progress(runs[0].ID)
		if p.InFlight == nil || p.InFlight.Step != "read" || p.InFlight.Kind != workflow.KindCapability || string(p.InFlight.Input) != `{"month":"2026-09"}` ||
			p.InFlight.LeaseToken == 0 || p.Checkpoint.Seq != 0 {
			return nil, fmt.Errorf("called before the attempt was durable: %+v", p.InFlight)
		}
		deadline, bounded := ctx.Deadline()
		observed = append(observed, fmt.Sprintf("%s %s bounded=%v", req.Connection, req.Resource, bounded && time.Until(deadline) <= defaultReadTimeout))
		return monthFlow("2026-09", 900), nil
	})
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	res, err := h.startSpending(h.rt, w.ID, "2026-09", 3, false)
	if err != nil || res.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run = %+v, %v", res.Run.Run, err)
	}
	if want := []string{"finance-local ledger bounded=true"}; !reflect.DeepEqual(observed, want) {
		t.Fatalf("observed %v", observed)
	}
	_ = ctx
}

// A read that fails, times out, panics or returns output outside its
// contract fails its attempt; the step retries within its Retry Policy
// after its backoff, and fails the run after its last attempt.
func TestWorkflowCapabilityReadFailuresRetryWithinThePolicy(t *testing.T) {
	ctx := context.Background()
	answers := map[string][]func(context.Context, plugins.WorkflowReadRequest) (any, error){
		"2026-01": {func(context.Context, plugins.WorkflowReadRequest) (any, error) {
			return nil, errors.New("ledger locked")
		}},
		"2026-02": {func(context.Context, plugins.WorkflowReadRequest) (any, error) {
			malformed := monthFlow("2026-02", 900)
			malformed["outflow_cents"] = "lots"
			return malformed, nil
		}, flow(900)},
		"2026-03": {func(ctx context.Context, _ plugins.WorkflowReadRequest) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, flow(900)},
		"2026-04": {func(context.Context, plugins.WorkflowReadRequest) (any, error) { panic("provider bug") }, flow(900)},
	}
	calls := map[string]int{}
	var mu sync.Mutex
	h, _, script := newCapabilityHarness(t, func(ctx context.Context, req plugins.WorkflowReadRequest) (any, error) {
		month := req.Input["month"].(string)
		mu.Lock()
		calls[month]++
		list := answers[month]
		answer := list[min(calls[month], len(list))-1]
		mu.Unlock()
		return answer(ctx, req)
	})
	h.scheduler.ReadTimeout = 40 * time.Millisecond
	h.reopen()
	h.advanceOnSleep(h.rt)
	w, _ := h.approveSharedSpending(sharedSpendingDir)

	failed, err := h.startSpending(h.rt, w.ID, "2026-01", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(failed.Run.Run.ID)
	if got := summarize(d); d.Run.State != eviedb.WorkflowRunFailed || d.Run.Error != "steps.read failed after 3 attempts: ledger locked" || len(got) != 3 ||
		got[2].State != eviedb.WorkflowAttemptFailed || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventRetryScheduled, eviedb.WorkflowEventRetryScheduled}) {
		t.Fatalf("deterministic failure = %+v attempts %+v events %+v", d.Run, got, d.Events)
	}
	if d.Attempts[1].StartedAt.Sub(d.Attempts[0].StartedAt) < time.Minute {
		t.Fatalf("retried before the step's backoff: %+v", d.Attempts)
	}
	for _, tc := range []struct{ month, wantError string }{
		{"2026-02", "malformed response: outflow_cents: expected integer, got string"},
		{"2026-03", "the read did not finish within 40ms"},
		{"2026-04", "the provider panicked: provider bug"},
	} {
		month, wantError := tc.month, tc.wantError
		res, err := h.startSpending(h.rt, w.ID, month, 3, false)
		if err != nil {
			t.Fatal(err)
		}
		d := h.inspect(res.Run.Run.ID)
		if d.Run.State != eviedb.WorkflowRunSucceeded || d.Attempts[0].State != eviedb.WorkflowAttemptFailed || d.Attempts[0].Error != wantError ||
			d.Attempts[0].Output != nil || d.Attempts[1].Step != "read" || d.Attempts[1].Attempt != 2 || d.Attempts[1].State != eviedb.WorkflowAttemptSucceeded {
			t.Fatalf("%s: run = %+v attempts %+v", month, d.Run, d.Attempts)
		}
		checkOrdering(t, d)
	}
	if err := script.ctx(5).Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("the timed-out read's context: %v", err)
	}
	_ = ctx
}

// A failed read's backoff is a durable not-before time: a restarted
// process does not retry it early, and retries it once the time comes.
func TestWorkflowCapabilityReadBackoffIsDurableAcrossRestart(t *testing.T) {
	ctx := context.Background()
	h, _, script := newCapabilityHarness(t, func(context.Context, plugins.WorkflowReadRequest) (any, error) { return nil, errors.New("ledger busy") }, flow(600))
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	failedAt := h.clock.Now()
	run, err := h.startSpending(h.rt, w.ID, "2026-09", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	a := h.process()
	stop := a.rt.Start(ctx)
	eventually(t, "the first read to fail", func() bool { p := h.progress(id); return p.Attempts == 1 && p.Run.NotBefore != nil })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if p := h.progress(id); !p.Run.NotBefore.Equal(failedAt.Add(time.Minute)) || p.InFlight != nil {
		t.Fatalf("after the failed read = %+v", p.Run)
	}
	h.reopen()
	b := h.process()
	stop = b.rt.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	if script.count() != 1 || h.progress(id).Attempts != 1 {
		t.Fatalf("retried before its not-before time: %d calls", script.count())
	}
	h.clock.Advance(time.Minute)
	eventually(t, "the retry", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Attempts[1].Step != "read" || d.Attempts[1].LeaseHolder != b.rt.Holder() || script.count() != 2 ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventRetryScheduled}) {
		t.Fatalf("retried run = %+v attempts %+v events %+v", d.Run, summarize(d), d.Events)
	}
}

// Losing the run's lease cancels the read in flight through its context;
// the stale holder records nothing, and the new holder records the attempt
// interrupted and reads again from the same checkpoint.
func TestWorkflowCapabilityLeaseLossCancelsTheReadInFlight(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{})
	h, _, script := newCapabilityHarness(t, blockUntilCancelled(entered), flow(900))
	h.scheduler.Lease = 300 * time.Millisecond
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	run, err := h.startSpending(h.rt, w.ID, "2026-09", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	a := h.process()
	stopA := a.rt.Start(ctx)
	<-entered
	h.clock.Advance(time.Second)
	eventually(t, "the read in flight to be cancelled", func() bool { return script.ctx(0).Err() != nil })
	if err := script.ctx(0).Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("the read's context ended with %v, want cancellation by the lost lease", err)
	}
	b := h.process()
	stopB := b.rt.Start(ctx)
	eventually(t, "the new holder to finish the run", func() bool { return h.state(id).Terminal() })
	if err := errors.Join(stopB(), stopA()); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	// Whichever process claims the run next (B, or A again under a new
	// lease) reads again; A's lost lease wrote nothing.
	if got := summarize(d); d.Run.State != eviedb.WorkflowRunSucceeded || got[0] != (attemptSummary{"read", 1, eviedb.WorkflowAttemptInterrupted, "", 0}) ||
		got[1] != (attemptSummary{"read", 2, eviedb.WorkflowAttemptSucceeded, "split", 0}) ||
		d.Attempts[0].LeaseHolder != a.rt.Holder() || d.Attempts[1].LeaseToken <= d.Attempts[0].LeaseToken || d.Attempts[0].Output != nil {
		t.Fatalf("run = %+v attempts %+v", d.Run, d.Attempts)
	}
	if !strings.Contains(d.Attempts[0].Error, a.rt.Holder()) || script.count() != 2 {
		t.Fatalf("interrupted attempt = %+v, %d reads", d.Attempts[0], script.count())
	}
}

// Stopping the runtime stops a read in flight: its holder records the
// attempt interrupted and releases the run, and the next process reads
// again from the same checkpoint at once.
func TestWorkflowCapabilityShutdownStopsTheReadInFlight(t *testing.T) {
	ctx := context.Background()
	entered := make(chan struct{})
	h, _, script := newCapabilityHarness(t, blockUntilCancelled(entered), flow(900))
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	run, err := h.startSpending(h.rt, w.ID, "2026-09", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	p := h.process()
	stop := p.rt.Start(ctx)
	<-entered
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d := h.inspect(id)
	if len(d.Attempts) != 1 || d.Attempts[0].State != eviedb.WorkflowAttemptInterrupted || !strings.Contains(d.Attempts[0].Error, "the runtime stopped during the read") ||
		d.Lease == nil || d.Lease.ReleasedAt == nil ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventAttemptInterrupted, eviedb.WorkflowEventLeaseReleased}) {
		t.Fatalf("after shutdown = %+v attempts %+v events %+v", d.Run, d.Attempts, d.Events)
	}
	next := h.process()
	stop = next.rt.Start(ctx)
	eventually(t, "the next process to finish the run", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || d.Attempts[1].Attempt != 2 || d.Attempts[1].State != eviedb.WorkflowAttemptSucceeded || script.count() != 2 {
		t.Fatalf("continued run = %+v attempts %+v", d.Run, summarize(d))
	}
}

// Workspace Access applies to every read when it runs, not only when the
// run starts or resumes: access narrowed during a run pauses it before the
// read, and the provider is never called.
func TestWorkflowCapabilityReadOutsideCurrentAccessIsRefusedAtRuntime(t *testing.T) {
	h, _, script := newCapabilityHarness(t, flow(900))
	dir := copyDefinition(t, "shared-spending")
	yaml := filepath.Join(dir, "workflow.yaml")
	text := strings.Replace(readFile(t, yaml), "start: read\n", "start: prepare\n", 1)
	text = strings.Replace(text, "steps:\n", "steps:\n  - id: prepare\n    kind: deterministic\n    function: split_evenly@1\n    input:\n      total: $inputs.people\n      parts: 1\n    next: read\n\n", 1)
	writeFile(t, yaml, text)
	w, _ := h.approveSharedSpending(dir)
	h.rt.call = func(_ string, v *workflow.Validated, step string, input map[string]any) (any, error) {
		if step == "prepare" {
			h.setFinanceAccess(w.ID, 1, "archive")
		}
		return v.CallFunction(step, input)
	}
	res, err := h.startSpending(h.rt, w.ID, "2026-09", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	d := h.inspect(res.Run.Run.ID)
	if d.Run.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(d.Run.Error, "revoked part of its authority") || len(d.Attempts) != 1 ||
		d.Attempts[0].Step != "prepare" || script.count() != 0 {
		t.Fatalf("run = %+v attempts %+v reads %d", d.Run, summarize(d), script.count())
	}
}

// A run pins its providers at start. When the pinned provider is replaced,
// the run pauses visibly: a replacement that declares it continues the
// pinned implementation with the same contract and schema needs the
// owner's audited substitution, and then the run continues; one that does
// not declare it, or that changed the contract's schema, cannot be
// substituted; a missing provider pauses the run too. A new run pins the
// provider it starts with.
func TestWorkflowCapabilityProviderChangesPauseUntilAnAuditedSubstitution(t *testing.T) {
	ctx := context.Background()
	h, fp, script := newCapabilityHarness(t, flow(900))
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	start := func(month string) string {
		t.Helper()
		res, err := h.startSpending(h.rt, w.ID, month, 3, true)
		if err != nil {
			t.Fatal(err)
		}
		return res.Run.Run.ID
	}
	pausedWith := func(id, reason string) {
		t.Helper()
		eventually(t, "run "+id+" to pause", func() bool {
			d := h.inspect(id)
			return d.Run.State == eviedb.WorkflowRunWaitingForDependency && strings.Contains(d.Run.Error, reason)
		})
	}
	original := financeCashFlowContract(t)
	compatible := start("2026-01")
	fp.change(cashFlowID, func(p *fakeProvider) *fakeProvider {
		p.version = "1.2.0"
		p.resumable = map[string]plugins.CapabilityCompatibility{"1.1.0": {ID: plugins.FinanceMonthCashFlowCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: original.SchemaSHA256()}}
		return p
	})
	p := h.process()
	stop := p.rt.Start(ctx)
	pausedWith(compatible, fmt.Sprintf("evie workflows substitute %s %s 1.2.0", compatible, cashFlowID))
	if script.count() != 0 {
		t.Fatalf("a replaced provider was called before the owner approved it: %d reads", script.count())
	}
	if _, err := p.rt.Substitute(ctx, Substitution{RunID: compatible, Capability: cashFlowID, Version: "1.1.9", Surface: "cli"}); err == nil {
		t.Fatal("a substitution for a version this Evie does not have was recorded")
	}
	if _, err := p.rt.Substitute(ctx, Substitution{RunID: compatible, Capability: cashFlowID, Version: "1.2.0", Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("a substitution from chat: %v", err)
	}
	sub, err := p.rt.Substitute(ctx, Substitution{RunID: compatible, Capability: cashFlowID, Version: "1.2.0", Surface: "cli"})
	if err != nil || sub.PinnedVersion != "1.1.0" || sub.ReplacementVersion != "1.2.0" || sub.Contract != "1.0.0" || sub.SchemaSHA256 != original.SchemaSHA256() ||
		!strings.Contains(sub.Evidence, "1.1.0") {
		t.Fatalf("substitution = %+v, %v", sub, err)
	}
	h.clock.Advance(time.Minute)
	eventually(t, "the substituted run to finish", func() bool { return h.state(compatible).Terminal() })
	d := h.inspect(compatible)
	if d.Run.State != eviedb.WorkflowRunSucceeded || len(d.Substitutions) != 1 || pinnedProviders(t, d.Run)[0].Version != "1.1.0" ||
		!reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused, eviedb.WorkflowEventResumed}) ||
		!strings.Contains(RunText(d), "Substituted: finance.month_cash_flow finance 1.1.0 -> 1.2.0") {
		t.Fatalf("substituted run = %+v substitutions %+v events %+v", d.Run, d.Substitutions, d.Events)
	}

	if err := stop(); err != nil {
		t.Fatal(err)
	}

	// A new run pins the provider it starts with; replacing it with one
	// that declares nothing pauses it.
	undeclared := start("2026-02")
	if pins := pinnedProviders(t, h.inspect(undeclared).Run); pins[0].Version != "1.2.0" {
		t.Fatalf("a new run pinned %+v", pins)
	}
	fp.change(cashFlowID, func(p *fakeProvider) *fakeProvider { p.version, p.resumable = "1.3.0", nil; return p })
	p = h.process()
	stop = p.rt.Start(ctx)
	pausedWith(undeclared, "does not declare")
	if _, err := p.rt.Substitute(ctx, Substitution{RunID: undeclared, Capability: cashFlowID, Version: "1.3.0", Surface: "cli"}); !errors.Is(err, ErrSubstitutionRefused) {
		t.Fatalf("an undeclared substitution: %v", err)
	}

	changed := original
	changed.Output = financeCashFlowContract(t).Output
	changed.Output.Properties["currency"] = workflow.StringType()
	fp.change(cashFlowID, func(p *fakeProvider) *fakeProvider {
		p.version, p.contract = "1.4.0", changed
		p.resumable = map[string]plugins.CapabilityCompatibility{"1.2.0": {ID: plugins.FinanceMonthCashFlowCapabilityID, ContractVersion: "1.0.0", SchemaSHA256: changed.SchemaSHA256()}}
		return p
	})
	h.clock.Advance(time.Minute)
	pausedWith(undeclared, "changed incompatibly")
	if _, err := p.rt.Substitute(ctx, Substitution{RunID: undeclared, Capability: cashFlowID, Version: "1.4.0", Surface: "cli"}); !errors.Is(err, ErrSubstitutionRefused) {
		t.Fatalf("a substitution with a changed schema: %v", err)
	}

	fp.change(cashFlowID, func(*fakeProvider) *fakeProvider { return nil })
	h.clock.Advance(time.Minute)
	pausedWith(undeclared, "is missing")
	// Without the capability the approved definition no longer validates.
	if _, err := h.startSpending(h.rt, w.ID, "2026-03", 3, true); !errors.Is(err, ErrDefinitionMismatch) || !strings.Contains(err.Error(), "not in the capability catalog") {
		t.Fatalf("a start without the provider: %v", err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if d := h.inspect(undeclared); len(d.Attempts) != 0 || len(d.Substitutions) != 0 {
		t.Fatalf("the unsubstituted run = %+v attempts %+v", d.Run, d.Attempts)
	}
}

// Through the real Plugin Manager: a disabled provider refuses a new run;
// a run whose provider is disabled before its read pauses waiting for it,
// stays paused through rechecks, and resumes by itself once the plugin is
// enabled again (ADR 0029: nothing is uncertain about a read).
func TestWorkflowCapabilityUnavailableProviderPausesAndResumes(t *testing.T) {
	ctx := context.Background()
	seedFinance(t)
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	h := newLeaseHarness(t)
	h.providers = manager
	h.reopen()
	w, _ := h.approveSharedSpending(sharedSpendingDir)
	if _, err := h.startSpending(h.rt, w.ID, "2026-09", 3, true); !errors.Is(err, ErrCapabilityUnavailable) || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("start with the plugin disabled: %v", err)
	}
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("a refused start recorded %+v", runs)
	}
	if err := manager.SetEnabled(plugins.FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	run, err := h.startSpending(h.rt, w.ID, "2026-09", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	id := run.Run.Run.ID
	if err := manager.SetEnabled(plugins.FinancePluginID, false); err != nil {
		t.Fatal(err)
	}
	p := h.process()
	stop := p.rt.Start(ctx)
	eventually(t, "the run to pause", func() bool { return h.state(id) == eviedb.WorkflowRunWaitingForDependency })
	h.clock.Advance(time.Minute)
	eventually(t, "a recheck", func() bool { return h.progress(id).Run.NotBefore.After(h.clock.Now()) })
	d := h.inspect(id)
	if !strings.Contains(d.Run.Error, "disabled") || len(d.Attempts) != 0 || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused}) {
		t.Fatalf("paused run = %+v events %+v", d.Run, d.Events)
	}
	if err := manager.SetEnabled(plugins.FinancePluginID, true); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Minute)
	eventually(t, "the run to resume and finish", func() bool { return h.state(id).Terminal() })
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	d = h.inspect(id)
	if d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(kinds(d.Events), []eviedb.WorkflowRunEventKind{eviedb.WorkflowEventPaused, eviedb.WorkflowEventResumed}) {
		t.Fatalf("resumed run = %+v events %+v", d.Run, d.Events)
	}
}

// Runs recorded by earlier runtimes keep their shapes: a run pinned to
// evie.workflow.runtime/1 (sub-stages 3 and 4, no providers), /2
// (sub-stage 5), /3 (sub-stage 6) or /4 (sub-stages 7 and 8) resumes under
// this runtime, which declares each compatible.
func TestWorkflowRunsOfEarlierRuntimeShapesStillResume(t *testing.T) {
	ctx := context.Background()
	h := newLeaseHarness(t)
	w, a := h.approveSplitBill(splitBillDir)
	v, err := workflow.Validate(workflow.DirSource{Dir: splitBillDir}, workflow.Environment{Workers: ceiling})
	if err != nil {
		t.Fatal(err)
	}
	for i, runtime := range []string{"evie.workflow.runtime/1", "evie.workflow.runtime/2", "evie.workflow.runtime/3", "evie.workflow.runtime/4"} {
		bill := fmt.Sprintf("dinner-%d", i+1)
		inputs, _ := json.Marshal(map[string]any{"bill_id": bill, "total_cents": 1000, "people": 3})
		accepted, err := v.AcceptInputs(map[string]json.RawMessage{"bill_id": json.RawMessage(fmt.Sprintf("%q", bill)), "total_cents": json.RawMessage(`1000`), "people": json.RawMessage(`3`)}, nil)
		if err != nil {
			t.Fatal(err)
		}
		old, _, err := h.store.StartWorkflowRun(ctx, eviedb.WorkflowRunStart{
			WorkspaceID: w.ID, Workflow: a.Name, Version: a.Version, ExecutableHash: a.ExecutableHash, AuthorityHash: a.AuthorityHash,
			Composition: json.RawMessage(`{"runtime":"` + runtime + `","step_kinds":["branch","deterministic"],"functions":["split_evenly@1"]}`),
			RunKey:      v.RunKey(accepted), Inputs: inputs, Start: "split", Surface: "cli",
		})
		if err != nil {
			t.Fatal(err)
		}
		p := h.process()
		stop := p.rt.Start(ctx)
		eventually(t, "the earlier run to finish", func() bool { return h.state(old.ID).Terminal() })
		if err := stop(); err != nil {
			t.Fatal(err)
		}
		if d := h.inspect(old.ID); d.Run.State != eviedb.WorkflowRunSucceeded || !reflect.DeepEqual(decoded(t, d.Run.Outputs), dinnerOutputs) || len(d.Events) != 0 {
			t.Fatalf("%s run = %+v events %+v", runtime, d.Run, d.Events)
		}
	}
}
