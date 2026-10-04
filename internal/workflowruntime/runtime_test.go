package workflowruntime

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/workflow"
)

var testdata = filepath.Join("..", "workflow", "testdata")

func notesCatalog() workflow.Catalog {
	note := workflow.ObjectType(workflow.Field{Name: "id", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()})
	return workflow.NewCatalog(
		workflow.CapabilityContract{ID: "notes.list", Contract: "1.0.0",
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "notes", Type: workflow.ArrayType(note)})},
		workflow.CapabilityContract{ID: "notes.append", Contract: "1.0.0", Effect: true, Idempotent: true,
			Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()}),
			Output: workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()})},
	)
}

var ceiling = workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000}

type harness struct {
	t     *testing.T
	store *eviedb.Store
	repo  *procedural.Repo
	rt    *Runtime
	// clock, when set, is the store's and runtime's clock; scheduler
	// configures each runtime's scheduler and leases.
	clock     *testClock
	scheduler SchedulerConfig
	// providers are every runtime's capability providers.
	providers CapabilityProviders
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	h := &harness{t: t, store: eviedb.NewStore(db), repo: procedural.New(filepath.Join(t.TempDir(), "procedural"))}
	h.rt = h.runtime()
	return h
}

// runtime is a runtime over the harness's store with its capability
// providers (the notes contracts, which no run can call, unless a test set
// others).
func (h *harness) runtime() *Runtime {
	h.t.Helper()
	if h.providers == nil {
		h.providers = ContractsOnly(notesCatalog())
	}
	return h.runtimeWith(h.providers)
}

func (h *harness) runtimeWith(providers CapabilityProviders) *Runtime {
	h.t.Helper()
	rt, err := New(Config{Store: h.store, Repo: h.repo, Capabilities: providers, Workers: ceiling, Scheduler: h.scheduler})
	if err != nil {
		h.t.Fatal(err)
	}
	if h.clock != nil {
		h.store.UseClock(h.clock.Now, time.UTC)
		rt.now = h.clock.Now
	}
	return rt
}

func (h *harness) workspace(resources ...string) memory.Workspace {
	h.t.Helper()
	ctx := context.Background()
	w, err := h.store.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		h.t.Fatal(err)
	}
	if len(resources) > 0 {
		h.setAccess(w.ID, 0, resources...)
	}
	return w
}

func (h *harness) setAccess(ws memory.WorkspaceID, expected int64, resources ...string) {
	h.t.Helper()
	access := workflow.WorkspaceAccess{}
	if len(resources) > 0 {
		access.Connections = []workflow.ConnectionAccess{{ID: "conn-notes", Resources: resources}}
	}
	if _, err := h.store.SetWorkspaceAccess(context.Background(), ws, expected, access); err != nil {
		h.t.Fatal(err)
	}
}

