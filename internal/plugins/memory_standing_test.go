package plugins

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// preStandingMemorySchemaHashes are the Memory Plugin 1.3.0 tool schemas,
// which 1.4.0 kept and extended with episode search before the standing tier.
// Receipts pin these hashes.
var preStandingMemorySchemaHashes = map[CapabilityID]string{
	MemorySearchCapabilityID:              "2ac14f7875c918560f58ccff51ce988e6321c8412539b29a693ab4a4e26c129a",
	MemorySearchConversationsCapabilityID: "d599cfc9750064972c9c8d02ca423e88988567d23742113586d466958f95c467",
	MemoryExpandConversationCapabilityID:  "cc6722f95341e3697473c1fed647ed07aeaf73d7abb3a958a388f3dfc61b6a49",
	MemoryListScopesCapabilityID:          "ba8ab85eb214b54857940341a23ae79b00e0802e095d639705dab185a7ef49ad",
	MemoryListObjectsCapabilityID:         "6c5527e4b885de59ebfbffa94e5e8a7a4855340b4b90a6f25e299db5aec10d9c",
	MemoryInspectObjectCapabilityID:       "9a5e17ad16852566382cd75d7515f04fce7d8ea88b3b9c743fc6f11ae8f83491",
	MemoryQueryClaimsCapabilityID:         "b6c6db831ed13cc0401fb10ce79da099729e08193558b21987ebd276f2c47de7",
	MemoryLookupAliasCapabilityID:         "883ab9277935ab4f3d6cf4b3471bd92e09a320bf4d1587016df8becc40f290f8",
	MemoryTraverseCapabilityID:            "ae9dfaf90758d0c360730433387661107fcb7f2d7165db4e69951f4c46b49853",
	MemoryRememberLiteralCapabilityID:     "08c6c383b748df240765273c6cae0bb92a5f0ee76f57e44186484132537c1c9c",
	MemoryRememberEntityCapabilityID:      "d993241b84d81f15064875741130f7e8c8c3c5f31543a97f21323348c1f464f8",
	MemoryCorrectClaimCapabilityID:        "1dabe903a16e3ec8b5c99648588223761bf828c34f21e2ffe4faacdb41d63435",
	MemoryCreateGraphLinkCapabilityID:     "0d1c05a252e501d3901a5c2772c5bda54ebc9548a8be5e22c3a15c0b299316bc",
	MemoryPromoteClaimCapabilityID:        "728702285374950556969bd513881403a9b3c7a3758ea7d1a506e7522d472629",
	MemoryRetireCapabilityID:              "0a4449b76c6a8365c83dad93ff563b3edb4da93b47c166afe448b9bfc24366c6",
	MemoryRestoreCapabilityID:             "b79f1d17d51324ee1b9f71817fb58c94f37aeb2ecae9420743a43137e5c7682d",
	MemoryRetractSourceCapabilityID:       "594b65e93d4cd44ae04c69493b4024de5bdb6cc214cc47e70781cd33dcbd74c4",
	MemoryRestoreSourceCapabilityID:       "092faf76ad89ae4d86234de72cb3b07fdd0b01d4f4fb4ee79c8328c55ba18119",
	MemoryReadToolResultCapabilityID:      "c3c7f540296c0998b81dafb04c10b191de873921fd6059d8e70b869cf2444a74",
}

// episodeSearchSchemaHash is the schema 1.4.0 added for memory_search_episodes.
const episodeSearchSchemaHash = "2f8aefd9a202f2680dc9507d116e22eff277a1bc2dad8a71b385816d6dad2356"

