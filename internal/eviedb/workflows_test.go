package eviedb

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/workflow"
)

const testCommit = "0123456789abcdef0123456789abcdef01234567"

// workflowTestEnv is the notes-digest example's review environment: a read
// and an idempotent effect on one connection with two resources.
func workflowTestEnv() workflow.Environment {
	note := workflow.ObjectType(workflow.Field{Name: "id", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()})
	return workflow.Environment{
		Capabilities: workflow.NewCatalog(
			workflow.CapabilityContract{ID: "notes.list", Contract: "1.0.0",
				Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}),
				Output: workflow.ObjectType(workflow.Field{Name: "notes", Type: workflow.ArrayType(note)})},
			workflow.CapabilityContract{ID: "notes.append", Contract: "1.0.0", Effect: true, Idempotent: true,
				Input:  workflow.ObjectType(workflow.Field{Name: "day", Type: workflow.StringType()}, workflow.Field{Name: "text", Type: workflow.StringType()}),
				Output: workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()})},
		),
		Access:  notesAccess("notebook:inbox", "notebook:digest"),
		Workers: workflow.WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000},
	}
}

func notesAccess(resources ...string) workflow.WorkspaceAccess {
	if len(resources) == 0 {
		return workflow.WorkspaceAccess{}
	}
	return workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: "conn-notes", Resources: resources}}}
}

const notesFixtures = `{"inputs": {"day": "2026-10-05"}, "steps": {
	"read": {"output": {"notes": [{"id": "n1", "text": "Call the bank."}]}},
	"summarize": {"output": {"digest": "- Call the bank."}},
	"write": {"output": {"entry_id": "e1"}}}}`

// copyTestDefinition copies ../workflow/testdata/<name> so a test can edit it.
func copyTestDefinition(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "workflow", "testdata", name)
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

