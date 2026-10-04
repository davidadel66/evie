package subagents

import (
	"fmt"
	"slices"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/delegation"
)

// researchReceipt inherits only the restricted template's required capabilities
// from the immutable parent. A new worker must not upgrade an older parent's
// authority merely because a newer provider implementation is installed.
func researchReceipt(template, parent composition.Receipt) (composition.Receipt, error) {
	result := template
	result.Capabilities = slices.Clone(template.Capabilities)
	result.Providers = slices.Clone(template.Providers)
	result.ToolSchemas = slices.Clone(template.ToolSchemas)
	for i, required := range result.Capabilities {
		found := slices.IndexFunc(parent.Capabilities, func(c composition.Capability) bool { return c.ID == required.ID && c.ProviderID == required.ProviderID })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks research capability %s", delegation.ErrAuthority, required.ID)
		}
		result.Capabilities[i] = parent.Capabilities[found]
	}
	for i, required := range result.Providers {
		found := slices.IndexFunc(parent.Providers, func(p composition.Provider) bool { return p.ID == required.ID })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks research provider", delegation.ErrAuthority)
		}
		result.Providers[i] = parent.Providers[found]
	}
	for i, required := range result.ToolSchemas {
		found := slices.IndexFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == required.Name })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks research tool", delegation.ErrAuthority)
		}
		result.ToolSchemas[i] = parent.ToolSchemas[found]
	}
	return result, nil
}

// buildReceipt is the build preset's receipt once the parent's ceiling
// allows it: its Task Capabilities, providers and their tool schemas are the
// parent's own pinned ones, never a newer installed version, and the parent
// must hold read_file, edit_file and bash, of which the build worker's Kernel
// tools are worktree-confined forms (Stage 11).
func buildReceipt(template, parent composition.Receipt) (composition.Receipt, error) {
	for _, name := range delegation.BuildParentToolNames {
		if !slices.ContainsFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == name }) {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks %s, so it cannot delegate build work", delegation.ErrAuthority, name)
		}
	}
	result := composition.Clone(template)
	kernel := map[string]bool{}
	for _, name := range delegation.BuildKernelToolNames {
		kernel[name] = true
	}
	for i, required := range result.Capabilities {
		found := slices.IndexFunc(parent.Capabilities, func(c composition.Capability) bool { return c.ID == required.ID && c.ProviderID == required.ProviderID })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks build capability %s", delegation.ErrAuthority, required.ID)
		}
		result.Capabilities[i] = parent.Capabilities[found]
	}
	for i, required := range result.Providers {
		found := slices.IndexFunc(parent.Providers, func(p composition.Provider) bool { return p.ID == required.ID })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks build provider %s", delegation.ErrAuthority, required.ID)
		}
		result.Providers[i] = parent.Providers[found]
	}
	for i, required := range result.ToolSchemas {
		if kernel[required.Name] {
			continue
		}
		found := slices.IndexFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == required.Name })
		if found < 0 {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks build tool %s", delegation.ErrAuthority, required.Name)
		}
		result.ToolSchemas[i] = parent.ToolSchemas[found]
	}
	return result, nil
}

// exploreReceipt is the explore preset's receipt once the parent's ceiling
// allows it: the parent must itself hold read_file, query_db and bash, of
// which the worker's tools are confined, read-only forms. The explore preset
// selects no plugin Capability, so nothing is inherited from the parent.
func exploreReceipt(template, parent composition.Receipt) (composition.Receipt, error) {
	for _, name := range delegation.ExploreToolNames {
		if !slices.ContainsFunc(parent.ToolSchemas, func(s composition.ToolSchema) bool { return s.Name == name }) {
			return composition.Receipt{}, fmt.Errorf("%w: parent lacks %s, so it cannot delegate explore work", delegation.ErrAuthority, name)
		}
	}
	return composition.Clone(template), nil
}
