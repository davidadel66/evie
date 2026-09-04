package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
)

func claimMutationContext(lease memory.TurnLease, run string) context.Context {
	return task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: string(lease.SessionID), RunID: run,
		LeaseHolderID: string(lease.HolderID), LeaseToken: uint64(lease.FencingToken),
		LeaseGeneration: uint64(lease.Generation),
	})
}

func createClaimLease(t *testing.T, store *Store, holder string, duration time.Duration) memory.TurnLease {
	t.Helper()
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(context.Background(), session.ID, memory.LeaseHolderID(holder), duration)
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func TestTaskClaimAcquireConfirmReleaseIsIdempotentAndDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	lease := createClaimLease(t, store, "claim-holder", 24*time.Hour)
	ctx := claimMutationContext(lease, "claim-tool")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "claim me", IdempotencyKey: "claim-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ctx, created.ID, task.ClaimInput{IdempotencyKey: "claim-root"}); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("create-to-claim idempotency conflict = %v", err)
	}
	claimInput := task.ClaimInput{IdempotencyKey: "claim-acquire"}
	claimed, err := store.ClaimGlobalTask(ctx, created.ID, claimInput)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID == "" || claimed.TaskID != created.ID || !claimed.ClaimedAt.Equal(now) {
		t.Fatalf("claim = %+v", claimed)
	}
	unchanged, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || unchanged.Revision != 1 || unchanged.Status != task.StatusOpen {
		t.Fatalf("claim changed Task = %+v, %v", unchanged, err)
	}
	confirmed, err := store.ClaimGlobalTask(
		claimMutationContext(lease, "confirm-tool"), created.ID,
		task.ClaimInput{IdempotencyKey: "claim-confirm"},
	)
	if err != nil || confirmed != claimed {
		t.Fatalf("confirmed claim = %+v, %v; want %+v", confirmed, err, claimed)
	}
	replayed, err := store.ClaimGlobalTask(
		claimMutationContext(lease, "retry-tool"), created.ID,
		task.ClaimInput{IdempotencyKey: "claim-confirm"},
	)
	if err != nil || replayed != claimed {
		t.Fatalf("replayed confirmation = %+v, %v", replayed, err)
	}
	releaseInput := task.ReleaseInput{IdempotencyKey: "claim-release"}
	released, err := store.ReleaseGlobalTaskClaim(ctx, created.ID, releaseInput)
	if err != nil || released.Claim != claimed || released.Reason != "explicit" || !released.ReleasedAt.Equal(now) {
		t.Fatalf("release = %+v, %v", released, err)
	}
	replayedRelease, err := store.ReleaseGlobalTaskClaim(
		claimMutationContext(lease, "release-retry"), created.ID, releaseInput,
	)
	if err != nil || !reflect.DeepEqual(replayedRelease, released) {
		t.Fatalf("release replay = %+v, %v; want %+v", replayedRelease, err, released)
	}
	if active, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || found {
		t.Fatalf("active claim after release = %+v, %v, found=%v", active, err, found)
	}
	metadata := "must conflict"
	if _, err := store.UpdateGlobalTask(ctx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Title: &metadata, IdempotencyKey: "claim-acquire",
	}); !errors.Is(err, task.ErrIdempotencyConflict) {
		t.Fatalf("claim-to-update idempotency conflict = %v", err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || len(events) != 4 || events[1].Operation != task.OperationClaim ||
		events[2].Operation != task.OperationClaim || events[3].Operation != task.OperationRelease ||
		events[1].ClaimID != claimed.ID || events[3].ClaimReason != "explicit" {
		t.Fatalf("claim events = %+v, %v", events, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	store.now = func() time.Time { return now }
	replayedRelease, err = store.ReleaseGlobalTaskClaim(ctx, created.ID, releaseInput)
	if err != nil || !reflect.DeepEqual(replayedRelease, released) {
		t.Fatalf("reopened release replay = %+v, %v; want %+v", replayedRelease, err, released)
	}
	newClaim, err := store.ClaimGlobalTask(ctx, created.ID, task.ClaimInput{IdempotencyKey: "claim-again"})
	if err != nil || newClaim.ID == claimed.ID {
		t.Fatalf("new claim = %+v, %v", newClaim, err)
	}
}

func TestTaskClaimOwnershipConflictAndManagementOverride(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	firstLease := createClaimLease(t, store, "first-holder", time.Hour)
	secondLease := createClaimLease(t, store, "second-holder", time.Hour)
	firstCtx := claimMutationContext(firstLease, "first-claim")
	secondCtx := claimMutationContext(secondLease, "second-claim")
	created, err := store.CreateGlobalTask(firstCtx, task.CreateInput{Title: "exclusive", IdempotencyKey: "exclusive-root"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimGlobalTask(firstCtx, created.ID, task.ClaimInput{IdempotencyKey: "first-key"})
	if err != nil {
		t.Fatal(err)
	}
	losingInput := task.ClaimInput{IdempotencyKey: "losing-key"}
	if _, err := store.ClaimGlobalTask(secondCtx, created.ID, losingInput); !errors.Is(err, task.ErrClaimHeld) ||
		strings.Contains(err.Error(), string(firstLease.SessionID)) || strings.Contains(err.Error(), string(firstLease.HolderID)) {
		t.Fatalf("competing claim error = %v", err)
	}
	if _, err := store.ReleaseGlobalTaskClaim(secondCtx, created.ID, task.ReleaseInput{
		IdempotencyKey: "wrong-release",
	}); !errors.Is(err, task.ErrClaimNotOwned) {
		t.Fatalf("wrong-owner release error = %v", err)
	}
	if _, err := store.OverrideReleaseGlobalTaskClaim(secondCtx, created.ID, ""); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("blank management override reason error = %v", err)
	}
	delegatedCtx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: string(secondLease.SessionID), ParentSessionID: "owner-session", RunID: "delegated-override",
	})
	if _, err := store.OverrideReleaseGlobalTaskClaim(delegatedCtx, created.ID, "steal work"); !errors.Is(err, task.ErrManagementOverrideDenied) {
		t.Fatalf("delegated management override error = %v", err)
	}
	managementCtx := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: string(secondLease.SessionID), RunID: "owner-override",
	})
	overridden, err := store.OverrideReleaseGlobalTaskClaim(managementCtx, created.ID, "owner recovery")
	if err != nil || overridden.Claim != claimed || overridden.Reason != "management_override" {
		t.Fatalf("management override = %+v, %v", overridden, err)
	}
	if _, err := store.ClaimGlobalTask(secondCtx, created.ID, losingInput); !errors.Is(err, task.ErrClaimHeld) {
		t.Fatalf("losing claim replay after release = %v", err)
	}
	acquired, err := store.ClaimGlobalTask(secondCtx, created.ID, task.ClaimInput{IdempotencyKey: "second-new-key"})
	if err != nil || acquired.ID == claimed.ID {
		t.Fatalf("second acquisition = %+v, %v", acquired, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || !events[len(events)-2].ManagementOverride || events[len(events)-2].ClaimReason != "management_override" ||
		events[len(events)-2].ManagementReason != "owner recovery" {
		t.Fatalf("override events = %+v, %v", events, err)
	}
}

func TestTaskClaimRejectsTerminalWork(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	lease := createClaimLease(t, store, "single-claim-holder", time.Hour)
	ctx := claimMutationContext(lease, "single-claim")
	first, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "first", IdempotencyKey: "single-first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "second", IdempotencyKey: "single-second"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ctx, first.ID, task.ClaimInput{IdempotencyKey: "single-first-claim"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReleaseGlobalTaskClaim(ctx, first.ID, task.ReleaseInput{IdempotencyKey: "single-release"}); err != nil {
		t.Fatal(err)
	}
	cancelled := task.StatusCancelled
	terminal, err := store.UpdateGlobalTask(ctx, second.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &cancelled, IdempotencyKey: "terminal-cancel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ctx, terminal.ID, task.ClaimInput{IdempotencyKey: "terminal-claim"}); !errors.Is(err, task.ErrInvalidTransition) {
		t.Fatalf("terminal Task claim error = %v", err)
	}
}

func TestTaskProgressAndResultRequireOwnedClaim(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	store.now = func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) }
	ownerLease := createClaimLease(t, store, "owner-holder", time.Hour)
	otherLease := createClaimLease(t, store, "other-holder", time.Hour)
	ownerCtx := claimMutationContext(ownerLease, "owner")
	otherCtx := claimMutationContext(otherLease, "other")
	created, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "progress", IdempotencyKey: "progress-root"})
	if err != nil {
		t.Fatal(err)
	}
	inProgress := task.StatusInProgress
	blocked := task.StatusBlocked
	completed := task.StatusCompleted
	resultWithoutClaim := "not owned"
	for _, input := range []task.UpdateInput{
		{ExpectedRevision: 1, Status: &inProgress, IdempotencyKey: "progress-without-claim"},
		{ExpectedRevision: 1, Status: &blocked, IdempotencyKey: "blocked-without-claim"},
		{ExpectedRevision: 1, Status: &completed, IdempotencyKey: "completed-without-claim"},
		{ExpectedRevision: 1, ResultSummary: &resultWithoutClaim, IdempotencyKey: "result-without-claim"},
	} {
		if _, err := store.UpdateGlobalTask(ownerCtx, created.ID, input); !errors.Is(err, task.ErrClaimRequired) {
			t.Fatalf("unclaimed execution update error = %v for %+v", err, input)
		}
	}
	if _, err := store.ClaimGlobalTask(ownerCtx, created.ID, task.ClaimInput{IdempotencyKey: "progress-claim"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateGlobalTask(otherCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &inProgress, IdempotencyKey: "other-progress",
	}); !errors.Is(err, task.ErrClaimNotOwned) {
		t.Fatalf("other execution progress error = %v", err)
	}
	result := "implementation and focused checks complete"
	progressed, err := store.UpdateGlobalTask(ownerCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &inProgress, ResultSummary: &result, IdempotencyKey: "owned-progress",
	})
	if err != nil || progressed.ResultSummary != result || progressed.Revision != 2 {
		t.Fatalf("owned progress = %+v, %v", progressed, err)
	}
	result = "verification complete"
	progressed, err = store.UpdateGlobalTask(ownerCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 2, Status: &inProgress, ResultSummary: &result, IdempotencyKey: "owned-result",
	})
	if err != nil || progressed.ResultSummary != result || progressed.Revision != 3 {
		t.Fatalf("owned result = %+v, %v", progressed, err)
	}
	completedTask, err := store.UpdateGlobalTask(ownerCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 3, Status: &completed, IdempotencyKey: "owned-completion",
	})
	if err != nil || completedTask.Status != task.StatusCompleted {
		t.Fatalf("owned completion = %+v, %v", completedTask, err)
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || found {
		t.Fatalf("completion retained claim: found=%v err=%v", found, err)
	}
	metadata := "corrected metadata"
	if _, err := store.UpdateGlobalTask(otherCtx, created.ID, task.UpdateInput{
		ExpectedRevision: 4, Title: &metadata, IdempotencyKey: "metadata-without-claim",
	}); err != nil {
		t.Fatalf("metadata correction required claim: %v", err)
	}
	cancelTarget, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{Title: "cancel claimed", IdempotencyKey: "cancel-claimed-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ownerCtx, cancelTarget.ID, task.ClaimInput{IdempotencyKey: "cancel-claimed-claim"}); err != nil {
		t.Fatal(err)
	}
	cancelled := task.StatusCancelled
	if _, err := store.UpdateGlobalTask(otherCtx, cancelTarget.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &cancelled, IdempotencyKey: "cancel-claimed-wrong-owner",
	}); !errors.Is(err, task.ErrClaimNotOwned) {
		t.Fatalf("wrong-owner cancellation error = %v", err)
	}
	if active, found, err := store.GetGlobalTaskClaim(context.Background(), cancelTarget.ID); err != nil || !found || active.TaskID != cancelTarget.ID {
		t.Fatalf("wrong-owner cancellation changed claim: active=%+v found=%v err=%v", active, found, err)
	}
	if _, err := store.UpdateGlobalTask(ownerCtx, cancelTarget.ID, task.UpdateInput{
		ExpectedRevision: 1, Status: &cancelled, IdempotencyKey: "cancel-claimed-update",
	}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), cancelTarget.ID); err != nil || found {
		t.Fatalf("cancellation retained claim: found=%v err=%v", found, err)
	}
	cancelEvents, err := store.ListTaskEvents(context.Background(), cancelTarget.ID)
	if err != nil || cancelEvents[len(cancelEvents)-1].Operation != task.OperationRelease ||
		cancelEvents[len(cancelEvents)-1].ClaimReason != "task_cancelled" {
		t.Fatalf("cancellation claim events = %+v, %v", cancelEvents, err)
	}
}