func fixtures(t *testing.T, raw string) *workflow.Fixtures {
	t.Helper()
	f, err := workflow.ParseFixtures([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return &f
}

const notesFixtures = `{"inputs": {"day": "2026-10-05"}, "steps": {
	"read": {"output": {"notes": [{"id": "n1", "text": "Call the bank."}]}},
	"summarize": {"output": {"digest": "- Call the bank."}},
	"write": {"output": {"entry_id": "e1"}}}}`

func deepResearchFixtures(t *testing.T) *workflow.Fixtures {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdata, "deep-research.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	return fixtures(t, string(data))
}

func (h *harness) propose(ws memory.WorkspaceID, dir string, f *workflow.Fixtures) Card {
	h.t.Helper()
	card, err := h.rt.Propose(context.Background(), Proposal{WorkspaceID: ws, Dir: dir, Fixtures: f, Surface: "cli"})
	if err != nil {
		h.t.Fatal(err)
	}
	return card
}

// approvalFor is exactly what the card showed.
func approvalFor(c Card) Approval {
	return Approval{ProposalID: c.Proposal.ID, ExecutableHash: c.Proposal.ExecutableHash, AuthorityHash: c.Proposal.AuthorityHash,
		ExpectedVersion: c.ExpectedVersion, Surface: "cli"}
}

func (h *harness) approve(c Card) eviedb.WorkflowActivation {
	h.t.Helper()
	a, err := h.rt.Approve(context.Background(), approvalFor(c))
	if err != nil {
		h.t.Fatal(err)
	}
	return a
}

func (h *harness) show(ws memory.WorkspaceID, name string) eviedb.WorkflowDefinitionState {
	h.t.Helper()
	d, err := h.rt.Show(context.Background(), ws, name)
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

func copyDefinition(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join(testdata, name)
	dst := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Proposal, validation and simulation, then one owner approval: the
// definition's exact files are committed to procedural Git, the review card
// shows what approval would bind, and approval activates exactly that.
func TestWorkflowProposeValidateSimulateApproveActive(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	dir := filepath.Join(testdata, "notes-digest")
	card := h.propose(w.ID, dir, fixtures(t, notesFixtures))
	p := card.Proposal
	if !card.NeedsApproval || card.ExpectedVersion != 0 || card.Current != nil || len(card.Refusals) != 0 {
		t.Fatalf("card = %+v", card)
	}
	if p.Simulation == nil || p.Simulation.Status != workflow.SimulationSucceeded || len(p.Simulation.Previews) != 1 {
		t.Fatalf("simulation = %+v", p.Simulation)
	}
	if p.Path != "workspaces/"+string(w.ID)+"/workflows/notes-digest" || p.ProposedBy != "owner" {
		t.Fatalf("proposal = %+v", p)
	}
	source, err := os.ReadFile(filepath.Join(dir, "workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := h.repo.ReadFile(ctx, p.Commit, p.Path+"/workflow.yaml")
	if err != nil || string(committed) != string(source) {
		t.Fatalf("committed workflow.yaml differs: %v", err)
	}
	if !card.Diff.ExecutableChanged || len(card.Diff.Authority) != 2 || !strings.Contains(card.Patch, "b/workflow.yaml") {
		t.Fatalf("diff = %+v patch:\n%s", card.Diff, card.Patch)
	}
	text := card.Text()
	for _, want := range []string{"notes-digest", p.ExecutableHash, p.AuthorityHash, "Requested Standing Authority", "notes.append on conn-notes",
		"Schedule: 0 7 * * 1-5 America/New_York", "also activates this schedule", "Dry run: succeeded", "evie workflows approve " + p.ID} {
		if !strings.Contains(text, want) {
			t.Fatalf("card text lacks %q:\n%s", want, text)
		}
	}
	if d := h.show(w.ID, "notes-digest"); d.Status != eviedb.WorkflowProposed || d.Pending != 1 {
		t.Fatalf("before approval = %+v", d)
	}
	a := h.approve(card)
	if a.Version != 1 || a.ExecutableHash != p.ExecutableHash || a.AuthorityHash != p.AuthorityHash || a.Commit != p.Commit || a.Approver.Surface != "cli" {
		t.Fatalf("activation = %+v", a)
	}
	if d := h.show(w.ID, "notes-digest"); d.Status != eviedb.WorkflowActive || d.Pending != 0 || d.Current.Version != 1 {
		t.Fatalf("after approval = %+v", d)
	}
	listed, err := h.rt.List(ctx, "")
	if err != nil || len(listed) != 1 || listed[0].Status != eviedb.WorkflowActive {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	// The card for the approved proposal now needs nothing.
	again, err := h.rt.Card(ctx, p.ID)
	if err != nil || again.NeedsApproval || again.ExpectedVersion != 1 {
		t.Fatalf("card after approval = %+v, %v", again, err)
	}
}

// Approval binds the exact hashes the owner reviewed; anything else, or a
// stale review, is refused.
func TestWorkflowApprovalBindsTheExactHashAndAuthority(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	card := h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures))
	wrong := approvalFor(card)
	wrong.ExecutableHash = "sha256:" + strings.Repeat("a", 64)
	if _, err := h.rt.Approve(ctx, wrong); !errors.Is(err, eviedb.ErrWorkflowHashMismatch) {
		t.Fatalf("wrong executable hash: %v", err)
	}
	wrong = approvalFor(card)
	wrong.AuthorityHash = workflow.AuthorityHash(nil)
	if _, err := h.rt.Approve(ctx, wrong); !errors.Is(err, eviedb.ErrWorkflowHashMismatch) {
		t.Fatalf("wrong authority hash: %v", err)
	}
	stale := approvalFor(card)
	stale.ExpectedVersion = 4
	if _, err := h.rt.Approve(ctx, stale); !errors.Is(err, eviedb.ErrWorkflowChanged) {
		t.Fatalf("stale review: %v", err)
	}
	h.approve(card)
}

// Any behavior change is a new proposal that needs approval, while the
// approved version keeps running; a README-only change is recorded against
// the active version and needs none.
func TestWorkflowBehaviorChangeNeedsReapprovalButREADMEDoesNot(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	first := h.approve(h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures)))

	docs := copyDefinition(t, "notes-digest")
	writeFile(t, filepath.Join(docs, "README.md"), "# Notes digest\n\nA clearer explanation.\n")
	docCard := h.propose(w.ID, docs, nil)
	if docCard.NeedsApproval || docCard.Proposal.DocumentsVersion != 1 || docCard.Diff.ExecutableChanged || !docCard.Diff.DocumentationChanged {
		t.Fatalf("README-only card = %+v", docCard)
	}
	if !strings.Contains(docCard.Text(), "No approval needed") || !strings.Contains(docCard.Patch, "+A clearer explanation.") {
		t.Fatalf("README-only card text:\n%s\n%s", docCard.Text(), docCard.Patch)
	}
	if d := h.show(w.ID, "notes-digest"); d.Status != eviedb.WorkflowActive || d.Current.Version != 1 || d.Pending != 0 {
		t.Fatalf("after README-only = %+v", d)
	}

	changed := copyDefinition(t, "notes-digest")
	writeFile(t, filepath.Join(changed, "prompts", "summarize.md"), "Summarize the notes in at most three bullets.\n")
	card := h.propose(w.ID, changed, fixtures(t, notesFixtures))
	if !card.NeedsApproval || card.ExpectedVersion != 1 || !card.Diff.ExecutableChanged || len(card.Diff.Authority) != 0 {
		t.Fatalf("behavior change card = %+v", card)
	}
	found := false
	for _, s := range card.Diff.Executable {
		found = found || s.Section == "Prompts and schemas"
	}
	if !found || !strings.Contains(card.Patch, "+Summarize the notes in at most three bullets.") {
		t.Fatalf("diff = %+v patch:\n%s", card.Diff, card.Patch)
	}
	if d := h.show(w.ID, "notes-digest"); d.Status != eviedb.WorkflowActive || d.Current.Version != 1 || d.Current.ExecutableHash != first.ExecutableHash || d.Pending != 1 {
		t.Fatalf("a behavior change took effect before approval: %+v", d)
	}
	second := h.approve(card)
	d := h.show(w.ID, "notes-digest")
	if second.Version != 2 || d.Current.ExecutableHash != card.Proposal.ExecutableHash || len(d.Versions) != 2 || d.Versions[1].ExecutableHash != first.ExecutableHash {
		t.Fatalf("after re-approval = %+v", d)
	}
	if _, err := h.rt.Approve(ctx, approvalFor(docCard)); !errors.Is(err, eviedb.ErrWorkflowChanged) {
		t.Fatalf("approving a review of an older version: %v", err)
	}
}

// Requested Standing Authority must be within Workspace Access: a proposal
// outside it is invalid, and access narrowed after a proposal refuses its
// approval.
func TestWorkflowAuthorityOutsideWorkspaceAccessIsRefused(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace()
	_, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Dir: filepath.Join(testdata, "notes-digest"), Fixtures: fixtures(t, notesFixtures), Surface: "cli"})
	var invalid *workflow.InvalidError
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Error(), "outside Workspace Access") {
		t.Fatalf("proposal outside access: %v", err)
	}
	h.setAccess(w.ID, 0, "notebook:inbox", "notebook:digest")
	card := h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures))
	h.setAccess(w.ID, 1, "notebook:inbox")
	card, err = h.rt.Card(ctx, card.Proposal.ID)
	if err != nil || len(card.Refusals) == 0 || !strings.Contains(strings.Join(card.Refusals, " "), "notebook:digest") {
		t.Fatalf("card after narrowing = %+v, %v", card.Refusals, err)
	}
	if _, err := h.rt.Approve(ctx, approvalFor(card)); err == nil || !strings.Contains(err.Error(), "outside Workspace Access") {
		t.Fatalf("approval outside access: %v", err)
	}
	if d := h.show(w.ID, "notes-digest"); d.Status != eviedb.WorkflowProposed {
		t.Fatalf("state = %+v", d)
	}
}

