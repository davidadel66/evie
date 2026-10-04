package subagents_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/google/uuid"
)

// A Workspace allows explore by default exactly while it has a local
// folder, confined to that folder; a research allowance never implies it.
// The owner's explicit off revokes it durably; the folder moving or going
// away ends a child's authority (2026-10-03).
func TestWorkspaceExploreDefaultsToTheFolder(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	root, outside := exploreProject(t)
	ctx := context.Background()
	w, err := f.store.RegisterWorkspaceWithOptions(ctx, eviedb.WorkspaceRegistration{DisplayName: "Explored", AllowResearchDelegation: true})
	if err != nil {
		t.Fatal(err)
	}
	client := &recordingClient{next: func(n int, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		return response(report("workspace survey")), nil
	}}
	configure(t, f, client)
	// No folder and no explicit setting: explore is off, even with research
	// allowed and an absolute path declared.
	parent := f.workspaceParent(t, w)
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "w1", Objective: "look", Path: root}}); err == nil || !strings.Contains(err.Error(), "reviewed permission") {
		t.Fatalf("explore in a Workspace without a folder: %v", err)
	}
	// Attaching a folder turns the default on, for this chat too.
	if _, err := f.store.SetWorkspaceFolder(ctx, w.ID, 0, root); err != nil {
		t.Fatal(err)
	}
	r, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "w2", Objective: "look"}})
	if err != nil || r[0].Status != "succeeded" {
		t.Fatalf("Workspace explore by default: %+v %v", r, err)
	}
	a, err := f.store.InspectSubagent(ctx, parent, r[0].ExecutionID)
	if err != nil || a.Root != root {
		t.Fatalf("Workspace explore root = %q, %v", a.Root, err)
	}
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "w3", Objective: "look", Path: outside}}); err == nil {
		t.Fatal("default explore read outside the Workspace folder")
	}
	// The owner turns explore off: this chat and new ones are refused, and
	// returning to the default reaches only chats started afterwards.
	off, err := f.store.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, memory.WorkspaceExploreOff)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "w4", Objective: "look"}}); err == nil {
		t.Fatal("explicitly disabled explore admitted work")
	}
	if _, err := f.explore(t, ctx, f.workspaceParent(t, off), []delegation.Assignment{{Key: "w5", Objective: "look"}}); err == nil {
		t.Fatal("a chat started after explore was turned off can explore")
	}
	back, err := f.store.SetWorkspaceExplore(ctx, w.ID, off.CurrentRevisionID, memory.WorkspaceExploreDefault)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "w6", Objective: "look"}}); err == nil {
		t.Fatal("returning to the default restored a revoked chat")
	}
	fresh := f.workspaceParent(t, back)
	r, err = f.explore(t, ctx, fresh, []delegation.Assignment{{Key: "w7", Objective: "look"}})
	if err != nil || r[0].Status != "succeeded" {
		t.Fatalf("explore after returning to the default: %+v %v", r, err)
	}
	// The owner points the Workspace at another folder: the child cannot be
	// continued in a folder that is no longer the Workspace's.
	if _, err := f.store.SetWorkspaceFolder(ctx, w.ID, 1, outside); err != nil {
		t.Fatal(err)
	}
	if _, err := f.continueResearch(t, fresh, r[0].ExecutionID, "keep going"); err == nil || !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("continuation outside the current Workspace folder: %v", err)
	}
	// Detaching the folder turns the default off.
	if _, err := f.store.SetWorkspaceFolder(ctx, w.ID, 2, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.explore(t, ctx, fresh, []delegation.Assignment{{Key: "w8", Objective: "look", Path: outside}}); err == nil || !strings.Contains(err.Error(), "reviewed permission") {
		t.Fatalf("default explore after detaching the folder: %v", err)
	}
}

// A Workspace without a folder explores only once the owner turns explore
// on, for chats started afterwards, and each assignment then declares an
// absolute folder under the harness's root rules. Attaching a folder later
// confines explicit-on explore to it.
func TestWorkspaceExploreWithoutAFolderNeedsTheToggleAndAnAbsolutePath(t *testing.T) {
	f := setup(t, delegation.DefaultPolicy())
	root, outside := exploreProject(t)
	ctx := context.Background()
	w, err := f.store.RegisterWorkspace(ctx, "No folder")
	if err != nil {
		t.Fatal(err)
	}
	client := &recordingClient{next: func(n int, r openrouter.ChatRequest) (openrouter.ChatResponse, error) {
		return response(report("absolute survey")), nil
	}}
	configure(t, f, client)
	before := f.workspaceParent(t, w)
	on, err := f.store.SetWorkspaceExplore(ctx, w.ID, w.CurrentRevisionID, memory.WorkspaceExploreOn)
	if err != nil {
		t.Fatal(err)
	}
	if !on.Explore.Allowed || on.Explore.Setting != memory.WorkspaceExploreOn {
		t.Fatalf("explicit on = %+v", on.Explore)
	}
	if _, err := f.explore(t, ctx, before, []delegation.Assignment{{Key: "n1", Objective: "look", Path: root}}); err == nil {
		t.Fatal("a chat started before explore was turned on can explore")
	}
	parent := f.workspaceParent(t, on)
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "n2", Objective: "look"}}); err == nil || !strings.Contains(err.Error(), "declare an absolute path") {
		t.Fatalf("explore without a folder or path: %v", err)
	}
	for _, path := range []string{"docs", "~/.ssh", filepath.Join(root, "missing")} {
		if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "n-" + uuid.NewString(), Objective: "look", Path: path}}); err == nil {
			t.Fatalf("explore path %q was admitted", path)
		}
	}
	r, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "n3", Objective: "look", Path: root}})
	if err != nil || r[0].Status != "succeeded" {
		t.Fatalf("explicit-on explore with an absolute path: %+v %v", r, err)
	}
	a, err := f.store.InspectSubagent(ctx, parent, r[0].ExecutionID)
	if err != nil || a.Root != root {
		t.Fatalf("pinned root = %q, %v", a.Root, err)
	}
	// A folder attached later becomes the ceiling: the child pinned outside
	// it loses its authority, and new work stays inside it.
	if _, err := f.store.SetWorkspaceFolder(ctx, w.ID, 0, outside); err != nil {
		t.Fatal(err)
	}
	if _, err := f.continueResearch(t, parent, r[0].ExecutionID, "keep going"); err == nil || !errors.Is(err, delegation.ErrAuthority) {
		t.Fatalf("continuation outside the newly attached folder: %v", err)
	}
	if _, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "n4", Objective: "look", Path: root}}); err == nil {
		t.Fatal("explicit-on explore read outside the attached folder")
	}
	if r, err := f.explore(t, ctx, parent, []delegation.Assignment{{Key: "n5", Objective: "look"}}); err != nil || r[0].Status != "succeeded" {
		t.Fatalf("explicit-on explore in the attached folder: %+v %v", r, err)
	}
}