func TestPreStandingMemoryCapabilitiesKeepTheirExactSchemas(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	plugin := NewMemory(&stubSemanticKernel{})
	for version, extra := range map[string]map[CapabilityID]string{"1.3.0": {}, "1.4.0": {MemorySearchEpisodesCapabilityID: episodeSearchSchemaHash}} {
		legacy := plugin.ResumableToolCapabilities(version)
		if len(legacy) != len(preStandingMemorySchemaHashes)+len(extra) {
			t.Fatalf("%s capabilities = %d, want %d", version, len(legacy), len(preStandingMemorySchemaHashes)+len(extra))
		}
		for _, capability := range legacy {
			want, ok := preStandingMemorySchemaHashes[capability.ID]
			if !ok {
				want, ok = extra[capability.ID]
			}
			if !ok || schemaHash(capability.Tool.Schema) != want {
				t.Fatalf("%s schema of %s changed", version, capability.ID)
			}
		}
	}
	current := map[CapabilityID]openrouter.Tool{}
	for _, capability := range plugin.ToolCapabilities() {
		current[capability.ID] = capability.Tool.Schema
	}
	for _, id := range []CapabilityID{MemoryRememberLiteralCapabilityID, MemoryRememberEntityCapabilityID, MemoryCorrectClaimCapabilityID, MemoryPromoteClaimCapabilityID} {
		kind, ok := current[id].Function.Parameters.Properties["kind"]
		if !ok || !reflect.DeepEqual(kind.Enum, []string{"preference", "instruction", "profile_fact", "workspace_brief", "fact"}) {
			t.Fatalf("%s lacks the closed kind enum: %+v", id, kind)
		}
	}
	for _, id := range []CapabilityID{MemoryRememberLiteralCapabilityID, MemoryRememberEntityCapabilityID} {
		if !containsString(current[id].Function.Parameters.Required, "kind") {
			t.Fatalf("%s does not require a kind", id)
		}
	}
	if _, ok := current[MemorySetStandingCapabilityID]; !ok {
		t.Fatal("current Memory Plugin lacks memory_set_standing")
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// preStandingMemoryPlugin is the Memory Plugin as released before the
// standing tier.
type preStandingMemoryPlugin struct{ *Memory }

func (p preStandingMemoryPlugin) ToolCapabilities() []ToolCapability {
	return p.Memory.ResumableToolCapabilities("1.4.0")
}
func (p preStandingMemoryPlugin) Manifest() Manifest {
	manifest := p.Memory.Manifest()
	manifest.ImplementationVersion = "1.4.0"
	manifest.ResumableFrom = nil
	manifest.Capabilities = nil
	for _, capability := range p.ToolCapabilities() {
		manifest.Capabilities = append(manifest.Capabilities, CapabilityContract{ID: capability.ID, Version: capability.ContractVersion})
	}
	return manifest
}

func TestPreStandingStandardReceiptResumesWithItsExactMemoryTools(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	makeManager := func(memoryPlugin Plugin) *Manager {
		manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}), memoryPlugin)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID, MemoryPluginID} {
			if err := manager.SetEnabled(id, true); err != nil {
				t.Fatal(err)
			}
		}
		return manager
	}
	plugin := NewMemory(&stubSemanticKernel{})
	legacyPreset := preStandingStandardPreset()
	if got := canonicalPresetVersion(legacyPreset); got != preStandingStandardPresetVersion ||
		preStandingStandardPresetVersion != "sha256:983a77d1935c12e9165cba63b28bcfa4b035a9fe4e55ce37f4630bd17e26c654" {
		t.Fatalf("historical content hash=%s, want frozen %s", got, preStandingStandardPresetVersion)
	}
	legacy, err := makeManager(preStandingMemoryPlugin{plugin}).resolvePreset(legacyPreset)
	if err != nil {
		t.Fatal(err)
	}
	current := makeManager(plugin)
	resumed, err := current.ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) || !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatal("resuming added capabilities or rewrote the old receipt")
	}
	if countSchema(resumed.Toolset, "memory_set_standing") != 0 || countSchema(resumed.Toolset, "read_tool_result") != 1 ||
		countSchema(resumed.Toolset, "memory_search_episodes") != 1 {
		t.Fatalf("pinned conversation tools changed: %v", schemaNames(resumed.Toolset))
	}
	if len(resumed.CompatibilityResolutions) != 1 || resumed.CompatibilityResolutions[0].OriginalProvider.ImplementationVersion != "1.4.0" ||
		resumed.CompatibilityResolutions[0].ReplacementImplementationVersion != "1.5.0" {
		t.Fatalf("missing exact provider compatibility audit: %+v", resumed.CompatibilityResolutions)
	}
	fresh, err := current.ResolvePreset(StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Receipt.Preset.Version == legacy.Receipt.Preset.Version || countSchema(fresh.Toolset, "memory_set_standing") != 1 {
		t.Fatal("a new conversation does not pin the standing composition")
	}
}