// Agent steps need the Workspace's worker allowance at activation; turning
// the worker off afterwards blocks the approved definition.
func TestWorkflowWorkerAllowanceAtActivationAndRevocation(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace()
	card := h.propose(w.ID, filepath.Join(testdata, "deep-research"), deepResearchFixtures(t))
	if len(card.Refusals) == 0 || !strings.Contains(strings.Join(card.Refusals, " "), "research workers") {
		t.Fatalf("card refusals = %v", card.Refusals)
	}
	_, err := h.rt.Approve(ctx, approvalFor(card))
	if !errors.Is(err, eviedb.ErrWorkflowRefused) {
		t.Fatalf("approval without research allowance: %v", err)
	}
	on, err := h.store.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if card, err = h.rt.Card(ctx, card.Proposal.ID); err != nil || len(card.Refusals) != 0 {
		t.Fatalf("card after allowing research = %v, %v", card.Refusals, err)
	}
	h.approve(card)
	if _, err := h.store.SetWorkspaceResearch(ctx, w.ID, on.CurrentRevisionID, false); err != nil {
		t.Fatal(err)
	}
	d := h.show(w.ID, "deep-research")
	if d.Status != eviedb.WorkflowBlocked || !strings.Contains(strings.Join(d.Blocked, " "), "research workers") {
		t.Fatalf("after research off = %+v", d)
	}
}

