package workflowruntime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
)

// Built-in workflows (owner decision 2026-10-04). A built-in runs in every
// Workspace without a proposal or approval: the runtime records its
// version (its files committed to procedural Git, its executable hash its
// identity) before the first run pins it, and every run reads its pinned
// version from that commit. The owner disables and enables it, and a
// Workspace's or the Global approved custom version of the same name
// overrides it in its scope.

// dirBuiltins is a built-in catalog of definition folders, each with its
// dry-run fixtures.
func dirBuiltins(t *testing.T, dirs map[string]string, fixtures map[string]string) *builtinCatalog {
	t.Helper()
	return &builtinCatalog{
		list: func() []workflow.TemplateInfo {
			var out []workflow.TemplateInfo
			for name := range dirs {
				out = append(out, workflow.TemplateInfo{Name: name, Description: "built-in " + name, Inputs: []string{}})
			}
			slices.SortFunc(out, func(a, b workflow.TemplateInfo) int { return strings.Compare(a.Name, b.Name) })
			return out
		},
		source: func(name string) (workflow.Source, bool) {
			dir, ok := dirs[name]
			return workflow.DirSource{Dir: dir}, ok
		},
		fixtures: func(name string) (workflow.Fixtures, error) {
			return workflow.ParseFixtures([]byte(fixtures[name]))
		},
	}
}

const splitFixtureJSON = `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`

// splitBuiltinHarness is a harness whose only built-in is the split-bill
// example (deterministic, no workers), in dir.
func splitBuiltinHarness(t *testing.T, dir string) *runHarness {
	t.Helper()
	h := newLeaseHarness(t)
	h.builtins = dirBuiltins(t, map[string]string{"split-bill": dir}, map[string]string{"split-bill": splitFixtureJSON})
	h.rt = h.runtime()
	return h
}

func splitInputs(bill string) map[string]string {
	return map[string]string{"bill_id": bill, "total_cents": "1000", "people": "3"}
}

func (h *runHarness) startSplit(ws memory.WorkspaceID, bill string, detach bool) RunResult {
	h.t.Helper()
	res, err := h.rt.StartRun(context.Background(), RunRequest{WorkspaceID: ws, Workflow: "split-bill", TextInputs: splitInputs(bill), Surface: "cli", Detach: detach})
	if err != nil {
		h.t.Fatal(err)
	}
	return res
}

// drive executes a detached run until it finishes, moving the test clock
// past each wait.
func (h *runHarness) drive(id string) {
	h.t.Helper()
	for range 30 {
		end, err := h.rt.execute(context.Background(), id)
		if err != nil {
			h.t.Fatal(err)
		}
		if end == passDone {
			return
		}
		h.clock.Advance(2 * time.Minute)
	}
	h.t.Fatalf("run %s did not finish:\n%s", id, RunText(h.inspect(id)))
}

