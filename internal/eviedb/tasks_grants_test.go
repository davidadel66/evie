package eviedb

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
)

var allTodoCapabilities = []task.Capability{
	task.CapabilityList, task.CapabilityAdd, task.CapabilityGet, task.CapabilityUpdate,
	task.CapabilityDecompose, task.CapabilityClaim, task.CapabilityRelease,
}

func todoCapabilityReceipt(t *testing.T, capabilities ...task.Capability) composition.Receipt {
	t.Helper()
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	receipt := composition.Receipt{
		FormatVersion: composition.FormatVersion,
		Preset:        composition.PresetIdentity{ID: "delegated", Version: "sha256:" + hash},
		EvieVersion:   "1.0.0",
		Providers:     []composition.Provider{{ID: "todo", ImplementationVersion: "1.0.0"}},
	}
	for _, capability := range capabilities {
		receipt.Capabilities = append(receipt.Capabilities, composition.Capability{
			ID: string(capability), ProviderID: "todo", ContractVersion: "1.0.0", SchemaSHA256: hash,
		})
	}
	return receipt
}

func delegatedTaskContext(session memory.Session, run string, capability task.Capability) context.Context {
	_ = capability // The Store maps each operation to its required pinned Capability.
	return scopedTaskContext(session, run)
}

func delegatedClaimContext(session memory.Session, lease memory.TurnLease, run string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: string(memory.LocalOwnerID), SessionID: string(session.ID), ParentSessionID: string(session.ParentSessionID),
		WorkspaceID: string(session.WorkspaceID), ProjectID: string(session.ProjectID), RunID: run,
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken),
		LeaseGeneration: uint64(lease.Generation),
	})
}

