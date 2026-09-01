package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

const (
	EvieVersion                    = "1.0.0"
	StandardPresetID      PresetID = "standard"
	StandardPresetVersion          = "sha256:41b87e45541e81e6a6e45b4cb5877db1d6fb7ab0ebb3cea5f4b24df5f77c2734"
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

type PresetIdentity = composition.PresetIdentity
type ProviderReceipt = composition.Provider
type CapabilityReceipt = composition.Capability
type ToolSchemaReceipt = composition.ToolSchema
type CompositionWarning = composition.Warning
type CompositionReceipt = composition.Receipt

type ResolvedComposition struct {
	Toolset  tools.Toolset
	Receipt  CompositionReceipt
	Warnings []string
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
		},
	}
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
	if id == "" {
		id = StandardPresetID
	}
	if id != StandardPresetID {
		return ResolvedComposition{}, fmt.Errorf("Agent Preset %q is not allowed; choose %q", id, StandardPresetID)
	}
	return m.resolvePreset(BuiltinStandardPreset())
}

func (m *Manager) resolvePreset(preset Preset) (ResolvedComposition, error) {
	if err := validatePreset(preset); err != nil {
		return ResolvedComposition{}, fmt.Errorf("Agent Preset %q is invalid: %w", preset.ID, err)
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
		if version.compare(minimum) < 0 || version.compare(maximum) >= 0 {
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

// ResumeComposition reconstructs the exact standard composition. Compatible
// replacement and cross-Evie-version resolution are intentionally deferred.
func (m *Manager) ResumeComposition(receipt CompositionReceipt) (ResolvedComposition, error) {
	if err := ValidateCompositionReceipt(receipt); err != nil {
		return ResolvedComposition{}, err
	}
	if PresetID(receipt.Preset.ID) != StandardPresetID {
		return ResolvedComposition{}, fmt.Errorf("pinned Agent Preset %q is not available", receipt.Preset.ID)
	}
	if receipt.Preset.Version != StandardPresetVersion {
		return ResolvedComposition{}, fmt.Errorf("pinned Agent Preset %q version %q is not available", receipt.Preset.ID, receipt.Preset.Version)
	}
	if receipt.EvieVersion != EvieVersion {
		return ResolvedComposition{}, fmt.Errorf("pinned Evie version %q is not available; current version is %q", receipt.EvieVersion, EvieVersion)
	}
	resolved, err := m.ResolvePreset(PresetID(receipt.Preset.ID))
	if err != nil {
		return ResolvedComposition{}, err
	}
	want, err := json.Marshal(receipt)
	if err != nil {
		return ResolvedComposition{}, err
	}
	got, err := json.Marshal(resolved.Receipt)
	if err != nil {
		return ResolvedComposition{}, err
	}
	if string(got) != string(want) {
		return ResolvedComposition{}, errors.New("pinned Composition Receipt does not match the exact loaded providers, contracts, schemas, and instructions")
	}
	return resolved, nil
}
