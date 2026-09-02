package plugins

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

const (
	EvieVersion                    = "1.0.0"
	StandardPresetID      PresetID = "standard"
	StandardPresetVersion          = "sha256:3a40a38be361fc81f2e6e4bfa33621b452e3f749fd8b9661a6766810f3e0cf67"

	preMemoryStandardPresetVersion  = "sha256:41b87e45541e81e6a6e45b4cb5877db1d6fb7ab0ebb3cea5f4b24df5f77c2734"
	preYouTubeStandardPresetVersion = "sha256:b9907aeee8dcd35e3297ea0f56d8d79eaf44851d3d9a67c0595eb7334022ea16"
	preTodoStandardPresetVersion    = "sha256:8149efdcc636e89d8c404c181cfa595bfe8ab09b38bffbefb756f73e99e5d6c0"
	preDurableTodoPresetVersion     = "sha256:6490dc45771d4fc2d865fa9c3380d660b8100ad2c77bc79007c8d4e7b2053694"
	preLifecycleTodoPresetVersion   = "sha256:d3bb965df9ac07057a94b4816e42e072130335422baa0f0ce3d38cffa8702554"
)

type PresetID string

type CapabilityRequirement struct {
	ID            CapabilityID
	Compatibility VersionRange
}

type InstructionReference = composition.InstructionReference
type ConfigurationReferenceKind = composition.ConfigurationReferenceKind
type ConfigurationReference = composition.ConfigurationReference

const ConfigurationConnection = composition.ConfigurationConnection

type Preset struct {
	ID                   PresetID
	Version              string
	RequiredCapabilities []CapabilityRequirement
	OptionalCapabilities []CapabilityRequirement
	Instructions         []InstructionReference
	Configuration        []ConfigurationReference
}

// PresetInspection is the complete generic management representation of one
// immutable built-in preset. Errors contain every unavailable or incompatible
// required Capability; warnings contain every unavailable optional one.
type PresetInspection struct {
	ID                   PresetID                `json:"id"`
	Version              string                  `json:"version"`
	RequiredCapabilities []CapabilityRequirement `json:"requiredCapabilities"`
	OptionalCapabilities []CapabilityRequirement `json:"optionalCapabilities"`
	Valid                bool                    `json:"valid"`
	Warnings             []string                `json:"warnings"`
	Errors               []string                `json:"errors"`
	Immutable            bool                    `json:"immutable"`
}

type PresetIdentity = composition.PresetIdentity
type ProviderReceipt = composition.Provider
type CapabilityReceipt = composition.Capability
type ToolSchemaReceipt = composition.ToolSchema
type CompositionWarning = composition.Warning
type CompositionReceipt = composition.Receipt
type CompatibilityResolution = composition.CompatibilityResolution

type ResolvedComposition struct {
	Toolset                  tools.Toolset
	Receipt                  CompositionReceipt
	Warnings                 []string
	CompatibilityResolutions []CompatibilityResolution
}

func standardPresetContent() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID: StandardPresetID,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
			{ID: YouTubeTranscriptCapabilityID, Compatibility: compatibility},
			{ID: YouTubeScrapeChannelCapabilityID, Compatibility: compatibility},
			{ID: TodoListCapabilityID, Compatibility: compatibility},
			{ID: TodoAddCapabilityID, Compatibility: compatibility},
			{ID: TodoGetCapabilityID, Compatibility: compatibility},
			{ID: TodoUpdateCapabilityID, Compatibility: compatibility},
		},
		OptionalCapabilities: memoryCapabilityRequirements(compatibility),
	}
}

func preLifecycleTodoStandardPreset() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID:      StandardPresetID,
		Version: preLifecycleTodoPresetVersion,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
			{ID: YouTubeTranscriptCapabilityID, Compatibility: compatibility},
			{ID: YouTubeScrapeChannelCapabilityID, Compatibility: compatibility},
			{ID: TodoListCapabilityID, Compatibility: compatibility},
			{ID: TodoAddCapabilityID, Compatibility: compatibility},
			{ID: TodoGetCapabilityID, Compatibility: compatibility},
		},
		OptionalCapabilities: memoryCapabilityRequirements(compatibility),
	}
}

