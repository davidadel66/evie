package plugins

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	kernelcomposition "github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/tools"
)

type cancelingEnabledStateStore struct {
	*enabledStateMemoryStore
	cancelReads bool
	entered     chan struct{}
}

func (s *cancelingEnabledStateStore) PluginEnabled(
	ctx context.Context,
	id string,
) (bool, uint64, bool, error) {
	if s.cancelReads {
		close(s.entered)
		<-ctx.Done()
		return false, 0, false, ctx.Err()
	}
	return s.enabledStateMemoryStore.PluginEnabled(ctx, id)
}

func TestStandardPresetComposesOnlyItsPinnedCapabilities(t *testing.T) {
	if got := canonicalPresetVersion(standardPresetContent()); got != StandardPresetVersion {
		t.Fatalf("standard canonical version = %q, want reserved %q", got, StandardPresetVersion)
	}
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}

	composition, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	if PresetID(composition.Receipt.Preset.ID) != StandardPresetID ||
		composition.Receipt.Preset.Version != StandardPresetVersion {
		t.Fatalf("preset identity = %+v, want %s@%s", composition.Receipt.Preset, StandardPresetID, StandardPresetVersion)
	}
	if composition.Receipt.EvieVersion != EvieVersion {
		t.Fatalf("Evie version = %q, want %q", composition.Receipt.EvieVersion, EvieVersion)
	}
	wantSchemas := schemaNames(tools.KernelToolset().
		WithTools(tools.FinanceTools()).
		WithTools(tools.WebTools()).
		WithTools(tools.YouTubeTools()).
		WithTools(append(tools.TodoTools(), tools.TodoGetTool())))
	if got := schemaNames(composition.Toolset); !reflect.DeepEqual(got, wantSchemas) {
		t.Fatalf("standard schemas = %v, want %v", got, wantSchemas)
	}

	standard := BuiltinStandardPreset()
	standard.RequiredCapabilities[0].ID = "changed.capability"
	again := BuiltinStandardPreset()
	if again.RequiredCapabilities[0].ID == "changed.capability" {
		t.Fatal("caller mutated immutable built-in standard preset")
	}
	if _, err := manager.ResolvePreset("missing"); err == nil ||
		!strings.Contains(err.Error(), `Agent Preset "missing" is not allowed`) {
		t.Fatalf("unknown explicit preset error = %v", err)
	}
}

func TestPreYouTubeExtractionStandardReceiptResumesWithoutMutation(t *testing.T) {
	if got := canonicalPresetVersion(preYouTubeStandardPreset()); got != preYouTubeStandardPresetVersion {
		t.Fatalf("pre-YouTube canonical version = %q, want frozen %q", got, preYouTubeStandardPresetVersion)
	}
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	legacyPreset := Preset{
		ID:      StandardPresetID,
		Version: "sha256:41b87e45541e81e6a6e45b4cb5877db1d6fb7ab0ebb3cea5f4b24df5f77c2734",
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
		},
	}
	legacyManager, err := NewManager(tools.LegacyKernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID} {
		if err := legacyManager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := legacyManager.resolvePreset(legacyPreset)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSessionWithComposition(context.Background(), legacy.Receipt)
	if err != nil {
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
	storedReceipt, err := store.GetCompositionReceipt(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := manager.ResumeComposition(storedReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) {
		t.Fatalf("resume mutated legacy receipt\n got: %#v\nwant: %#v", resumed.Receipt, legacy.Receipt)
	}
	if !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatalf("resumed legacy schemas changed\n got: %#v\nwant: %#v", resumed.Toolset.Schemas(), legacy.Toolset.Schemas())
	}
	for _, name := range []string{"youtube_transcript", "youtube_scrape_channel"} {
		if countSchema(resumed.Toolset, name) != 1 {
			t.Fatalf("legacy resume exposes %q %d times", name, countSchema(resumed.Toolset, name))
		}
	}
}

func TestPostMemoryPreYouTubeStandardReceiptResumesWithoutMutation(t *testing.T) {
	legacyManager, err := NewManager(tools.LegacyKernelToolset(), NewWeb(), NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID} {
		if err := legacyManager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := legacyManager.resolvePreset(preYouTubeStandardPreset())
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := manager.ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) ||
		!reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatalf("post-Memory pre-YouTube receipt changed: got=%+v want=%+v", resumed.Receipt, legacy.Receipt)
	}
}

