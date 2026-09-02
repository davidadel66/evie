// Package plugins contains the Kernel-owned composition seam for compiled
// first-party plugins.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/tools"
)

const KernelAPIVersion = "1.0.0"

const defaultPluginCleanupTimeout = 5 * time.Second

var ErrEnabledStateUnavailable = errors.New("plugin enabled configuration is unavailable")

type PluginID string

type CapabilityID string

// VersionRange is a [Minimum, MaximumExclusive) compatibility interval. This
// phase accepts stable MAJOR.MINOR.PATCH versions without prerelease or build
// suffixes.
type VersionRange struct {
	Minimum          string
	MaximumExclusive string
}

type CapabilityContract struct {
	ID                   CapabilityID
	Version              string
	OptionalDependencies []PluginID
}

// CapabilityCompatibility is the exact prior contract and schema that the
// current implementation declares it can continue serving.
type CapabilityCompatibility struct {
	ID              CapabilityID
	ContractVersion string
	SchemaSHA256    string
}

// ImplementationCompatibility explicitly names one unavailable prior
// implementation that the current implementation may replace.
type ImplementationCompatibility struct {
	ImplementationVersion string
	Capabilities          []CapabilityCompatibility
}

// Dependency identifies one compiled plugin and the implementation versions
// with which the dependent plugin is compatible.
type Dependency struct {
	ID            PluginID
	Compatibility VersionRange
}

// Manifest is the stable declarative identity of one compiled plugin.
type Manifest struct {
	ID                    PluginID
	ImplementationVersion string
	KernelCompatibility   VersionRange
	RequiredDependencies  []Dependency
	OptionalDependencies  []Dependency
	Capabilities          []CapabilityContract
	ResumableFrom         []ImplementationCompatibility
}

// Plugin is the smallest common compiled-plugin boundary needed by the
// Manager. Focused provider interfaces carry each family of contributions.
type Plugin interface {
	Manifest() Manifest
	Start(context.Context) error
	Stop(context.Context) error
}

// ToolProvider is owned by the Kernel composition package that consumes tool
// contributions.
type ToolProvider interface {
	ToolCapabilities() []ToolCapability
}

// ResumableToolProvider supplies frozen contract/schema variants for prior
// implementation receipts while retaining the current provider's execution.
// These variants never participate in new composition.
type ResumableToolProvider interface {
	ResumableToolCapabilities(implementationVersion string) []ToolCapability
}

type ToolCapability struct {
	ID              CapabilityID
	ContractVersion string
	Tool            tools.Tool
}

type compiledPlugin struct {
	callbackMu            sync.Mutex
	plugin                Plugin
	manifest              Manifest
	capabilities          []ToolCapability
	resumableCapabilities map[string][]ToolCapability
	activeCapabilities    []ToolCapability
	enabled               bool
	manualStopped         bool
	started               bool
	cleanupPending        bool
	cleanupRetryAllowed   bool
	retryRequested        bool
	loadedOptional        map[PluginID]bool
	state                 LifecycleState
	diagnostic            string
	warnings              []string
	connectionReadiness   ConnectionReadiness
}

// LifecycleState is the complete externally observable Plugin Lifecycle.
type LifecycleState string

const (
	StateDisabled LifecycleState = "disabled"
	StateWaiting  LifecycleState = "waiting"
	StateLoading  LifecycleState = "loading"
	StateReady    LifecycleState = "ready"
	StateFailed   LifecycleState = "failed"
	StateStopping LifecycleState = "stopping"
	StateStopped  LifecycleState = "stopped"
)

// PluginHealth reports whether compiled code and its required dependencies
// work. Account and external-resource setup is deliberately represented by
// ConnectionReadiness instead.
type PluginHealth string

const (
	HealthHealthy   PluginHealth = "healthy"
	HealthDegraded  PluginHealth = "degraded"
	HealthUnhealthy PluginHealth = "unhealthy"
)

type ConnectionReadinessState string

const (
	ConnectionNotRequired ConnectionReadinessState = "not_required"
	ConnectionReady       ConnectionReadinessState = "ready"
	ConnectionNotReady    ConnectionReadinessState = "not_ready"
)

type ConnectionDiagnosticCode string

const ConnectionDiagnosticNotReady ConnectionDiagnosticCode = "connection_not_ready"

type PluginDiagnosticCode string

const (
	DiagnosticPluginFailed          PluginDiagnosticCode = "plugin_failed"
	DiagnosticCleanupPending        PluginDiagnosticCode = "cleanup_pending"
	DiagnosticDependencyUnavailable PluginDiagnosticCode = "dependency_unavailable"
)

// ConnectionReadiness is intentionally secret-free: providers may expose a
// state and repair diagnostic, never credentials or provider configuration.
type ConnectionReadiness struct {
	State      ConnectionReadinessState `json:"state"`
	Code       ConnectionDiagnosticCode `json:"code,omitempty"`
	Message    string                   `json:"message,omitempty"`
	Diagnostic string                   `json:"-"`
}

// ConnectionReadinessProvider is a focused inspection seam for plugins that
// need an account or external resource. It does not participate in lifecycle
// or preset validity.
type ConnectionReadinessProvider interface {
	ConnectionReadiness() ConnectionReadiness
}

type PluginStatus struct {
	ID                   PluginID             `json:"id"`
	Version              string               `json:"version"`
	Enabled              bool                 `json:"enabled"`
	State                LifecycleState       `json:"lifecycle"`
	RequiredDependencies []Dependency         `json:"requiredDependencies"`
	OptionalDependencies []Dependency         `json:"optionalDependencies"`
	Health               PluginHealth         `json:"health"`
	ConnectionReadiness  ConnectionReadiness  `json:"connectionReadiness"`
	DiagnosticCode       PluginDiagnosticCode `json:"diagnosticCode,omitempty"`
	Diagnostic           string               `json:"diagnostic,omitempty"`
	Warnings             []string             `json:"warnings"`
}

type Inspection struct {
	Degraded bool           `json:"degraded"`
	Plugins  []PluginStatus `json:"plugins"`
}

type LifecycleTransition struct {
	PluginID           PluginID       `json:"pluginId"`
	From               LifecycleState `json:"from"`
	To                 LifecycleState `json:"to"`
	Enabled            bool           `json:"enabled"`
	AffectedDependents []PluginStatus `json:"affectedDependents"`
}