func preDurableTodoStandardPreset() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID:      StandardPresetID,
		Version: preDurableTodoPresetVersion,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
			{ID: YouTubeTranscriptCapabilityID, Compatibility: compatibility},
			{ID: YouTubeScrapeChannelCapabilityID, Compatibility: compatibility},
			{ID: TodoListCapabilityID, Compatibility: compatibility},
			{ID: TodoAddCapabilityID, Compatibility: compatibility},
		},
		OptionalCapabilities: memoryCapabilityRequirements(compatibility),
	}
}

func preTodoStandardPreset() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID:      StandardPresetID,
		Version: preTodoStandardPresetVersion,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
			{ID: YouTubeTranscriptCapabilityID, Compatibility: compatibility},
			{ID: YouTubeScrapeChannelCapabilityID, Compatibility: compatibility},
		},
		OptionalCapabilities: memoryCapabilityRequirements(compatibility),
	}
}

func preYouTubeStandardPreset() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID:      StandardPresetID,
		Version: preYouTubeStandardPresetVersion,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
		},
		OptionalCapabilities: memoryCapabilityRequirements(compatibility),
	}
}

func preMemoryStandardPreset() Preset {
	compatibility := VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"}
	return Preset{
		ID:      StandardPresetID,
		Version: preMemoryStandardPresetVersion,
		RequiredCapabilities: []CapabilityRequirement{
			{ID: FinanceSyncCapabilityID, Compatibility: compatibility},
			{ID: FinanceRulesCapabilityID, Compatibility: compatibility},
			{ID: FinanceCategorizeCapabilityID, Compatibility: compatibility},
			{ID: WebFetchCapabilityID, Compatibility: compatibility},
			{ID: WebSearchCapabilityID, Compatibility: compatibility},
		},
	}
}

func memoryCapabilityRequirements(compatibility VersionRange) []CapabilityRequirement {
	ids := allMemoryCapabilityIDs()
	requirements := make([]CapabilityRequirement, len(ids))
	for i, id := range ids {
		requirements[i] = CapabilityRequirement{ID: id, Compatibility: compatibility}
	}
	return requirements
}

// BuiltinStandardPreset returns a detached snapshot so callers cannot mutate
// the reserved built-in definition.
func BuiltinStandardPreset() Preset {
	preset := standardPresetContent()
	preset.Version = StandardPresetVersion
	return clonePreset(preset)
}

func canonicalPresetVersion(preset Preset) string {
	preset.Version = ""
	encoded, err := json.Marshal(preset)
	if err != nil {
		panic(fmt.Sprintf("marshal built-in Agent Preset: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func clonePreset(preset Preset) Preset {
	preset.RequiredCapabilities = append([]CapabilityRequirement(nil), preset.RequiredCapabilities...)
	preset.OptionalCapabilities = append([]CapabilityRequirement(nil), preset.OptionalCapabilities...)
	preset.Instructions = append([]InstructionReference(nil), preset.Instructions...)
	preset.Configuration = append([]ConfigurationReference(nil), preset.Configuration...)
	return preset
}

// ResolvePreset resolves one allowed preset once. Empty means the reserved
// standard preset; unknown explicit identities fail without fallback.
func (m *Manager) ResolvePreset(id PresetID) (ResolvedComposition, error) {
	return m.ResolvePresetContext(context.Background(), id)
}

func (m *Manager) ResolvePresetContext(ctx context.Context, id PresetID) (ResolvedComposition, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return ResolvedComposition{}, err
	}
	if id == "" {
		id = StandardPresetID
	}
	if id != StandardPresetID {
		return ResolvedComposition{}, fmt.Errorf("Agent Preset %q is not allowed; choose %q", id, StandardPresetID)
	}
	return m.resolvePreset(BuiltinStandardPreset())
}

// InspectPresets lists detached snapshots of the built-ins known to this
// binary. Phase 1 intentionally has no mutable or plugin-injected presets.
func (m *Manager) InspectPresets() ([]PresetInspection, error) {
	return m.InspectPresetsContext(context.Background())
}

func (m *Manager) InspectPresetsContext(ctx context.Context) ([]PresetInspection, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return nil, err
	}
	return []PresetInspection{m.inspectPreset(BuiltinStandardPreset(), true)}, nil
}

// ValidatePreset validates exactly the requested built-in. Unknown IDs are
// returned as invalid and are never replaced with the default preset.
func (m *Manager) ValidatePreset(id PresetID) (PresetInspection, error) {
	return m.ValidatePresetContext(context.Background(), id)
}

func (m *Manager) validatePresetCurrent(id PresetID) PresetInspection {
	if id == "" {
		id = StandardPresetID
	}
	if id != StandardPresetID {
		return PresetInspection{
			ID: id, Valid: false, Immutable: true,
			Errors:   []string{fmt.Sprintf("Agent Preset %q is not allowed; choose %q", id, StandardPresetID)},
			Warnings: []string{}, RequiredCapabilities: []CapabilityRequirement{}, OptionalCapabilities: []CapabilityRequirement{},
		}
	}
	return m.inspectPreset(BuiltinStandardPreset(), true)
}

func (m *Manager) ValidatePresetContext(ctx context.Context, id PresetID) (PresetInspection, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return PresetInspection{}, err
	}
	return m.validatePresetCurrent(id), nil
}

