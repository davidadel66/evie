package agent

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestStandardPresetReceiptReopensIntoExactScriptedAgentSchemas(t *testing.T) {
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
	firstManager := standardManager(t)
	first, err := firstManager.ResolvePreset("")
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
	store = eviedb.NewStore(db)
	receipt, err := store.GetCompositionReceipt(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(receipt, first.Receipt) {
		t.Fatalf("reopened receipt = %#v, want %#v", receipt, first.Receipt)
	}
	secondManager := standardManager(t)
	resumed, err := secondManager.ResumeComposition(receipt)
	if err != nil {
		t.Fatal(err)
	}

	wantSchemas := tools.KernelToolset().WithTools(tools.FinanceTools()).WithTools(tools.WebTools()).Schemas()
	if !reflect.DeepEqual(resumed.Toolset.Schemas(), wantSchemas) {
		t.Fatalf("resumed schemas = %#v, want exact standard schemas %#v", resumed.Toolset.Schemas(), wantSchemas)
	}
	wantProviders := []plugins.ProviderReceipt{
		{ID: "finance", ImplementationVersion: "1.0.0"},
		{ID: "web", ImplementationVersion: "1.0.0"},
	}
	if !reflect.DeepEqual(receipt.Providers, wantProviders) {
		t.Fatalf("receipt providers = %#v, want %#v", receipt.Providers, wantProviders)
	}
	wantCapabilities := []string{
		"finance.sync@1.0.0", "finance.rules@1.0.0", "finance.categorize@1.0.0",
		"web.fetch@1.0.0", "web.search@1.0.0",
	}
	gotCapabilities := make([]string, len(receipt.Capabilities))
	for i, capability := range receipt.Capabilities {
		gotCapabilities[i] = capability.ID + "@" + capability.ContractVersion
	}
	if !reflect.DeepEqual(gotCapabilities, wantCapabilities) {
		t.Fatalf("receipt capabilities = %v, want %v", gotCapabilities, wantCapabilities)
	}

	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("selected-call", "web_fetch", `{}`)),
		assistantStep("", nil, toolCall("absent-call", "absent_standard_tool", `{}`)),
		assistantStep("done", nil),
	}}
	recorded := &recorder{}
	session := NewWithToolset(
		client,
		testContextProfile("test-model"),
		store.BindHistory(storedSession.ID, "holder"),
		storedSession.ScopeContext(),
		store.BindTurnOwner(storedSession.ID, "holder"),
		resumed.Toolset,
	)
	if err := session.Send(ctx, "hello", recorded, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 3 {
		t.Fatalf("scripted requests = %d, want three", len(client.reqs))
	}
	for i, request := range client.reqs {
		if !reflect.DeepEqual(request.Tools, wantSchemas) {
			t.Fatalf("scripted request %d schemas = %#v, want %#v", i, request.Tools, wantSchemas)
		}
	}
	wantEvents := []string{
		"done:",
		`call:selected-call:web_fetch:{}`,
		"result:selected-call:false:deterministic web.fetch result",
		"done:",
		`call:absent-call:absent_standard_tool:{}`,
		"result:absent-call:true:Unknown Tool Call: absent_standard_tool",
		"done:done",
	}
	if !reflect.DeepEqual(recorded.events, wantEvents) {
		t.Fatalf("scripted public events = %#v, want %#v", recorded.events, wantEvents)
	}
	if len(client.reqs) != 3 || !reflect.DeepEqual(client.reqs[0].Tools, wantSchemas) {
		t.Fatalf("scripted request schemas = %#v, want %#v", client.reqs, wantSchemas)
	}
}

func standardManager(t *testing.T) *plugins.Manager {
	t.Helper()
	web := plugins.NewWeb()
	webCapabilities := web.ToolCapabilities()
	for i := range webCapabilities {
		capabilityID := webCapabilities[i].ID
		webCapabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "deterministic " + string(capabilityID) + " result", nil
		}
	}
	finance := plugins.NewFinance()
	financeCapabilities := finance.ToolCapabilities()
	for i := range financeCapabilities {
		capabilityID := financeCapabilities[i].ID
		financeCapabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "deterministic " + string(capabilityID) + " result", nil
		}
	}
	manager, err := plugins.NewManager(
		tools.KernelToolset(),
		deterministicToolPlugin{manifest: web.Manifest(), capabilities: webCapabilities},
		deterministicToolPlugin{manifest: finance.Manifest(), capabilities: financeCapabilities},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

type deterministicToolPlugin struct {
	manifest     plugins.Manifest
	capabilities []plugins.ToolCapability
}

func (p deterministicToolPlugin) Manifest() plugins.Manifest { return p.manifest }

func (deterministicToolPlugin) Start(context.Context) error { return nil }

func (deterministicToolPlugin) Stop(context.Context) error { return nil }

func (p deterministicToolPlugin) ToolCapabilities() []plugins.ToolCapability {
	return append([]plugins.ToolCapability(nil), p.capabilities...)
}