// Manager validates the compiled catalog once and resolves a fresh immutable
// Toolset from the currently enabled plugins for each new session.
type Manager struct {
	gate             chan struct{}
	mu               sync.RWMutex
	base             tools.Toolset
	plugins          map[PluginID]*compiledPlugin
	order            []PluginID
	cleanupTimeout   time.Duration
	enabledStore     EnabledStateStore
	consumedRevision map[PluginID]uint64
}

// EnabledStateStore is the Kernel-owned durable desired-state boundary.
// Implementations must atomically seed an absent value without overwriting a
// concurrent or previously persisted owner choice.
type EnabledStateStore interface {
	ResolvePluginEnabled(context.Context, string, bool) (bool, uint64, error)
	PluginEnabled(context.Context, string) (bool, uint64, bool, error)
	SetPluginEnabled(context.Context, string, bool) (uint64, error)
}

func NewManager(base tools.Toolset, compiled ...Plugin) (*Manager, error) {
	manager := &Manager{
		gate:             make(chan struct{}, 1),
		base:             base,
		plugins:          make(map[PluginID]*compiledPlugin, len(compiled)),
		cleanupTimeout:   defaultPluginCleanupTimeout,
		consumedRevision: make(map[PluginID]uint64, len(compiled)),
	}
	manager.gate <- struct{}{}

	manifests := make([]Manifest, len(compiled))
	for i, plugin := range compiled {
		manifest := cloneManifest(plugin.Manifest())
		if manifest.ID == "" {
			return nil, fmt.Errorf("plugin at compiled index %d has an empty ID", i)
		}
		if _, exists := manager.plugins[manifest.ID]; exists {
			return nil, fmt.Errorf("duplicate plugin ID %q", manifest.ID)
		}
		manager.plugins[manifest.ID] = &compiledPlugin{
			plugin: plugin, manifest: manifest, state: StateDisabled,
			connectionReadiness: ConnectionReadiness{State: ConnectionNotRequired},
		}
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
		manager.plugins[manifest.ID].capabilities = capabilities
		resumable := make(map[string][]ToolCapability)
		if resumableProvider, ok := plugin.(ResumableToolProvider); ok {
			for _, declaration := range manifest.ResumableFrom {
				variants, err := snapshotResumableToolCapabilities(manifest, declaration, resumableProvider.ResumableToolCapabilities(declaration.ImplementationVersion))
				if err != nil {
					return nil, err
				}
				resumable[declaration.ImplementationVersion] = variants
			}
		}
		manager.plugins[manifest.ID].resumableCapabilities = resumable
		if err := validateImplementationCompatibility(manifest, capabilities, resumable); err != nil {
			return nil, err
		}
	}
	if err := manager.validateDependencies(); err != nil {
		return nil, err
	}
	order, err := manager.dependencyOrder()
	if err != nil {
		return nil, err
	}
	manager.order = order

	return manager, nil
}

func validateImplementationCompatibility(manifest Manifest, capabilities []ToolCapability, resumable map[string][]ToolCapability) error {
	current, err := parseManifestVersion(manifest.ImplementationVersion)
	if err != nil {
		return err
	}
	active := make(map[CapabilityID]ToolCapability, len(capabilities))
	for _, capability := range capabilities {
		active[capability.ID] = capability
	}
	seenVersions := make(map[string]struct{}, len(manifest.ResumableFrom))
	for _, declaration := range manifest.ResumableFrom {
		prior, err := parseManifestVersion(declaration.ImplementationVersion)
		if err != nil {
			return fmt.Errorf("plugin %q has invalid resumable implementation version %q: %w", manifest.ID, declaration.ImplementationVersion, err)
		}
		if prior.Compare(current) >= 0 {
			return fmt.Errorf(
				"plugin %q resumable implementation version %s is not older than replacement version %s",
				manifest.ID, declaration.ImplementationVersion, manifest.ImplementationVersion,
			)
		}
		if _, duplicate := seenVersions[declaration.ImplementationVersion]; duplicate {
			return fmt.Errorf("plugin %q repeats resumable implementation version %s", manifest.ID, declaration.ImplementationVersion)
		}
		seenVersions[declaration.ImplementationVersion] = struct{}{}
		available := active
		if variants, exists := resumable[declaration.ImplementationVersion]; exists {
			available = make(map[CapabilityID]ToolCapability, len(variants))
			for _, capability := range variants {
				available[capability.ID] = capability
			}
		}
		seenCapabilities := make(map[CapabilityID]struct{}, len(declaration.Capabilities))
		for _, evidence := range declaration.Capabilities {
			if _, duplicate := seenCapabilities[evidence.ID]; duplicate {
				return fmt.Errorf(
					"plugin %q compatibility for implementation %s repeats Capability %q",
					manifest.ID, declaration.ImplementationVersion, evidence.ID,
				)
			}
			seenCapabilities[evidence.ID] = struct{}{}
			capability, exists := available[evidence.ID]
			if !exists {
				return fmt.Errorf(
					"plugin %q compatibility for implementation %s names unavailable Capability %q",
					manifest.ID, declaration.ImplementationVersion, evidence.ID,
				)
			}
			if evidence.ContractVersion != capability.ContractVersion {
				return fmt.Errorf(
					"plugin %q compatibility for implementation %s declares Capability %q contract %s, loaded contract is %s",
					manifest.ID, declaration.ImplementationVersion, evidence.ID,
					evidence.ContractVersion, capability.ContractVersion,
				)
			}
			actualSchema := schemaHash(capability.Tool.Schema)
			if evidence.SchemaSHA256 != actualSchema {
				return fmt.Errorf(
					"plugin %q compatibility for implementation %s declares Capability %q schema %s, loaded schema is %s",
					manifest.ID, declaration.ImplementationVersion, evidence.ID,
					evidence.SchemaSHA256, actualSchema,
				)
			}
		}
	}
	return nil
}

