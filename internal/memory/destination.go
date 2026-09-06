package memory

import "fmt"

// MemoryDestination describes applicability; concrete IDs are always derived
// from the immutable session rather than supplied by the model.
type MemoryDestination string

const (
	MemoryEverywhere MemoryDestination = "everywhere"
	MemoryWorkspace  MemoryDestination = "workspace"
	MemorySession    MemoryDestination = "session"
)

func ResolveMemoryDestination(scope ScopeContext, destination MemoryDestination, legacySession bool) (string, error) {
	contextKey := "global"
	if scope.WorkspaceID != "" {
		contextKey = "workspace:" + string(scope.WorkspaceID)
	} else if scope.ProjectID != "" {
		contextKey = "project:" + string(scope.ProjectID)
	}
	if legacySession && destination != "" && destination != MemorySession {
		return "", fmt.Errorf("conflicting memory destinations")
	}
	switch destination {
	case "":
		if legacySession {
			return "session:" + string(scope.SessionID), nil
		}
		return contextKey, nil
	case MemoryEverywhere:
		return "global", nil
	case MemoryWorkspace:
		if contextKey == "global" {
			return "", fmt.Errorf("this session has no workspace or project")
		}
		return contextKey, nil
	case MemorySession:
		if scope.SessionID == "" {
			return "", fmt.Errorf("session memory requires a bound session")
		}
		return "session:" + string(scope.SessionID), nil
	default:
		return "", fmt.Errorf("unsupported memory destination %q", destination)
	}
}
