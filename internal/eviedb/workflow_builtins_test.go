package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

// Built-in and Global workflows (owner decision 2026-10-04). A built-in is
// available in every Workspace without a proposal or approval; the owner
// disables and enables it with immutable controls; a Global custom version
// overrides it everywhere and a Workspace's custom version overrides both
// in that Workspace. Runs pin their source and pass the Workspace's worker
// allowance and run admission at start and at every node.

const otherCommit = "89abcdef0123456789abcdef0123456789abcdef"

var deepResearchDir = filepath.Join("..", "workflow", "testdata", "deep-research")

// builtinVersion validates dir as Evie validates an embedded built-in and
// names the commit its files were recorded at.
func builtinVersion(t *testing.T, dir, commit string) WorkflowBuiltinVersion {
	t.Helper()
	v, err := workflow.Validate(workflow.DirSource{Dir: dir}, workflowTestEnv())
	if err != nil {
		t.Fatal(err)
	}
	return WorkflowBuiltinVersion{Name: v.Name(), Commit: commit, Path: BuiltinWorkflowDefinitionPath(v.Name()), ExecutableHash: v.Hash(),
		AuthorityHash: workflow.RequirementsOf(v.Report()).AuthorityHash, Report: v.Report()}
}

func recordBuiltin(t *testing.T, s *Store, v WorkflowBuiltinVersion) WorkflowBuiltinVersion {
	t.Helper()
	recorded, err := s.RecordWorkflowBuiltinVersion(context.Background(), v)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

func builtinStart(ws memory.WorkspaceID, v WorkflowBuiltinVersion, key string) WorkflowRunStart {
	return WorkflowRunStart{WorkspaceID: ws, Workflow: v.Name, Source: WorkflowSourceBuiltin, Version: v.Version, ExecutableHash: v.ExecutableHash,
		AuthorityHash: v.AuthorityHash, Composition: json.RawMessage(`{"runtime":"test"}`), RunKey: key, Inputs: json.RawMessage(`{"question":"q"}`),
		Start: "plan", Surface: "cli"}
}

func deepResearchFixtures(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "workflow", "testdata", "deep-research.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// testGlobalProposal is testProposal for a Global custom definition.
func testGlobalProposal(t *testing.T, dir, fixtures string) WorkflowProposal {
	t.Helper()
	p := testProposal(t, "", dir, fixtures)
	p.Path = GlobalWorkflowDefinitionPath(p.Name)
	return p
}

func researchWorkspace(t *testing.T, s *Store) memory.Workspace {
	t.Helper()
	w, err := s.RegisterWorkspaceWithOptions(context.Background(), WorkspaceRegistration{DisplayName: "Research", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBuiltinWorkflowIsAvailableByDefaultWithoutProposalOrApproval(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w, err := r.s.RegisterWorkspace(ctx, "Fresh")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.s.ResolveWorkflow(ctx, w.ID, "deep-research")
	if err != nil || res.Source != WorkflowSourceBuiltin || !res.BuiltinEnabled || res.Activation != nil || res.BuiltinControl != nil {
		t.Fatalf("fresh resolution = %+v, %v", res, err)
	}
	v := recordBuiltin(t, r.s, builtinVersion(t, deepResearchDir, testCommit))
	if v.Version != 1 || v.Path != "builtin/workflows/deep-research" || strings.Join(v.Workers, ",") != "research" {
		t.Fatalf("recorded = %+v", v)
	}
	// The same executable hash is the same version; another is the next.
	if again := recordBuiltin(t, r.s, builtinVersion(t, deepResearchDir, otherCommit)); again.Version != 1 || again.Commit != testCommit {
		t.Fatalf("re-recorded = %+v", again)
	}
	edited := copyTestDefinition(t, "deep-research")
	if err := os.WriteFile(filepath.Join(edited, "prompts", "plan.md"), []byte("Plan the angles differently.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v2 := recordBuiltin(t, r.s, builtinVersion(t, edited, otherCommit)); v2.Version != 2 {
		t.Fatalf("second built-in version = %+v", v2)
	}
	// The Workspace's worker allowance still applies, and says what to turn on.
	_, _, err = r.s.StartWorkflowRun(ctx, builtinStart(w.ID, v, "deep-research:q"))
	var blocked *WorkflowBlockedError
	if !errors.As(err, &blocked) || !strings.Contains(err.Error(), "turns on research delegation in the Workspace's settings") {
		t.Fatalf("start without research allowance: %v", err)
	}
	if _, err := r.s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true); err != nil {
		t.Fatal(err)
	}
	run, existing, err := r.s.StartWorkflowRun(ctx, builtinStart(w.ID, v, "deep-research:q"))
	if err != nil || existing {
		t.Fatalf("built-in start = %v, %v", existing, err)
	}
	if run.Source != WorkflowSourceBuiltin || run.Version != 1 || run.Commit != testCommit || run.Path != "builtin/workflows/deep-research" || run.ExecutableHash != v.ExecutableHash {
		t.Fatalf("run = %+v", run)
	}
	// Nothing was proposed or approved, and nothing lists it as a Workspace definition.
	if n := countRows(t, r.s, "workflow_proposals") + countRows(t, r.s, "workflow_activations"); n != 0 {
		t.Fatalf("%d proposal or activation rows", n)
	}
	if defs, err := r.s.ListWorkflowDefinitions(ctx, w.ID); err != nil || len(defs) != 0 {
		t.Fatalf("definitions = %+v, %v", defs, err)
	}
	if err := r.s.AdmitWorkflowRun(ctx, testLease(t, r.s, run.ID)); err != nil {
		t.Fatalf("admission of the built-in run: %v", err)
	}
	r.reopen()
	listed, err := r.s.ListWorkflowRuns(ctx, WorkflowRunFilter{WorkspaceID: w.ID})
	if err != nil || len(listed) != 1 || listed[0].Source != WorkflowSourceBuiltin {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	// A run cannot pin a built-in version that was never recorded.
	_, _, err = r.s.StartWorkflowRun(ctx, builtinStart(w.ID, WorkflowBuiltinVersion{Name: "deep-research", Version: 9, ExecutableHash: v.ExecutableHash, AuthorityHash: v.AuthorityHash}, "deep-research:x"))
	if !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("start of an unrecorded version: %v", err)
	}
	if _, err := r.db.ExecContext(ctx, `INSERT INTO workflow_runs SELECT 'forged',workspace_id,workflow,7,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,composition,'forged-key',inputs,'queued',NULL,'',origin_surface,session_id,created_at,NULL,NULL,updated_at,NULL,source FROM workflow_runs WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "recorded built-in version") {
		t.Fatalf("a run of an unrecorded built-in version was stored: %v", err)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE workflow_runs SET source='workspace' WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("a run's source changed: %v", err)
	}
}

func TestBuiltinWorkflowVersionsHoldNoAuthority(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	notes := builtinVersion(t, filepath.Join("..", "workflow", "testdata", "notes-digest"), testCommit)
	if _, err := s.RecordWorkflowBuiltinVersion(ctx, notes); !errors.Is(err, ErrWorkflowNotStandalone) || !strings.Contains(err.Error(), "calls a capability") {
		t.Fatalf("a built-in with effects was recorded: %v", err)
	}
	v := builtinVersion(t, deepResearchDir, testCommit)
	v.Path = "workspaces/x/workflows/deep-research"
	if _, err := s.RecordWorkflowBuiltinVersion(ctx, v); !errors.Is(err, ErrWorkflowProposal) {
		t.Fatalf("a built-in outside builtin/workflows was recorded: %v", err)
	}
	recordBuiltin(t, s, builtinVersion(t, deepResearchDir, testCommit))
	// The database refuses a build worker, authority or a schedule by itself.
	report, _ := json.Marshal(notes.Report)
	for name, insert := range map[string][2]string{
		"build worker": {`["build"]`, `{"workflow":"x"}`},
		"authority":    {`["research"]`, string(report)},
	} {
		_, err := s.db.ExecContext(ctx, `INSERT INTO workflow_builtin_versions(name,version,commit_id,tree_path,executable_hash,authority_hash,authority,workers,report,recorded_by,recorded_at)
 VALUES('forged',1,?,?,'sha256:x','sha256:y','[]',?,?,'evie','2026-10-04T00:00:00Z')`, testCommit, "builtin/workflows/forged", insert[0], insert[1])
		if err == nil || !strings.Contains(err.Error(), "hold no Standing Authority") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// A report whose lists are null holds no authority either.
	if _, err := s.db.ExecContext(ctx, `INSERT INTO workflow_builtin_versions(name,version,commit_id,tree_path,executable_hash,authority_hash,authority,workers,report,recorded_by,recorded_at)
 VALUES('nulls',1,?,'builtin/workflows/nulls','sha256:x','sha256:y','[]','["research"]','{"requested_authority":null,"capabilities":null}','evie','2026-10-04T00:00:00Z')`, testCommit); err != nil {
		t.Fatalf("a report with null lists was refused: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE workflow_builtin_versions SET commit_id=commit_id`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("built-in version update: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM workflow_builtin_versions`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("built-in version delete: %v", err)
	}
}

func TestBuiltinWorkflowDisableAndEnable(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	w := researchWorkspace(t, r.s)
	v := recordBuiltin(t, r.s, builtinVersion(t, deepResearchDir, testCommit))
	before, _, err := r.s.StartWorkflowRun(ctx, builtinStart(w.ID, v, "deep-research:before"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.ControlWorkflowBuiltin(ctx, WorkflowBuiltinControlRequest{Name: "deep-research", Enabled: false, Surface: "chat"}); !errors.Is(err, ErrWorkflowApprover) {
		t.Fatalf("a chat disabled a built-in: %v", err)
	}
	if _, err := r.s.ControlWorkflowBuiltin(ctx, WorkflowBuiltinControlRequest{Name: "deep-research", Enabled: true, Surface: "cli"}); !errors.Is(err, ErrWorkflowBuiltinUnchanged) {
		t.Fatalf("enabling an enabled built-in: %v", err)
	}
	off, err := r.s.ControlWorkflowBuiltin(ctx, WorkflowBuiltinControlRequest{Name: "deep-research", Enabled: false, Reason: "trying a custom one", Surface: "cli"})
	if err != nil || off.Enabled || off.Reason != "trying a custom one" {
		t.Fatalf("disable = %+v, %v", off, err)
	}
	res, err := r.s.ResolveWorkflow(ctx, w.ID, "deep-research")
	if err != nil || res.Source != WorkflowSourceBuiltin || res.BuiltinEnabled || res.BuiltinControl == nil || res.BuiltinControl.Seq != off.Seq {
		t.Fatalf("resolution while disabled = %+v, %v", res, err)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, builtinStart(w.ID, v, "deep-research:off")); !errors.Is(err, ErrWorkflowBuiltinDisabled) || !errors.Is(err, ErrWorkflowNotActive) ||
		!strings.Contains(err.Error(), "evie workflows builtin enable deep-research") {
		t.Fatalf("start while disabled: %v", err)
	}
	// A run in progress stops at its next node (revocation applies despite the pin).
	lease := testLease(t, r.s, before.ID)
	if err := r.s.AdmitWorkflowRun(ctx, lease); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("admission of a run while disabled: %v", err)
	}
	on, err := r.s.ControlWorkflowBuiltin(ctx, WorkflowBuiltinControlRequest{Name: "deep-research", Enabled: true, Surface: "web"})
	if err != nil || !on.Enabled || on.Seq <= off.Seq {
		t.Fatalf("enable = %+v, %v", on, err)
	}
	if _, _, err := r.s.StartWorkflowRun(ctx, builtinStart(w.ID, v, "deep-research:on")); err != nil {
		t.Fatalf("start after enabling: %v", err)
	}
	state, err := r.s.WorkflowBuiltinState(ctx, "deep-research")
	if err != nil || !state.Enabled || len(state.Controls) != 2 || state.Controls[0].Seq != on.Seq || len(state.Versions) != 1 {
		t.Fatalf("state = %+v, %v", state, err)
	}
	if _, err := r.s.db.ExecContext(ctx, `UPDATE workflow_builtin_controls SET enabled=0`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("control update: %v", err)
	}
	if _, err := r.s.db.ExecContext(ctx, `DELETE FROM workflow_builtin_controls`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("control delete: %v", err)
	}
	// Turning research off pauses the built-in run too.
	if err := r.s.AdmitWorkflowRun(ctx, lease); err != nil {
		t.Fatalf("admission after enabling: %v", err)
	}
	if _, err := r.s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false); err != nil {
		t.Fatal(err)
	}
	if err := r.s.AdmitWorkflowRun(ctx, lease); !errors.Is(err, ErrWorkflowBlocked) || !strings.Contains(err.Error(), "no longer allows") {
		t.Fatalf("admission after research was turned off: %v", err)
	}
}

func TestWorkflowResolutionPrefersWorkspaceThenGlobalThenBuiltin(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	s := r.s
	w := researchWorkspace(t, s)
	other := researchWorkspace(t, s)
	fixtures := deepResearchFixtures(t)
	resolve := func(ws memory.WorkspaceID) WorkflowResolution {
		t.Helper()
		res, err := s.ResolveWorkflow(ctx, ws, "deep-research")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		return res
	}
	builtin := recordBuiltin(t, s, builtinVersion(t, deepResearchDir, testCommit))
	run, _, err := s.StartWorkflowRun(ctx, builtinStart(w.ID, builtin, "deep-research:first"))
	if err != nil {
		t.Fatal(err)
	}
	// A proposal alone overrides nothing.
	custom := recordProposal(t, s, testProposal(t, w.ID, deepResearchDir, fixtures))
	if res := resolve(w.ID); res.Source != WorkflowSourceBuiltin {
		t.Fatalf("a proposal overrode the built-in: %+v", res)
	}
	// An approved Global custom version overrides the built-in everywhere.
	gp, err := s.RecordGlobalWorkflowProposal(ctx, testGlobalProposal(t, deepResearchDir, fixtures))
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.ActivateGlobalWorkflow(ctx, approval(gp, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, ws := range []memory.WorkspaceID{w.ID, other.ID} {
		if res := resolve(ws); res.Source != WorkflowSourceGlobal || res.Activation.Version != g.Version || res.Activation.WorkspaceID != ws {
			t.Fatalf("with a Global version = %+v", res)
		}
	}
	// A start that still names the built-in is refused as stale.
	if _, _, err := s.StartWorkflowRun(ctx, builtinStart(w.ID, builtin, "deep-research:stale")); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("stale built-in start: %v", err)
	}
	globalRun, _, err := s.StartWorkflowRun(ctx, WorkflowRunStart{WorkspaceID: other.ID, Workflow: g.Name, Source: WorkflowSourceGlobal, Version: g.Version,
		ExecutableHash: g.ExecutableHash, AuthorityHash: g.AuthorityHash, Composition: json.RawMessage(`{}`), RunKey: "deep-research:global",
		Inputs: json.RawMessage(`{}`), Start: "plan", Surface: "cli"})
	if err != nil || globalRun.Source != WorkflowSourceGlobal || globalRun.Path != "global/workflows/deep-research" {
		t.Fatalf("Global start = %+v, %v", globalRun, err)
	}
	// The Workspace's approved custom version overrides both, there only.
	a := activate(t, s, custom, 0)
	if res := resolve(w.ID); res.Source != WorkflowSourceWorkspace || res.Activation.Version != a.Version {
		t.Fatalf("with a Workspace version = %+v", res)
	}
	if res := resolve(other.ID); res.Source != WorkflowSourceGlobal {
		t.Fatalf("another Workspace = %+v", res)
	}
	// The built-in run started before continues under its own pins.
	if err := s.AdmitWorkflowRun(ctx, testLease(t, s, run.ID)); err != nil {
		t.Fatalf("an earlier built-in run: %v", err)
	}
	// A blocked custom version is the answer, with its blocks; it never
	// falls through to another definition silently.
	off, err := s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s.ResolveWorkflow(ctx, w.ID, "deep-research")
	if !errors.Is(err, ErrWorkflowBlocked) || res.Source != WorkflowSourceWorkspace {
		t.Fatalf("blocked custom = %+v, %v", res, err)
	}
	if _, err := s.SetWorkspaceResearch(ctx, w.ID, off.CurrentRevisionID, true); err != nil {
		t.Fatal(err)
	}
	// Deactivating the Workspace's version falls back to the Global one.
	if _, err := s.DeactivateWorkflow(ctx, w.ID, "deep-research", a.Version, "back to Global", "cli"); err != nil {
		t.Fatal(err)
	}
	if res := resolve(w.ID); res.Source != WorkflowSourceGlobal {
		t.Fatalf("after deactivating the Workspace version = %+v", res)
	}
	// Deactivating the Global version falls back to the built-in, and pauses its runs.
	if _, err := s.DeactivateGlobalWorkflow(ctx, "deep-research", g.Version, "", "web"); err != nil {
		t.Fatal(err)
	}
	if res := resolve(w.ID); res.Source != WorkflowSourceBuiltin || !res.BuiltinEnabled {
		t.Fatalf("after deactivating the Global version = %+v", res)
	}
	if err := s.AdmitWorkflowRun(ctx, testLease(t, s, globalRun.ID)); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("a run of a deactivated Global version: %v", err)
	}
	// A Workspace's custom run of the same name shares the Run Key space.
	if _, existing, err := s.StartWorkflowRun(ctx, builtinStart(w.ID, builtin, "deep-research:first")); err != nil || !existing {
		t.Fatalf("same key: %v, %v", existing, err)
	}
}

func TestGlobalWorkflowProposalsQualifyLikeBuiltins(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	fixtures := deepResearchFixtures(t)
	notes := testGlobalProposal(t, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures)
	if _, err := s.RecordGlobalWorkflowProposal(ctx, notes); !errors.Is(err, ErrWorkflowNotStandalone) || !strings.Contains(err.Error(), "declares a schedule") {
		t.Fatalf("a Global proposal with effects was recorded: %v", err)
	}
	p := testGlobalProposal(t, deepResearchDir, fixtures)
	for name, mutate := range map[string]func(*WorkflowProposal){
		"a Workspace":   func(p *WorkflowProposal) { p.WorkspaceID = "w" },
		"a chat":        func(p *WorkflowProposal) { p.Surface = "chat" },
		"Evie":          func(p *WorkflowProposal) { p.ProposedBy = "evie" },
		"another path":  func(p *WorkflowProposal) { p.Path = "workspaces/w/workflows/deep-research" },
		"a commit-less": func(p *WorkflowProposal) { p.Commit = "main" },
	} {
		bad := p
		mutate(&bad)
		if _, err := s.RecordGlobalWorkflowProposal(ctx, bad); err == nil {
			t.Fatalf("a Global proposal from %s was recorded", name)
		}
	}
	undry := p
	undry.Simulation = nil
	noDryRun, err := s.RecordGlobalWorkflowProposal(ctx, undry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ActivateGlobalWorkflow(ctx, approval(noDryRun, 0)); !errors.Is(err, ErrWorkflowSimulationRequired) {
		t.Fatalf("approval without a dry run: %v", err)
	}
	recorded, err := s.RecordGlobalWorkflowProposal(ctx, p)
	if err != nil || recorded.Source != WorkflowSourceGlobal {
		t.Fatalf("Global proposal = %+v, %v", recorded, err)
	}
	if _, err := s.ActivateGlobalWorkflow(ctx, WorkflowApproval{ProposalID: recorded.ID, ExecutableHash: recorded.ExecutableHash, AuthorityHash: recorded.AuthorityHash,
		Approver: WorkflowApprover{Actor: "evie", Surface: "chat"}}); !errors.Is(err, ErrWorkflowApprover) {
		t.Fatalf("a chat approved a Global version: %v", err)
	}
	if _, err := s.ActivateGlobalWorkflow(ctx, approval(recorded, 1)); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("a stale review approved: %v", err)
	}
	g, err := s.ActivateGlobalWorkflow(ctx, approval(recorded, 0))
	if err != nil || g.Version != 1 || g.Source != WorkflowSourceGlobal || len(g.Authority) != 0 || strings.Join(g.Workers, ",") != "research" {
		t.Fatalf("Global activation = %+v, %v", g, err)
	}
	if _, err := s.ActivateGlobalWorkflow(ctx, approval(recorded, 1)); !errors.Is(err, ErrWorkflowAlreadyActive) {
		t.Fatalf("approving the active version again: %v", err)
	}
	// A README-only proposal documents the active version.
	doc, err := s.RecordGlobalWorkflowProposal(ctx, p)
	if err != nil || doc.DocumentsVersion != 1 {
		t.Fatalf("documentation proposal = %+v, %v", doc, err)
	}
	d, err := s.GlobalWorkflowDefinition(ctx, "deep-research")
	if err != nil || d.Status != WorkflowActive || d.Source != WorkflowSourceGlobal || d.Current.Version != 1 || len(d.Proposals) != 3 || d.Pending != 0 {
		t.Fatalf("Global definition = %+v, %v", d, err)
	}
	if _, err := s.DeactivateGlobalWorkflow(ctx, "deep-research", 1, "", "chat"); !errors.Is(err, ErrWorkflowApprover) {
		t.Fatalf("a chat deactivated a Global version: %v", err)
	}
	if _, err := s.DeactivateGlobalWorkflow(ctx, "deep-research", 1, "replaced", "cli"); err != nil {
		t.Fatal(err)
	}
	listed, err := s.ListGlobalWorkflowDefinitions(ctx)
	if err != nil || len(listed) != 1 || listed[0].Status != WorkflowInactive {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	for _, table := range []string{"workflow_global_proposals", "workflow_global_activations", "workflow_global_deactivations"} {
		if _, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET name=name`); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s update: %v", table, err)
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table); err == nil || !strings.Contains(err.Error(), "immutable") {
			t.Fatalf("%s delete: %v", table, err)
		}
	}
}

// A database from before built-in workflows pins every run to a Workspace
// activation by foreign key; the upgrade rebuilds workflow_runs once into
// the source column (every existing run a Workspace run), keeping every
// row, rowid, index and trigger, and adds the source checks.
func TestWorkflowRunsTableUpgradesToSources(t *testing.T) {
	ctx := context.Background()
	r := openRunDB(t)
	_, a := splitBillActivation(t, r.s)
	run, _, err := r.s.StartWorkflowRun(ctx, runStart(a, "split-bill:before"))
	if err != nil {
		t.Fatal(err)
	}
	var rowid int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, run.ID).Scan(&rowid); err != nil {
		t.Fatal(err)
	}
	downgradeWorkflowRunSources(t, r.db)
	var definition string
	if err := r.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_runs'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(definition, workflowRunActivationKey) || strings.Contains(definition, "source TEXT") {
		t.Fatalf("downgraded table = %s", definition)
	}
	r.reopen()
	if err := r.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_runs'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(definition, workflowRunActivationKey) || !strings.Contains(definition, workflowRunSource) {
		t.Fatalf("upgraded table = %s", definition)
	}
	got, err := r.s.WorkflowRunRecord(ctx, run.ID)
	if err != nil || got.Source != WorkflowSourceWorkspace {
		t.Fatalf("upgraded run = %+v, %v", got, err)
	}
	var kept int64
	if err := r.db.QueryRowContext(ctx, `SELECT rowid FROM workflow_runs WHERE id=?`, run.ID).Scan(&kept); err != nil || kept != rowid {
		t.Fatalf("rowid = %d, %v; want %d", kept, err, rowid)
	}
	var triggers int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name IN ('workflow_runs_pinned_version','workflow_runs_source_pinned','workflow_runs_pinned','workflow_runs_terminal')`).Scan(&triggers); err != nil || triggers != 4 {
		t.Fatalf("triggers = %d, %v", triggers, err)
	}
	// The Workspace check that replaced the foreign key still refuses an
	// unapproved version.
	if _, err := r.db.ExecContext(ctx, `INSERT INTO workflow_runs SELECT 'forged',workspace_id,workflow,99,commit_id,tree_path,executable_hash,authority_hash,workspace_revision_id,access_revision,composition,'forged',inputs,'queued',NULL,'',origin_surface,session_id,created_at,NULL,NULL,updated_at,NULL,source FROM workflow_runs WHERE id=?`, run.ID); err == nil || !strings.Contains(err.Error(), "approved Workspace") {
		t.Fatalf("a run of an unapproved version was stored: %v", err)
	}
}

// downgradeWorkflowRunSources puts workflow_runs back in its shape from
// before built-in workflows: the foreign key in place of the source
// column, without the source checks.
func downgradeWorkflowRunSources(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	exec := func(statement string) {
		t.Helper()
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	exec(`PRAGMA foreign_keys=OFF`)
	exec(`PRAGMA legacy_alter_table=ON`)
	defer exec(`PRAGMA legacy_alter_table=OFF`)
	defer exec(`PRAGMA foreign_keys=ON`)
	exec(`DROP TRIGGER workflow_runs_pinned_version`)
	exec(`DROP TRIGGER workflow_runs_source_pinned`)
	var definition string
	if err := conn.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='workflow_runs'`).Scan(&definition); err != nil {
		t.Fatal(err)
	}
	var dependents []string
	rows, err := conn.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name='workflow_runs' AND type IN ('index','trigger') AND sql IS NOT NULL`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		dependents = append(dependents, s)
	}
	rows.Close()
	old := workflowRunsTableName.ReplaceAllString(strings.Replace(definition, workflowRunSource, workflowRunActivationKey, 1), "CREATE TABLE workflow_runs_old (")
	columns := strings.Join(except(runColumnsAll), ",")
	exec(old)
	exec(`INSERT INTO workflow_runs_old(rowid,` + columns + `) SELECT rowid,` + columns + ` FROM workflow_runs`)
	exec(`DROP TABLE workflow_runs`)
	exec(`ALTER TABLE workflow_runs_old RENAME TO workflow_runs`)
	for _, s := range dependents {
		exec(s)
	}
}