func validateManifest(manifest Manifest) error {
	if !composition.ValidProviderID(string(manifest.ID)) {
		return fmt.Errorf("plugin %q has invalid ID", manifest.ID)
	}
	if strings.TrimSpace(manifest.ImplementationVersion) == "" {
		return fmt.Errorf("plugin %q has an empty implementation version", manifest.ID)
	}
	if _, err := parseManifestVersion(manifest.ImplementationVersion); err != nil {
		return fmt.Errorf("plugin %q has invalid implementation version %q: %w", manifest.ID, manifest.ImplementationVersion, err)
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
	if minimum.Compare(maximum) >= 0 {
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
	if kernelVersion.Compare(minimum) < 0 || kernelVersion.Compare(maximum) >= 0 {
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
		if !composition.ValidIdentity(string(contract.ID)) {
			return fmt.Errorf("Capability %q has invalid ID", contract.ID)
		}
		if !composition.ValidCapabilityID(string(contract.ID), string(manifest.ID)) {
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

func (m *Manager) validateDependencies() error {
	for _, id := range sortedPluginIDs(m.plugins) {
		entry := m.plugins[id]
		optional := make(map[PluginID]struct{}, len(entry.manifest.OptionalDependencies))
		seen := make(map[PluginID]string)
		for _, group := range []struct {
			kind string
			deps []Dependency
		}{
			{kind: "required", deps: entry.manifest.RequiredDependencies},
			{kind: "optional", deps: entry.manifest.OptionalDependencies},
		} {
			for _, dependency := range group.deps {
				if dependency.ID == "" {
					return fmt.Errorf("plugin %q has an empty %s dependency ID", id, group.kind)
				}
				if prior, exists := seen[dependency.ID]; exists {
					return fmt.Errorf("plugin %q declares dependency %q more than once (%s and %s)", id, dependency.ID, prior, group.kind)
				}
				seen[dependency.ID] = group.kind
				if group.kind == "optional" {
					optional[dependency.ID] = struct{}{}
				}
				minimum, maximum, err := parseVersionRange(dependency.Compatibility)
				if err != nil {
					return fmt.Errorf("plugin %q has invalid %s dependency %q compatibility: %w", id, group.kind, dependency.ID, err)
				}
				target, exists := m.plugins[dependency.ID]
				if !exists {
					continue
				}
				targetVersion, err := parseManifestVersion(target.manifest.ImplementationVersion)
				if err != nil {
					return fmt.Errorf("plugin %q dependency %q has invalid compiled implementation version %q: %w", id, dependency.ID, target.manifest.ImplementationVersion, err)
				}
				if targetVersion.Compare(minimum) < 0 || targetVersion.Compare(maximum) >= 0 {
					return fmt.Errorf(
						"plugin %q requires %s dependency %q in range [%s,%s), compiled version is %s",
						id, group.kind, dependency.ID,
						dependency.Compatibility.Minimum, dependency.Compatibility.MaximumExclusive,
						target.manifest.ImplementationVersion,
					)
				}
			}
		}
		for _, capability := range entry.manifest.Capabilities {
			seenOptional := make(map[PluginID]struct{}, len(capability.OptionalDependencies))
			for _, dependencyID := range capability.OptionalDependencies {
				if _, declared := optional[dependencyID]; !declared {
					return fmt.Errorf("Capability %q references undeclared optional dependency %q", capability.ID, dependencyID)
				}
				if _, duplicate := seenOptional[dependencyID]; duplicate {
					return fmt.Errorf("Capability %q references optional dependency %q more than once", capability.ID, dependencyID)
				}
				seenOptional[dependencyID] = struct{}{}
			}
		}
	}
	return nil
}

func parseVersionRange(versionRange VersionRange) (composition.Version, composition.Version, error) {
	minimum, err := parseManifestVersion(versionRange.Minimum)
	if err != nil {
		return composition.Version{}, composition.Version{}, fmt.Errorf("invalid minimum %q: %w", versionRange.Minimum, err)
	}
	maximum, err := parseManifestVersion(versionRange.MaximumExclusive)
	if err != nil {
		return composition.Version{}, composition.Version{}, fmt.Errorf("invalid maximum %q: %w", versionRange.MaximumExclusive, err)
	}
	if minimum.Compare(maximum) >= 0 {
		return composition.Version{}, composition.Version{}, fmt.Errorf("invalid range [%s,%s)", versionRange.Minimum, versionRange.MaximumExclusive)
	}
	return minimum, maximum, nil
}

func sortedPluginIDs(plugins map[PluginID]*compiledPlugin) []PluginID {
	ids := make([]PluginID, 0, len(plugins))
	for id := range plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (m *Manager) dependencyOrder() ([]PluginID, error) {
	const (
		unvisited = iota
		visiting
		visited
	)
	marks := make(map[PluginID]int, len(m.plugins))
	stack := make([]PluginID, 0, len(m.plugins))
	order := make([]PluginID, 0, len(m.plugins))
	var visit func(PluginID) error
	visit = func(id PluginID) error {
		switch marks[id] {
		case visited:
			return nil
		case visiting:
			start := 0
			for i, stackID := range stack {
				if stackID == id {
					start = i
					break
				}
			}
			cycle := append(append([]PluginID(nil), stack[start:]...), id)
			parts := make([]string, len(cycle))
			for i, cycleID := range cycle {
				parts[i] = string(cycleID)
			}
			return fmt.Errorf("plugin dependency cycle: %s", strings.Join(parts, " -> "))
		}
		marks[id] = visiting
		stack = append(stack, id)
		entry := m.plugins[id]
		dependencies := make([]PluginID, 0, len(entry.manifest.RequiredDependencies)+len(entry.manifest.OptionalDependencies))
		for _, dependency := range entry.manifest.RequiredDependencies {
			if _, exists := m.plugins[dependency.ID]; exists {
				dependencies = append(dependencies, dependency.ID)
			}
		}
		for _, dependency := range entry.manifest.OptionalDependencies {
			if _, exists := m.plugins[dependency.ID]; exists {
				dependencies = append(dependencies, dependency.ID)
			}
		}
		sort.Slice(dependencies, func(i, j int) bool { return dependencies[i] < dependencies[j] })
		for _, dependencyID := range dependencies {
			if err := visit(dependencyID); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		marks[id] = visited
		order = append(order, id)
		return nil
	}
	for _, id := range sortedPluginIDs(m.plugins) {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return order, nil
}

func parseManifestVersion(value string) (composition.Version, error) {
	return composition.ParseVersion(value)
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

func snapshotResumableToolCapabilities(
	manifest Manifest,
	declaration ImplementationCompatibility,
	supplied []ToolCapability,
) ([]ToolCapability, error) {
	contracts := make(map[CapabilityID]CapabilityCompatibility, len(declaration.Capabilities))
	for _, evidence := range declaration.Capabilities {
		contracts[evidence.ID] = evidence
	}
	if len(supplied) != len(contracts) {
		return nil, fmt.Errorf(
			"plugin %q resumable implementation %s supplied %d tool variants, want %d",
			manifest.ID, declaration.ImplementationVersion, len(supplied), len(contracts),
		)
	}
	variants := make([]ToolCapability, len(supplied))
	seen := make(map[CapabilityID]struct{}, len(supplied))
	for i, capability := range supplied {
		evidence, exists := contracts[capability.ID]
		if !exists {
			return nil, fmt.Errorf(
				"plugin %q resumable implementation %s contributes undeclared Capability %q",
				manifest.ID, declaration.ImplementationVersion, capability.ID,
			)
		}
		if _, duplicate := seen[capability.ID]; duplicate {
			return nil, fmt.Errorf(
				"plugin %q resumable implementation %s repeats Capability %q",
				manifest.ID, declaration.ImplementationVersion, capability.ID,
			)
		}
		if capability.ContractVersion != evidence.ContractVersion || schemaHash(capability.Tool.Schema) != evidence.SchemaSHA256 {
			return nil, fmt.Errorf(
				"plugin %q resumable implementation %s Capability %q does not match declared contract/schema evidence",
				manifest.ID, declaration.ImplementationVersion, capability.ID,
			)
		}
		if capability.Tool.Schema.Function.Name == "" || (capability.Tool.Execute == nil && capability.Tool.Prepare == nil) {
			return nil, fmt.Errorf(
				"plugin %q resumable implementation %s Capability %q lacks schema or execution behavior",
				manifest.ID, declaration.ImplementationVersion, capability.ID,
			)
		}
		capability.Tool.Schema = tools.NewToolset([]tools.Tool{capability.Tool}).Schemas()[0]
		variants[i] = capability
		seen[capability.ID] = struct{}{}
	}
	return variants, nil
}

func cloneManifest(manifest Manifest) Manifest {
	manifest.RequiredDependencies = append([]Dependency(nil), manifest.RequiredDependencies...)
	manifest.OptionalDependencies = append([]Dependency(nil), manifest.OptionalDependencies...)
	manifest.Capabilities = append([]CapabilityContract(nil), manifest.Capabilities...)
	manifest.ResumableFrom = append([]ImplementationCompatibility(nil), manifest.ResumableFrom...)
	for i := range manifest.ResumableFrom {
		manifest.ResumableFrom[i].Capabilities = append(
			[]CapabilityCompatibility(nil), manifest.ResumableFrom[i].Capabilities...,
		)
	}
	for i := range manifest.Capabilities {
		manifest.Capabilities[i].OptionalDependencies = append(
			[]PluginID(nil), manifest.Capabilities[i].OptionalDependencies...,
		)
	}
	return manifest
}

// SetEnabled is the context-free compatibility entry point for callers that do
// not own a lifecycle context.
func (m *Manager) SetEnabled(id PluginID, enabled bool) error {
	_, err := m.SetEnabledWithTransition(context.Background(), id, enabled)
	return err
}

// ConfigureEnabledState attaches durable Kernel configuration and replays the
// exact desired state for every currently compiled plugin. Stale rows for
// removed plugins are intentionally ignored; unknown defaults are rejected so
// a misspelled compiled identity cannot silently create configuration.
func (m *Manager) ConfigureEnabledState(
	ctx context.Context,
	store EnabledStateStore,
	defaults map[PluginID]bool,
) error {
	if store == nil {
		return errors.New("plugin enabled-state store is required")
	}
	return m.withLifecycleOperation(ctx, func() error {
		for id := range defaults {
			if _, exists := m.plugins[id]; !exists {
				return fmt.Errorf("default plugin %q is not compiled into Evie", id)
			}
		}
		type desiredIntent struct {
			enabled  bool
			revision uint64
		}
		desired := make(map[PluginID]desiredIntent, len(m.plugins))
		for _, id := range sortedPluginIDs(m.plugins) {
			enabled, revision, err := store.ResolvePluginEnabled(ctx, string(id), defaults[id])
			if err != nil {
				return fmt.Errorf("load desired state for plugin %q", id)
			}
			desired[id] = desiredIntent{enabled: enabled, revision: revision}
		}
		m.mu.Lock()
		m.enabledStore = store
		for _, id := range sortedPluginIDs(m.plugins) {
			intent := desired[id]
			m.consumeEnabledIntentLocked(id, intent.enabled, intent.revision)
		}
		m.mu.Unlock()
		return m.reconcile(ctx, nil)
	})
}

// RefreshEnabledState reconciles this process with the latest committed owner
// choices. It is request-bound so every observable boundary can fail closed
// without a background watcher or stale session composition.
func (m *Manager) RefreshEnabledState(ctx context.Context) error {
	m.mu.RLock()
	configured := m.enabledStore != nil
	m.mu.RUnlock()
	if !configured {
		return nil
	}
	return m.withLifecycleOperation(ctx, func() error {
		return m.refreshEnabledStateInOperation(ctx)
	})
}

func (m *Manager) refreshEnabledStateInOperation(ctx context.Context) error {
	m.mu.RLock()
	store := m.enabledStore
	m.mu.RUnlock()
	if store == nil {
		return nil
	}
	type desiredIntent struct {
		enabled  bool
		revision uint64
	}
	desired := make(map[PluginID]desiredIntent, len(m.plugins))
	for _, id := range sortedPluginIDs(m.plugins) {
		enabled, revision, found, err := store.PluginEnabled(ctx, string(id))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return fmt.Errorf("%w for plugin %q", ErrEnabledStateUnavailable, id)
		}
		if !found {
			return fmt.Errorf("%w for plugin %q", ErrEnabledStateUnavailable, id)
		}
		desired[id] = desiredIntent{enabled: enabled, revision: revision}
	}
	changed := false
	m.mu.Lock()
	for _, id := range sortedPluginIDs(m.plugins) {
		intent := desired[id]
		if intent.revision < m.consumedRevision[id] {
			m.mu.Unlock()
			return fmt.Errorf("%w for plugin %q", ErrEnabledStateUnavailable, id)
		}
		if intent.revision > m.consumedRevision[id] {
			changed = m.consumeEnabledIntentLocked(id, intent.enabled, intent.revision) || changed
		}
	}
	m.mu.Unlock()
	if !changed {
		return nil
	}
	return m.reconcile(ctx, nil)
}

// Enable makes one compiled plugin eligible and reconciles every affected
// plugin in dependency order.
func (m *Manager) Enable(ctx context.Context, id PluginID) error {
	_, err := m.SetEnabledWithTransition(ctx, id, true)
	return err
}

// Disable removes one plugin from the eligible set and stops loaded dependents
// before their required dependency.
func (m *Manager) Disable(ctx context.Context, id PluginID) error {
	_, err := m.SetEnabledWithTransition(ctx, id, false)
	return err
}

// SetEnabledWithTransition atomically serializes a management request and
// reports both the target transition and each dependent whose observable
// lifecycle changed as a consequence.
func (m *Manager) SetEnabledWithTransition(
	ctx context.Context,
	id PluginID,
	enabled bool,
) (LifecycleTransition, error) {
	var transition LifecycleTransition
	err := m.withLifecycleOperation(ctx, func() error {
		if err := m.refreshEnabledStateInOperation(ctx); err != nil {
			return err
		}
		m.mu.RLock()
		dependentClosure := m.managementDependentClosureLocked(id)
		m.mu.RUnlock()
		before := m.inspectSnapshot()
		beforeByID := make(map[PluginID]PluginStatus, len(before.Plugins))
		for _, status := range before.Plugins {
			beforeByID[status.ID] = status
		}
		prior, exists := beforeByID[id]
		if !exists {
			return fmt.Errorf("plugin %q is not compiled into Evie", id)
		}
		m.mu.RLock()
		store := m.enabledStore
		m.mu.RUnlock()
		if store != nil {
			revision, err := store.SetPluginEnabled(ctx, string(id), enabled)
			if err != nil {
				return fmt.Errorf("persist desired state for plugin %q", id)
			}
			m.mu.Lock()
			m.consumeEnabledIntentLocked(id, enabled, revision)
			m.mu.Unlock()
		} else {
			m.mu.Lock()
			m.authorizeCleanupRetryClosureLocked(id)
			m.applyEnabledIntentLocked(id, enabled)
			m.mu.Unlock()
		}
		lifecycleActivity := make(map[PluginID]bool)
		if err := m.reconcile(ctx, lifecycleActivity); err != nil {
			return err
		}

		after := m.inspectSnapshot()
		m.mu.RLock()
		for dependent := range m.managementDependentClosureLocked(id) {
			dependentClosure[dependent] = true
		}
		m.mu.RUnlock()
		transition = LifecycleTransition{
			PluginID: id, From: prior.State, Enabled: enabled,
			AffectedDependents: []PluginStatus{},
		}
		for _, status := range after.Plugins {
			if status.ID == id {
				transition.To = status.State
				transition.Enabled = status.Enabled
				continue
			}
			if dependentClosure[status.ID] {
				if priorStatus, ok := beforeByID[status.ID]; ok &&
					(lifecycleActivity[status.ID] || managementStatusChanged(priorStatus, status)) {
					transition.AffectedDependents = append(transition.AffectedDependents, status)
				}
			}
		}
		return nil
	})
	return transition, err
}

// managementStatusChanged compares only lifecycle and composition evidence.
// Connection Readiness is cached separately from lifecycle/composition state,
// so it must not make a plugin an affected dependent.
func managementStatusChanged(before, after PluginStatus) bool {
	return before.Enabled != after.Enabled ||
		before.State != after.State ||
		before.Health != after.Health ||
		before.DiagnosticCode != after.DiagnosticCode ||
		before.Diagnostic != after.Diagnostic ||
		!stringSlicesEqual(before.Warnings, after.Warnings)
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// managementDependentClosureLocked includes transitive required dependents
// and optional dependents whose current composition actually loaded the target.
// It deliberately excludes unrelated plugins even if their readiness changes
// while a serialized lifecycle request is in flight.
func (m *Manager) managementDependentClosureLocked(target PluginID) map[PluginID]bool {
	closure := map[PluginID]bool{target: true}
	for changed := true; changed; {
		changed = false
		for _, id := range m.order {
			if closure[id] {
				continue
			}
			entry := m.plugins[id]
			for _, dependency := range entry.manifest.RequiredDependencies {
				if closure[dependency.ID] {
					closure[id] = true
					changed = true
					break
				}
			}
			if closure[id] {
				continue
			}
			for _, dependency := range entry.manifest.OptionalDependencies {
				if closure[dependency.ID] && entry.loadedOptional[dependency.ID] {
					closure[id] = true
					changed = true
					break
				}
			}
		}
	}
	delete(closure, target)
	return closure
}

func (m *Manager) applyEnabledIntentLocked(id PluginID, enabled bool) {
	entry := m.plugins[id]
	entry.enabled = enabled
	entry.manualStopped = false
	if enabled {
		if entry.cleanupPending {
			m.beginShutdownLocked(id)
		}
		if entry.state == StateFailed {
			entry.retryRequested = true
		}
		if entry.state == StateDisabled || entry.state == StateStopped ||
			(entry.state == StateFailed && !entry.cleanupPending) {
			entry.state = StateWaiting
			entry.diagnostic = ""
		}
		return
	}
	m.beginShutdownLocked(id)
}

// consumeEnabledIntentLocked applies one durable command generation at most
// once in this Manager. Cleanup-pending entries retry cleanup for every newer
// command revision regardless of whether its Boolean changed. A clean
// same-value enable retries only an ordinary failed start; ready and cleanly
// disabled same-value commands are consumed without work.
func (m *Manager) consumeEnabledIntentLocked(id PluginID, enabled bool, revision uint64) bool {
	entry := m.plugins[id]
	m.consumedRevision[id] = revision
	cleanupAuthorized := m.authorizeCleanupRetryClosureLocked(id)
	if entry.enabled != enabled {
		m.applyEnabledIntentLocked(id, enabled)
		return true
	}
	if entry.cleanupPending {
		m.applyEnabledIntentLocked(id, enabled)
		return true
	}
	if enabled && (entry.manualStopped || entry.state == StateStopped) {
		m.applyEnabledIntentLocked(id, true)
		return true
	}
	if enabled && entry.state == StateFailed && !entry.cleanupPending {
		m.applyEnabledIntentLocked(id, true)
		return true
	}
	return cleanupAuthorized
}

// Stop shuts down a compiled plugin without disabling it, allowing Start to
// restart the same in-process implementation later.
func (m *Manager) Stop(ctx context.Context, id PluginID) error {
	return m.withLifecycleOperation(ctx, func() error {
		m.mu.Lock()
		entry, exists := m.plugins[id]
		if !exists {
			m.mu.Unlock()
			return fmt.Errorf("plugin %q is not compiled into Evie", id)
		}
		entry.manualStopped = true
		m.beginShutdownLocked(id)
		m.mu.Unlock()
		return m.reconcile(ctx, nil)
	})
}

// Start enables or restarts one already-compiled plugin.
func (m *Manager) Start(ctx context.Context, id PluginID) error {
	return m.Enable(ctx, id)
}

func (m *Manager) withLifecycleOperation(ctx context.Context, operation func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.gate:
	}
	defer func() { m.gate <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}

// beginShutdownLocked closes capability visibility before any potentially
// blocking cleanup hook runs. Required dependents are always affected;
// optional dependents are affected only when their loaded composition recorded
// the provider as available.
func (m *Manager) beginShutdownLocked(id PluginID) {
	affected := m.shutdownClosureLocked(map[PluginID]bool{id: true})
	m.authorizeCleanupRetriesLocked(affected)
	m.markShutdownLocked(affected)
}

func (m *Manager) authorizeCleanupRetryClosureLocked(id PluginID) bool {
	affected := m.shutdownClosureLocked(map[PluginID]bool{id: true})
	return m.authorizeCleanupRetriesLocked(affected)
}

func (m *Manager) authorizeCleanupRetriesLocked(affected map[PluginID]bool) bool {
	authorized := false
	for pluginID := range affected {
		if m.plugins[pluginID].cleanupPending {
			m.plugins[pluginID].cleanupRetryAllowed = true
			authorized = true
		}
	}
	return authorized
}

func (m *Manager) shutdownClosureLocked(seeds map[PluginID]bool) map[PluginID]bool {
	affected := make(map[PluginID]bool, len(seeds))
	for id := range seeds {
		affected[id] = true
	}
	for changed := true; changed; {
		changed = false
		for pluginID, entry := range m.plugins {
			if affected[pluginID] || !entry.started {
				continue
			}
			for _, dependency := range entry.manifest.RequiredDependencies {
				if affected[dependency.ID] {
					affected[pluginID] = true
					changed = true
					break
				}
			}
			if affected[pluginID] {
				continue
			}
			for _, dependency := range entry.manifest.OptionalDependencies {
				if affected[dependency.ID] && entry.loadedOptional[dependency.ID] {
					affected[pluginID] = true
					changed = true
					break
				}
			}
		}
	}
	return affected
}

func (m *Manager) markShutdownLocked(affected map[PluginID]bool) {
	for pluginID := range affected {
		entry := m.plugins[pluginID]
		if entry.cleanupPending && !entry.cleanupRetryAllowed {
			continue
		}
		entry.activeCapabilities = nil
		if entry.started {
			entry.state = StateStopping
			entry.diagnostic = ""
		}
	}
}

func (m *Manager) planShutdownLocked(cleanupAttempted map[PluginID]bool) map[PluginID]bool {
	seeds := make(map[PluginID]bool)
	for id, entry := range m.plugins {
		pendingAlreadyAttempted := entry.cleanupPending && cleanupAttempted[id]
		if entry.started && !pendingAlreadyAttempted &&
			((entry.cleanupPending && entry.cleanupRetryAllowed) ||
				(!entry.cleanupPending && !m.shouldRemainLoadedLocked(entry))) {
			seeds[id] = true
		}
	}
	affected := m.shutdownClosureLocked(seeds)
	m.markShutdownLocked(affected)
	return affected
}

func (m *Manager) reconcile(ctx context.Context, lifecycleActivity map[PluginID]bool) error {
	cleanupAttempted := make(map[PluginID]bool)
	var operationErrors []error
	maxPasses := 4*len(m.order) + 1
	for pass := 0; ; pass++ {
		changed := false
		m.mu.Lock()
		shutdown := m.planShutdownLocked(cleanupAttempted)
		m.mu.Unlock()
		for i := len(m.order) - 1; i >= 0; i-- {
			id := m.order[i]
			if !shutdown[id] {
				continue
			}
			entry := m.plugins[id]
			m.mu.Lock()
			pendingAlreadyAttempted := entry.cleanupPending && cleanupAttempted[id]
			stop := entry.started && !pendingAlreadyAttempted &&
				(!entry.cleanupPending || entry.cleanupRetryAllowed)
			if stop && entry.cleanupPending {
				entry.cleanupRetryAllowed = false
			}
			m.mu.Unlock()
			if stop {
				if lifecycleActivity != nil {
					lifecycleActivity[id] = true
				}
				cleanupAttempted[id] = true
				if err := m.stopPlugin(ctx, entry); err != nil {
					operationErrors = append(operationErrors, err)
					continue
				}
				changed = true
			}
		}
		m.normalizeInactiveStates()

		if ctx.Err() == nil {
		startPlugins:
			for _, id := range m.order {
				entry := m.plugins[id]
				m.mu.Lock()
				if !entry.enabled {
					if !entry.started {
						entry.state = StateDisabled
						entry.diagnostic = ""
						entry.warnings = nil
					}
					m.mu.Unlock()
					continue
				}
				if entry.manualStopped {
					if !entry.started {
						entry.state = StateStopped
						entry.diagnostic = ""
					}
					m.mu.Unlock()
					continue
				}
				if entry.started || (entry.state == StateFailed && !entry.retryRequested) {
					m.mu.Unlock()
					continue
				}
				diagnostic := m.requiredDependencyDiagnosticLocked(entry)
				if diagnostic != "" {
					entry.state = StateWaiting
					entry.diagnostic = diagnostic
					entry.warnings = nil
					m.mu.Unlock()
					continue
				}
				entry.retryRequested = false
				m.mu.Unlock()
				if lifecycleActivity != nil {
					lifecycleActivity[id] = true
				}
				if err := m.startPlugin(ctx, entry); err != nil {
					m.mu.RLock()
					if entry.cleanupPending {
						cleanupAttempted[id] = true
					}
					m.mu.RUnlock()
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						operationErrors = append(operationErrors, err)
						break startPlugins
					}
					continue
				}
				changed = true
			}
		}
		m.normalizeInactiveStates()
		if !changed || pass+1 >= maxPasses {
			if changed {
				operationErrors = append(operationErrors, fmt.Errorf(
					"plugin lifecycle reconciliation did not stabilize after %d passes", maxPasses,
				))
			}
			m.refreshReadyWarnings()
			if len(operationErrors) != 0 {
				return errors.Join(operationErrors...)
			}
			return nil
		}
	}
}

func (m *Manager) normalizeInactiveStates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.plugins {
		if !entry.started {
			switch {
			case !entry.enabled:
				entry.state = StateDisabled
				entry.diagnostic = ""
				entry.warnings = nil
			case entry.manualStopped:
				entry.state = StateStopped
				entry.diagnostic = ""
			}
		}
	}
}

func (m *Manager) refreshReadyWarnings() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.plugins {
		if entry.state == StateReady {
			_, entry.warnings = m.optionalDependenciesLocked(entry)
		}
	}
}

func (m *Manager) shouldRemainLoadedLocked(entry *compiledPlugin) bool {
	if !entry.enabled || entry.manualStopped {
		return false
	}
	if !m.requiredDependenciesWillRemainReadyLocked(entry, make(map[PluginID]bool)) {
		return false
	}
	for _, dependency := range entry.manifest.OptionalDependencies {
		available := false
		if target, exists := m.plugins[dependency.ID]; exists {
			available = target.enabled && !target.manualStopped && target.state == StateReady &&
				m.requiredDependenciesWillRemainReadyLocked(target, make(map[PluginID]bool))
		}
		if entry.loadedOptional[dependency.ID] != available {
			return false
		}
	}
	return true
}

func (m *Manager) requiredDependenciesWillRemainReadyLocked(entry *compiledPlugin, visiting map[PluginID]bool) bool {
	if visiting[entry.manifest.ID] {
		return false
	}
	visiting[entry.manifest.ID] = true
	defer delete(visiting, entry.manifest.ID)
	for _, dependency := range entry.manifest.RequiredDependencies {
		target, exists := m.plugins[dependency.ID]
		if !exists || !target.enabled || target.manualStopped || target.state != StateReady ||
			!m.requiredDependenciesWillRemainReadyLocked(target, visiting) {
			return false
		}
	}
	return true
}

func (m *Manager) requiredDependencyDiagnosticLocked(entry *compiledPlugin) string {
	for _, dependency := range entry.manifest.RequiredDependencies {
		target, exists := m.plugins[dependency.ID]
		if !exists {
			return fmt.Sprintf("required dependency %q is not compiled into Evie", dependency.ID)
		}
		if !target.enabled {
			return fmt.Sprintf("required dependency %q is disabled", dependency.ID)
		}
		if target.manualStopped || target.state == StateStopped {
			return fmt.Sprintf("required dependency %q is stopped", dependency.ID)
		}
		if target.state == StateFailed {
			return fmt.Sprintf("required dependency %q failed", dependency.ID)
		}
		if target.state != StateReady {
			return fmt.Sprintf("required dependency %q is %s", dependency.ID, target.state)
		}
	}
	return ""
}

func (m *Manager) startPlugin(ctx context.Context, entry *compiledPlugin) error {
	m.mu.Lock()
	entry.state = StateLoading
	entry.diagnostic = ""
	entry.warnings = nil
	entry.activeCapabilities = nil
	entry.started = true
	entry.cleanupPending = false
	entry.cleanupRetryAllowed = false
	m.mu.Unlock()

	entry.callbackMu.Lock()
	err := entry.plugin.Start(ctx)
	entry.callbackMu.Unlock()
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		cleanupErr := m.rollbackPlugin(ctx, entry)
		diagnostic := fmt.Sprintf("plugin %q failed to start", entry.manifest.ID)
		if cleanupErr != nil {
			diagnostic += "; cleanup is pending"
		}
		m.mu.Lock()
		entry.activeCapabilities = nil
		entry.loadedOptional = nil
		entry.state = StateFailed
		entry.diagnostic = diagnostic
		m.mu.Unlock()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if errors.Is(err, context.DeadlineExceeded) {
				return context.DeadlineExceeded
			}
			return context.Canceled
		}
		return fmt.Errorf("plugin %q failed to start", entry.manifest.ID)
	}

	readiness := m.readConnectionReadiness(entry)
	m.mu.Lock()
	optional, warnings := m.optionalDependenciesLocked(entry)
	active := make([]ToolCapability, 0, len(entry.capabilities))
	for _, capability := range entry.capabilities {
		contract := capabilityContract(entry.manifest, capability.ID)
		available := true
		for _, dependencyID := range contract.OptionalDependencies {
			if !optional[dependencyID] {
				available = false
				break
			}
		}
		if available {
			active = append(active, capability)
		}
	}
	entry.activeCapabilities = active
	entry.loadedOptional = optional
	entry.started = true
	entry.cleanupPending = false
	entry.cleanupRetryAllowed = false
	entry.state = StateReady
	entry.diagnostic = ""
	entry.warnings = warnings
	entry.connectionReadiness = readiness
	m.mu.Unlock()
	return nil
}

func (m *Manager) optionalDependenciesLocked(entry *compiledPlugin) (map[PluginID]bool, []string) {
	optional := make(map[PluginID]bool, len(entry.manifest.OptionalDependencies))
	warnings := make([]string, 0)
	for _, dependency := range entry.manifest.OptionalDependencies {
		available := false
		detail := "is not compiled into Evie"
		if target, exists := m.plugins[dependency.ID]; exists {
			available = target.enabled && !target.manualStopped && target.state == StateReady
			detail = fmt.Sprintf("is %s", target.state)
			if target.state == StateFailed {
				detail = "failed"
			}
		}
		optional[dependency.ID] = available
		if !available {
			warnings = append(warnings, fmt.Sprintf("optional dependency %q %s", dependency.ID, detail))
		}
	}
	return optional, warnings
}

func capabilityContract(manifest Manifest, id CapabilityID) CapabilityContract {
	for _, contract := range manifest.Capabilities {
		if contract.ID == id {
			return contract
		}
	}
	return CapabilityContract{}
}

func (m *Manager) stopPlugin(ctx context.Context, entry *compiledPlugin) error {
	m.mu.Lock()
	entry.state = StateStopping
	entry.activeCapabilities = nil
	m.mu.Unlock()

	cleanupCtx, cancel := m.boundedCleanupContext(ctx, false)
	entry.callbackMu.Lock()
	err := entry.plugin.Stop(cleanupCtx)
	entry.callbackMu.Unlock()
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.activeCapabilities = nil
	entry.loadedOptional = nil
	entry.warnings = nil
	if err != nil {
		entry.started = true
		entry.cleanupPending = true
		entry.cleanupRetryAllowed = false
		entry.state = StateFailed
		entry.diagnostic = fmt.Sprintf("plugin %q cleanup is pending", entry.manifest.ID)
		if errors.Is(err, context.Canceled) {
			return context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}
		return fmt.Errorf("plugin %q cleanup failed", entry.manifest.ID)
	}
	entry.started = false
	entry.cleanupPending = false
	entry.cleanupRetryAllowed = false
	entry.retryRequested = false
	entry.state = StateStopped
	entry.diagnostic = ""
	return nil
}

func (m *Manager) rollbackPlugin(ctx context.Context, entry *compiledPlugin) error {
	cleanupCtx, cancel := m.boundedCleanupContext(ctx, true)
	entry.callbackMu.Lock()
	err := entry.plugin.Stop(cleanupCtx)
	entry.callbackMu.Unlock()
	cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	entry.activeCapabilities = nil
	entry.loadedOptional = nil
	entry.warnings = nil
	if err != nil {
		entry.started = true
		entry.cleanupPending = true
		entry.cleanupRetryAllowed = false
		return err
	}
	entry.started = false
	entry.cleanupPending = false
	entry.cleanupRetryAllowed = false
	return nil
}

func (m *Manager) boundedCleanupContext(parent context.Context, detach bool) (context.Context, context.CancelFunc) {
	base := parent
	if detach {
		base = context.WithoutCancel(parent)
	}
	timeout := m.cleanupTimeout
	if timeout <= 0 {
		timeout = defaultPluginCleanupTimeout
	}
	return context.WithTimeout(base, timeout)
}

func (m *Manager) Status(id PluginID) (PluginStatus, error) {
	if err := m.RefreshEnabledState(context.Background()); err != nil {
		return PluginStatus{}, err
	}
	return m.statusCurrent(id)
}

func (m *Manager) statusCurrent(id PluginID) (PluginStatus, error) {
	m.mu.RLock()
	entry, exists := m.plugins[id]
	if !exists {
		m.mu.RUnlock()
		return PluginStatus{}, fmt.Errorf("plugin %q is not compiled into Evie", id)
	}
	status := statusSnapshot(entry)
	m.mu.RUnlock()
	return status, nil
}

func (m *Manager) Inspect() (Inspection, error) {
	return m.InspectContext(context.Background())
}

func (m *Manager) inspectCurrent() Inspection {
	return m.inspectSnapshot()
}

// InspectContext refreshes durable configuration before producing a list.
func (m *Manager) InspectContext(ctx context.Context) (Inspection, error) {
	if err := m.RefreshEnabledState(ctx); err != nil {
		return Inspection{}, err
	}
	return m.inspectCurrent(), nil
}

func (m *Manager) inspectSnapshot() Inspection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inspection := Inspection{Plugins: make([]PluginStatus, 0, len(m.order))}
	for _, id := range m.order {
		entry := m.plugins[id]
		status := statusSnapshot(entry)
		inspection.Plugins = append(inspection.Plugins, status)
		if status.State == StateFailed || (status.Enabled && status.State == StateWaiting) {
			inspection.Degraded = true
		}
	}
	return inspection
}

func statusSnapshot(entry *compiledPlugin) PluginStatus {
	health := HealthHealthy
	switch entry.state {
	case StateFailed:
		health = HealthUnhealthy
	case StateWaiting:
		if entry.enabled {
			health = HealthDegraded
		}
	}
	readiness := entry.connectionReadiness
	var diagnosticCode PluginDiagnosticCode
	if entry.state == StateFailed {
		diagnosticCode = DiagnosticPluginFailed
		if entry.cleanupPending {
			diagnosticCode = DiagnosticCleanupPending
		}
	} else if entry.state == StateWaiting && entry.enabled {
		diagnosticCode = DiagnosticDependencyUnavailable
	}
	return PluginStatus{
		ID: entry.manifest.ID, Version: entry.manifest.ImplementationVersion,
		Enabled: entry.enabled, State: entry.state,
		RequiredDependencies: append([]Dependency(nil), entry.manifest.RequiredDependencies...),
		OptionalDependencies: append([]Dependency(nil), entry.manifest.OptionalDependencies...),
		Health:               health, ConnectionReadiness: readiness, DiagnosticCode: diagnosticCode,
		Diagnostic: entry.diagnostic, Warnings: append([]string(nil), entry.warnings...),
	}
}

func (m *Manager) readConnectionReadiness(entry *compiledPlugin) ConnectionReadiness {
	provider, ok := entry.plugin.(ConnectionReadinessProvider)
	if !ok {
		return ConnectionReadiness{State: ConnectionNotRequired}
	}
	entry.callbackMu.Lock()
	provided := provider.ConnectionReadiness()
	entry.callbackMu.Unlock()
	readiness := ConnectionReadiness{State: ConnectionNotReady}
	switch provided.State {
	case ConnectionNotRequired, ConnectionReady:
		readiness.State = provided.State
	case ConnectionNotReady:
		readiness.State = ConnectionNotReady
	default:
		readiness.State = ConnectionNotReady
	}
	if readiness.State == ConnectionNotReady {
		readiness.Code = ConnectionDiagnosticNotReady
		readiness.Message = fmt.Sprintf("plugin %q requires connection setup", entry.manifest.ID)
	}
	return readiness
}

// NewSessionToolset resolves the current enabled catalog into a new immutable
// Toolset. Registration order never resolves duplicate model-facing schemas.
func (m *Manager) NewSessionToolset() (tools.Toolset, error) {
	if err := m.RefreshEnabledState(context.Background()); err != nil {
		return tools.Toolset{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	definitions := make([]tools.Tool, 0)
	schemaOwner := make(map[string]string)
	for _, schema := range m.base.Schemas() {
		schemaOwner[schema.Function.Name] = "Kernel"
	}
	for _, id := range m.order {
		plugin := m.plugins[id]
		if plugin.state != StateReady {
			continue
		}
		for _, capability := range plugin.activeCapabilities {
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