func TestTaskClaimRequiredUpdatesRejectLeaseLessAttributionAndAuditCoordination(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ctx := mutationContext("local", "lease-less", "run")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "protected", IdempotencyKey: "lease-less-create"})
	if err != nil {
		t.Fatal(err)
	}
	inProgress := task.StatusInProgress
	result := "forged result"
	for index, input := range []task.UpdateInput{
		{ExpectedRevision: 1, Status: &inProgress, IdempotencyKey: "lease-less-progress"},
		{ExpectedRevision: 1, ResultSummary: &result, IdempotencyKey: "lease-less-result"},
	} {
		if _, err := store.UpdateGlobalTask(ctx, created.ID, input); !errors.Is(err, task.ErrMissingAttribution) {
			t.Fatalf("lease-less update %d error = %v", index, err)
		}
	}
	if _, err := store.UpdateGlobalTask(
		mutationContext("local", "lease-less", "retry"), created.ID,
		task.UpdateInput{ExpectedRevision: 1, Status: &inProgress, IdempotencyKey: "lease-less-progress"},
	); !errors.Is(err, task.ErrMissingAttribution) {
		t.Fatalf("lease-less update replay error = %v", err)
	}
	got, err := store.GetGlobalTask(context.Background(), created.ID)
	if err != nil || !reflect.DeepEqual(got, created) {
		t.Fatalf("lease-less update changed Task: got=%+v err=%v", got, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 5 {
		t.Fatalf("lease-less coordination events = %+v", events)
	}
	for _, event := range []task.Event{events[2], events[4]} {
		if event.Operation != task.OperationUpdate || event.Outcome != task.MutationRejected ||
			event.DiagnosticCode != task.DiagnosticClaimRequired || event.ClaimID != "" {
			t.Fatalf("lease-less coordination audit = %+v", event)
		}
	}
}

func TestTaskActiveLifecycleReturnToOpenRequiresOwnedClaim(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	ownerLease := createClaimLease(t, store, "lifecycle-owner", time.Hour)
	otherLease := createClaimLease(t, store, "lifecycle-other", time.Hour)
	ownerCtx := claimMutationContext(ownerLease, "owner")
	otherCtx := claimMutationContext(otherLease, "other")
	managementCtx := mutationContext("local", string(ownerLease.SessionID), "management-seed")

	for _, activeStatus := range []task.Status{task.StatusInProgress, task.StatusBlocked} {
		t.Run(string(activeStatus), func(t *testing.T) {
			created, err := store.CreateGlobalTask(ownerCtx, task.CreateInput{
				Title: string(activeStatus), IdempotencyKey: task.IdempotencyKey("active-create-" + string(activeStatus)),
			})
			if err != nil {
				t.Fatal(err)
			}
			active, err := store.ManagementUpdateGlobalTask(managementCtx, created.ID, task.UpdateInput{
				ExpectedRevision: 1, Status: &activeStatus,
				IdempotencyKey: task.IdempotencyKey("active-seed-" + string(activeStatus)),
			}, "pre-claim lifecycle fixture")
			if err != nil {
				t.Fatal(err)
			}
			open := task.StatusOpen
			if _, err := store.UpdateGlobalTask(ownerCtx, created.ID, task.UpdateInput{
				ExpectedRevision: active.Revision, Status: &open,
				IdempotencyKey: task.IdempotencyKey("active-unclaimed-open-" + string(activeStatus)),
			}); !errors.Is(err, task.ErrClaimRequired) {
				t.Fatalf("unclaimed %s -> open error = %v", activeStatus, err)
			}
			claim, err := store.ClaimGlobalTask(ownerCtx, created.ID, task.ClaimInput{
				IdempotencyKey: task.IdempotencyKey("active-claim-" + string(activeStatus)),
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdateGlobalTask(otherCtx, created.ID, task.UpdateInput{
				ExpectedRevision: active.Revision, Status: &open,
				IdempotencyKey: task.IdempotencyKey("active-wrong-open-" + string(activeStatus)),
			}); !errors.Is(err, task.ErrClaimNotOwned) {
				t.Fatalf("wrong-owner %s -> open error = %v", activeStatus, err)
			}
			events, err := store.ListTaskEvents(context.Background(), created.ID)
			if err != nil {
				t.Fatal(err)
			}
			last := events[len(events)-1]
			if last.DiagnosticCode != task.DiagnosticClaimNotOwned || last.ClaimID != claim.ID {
				t.Fatalf("wrong-owner lifecycle audit = %+v", last)
			}
		})
	}
}

func TestManagementTaskUpdateRequiresPrimaryOwnerAndAuditsOverride(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	lease := createClaimLease(t, store, "management-owner", time.Hour)
	claimCtx := claimMutationContext(lease, "claim")
	created, err := store.CreateGlobalTask(claimCtx, task.CreateInput{Title: "managed", IdempotencyKey: "management-create"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimGlobalTask(claimCtx, created.ID, task.ClaimInput{IdempotencyKey: "management-claim"})
	if err != nil {
		t.Fatal(err)
	}
	completed := task.StatusCompleted
	input := task.UpdateInput{ExpectedRevision: 1, Status: &completed, IdempotencyKey: "management-complete"}
	delegated := task.WithMutationAttribution(context.Background(), task.MutationAttribution{
		ActorID: "local", SessionID: string(lease.SessionID), ParentSessionID: "owner", RunID: "delegated",
	})
	if _, err := store.ManagementUpdateGlobalTask(delegated, created.ID, input, "recover"); !errors.Is(err, task.ErrManagementOverrideDenied) {
		t.Fatalf("delegated management update error = %v", err)
	}
	owner := mutationContext("local", string(lease.SessionID), "management")
	if _, err := store.ManagementUpdateGlobalTask(owner, created.ID, input, ""); !errors.Is(err, task.ErrInvalidInput) {
		t.Fatalf("blank management reason error = %v", err)
	}
	updated, err := store.ManagementUpdateGlobalTask(owner, created.ID, input, "owner recovery")
	if err != nil || updated.Status != task.StatusCompleted {
		t.Fatalf("management update = %+v, %v", updated, err)
	}
	replayed, err := store.ManagementUpdateGlobalTask(owner, created.ID, input, "different authority reason")
	if err != nil || !reflect.DeepEqual(replayed, updated) {
		t.Fatalf("management update replay = %+v, %v; want %+v", replayed, err, updated)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var updateAudited, releaseAudited bool
	for _, event := range events {
		if event.Operation == task.OperationUpdate && event.ManagementOverride &&
			event.ManagementReason == "owner recovery" && event.ClaimID == claimed.ID {
			updateAudited = true
		}
		if event.Operation == task.OperationRelease && event.ManagementOverride &&
			event.ManagementReason == "owner recovery" && event.ClaimID == claimed.ID {
			releaseAudited = true
		}
	}
	if !updateAudited || !releaseAudited {
		t.Fatalf("management update events = %+v", events)
	}
}

func TestTaskClaimLeaseCleanupRecoveryAndLongHeartbeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	lease := createClaimLease(t, store, "long-holder", 2*time.Hour)
	ctx := claimMutationContext(lease, "long-claim")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "long work", IdempotencyKey: "long-root"})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimGlobalTask(ctx, created.ID, task.ClaimInput{IdempotencyKey: "long-key"})
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		now = now.Add(time.Hour)
		lease, err = store.HeartbeatTurnLease(
			context.Background(), lease.SessionID, lease.HolderID, lease.FencingToken, 2*time.Hour,
		)
		if err != nil {
			t.Fatal(err)
		}
	}
	if active, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || !found || active != claimed {
		t.Fatalf("long active claim = %+v found=%v err=%v", active, found, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = NewStore(db)
	store.now = func() time.Time { return now }
	if count, err := store.RecoverInactiveTaskClaims(context.Background()); err != nil || count != 0 {
		t.Fatalf("live claim recovery = %d, %v", count, err)
	}
	if active, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || !found || active != claimed {
		t.Fatalf("reopened live claim = %+v found=%v err=%v", active, found, err)
	}
	now = lease.ExpiresAt.Add(time.Nanosecond)
	if count, err := store.RecoverInactiveTaskClaims(context.Background()); err != nil || count != 1 {
		t.Fatalf("expired claim recovery = %d, %v", count, err)
	}
	if count, err := store.RecoverInactiveTaskClaims(context.Background()); err != nil || count != 0 {
		t.Fatalf("repeated recovery = %d, %v", count, err)
	}
	closedLease := createClaimLease(t, store, "closed-holder", time.Hour)
	closedCtx := claimMutationContext(closedLease, "closed-claim")
	closedTask, err := store.CreateGlobalTask(closedCtx, task.CreateInput{Title: "closed session", IdempotencyKey: "closed-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(closedCtx, closedTask.ID, task.ClaimInput{IdempotencyKey: "closed-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'closed' WHERE id = ?`, closedLease.SessionID); err != nil {
		t.Fatal(err)
	}
	if count, err := store.RecoverInactiveTaskClaims(context.Background()); err != nil || count != 1 {
		t.Fatalf("inactive-session recovery = %d, %v", count, err)
	}
	if count, err := store.RecoverInactiveTaskClaims(context.Background()); err != nil || count != 0 {
		t.Fatalf("repeated inactive-session recovery = %d, %v", count, err)
	}
}

func TestTurnLeaseReleaseAndReplacementCleanUpTaskClaimsAtomically(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	lease := createClaimLease(t, store, "cleanup-holder", time.Hour)
	ctx := claimMutationContext(lease, "cleanup-claim")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "cleanup", IdempotencyKey: "cleanup-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ctx, created.ID, task.ClaimInput{IdempotencyKey: "cleanup-key"}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(context.Background(), lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || found {
		t.Fatalf("claim after execution end: found=%v err=%v", found, err)
	}
	events, err := store.ListTaskEvents(context.Background(), created.ID)
	if err != nil || events[len(events)-1].ClaimReason != "execution_ended" {
		t.Fatalf("execution-end events = %+v, %v", events, err)
	}

	expiring := createClaimLease(t, store, "expired-holder", time.Minute)
	expiringCtx := claimMutationContext(expiring, "expired-claim")
	other, err := store.CreateGlobalTask(expiringCtx, task.CreateInput{Title: "replacement", IdempotencyKey: "replacement-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(expiringCtx, other.ID, task.ClaimInput{IdempotencyKey: "replacement-claim"}); err != nil {
		t.Fatal(err)
	}
	now = expiring.ExpiresAt.Add(time.Nanosecond)
	replacement, err := store.AcquireTurnLease(context.Background(), expiring.SessionID, "replacement-holder", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Generation <= expiring.Generation {
		t.Fatalf("replacement generation = %d; want > %d", replacement.Generation, expiring.Generation)
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), other.ID); err != nil || found {
		t.Fatalf("claim after lease replacement: found=%v err=%v", found, err)
	}
}

func TestTurnLeaseReleaseRollsBackWhenTaskClaimAuditFails(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	lease := createClaimLease(t, store, "rollback-holder", time.Hour)
	ctx := claimMutationContext(lease, "rollback-claim")
	created, err := store.CreateGlobalTask(ctx, task.CreateInput{Title: "rollback", IdempotencyKey: "rollback-root"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimGlobalTask(ctx, created.ID, task.ClaimInput{IdempotencyKey: "rollback-key"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TRIGGER reject_execution_end_claim_event
		BEFORE INSERT ON task_claim_events
		WHEN NEW.claim_reason = 'execution_ended'
		BEGIN SELECT RAISE(ABORT, 'forced claim audit failure'); END;
	`); err != nil {
		t.Fatal(err)
	}
	if err := store.ReleaseTurnLease(context.Background(), lease.SessionID, lease.HolderID, lease.FencingToken); err == nil {
		t.Fatal("ReleaseTurnLease succeeded despite forced claim audit failure")
	}
	if _, found, err := store.GetGlobalTaskClaim(context.Background(), created.ID); err != nil || !found {
		t.Fatalf("claim did not roll back: found=%v err=%v", found, err)
	}
	if _, err := store.HeartbeatTurnLease(context.Background(), lease.SessionID, lease.HolderID, lease.FencingToken, time.Hour); err != nil {
		t.Fatalf("turn lease did not roll back: %v", err)
	}
}

func TestTaskClaimsCompeteDeterministicallyAndDifferentTasksProgressConcurrently(t *testing.T) {
	storeA, storeB := openIndependentTaskStores(t)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	storeA.now = func() time.Time { return now }
	storeB.now = func() time.Time { return now }
	leaseA := createClaimLease(t, storeA, "holder-a", time.Hour)
	leaseB := createClaimLease(t, storeA, "holder-b", time.Hour)
	ctxA := claimMutationContext(leaseA, "run-a")
	ctxB := claimMutationContext(leaseB, "run-b")
	first, err := storeA.CreateGlobalTask(ctxA, task.CreateInput{Title: "first", IdempotencyKey: "concurrent-first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := storeA.CreateGlobalTask(ctxA, task.CreateInput{Title: "second", IdempotencyKey: "concurrent-second"})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		claim task.Claim
		err   error
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan result, 2)
	var wait sync.WaitGroup
	for _, operation := range []func() (task.Claim, error){
		func() (task.Claim, error) {
			return storeA.ClaimGlobalTask(ctxA, first.ID, task.ClaimInput{IdempotencyKey: "compete-a"})
		},
		func() (task.Claim, error) {
			return storeB.ClaimGlobalTask(ctxB, first.ID, task.ClaimInput{IdempotencyKey: "compete-b"})
		},
	} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			claim, err := operation()
			results <- result{claim: claim, err: err}
		}()
	}
	<-ready
	<-ready
	close(start)
	wait.Wait()
	close(results)
	winners, losers := 0, 0
	for result := range results {
		if result.err == nil {
			winners++
		} else if errors.Is(result.err, task.ErrClaimHeld) {
			losers++
		} else {
			t.Fatalf("competing claim error = %v", result.err)
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("claim race winners=%d losers=%d", winners, losers)
	}
	if _, err := storeA.OverrideReleaseGlobalTaskClaim(
		task.WithMutationAttribution(context.Background(), task.MutationAttribution{
			ActorID: "local", SessionID: string(leaseA.SessionID), RunID: "cleanup",
		}), first.ID, "test cleanup",
	); err != nil {
		t.Fatal(err)
	}
	ready = make(chan struct{}, 2)
	start = make(chan struct{})
	claimErrors := make(chan error, 2)
	for _, operation := range []func() error{
		func() error {
			_, err := storeA.ClaimGlobalTask(ctxA, first.ID, task.ClaimInput{IdempotencyKey: "different-a"})
			return err
		},
		func() error {
			_, err := storeB.ClaimGlobalTask(ctxB, second.ID, task.ClaimInput{IdempotencyKey: "different-b"})
			return err
		},
	} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			ready <- struct{}{}
			<-start
			claimErrors <- operation()
		}()
	}
	<-ready
	<-ready
	close(start)
	wait.Wait()
	close(claimErrors)
	for err := range claimErrors {
		if err != nil {
			t.Fatalf("different Task claim error = %v", err)
		}
	}
	progress := task.StatusInProgress
	progressErrors := make(chan error, 2)
	wait = sync.WaitGroup{}
	for _, operation := range []func() error{
		func() error {
			_, err := storeA.UpdateGlobalTask(ctxA, first.ID, task.UpdateInput{
				ExpectedRevision: 1, Status: &progress, IdempotencyKey: "different-progress-a",
			})
			return err
		},
		func() error {
			_, err := storeB.UpdateGlobalTask(ctxB, second.ID, task.UpdateInput{
				ExpectedRevision: 1, Status: &progress, IdempotencyKey: "different-progress-b",
			})
			return err
		},
	} {
		wait.Add(1)
		go func() {
			defer wait.Done()
			progressErrors <- operation()
		}()
	}
	wait.Wait()
	close(progressErrors)
	for err := range progressErrors {
		if err != nil {
			t.Fatalf("different Task progress error = %v", err)
		}
	}
}