func TestTaskAccessGrantIsKernelIssuedDurableAndCapabilityIntersected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateGlobalTask(scopedTaskContext(owner, "seed"), task.CreateInput{
		Title: "delegated root", IdempotencyKey: "delegated-root",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID,
		todoCapabilityReceipt(t, task.CapabilityGet, task.CapabilityAdd),
	)
	if err != nil {
		t.Fatal(err)
	}
	issuer := scopedTaskContext(owner, "delegate-run")
	grant, err := store.IssueTaskAccessGrant(issuer, task.GrantInput{
		GranteeSessionID: string(child.ID), RootID: root.ID, Level: task.AccessRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if grant.ID == "" || grant.GranteeSessionID != string(child.ID) || grant.RootID != root.ID ||
		grant.Level != task.AccessRead || grant.IssuerActorID != string(memory.LocalOwnerID) ||
		grant.IssuerSessionID != string(owner.ID) || grant.IssuerRunID != "delegate-run" || grant.IssuedAt.IsZero() {
		t.Fatalf("grant = %+v", grant)
	}
	if grant.EndedAt != nil || grant.EndReason != "" {
		t.Fatalf("new grant unexpectedly ended: %+v", grant)
	}

	got, err := store.GetGlobalTask(delegatedTaskContext(child, "read", task.CapabilityGet), root.ID)
	if err != nil || got.ID != root.ID {
		t.Fatalf("granted read = %+v, %v", got, err)
	}
	if _, err := store.CreateGlobalTask(delegatedTaskContext(child, "write", task.CapabilityAdd), task.CreateInput{
		Title: "forbidden", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "read-write",
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("read grant write error = %v", err)
	}
	if _, err := store.ListGlobalTasks(delegatedTaskContext(child, "list", task.CapabilityList), task.ListFilter{}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("missing composed list Capability error = %v", err)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened, err := NewStore(db).GetTaskAccessGrant(context.Background(), grant.ID)
	if err != nil || !reflect.DeepEqual(reopened, grant) {
		t.Fatalf("reopened grant = %+v, %v; want %+v", reopened, err, grant)
	}
}

func TestTaskAccessGrantContainsDelegatedReadsToItsSubtree(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := scopedTaskContext(owner, "seed")
	root, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "root", IdempotencyKey: "root"})
	if err != nil {
		t.Fatal(err)
	}
	left, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{
		Title: "left", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "left",
	})
	if err != nil {
		t.Fatal(err)
	}
	root, err = store.GetGlobalTask(ownerCtx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	right, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{
		Title: "right", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "right",
	})
	if err != nil {
		t.Fatal(err)
	}
	adjacent, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "adjacent", IdempotencyKey: "adjacent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, task.CapabilityList, task.CapabilityGet),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.IssueTaskAccessGrant(scopedTaskContext(owner, "delegate"), task.GrantInput{
		GranteeSessionID: string(child.ID), RootID: left.ID, Level: task.AccessRead,
	}); err != nil {
		t.Fatal(err)
	}

	listCtx := delegatedTaskContext(child, "list", task.CapabilityList)
	listed, err := store.ListGlobalTasks(listCtx, task.ListFilter{IncludeHistory: true})
	wantLeft := left
	wantLeft.ParentID, wantLeft.RootID, wantLeft.SiblingOrder = "", left.ID, 0
	if err != nil || !reflect.DeepEqual(listed, []task.Task{wantLeft}) {
		t.Fatalf("delegated list = %#v, %v; want only left", listed, err)
	}
	getCtx := delegatedTaskContext(child, "get", task.CapabilityGet)
	for _, inaccessible := range []task.ID{root.ID, right.ID, adjacent.ID} {
		if _, err := store.GetGlobalTask(getCtx, inaccessible); !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("get inaccessible %s error = %v", inaccessible, err)
		}
		if _, err := store.GetGlobalTaskTree(getCtx, inaccessible, task.TreeQuery{}); !errors.Is(err, task.ErrNotFound) {
			t.Fatalf("tree inaccessible %s error = %v", inaccessible, err)
		}
	}

	root, err = store.GetGlobalTask(ownerCtx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	future, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{
		Title: "future", ParentID: left.ID, ExpectedParentRevision: left.Revision, IdempotencyKey: "future",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetGlobalTask(getCtx, future.ID)
	if err != nil || got.ID != future.ID {
		t.Fatalf("future descendant = %+v, %v", got, err)
	}
	if leaked, err := store.ListGlobalTasks(listCtx, task.ListFilter{RootID: root.ID, IncludeHistory: true}); err != nil || len(leaked) != 0 {
		t.Fatalf("real ancestor root filter leaked = %#v, %v", leaked, err)
	}
	if leaked, err := store.ListGlobalTasks(listCtx, task.ListFilter{ParentID: root.ID, IncludeHistory: true}); err != nil || len(leaked) != 0 {
		t.Fatalf("real parent filter leaked = %#v, %v", leaked, err)
	}
}

func TestTaskAccessGrantUsesPersistedDelegationAndLevels(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := scopedTaskContext(owner, "seed")
	root, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "root", IdempotencyKey: "levels-root"})
	if err != nil {
		t.Fatal(err)
	}

	noGrant, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	forged := noGrant
	forged.ParentSessionID = ""
	if _, err := store.GetGlobalTask(scopedTaskContext(forged, "forged"), root.ID); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("forged primary identity error = %v", err)
	}
	if _, err := store.CreateGlobalTask(scopedTaskContext(forged, "forged-root"), task.CreateInput{
		Title: "forged root", IdempotencyKey: "forged-root",
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("persisted delegated root error = %v", err)
	}
	if _, err := store.IssueTaskAccessGrant(scopedTaskContext(forged, "forged-grant"), task.GrantInput{
		GranteeSessionID: string(noGrant.ID), RootID: root.ID, Level: task.AccessManage,
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("delegated grant issuance error = %v", err)
	}

	contributor, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	contributeGrant, err := store.IssueTaskAccessGrant(ownerCtx, task.GrantInput{
		GranteeSessionID: string(contributor.ID), RootID: root.ID, Level: task.AccessContribute,
	})
	if err != nil {
		t.Fatal(err)
	}
	contributorCtx := scopedTaskContext(contributor, "contribute")
	child, err := store.CreateGlobalTask(contributorCtx, task.CreateInput{
		Title: "contributed", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "contributed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.RootID != root.ID || child.ParentID != root.ID {
		t.Fatalf("projected contributed child = %+v", child)
	}
	title := "refined"
	child, err = store.UpdateGlobalTask(contributorCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Title: &title, IdempotencyKey: "refine",
	})
	if err != nil {
		t.Fatalf("contributor metadata update: %v", err)
	}
	currentRoot, err := store.GetGlobalTask(ownerCtx, root.ID)
	if err != nil {
		t.Fatal(err)
	}
	decomposition, err := store.DecomposeGlobalTask(contributorCtx, root.ID, task.DecomposeInput{
		ExpectedRevision: currentRoot.Revision, IdempotencyKey: "contributed-decompose",
		Children: []task.ChildInput{{Title: "separate contribution"}},
	})
	if err != nil || len(decomposition.Children) != 1 || decomposition.Parent.RootID != root.ID {
		t.Fatalf("contributed decomposition = %+v, %v", decomposition, err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), contributor.ID, "contributor-holder", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	claimCtx := delegatedClaimContext(contributor, lease, "contributor-claim")
	if _, err := store.ClaimGlobalTask(claimCtx, child.ID, task.ClaimInput{IdempotencyKey: "contributor-claim"}); err != nil {
		t.Fatalf("contributor claim: %v", err)
	}
	inProgress := task.StatusInProgress
	child, err = store.UpdateGlobalTask(claimCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &inProgress, IdempotencyKey: "contributor-progress",
	})
	if err != nil || child.Status != task.StatusInProgress {
		t.Fatalf("contributor progress = %+v, %v", child, err)
	}
	blocked := task.StatusBlocked
	child, err = store.UpdateGlobalTask(claimCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &blocked, IdempotencyKey: "contributor-blocked",
	})
	if err != nil || child.Status != task.StatusBlocked {
		t.Fatalf("contributor blocked progress = %+v, %v", child, err)
	}
	open := task.StatusOpen
	child, err = store.UpdateGlobalTask(claimCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &open, IdempotencyKey: "contributor-resume",
	})
	if err != nil || child.Status != task.StatusOpen {
		t.Fatalf("contributor blocked-to-open progress = %+v, %v", child, err)
	}
	if _, err := store.ReleaseGlobalTaskClaim(claimCtx, child.ID, task.ReleaseInput{IdempotencyKey: "contributor-release"}); err != nil {
		t.Fatalf("contributor release: %v", err)
	}
	cancelled := task.StatusCancelled
	if _, err := store.UpdateGlobalTask(contributorCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &cancelled, IdempotencyKey: "cancel",
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("contributor cancel error = %v", err)
	}
	if _, err := store.ManagementUpdateGlobalTask(contributorCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &cancelled, IdempotencyKey: "override-cancel",
	}, "owner-style override"); !errors.Is(err, task.ErrManagementOverrideDenied) {
		t.Fatalf("contributor override error = %v", err)
	}
	events, err := store.ListTaskEvents(context.Background(), child.ID)
	if err != nil || len(events) == 0 || events[0].GrantID != contributeGrant.ID {
		t.Fatalf("contributed event grant linkage = %+v, %v", events, err)
	}

	manager, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	manageGrant, err := store.IssueTaskAccessGrant(ownerCtx, task.GrantInput{
		GranteeSessionID: string(manager.ID), RootID: child.ID, Level: task.AccessManage,
	})
	if err != nil {
		t.Fatal(err)
	}
	managerCtx := scopedTaskContext(manager, "manage")
	managed, err := store.ManagementUpdateGlobalTask(managerCtx, child.ID, task.UpdateInput{
		ExpectedRevision: child.Revision, Status: &cancelled, IdempotencyKey: "managed-cancel",
	}, "delegated recovery")
	if err != nil || managed.Status != task.StatusCancelled {
		t.Fatalf("managed cancel = %+v, %v", managed, err)
	}
	reopenedStatus := task.StatusOpen
	if _, err := store.UpdateGlobalTask(contributorCtx, child.ID, task.UpdateInput{
		ExpectedRevision: managed.Revision, Status: &reopenedStatus, IdempotencyKey: "contributor-reopen",
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("contributor reopen error = %v", err)
	}
	managed, err = store.ManagementUpdateGlobalTask(managerCtx, child.ID, task.UpdateInput{
		ExpectedRevision: managed.Revision, Status: &reopenedStatus, IdempotencyKey: "managed-reopen",
	}, "delegated recovery")
	if err != nil || managed.Status != task.StatusOpen {
		t.Fatalf("managed reopen = %+v, %v", managed, err)
	}
	events, err = store.ListTaskEvents(context.Background(), child.ID)
	if err != nil || events[len(events)-1].GrantID != manageGrant.ID || !events[len(events)-1].ManagementOverride {
		t.Fatalf("managed event grant linkage = %+v, %v", events, err)
	}
}

