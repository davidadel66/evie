package main

import (
	"context"
	"fmt"
	"slices"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/skill"
)

// activeSkillReader lists the active version of every skill name.
type activeSkillReader interface {
	ActiveSkills(context.Context) ([]eviedb.SkillActivation, error)
}

// pinActiveSkills adds every active Global skill to a new chat's
// composition, as an instruction reference skill.global.<name> with its
// version hash (skills.spec.md, Visibility and pinning), when the
// composition has the skill capability. It reads the activations just
// before the chat's creation transaction, which writes the receipt with the
// chat; approvals and deactivations after that never change it. A
// composition without the capability (another preset, or a disabled skills
// plugin) is returned unchanged.
func pinActiveSkills(ctx context.Context, store activeSkillReader, resolved plugins.ResolvedComposition) (plugins.ResolvedComposition, error) {
	if !slices.ContainsFunc(resolved.Receipt.Capabilities, func(c composition.Capability) bool {
		return c.ID == string(plugins.SkillCapabilityID)
	}) {
		return resolved, nil
	}
	active, err := store.ActiveSkills(ctx)
	if err != nil {
		return plugins.ResolvedComposition{}, fmt.Errorf("read active skills: %w", err)
	}
	if len(active) == 0 {
		return resolved, nil
	}
	resolved.Receipt = composition.Clone(resolved.Receipt)
	for _, a := range active {
		resolved.Receipt.Instructions = append(resolved.Receipt.Instructions,
			composition.InstructionReference{ID: skill.GlobalReference(a.Name), SHA256: a.VersionHash})
	}
	if err := composition.Validate(resolved.Receipt); err != nil {
		return plugins.ResolvedComposition{}, fmt.Errorf("pin active skills: %w", err)
	}
	return resolved, nil
}
