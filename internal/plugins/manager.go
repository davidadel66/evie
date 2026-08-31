// Package plugins contains the Kernel-owned composition seam for compiled
// first-party plugins.
package plugins

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/davidadel66/evie/internal/tools"
)

const KernelAPIVersion = "1.0.0"

type PluginID string

type CapabilityID string

// VersionRange is a [Minimum, MaximumExclusive) compatibility interval. This
// phase accepts stable MAJOR.MINOR.PATCH versions without prerelease or build
// suffixes; later dependency lifecycle work may introduce richer constraints.
type VersionRange struct {
	Minimum          string
	MaximumExclusive string
}

type CapabilityContract struct {
	ID      CapabilityID
	Version string
}

// Manifest is the stable declarative identity of one compiled plugin.
type Manifest struct {
	ID                    PluginID
	ImplementationVersion string
	KernelCompatibility   VersionRange
	Capabilities          []CapabilityContract
}

// Plugin is the smallest common compiled-plugin boundary needed by the
// Manager. Focused provider interfaces carry each family of contributions.
type Plugin interface {
	Manifest() Manifest
}

// ToolProvider is owned by the Kernel composition package that consumes tool
// contributions.
type ToolProvider interface {
	ToolCapabilities() []ToolCapability
}

type ToolCapability struct {
	ID              CapabilityID
	ContractVersion string
	Tool            tools.Tool
}

type compiledPlugin struct {
	manifest     Manifest
	capabilities []ToolCapability
}

// Manager validates the compiled catalog once and resolves a fresh immutable
// Toolset from the currently enabled plugins for each new session.
type Manager struct {
	mu      sync.RWMutex
	base    tools.Toolset
	plugins map[PluginID]compiledPlugin
	order   []PluginID
	enabled map[PluginID]bool
}

func NewManager(base tools.Toolset, compiled ...Plugin) (*Manager, error) {
	manager := &Manager{
		base:    base,
		plugins: make(map[PluginID]compiledPlugin, len(compiled)),
		enabled: make(map[PluginID]bool, len(compiled)),
	}

	manifests := make([]Manifest, len(compiled))
	for i, plugin := range compiled {
		manifest := cloneManifest(plugin.Manifest())
		if manifest.ID == "" {
			return nil, fmt.Errorf("plugin at compiled index %d has an empty ID", i)
		}
		if _, exists := manager.plugins[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate plugin ID %q", manifest.ID)
		}
		manager.plugins[manifest.ID] = compiledPlugin{manifest: manifest}
		manager.order = append(manager.order, manifest.ID)
		manifests[i] = manifest
	}

	capabilityOwner := make(map[CapabilityID]PluginID)
	for _, manifest := range manifests {
		for _, contract := range manifest.Capabilities {
			if owner, exists := capabilityOwner[contract.ID]; exists {
				return nil, fmt.Errorf(
					"duplicate Capability ID %q declared by plugins %q and %q",
					contract.ID, owner, manifest.ID,
				)
			}
			capabilityOwner[contract.ID] = manifest.ID
		}
	}

	for i, plugin := range compiled {
		manifest := manifests[i]
		if err := validateManifest(manifest); err != nil {
			return nil, err
		}
		provider, ok := plugin.(ToolProvider)
		if !ok {
			if len(manifest.Capabilities) != 0 {
				return nil, fmt.Errorf("plugin %q declares capabilities but implements no ToolProvider", manifest.ID)
			}
			continue
		}
		capabilities, err := snapshotToolCapabilities(manifest, provider.ToolCapabilities())
		if err != nil {
			return nil, err
		}
		entry := manager.plugins[manifest.ID]
		entry.capabilities = capabilities
		manager.plugins[manifest.ID] = entry
	}

	return manager, nil
}

