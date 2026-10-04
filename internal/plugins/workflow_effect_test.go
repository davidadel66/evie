package plugins

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/workflow"
)

// Workflow capability effects (Stage 14 sub-stage 6): a plugin contributes
// effects through the focused WorkflowEffectProvider role, separate from
// reads (ADR 0015). Each declares its contract, whether it applies one
// idempotency key at most once, how it derives Resource Conflict Keys, and
// whether it can reconcile an Effect Intent whose outcome is unknown; the
// Manager validates them with the manifest and resolves them through the
// lifecycle like reads.

type effectPlugin struct {
	workflowPlugin
	effects []WorkflowEffectCapability
}

func (p effectPlugin) WorkflowEffects() []WorkflowEffectCapability { return p.effects }

func ledgerPost(version string) WorkflowEffectCapability {
	return WorkflowEffectCapability{
		ID: "ledger.post", ContractVersion: version,
		Input:       workflow.ObjectType(workflow.Field{Name: "amount_cents", Type: workflow.IntegerType()}),
		Output:      workflow.ObjectType(workflow.Field{Name: "entry_id", Type: workflow.StringType()}),
		Connections: []string{"ledger-local"}, Resources: []string{"book"},
		Idempotent: true,
		Perform: func(_ context.Context, req WorkflowEffectRequest) (any, error) {
			return map[string]any{"entry_id": "entry-" + req.IdempotencyKey}, nil
		},
		Reconcile: func(_ context.Context, req WorkflowEffectRequest) (WorkflowEffectLookup, error) {
			return WorkflowEffectLookup{Outcome: WorkflowEffectHappened, Output: map[string]any{"entry_id": "entry-" + req.IdempotencyKey}, Evidence: "found"}, nil
		},
	}
}

func ledgerEffectPlugin(implementation string, reads []WorkflowCapability, effects ...WorkflowEffectCapability) effectPlugin {
	p := effectPlugin{workflowPlugin: ledgerPlugin(implementation, reads...), effects: effects}
	for _, e := range effects {
		p.manifest.Capabilities = append(p.manifest.Capabilities, CapabilityContract{ID: e.ID, Version: e.ContractVersion})
	}
	return p
}

// effectOnlyPlugin implements only the effect role.
type effectOnlyPlugin struct {
	manifest Manifest
	effects  []WorkflowEffectCapability
}

func (p effectOnlyPlugin) Manifest() Manifest                          { return p.manifest }
func (p effectOnlyPlugin) Start(context.Context) error                 { return nil }
func (p effectOnlyPlugin) Stop(context.Context) error                  { return nil }
func (p effectOnlyPlugin) WorkflowEffects() []WorkflowEffectCapability { return p.effects }

// An effect is listed in the compiled catalog as an effect with its
// idempotency, reconciliation and Resource Conflict Key rule, whatever its
// plugin's state; it is callable only while its plugin is ready, and
// changing any of those declarations changes its schema hash, so a pinned
// run notices.
func TestWorkflowEffectsRegisterThroughTheManagerLifecycle(t *testing.T) {
	ctx := context.Background()
	post := ledgerPost("1.0.0")
	only := effectOnlyPlugin{manifest: Manifest{ID: "ledger", ImplementationVersion: "1.0.0", KernelCompatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		Capabilities: []CapabilityContract{{ID: post.ID, Version: post.ContractVersion}}}, effects: []WorkflowEffectCapability{post}}
	manager, err := NewManager(tools.NewToolset(nil), only)
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := manager.WorkflowContract("ledger.post")
	if !ok || !contract.Effect || !contract.Idempotent || !contract.Reconciles || contract.ConflictKeys != "resource" ||
		strings.Join(contract.Connections, ",") != "ledger-local" || strings.Join(contract.Resources, ",") != "book" {
		t.Fatalf("compiled effect contract = %+v, %v", contract, ok)
	}
	provider, err := manager.ResolveWorkflowCapability(ctx, "ledger.post")
	if !errors.Is(err, ErrWorkflowCapabilityUnavailable) || provider.Perform != nil || provider.Reconcile != nil || provider.Read != nil ||
		provider.SchemaSHA256 != contract.SchemaSHA256() {
		t.Fatalf("disabled effect provider = %+v, %v", provider, err)
	}
	if err := manager.SetEnabled("ledger", true); err != nil {
		t.Fatal(err)
	}
	provider, err = manager.ResolveWorkflowCapability(ctx, "ledger.post")
	if err != nil || provider.Perform == nil || provider.Reconcile == nil || provider.Read != nil || provider.ConflictKeys != nil {
		t.Fatalf("ready effect provider = %+v, %v", provider, err)
	}
	out, err := provider.Perform(ctx, WorkflowEffectRequest{Connection: "ledger-local", Resource: "book", Input: map[string]any{"amount_cents": int64(5)}, IdempotencyKey: "k1"})
	if err != nil || out.(map[string]any)["entry_id"] != "entry-k1" {
		t.Fatalf("perform = %v, %v", out, err)
	}

	// Each declaration is part of the effect's schema.
	schemas := map[string]bool{contract.SchemaSHA256(): true}
	for name, edit := range map[string]func(*WorkflowEffectCapability){
		"not idempotent": func(e *WorkflowEffectCapability) { e.Idempotent = false },
		"no reconcile":   func(e *WorkflowEffectCapability) { e.Reconcile = nil },
		"per account": func(e *WorkflowEffectCapability) {
			e.ConflictRule = "account"
			e.ConflictKeys = func(WorkflowEffectRequest) ([]string, error) { return []string{"a"}, nil }
		},
	} {
		changed := ledgerPost("1.0.0")
		edit(&changed)
		m, err := NewManager(tools.NewToolset(nil), ledgerEffectPlugin("1.0.0", nil, changed))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c, _ := m.WorkflowContract("ledger.post")
		if schemas[c.SchemaSHA256()] {
			t.Fatalf("%s: the schema hash did not change", name)
		}
		schemas[c.SchemaSHA256()] = true
	}
	// A read's schema hash is unchanged by the effect declarations.
	read := ledgerRead("1.0.0")
	if got, want := (workflow.CapabilityContract{Input: read.Input, Output: read.Output}).SchemaSHA256(),
		(workflow.CapabilityContract{Input: read.Input, Output: read.Output, ConflictKeys: ""}).SchemaSHA256(); got != want {
		t.Fatal("read schema hash changed")
	}
}