func (m *Manager) inspectPreset(preset Preset, immutable bool) PresetInspection {
	report := PresetInspection{
		ID: preset.ID, Version: preset.Version,
		RequiredCapabilities: append([]CapabilityRequirement(nil), preset.RequiredCapabilities...),
		OptionalCapabilities: append([]CapabilityRequirement(nil), preset.OptionalCapabilities...),
		Warnings:             []string{}, Errors: []string{}, Immutable: immutable,
	}
	if err := validatePreset(preset); err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Agent Preset %q is invalid: %v", preset.ID, err))
		report.Valid = false
		return report
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, requirement := range preset.RequiredCapabilities {
		_, _, _, diagnostic := m.resolveCapabilityLocked(requirement)
		if diagnostic != "" {
			report.Errors = append(report.Errors, fmt.Sprintf("required Capability %q is unavailable: %s", requirement.ID, diagnostic))
		}
	}
	for _, requirement := range preset.OptionalCapabilities {
		_, _, _, diagnostic := m.resolveCapabilityLocked(requirement)
		if diagnostic != "" {
			report.Warnings = append(report.Warnings, fmt.Sprintf("optional Capability %q is unavailable: %s", requirement.ID, diagnostic))
		}
	}
	report.Valid = len(report.Errors) == 0
	return report
}