func validateManifest(manifest Manifest) error {
	if !validProviderID(manifest.ID) {
		return fmt.Errorf("plugin %q has invalid ID", manifest.ID)
	}
	if strings.TrimSpace(manifest.ImplementationVersion) == "" {
		return fmt.Errorf("plugin %q has an empty implementation version", manifest.ID)
	}
	minimum, err := parseManifestVersion(manifest.KernelCompatibility.Minimum)
	if err != nil {
		return fmt.Errorf(
			"plugin %q has invalid Kernel compatibility minimum %q: %w",
			manifest.ID, manifest.KernelCompatibility.Minimum, err,
		)
	}
	maximum, err := parseManifestVersion(manifest.KernelCompatibility.MaximumExclusive)
	if err != nil {
		return fmt.Errorf(
			"plugin %q has invalid Kernel compatibility maximum %q: %w",
			manifest.ID, manifest.KernelCompatibility.MaximumExclusive, err,
		)
	}
	if minimum.compare(maximum) >= 0 {
		return fmt.Errorf(
			"plugin %q has invalid Kernel compatibility range [%s,%s)",
			manifest.ID,
			manifest.KernelCompatibility.Minimum,
			manifest.KernelCompatibility.MaximumExclusive,
		)
	}
	kernelVersion, err := parseManifestVersion(KernelAPIVersion)
	if err != nil {
		return fmt.Errorf("Kernel API version %q is invalid: %w", KernelAPIVersion, err)
	}
	if kernelVersion.compare(minimum) < 0 || kernelVersion.compare(maximum) >= 0 {
		return fmt.Errorf(
			"plugin %q is incompatible with Kernel API %s; supported range is [%s,%s)",
			manifest.ID,
			KernelAPIVersion,
			manifest.KernelCompatibility.Minimum,
			manifest.KernelCompatibility.MaximumExclusive,
		)
	}
	seen := make(map[CapabilityID]struct{}, len(manifest.Capabilities))
	for _, contract := range manifest.Capabilities {
		if !validCapabilityID(contract.ID, manifest.ID) {
			return fmt.Errorf(
				"Capability ID %q is not namespaced by plugin %q",
				contract.ID, manifest.ID,
			)
		}
		if strings.TrimSpace(contract.Version) == "" {
			return fmt.Errorf("Capability %q has an empty contract version", contract.ID)
		}
		if _, err := parseManifestVersion(contract.Version); err != nil {
			return fmt.Errorf(
				"Capability %q has invalid contract version %q: %w",
				contract.ID, contract.Version, err,
			)
		}
		if _, exists := seen[contract.ID]; exists {
			return fmt.Errorf("duplicate Capability ID %q in plugin %q", contract.ID, manifest.ID)
		}
		seen[contract.ID] = struct{}{}
	}
	return nil
}

func validProviderID(id PluginID) bool {
	return validIdentitySegment(string(id))
}

func validCapabilityID(id CapabilityID, provider PluginID) bool {
	value := string(id)
	if !validProviderID(provider) || !strings.HasPrefix(value, string(provider)+".") {
		return false
	}
	for _, segment := range strings.Split(value, ".") {
		if !validIdentitySegment(segment) {
			return false
		}
	}
	return true
}