// Every effect must be declared in the manifest at the same contract
// version, contributed once by exactly one role (never also a read or a
// tool), typed with object input and output, bound to connections and
// resources, performable, and either use the default Resource Conflict Key
// rule or name the rule its function derives.
func TestWorkflowEffectRegistrationIsValidated(t *testing.T) {
	post := ledgerPost("1.0.0")
	for name, tc := range map[string]struct {
		plugin Plugin
		want   string
	}{
		"undeclared": {plugin: func() Plugin {
			p := ledgerEffectPlugin("1.0.0", nil, post)
			p.manifest.Capabilities = nil
			return p
		}(), want: "undeclared Capability"},
		"contract mismatch": {plugin: func() Plugin {
			p := ledgerEffectPlugin("1.0.0", nil, post)
			p.manifest.Capabilities[0].Version = "2.0.0"
			return p
		}(), want: "manifest declares"},
		"twice": {plugin: func() Plugin {
			p := ledgerEffectPlugin("1.0.0", nil, post)
			p.effects = append(p.effects, post)
			return p
		}(), want: "more than once"},
		"read and effect at once": {plugin: func() Plugin {
			r := ledgerRead("1.0.0")
			r.ID = "ledger.post"
			p := ledgerEffectPlugin("1.0.0", nil, post)
			p.reads = []WorkflowCapability{r}
			return p
		}(), want: "both"},
		"no perform": {plugin: func() Plugin {
			e := post
			e.Perform = nil
			return ledgerEffectPlugin("1.0.0", nil, e)
		}(), want: "perform"},
		"list output": {plugin: func() Plugin {
			e := post
			e.Output = workflow.ArrayType(workflow.StringType())
			return ledgerEffectPlugin("1.0.0", nil, e)
		}(), want: "object"},
		"no resource": {plugin: func() Plugin {
			e := post
			e.Resources = nil
			return ledgerEffectPlugin("1.0.0", nil, e)
		}(), want: "resource"},
		"conflict keys without a rule": {plugin: func() Plugin {
			e := post
			e.ConflictKeys = func(WorkflowEffectRequest) ([]string, error) { return []string{"a"}, nil }
			return ledgerEffectPlugin("1.0.0", nil, e)
		}(), want: "conflict"},
		"a rule without conflict keys": {plugin: func() Plugin {
			e := post
			e.ConflictRule = "account"
			return ledgerEffectPlugin("1.0.0", nil, e)
		}(), want: "conflict"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewManager(tools.NewToolset(nil), tc.plugin); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("NewManager = %v, want an error containing %q", err, tc.want)
			}
		})
	}
	// A plugin may contribute reads and effects together.
	if _, err := NewManager(tools.NewToolset(nil), ledgerEffectPlugin("1.0.0", []WorkflowCapability{ledgerRead("1.0.0")}, post)); err != nil {
		t.Fatalf("reads and effects together: %v", err)
	}
}

// ResumableFrom evidence covers effects exactly like reads: the resolved
// provider carries it, and evidence that does not match the effect's
// loaded schema is refused.
func TestWorkflowEffectResumableEvidenceIsExact(t *testing.T) {
	post := ledgerPost("1.0.0")
	m, err := NewManager(tools.NewToolset(nil), ledgerEffectPlugin("1.0.0", nil, post))
	if err != nil {
		t.Fatal(err)
	}
	contract, _ := m.WorkflowContract("ledger.post")
	p := ledgerEffectPlugin("1.1.0", nil, post)
	p.manifest.ResumableFrom = []ImplementationCompatibility{{ImplementationVersion: "1.0.0", Capabilities: []CapabilityCompatibility{
		{ID: "ledger.post", ContractVersion: "1.0.0", SchemaSHA256: contract.SchemaSHA256()}}}}
	replacement, err := NewManager(tools.NewToolset(nil), p)
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := replacement.ResolveWorkflowCapability(context.Background(), "ledger.post")
	if evidence, ok := provider.ResumableFrom["1.0.0"]; !ok || evidence.SchemaSHA256 != contract.SchemaSHA256() {
		t.Fatalf("effect evidence = %+v, %v", provider.ResumableFrom, ok)
	}
	p.manifest.ResumableFrom[0].Capabilities[0].SchemaSHA256 = strings.Repeat("0", 64)
	if _, err := NewManager(tools.NewToolset(nil), p); err == nil || !strings.Contains(err.Error(), "schema") {
		t.Fatalf("mismatched effect evidence: %v", err)
	}
}