type standingKernel struct {
	stubSemanticKernel
	literal  memory.RememberLiteralRequest
	standing memory.ClaimStandingRequest
	applied  []memory.ClaimClassification
}

func (k *standingKernel) PrepareRememberLiteral(_ context.Context, _ memory.ScopeContext, request memory.RememberLiteralRequest) (memory.RememberLiteralProposal, error) {
	k.literal = request
	return memory.RememberLiteralProposal{
		OperationID: "60000000-0000-4000-8000-000000000051", Source: memory.SemanticSource{EventID: request.SourceEventID},
		Request: request, Classification: &memory.ClaimClassification{Kind: request.Kind, Standing: true, BytesPerTokenMilli: request.StandingBytesPerTokenMilli},
		ProposalSHA256: "sha256:standing", PreparedSHA256: "sha256:prepared-standing",
		StandingAlternative: &memory.ProposalHashes{ProposalSHA256: "sha256:not-standing", PreparedSHA256: "sha256:prepared-not-standing"},
	}, nil
}

func (k *standingKernel) ApplyRememberLiteral(_ context.Context, _ memory.TurnLease, proposal memory.RememberLiteralProposal) (memory.RememberLiteralResult, error) {
	k.applied = append(k.applied, *proposal.Classification)
	if proposal.Classification.Standing != (proposal.ProposalSHA256 == "sha256:standing") {
		return memory.RememberLiteralResult{}, errorString("alternative hashes do not match its classification")
	}
	return memory.RememberLiteralResult{OperationID: proposal.OperationID}, nil
}

func (k *standingKernel) PrepareClaimStanding(_ context.Context, _ memory.ScopeContext, request memory.ClaimStandingRequest) (memory.ClaimStandingProposal, error) {
	k.standing = request
	return memory.ClaimStandingProposal{OperationID: "60000000-0000-4000-8000-000000000052", Evidence: memory.SemanticOperationEvidence{EventID: request.SourceEventID},
		Request: request, ProposalSHA256: "sha256:classify", PreparedSHA256: "sha256:prepared-classify"}, nil
}

