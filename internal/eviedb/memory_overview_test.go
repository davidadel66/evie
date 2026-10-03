package eviedb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// Memory overview (docs/memory-architecture.md, Stage 3): one bounded read of
// the standing set plus current profile and Workspace memories, isolated to
// the reader's scopes.

func overviewTexts(items []memory.MemoryOverviewItem) []string {
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Applies + " " + string(item.Kind) + " " + item.Memory
	}
	return texts
}

func overview(t *testing.T, store *Store, session memory.Session, scope memory.MemoryOverviewScope) memory.MemoryOverview {
	t.Helper()
	result, err := store.MemoryOverview(context.Background(), session.ScopeContext(), memory.MemoryOverviewQuery{Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMemoryOverviewListsStandingAndCurrentMemoriesInTheReadersScopes(t *testing.T) {
	f := newStandingFixture(t)
	ctx := context.Background()
	global := f.globalSession(t)
	_, kitchen := f.workspaceSession(t, "Kitchen")
	_, garden := f.workspaceSession(t, "Garden")

	f.remember(t, global, "answer_style", "answer style", "concise answers", memory.ClaimKindPreference, "")
	city := f.remember(t, global, "home_city", "home city", "Boston", memory.ClaimKindFact, "")
	f.remember(t, global, "legacy_note", "legacy note", "saved before kinds", "", "")
	f.remember(t, kitchen, "kitchen_brief", "kitchen brief", "remodel on a budget", memory.ClaimKindWorkspaceBrief, "")
	f.remember(t, kitchen, "contractor", "contractor", "Bob", memory.ClaimKindFact, "")
	f.remember(t, garden, "garden_secret", "garden plan", "hidden pond", memory.ClaimKindFact, "")

	all := overview(t, f.store, kitchen, memory.MemoryOverviewAll)
	if got := strings.Join(overviewTexts(all.Standing), "\n"); got != "Global preference owner — answer style: concise answers\nWorkspace workspace_brief owner — kitchen brief: remodel on a budget" {
		t.Fatalf("standing = %s", got)
	}
	if all.Profile == nil || all.Workspace == nil {
		t.Fatalf("overview sections = %+v", all)
	}
	profile := strings.Join(overviewTexts(all.Profile.Items), "\n")
	if !strings.Contains(profile, "Global fact owner — home city: Boston") || !strings.Contains(profile, "Global fact owner — legacy note: saved before kinds") ||
		strings.Contains(profile, "answer style") {
		t.Fatalf("profile = %s", profile)
	}
	if got := strings.Join(overviewTexts(all.Workspace.Items), "\n"); got != "Workspace fact owner — contractor: Bob" {
		t.Fatalf("workspace = %s", got)
	}
	encoded, _ := json.Marshal(all)
	if strings.Contains(string(encoded), "hidden pond") {
		t.Fatal("another Workspace's memory reached the overview")
	}
	for _, item := range append(append(all.Standing, all.Profile.Items...), all.Workspace.Items...) {
		if item.ClaimID == "" || item.ScopeKey == "" {
			t.Fatalf("overview item lacks its Claim ID or scope: %+v", item)
		}
	}

	profileOnly := overview(t, f.store, kitchen, memory.MemoryOverviewProfile)
	if profileOnly.Workspace != nil || len(profileOnly.Standing) != 1 || profileOnly.Standing[0].Applies != "Global" {
		t.Fatalf("profile overview = %+v", profileOnly)
	}
	workspaceOnly := overview(t, f.store, kitchen, memory.MemoryOverviewWorkspace)
	if workspaceOnly.Profile != nil || len(workspaceOnly.Standing) != 1 || workspaceOnly.Standing[0].Applies != "Workspace" {
		t.Fatalf("workspace overview = %+v", workspaceOnly)
	}
	unscoped := overview(t, f.store, global, memory.MemoryOverviewWorkspace)
	if unscoped.Workspace != nil || unscoped.WorkspaceNote == "" {
		t.Fatalf("Global conversation's workspace overview = %+v", unscoped)
	}

	// A retired memory leaves the overview.
	lease := f.lease(t, global)
	retireEvent := f.say(t, global, lease, "Forget my home city.")
	retire, err := f.store.PrepareMemoryLifecycle(ctx, global.ScopeContext(), memory.MemoryLifecycleRequest{IdempotencyKey: f.key(), SourceEventID: retireEvent.ID,
		Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: city.ClaimID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AppendEventWithLease(ctx, global.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: retireEvent.ID, Type: memory.EventApproval,
		ExecutionID: memory.ExecutionID(retire.OperationID), Payload: json.RawMessage(`{"decision":"approved","proposal_sha256":"` + retire.ProposalSHA256 + `","prepared_sha256":"` + retire.PreparedSHA256 + `"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ApplyMemoryLifecycle(ctx, lease, retire); err != nil {
		t.Fatal(err)
	}
	if after := overview(t, f.store, kitchen, memory.MemoryOverviewProfile); strings.Contains(strings.Join(overviewTexts(after.Profile.Items), "\n"), "Boston") {
		t.Fatalf("retired memory is still in the overview: %+v", after.Profile.Items)
	}

	// Pending proposals are counted, never shown, and only in the reader's scopes.
	f.release(t, global)
	f.propose(t, kitchen, "We decided on oak cabinets.", "cabinet_wood", "cabinet wood", "oak", memory.ClaimKindFact)
	if counted := overview(t, f.store, kitchen, memory.MemoryOverviewAll); counted.PendingReview != 1 {
		t.Fatalf("kitchen pending review = %d", counted.PendingReview)
	} else if encoded, _ := json.Marshal(counted); strings.Contains(string(encoded), "oak") {
		t.Fatal("the overview shows a pending proposal's content")
	}
	if elsewhere := overview(t, f.store, garden, memory.MemoryOverviewAll); elsewhere.PendingReview != 0 {
		t.Fatalf("another Workspace counts the kitchen's proposal: %d", elsewhere.PendingReview)
	}
}

func TestMemoryOverviewIsBounded(t *testing.T) {
	f := newStandingFixture(t)
	session := f.globalSession(t)
	long := strings.Repeat("word ", 200)
	for i := 0; i < memory.MemoryOverviewSectionLimit+3; i++ {
		f.remember(t, session, "note_"+string(rune('a'+i%26))+string(rune('a'+i/26)), "note", long+string(rune('a'+i%26)), memory.ClaimKindFact, "")
	}
	result := overview(t, f.store, session, memory.MemoryOverviewProfile)
	if len(result.Profile.Items) != memory.MemoryOverviewSectionLimit || result.Profile.Omitted != 3 {
		t.Fatalf("profile section = %d items, %d omitted", len(result.Profile.Items), result.Profile.Omitted)
	}
	for _, item := range result.Profile.Items {
		if len(item.Memory) > memory.MemoryOverviewTextLimit+len("…") || !item.Shortened {
			t.Fatalf("long memory was not shortened visibly: %d bytes", len(item.Memory))
		}
	}
}
