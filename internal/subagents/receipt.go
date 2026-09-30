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