func TestPreTodoExtractionStandardReceiptResumesWithoutMutation(t *testing.T) {
	if got := canonicalPresetVersion(preTodoStandardPreset()); got != preTodoStandardPresetVersion {
		t.Fatalf("pre-Todo canonical version = %q, want frozen %q", got, preTodoStandardPresetVersion)
	}
	legacyManager, err := NewManager(
		tools.PreTodoExtractionKernelToolset(), NewWeb(), NewFinance(), NewYouTube(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID} {
		if err := legacyManager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := legacyManager.resolvePreset(preTodoStandardPreset())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSessionWithComposition(context.Background(), legacy.Receipt)
	if err != nil {
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
	storedReceipt, err := store.GetCompositionReceipt(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := manager.ResumeComposition(storedReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) {
		t.Fatalf("resume mutated pre-Todo receipt\n got: %#v\nwant: %#v", resumed.Receipt, legacy.Receipt)
	}
	if !reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatalf("resumed pre-Todo schemas changed\n got: %#v\nwant: %#v", resumed.Toolset.Schemas(), legacy.Toolset.Schemas())
	}
	for _, name := range []string{"todo_list", "todo_add"} {
		if countSchema(resumed.Toolset, name) != 1 {
			t.Fatalf("pre-Todo resume exposes %q %d times", name, countSchema(resumed.Toolset, name))
		}
	}
}

type preDurableTodoPlugin struct{}

func (preDurableTodoPlugin) Start(context.Context) error { return nil }

func (preDurableTodoPlugin) Stop(context.Context) error { return nil }

func (preDurableTodoPlugin) Manifest() Manifest {
	return Manifest{
		ID: TodoPluginID, ImplementationVersion: "1.0.0",
		KernelCompatibility: VersionRange{Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0"},
		Capabilities: []CapabilityContract{
			{ID: TodoListCapabilityID, Version: "1.0.0"},
			{ID: TodoAddCapabilityID, Version: "1.0.0"},
		},
	}
}

func (preDurableTodoPlugin) ToolCapabilities() []ToolCapability {
	definitions := tools.TodoTools()
	return []ToolCapability{
		{ID: TodoListCapabilityID, ContractVersion: "1.0.0", Tool: definitions[0]},
		{ID: TodoAddCapabilityID, ContractVersion: "1.0.0", Tool: definitions[1]},
	}
}

func TestPreDurableTodoStandardReceiptResumesThroughDeclaredCompatibility(t *testing.T) {
	if got := canonicalPresetVersion(preDurableTodoStandardPreset()); got != preDurableTodoPresetVersion {
		t.Fatalf("pre-durable-Todo canonical version = %q, want frozen %q", got, preDurableTodoPresetVersion)
	}
	legacyManager, err := NewManager(
		tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), preDurableTodoPlugin{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := legacyManager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	legacy, err := legacyManager.resolvePreset(preDurableTodoStandardPreset())
	if err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := manager.ResumeComposition(legacy.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Receipt, legacy.Receipt) ||
		!reflect.DeepEqual(resumed.Toolset.Schemas(), legacy.Toolset.Schemas()) {
		t.Fatalf("compatible resume changed frozen composition: %+v", resumed)
	}
	if len(resumed.CompatibilityResolutions) != 1 ||
		resumed.CompatibilityResolutions[0].OriginalProvider.ID != string(TodoPluginID) ||
		resumed.CompatibilityResolutions[0].ReplacementImplementationVersion != "1.1.0" {
		t.Fatalf("Todo compatibility resolutions = %+v", resumed.CompatibilityResolutions)
	}
}

func TestPresetResolutionFailsRequiredAndPersistsOptionalDiagnostics(t *testing.T) {
	required := fakeToolPlugin("required", "required.echo", "required_echo", "required")
	optional := fakeToolPlugin("optional", "optional.echo", "optional_echo", "optional")
	manager, err := NewManager(tools.NewToolset(nil), required, optional)
	if err != nil {
		t.Fatal(err)
	}
	preset := Preset{
		ID:      "fixture",
		Version: "sha256:3df4b83c2a4f7b40c88c1a78237f622644487aa118f47d2194d637c452432bbd",
		RequiredCapabilities: []CapabilityRequirement{{
			ID: "required.echo", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
		OptionalCapabilities: []CapabilityRequirement{{
			ID: "optional.echo", Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
	}

	if _, err := manager.resolvePreset(preset); err == nil ||
		!strings.Contains(err.Error(), `required Capability "required.echo"`) ||
		!strings.Contains(err.Error(), `enable plugin "required"`) {
		t.Fatalf("missing required error = %v", err)
	}
	if err := manager.SetEnabled("required", true); err != nil {
		t.Fatal(err)
	}
	incompatible := preset
	incompatible.RequiredCapabilities = append([]CapabilityRequirement(nil), preset.RequiredCapabilities...)
	incompatible.RequiredCapabilities[0].Compatibility = VersionRange{Minimum: "2.0.0", MaximumExclusive: "3.0.0"}
	if _, err := manager.resolvePreset(incompatible); err == nil ||
		!strings.Contains(err.Error(), "outside required range [2.0.0,3.0.0)") {
		t.Fatalf("incompatible required Capability error = %v", err)
	}
	composition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	if len(composition.Warnings) != 1 ||
		!strings.Contains(composition.Warnings[0], `optional Capability "optional.echo"`) {
		t.Fatalf("warnings = %v", composition.Warnings)
	}
	wantReceiptWarnings := []CompositionWarning{{
		Code: kernelcomposition.WarningProviderDisabled, CapabilityID: "optional.echo", ProviderID: "optional",
	}}
	if !reflect.DeepEqual(composition.Receipt.Warnings, wantReceiptWarnings) {
		t.Fatalf("receipt warnings = %v, want %v", composition.Receipt.Warnings, wantReceiptWarnings)
	}
	assertToolResult(t, composition.Toolset, "required_echo", "required")
	assertUnknownTool(t, composition.Toolset, "optional_echo")

	if err := manager.SetEnabled("optional", true); err != nil {
		t.Fatal(err)
	}
	newComposition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, newComposition.Toolset, "optional_echo", "optional")
	assertUnknownTool(t, composition.Toolset, "optional_echo")
}

func TestPresetValidityIgnoresConnectionReadiness(t *testing.T) {
	manager, err := NewManager(
		tools.NewToolset(nil),
		fakeToolPlugin("account", "account.read", "account_read", "connect account"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("account", true); err != nil {
		t.Fatal(err)
	}
	preset := fixturePreset("account.read")
	preset.Configuration = []ConfigurationReference{{
		Kind: ConfigurationConnection, ID: "c9299e6b-81db-47df-8ee7-d5d9a5f9e94f",
	}}
	composition, err := manager.resolvePreset(preset)
	if err != nil {
		t.Fatalf("connection readiness invalidated preset: %v", err)
	}
	assertToolResult(t, composition.Toolset, "account_read", "connect account")
	if !reflect.DeepEqual(composition.Receipt.Configuration, preset.Configuration) {
		t.Fatalf("missing Connection ID receipt refs = %#v, want %#v", composition.Receipt.Configuration, preset.Configuration)
	}
}

func fixturePreset(required CapabilityID) Preset {
	return Preset{
		ID:      "fixture",
		Version: "sha256:3df4b83c2a4f7b40c88c1a78237f622644487aa118f47d2194d637c452432bbd",
		RequiredCapabilities: []CapabilityRequirement{{
			ID: required, Compatibility: VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
		}},
	}
}

func TestReceiptRejectsContentThatCouldCarryCredentials(t *testing.T) {
	preset := fixturePreset("fixture.echo")
	preset.Configuration = []ConfigurationReference{{Kind: ConfigurationConnection, ID: "sk-live-secret-token"}}
	manager, err := NewManager(tools.NewToolset(nil), fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "ok"))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.resolvePreset(preset); err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("credential-shaped configuration error = %v", err)
	}
}

func TestResumeCompositionRequiresEveryExactPinnedProviderAndSchema(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []PluginID{WebPluginID, FinancePluginID, YouTubePluginID, TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	pinned, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}

	changedProvider := pinned.Receipt
	changedProvider.Providers = append([]ProviderReceipt(nil), pinned.Receipt.Providers...)
	changedProvider.Providers[0].ImplementationVersion = "1.0.1"
	if _, err := manager.ResumeComposition(changedProvider); err == nil ||
		!strings.Contains(err.Error(), `provider plugin "finance" requires implementation version "1.0.1"`) {
		t.Fatalf("changed provider resume error = %v", err)
	}

	changedSchema := pinned.Receipt
	changedSchema.Capabilities = append([]CapabilityReceipt(nil), pinned.Receipt.Capabilities...)
	changedSchema.Capabilities[0].SchemaSHA256 = strings.Repeat("0", 64)
	if _, err := manager.ResumeComposition(changedSchema); err == nil ||
		!strings.Contains(err.Error(), `pinned Capability "finance.sync" requires schema`) {
		t.Fatalf("changed schema resume error = %v", err)
	}

	if err := manager.SetEnabled(FinancePluginID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResumeComposition(pinned.Receipt); err == nil ||
		!strings.Contains(err.Error(), `pinned provider plugin "finance" is disabled`) {
		t.Fatalf("missing pinned provider resume error = %v", err)
	}
}

func TestResumeCompositionContextCancelsEnabledStateRefresh(t *testing.T) {
	manager, err := NewManager(tools.KernelToolset(), NewWeb(), NewFinance(), NewYouTube(), NewTodo(&taskServiceFixture{}))
	if err != nil {
		t.Fatal(err)
	}
	store := &cancelingEnabledStateStore{enabledStateMemoryStore: &enabledStateMemoryStore{
		values: map[string]bool{string(WebPluginID): true, string(FinancePluginID): true, string(YouTubePluginID): true, string(TodoPluginID): true},
	}}
	if err := manager.ConfigureEnabledState(context.Background(), store, map[PluginID]bool{
		WebPluginID: true, FinancePluginID: true, YouTubePluginID: true, TodoPluginID: true,
	}); err != nil {
		t.Fatal(err)
	}
	pinned, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}

	store.cancelReads = true
	store.entered = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, resumeErr := manager.ResumeCompositionContext(ctx, pinned.Receipt)
		result <- resumeErr
	}()
	<-store.entered
	cancel()
	err = <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ResumeCompositionContext error = %v, want context canceled", err)
	}
}

func TestResumeCompositionUsesOnlyExplicitCompatibleReplacement(t *testing.T) {
	originalPlugin := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "original")
	originalManager, err := NewManager(tools.NewToolset(nil), originalPlugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := originalManager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}
	preset := fixturePreset("fixture.echo")
	pinned, err := originalManager.resolvePreset(preset)
	if err != nil {
		t.Fatal(err)
	}

	exact, err := originalManager.resumePreset(preset, pinned.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.CompatibilityResolutions) != 0 {
		t.Fatalf("exact resume resolutions = %#v, want none", exact.CompatibilityResolutions)
	}

	replacementPlugin := fakeToolPlugin("fixture", "fixture.echo", "fixture_echo", "replacement")
	replacementPlugin.manifest.ImplementationVersion = "1.1.0"
	replacementPlugin.manifest.ResumableFrom = []ImplementationCompatibility{{
		ImplementationVersion: "1.0.0",
		Capabilities: []CapabilityCompatibility{{
			ID: "fixture.echo", ContractVersion: "1.0.0",
			SchemaSHA256: pinned.Receipt.Capabilities[0].SchemaSHA256,
		}},
	}}
	replacementManager, err := NewManager(tools.NewToolset(nil), replacementPlugin)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacementManager.SetEnabled("fixture", true); err != nil {
		t.Fatal(err)
	}

	resumed, err := replacementManager.resumePreset(preset, pinned.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	assertToolResult(t, resumed.Toolset, "fixture_echo", "replacement")
	if !reflect.DeepEqual(resumed.Receipt, pinned.Receipt) {
		t.Fatalf("resumed receipt = %#v, want immutable original %#v", resumed.Receipt, pinned.Receipt)
	}
	if len(resumed.CompatibilityResolutions) != 1 {
		t.Fatalf("replacement resolutions = %#v, want one", resumed.CompatibilityResolutions)
	}
	resolution := resumed.CompatibilityResolutions[0]
	if resolution.OriginalProvider.ID != "fixture" ||
		resolution.OriginalProvider.ImplementationVersion != "1.0.0" ||
		resolution.ReplacementImplementationVersion != "1.1.0" ||
		resolution.KernelAPIVersion != KernelAPIVersion ||
		len(resolution.Capabilities) != 1 ||
		resolution.Capabilities[0].ID != "fixture.echo" ||
		resolution.Capabilities[0].ContractVersion != "1.0.0" ||
		resolution.Capabilities[0].SchemaSHA256 != pinned.Receipt.Capabilities[0].SchemaSHA256 ||
		resolution.ResolvedAt.IsZero() {
		t.Fatalf("replacement resolution = %#v, want exact requirement, evidence, and time", resolution)
	}
}
