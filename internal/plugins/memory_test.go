package plugins

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type stubSemanticKernel struct{ SemanticMemoryKernel }

func memoryTool(t *testing.T, plugin *Memory, name string) tools.Tool {
	t.Helper()
	for _, capability := range plugin.ToolCapabilities() {
		if capability.Tool.Schema.Function.Name == name {
			return capability.Tool
		}
	}
	t.Fatalf("Memory tool %q is unavailable", name)
	return tools.Tool{}
}

func TestMemoryPluginLifecycleAndFocusedToolCapabilities(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	plugin := NewMemory(&stubSemanticKernel{})
	manifest := plugin.Manifest()
	if manifest.ID != MemoryPluginID || manifest.ImplementationVersion != "1.0.0" {
		t.Fatalf("manifest identity = %s@%s", manifest.ID, manifest.ImplementationVersion)
	}
	if err := plugin.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}

	capabilities := plugin.ToolCapabilities()
	gotIDs := make([]string, len(capabilities))
	gotSchemas := make([]string, len(capabilities))
	for i, capability := range capabilities {
		gotIDs[i] = string(capability.ID)
		gotSchemas[i] = capability.Tool.Schema.Function.Name
		if capability.ContractVersion != MemoryContractVersion {
			t.Fatalf("Capability %q contract = %q", capability.ID, capability.ContractVersion)
		}
		for name := range capability.Tool.Schema.Function.Parameters.Properties {
			if strings.Contains(name, "scope") || strings.Contains(name, "session") || strings.Contains(name, "lease") {
				t.Fatalf("Capability %q exposes harness authority argument %q", capability.ID, name)
			}
		}
	}
	wantIDs := []string{
		"memory.list_scopes", "memory.list_objects", "memory.inspect_object", "memory.query_claims",
		"memory.lookup_alias", "memory.traverse", "memory.remember_literal", "memory.remember_entity",
		"memory.correct_claim", "memory.create_graph_link", "memory.promote_claim", "memory.retire",
		"memory.restore", "memory.retract_source", "memory.restore_source",
	}
	wantSchemas := []string{
		"memory_list_scopes", "memory_list_objects", "memory_inspect_object", "memory_query_claims",
		"memory_lookup_alias", "memory_traverse", "memory_remember_literal", "memory_remember_entity",
		"memory_correct_claim", "memory_create_graph_link", "memory_promote_claim", "memory_retire",
		"memory_restore", "memory_retract_source", "memory_restore_source",
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) || !reflect.DeepEqual(gotSchemas, wantSchemas) {
		t.Fatalf("focused capabilities = %v / %v", gotIDs, gotSchemas)
	}
	declared := make([]string, len(manifest.Capabilities))
	for i, capability := range manifest.Capabilities {
		declared[i] = string(capability.ID)
	}
	sort.Strings(declared)
	sortedIDs := append([]string(nil), gotIDs...)
	sort.Strings(sortedIDs)
	if !reflect.DeepEqual(declared, sortedIDs) {
		t.Fatalf("manifest/tool Capability mismatch = %v / %v", declared, sortedIDs)
	}
}

