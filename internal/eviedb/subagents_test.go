package eviedb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
)

func newSubagentAdmission(t *testing.T) (*eviedb.Store, *sql.DB, string, delegation.Parent, composition.Receipt) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := eviedb.NewStore(db)
	manager, err := plugins.NewManager(tools.NewToolset(nil), plugins.NewWeb())
	if err != nil {
		t.Fatal(err)
	}
	if err = manager.SetEnabled(plugins.WebPluginID, true); err != nil {
		t.Fatal(err)
	}
	research, err := manager.ResolvePreset(plugins.ResearchPresetID)
	if err != nil {
		t.Fatal(err)
	}
	parentReceipt := composition.Clone(research.Receipt)
	parentReceipt.Preset = composition.PresetIdentity{ID: "test-parent", Version: research.Receipt.Preset.Version}
	parentReceipt.Capabilities = append(parentReceipt.Capabilities, composition.Capability{ID: delegation.CapabilityID, ProviderID: "subagents", ContractVersion: "1.0.0", SchemaSHA256: parentReceipt.Capabilities[0].SchemaSHA256})
	parentReceipt.Providers = append(parentReceipt.Providers, composition.Provider{ID: "subagents", ImplementationVersion: "1.0.0"})
	parent, err := store.CreateGlobalSessionWithComposition(ctx, parentReceipt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, parent.ID, "orchestrator", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, parent.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "research"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{ID: "call", Name: delegation.ToolName, Arguments: `{"assignments":[{"idempotency_key":"one","objective":"find evidence"}]}`}})
	intent, err := store.AppendEventWithLease(ctx, parent.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: root.ID, Type: memory.EventToolIntent, ExecutionID: "invocation", Payload: payload})
	if err != nil {
		t.Fatal(err)
	}
	authority := delegation.Parent{Scope: parent.ScopeContext(), Lease: lease, SourceEventID: root.ID, IntentEventID: intent.ID}
	return store, db, path, authority, research.Receipt
}

func TestSubagentAdmissionReopensWithoutDuplicateExecution(t *testing.T) {
	ctx := context.Background()
	store, db, path, authority, receipt := newSubagentAdmission(t)
	var err error
	request := []delegation.Assignment{{Key: "one", Objective: "find evidence"}}
	first, err := store.AdmitSubagents(ctx, authority, request, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	second, err := store.AdmitSubagents(ctx, authority, request, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].ID != first[0].ID || second[0].Child.ID != first[0].Child.ID {
		t.Fatalf("duplicate attempt: %+v %+v", first, second)
	}
	request[0].Objective = "changed"
	if _, err = store.AdmitSubagents(ctx, authority, request, receipt, delegation.DefaultPolicy(), "dispatch"); err == nil {
		t.Fatal("changed key accepted")
	}
}

func TestRecoveryPreservesAcceptedChildEvidenceAndInterruptsUnfinishedWork(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint(accepted), func(t *testing.T) {
			ctx := context.Background()
			store, db, path, parent, receipt := newSubagentAdmission(t)
			attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
			if err != nil {
				t.Fatal(err)
			}
			a, started, err := store.StartSubagent(ctx, attempts[0].ID)
			if err != nil || !started {
				t.Fatalf("start: %t %v", started, err)
			}
			childLease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			root, err := store.AppendEventWithLease(ctx, a.Child.ID, childLease.HolderID, childLease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
			if err != nil {
				t.Fatal(err)
			}
			if accepted {
				if _, err = store.AppendEventWithLease(ctx, a.Child.ID, childLease.HolderID, childLease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "accepted findings https://example.com"}); err != nil {
					t.Fatal(err)
				}
			}
			if n, err := store.RecoverSubagents(ctx); err != nil || n != 0 {
				t.Fatalf("recovered foreign live work: %d %v", n, err)
			}
			if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
				t.Fatal(err)
			}
			if _, err = store.AppendEventWithLease(ctx, a.Child.ID, childLease.HolderID, childLease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "stale result"}); err == nil {
				t.Fatal("child accepted result without original parent ownership")
			}
			before, err := store.LoadEvents(ctx, a.Child.ID)
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			db, err = eviedb.OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store = eviedb.NewStore(db)
			if n, err := store.RecoverSubagents(ctx); err != nil || n != 1 {
				t.Fatalf("recovery=%d %v", n, err)
			}
			retained, err := store.InspectSubagent(ctx, parent, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "interrupted"
			if accepted {
				want = "succeeded"
			}
			if retained.State != want || retained.Result == nil {
				t.Fatalf("retained: %+v", retained)
			}
			after, err := store.LoadEvents(ctx, a.Child.ID)
			if err != nil || len(after) != len(before) {
				t.Fatalf("recovery fabricated history: %d -> %d %v", len(before), len(after), err)
			}
			// Plugin disable cannot destroy Kernel-owned outcome inspection.
			if _, err = store.SetPluginEnabled(ctx, "subagents", false); err != nil {
				t.Fatal(err)
			}
			if _, err = store.InspectSubagent(ctx, parent, a.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOngoingRecoveryRevisitsAttemptAfterEarlyRestart(t *testing.T) {
	ctx := context.Background()
	store, db, path, parent, receipt := newSubagentAdmission(t)
	a, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "original-process")
	if err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.StartSubagent(ctx, a[0].ID); err != nil || !started {
		t.Fatalf("start=%t %v", started, err)
	}
	db.Close()
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	supervisor, err := subagents.New(store, delegation.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if err = supervisor.Start(ctx); err != nil {
		t.Fatal(err)
	}
	retained, err := store.InspectSubagent(ctx, parent, a[0].ID)
	if err != nil || retained.State != "running" {
		t.Fatalf("startup did not preserve live owner: %+v %v", retained, err)
	}
	recoveryCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- supervisor.RunRecovery(recoveryCtx, nil) }()
	defer func() { cancel(); <-done }()
	if err = store.ReleaseTurnLease(ctx, parent.Scope.SessionID, parent.Lease.HolderID, parent.Lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			retained, err = store.InspectSubagent(ctx, parent, a[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			if retained.State == "interrupted" {
				return
			}
		case <-deadline:
			t.Fatal("early restart stranded execution after original ownership ended")
		}
	}
}