func validIdentitySegment(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value[1:] {
		if (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

type manifestVersion struct {
	major uint64
	minor uint64
	patch uint64
}

func parseManifestVersion(value string) (manifestVersion, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return manifestVersion{}, fmt.Errorf("want MAJOR.MINOR.PATCH")
	}
	components := [3]uint64{}
	for i, part := range parts {
		if part == "" {
			return manifestVersion{}, fmt.Errorf("version component %d is empty", i+1)
		}
		if len(part) > 1 && part[0] == '0' {
			return manifestVersion{}, fmt.Errorf("version component %d has a leading zero", i+1)
		}
		for _, digit := range part {
			if digit < '0' || digit > '9' {
				return manifestVersion{}, fmt.Errorf("version component %d is not numeric", i+1)
			}
		}
		parsed, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return manifestVersion{}, fmt.Errorf("version component %d: %w", i+1, err)
		}
		components[i] = parsed
	}
	return manifestVersion{major: components[0], minor: components[1], patch: components[2]}, nil
}

func (v manifestVersion) compare(other manifestVersion) int {
	for i, component := range [...]uint64{v.major, v.minor, v.patch} {
		otherComponent := [...]uint64{other.major, other.minor, other.patch}[i]
		if component < otherComponent {
			return -1
		}
		if component > otherComponent {
			return 1
		}
	}
	return 0
}

func snapshotToolCapabilities(manifest Manifest, supplied []ToolCapability) ([]ToolCapability, error) {
	contracts := make(map[CapabilityID]string, len(manifest.Capabilities))
	for _, contract := range manifest.Capabilities {
		contracts[contract.ID] = contract.Version
	}
	capabilities := make([]ToolCapability, len(supplied))
	seen := make(map[CapabilityID]struct{}, len(supplied))
	for i, capability := range supplied {
		contractVersion, declared := contracts[capability.ID]
		if !declared {
			return nil, fmt.Errorf("plugin %q contributes undeclared Capability %q", manifest.ID, capability.ID)
		}
		if _, exists := seen[capability.ID]; exists {
			return nil, fmt.Errorf("plugin %q contributes Capability %q more than once", manifest.ID, capability.ID)
		}
		if capability.ContractVersion != contractVersion {
			return nil, fmt.Errorf(
				"plugin %q contributes Capability %q at contract version %q, manifest declares %q",
				manifest.ID, capability.ID, capability.ContractVersion, contractVersion,
			)
		}
		if capability.Tool.Schema.Function.Name == "" {
			return nil, fmt.Errorf("plugin %q contributes Capability %q without a tool schema name", manifest.ID, capability.ID)
		}
		if capability.Tool.Execute == nil && capability.Tool.Prepare == nil {
			return nil, fmt.Errorf("plugin %q contributes Capability %q without execution behavior", manifest.ID, capability.ID)
		}
		capability.Tool.Schema = tools.NewToolset([]tools.Tool{capability.Tool}).Schemas()[0]
		capabilities[i] = capability
		seen[capability.ID] = struct{}{}
	}
	if len(seen) != len(contracts) {
		for id := range contracts {
			if _, exists := seen[id]; !exists {
				return nil, fmt.Errorf("plugin %q declares Capability %q without a tool contribution", manifest.ID, id)
			}
		}
	}
	return capabilities, nil
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.Capabilities = append([]CapabilityContract(nil), manifest.Capabilities...)
	return manifest
}

// SetEnabled changes which compiled plugins are eligible for newly resolved
// session Toolsets. Toolsets already returned by NewSessionToolset are values
// and remain unchanged.
func (m *Manager) SetEnabled(id PluginID, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.plugins[id]; !exists {
		return fmt.Errorf("plugin %q is not compiled into Evie", id)
	}
	m.enabled[id] = enabled
	return nil
}

// NewSessionToolset resolves the current enabled catalog into a new immutable
// Toolset. Registration order never resolves duplicate model-facing schemas.
func (m *Manager) NewSessionToolset() (tools.Toolset, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	definitions := make([]tools.Tool, 0)
	schemaOwner := make(map[string]string)
	for _, schema := range m.base.Schemas() {
		schemaOwner[schema.Function.Name] = "Kernel"
	}
	for _, id := range m.order {
		if !m.enabled[id] {
			continue
		}
		plugin := m.plugins[id]
		for _, capability := range plugin.capabilities {
			name := capability.Tool.Schema.Function.Name
			if owner, exists := schemaOwner[name]; exists {
				return tools.Toolset{}, fmt.Errorf(
					"duplicate model-facing tool schema %q from %s and Capability %q",
					name, owner, capability.ID,
				)
			}
			schemaOwner[name] = fmt.Sprintf("Capability %q", capability.ID)
			definitions = append(definitions, capability.Tool)
		}
	}
	return m.base.WithTools(definitions), nil
}