func TestStandardPresetTreatsMemoryCapabilitiesAsOptional(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	preset := BuiltinStandardPreset()
	var optional []CapabilityID
	for _, requirement := range preset.OptionalCapabilities {
		if strings.HasPrefix(string(requirement.ID), "memory.") {
			optional = append(optional, requirement.ID)
		}
	}
	want := make([]CapabilityID, len(NewMemory(&stubSemanticKernel{}).ToolCapabilities()))
	for i, capability := range NewMemory(&stubSemanticKernel{}).ToolCapabilities() {
		want[i] = capability.ID
	}
	if !reflect.DeepEqual(optional, want) {
		t.Fatalf("optional Memory Capabilities = %v, want %v", optional, want)
	}

	manager, err := NewManager(tools.NewToolset(nil), NewWeb(), NewFinance(), NewYouTube(), NewMemory(&stubSemanticKernel{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	disabled, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatalf("disabled optional Memory Plugin invalidated standard: %v", err)
	}
	if len(disabled.Warnings) != len(want) || containsMemorySchema(disabled.Toolset) {
		t.Fatalf("disabled composition warnings/schemas = %d/%v", len(disabled.Warnings), disabled.Toolset.Schemas())
	}
	if err := manager.Enable(context.Background(), MemoryPluginID); err != nil {
		t.Fatal(err)
	}
	enabled, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	memoryReceipts := 0
	for _, capability := range enabled.Receipt.Capabilities {
		if strings.HasPrefix(capability.ID, "memory.") {
			memoryReceipts++
		}
	}
	if len(enabled.Warnings) != 0 || memoryReceipts != len(want) {
		t.Fatalf("enabled composition warnings/capabilities = %v/%v", enabled.Warnings, enabled.Receipt.Capabilities)
	}
}

func TestRemoteMemoryOptOutRemovesReadCapabilitiesFromComposition(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	manager, err := NewManager(tools.NewToolset(nil), NewWeb(), NewFinance(), NewYouTube(), NewMemory(&stubSemanticKernel{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, MemoryPluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	for _, schema := range resolved.Toolset.Schemas() {
		if schema.Function.Name == "memory_list_scopes" || schema.Function.Name == "memory_list_objects" ||
			schema.Function.Name == "memory_inspect_object" || schema.Function.Name == "memory_query_claims" ||
			schema.Function.Name == "memory_lookup_alias" || schema.Function.Name == "memory_traverse" {
			t.Fatalf("remote-memory opt-out exposed read schema %q", schema.Function.Name)
		}
	}
	for _, capability := range resolved.Receipt.Capabilities {
		switch CapabilityID(capability.ID) {
		case MemoryListScopesCapabilityID, MemoryListObjectsCapabilityID, MemoryInspectObjectCapabilityID,
			MemoryQueryClaimsCapabilityID, MemoryLookupAliasCapabilityID, MemoryTraverseCapabilityID:
			t.Fatalf("remote-memory opt-out pinned read Capability %q", capability.ID)
		}
	}
	if !containsSchema(resolved.Toolset, "memory_remember_literal") {
		t.Fatal("remote-memory opt-out removed non-egress mutation capabilities")
	}
	if len(resolved.Warnings) != 6 {
		t.Fatalf("remote-memory opt-out warnings = %v, want one per unavailable read Capability", resolved.Warnings)
	}
}

func containsSchema(toolset tools.Toolset, name string) bool {
	for _, schema := range toolset.Schemas() {
		if schema.Function.Name == name {
			return true
		}
	}
	return false
}

func containsMemorySchema(toolset tools.Toolset) bool {
	for _, schema := range toolset.Schemas() {
		if strings.HasPrefix(schema.Function.Name, "memory_") {
			return true
		}
	}
	return false
}

type behaviorSemanticKernel struct {
	stubSemanticKernel
	object          memory.SemanticObjectInspection
	list            memory.SemanticObjectPage
	readScope       memory.ScopeContext
	listScopeQuery  memory.SemanticScopeListQuery
	preparedScope   memory.ScopeContext
	preparedLiteral memory.RememberLiteralRequest
	preparedGraph   memory.CreateGraphLinkRequest
	appliedLease    memory.TurnLease
	applied         bool
	blockRead       bool
}

func (f *behaviorSemanticKernel) ListSemanticScopes(_ context.Context, scope memory.ScopeContext, query memory.SemanticScopeListQuery) (memory.SemanticScopePage, error) {
	f.readScope, f.listScopeQuery = scope, query
	return memory.SemanticScopePage{}, nil
}

func (f *behaviorSemanticKernel) InspectSemanticObjectAt(ctx context.Context, scope memory.ScopeContext, _ memory.SemanticObjectKind, _ memory.SemanticID, _ memory.ClaimQuery) (memory.SemanticObjectInspection, error) {
	f.readScope = scope
	if f.blockRead {
		<-ctx.Done()
		return memory.SemanticObjectInspection{}, ctx.Err()
	}
	return f.object, nil
}

func (f *behaviorSemanticKernel) ListSemanticObjects(_ context.Context, scope memory.ScopeContext, _ memory.SemanticObjectListQuery) (memory.SemanticObjectPage, error) {
	f.readScope = scope
	return f.list, nil
}

func (f *behaviorSemanticKernel) PrepareRememberLiteral(_ context.Context, scope memory.ScopeContext, request memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error) {
	f.preparedScope, f.preparedLiteral = scope, request
	return memory.RememberLiteralProposal{
		OperationID: "60000000-0000-4000-8000-000000000001", SessionID: scope.SessionID,
		Source:         memory.SemanticSource{EventID: request.SourceEventID},
		ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared",
	}, nil
}

func (f *behaviorSemanticKernel) PrepareCreateGraphLink(_ context.Context, scope memory.ScopeContext, request memory.CreateGraphLinkRequest) (memory.CreateGraphLinkProposal, error) {
	f.preparedScope, f.preparedGraph = scope, request
	return memory.CreateGraphLinkProposal{
		OperationID:    "60000000-0000-4000-8000-000000000002",
		Evidence:       memory.SemanticOperationEvidence{EventID: request.SourceEventID},
		ProposalSHA256: "sha256:graph-proposal", PreparedSHA256: "sha256:graph-prepared",
	}, nil
}

func (f *behaviorSemanticKernel) ApplyCreateGraphLink(_ context.Context, lease memory.TurnLease, proposal memory.CreateGraphLinkProposal) (memory.CreateGraphLinkResult, error) {
	f.applied, f.appliedLease = true, lease
	return memory.CreateGraphLinkResult{OperationID: proposal.OperationID}, nil
}

type mutationParityKernel struct {
	stubSemanticKernel
	prepared              any
	applied               bool
	appliedBeforeApproval bool
	approvalObserved      *bool
	lease                 memory.TurnLease
}

func (f *mutationParityKernel) recordApply(lease memory.TurnLease) {
	f.applied = true
	f.lease = lease
	f.appliedBeforeApproval = f.approvalObserved == nil || !*f.approvalObserved
}

func (f *mutationParityKernel) InspectSemanticObjectAt(context.Context, memory.ScopeContext, memory.SemanticObjectKind, memory.SemanticID, memory.ClaimQuery) (memory.SemanticObjectInspection, error) {
	return memory.SemanticObjectInspection{Scope: memory.SemanticScope{Key: "project:project-1"}}, nil
}

func (f *mutationParityKernel) PrepareRememberLiteral(_ context.Context, _ memory.ScopeContext, request memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error) {
	f.prepared = request
	return memory.RememberLiteralProposal{OperationID: "60000000-0000-4000-8000-000000000031", Source: memory.SemanticSource{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyRememberLiteral(_ context.Context, lease memory.TurnLease, proposal memory.RememberLiteralProposal) (memory.RememberLiteralResult, error) {
	f.recordApply(lease)
	return memory.RememberLiteralResult{OperationID: proposal.OperationID}, nil
}
func (f *mutationParityKernel) PrepareRememberEntity(_ context.Context, _ memory.ScopeContext, request memory.RememberEntityRequest) (memory.RememberEntityProposal, error) {
	f.prepared = request
	return memory.RememberEntityProposal{OperationID: "60000000-0000-4000-8000-000000000031", Source: memory.SemanticSource{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyRememberEntity(_ context.Context, lease memory.TurnLease, proposal memory.RememberEntityProposal) (memory.RememberEntityResult, error) {
	f.recordApply(lease)
	return memory.RememberEntityResult{OperationID: proposal.OperationID}, nil
}
func (f *mutationParityKernel) PrepareCorrectClaim(_ context.Context, _ memory.ScopeContext, request memory.CorrectClaimRequest) (memory.CorrectClaimProposal, error) {
	f.prepared = request
	return memory.CorrectClaimProposal{OperationID: "60000000-0000-4000-8000-000000000031", Source: memory.SemanticSource{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyCorrectClaim(_ context.Context, lease memory.TurnLease, proposal memory.CorrectClaimProposal) (memory.CorrectClaimResult, error) {
	f.recordApply(lease)
	return memory.CorrectClaimResult{OperationID: proposal.OperationID}, nil
}
func (f *mutationParityKernel) PrepareCreateGraphLink(_ context.Context, _ memory.ScopeContext, request memory.CreateGraphLinkRequest) (memory.CreateGraphLinkProposal, error) {
	f.prepared = request
	return memory.CreateGraphLinkProposal{OperationID: "60000000-0000-4000-8000-000000000031", Evidence: memory.SemanticOperationEvidence{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyCreateGraphLink(_ context.Context, lease memory.TurnLease, proposal memory.CreateGraphLinkProposal) (memory.CreateGraphLinkResult, error) {
	f.recordApply(lease)
	return memory.CreateGraphLinkResult{OperationID: proposal.OperationID}, nil
}
func (f *mutationParityKernel) PreparePromotion(_ context.Context, _ memory.ScopeContext, request memory.PromotionRequest) (memory.PromotionProposal, error) {
	f.prepared = request
	return memory.PromotionProposal{OperationID: "60000000-0000-4000-8000-000000000031", Evidence: memory.SemanticOperationEvidence{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyPromotion(_ context.Context, lease memory.TurnLease, proposal memory.PromotionProposal) (memory.PromotionResult, error) {
	f.recordApply(lease)
	return memory.PromotionResult{OperationID: proposal.OperationID}, nil
}
func (f *mutationParityKernel) PrepareMemoryLifecycle(_ context.Context, _ memory.ScopeContext, request memory.MemoryLifecycleRequest) (memory.MemoryLifecycleProposal, error) {
	f.prepared = request
	return memory.MemoryLifecycleProposal{OperationID: "60000000-0000-4000-8000-000000000031", Evidence: memory.SemanticOperationEvidence{EventID: request.SourceEventID}, ProposalSHA256: "sha256:proposal", PreparedSHA256: "sha256:prepared"}, nil
}
func (f *mutationParityKernel) ApplyMemoryLifecycle(_ context.Context, lease memory.TurnLease, proposal memory.MemoryLifecycleProposal) (memory.MemoryLifecycleResult, error) {
	f.recordApply(lease)
	return memory.MemoryLifecycleResult{OperationID: proposal.OperationID}, nil
}

func TestMemoryToolSchemaArgumentsMapExactlyIntoTypedKernelRequests(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	kernel := &behaviorSemanticKernel{}
	scope := memory.ScopeContext{SessionID: "session-1", ProjectID: "project-1"}
	lease := memory.TurnLease{SessionID: scope.SessionID, HolderID: "holder", FencingToken: 3, Generation: 3}
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: scope, Lease: lease, SourceEventID: "owner-event"})
	plugin := NewMemory(kernel)

	readSet := tools.NewToolset([]tools.Tool{memoryTool(t, plugin, "memory_list_scopes")})
	_, isErr, err := readSet.ExecuteWithApprovalAuthorizedCompletion(ctx, openrouter.ToolCall{ID: "read", Type: "function", Function: openrouter.FunctionCall{
		Name: "memory_list_scopes", Arguments: `{"page_size":7,"cursor":"next","valid_at":"2026-01-02T03:04:05Z","as_known_at":"2026-02-03T04:05:06Z"}`,
	}}, nil, nil, nil, nil)
	if err != nil || isErr {
		t.Fatalf("list scopes = (%v, %v)", isErr, err)
	}
	if kernel.listScopeQuery.PageSize != 7 || kernel.listScopeQuery.Cursor != "next" || kernel.listScopeQuery.ValidAt == nil || kernel.listScopeQuery.AsKnownAt == nil {
		t.Fatalf("list scope query = %+v", kernel.listScopeQuery)
	}

	graphSet := tools.NewToolset([]tools.Tool{memoryTool(t, plugin, "memory_create_graph_link")})
	_, isErr, err = graphSet.ExecuteWithApprovalAuthorizedCompletion(ctx, openrouter.ToolCall{ID: "graph", Type: "function", Function: openrouter.FunctionCall{
		Name: "memory_create_graph_link", Arguments: `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000021","relation":"derivation","source_kind":"claim","source_id":"claim-1","target_kind":"claim","target_id":"claim-2"}`,
	}}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }, func(context.Context, tools.Decision, tools.ApprovalMetadata) error { return nil }, nil, nil)
	if err != nil || isErr {
		t.Fatalf("create graph link = (%v, %v)", isErr, err)
	}
	if kernel.preparedGraph.Source.Kind != memory.SemanticObjectClaim || kernel.preparedGraph.Target.Kind != memory.SemanticObjectClaim {
		t.Fatalf("graph request = %+v", kernel.preparedGraph)
	}
}

func TestEveryMemoryMutationAdapterPreservesTypedRequestApprovalAndCancellation(t *testing.T) {
	const idempotencyKey = "idem:v1:60000000-0000-4000-8000-000000000041"
	sourceEventID := memory.EventID("owner-event")
	tests := []struct {
		name     string
		toolName string
		args     string
		want     any
	}{
		{name: "remember literal", toolName: "memory_remember_literal", args: `{"idempotency_key":"` + idempotencyKey + `","predicate":"home_city","predicate_label":"home city","cardinality":"one","literal_kind":"text","literal_value":"Detroit","polarity":"affirmed"}`, want: memory.RememberLiteralRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Predicate: "home_city", PredicateLabel: "home city", PredicateCardinality: memory.CardinalityOne, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"}, Polarity: memory.PolarityAffirmed}},
		{name: "remember entity", toolName: "memory_remember_entity", args: `{"idempotency_key":"` + idempotencyKey + `","predicate":"works_with","predicate_label":"works with","cardinality":"many","polarity":"affirmed","subject_entity_id":"entity-1","object_create":true,"object_name":"Acme","object_type":"company","object_alias":"ACME"}`, want: memory.RememberEntityRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Predicate: "works_with", PredicateLabel: "works with", PredicateCardinality: memory.CardinalityMany, Polarity: memory.PolarityAffirmed, Subject: memory.EntitySelector{EntityID: "entity-1"}, Object: memory.EntitySelector{Create: true, CanonicalName: "Acme", EntityType: "company", Alias: "ACME"}, UseSessionScope: false}},
		{name: "correct claim", toolName: "memory_correct_claim", args: `{"idempotency_key":"` + idempotencyKey + `","claim_id":"claim-1","subject_entity_id":"entity-1","predicate_id":"predicate-1","literal_kind":"text","literal_value":"Detroit","polarity":"affirmed","mode":"error"}`, want: memory.CorrectClaimRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, OldClaimID: "claim-1", Replacement: memory.ClaimProposition{SubjectEntityID: "entity-1", PredicateID: "predicate-1", Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"}}, Polarity: memory.PolarityAffirmed}, Mode: memory.CorrectionError}},
		{name: "create graph link", toolName: "memory_create_graph_link", args: `{"idempotency_key":"` + idempotencyKey + `","relation":"derivation","source_kind":"claim","source_id":"claim-1","target_kind":"claim","target_id":"claim-2"}`, want: memory.CreateGraphLinkRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Relation: memory.GraphRelationDerivation, Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: "claim-1"}, Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: "claim-2"}, UseSessionScope: false}},
		{name: "promote claim", toolName: "memory_promote_claim", args: `{"idempotency_key":"` + idempotencyKey + `","claim_id":"claim-1"}`, want: memory.PromotionRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, SourceClaimID: "claim-1", DestinationScopeKey: "global"}},
		{name: "retire", toolName: "memory_retire", args: `{"idempotency_key":"` + idempotencyKey + `","object_kind":"claim","object_id":"claim-1"}`, want: memory.MemoryLifecycleRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Action: memory.LifecycleRetire, ObjectKind: memory.SemanticObjectClaim, ObjectID: "claim-1", UseSessionScope: false}},
		{name: "restore", toolName: "memory_restore", args: `{"idempotency_key":"` + idempotencyKey + `","object_kind":"claim","object_id":"claim-1"}`, want: memory.MemoryLifecycleRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Action: memory.LifecycleRestore, ObjectKind: memory.SemanticObjectClaim, ObjectID: "claim-1", UseSessionScope: false}},
		{name: "retract source", toolName: "memory_retract_source", args: `{"idempotency_key":"` + idempotencyKey + `","source_link_id":"source-1"}`, want: memory.MemoryLifecycleRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Action: memory.LifecycleRetractSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: "source-1", UseSessionScope: false}},
		{name: "restore source", toolName: "memory_restore_source", args: `{"idempotency_key":"` + idempotencyKey + `","source_link_id":"source-1"}`, want: memory.MemoryLifecycleRequest{IdempotencyKey: idempotencyKey, SourceEventID: sourceEventID, Action: memory.LifecycleRestoreSource, ObjectKind: memory.SemanticObjectSourceLink, ObjectID: "source-1", UseSessionScope: false}},
	}
	scope := memory.ScopeContext{SessionID: "session-1", ProjectID: "project-1"}
	lease := memory.TurnLease{SessionID: scope.SessionID, HolderID: "holder", FencingToken: 5, Generation: 5}
	invocation := tools.InvocationContext{Scope: scope, Lease: lease, SourceEventID: sourceEventID}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			kernel := &mutationParityKernel{}
			tool := memoryTool(t, NewMemory(kernel), tc.toolName)
			set := tools.NewToolset([]tools.Tool{tool})
			call := openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: tc.toolName, Arguments: tc.args}}
			observed := false
			kernel.approvalObserved = &observed
			result, isErr, err := set.ExecuteWithApprovalAuthorizedCompletion(
				tools.WithInvocationContext(context.Background(), invocation), call,
				func(_ context.Context, _ string, args string, _ *tools.FileChangePreview) tools.Decision {
					if !strings.Contains(args, `"operation_id":"60000000-0000-4000-8000-000000000031"`) {
						t.Fatalf("approval arguments = %s", args)
					}
					return tools.Approved
				},
				func(_ context.Context, decision tools.Decision, metadata tools.ApprovalMetadata) error {
					if decision != tools.Approved || metadata.ParentEventID != sourceEventID || metadata.ExecutionID != "60000000-0000-4000-8000-000000000031" || metadata.ProposalSHA256 != "sha256:proposal" || metadata.PreparedSHA256 != "sha256:prepared" {
						t.Fatalf("approval metadata = %+v decision=%v", metadata, decision)
					}
					observed = true
					return nil
				}, nil, nil,
			)
			if err != nil || isErr || result.Content == "" || !reflect.DeepEqual(kernel.prepared, tc.want) || !kernel.applied || kernel.appliedBeforeApproval || kernel.lease != lease {
				t.Fatalf("adapter result=(%+v,%v,%v) prepared=%+v want=%+v applied=%v before=%v lease=%+v", result, isErr, err, kernel.prepared, tc.want, kernel.applied, kernel.appliedBeforeApproval, kernel.lease)
			}

			kernel.applied = false
			observed = false
			cancelCtx, cancel := context.WithCancel(tools.WithInvocationContext(context.Background(), invocation))
			_, _, err = set.ExecuteWithApprovalAuthorizedCompletion(cancelCtx, call,
				func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved },
				func(context.Context, tools.Decision, tools.ApprovalMetadata) error {
					observed = true
					cancel()
					return nil
				}, nil, nil)
			if !errors.Is(err, context.Canceled) || !observed || kernel.applied {
				t.Fatalf("post-approval cancellation = err %v observed/applied %v/%v", err, observed, kernel.applied)
			}
		})
	}
}

func (f *behaviorSemanticKernel) ApplyRememberLiteral(_ context.Context, lease memory.TurnLease, _ memory.RememberLiteralProposal) (memory.RememberLiteralResult, error) {
	f.applied, f.appliedLease = true, lease
	return memory.RememberLiteralResult{OperationID: "60000000-0000-4000-8000-000000000001"}, nil
}

func TestMemoryReadEgressRequiresOptInRedactsScopesScansSecretsAndBoundsOutput(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	scope := memory.ScopeContext{SessionID: "session-1", ProjectID: "project-1"}
	invocation := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: scope})
	kernel := &behaviorSemanticKernel{object: memory.SemanticObjectInspection{
		ObjectKind: memory.SemanticObjectSourceLink,
		Source:     &memory.SemanticSource{ScopeKey: "session:other", Evidence: "private sibling evidence", OperationID: "operation-1"},
		Metadata:   memory.ExactReadMetadata{AllowedScopes: []string{"global", "project:project-1", "session:session-1"}},
		Operations: []memory.SemanticOperationInspection{{OperationID: "operation-1", ProposalJSON: "private sibling evidence"}},
	}}
	call := openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_inspect_object", Arguments: `{"object_kind":"source_link","object_id":"id"}`}}
	plugin := NewMemory(kernel)
	definition := memoryTool(t, plugin, "memory_inspect_object")
	set := tools.NewToolset([]tools.Tool{definition})
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	result, isErr, err := set.ExecuteWithApprovalAuthorizedCompletion(invocation, call, nil, nil, nil, nil)
	if err != nil || !isErr || !strings.Contains(result.Content, "EVIE_REMOTE_MEMORY=on") {
		t.Fatalf("opt-in result = (%+v, %v, %v)", result, isErr, err)
	}
	if kernel.readScope != (memory.ScopeContext{}) {
		t.Fatalf("kernel read ran before remote-memory opt-in with scope %+v", kernel.readScope)
	}
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	result, isErr, err = set.ExecuteWithApprovalAuthorizedCompletion(invocation, call, nil, nil, nil, nil)
	if err != nil || isErr || !strings.Contains(result.Content, "[begin untrusted semantic memory") || strings.Contains(result.Content, "private sibling evidence") {
		t.Fatalf("redacted untrusted result = (%+v, %v, %v)", result, isErr, err)
	}
	if kernel.readScope != scope {
		t.Fatalf("read scope = %+v, want harness scope %+v", kernel.readScope, scope)
	}

	kernel.list = memory.SemanticObjectPage{Objects: []memory.SemanticObjectSummary{{ObjectID: memory.SemanticID("sk-12345678901234567890")}}}
	listSet := tools.NewToolset([]tools.Tool{memoryTool(t, plugin, "memory_list_objects")})
	listCall := openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_list_objects", Arguments: `{}`}}
	result, isErr, err = listSet.ExecuteWithApprovalAuthorizedCompletion(invocation, listCall, nil, nil, nil, nil)
	if err != nil || !isErr || !strings.Contains(result.Content, "secret scanning") || strings.Contains(result.Content, "sk-123") {
		t.Fatalf("secret scan result = (%+v, %v, %v)", result, isErr, err)
	}
	kernel.list = memory.SemanticObjectPage{Objects: []memory.SemanticObjectSummary{{
		ObjectID: "claim-password", Claim: &memory.SemanticClaim{
			Predicate: memory.SemanticPredicate{Token: "password"},
			Object:    memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "hunter123"}},
		},
	}}}
	result, isErr, err = listSet.ExecuteWithApprovalAuthorizedCompletion(invocation, listCall, nil, nil, nil, nil)
	if err != nil || !isErr || !strings.Contains(result.Content, "secret scanning") || strings.Contains(result.Content, "hunter123") {
		t.Fatalf("typed secret scan result = (%+v, %v, %v)", result, isErr, err)
	}
	kernel.list = memory.SemanticObjectPage{Objects: []memory.SemanticObjectSummary{{ObjectID: memory.SemanticID(strings.Repeat("x", memoryReadOutputLimit))}}}
	result, isErr, err = listSet.ExecuteWithApprovalAuthorizedCompletion(invocation, listCall, nil, nil, nil, nil)
	if err != nil || !isErr || !strings.Contains(result.Content, "byte model-facing limit") {
		t.Fatalf("bounded result = (%+v, %v, %v)", result, isErr, err)
	}
}

func TestMemoryMutationUsesHarnessScopeEvidenceFenceAndExactPreparedApproval(t *testing.T) {
	kernel := &behaviorSemanticKernel{}
	definition := memoryTool(t, NewMemory(kernel), "memory_remember_literal")
	set := tools.NewToolset([]tools.Tool{definition})
	scope := memory.ScopeContext{SessionID: "session-1", WorkspaceID: "workspace-1"}
	lease := memory.TurnLease{SessionID: scope.SessionID, HolderID: "holder", FencingToken: 8, Generation: 8}
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: scope, Lease: lease, SourceEventID: "owner-event"})
	call := openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_remember_literal", Arguments: `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000011","predicate":"home_city","predicate_label":"home city","cardinality":"one","literal_kind":"text","literal_value":"Detroit","polarity":"affirmed"}`}}
	observed := false
	result, isErr, err := set.ExecuteWithApprovalAuthorizedCompletion(ctx, call,
		func(_ context.Context, _ string, args string, _ *tools.FileChangePreview) tools.Decision {
			if !strings.Contains(args, `"operation_id":"60000000-0000-4000-8000-000000000001"`) || strings.Contains(args, "workspace_id") {
				t.Fatalf("approval did not receive exact prepared proposal: %s", args)
			}
			return tools.Approved
		},
		func(_ context.Context, decision tools.Decision, metadata tools.ApprovalMetadata) error {
			observed = true
			if decision != tools.Approved || metadata.ParentEventID != "owner-event" || metadata.ExecutionID != "60000000-0000-4000-8000-000000000001" || metadata.ProposalSHA256 != "sha256:proposal" || metadata.PreparedSHA256 != "sha256:prepared" {
				t.Fatalf("approval metadata = %+v decision=%v", metadata, decision)
			}
			if kernel.applied {
				t.Fatal("mutation applied before approval observation")
			}
			return nil
		}, nil, nil)
	if err != nil || isErr || !observed || !kernel.applied {
		t.Fatalf("mutation result = (%+v, %v, %v), observed/applied=%v/%v", result, isErr, err, observed, kernel.applied)
	}
	if kernel.preparedScope != scope || kernel.preparedLiteral.SourceEventID != "owner-event" || kernel.appliedLease != lease {
		t.Fatalf("harness binding = scope %+v request %+v lease %+v", kernel.preparedScope, kernel.preparedLiteral, kernel.appliedLease)
	}
}

func TestMemoryReadCancellationStopsKernelCall(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	kernel := &behaviorSemanticKernel{blockRead: true}
	set := tools.NewToolset([]tools.Tool{memoryTool(t, NewMemory(kernel), "memory_inspect_object")})
	ctx, cancel := context.WithCancel(tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: memory.ScopeContext{SessionID: "session-1"}}))
	cancel()
	_, _, err := set.ExecuteWithApprovalAuthorizedCompletion(ctx, openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_inspect_object", Arguments: `{"object_kind":"claim","object_id":"id"}`}}, nil, nil, nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

type changedMemorySchema struct{ *Memory }

func (p changedMemorySchema) ToolCapabilities() []ToolCapability {
	capabilities := p.Memory.ToolCapabilities()
	for i := range capabilities {
		if capabilities[i].ID == MemoryListScopesCapabilityID {
			capabilities[i].Tool.Schema.Function.Description += " incompatible change"
		}
	}
	return capabilities
}

func TestMemoryCompositionResumeFailsClosedWithoutRewritingReceipt(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	newManager := func(memoryPlugin Plugin) *Manager {
		manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), memoryPlugin)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, MemoryPluginID} {
			if err := manager.Enable(context.Background(), id); err != nil {
				t.Fatal(err)
			}
		}
		return manager
	}
	originalManager := newManager(NewMemory(&stubSemanticKernel{}))
	resolved, err := originalManager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	original := resolved.Receipt
	before := composition.Clone(original)

	replacement := newManager(changedMemorySchema{Memory: NewMemory(&stubSemanticKernel{})})
	if _, err := replacement.ResumeComposition(original); err == nil || !strings.Contains(err.Error(), "requires schema") {
		t.Fatalf("changed Memory schema resume error = %v", err)
	}
	if !reflect.DeepEqual(original, before) {
		t.Fatalf("failed resume rewrote original receipt: before=%+v after=%+v", before, original)
	}

	changedContract := original
	changedContract.Capabilities = append([]CapabilityReceipt(nil), original.Capabilities...)
	for i := range changedContract.Capabilities {
		if changedContract.Capabilities[i].ID == string(MemoryListScopesCapabilityID) {
			changedContract.Capabilities[i].ContractVersion = "2.0.0"
		}
	}
	if _, err := originalManager.ResumeComposition(changedContract); err == nil || !strings.Contains(err.Error(), "outside Agent Preset") {
		t.Fatalf("changed Memory contract resume error = %v", err)
	}
}

func TestDisablingMemoryPluginLeavesKernelSemanticInterfaceUsable(t *testing.T) {
	kernel := &behaviorSemanticKernel{object: memory.SemanticObjectInspection{ObjectID: "claim-1"}}
	plugin := NewMemory(kernel)
	manager, err := NewManager(tools.NewToolset(nil), NewWeb(), NewFinance(), NewYouTube(), plugin)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, MemoryPluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Disable(context.Background(), MemoryPluginID); err != nil {
		t.Fatal(err)
	}
	composition, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if containsMemorySchema(composition.Toolset) {
		t.Fatal("disabled plugin retained model-facing memory tools")
	}
	got, err := kernel.InspectSemanticObjectAt(context.Background(), memory.ScopeContext{SessionID: "session-1"}, memory.SemanticObjectClaim, "claim-1", memory.ClaimQuery{})
	if err != nil || got.ObjectID != "claim-1" {
		t.Fatalf("local Kernel semantic truth after plugin disable = (%+v, %v)", got, err)
	}
}

func TestFailedMemoryPluginStaysOutOfComposition(t *testing.T) {
	manager, err := NewManager(tools.NewToolset(nil), NewWeb(), NewFinance(), NewYouTube(), NewMemory(nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID} {
		if err := manager.Enable(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if err := manager.Enable(context.Background(), MemoryPluginID); err != nil {
		t.Fatal(err)
	}
	inspection, err := manager.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range inspection.Plugins {
		if status.ID == MemoryPluginID && status.State != StateFailed {
			t.Fatalf("Memory lifecycle = %s, want failed", status.State)
		}
	}
	composition, err := manager.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatalf("failed optional Memory Plugin invalidated standard preset: %v", err)
	}
	if containsMemorySchema(composition.Toolset) || len(composition.Warnings) != len(allMemoryCapabilityIDs()) {
		t.Fatalf("failed Memory Plugin composition = warnings %v schemas %v", composition.Warnings, composition.Toolset.Schemas())
	}
}
