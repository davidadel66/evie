package eviedb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/skill"
)

// Pinned returns the version an activation approved, as a chat pins it.
func (a SkillActivation) Pinned() skill.Pinned {
	return skill.Pinned{Name: a.Name, Hash: a.VersionHash, Commit: a.Commit, Path: a.Path, Manifest: a.Manifest, Metadata: a.Metadata}
}

// PinnedSkills resolves the skill references of a chat's Composition
// Receipt, in receipt order, to the approved versions they pin
// (skills.spec.md, Visibility and pinning). Each reference must be a Global
// skill reference whose name was approved at exactly that hash, active or
// not; anything else is an error, never a fallback to another version.
func (s *Store) PinnedSkills(ctx context.Context, receipt composition.Receipt) ([]skill.Pinned, error) {
	var out []skill.Pinned
	seen := map[string]bool{}
	for _, ref := range receipt.Instructions {
		if !strings.HasPrefix(ref.ID, skill.ReferencePrefix) {
			continue
		}
		name, ok := skill.GlobalReferenceName(ref.ID)
		if !ok {
			return nil, fmt.Errorf("pinned skill reference %q is not a Global skill", ref.ID)
		}
		if seen[name] {
			return nil, fmt.Errorf("skill %s is pinned twice", name)
		}
		seen[name] = true
		activation, err := s.SkillActivationByHash(ctx, name, ref.SHA256)
		if err != nil {
			return nil, fmt.Errorf("pinned skill: %w", err)
		}
		out = append(out, activation.Pinned())
	}
	return out, nil
}

// SessionSkills reads a session's Composition Receipt and resolves the
// skills it pins, owner-only ones included. A session without a receipt
// pins none.
func (s *Store) SessionSkills(ctx context.Context, id memory.SessionID) ([]skill.Pinned, error) {
	receipt, err := s.GetCompositionReceipt(ctx, id)
	if errors.Is(err, ErrCompositionReceiptNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.PinnedSkills(ctx, receipt)
}

// SessionSkills are the skills the bound session pinned.
func (h *SessionHistory) SessionSkills(ctx context.Context) ([]skill.Pinned, error) {
	return h.store.SessionSkills(ctx, h.sessionID)
}