// testProposal validates dir and, with fixtures, simulates it, as the
// runtime does before recording a proposal.
func testProposal(t *testing.T, ws memory.WorkspaceID, dir, fixtures string) WorkflowProposal {
	t.Helper()
	v, err := workflow.Validate(workflow.DirSource{Dir: dir}, workflowTestEnv())
	if err != nil {
		t.Fatal(err)
	}
	p := WorkflowProposal{
		WorkspaceID: ws, Name: v.Name(), Commit: testCommit, Path: "workspaces/" + string(ws) + "/workflows/" + v.Name(),
		ExecutableHash: v.Hash(), AuthorityHash: workflow.RequirementsOf(v.Report()).AuthorityHash, Report: v.Report(),
		ProposedBy: "owner", Surface: "cli",
	}
	if fixtures != "" {
		f, err := workflow.ParseFixtures([]byte(fixtures))
		if err != nil {
			t.Fatal(err)
		}
		if p.Simulation, err = v.Simulate(f); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func recordProposal(t *testing.T, s *Store, p WorkflowProposal) WorkflowProposal {
	t.Helper()
	recorded, err := s.RecordWorkflowProposal(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return recorded
}

var cliOwner = WorkflowApprover{Actor: "owner", Surface: "cli"}

func approval(p WorkflowProposal, expected int64) WorkflowApproval {
	return WorkflowApproval{ProposalID: p.ID, ExecutableHash: p.ExecutableHash, AuthorityHash: p.AuthorityHash, ExpectedVersion: expected, Approver: cliOwner}
}

func activate(t *testing.T, s *Store, p WorkflowProposal, expected int64) WorkflowActivation {
	t.Helper()
	a, err := s.ActivateWorkflow(context.Background(), approval(p, expected))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func notesWorkspace(t *testing.T, s *Store, resources ...string) memory.Workspace {
	t.Helper()
	ctx := context.Background()
	w, err := s.RegisterWorkspace(ctx, "Notes")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) > 0 {
		if _, err := s.SetWorkspaceAccess(ctx, w.ID, 0, notesAccess(resources...)); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func definitionState(t *testing.T, s *Store, ws memory.WorkspaceID, name string) WorkflowDefinitionState {
	t.Helper()
	d, err := s.WorkflowDefinition(context.Background(), ws, name)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func wantImmutable(t *testing.T, s *Store, table string) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, `UPDATE `+table+` SET workspace_id=workspace_id`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("%s update: %v", table, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("%s delete: %v", table, err)
	}
}

// Workspace Access is the owner's ceiling of Connection IDs and exact
// resources: each change is an immutable revision written with a
// compare-and-set, and the latest applies at once.
func TestWorkspaceAccessRevisionsAreImmutableAndCompareAndSet(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w, err := s.RegisterWorkspace(ctx, "Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	empty, err := s.WorkspaceAccess(ctx, w.ID)
	if err != nil || empty.Revision != 0 || len(empty.Access.Connections) != 0 {
		t.Fatalf("initial access = %+v, %v", empty, err)
	}
	set, err := s.SetWorkspaceAccess(ctx, w.ID, 0, workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{
		{ID: "conn-sheets", Resources: []string{"sheet:tips", "sheet:hours"}}, {ID: "conn-notes", Resources: []string{"notebook:inbox"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if set.Revision != 1 || set.Access.Connections[0].ID != "conn-notes" || !reflect.DeepEqual(set.Access.Connections[1].Resources, []string{"sheet:hours", "sheet:tips"}) {
		t.Fatalf("set = %+v", set)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 0, notesAccess("notebook:inbox")); !errors.Is(err, ErrWorkspaceAccessChanged) {
		t.Fatalf("stale write: %v", err)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 1, workflow.WorkspaceAccess{Connections: []workflow.ConnectionAccess{{ID: "c"}}}); err == nil {
		t.Fatal("malformed access was recorded")
	}
	if _, err := s.SetWorkspaceAccess(ctx, "missing", 0, notesAccess("r")); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("unknown Workspace: %v", err)
	}
	got, err := s.WorkspaceAccess(ctx, w.ID)
	if err != nil || got.Revision != 1 || !reflect.DeepEqual(got.Access, set.Access) {
		t.Fatalf("read back = %+v, %v", got, err)
	}
	wantImmutable(t, s, "workspace_access_revisions")
}

// One approval activates one exact proposal: its executable hash and its
// requested Standing Authority, with the schedule it declares, pinned to the
// Workspace revision and access it was checked against, by the owner.
func TestWorkflowApprovalActivatesTheExactProposal(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(db)
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	if p.ID == "" || p.DocumentsVersion != 0 || p.AccessRevision != 1 || p.WorkspaceRevision != w.CurrentRevisionID {
		t.Fatalf("proposal = %+v", p)
	}
	if d := definitionState(t, s, w.ID, "notes-digest"); d.Status != WorkflowProposed || d.Current != nil || len(d.Proposals) != 1 || !d.Proposals[0].Pending {
		t.Fatalf("proposed state = %+v", d)
	}

	for name, bad := range map[string]WorkflowApproval{
		"executable hash": {ProposalID: p.ID, ExecutableHash: "sha256:" + strings.Repeat("0", 64), AuthorityHash: p.AuthorityHash, Approver: cliOwner},
		"authority hash":  {ProposalID: p.ID, ExecutableHash: p.ExecutableHash, AuthorityHash: workflow.AuthorityHash(nil), Approver: cliOwner},
	} {
		if _, err := s.ActivateWorkflow(ctx, bad); !errors.Is(err, ErrWorkflowHashMismatch) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := s.ActivateWorkflow(ctx, approval(p, 3)); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("stale expected version: %v", err)
	}
	a := activate(t, s, p, 0)
	if a.Version != 1 || a.ExecutableHash != p.ExecutableHash || a.AuthorityHash != p.AuthorityHash || a.Commit != testCommit {
		t.Fatalf("activation = %+v", a)
	}
	if len(a.Authority) != 2 || a.Authority[0].Capability != "notes.append" || a.Schedule == nil || a.Schedule.Cron != "0 7 * * 1-5" {
		t.Fatalf("bound authority %+v schedule %+v", a.Authority, a.Schedule)
	}
	if a.WorkspaceRevision != w.CurrentRevisionID || a.AccessRevision != 1 || a.Approver != cliOwner || a.ApprovedAt.IsZero() {
		t.Fatalf("pins and evidence = %+v", a)
	}
	if _, err := s.ActivateWorkflow(ctx, approval(p, 1)); !errors.Is(err, ErrWorkflowAlreadyActive) {
		t.Fatalf("re-approving the active version: %v", err)
	}

	// The activation survives a restart exactly.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s = NewStore(db)
	d := definitionState(t, s, w.ID, "notes-digest")
	if d.Status != WorkflowActive || d.Current == nil || !reflect.DeepEqual(*d.Current, a) || len(d.Blocked) != 0 {
		t.Fatalf("after reopen = %+v, want %+v", d.Current, a)
	}
	if d.Proposals[0].Pending || !reflect.DeepEqual(d.Proposals[0].ApprovedVersions, []int64{1}) {
		t.Fatalf("proposal summary = %+v", d.Proposals[0])
	}
	active, err := s.ActiveWorkflow(ctx, w.ID, "notes-digest")
	if err != nil || active.Version != 1 {
		t.Fatalf("active = %+v, %v", active, err)
	}
	listed, err := s.ListWorkflowDefinitions(ctx, w.ID)
	if err != nil || len(listed) != 1 || listed[0].Status != WorkflowActive {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
}

// Approval needs a dry run of the proposal that reached the end, and only
// the owner approves, from the CLI or the web: no model or chat surface.
func TestWorkflowApprovalNeedsADryRunAndTheOwner(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	dir := filepath.Join("..", "workflow", "testdata", "notes-digest")
	unsimulated := recordProposal(t, s, testProposal(t, w.ID, dir, ""))
	if _, err := s.ActivateWorkflow(ctx, approval(unsimulated, 0)); !errors.Is(err, ErrWorkflowSimulationRequired) {
		t.Fatalf("no simulation: %v", err)
	}
	failed := recordProposal(t, s, testProposal(t, w.ID, dir, `{"inputs": {"day": "2026-10-05"}, "steps": {
		"read": {"output": {"notes": [{"id": "n1", "text": "x"}]}}, "summarize": {"output": {"digest": 7}}}}`))
	if failed.Simulation == nil || failed.Simulation.Status != workflow.SimulationFailed {
		t.Fatalf("simulation = %+v", failed.Simulation)
	}
	if _, err := s.ActivateWorkflow(ctx, approval(failed, 0)); !errors.Is(err, ErrWorkflowSimulationRequired) {
		t.Fatalf("failed simulation: %v", err)
	}
	ok := recordProposal(t, s, testProposal(t, w.ID, dir, notesFixtures))
	for _, approver := range []WorkflowApprover{{Actor: "evie", Surface: "cli"}, {Actor: "owner", Surface: "chat"}, {Actor: "owner", Surface: "tool"}, {}} {
		a := approval(ok, 0)
		a.Approver = approver
		if _, err := s.ActivateWorkflow(ctx, a); !errors.Is(err, ErrWorkflowApprover) {
			t.Fatalf("approver %+v: %v", approver, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO workflow_activations(workspace_id,name,version,proposal_id,commit_id,tree_path,executable_hash,authority_hash,authority,workers,workspace_revision_id,access_revision,approver,approval_surface,approval_detail,approved_at)
 VALUES(?,?,1,?,?,?,?,?,'[]','[]','r',0,'evie','chat','','2026-10-04T00:00:00Z')`, w.ID, ok.Name, ok.ID, testCommit, ok.Path, ok.ExecutableHash, ok.AuthorityHash); err == nil {
		t.Fatal("the database accepted a non-owner approval")
	}
	web := approval(ok, 0)
	web.Approver = WorkflowApprover{Actor: "owner", Surface: "web", Detail: "POST /api/workflows/approve"}
	if a, err := s.ActivateWorkflow(ctx, web); err != nil || a.Approver.Surface != "web" {
		t.Fatalf("web approval = %+v, %v", a, err)
	}
}

// Requested Standing Authority must be a subset of the Workspace's current
// access; Workspace Access alone grants nothing.
func TestWorkflowAuthorityMustBeWithinWorkspaceAccess(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s)
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	_, err := s.ActivateWorkflow(ctx, approval(p, 0))
	var refused *WorkflowRefusal
	if !errors.As(err, &refused) || !errors.Is(err, ErrWorkflowRefused) || len(refused.Reasons) != 2 || !strings.Contains(refused.Reasons[0], "conn-notes") {
		t.Fatalf("no access: %v", err)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 0, notesAccess("notebook:inbox")); err != nil {
		t.Fatal(err)
	}
	_, err = s.ActivateWorkflow(ctx, approval(p, 0))
	if !errors.As(err, &refused) || len(refused.Reasons) != 1 || !strings.Contains(refused.Reasons[0], "notebook:digest") {
		t.Fatalf("partial access: %v", err)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 1, notesAccess("notebook:inbox", "notebook:digest", "notebook:other")); err != nil {
		t.Fatal(err)
	}
	if a := activate(t, s, p, 0); a.AccessRevision != 2 {
		t.Fatalf("activation pinned access %d", a.AccessRevision)
	}
}

// Agent steps need the Workspace's allowance for their worker presets at
// activation, as chat delegation does at admission.
func TestWorkflowWorkerAllowanceIsCheckedAtActivation(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w, err := s.RegisterWorkspace(ctx, "Research")
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := os.ReadFile(filepath.Join("..", "workflow", "testdata", "deep-research.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "deep-research"), string(fixtures)))
	_, err = s.ActivateWorkflow(ctx, approval(p, 0))
	var refused *WorkflowRefusal
	if !errors.As(err, &refused) || !strings.Contains(strings.Join(refused.Reasons, " "), "research workers") {
		t.Fatalf("without research allowance: %v", err)
	}
	allowed, err := s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	a := activate(t, s, p, 0)
	if !reflect.DeepEqual(a.Workers, []string{"research"}) || a.WorkspaceRevision != allowed.CurrentRevisionID {
		t.Fatalf("activation = %+v", a)
	}
}

// Narrowing Workspace Access is a safety revocation: every activation whose
// authority it no longer covers is blocked at once, and widening it again
// does not restore the old approval; a new approval does.
func TestWorkflowAccessNarrowingBlocksTheDefinition(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	activate(t, s, p, 0)
	// Adding unrelated access blocks nothing.
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 1, notesAccess("notebook:inbox", "notebook:digest", "notebook:archive")); err != nil {
		t.Fatal(err)
	}
	if d := definitionState(t, s, w.ID, "notes-digest"); d.Status != WorkflowActive {
		t.Fatalf("after widening = %+v", d)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 2, notesAccess("notebook:inbox")); err != nil {
		t.Fatal(err)
	}
	d := definitionState(t, s, w.ID, "notes-digest")
	if d.Status != WorkflowBlocked || len(d.Blocked) != 1 || !strings.Contains(d.Blocked[0], "notebook:digest") || !strings.Contains(d.Blocked[0], "revision 3") {
		t.Fatalf("after narrowing = %+v", d)
	}
	_, err := s.ActiveWorkflow(ctx, w.ID, "notes-digest")
	var blocked *WorkflowBlockedError
	if !errors.As(err, &blocked) || !errors.Is(err, ErrWorkflowBlocked) || len(blocked.Reasons) != 1 {
		t.Fatalf("run admission after narrowing: %v", err)
	}
	listed, err := s.ListWorkflowDefinitions(ctx, "")
	if err != nil || len(listed) != 1 || listed[0].Status != WorkflowBlocked {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if _, err := s.SetWorkspaceAccess(ctx, w.ID, 3, notesAccess("notebook:inbox", "notebook:digest")); err != nil {
		t.Fatal(err)
	}
	if d := definitionState(t, s, w.ID, "notes-digest"); d.Status != WorkflowBlocked {
		t.Fatalf("re-granting access restored the old approval: %+v", d)
	}
	// Re-approving the same proposal is a new version on the current access.
	again := activate(t, s, p, 1)
	if again.Version != 2 || again.AccessRevision != 4 {
		t.Fatalf("re-approval = %+v", again)
	}
	if d := definitionState(t, s, w.ID, "notes-digest"); d.Status != WorkflowActive || len(d.Versions) != 2 || d.Versions[1].Version != 1 {
		t.Fatalf("after re-approval = %+v", d)
	}
}

// Turning a worker off revokes it for every revision that allowed it, so an
// activation pinned to such a revision is blocked even after the worker is
// turned on again; archiving the Workspace blocks everything in it.
func TestWorkflowWorkerRevocationAndArchiveBlock(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w, err := s.RegisterWorkspaceWithOptions(ctx, WorkspaceRegistration{DisplayName: "Research", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := os.ReadFile(filepath.Join("..", "workflow", "testdata", "deep-research.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "deep-research"), string(fixtures)))
	activate(t, s, p, 0)
	off, err := s.SetWorkspaceResearch(ctx, w.ID, w.CurrentRevisionID, false)
	if err != nil {
		t.Fatal(err)
	}
	d := definitionState(t, s, w.ID, "deep-research")
	if d.Status != WorkflowBlocked || !strings.Contains(strings.Join(d.Blocked, " "), "research workers") {
		t.Fatalf("after research off = %+v", d)
	}
	on, err := s.SetWorkspaceResearch(ctx, w.ID, off.CurrentRevisionID, true)
	if err != nil {
		t.Fatal(err)
	}
	if d := definitionState(t, s, w.ID, "deep-research"); d.Status != WorkflowBlocked {
		t.Fatalf("turning research on again restored the old approval: %+v", d)
	}
	if a := activate(t, s, p, 1); a.WorkspaceRevision != on.CurrentRevisionID {
		t.Fatalf("re-approval pinned %s", a.WorkspaceRevision)
	}
	if _, err := s.ArchiveWorkspace(ctx, w.ID); err != nil {
		t.Fatal(err)
	}
	d = definitionState(t, s, w.ID, "deep-research")
	if d.Status != WorkflowBlocked || !strings.Contains(strings.Join(d.Blocked, " "), "archived") {
		t.Fatalf("after archive = %+v", d)
	}
	if _, err := s.ActivateWorkflow(ctx, approval(p, 2)); !errors.Is(err, ErrWorkflowRefused) {
		t.Fatalf("approval in an archived Workspace: %v", err)
	}
}

// Deactivating is the owner's revocation of Standing Authority: it ends the
// current version, which stays in history; every record is immutable.
func TestWorkflowDeactivationAndHistoryAreImmutable(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	activate(t, s, p, 0)
	if _, err := s.DeactivateWorkflow(ctx, w.ID, "notes-digest", 2, "stale", "cli"); !errors.Is(err, ErrWorkflowChanged) {
		t.Fatalf("stale deactivation: %v", err)
	}
	if _, err := s.DeactivateWorkflow(ctx, w.ID, "notes-digest", 1, "no longer needed", "chat"); err == nil {
		t.Fatal("a chat surface deactivated a workflow")
	}
	off, err := s.DeactivateWorkflow(ctx, w.ID, "notes-digest", 1, "no longer needed", "cli")
	if err != nil || off.Deactivated == nil || off.Deactivated.Reason != "no longer needed" {
		t.Fatalf("deactivated = %+v, %v", off, err)
	}
	if _, err := s.DeactivateWorkflow(ctx, w.ID, "notes-digest", 1, "again", "cli"); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("second deactivation: %v", err)
	}
	if _, err := s.ActiveWorkflow(ctx, w.ID, "notes-digest"); !errors.Is(err, ErrWorkflowNotActive) {
		t.Fatalf("run admission after deactivation: %v", err)
	}
	if d := definitionState(t, s, w.ID, "notes-digest"); d.Status != WorkflowInactive {
		t.Fatalf("state = %+v", d)
	}
	if _, err := s.ActiveWorkflow(ctx, w.ID, "missing"); !errors.Is(err, ErrWorkflowNotFound) {
		t.Fatalf("unknown definition: %v", err)
	}
	// A later approval is a new version; the deactivated one stays in history.
	v2 := activate(t, s, p, 1)
	d := definitionState(t, s, w.ID, "notes-digest")
	if v2.Version != 2 || d.Status != WorkflowActive || len(d.Versions) != 2 || d.Versions[1].Deactivated == nil {
		t.Fatalf("history = %+v", d.Versions)
	}
	for _, table := range []string{"workflow_proposals", "workflow_activations", "workflow_deactivations"} {
		wantImmutable(t, s, table)
	}
}

// A proposal whose executable hash is the active version's changes only
// documentation: it is recorded against that version and needs no approval.
func TestWorkflowREADMEOnlyProposalIsRecordedWithoutApproval(t *testing.T) {
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	p := recordProposal(t, s, testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures))
	activate(t, s, p, 0)
	dir := copyTestDefinition(t, "notes-digest")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Notes digest\n\nClearer prose.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	docs := testProposal(t, w.ID, dir, "")
	docs.Commit = "89abcdef0123456789abcdef0123456789abcdef"
	recorded := recordProposal(t, s, docs)
	if recorded.ExecutableHash != p.ExecutableHash || recorded.DocumentsVersion != 1 {
		t.Fatalf("documentation proposal = %+v", recorded)
	}
	d := definitionState(t, s, w.ID, "notes-digest")
	if d.Status != WorkflowActive || d.Current.Version != 1 || len(d.Proposals) != 2 || d.Proposals[0].Pending || d.Proposals[0].DocumentsVersion != 1 {
		t.Fatalf("state = %+v", d.Proposals)
	}
}

// A proposal record must carry the report it was validated with, for an
// active Workspace.
func TestWorkflowProposalRecordChecksItsReport(t *testing.T) {
	ctx := context.Background()
	s := NewStore(newTestDB(t))
	w := notesWorkspace(t, s, "notebook:inbox", "notebook:digest")
	base := testProposal(t, w.ID, filepath.Join("..", "workflow", "testdata", "notes-digest"), notesFixtures)
	for name, mutate := range map[string]func(*WorkflowProposal){
		"hash":      func(p *WorkflowProposal) { p.ExecutableHash = "sha256:" + strings.Repeat("1", 64) },
		"authority": func(p *WorkflowProposal) { p.AuthorityHash = workflow.AuthorityHash(nil) },
		"name":      func(p *WorkflowProposal) { p.Name = "other" },
		"path":      func(p *WorkflowProposal) { p.Path = "elsewhere/notes-digest" },
		"commit":    func(p *WorkflowProposal) { p.Commit = "HEAD" },
		"workspace": func(p *WorkflowProposal) { p.WorkspaceID = "missing" },
		"proposer":  func(p *WorkflowProposal) { p.ProposedBy = "someone" },
		"simulation": func(p *WorkflowProposal) {
			sim := *p.Simulation
			sim.ExecutableHash = "sha256:" + strings.Repeat("2", 64)
			p.Simulation = &sim
		},
	} {
		p := base
		mutate(&p)
		if _, err := s.RecordWorkflowProposal(ctx, p); err == nil {
			t.Errorf("%s: recorded", name)
		}
	}
	if _, err := s.WorkflowProposal(ctx, "missing"); !errors.Is(err, ErrWorkflowProposalNotFound) {
		t.Fatalf("missing proposal: %v", err)
	}
}