func (k *standingKernel) ApplyClaimStanding(_ context.Context, _ memory.TurnLease, proposal memory.ClaimStandingProposal) (memory.ClaimStandingResult, error) {
	return memory.ClaimStandingResult{OperationID: proposal.OperationID, ClaimID: proposal.Request.ClaimID}, nil
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestStandingMemoryToolsProposeKindAndOfferTheAlwaysAppliesSwitch(t *testing.T) {
	kernel := &standingKernel{}
	plugin := NewMemory(kernel)
	scope := memory.ScopeContext{SessionID: "session-1", WorkspaceID: "workspace-1"}
	lease := memory.TurnLease{SessionID: scope.SessionID, HolderID: "holder", FencingToken: 2, Generation: 2}
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{Scope: scope, Lease: lease, SourceEventID: "owner-event", StandingBytesPerTokenMilli: 4400})
	set := tools.NewToolset([]tools.Tool{memoryTool(t, plugin, "memory_remember_literal"), memoryTool(t, plugin, "memory_set_standing")})
	remember := func(args string, decision tools.Decision) (openrouter.Message, bool, tools.ApprovalMetadata) {
		t.Helper()
		var observed tools.ApprovalMetadata
		message, isErr, err := set.ExecuteWithApprovalAuthorizedCompletion(ctx,
			openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_remember_literal", Arguments: args}},
			func(_ context.Context, _ string, approval string, _ *tools.FileChangePreview) tools.Decision {
				if !strings.Contains(approval, `"classification":{"kind":"preference","standing":true`) {
					t.Fatalf("approval card lacks the classification: %s", approval)
				}
				return decision
			},
			func(_ context.Context, _ tools.Decision, metadata tools.ApprovalMetadata) error {
				observed = metadata
				return nil
			}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return message, isErr, observed
	}
	base := `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000050","predicate":"answer_style","predicate_label":"answer style","cardinality":"one","literal_kind":"text","literal_value":"concise","polarity":"affirmed"`
	if message, isErr, _ := remember(base+`}`, tools.Approved); !isErr || !strings.Contains(message.Content, "kind") {
		t.Fatalf("remember without a kind = %+v", message)
	}
	if message, isErr, _ := remember(base+`,"kind":"habit"}`, tools.Approved); !isErr || !strings.Contains(message.Content, "kind") {
		t.Fatalf("remember with an unknown kind = %+v", message)
	}
	message, isErr, observed := remember(base+`,"kind":"preference"}`, tools.Approved)
	if isErr || kernel.literal.Kind != memory.ClaimKindPreference || kernel.literal.StandingBytesPerTokenMilli != 4400 ||
		observed.ProposalSHA256 != "sha256:standing" || len(kernel.applied) != 1 || !kernel.applied[0].Standing {
		t.Fatalf("approved standing memory = %+v request=%+v observed=%+v applied=%+v", message, kernel.literal, observed, kernel.applied)
	}
	message, isErr, observed = remember(base+`,"kind":"preference"}`, tools.ApprovedAlternative)
	if isErr || observed.ProposalSHA256 != "sha256:not-standing" || observed.PreparedSHA256 != "sha256:prepared-not-standing" ||
		!strings.Contains(observed.Arguments, `"standing":false`) || len(kernel.applied) != 2 || kernel.applied[1].Standing {
		t.Fatalf("switched approval = %+v observed=%+v applied=%+v", message, observed, kernel.applied)
	}

	_, isErr, err := set.ExecuteWithApprovalAuthorizedCompletion(ctx,
		openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: "memory_set_standing",
			Arguments: `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000053","claim_id":"claim-1","standing":true,"kind":"preference"}`}},
		func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved },
		func(context.Context, tools.Decision, tools.ApprovalMetadata) error { return nil }, nil, nil)
	if err != nil || isErr || kernel.standing != (memory.ClaimStandingRequest{IdempotencyKey: "idem:v1:60000000-0000-4000-8000-000000000053", SourceEventID: "owner-event",
		ClaimID: "claim-1", Kind: memory.ClaimKindPreference, Standing: true, StandingBytesPerTokenMilli: 4400}) {
		t.Fatalf("set standing = %v %v %+v", isErr, err, kernel.standing)
	}
	if !memoryTool(t, plugin, "memory_set_standing").NeedsApproval {
		t.Fatal("memory_set_standing does not require approval")
	}
}

func TestPreStandingMemoryToolsRejectAKind(t *testing.T) {
	kernel := &standingKernel{}
	var legacy tools.Tool
	for _, capability := range NewMemory(kernel).ResumableToolCapabilities("1.4.0") {
		if capability.ID == MemoryRememberLiteralCapabilityID {
			legacy = capability.Tool
		}
	}
	ctx := tools.WithInvocationContext(context.Background(), tools.InvocationContext{
		Scope: memory.ScopeContext{SessionID: "session-1"}, SourceEventID: "owner-event",
		Lease: memory.TurnLease{SessionID: "session-1", HolderID: "holder", FencingToken: 2, Generation: 2},
	})
	if _, err := legacy.Prepare(ctx, `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000054","predicate":"answer_style","predicate_label":"answer style","cardinality":"one","literal_kind":"text","literal_value":"concise","polarity":"affirmed","kind":"preference"}`); err == nil {
		t.Fatal("a conversation pinned to the earlier memory tools proposed a kind")
	}
	if _, err := legacy.Prepare(ctx, `{"idempotency_key":"idem:v1:60000000-0000-4000-8000-000000000055","predicate":"answer_style","predicate_label":"answer style","cardinality":"one","literal_kind":"text","literal_value":"concise","polarity":"affirmed"}`); err != nil || kernel.literal.Kind != "" || kernel.literal.StandingBytesPerTokenMilli != 0 {
		t.Fatalf("earlier memory tools = %v %+v", err, kernel.literal)
	}
}