// Narrowing Workspace Access blocks an approved definition at once; the
// owner's deactivation revokes it; history keeps both.
func TestWorkflowAccessNarrowingBlocksAndDeactivationRevokes(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	h.approve(h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures)))
	h.setAccess(w.ID, 1, "notebook:inbox")
	d := h.show(w.ID, "notes-digest")
	if d.Status != eviedb.WorkflowBlocked || len(d.Blocked) != 1 {
		t.Fatalf("after narrowing = %+v", d)
	}
	if _, err := h.store.ActiveWorkflow(ctx, w.ID, "notes-digest"); !errors.Is(err, eviedb.ErrWorkflowBlocked) {
		t.Fatalf("run admission: %v", err)
	}
	if _, err := h.rt.Deactivate(ctx, Deactivation{WorkspaceID: w.ID, Name: "notes-digest", Version: 1, Reason: "retired", Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	d = h.show(w.ID, "notes-digest")
	if d.Status != eviedb.WorkflowInactive || d.Current.Deactivated == nil || d.Current.Deactivated.Reason != "retired" {
		t.Fatalf("after deactivation = %+v", d)
	}
	if _, err := h.store.ActiveWorkflow(ctx, w.ID, "notes-digest"); !errors.Is(err, eviedb.ErrWorkflowNotActive) {
		t.Fatalf("run admission after deactivation: %v", err)
	}
}

// Approval re-reads the proposal's exact commit and re-validates it against
// the current capability catalog: later commits in procedural Git cannot
// change what is approved, and a vanished capability refuses it.
func TestWorkflowApprovalRevalidatesTheExactCommit(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	card := h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures))
	// Someone commits other content at the same path afterwards.
	tampered := map[string][]byte{"workflow.yaml": []byte("schema_version: 1\nname: notes-digest\n"), "README.md": []byte("# x\n")}
	if _, _, err := h.repo.Commit(ctx, card.Proposal.Path, tampered, "edit outside review"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runtimeWith(ContractsOnly(workflow.NewCatalog())).Approve(ctx, approvalFor(card)); err == nil || !strings.Contains(err.Error(), "not in the capability catalog") {
		t.Fatalf("approval without the capabilities: %v", err)
	}
	a := h.approve(card)
	if a.Commit != card.Proposal.Commit {
		t.Fatalf("activation commit %s, proposal commit %s", a.Commit, card.Proposal.Commit)
	}
}

// A proposal to an archived or unknown Workspace is refused before
// anything is committed to procedural Git.
func TestWorkflowProposalNeedsAnActiveWorkspace(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	if _, err := h.store.ArchiveWorkspace(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(testdata, "notes-digest")
	if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Dir: dir, Fixtures: fixtures(t, notesFixtures), Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkspaceInactive) {
		t.Fatalf("archived Workspace: %v", err)
	}
	if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: "missing", Dir: dir, Surface: "cli"}); !errors.Is(err, eviedb.ErrWorkspaceNotFound) {
		t.Fatalf("unknown Workspace: %v", err)
	}
	if head, err := h.repo.Head(ctx); err != nil || head != "" {
		t.Fatalf("something was committed: %q, %v", head, err)
	}
}

// Only the owner's CLI and web surfaces approve or deactivate: there is no
// model or chat surface, and approval needs a dry run that succeeded.
func TestWorkflowModelsCannotApprove(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	w := h.workspace("notebook:inbox", "notebook:digest")
	unsimulated := h.propose(w.ID, filepath.Join(testdata, "notes-digest"), nil)
	if !strings.Contains(strings.Join(unsimulated.Refusals, " "), "dry run") {
		t.Fatalf("refusals = %v", unsimulated.Refusals)
	}
	if _, err := h.rt.Approve(ctx, approvalFor(unsimulated)); !errors.Is(err, eviedb.ErrWorkflowSimulationRequired) {
		t.Fatalf("approval without a dry run: %v", err)
	}
	card := h.propose(w.ID, filepath.Join(testdata, "notes-digest"), fixtures(t, notesFixtures))
	for _, surface := range []string{"chat", "tool", "model", ""} {
		a := approvalFor(card)
		a.Surface = surface
		if _, err := h.rt.Approve(ctx, a); !errors.Is(err, eviedb.ErrWorkflowApprover) {
			t.Fatalf("surface %q: %v", surface, err)
		}
	}
	for _, surface := range []string{"chat", ""} {
		if _, err := h.rt.Propose(ctx, Proposal{WorkspaceID: w.ID, Dir: filepath.Join(testdata, "notes-digest"), Surface: surface}); err == nil {
			t.Fatalf("surface %q proposed", surface)
		}
	}
	web := approvalFor(card)
	web.Surface = "web"
	if a, err := h.rt.Approve(ctx, web); err != nil || a.Approver.Surface != "web" || a.Approver.Actor != "owner" {
		t.Fatalf("web approval = %+v, %v", a, err)
	}
}
