package agent

import (
	"context"

	"github.com/davidadel66/evie/internal/skill"
)

// skillsProvider is implemented by durable histories that can resolve the
// skills their bound session pinned when it was created (skills.spec.md).
type skillsProvider interface {
	SessionSkills(context.Context) ([]skill.Pinned, error)
}

// skillCatalog renders the session's skill catalog message: its pinned
// model-invocable skills' names and descriptions. The pins and the
// approvals they name are immutable, so the catalog is the same for the
// chat's whole life and stays in the cached prefix. Delegated workers start
// without skills.
func (s *Session) skillCatalog(ctx context.Context) (string, error) {
	if s.workerInstructions != "" {
		return "", nil
	}
	provider, ok := s.history.(skillsProvider)
	if !ok {
		return "", nil
	}
	pinned, err := provider.SessionSkills(ctx)
	if err != nil {
		return "", err
	}
	return skill.Catalog(pinned), nil
}