func (m *Manager) resolvePreset(preset Preset) (ResolvedComposition, error) {
	report := m.inspectPreset(preset, preset.ID == StandardPresetID)
	if !report.Valid {
		return ResolvedComposition{}, fmt.Errorf("resolve Agent Preset %q: %s", preset.ID, strings.Join(report.Errors, "; "))
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	type selectedCapability struct {
		entry      *compiledPlugin
		capability ToolCapability
	}
	selected := make([]selectedCapability, 0, len(preset.RequiredCapabilities)+len(preset.OptionalCapabilities))
	warnings := make([]string, 0)
	warningReceipts := make([]CompositionWarning, 0)
	resolve := func(requirement CapabilityRequirement, required bool) error {
		entry, capability, warningCode, diagnostic := m.resolveCapabilityLocked(requirement)
		if diagnostic != "" {
			if required {
				return fmt.Errorf("required Capability %q is unavailable: %s", requirement.ID, diagnostic)
			}
			warnings = append(warnings, fmt.Sprintf("optional Capability %q is unavailable: %s", requirement.ID, diagnostic))
			warningReceipts = append(warningReceipts, CompositionWarning{
				Code: warningCode, CapabilityID: string(requirement.ID),
				ProviderID: string(providerIDForCapability(requirement.ID)),
			})
			return nil
		}
		selected = append(selected, selectedCapability{entry: entry, capability: capability})
		return nil
	}
	for _, requirement := range preset.RequiredCapabilities {
		if err := resolve(requirement, true); err != nil {
			return ResolvedComposition{}, fmt.Errorf("resolve Agent Preset %q: %w", preset.ID, err)
		}
	}
	for _, requirement := range preset.OptionalCapabilities {
		if err := resolve(requirement, false); err != nil {
			return ResolvedComposition{}, err
		}
	}

	definitions := make([]tools.Tool, 0, len(selected))
	schemaOwner := make(map[string]string)
	for _, schema := range m.base.Schemas() {
		schemaOwner[schema.Function.Name] = "Kernel"
	}
	providers := make(map[PluginID]ProviderReceipt)
	capabilities := make([]CapabilityReceipt, 0, len(selected))
	for _, selection := range selected {
		name := selection.capability.Tool.Schema.Function.Name
		if owner, exists := schemaOwner[name]; exists {
			return ResolvedComposition{}, fmt.Errorf(
				"resolve Agent Preset %q: duplicate model-facing tool schema %q from %s and Capability %q",
				preset.ID, name, owner, selection.capability.ID,
			)
		}
		schemaOwner[name] = fmt.Sprintf("Capability %q", selection.capability.ID)
		definitions = append(definitions, selection.capability.Tool)
		providers[selection.entry.manifest.ID] = ProviderReceipt{
			ID: string(selection.entry.manifest.ID), ImplementationVersion: selection.entry.manifest.ImplementationVersion,
		}
		capabilities = append(capabilities, CapabilityReceipt{
			ID: string(selection.capability.ID), ProviderID: string(selection.entry.manifest.ID),
			ContractVersion: selection.capability.ContractVersion,
			SchemaSHA256:    schemaHash(selection.capability.Tool.Schema),
		})
	}
	providerReceipts := make([]ProviderReceipt, 0, len(providers))
	for _, provider := range providers {
		providerReceipts = append(providerReceipts, provider)
	}
	sort.Slice(providerReceipts, func(i, j int) bool { return providerReceipts[i].ID < providerReceipts[j].ID })

	toolset := m.base.WithTools(definitions)
	receipt := CompositionReceipt{
		FormatVersion: composition.FormatVersion,
		Preset:        PresetIdentity{ID: string(preset.ID), Version: preset.Version},
		EvieVersion:   EvieVersion,
		Providers:     providerReceipts,
		Capabilities:  capabilities,
		ToolSchemas:   toolSchemaReceipts(toolset.Schemas()),
		Instructions:  append([]InstructionReference(nil), preset.Instructions...),
		Configuration: append([]ConfigurationReference(nil), preset.Configuration...),
		Warnings:      warningReceipts,
	}
	if err := ValidateCompositionReceipt(receipt); err != nil {
		return ResolvedComposition{}, fmt.Errorf("build Composition Receipt: %w", err)
	}
	return ResolvedComposition{
		Toolset: toolset, Receipt: composition.Clone(receipt), Warnings: append([]string(nil), warnings...),
	}, nil
}

func (m *Manager) resolveCapabilityLocked(
	requirement CapabilityRequirement,
) (*compiledPlugin, ToolCapability, composition.WarningCode, string) {
	providerID := providerIDForCapability(requirement.ID)
	entry, exists := m.plugins[providerID]
	if !exists {
		return nil, ToolCapability{}, composition.WarningProviderNotCompiled,
			fmt.Sprintf("provider plugin %q is not compiled into Evie", providerID)
	}
	if entry.state != StateReady {
		detail := string(entry.state)
		warningCode := composition.WarningProviderWaiting
		if entry.state == StateDisabled {
			detail += fmt.Sprintf("; enable plugin %q", providerID)
			warningCode = composition.WarningProviderDisabled
		} else if entry.state == StateStopped {
			warningCode = composition.WarningProviderStopped
		} else if entry.state == StateFailed {
			warningCode = composition.WarningProviderFailed
		}
		if entry.diagnostic != "" {
			detail += ": " + entry.diagnostic
		}
		return nil, ToolCapability{}, warningCode, fmt.Sprintf("provider plugin %q is %s", providerID, detail)
	}
	for _, capability := range entry.activeCapabilities {
		if capability.ID != requirement.ID {
			continue
		}
		version, err := parseManifestVersion(capability.ContractVersion)
		if err != nil {
			return nil, ToolCapability{}, composition.WarningProviderContractInvalid,
				fmt.Sprintf("provider contract version %q is invalid", capability.ContractVersion)
		}
		minimum, maximum, err := parseVersionRange(requirement.Compatibility)
		if err != nil {
			return nil, ToolCapability{}, composition.WarningContractIncompatible,
				fmt.Sprintf("requested compatibility is invalid: %v", err)
		}
		if version.Compare(minimum) < 0 || version.Compare(maximum) >= 0 {
			return nil, ToolCapability{}, composition.WarningContractIncompatible, fmt.Sprintf(
				"provider plugin %q contract version %s is outside required range [%s,%s)",
				providerID, capability.ContractVersion,
				requirement.Compatibility.Minimum, requirement.Compatibility.MaximumExclusive,
			)
		}
		return entry, capability, "", ""
	}
	return nil, ToolCapability{}, composition.WarningCapabilityNotExposed,
		fmt.Sprintf("provider plugin %q does not expose the Capability", providerID)
}

func providerIDForCapability(id CapabilityID) PluginID {
	value := string(id)
	separator := strings.IndexByte(value, '.')
	if separator <= 0 {
		return PluginID(value)
	}
	return PluginID(value[:separator])
}

func validatePreset(preset Preset) error {
	if strings.TrimSpace(string(preset.ID)) == "" {
		return errors.New("identity must not be empty")
	}
	if !validSHA256Version(preset.Version) {
		return errors.New("version must be sha256:<lowercase SHA-256>")
	}
	seen := make(map[CapabilityID]string)
	for _, group := range []struct {
		name         string
		requirements []CapabilityRequirement
	}{
		{name: "required", requirements: preset.RequiredCapabilities},
		{name: "optional", requirements: preset.OptionalCapabilities},
	} {
		for _, requirement := range group.requirements {
			if strings.TrimSpace(string(requirement.ID)) == "" {
				return fmt.Errorf("%s Capability ID must not be empty", group.name)
			}
			if prior, exists := seen[requirement.ID]; exists {
				return fmt.Errorf("Capability %q is listed as both %s and %s", requirement.ID, prior, group.name)
			}
			seen[requirement.ID] = group.name
			if _, _, err := parseVersionRange(requirement.Compatibility); err != nil {
				return fmt.Errorf("Capability %q compatibility: %w", requirement.ID, err)
			}
		}
	}
	for _, instruction := range preset.Instructions {
		if strings.TrimSpace(instruction.ID) == "" || !composition.ValidSHA256(instruction.SHA256) {
			return fmt.Errorf("instruction %q must have an identity and lowercase SHA-256 hash", instruction.ID)
		}
	}
	for _, reference := range preset.Configuration {
		if err := composition.ValidateConfigurationReference(reference); err != nil {
			return err
		}
	}
	return nil
}

func schemaHash(schema openrouter.Tool) string {
	encoded, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal validated tool schema: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func toolSchemaReceipts(schemas []openrouter.Tool) []ToolSchemaReceipt {
	receipts := make([]ToolSchemaReceipt, len(schemas))
	for i, schema := range schemas {
		receipts[i] = ToolSchemaReceipt{Name: schema.Function.Name, SHA256: schemaHash(schema)}
	}
	return receipts
}

func validSHA256Version(value string) bool {
	return composition.ValidSHA256Version(value)
}

func ValidateCompositionReceipt(receipt CompositionReceipt) error {
	return composition.Validate(receipt)
}

// ResumeComposition reconstructs the pinned standard composition. The exact
// provider version wins; a newer version is considered only when its Manifest
// explicitly proves compatibility with every pinned contract and schema.
func (m *Manager) ResumeComposition(receipt CompositionReceipt) (ResolvedComposition, error) {
	return m.ResumeCompositionContext(context.Background(), receipt)
}

// ResumeCompositionContext reconstructs a pinned composition while preserving
// cancellation across durable enabled-state refresh and lifecycle work.
func (m *Manager) ResumeCompositionContext(
	ctx context.Context,
	receipt CompositionReceipt,
) (ResolvedComposition, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return ResolvedComposition{}, err
	}
	switch receipt.Preset.Version {
	case preMemoryStandardPresetVersion:
		return m.resumePresetWithBase(
			preMemoryStandardPreset(), tools.LegacyKernelToolset(), receipt, YouTubePluginID, TodoPluginID,
		)
	case preYouTubeStandardPresetVersion:
		return m.resumePresetWithBase(
			preYouTubeStandardPreset(), tools.LegacyKernelToolset(), receipt, YouTubePluginID, TodoPluginID,
		)
	}
	if receipt.Preset.Version == preTodoStandardPresetVersion {
		return m.resumePresetWithBase(
			preTodoStandardPreset(), tools.PreTodoExtractionKernelToolset(), receipt, TodoPluginID,
		)
	}
	if receipt.Preset.Version == preDurableTodoPresetVersion {
		return m.resumePreset(preDurableTodoStandardPreset(), receipt)
	}
	if receipt.Preset.Version == preLifecycleTodoPresetVersion {
		return m.resumePreset(preLifecycleTodoStandardPreset(), receipt)
	}
	return m.resumePreset(BuiltinStandardPreset(), receipt)
}

func (m *Manager) resumePreset(preset Preset, receipt CompositionReceipt) (ResolvedComposition, error) {
	return m.resumePresetWithBase(preset, m.base, receipt)
}

func (m *Manager) resumePresetWithBase(
	preset Preset,
	base tools.Toolset,
	receipt CompositionReceipt,
	legacyProviderIDs ...PluginID,
) (ResolvedComposition, error) {
	if err := ValidateCompositionReceipt(receipt); err != nil {
		return ResolvedComposition{}, err
	}
	if PresetID(receipt.Preset.ID) != preset.ID {
		return ResolvedComposition{}, fmt.Errorf("pinned Agent Preset %q is not available", receipt.Preset.ID)
	}
	if receipt.Preset.Version != preset.Version {
		return ResolvedComposition{}, fmt.Errorf("pinned Agent Preset %q version %q is not available", receipt.Preset.ID, receipt.Preset.Version)
	}
	if err := validateReceiptCapabilitiesForPreset(preset, receipt); err != nil {
		return ResolvedComposition{}, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, id := range legacyProviderIDs {
		entry, exists := m.plugins[id]
		if !exists {
			return ResolvedComposition{}, fmt.Errorf("legacy provider plugin %q is not compiled into Evie", id)
		}
		if entry.state != StateReady {
			return ResolvedComposition{}, fmt.Errorf("legacy provider plugin %q is %s", id, entry.state)
		}
	}
	providerReceipts := make(map[PluginID]ProviderReceipt, len(receipt.Providers))
	for _, provider := range receipt.Providers {
		providerReceipts[PluginID(provider.ID)] = provider
	}
	providerCapabilities := make(map[PluginID][]CapabilityReceipt)
	definitions := make([]tools.Tool, 0, len(receipt.Capabilities))
	for _, pinnedCapability := range receipt.Capabilities {
		providerID := PluginID(pinnedCapability.ProviderID)
		providerCapabilities[providerID] = append(providerCapabilities[providerID], pinnedCapability)
		entry, exists := m.plugins[providerID]
		if !exists {
			provider := providerReceipts[providerID]
			return ResolvedComposition{}, fmt.Errorf(
				"pinned provider plugin %q version %q is not compiled into Evie",
				providerID, provider.ImplementationVersion,
			)
		}
		if entry.state != StateReady {
			return ResolvedComposition{}, fmt.Errorf(
				"pinned provider plugin %q is %s; required version is %q",
				providerID, entry.state, providerReceipts[providerID].ImplementationVersion,
			)
		}
		capability, exists := activeCapability(entry, CapabilityID(pinnedCapability.ID))
		if provider := providerReceipts[providerID]; provider.ImplementationVersion != entry.manifest.ImplementationVersion {
			if resumable, found := resumableCapability(entry, provider.ImplementationVersion, CapabilityID(pinnedCapability.ID)); found {
				capability, exists = resumable, true
			}
		}
		if !exists {
			return ResolvedComposition{}, fmt.Errorf(
				"pinned Capability %q is not exposed by provider plugin %q version %q",
				pinnedCapability.ID, providerID, entry.manifest.ImplementationVersion,
			)
		}
		if capability.ContractVersion != pinnedCapability.ContractVersion {
			return ResolvedComposition{}, fmt.Errorf(
				"pinned Capability %q requires contract version %q; provider plugin %q version %q exposes %q",
				pinnedCapability.ID, pinnedCapability.ContractVersion, providerID,
				entry.manifest.ImplementationVersion, capability.ContractVersion,
			)
		}
		actualSchema := schemaHash(capability.Tool.Schema)
		if actualSchema != pinnedCapability.SchemaSHA256 {
			return ResolvedComposition{}, fmt.Errorf(
				"pinned Capability %q requires schema %s; provider plugin %q version %q exposes %s",
				pinnedCapability.ID, pinnedCapability.SchemaSHA256, providerID,
				entry.manifest.ImplementationVersion, actualSchema,
			)
		}
		definitions = append(definitions, capability.Tool)
	}

	resolutions := make([]CompatibilityResolution, 0)
	for _, pinnedProvider := range receipt.Providers {
		entry := m.plugins[PluginID(pinnedProvider.ID)]
		if entry == nil {
			continue
		}
		if entry.manifest.ImplementationVersion == pinnedProvider.ImplementationVersion {
			continue
		}
		declaration, exists := compatibleImplementation(entry.manifest, pinnedProvider.ImplementationVersion)
		if !exists {
			return ResolvedComposition{}, fmt.Errorf(
				"pinned provider plugin %q requires implementation version %q; loaded version %q does not declare it resumable",
				pinnedProvider.ID, pinnedProvider.ImplementationVersion, entry.manifest.ImplementationVersion,
			)
		}
		evidenceByID := make(map[CapabilityID]CapabilityCompatibility, len(declaration.Capabilities))
		for _, evidence := range declaration.Capabilities {
			evidenceByID[evidence.ID] = evidence
		}
		resolution := CompatibilityResolution{
			OriginalProvider:                 pinnedProvider,
			ReplacementImplementationVersion: entry.manifest.ImplementationVersion,
			KernelAPIVersion:                 KernelAPIVersion,
			ResolvedAt:                       time.Now().UTC(),
		}
		for _, pinnedCapability := range providerCapabilities[PluginID(pinnedProvider.ID)] {
			evidence, declared := evidenceByID[CapabilityID(pinnedCapability.ID)]
			if !declared || evidence.ContractVersion != pinnedCapability.ContractVersion ||
				evidence.SchemaSHA256 != pinnedCapability.SchemaSHA256 {
				return ResolvedComposition{}, fmt.Errorf(
					"pinned provider plugin %q version %q lacks exact compatibility evidence for Capability %q contract %q schema %s",
					pinnedProvider.ID, pinnedProvider.ImplementationVersion, pinnedCapability.ID,
					pinnedCapability.ContractVersion, pinnedCapability.SchemaSHA256,
				)
			}
			resolution.Capabilities = append(resolution.Capabilities, compatibilityResolutionCapability(evidence))
		}
		if err := composition.ValidateCompatibilityResolution(resolution); err != nil {
			return ResolvedComposition{}, fmt.Errorf("build Compatibility Resolution: %w", err)
		}
		resolutions = append(resolutions, resolution)
	}

	toolset := base.WithTools(definitions)
	if err := validatePinnedToolSchemas(receipt.ToolSchemas, toolSchemaReceipts(toolset.Schemas())); err != nil {
		return ResolvedComposition{}, err
	}
	return ResolvedComposition{
		Toolset: toolset, Receipt: composition.Clone(receipt),
		CompatibilityResolutions: resolutions,
	}, nil
}

func compatibilityResolutionCapability(evidence CapabilityCompatibility) composition.CompatibilityCapability {
	return composition.CompatibilityCapability{
		ID: string(evidence.ID), ContractVersion: evidence.ContractVersion, SchemaSHA256: evidence.SchemaSHA256,
	}
}

func validateReceiptCapabilitiesForPreset(preset Preset, receipt CompositionReceipt) error {
	required := make(map[CapabilityID]struct{}, len(preset.RequiredCapabilities))
	allowed := make(map[CapabilityID]VersionRange, len(preset.RequiredCapabilities)+len(preset.OptionalCapabilities))
	for _, requirement := range preset.RequiredCapabilities {
		required[requirement.ID] = struct{}{}
		allowed[requirement.ID] = requirement.Compatibility
	}
	for _, requirement := range preset.OptionalCapabilities {
		allowed[requirement.ID] = requirement.Compatibility
	}
	usedProviders := make(map[string]struct{}, len(receipt.Providers))
	for _, capability := range receipt.Capabilities {
		id := CapabilityID(capability.ID)
		compatibility, exists := allowed[id]
		if !exists {
			return fmt.Errorf("pinned Agent Preset %q contains unexpected Capability %q", preset.ID, id)
		}
		version, err := parseManifestVersion(capability.ContractVersion)
		if err != nil {
			return fmt.Errorf("pinned Capability %q contract version %q is invalid: %w", id, capability.ContractVersion, err)
		}
		minimum, maximum, err := parseVersionRange(compatibility)
		if err != nil {
			return fmt.Errorf("pinned Agent Preset %q Capability %q compatibility is invalid: %w", preset.ID, id, err)
		}
		if version.Compare(minimum) < 0 || version.Compare(maximum) >= 0 {
			return fmt.Errorf(
				"pinned Capability %q contract version %q is outside Agent Preset %q required range [%s,%s)",
				id, capability.ContractVersion, preset.ID,
				compatibility.Minimum, compatibility.MaximumExclusive,
			)
		}
		usedProviders[capability.ProviderID] = struct{}{}
		delete(required, id)
	}
	if len(required) != 0 {
		ids := make([]string, 0, len(required))
		for id := range required {
			ids = append(ids, string(id))
		}
		sort.Strings(ids)
		return fmt.Errorf("pinned Agent Preset %q is missing required Capability %q", preset.ID, ids[0])
	}
	for _, provider := range receipt.Providers {
		if _, used := usedProviders[provider.ID]; !used {
			return fmt.Errorf(
				"pinned Agent Preset %q contains unused provider plugin %q version %q",
				preset.ID, provider.ID, provider.ImplementationVersion,
			)
		}
	}
	if !reflect.DeepEqual(receipt.Instructions, preset.Instructions) ||
		!reflect.DeepEqual(receipt.Configuration, preset.Configuration) {
		return fmt.Errorf("pinned Agent Preset %q instructions or configuration do not match version %q", preset.ID, preset.Version)
	}
	return nil
}

func validatePinnedToolSchemas(pinned, loaded []ToolSchemaReceipt) error {
	shared := len(pinned)
	if len(loaded) < shared {
		shared = len(loaded)
	}
	for i := 0; i < shared; i++ {
		if pinned[i] != loaded[i] {
			return fmt.Errorf(
				"pinned tool schema %q hash %s does not match loaded schema %q hash %s at position %d",
				pinned[i].Name, pinned[i].SHA256, loaded[i].Name, loaded[i].SHA256, i,
			)
		}
	}
	if len(pinned) > shared {
		return fmt.Errorf("pinned tool schema %q is missing from the loaded composition", pinned[shared].Name)
	}
	if len(loaded) > shared {
		return fmt.Errorf("loaded tool schema %q is absent from the pinned composition", loaded[shared].Name)
	}
	return nil
}

func activeCapability(entry *compiledPlugin, id CapabilityID) (ToolCapability, bool) {
	for _, capability := range entry.activeCapabilities {
		if capability.ID == id {
			return capability, true
		}
	}
	return ToolCapability{}, false
}

func resumableCapability(entry *compiledPlugin, version string, id CapabilityID) (ToolCapability, bool) {
	for _, capability := range entry.resumableCapabilities[version] {
		if capability.ID == id {
			return capability, true
		}
	}
	return ToolCapability{}, false
}

func compatibleImplementation(manifest Manifest, version string) (ImplementationCompatibility, bool) {
	for _, declaration := range manifest.ResumableFrom {
		if declaration.ImplementationVersion == version {
			return declaration, true
		}
	}
	return ImplementationCompatibility{}, false
}
