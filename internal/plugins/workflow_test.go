package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow capability providers (Stage 14 sub-stage 5): a plugin
// contributes capability reads through the focused WorkflowCapabilityProvider
// role, the Manager validates them with the manifest, and the Workflow
// Runtime resolves them through the Manager's lifecycle.

type workflowPlugin struct {
	manifest  Manifest
	tools     []ToolCapability
	reads     []WorkflowCapability
	readiness *ConnectionReadiness
}

func (p workflowPlugin) Manifest() Manifest                         { return p.manifest }
func (p workflowPlugin) Start(context.Context) error                { return nil }
func (p workflowPlugin) Stop(context.Context) error                 { return nil }
func (p workflowPlugin) WorkflowCapabilities() []WorkflowCapability { return p.reads }

type workflowToolPlugin struct{ workflowPlugin }

func (p workflowToolPlugin) ToolCapabilities() []ToolCapability { return p.tools }

type readinessWorkflowPlugin struct{ workflowPlugin }

func (p readinessWorkflowPlugin) ConnectionReadiness() ConnectionReadiness { return *p.readiness }

func ledgerRead(version string) WorkflowCapability {
	return WorkflowCapability{
		ID: "ledger.month_total", ContractVersion: version,
		Input:       workflow.ObjectType(workflow.Field{Name: "month", Type: workflow.StringType()}),
		Output:      workflow.ObjectType(workflow.Field{Name: "total_cents", Type: workflow.IntegerType()}),
		Connections: []string{"ledger-local"}, Resources: []string{"book"},
		Read: func(_ context.Context, req WorkflowReadRequest) (any, error) {
			return map[string]any{"total_cents": int64(len(req.Input["month"].(string)))}, nil
		},
	}
}

func ledgerPlugin(implementation string, reads ...WorkflowCapability) workflowPlugin {
	m := Manifest{ID: "ledger", ImplementationVersion: implementation, KernelCompatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}}
	for _, r := range reads {
		m.Capabilities = append(m.Capabilities, CapabilityContract{ID: r.ID, Version: r.ContractVersion})
	}
	return workflowPlugin{manifest: m, reads: reads}
}

// The compiled catalog lists every workflow capability whatever its
// plugin's state; resolving a provider follows the lifecycle: a disabled or
// stopped plugin's capability is unavailable (with the provider it would
// be), an enabled one is callable, and an unknown or tool-only ID is not a
// workflow capability. Workflow capabilities never become session tools.
func TestWorkflowCapabilitiesRegisterThroughTheManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	tool := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "echo")
	manager, err := NewManager(tools.NewToolset(nil), ledgerPlugin("1.2.0", ledgerRead("1.0.0")), tool)
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := manager.WorkflowContract("ledger.month_total")
	if !ok || contract.ID != "ledger.month_total" || contract.Contract != "1.0.0" || contract.Effect ||
		strings.Join(contract.Connections, ",") != "ledger-local" || strings.Join(contract.Resources, ",") != "book" || contract.Output == nil {
		t.Fatalf("compiled contract = %+v, %v", contract, ok)
	}
	if _, ok := manager.WorkflowContract("fixture.echo"); ok {
		t.Fatal("a tool capability is listed as a workflow capability")
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, "ledger.month_total")
	if !errors.Is(err, ErrWorkflowCapabilityUnavailable) || !strings.Contains(err.Error(), "disabled") || provider.Read != nil ||
		provider.Plugin != "ledger" || provider.Version != "1.2.0" || provider.SchemaSHA256 != contract.SchemaSHA256() {
		t.Fatalf("disabled provider = %+v, %v", provider, err)
	}
	if err := manager.SetEnabled("ledger", true); err != nil {
		t.Fatal(err)
	}
	provider, err = manager.ResolveWorkflowCapability(ctx, "ledger.month_total")
	if err != nil || provider.Read == nil || provider.Contract.Contract != "1.0.0" {
		t.Fatalf("ready provider = %+v, %v", provider, err)
	}
	out, err := provider.Read(ctx, WorkflowReadRequest{Connection: "ledger-local", Resource: "book", Input: map[string]any{"month": "2026-09"}})
	if err != nil || out.(map[string]any)["total_cents"] != int64(7) {
		t.Fatalf("read = %v, %v", out, err)
	}
	if err := manager.Stop(ctx, "ledger"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveWorkflowCapability(ctx, "ledger.month_total"); !errors.Is(err, ErrWorkflowCapabilityUnavailable) || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("stopped provider: %v", err)
	}
	for _, id := range []string{"ledger.missing", "fixture.echo", "unknown.read"} {
		if _, err := manager.ResolveWorkflowCapability(ctx, id); !errors.Is(err, ErrWorkflowCapabilityUnknown) {
			t.Fatalf("%s: %v", id, err)
		}
	}
	if err := manager.Start(ctx, "ledger"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	toolset, err := manager.NewSessionToolset()
	if err != nil {
		t.Fatal(err)
	}
	if got := schemaNames(toolset); strings.Join(got, ",") != "fixture_echo" {
		t.Fatalf("session tools = %v; a workflow capability became a tool", got)
	}
}