func TestTaskAccessGrantBoundsDelegatedLists(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := scopedTaskContext(owner, "seed")
	root, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "bounded root", IdempotencyKey: "bounded-root"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	now := formatTaskTime(store.now().UTC())
	for i := 1; i <= task.MaxTreeNodes; i++ {
		id := fmt.Sprintf("bounded-%04d", i)
		if _, err := tx.Exec(`
			INSERT INTO tasks (id, scope, title, status, revision, created_at, updated_at)
			VALUES (?, 'global', ?, 'open', 1, ?, ?)
		`, id, id, now, now); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if _, err := tx.Exec(`
			INSERT INTO task_hierarchy (task_id, parent_id, root_id, sibling_order)
			VALUES (?, ?, ?, ?)
		`, id, root.ID, root.ID, i); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, task.CapabilityList),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.IssueTaskAccessGrant(ownerCtx, task.GrantInput{
		GranteeSessionID: string(child.ID), RootID: root.ID, Level: task.AccessRead,
	}); err != nil {
		t.Fatal(err)
	}
	values, err := store.ListGlobalTasks(scopedTaskContext(child, "bounded-list"), task.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != task.MaxTreeNodes {
		t.Fatalf("delegated list length = %d; want server limit %d", len(values), task.MaxTreeNodes)
	}
}

func TestTaskAccessGrantFocusTerminationAndSessionLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	ownerCtx := scopedTaskContext(owner, "seed")
	root, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "private ancestor", IdempotencyKey: "focus-root"})
	if err != nil {
		t.Fatal(err)
	}
	granted, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{
		Title: "shared work", ParentID: root.ID, ExpectedParentRevision: root.Revision, IdempotencyKey: "focus-child",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	childCtx := scopedTaskContext(child, "focus")
	if err := store.SelectTaskFocus(childCtx, granted.ID); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("focus without grant error = %v", err)
	}
	grant, err := store.IssueTaskAccessGrant(ownerCtx, task.GrantInput{
		GranteeSessionID: string(child.ID), RootID: granted.ID, Level: task.AccessRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO session_task_focus (session_id, task_id, selected_at) VALUES (?, ?, ?)`,
		child.ID, root.ID, formatTaskTime(store.now().UTC())); err == nil {
		t.Fatal("database accepted delegated focus outside the granted subtree")
	}
	if working, err := store.workingTaskContext(context.Background(), child.ID); err != nil || working != "" {
		t.Fatalf("grant implicitly focused Task: %q, %v", working, err)
	}
	if err := store.SelectTaskFocus(childCtx, root.ID); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("focus ancestor error = %v", err)
	}
	if err := store.SelectTaskFocus(childCtx, granted.ID); err != nil {
		t.Fatal(err)
	}
	working, err := store.workingTaskContext(context.Background(), child.ID)
	if err != nil || !strings.Contains(working, "shared work") || strings.Contains(working, "private ancestor") {
		t.Fatalf("delegated working context = %q, %v", working, err)
	}
	ended, err := store.TerminateTaskAccessGrant(scopedTaskContext(owner, "terminate"), grant.ID, "delegation_complete")
	if err != nil || ended.EndedAt == nil || ended.EndReason != "delegation_complete" ||
		ended.EndedByActorID != string(memory.LocalOwnerID) || ended.EndedBySessionID != string(owner.ID) ||
		ended.EndedByRunID != "terminate" {
		t.Fatalf("ended grant = %+v, %v", ended, err)
	}
	working, err = store.workingTaskContext(context.Background(), child.ID)
	if err != nil || working != "" {
		t.Fatalf("working context after termination = %q, %v", working, err)
	}
	if _, err := store.GetGlobalTask(childCtx, granted.ID); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("read after termination = %v", err)
	}

	second, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetGlobalTask(scopedTaskContext(second, "new-run"), granted.ID); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("new delegated execution inherited grant: %v", err)
	}
	secondGrant, err := store.IssueTaskAccessGrant(ownerCtx, task.GrantInput{
		GranteeSessionID: string(second.ID), RootID: granted.ID, Level: task.AccessRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	closedAt := store.now().UTC().Add(time.Second)
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed', updated_at = ? WHERE id = ?`,
		formatTaskTime(closedAt), second.ID); err != nil {
		t.Fatal(err)
	}
	closedGrant, err := store.GetTaskAccessGrant(context.Background(), secondGrant.ID)
	if err != nil || closedGrant.EndedAt == nil || closedGrant.EndReason != "session_closed" ||
		closedGrant.EndedByActorID != "kernel" || closedGrant.EndedBySessionID != string(second.ID) ||
		closedGrant.EndedByRunID != "session-close" {
		t.Fatalf("session-closed grant = %+v, %v", closedGrant, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened, err := NewStore(db).GetTaskAccessGrant(context.Background(), grant.ID)
	if err != nil || reopened.EndedAt == nil || reopened.EndReason != "delegation_complete" {
		t.Fatalf("reopened ended grant = %+v, %v", reopened, err)
	}
}

func TestIssueFocusedTaskAccessGrantAtomicallyProjectsGrantedSubtree(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	owner, err := store.CreateGlobalSessionWithComposition(context.Background(), standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.CreateGlobalTask(scopedTaskContext(owner, "seed"), task.CreateInput{
		Title: "delegated focus", IdempotencyKey: "delegated-focus",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), owner.ID, todoCapabilityReceipt(t, task.CapabilityList, task.CapabilityGet),
	)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := store.IssueFocusedTaskAccessGrant(scopedTaskContext(owner, "delegate"), task.GrantInput{
		GranteeSessionID: string(child.ID), RootID: root.ID, Level: task.AccessRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetTaskAccessGrant(context.Background(), grant.ID)
	if err != nil || stored.RootID != root.ID || stored.GranteeSessionID != string(child.ID) {
		t.Fatalf("stored grant = %+v, %v", stored, err)
	}
	working, err := store.BindHistory(child.ID, "holder").WorkingContext(context.Background())
	if err != nil || !strings.Contains(working, string(root.ID)) || !strings.Contains(working, "delegated focus") {
		t.Fatalf("delegated working context = %q, %v", working, err)
	}
}

func TestTaskAccessGrantIssuanceRejectsUnrelatedAndCrossScopeTargets(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	workspaceA, err := store.RegisterWorkspace(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	workspaceB, err := store.RegisterWorkspace(context.Background(), "B")
	if err != nil {
		t.Fatal(err)
	}
	ownerA, err := store.CreateWorkspaceSessionWithComposition(
		context.Background(), workspaceA.ID, workspaceA.CurrentRevisionID, standardReceipt(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	ownerB, err := store.CreateWorkspaceSessionWithComposition(
		context.Background(), workspaceB.ID, workspaceB.CurrentRevisionID, standardReceipt(t),
	)
	if err != nil {
		t.Fatal(err)
	}
	rootB, err := store.CreateGlobalTask(scopedTaskContext(ownerB, "seed-b"), task.CreateInput{
		Title: "workspace B", IdempotencyKey: "workspace-b",
	})
	if err != nil {
		t.Fatal(err)
	}
	childA, err := store.CreateDelegatedSessionWithComposition(
		context.Background(), ownerA.ID, todoCapabilityReceipt(t, allTodoCapabilities...),
	)
	if err != nil {
		t.Fatal(err)
	}
	issuerA := scopedTaskContext(ownerA, "issue-a")
	if _, err := store.IssueTaskAccessGrant(issuerA, task.GrantInput{
		GranteeSessionID: string(childA.ID), RootID: rootB.ID, Level: task.AccessRead,
	}); !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("cross-Workspace grant error = %v", err)
	}
	if _, err := store.IssueTaskAccessGrant(issuerA, task.GrantInput{
		GranteeSessionID: string(ownerB.ID), RootID: rootB.ID, Level: task.AccessRead,
	}); !errors.Is(err, task.ErrAccessDenied) {
		t.Fatalf("unrelated grantee error = %v", err)
	}
	if _, err := store.IssueTaskAccessGrant(issuerA, task.GrantInput{
		GranteeSessionID: string(childA.ID), RootID: rootB.ID, Level: "owner",
	}); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("invalid access level error = %v", err)
	}
}