// replaceIn replaces exactly one occurrence of old in dir/name.
func replaceIn(t *testing.T, dir, name, old, replacement string) {
	t.Helper()
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), old) != 1 {
		t.Fatalf("%s: %q is not there exactly once", name, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuiltinRunsWithoutProposalOrApprovalAndPinsItsRecordedVersion(t *testing.T) {
	ctx := context.Background()
	h := splitBuiltinHarness(t, filepath.Join(testdata, "split-bill"))
	w := h.workspace()
	res := h.startSplit(w.ID, "b1", false)
	run := res.Run.Run
	if run.State != eviedb.WorkflowRunSucceeded || run.Source != eviedb.WorkflowSourceBuiltin || run.Version != 1 || run.Path != "builtin/workflows/split-bill" {
		t.Fatalf("run = %+v\n%s", run, RunText(res.Run))
	}
	// The built-in's exact files are in procedural Git at the pinned commit.
	if data, err := h.repo.ReadFile(ctx, run.Commit, "builtin/workflows/split-bill/workflow.yaml"); err != nil || !strings.Contains(string(data), "split_evenly@1") {
		t.Fatalf("recorded files: %v", err)
	}
	if defs, err := h.rt.List(ctx, w.ID); err != nil || len(defs) != 0 {
		t.Fatalf("a built-in run recorded Workspace definitions: %+v, %v", defs, err)
	}
	// Another run and another runtime pin the same version.
	h.rt = h.runtime()
	again := h.startSplit(w.ID, "b2", false).Run.Run
	if again.Version != 1 || again.Commit != run.Commit {
		t.Fatalf("second run = %+v", again)
	}
	views, err := h.rt.Builtins(ctx, w.ID)
	if err != nil || len(views) != 1 || !views[0].Enabled || views[0].Version != 1 || len(views[0].Versions) != 1 || !views[0].Versions[0].Current ||
		views[0].Workspace == nil || views[0].Workspace.Source != "builtin" || len(views[0].Workspace.Blocked) != 0 {
		t.Fatalf("built-ins = %+v, %v", views, err)
	}
	if text := RunText(h.inspect(run.ID)); !strings.Contains(text, "built-in version 1") {
		t.Fatalf("inspection does not say it is a built-in:\n%s", text)
	}
	// A built-in run is never migrated: its version stays resumable.
	if _, err := h.rt.Migrate(ctx, MigrationRequest{RunID: h.startSplit(w.ID, "b3", true).Run.Run.ID, ToVersion: 2, Surface: "cli"}); err == nil ||
		!strings.Contains(err.Error(), "stays resumable on its own pins") {
		t.Fatalf("migrating a built-in run: %v", err)
	}
}

// Evie shipping a newer built-in version leaves runs pinned to the older
// one continuing on it: the older version's files stay at their commit. An
// Evie that cannot read a pinned version pauses the run visibly (ADR 0025)
// instead of running another.
func TestBuiltinRunsKeepTheirVersionAfterEvieShipsANewerOne(t *testing.T) {
	ctx := context.Background()
	h := splitBuiltinHarness(t, filepath.Join(testdata, "split-bill"))
	w := h.workspace()
	old := h.startSplit(w.ID, "old", true).Run.Run
	newer := copyDefinition(t, "split-bill")
	replaceIn(t, newer, "workflow.yaml", "max_attempts: 10", "max_attempts: 12")
	h.builtins = dirBuiltins(t, map[string]string{"split-bill": newer}, map[string]string{"split-bill": splitFixtureJSON})
	h.rt = h.runtime()
	fresh := h.startSplit(w.ID, "new", false).Run.Run
	if fresh.Version != 2 || fresh.ExecutableHash == old.ExecutableHash || fresh.Commit == old.Commit || fresh.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("run of the newer built-in = %+v", fresh)
	}
	// An Evie whose procedural repository lacks the pinned commit pauses it.
	elsewhere, err := New(Config{Store: h.store, Repo: procedural.New(filepath.Join(t.TempDir(), "other")), Capabilities: h.providers, Workers: ceiling})
	if err != nil {
		t.Fatal(err)
	}
	elsewhere.builtinSet = *h.builtins
	if _, err := elsewhere.execute(ctx, old.ID); err != nil {
		t.Fatal(err)
	}
	paused := h.inspect(old.ID).Run
	if paused.State != eviedb.WorkflowRunWaitingForDependency || !strings.Contains(paused.Error, "built-in version 1") || !strings.Contains(paused.Error, "continue it with an Evie that can read it") {
		t.Fatalf("run without its version = %+v", paused)
	}
	// This Evie continues it on version 1.
	h.clock.Advance(2 * time.Minute)
	h.drive(old.ID)
	done := h.inspect(old.ID).Run
	if done.State != eviedb.WorkflowRunSucceeded || done.Version != 1 || done.ExecutableHash != old.ExecutableHash {
		t.Fatalf("the older run = %+v", done)
	}
}

func TestBuiltinDisableEnableAndCustomOverrides(t *testing.T) {
	ctx := context.Background()
	h := splitBuiltinHarness(t, filepath.Join(testdata, "split-bill"))
	w, other, third := h.workspace(), h.workspace(), h.workspace()
	h.startSplit(w.ID, "w-builtin", false)
	if _, err := h.rt.ControlBuiltin(ctx, BuiltinControl{Name: "split-bill", Enabled: false, Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("a chat disabled a built-in: %v", err)
	}
	if _, err := h.rt.ControlBuiltin(ctx, BuiltinControl{Name: "no-such", Enabled: false, Surface: "cli"}); !errors.Is(err, ErrBuiltinUnknown) {
		t.Fatalf("an unknown built-in: %v", err)
	}
	// A Workspace's approved copy overrides the built-in there only.
	card, err := h.rt.Customize(ctx, Customization{Name: "split-bill", WorkspaceID: w.ID, Surface: "cli"})
	if err != nil || card.Template != "split-bill" || !strings.Contains(card.Text(), "Template: split-bill (bundled with Evie)") || card.Proposal.Simulation == nil || card.Proposal.Simulation.Status != "succeeded" {
		t.Fatalf("customize = %+v, %v", card, err)
	}
	if _, err := h.rt.Customize(ctx, Customization{Name: "split-bill", WorkspaceID: w.ID, Global: true, Surface: "cli"}); err == nil {
		t.Fatal("a customization named both a Workspace and Global")
	}
	custom := h.approve(card)
	if run := h.startSplit(w.ID, "w-custom", false).Run.Run; run.Source != eviedb.WorkflowSourceWorkspace || run.Version != custom.Version {
		t.Fatalf("run with a Workspace copy = %+v", run)
	}
	if run := h.startSplit(other.ID, "o-builtin", false).Run.Run; run.Source != eviedb.WorkflowSourceBuiltin {
		t.Fatalf("another Workspace = %+v", run)
	}
	// A Global approved copy overrides it everywhere else.
	gcard, err := h.rt.Customize(ctx, Customization{Name: "split-bill", Global: true, Surface: "cli"})
	if err != nil || gcard.Proposal.Source != eviedb.WorkflowSourceGlobal || gcard.Proposal.Path != "global/workflows/split-bill" ||
		!strings.Contains(gcard.Text(), "Global workflow split-bill") || !strings.Contains(gcard.ApproveCommand, gcard.Proposal.ID) {
		t.Fatalf("Global customize = %+v, %v", gcard, err)
	}
	g := h.approve(gcard)
	if g.Source != eviedb.WorkflowSourceGlobal || g.Version != 1 {
		t.Fatalf("Global approval = %+v", g)
	}
	if run := h.startSplit(other.ID, "o-global", false).Run.Run; run.Source != eviedb.WorkflowSourceGlobal || run.Path != "global/workflows/split-bill" {
		t.Fatalf("run with a Global copy = %+v", run)
	}
	if run := h.startSplit(w.ID, "w-custom-2", false).Run.Run; run.Source != eviedb.WorkflowSourceWorkspace {
		t.Fatalf("the Workspace copy lost to the Global one: %+v", run)
	}
	views, err := h.rt.Builtins(ctx, other.ID)
	if err != nil || views[0].Workspace.Source != "global" || views[0].Global == nil || views[0].Global.Status != eviedb.WorkflowActive {
		t.Fatalf("built-ins in another Workspace = %+v, %v", views, err)
	}
	// Deactivating the Global copy brings the built-in back.
	if _, err := h.rt.Deactivate(ctx, Deactivation{Global: true, Name: "split-bill", Version: 1, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	// Disabling the built-in stops it where no copy overrides it.
	off, err := h.rt.ControlBuiltin(ctx, BuiltinControl{Name: "split-bill", Enabled: false, Reason: "use the copies", Surface: "cli"})
	if err != nil || off.Enabled {
		t.Fatalf("disable = %+v, %v", off, err)
	}
	if _, err := h.rt.StartRun(ctx, RunRequest{WorkspaceID: third.ID, Workflow: "split-bill", TextInputs: splitInputs("t-off"), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkflowBuiltinDisabled) ||
		!strings.Contains(err.Error(), "evie workflows builtin enable split-bill") {
		t.Fatalf("start of a disabled built-in: %v", err)
	}
	if run := h.startSplit(w.ID, "w-custom-3", false).Run.Run; run.Source != eviedb.WorkflowSourceWorkspace {
		t.Fatalf("disabling the built-in stopped the Workspace copy: %+v", run)
	}
	views, err = h.rt.Builtins(ctx, third.ID)
	if err != nil || views[0].Enabled || !strings.Contains(strings.Join(views[0].Workspace.Blocked, " "), "disabled") {
		t.Fatalf("built-ins while disabled = %+v, %v", views, err)
	}
	if _, err := h.rt.ControlBuiltin(ctx, BuiltinControl{Name: "split-bill", Enabled: true, Surface: "web"}); err != nil {
		t.Fatal(err)
	}
	if run := h.startSplit(third.ID, "t-on", false).Run.Run; run.Source != eviedb.WorkflowSourceBuiltin {
		t.Fatalf("after enabling = %+v", run)
	}
}

func TestGlobalDefinitionsQualifyLikeBuiltins(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	_, err := h.rt.Propose(ctx, Proposal{Global: true, Dir: filepath.Join(testdata, "notes-digest"), Fixtures: fixtures(t, notesFixtures), Surface: "cli"})
	if !errors.Is(err, eviedb.ErrWorkflowNotStandalone) || !strings.Contains(err.Error(), "calls a capability") || !strings.Contains(err.Error(), "declares a schedule") {
		t.Fatalf("a Global definition with effects: %v", err)
	}
	if _, err := h.rt.Propose(ctx, Proposal{Global: true, Dir: filepath.Join(testdata, "split-bill"), Surface: "chat"}); !errors.Is(err, eviedb.ErrWorkflowApprover) {
		t.Fatalf("a chat proposed a Global definition: %v", err)
	}
	if _, err := h.rt.Customize(ctx, Customization{Name: "no-such", Global: true, Surface: "cli"}); !errors.Is(err, ErrBuiltinUnknown) {
		t.Fatalf("customizing an unknown built-in: %v", err)
	}
}

// The built-in deep-research (v2) starts from a Workspace chat with no
// setup beyond research workers being allowed, runs to its checked report
// with scripted fakes, and records nothing as proposed or approved.
func TestBuiltinDeepResearchStartsFromAChatWithNoSetup(t *testing.T) {
	ctx := context.Background()
	chat := deepResearchV2Chat()
	h, _ := newAgentHarness(t, chat, deepResearchV2Workers(false))
	w := h.researchWorkspace()
	call := chatCall(t, h.store, w)
	catalog, err := h.rt.ChatWorkflows(ctx, call.Scope)
	if err != nil || len(catalog.Workflows) != 1 || catalog.Workflows[0].Name != "deep-research" || catalog.Workflows[0].Source != "builtin" ||
		catalog.Workflows[0].Unavailable != "" || catalog.Workflows[0].MaxTokens != 6_000_000 {
		t.Fatalf("catalog = %+v, %v", catalog, err)
	}
	call.Workflow = "deep-research"
	call.Inputs = map[string]json.RawMessage{"question": json.RawMessage(`"What limits the range of electric aircraft?"`)}
	started, err := h.rt.StartFromChat(ctx, call)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != "queued" || started.StartedFrom != "chat" || started.MaxTokens != 6_000_000 {
		t.Fatalf("started = %+v", started)
	}
	d := h.inspect(started.RunID)
	if d.Run.Source != eviedb.WorkflowSourceBuiltin || d.Run.Path != "builtin/workflows/deep-research" || d.Run.SessionID != call.Scope.SessionID {
		t.Fatalf("run = %+v", d.Run)
	}
	h.drive(started.RunID)
	status, err := h.rt.ChatRunStatus(ctx, call.Scope, started.RunID)
	if err != nil || status.State != "succeeded" || !strings.Contains(status.OutputsJSON, `"outcome":"confirmed"`) || !strings.Contains(status.OutputsJSON, `"status":"corrected"`) {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if defs, err := h.rt.List(ctx, w.ID); err != nil || len(defs) != 0 {
		t.Fatalf("definitions = %+v, %v", defs, err)
	}
}

// unavailableAgents is a worker supervisor whose workers cannot run: the
// Subagents plugin is off.
type unavailableAgents struct{ Agents }

func (unavailableAgents) RunWorkerAvailable(context.Context, string) error {
	return delegation.ErrUnavailable
}

// A chat start refused for David's settings names every setting the
// workflow needs in this Workspace at once.
func TestBuiltinChatStartTellsDavidEverythingToEnable(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.modelsOff = "AI steps are off: set EVIE_WORKFLOW_REMOTE_MODELS=on (with OPENROUTER_API_KEY)"
	h.rt = h.runtime()
	w := h.workspace()
	call := chatCall(t, h.store, w)
	call.Workflow = "deep-research"
	call.Inputs = map[string]json.RawMessage{"question": json.RawMessage(`"Which car should I buy for city driving under $30k?"`)}
	_, err := h.rt.StartFromChat(ctx, call)
	var needs *StartNeedsError
	if !errors.As(err, &needs) || len(needs.Needs) != 3 {
		t.Fatalf("refusal = %v", err)
	}
	for _, want := range []string{"EVIE_WORKFLOW_REMOTE_MODELS=on", "agent steps are off", "turns on research delegation in the Workspace's settings", "nothing was started", "Tell David"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal lacks %q: %v", want, err)
		}
	}
	if !errors.Is(err, ErrModelsUnavailable) {
		t.Fatalf("the refusal lost its cause: %v", err)
	}
	if runs := h.runs(w.ID); len(runs) != 0 {
		t.Fatalf("runs = %+v", runs)
	}
	// With models on and workers off at the plugin, it says how to turn
	// the plugin on; the disabled built-in is named too.
	h.models = NewOpenRouterModels(newFakeChat(reply(`{}`, 1, 1)))
	h.agentsFor = func(*eviedb.Store) Agents { return unavailableAgents{} }
	h.rt = h.runtime()
	if _, err := h.rt.ControlBuiltin(ctx, BuiltinControl{Name: "deep-research", Enabled: false, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	_, err = h.rt.StartFromChat(ctx, call)
	if !errors.As(err, &needs) || !strings.Contains(err.Error(), "evie plugins enable subagents") || !strings.Contains(err.Error(), "evie workflows builtin enable deep-research") {
		t.Fatalf("refusal = %v", err)
	}
}
