package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

type agentMemoryKernel struct {
	plugins.SemanticMemoryKernel
	history              *fakeHistory
	preparedScope        memory.ScopeContext
	preparedRequest      memory.RememberLiteralRequest
	applyCalls           int
	appliedAfterApproval bool
}

func agentMemoryTool(t *testing.T, plugin *plugins.Memory, name string) tools.Tool {
	t.Helper()
	for _, capability := range plugin.ToolCapabilities() {
		if capability.Tool.Schema.Function.Name == name {
			return capability.Tool
		}
	}
	t.Fatalf("Memory tool %q is unavailable", name)
	return tools.Tool{}
}

func TestMemoryPluginCompositionReceiptSurvivesProcessReopen(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	storedSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	newManager := func() *plugins.Manager {
		manager, err := plugins.NewManager(
			tools.NewToolset(nil), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store),
			plugins.NewMemory(&agentMemoryKernel{}),
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []plugins.PluginID{
			plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID,
			plugins.MemoryPluginID,
		} {
			if err := manager.Enable(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		return manager
	}
	first, err := newManager().ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCompositionReceipt(ctx, storedSession.ID, first.Receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	reopened, err := eviedb.NewStore(db).GetCompositionReceipt(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := newManager().ResumeComposition(reopened)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened, first.Receipt) || !reflect.DeepEqual(resumed.Receipt, reopened) {
		t.Fatalf("Memory receipt changed across reopen: first=%+v reopened=%+v resumed=%+v", first.Receipt, reopened, resumed.Receipt)
	}
	memorySchemas := 0
	for _, schema := range resumed.Toolset.Schemas() {
		if strings.HasPrefix(schema.Function.Name, "memory_") {
			memorySchemas++
		}
	}
	if memorySchemas != len(plugins.NewMemory(&agentMemoryKernel{}).ToolCapabilities()) {
		t.Fatalf("resumed Memory schemas = %d", memorySchemas)
	}
}

func (k *agentMemoryKernel) PrepareRememberLiteral(_ context.Context, scope memory.ScopeContext, request memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error) {
	k.preparedScope, k.preparedRequest = scope, request
	return memory.RememberLiteralProposal{
		OperationID: "60000000-0000-4000-8000-000000000001", SessionID: scope.SessionID,
		Source:         memory.SemanticSource{EventID: request.SourceEventID},
		ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared",
	}, nil
}

func (k *agentMemoryKernel) ApplyRememberLiteral(_ context.Context, _ memory.TurnLease, proposal memory.RememberLiteralProposal) (memory.RememberLiteralResult, error) {
	k.applyCalls++
	for _, event := range k.history.events {
		if event.Type != memory.EventApproval || event.ParentID != proposal.Source.EventID || event.ExecutionID != memory.ExecutionID(proposal.OperationID) {
			continue
		}
		var payload memory.ApprovalPayload
		if json.Unmarshal(event.Payload, &payload) == nil && payload.Decision == memory.ApprovalApproved &&
			payload.ProposalSHA256 == proposal.ProposalSHA256 && payload.PreparedSHA256 == proposal.PreparedSHA256 {
			k.appliedAfterApproval = true
		}
	}
	return memory.RememberLiteralResult{OperationID: proposal.OperationID}, nil
}

func TestMemoryPluginMutationUsesExistingActionApprovalRecordBeforeApply(t *testing.T) {
	history := &fakeHistory{}
	kernel := &agentMemoryKernel{history: history}
	definition := agentMemoryTool(t, plugins.NewMemory(kernel), "memory_remember_literal")
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("memory-call", "memory_remember_literal", `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000011","predicate":"home_city","predicate_label":"home city","cardinality":"one","literal_kind":"text","literal_value":"Detroit","polarity":"affirmed"}`)),
		assistantStep("done", nil),
	}}
	scope := memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session", ProjectID: "project-1"}
	session := NewWithToolset(client, testContextProfile("test-model"), history, scope, newFakeTurnOwner(), tools.NewToolset([]tools.Tool{definition}))
	approvalSawProposal := false
	err := session.Send(context.Background(), "Remember that my home city is Detroit", &recorder{}, func(_ context.Context, name, args string, _ *tools.FileChangePreview) tools.Decision {
		approvalSawProposal = name == "memory_remember_literal" && strings.Contains(args, `"operation_id":"60000000-0000-4000-8000-000000000001"`)
		return tools.Approved
	})
	if err != nil {
		t.Fatal(err)
	}
	if !approvalSawProposal || kernel.applyCalls != 1 || !kernel.appliedAfterApproval {
		t.Fatalf("approval proposal/apply = %v/%d/%v", approvalSawProposal, kernel.applyCalls, kernel.appliedAfterApproval)
	}
	if kernel.preparedScope != scope || kernel.preparedRequest.SourceEventID != "event-1" || kernel.preparedRequest.SourceEventID == "" {
		t.Fatalf("prepared harness binding = scope %+v request %+v", kernel.preparedScope, kernel.preparedRequest)
	}
	var intent, toolApproval, semanticApproval, outcome memory.Event
	for _, event := range history.events {
		switch {
		case event.Type == memory.EventToolIntent:
			intent = event
		case event.Type == memory.EventApproval && event.ParentID == intent.ID:
			toolApproval = event
		case event.Type == memory.EventApproval && event.ExecutionID == "60000000-0000-4000-8000-000000000001":
			semanticApproval = event
		case event.Type == memory.EventToolSucceeded:
			outcome = event
		}
	}
	if intent.ID == "" || toolApproval.ID == "" || semanticApproval.ID == "" || outcome.ID == "" ||
		toolApproval.ExecutionID != intent.ExecutionID || outcome.ExecutionID != intent.ExecutionID || outcome.ParentID != toolApproval.ID {
		t.Fatalf("tool/semantic approval chains = intent %+v tool approval %+v semantic approval %+v outcome %+v", intent, toolApproval, semanticApproval, outcome)
	}
}