// A plugin whose connection is not ready offers its workflow capabilities
// as unavailable, so a run waits for the connection instead of failing.
func TestWorkflowCapabilityWaitsForConnectionReadiness(t *testing.T) {
	ctx := context.Background()
	readiness := ConnectionReadiness{State: ConnectionNotReady}
	plugin := readinessWorkflowPlugin{ledgerPlugin("1.0.0", ledgerRead("1.0.0"))}
	plugin.readiness = &readiness
	manager, err := NewManager(tools.NewToolset(nil), plugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("ledger", true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveWorkflowCapability(ctx, "ledger.month_total"); !errors.Is(err, ErrWorkflowCapabilityUnavailable) || !strings.Contains(err.Error(), "connection") {
		t.Fatalf("not-ready connection: %v", err)
	}
}

// Every workflow capability must be declared in the manifest at the same
// contract version, contributed once by exactly one provider role, typed
// with object input and output, bound to valid connection and resource
// selectors, and callable.
func TestWorkflowCapabilityRegistrationIsValidated(t *testing.T) {
	read := ledgerRead("1.0.0")
	for name, tc := range map[string]struct {
		plugin Plugin
		want   string
	}{
		"undeclared": {plugin: func() Plugin {
			p := ledgerPlugin("1.0.0", read)
			p.manifest.Capabilities = nil
			return p
		}(), want: "undeclared Capability"},
		"contract mismatch": {plugin: func() Plugin {
			p := ledgerPlugin("1.0.0", read)
			p.manifest.Capabilities[0].Version = "1.1.0"
			return p
		}(), want: "manifest declares"},
		"declared without a contribution": {plugin: func() Plugin {
			p := ledgerPlugin("1.0.0", read)
			p.reads = nil
			return p
		}(), want: "without"},
		"twice": {plugin: func() Plugin {
			p := ledgerPlugin("1.0.0", read)
			p.reads = append(p.reads, read)
			return p
		}(), want: "more than once"},
		"no read": {plugin: func() Plugin {
			r := read
			r.Read = nil
			return ledgerPlugin("1.0.0", r)
		}(), want: "read"},
		"no output type": {plugin: func() Plugin {
			r := read
			r.Output = nil
			return ledgerPlugin("1.0.0", r)
		}(), want: "object"},
		"list input": {plugin: func() Plugin {
			r := read
			r.Input = workflow.ArrayType(workflow.StringType())
			return ledgerPlugin("1.0.0", r)
		}(), want: "object"},
		"no connection": {plugin: func() Plugin {
			r := read
			r.Connections = nil
			return ledgerPlugin("1.0.0", r)
		}(), want: "connection"},
		"bad resource": {plugin: func() Plugin {
			r := read
			r.Resources = []string{"two words"}
			return ledgerPlugin("1.0.0", r)
		}(), want: "resource"},
		"tool and workflow at once": {plugin: func() Plugin {
			p := workflowToolPlugin{ledgerPlugin("1.0.0", read)}
			p.tools = []ToolCapability{fakeToolPlugin("ledger", "ledger.month_total", "ledger_total", "x").capabilities[0]}
			return p
		}(), want: "both"},
		"no provider role": {plugin: barePlugin{manifest: ledgerPlugin("1.0.0", read).manifest}, want: "provider role"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewManager(tools.NewToolset(nil), tc.plugin); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewManager = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

// barePlugin declares capabilities but implements no provider role.
type barePlugin struct{ manifest Manifest }

func (p barePlugin) Manifest() Manifest          { return p.manifest }
func (p barePlugin) Start(context.Context) error { return nil }
func (p barePlugin) Stop(context.Context) error  { return nil }

// A replacement implementation declares, through the manifest's
// ResumableFrom, the earlier implementations whose pinned work it continues,
// with the exact contract and schema of each workflow capability; evidence
// that does not match what it loads is refused, and the resolved provider
// carries the evidence the Workflow Runtime checks before a substitution.
func TestWorkflowCapabilityResumableEvidenceIsExact(t *testing.T) {
	ctx := context.Background()
	read := ledgerRead("1.0.0")
	schema := workflow.CapabilityContract{Input: read.Input, Output: read.Output}.SchemaSHA256()
	p := ledgerPlugin("1.3.0", read)
	p.manifest.ResumableFrom = []ImplementationCompatibility{{ImplementationVersion: "1.2.0", Capabilities: []CapabilityCompatibility{
		{ID: "ledger.month_total", ContractVersion: "1.0.0", SchemaSHA256: schema}}}}
	manager, err := NewManager(tools.NewToolset(nil), p)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, "ledger.month_total")
	if !errors.Is(err, ErrWorkflowCapabilityUnavailable) {
		t.Fatalf("disabled: %v", err)
	}
	evidence, ok := provider.ResumableFrom["1.2.0"]
	if !ok || evidence.ContractVersion != "1.0.0" || evidence.SchemaSHA256 != schema || provider.SchemaSHA256 != schema {
		t.Fatalf("evidence = %+v, %v (provider %+v)", evidence, ok, provider)
	}
	if _, ok := provider.ResumableFrom["1.1.0"]; ok {
		t.Fatal("evidence for an undeclared version")
	}
	bad := ledgerPlugin("1.3.0", read)
	bad.manifest.ResumableFrom = []ImplementationCompatibility{{ImplementationVersion: "1.2.0", Capabilities: []CapabilityCompatibility{
		{ID: "ledger.month_total", ContractVersion: "1.0.0", SchemaSHA256: strings.Repeat("0", 64)}}}}
	if _, err := NewManager(tools.NewToolset(nil), bad); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("mismatched schema evidence: %v", err)
	}
}
