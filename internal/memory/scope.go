package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type (
	WorkspaceID         string
	WorkspaceRevisionID string
	WorkspaceState      string
	ProjectID           string
	OwnerID             string
	SessionID           string
	SessionStatus       string
	Project             struct {
		ID            ProjectID `json:"id"`
		DisplayName   string    `json:"displayName"`
		CanonicalRoot string    `json:"canonicalRoot"`
		Archived      bool      `json:"archived"`
		CreatedAt     time.Time `json:"createdAt"`
		UpdatedAt     time.Time `json:"updatedAt"`
	}
	Workspace struct {
		DefaultPresetID   string                        `json:"defaultPresetId"`
		AllowedPresetIDs  []string                      `json:"allowedPresetIds"`
		Instructions      RepositoryInstructionSettings `json:"instructions"`
		Folder            WorkspaceFolder               `json:"folder"`
		Explore           WorkspaceExplore              `json:"explore"`
		Build             WorkspaceBuild                `json:"build"`
		ID                WorkspaceID                   `json:"id"`
		DisplayName       string                        `json:"displayName"`
		State             WorkspaceState                `json:"state"`
		CurrentRevisionID WorkspaceRevisionID           `json:"currentRevisionId"`
		CreatedAt         time.Time                     `json:"createdAt"`
		UpdatedAt         time.Time                     `json:"updatedAt"`
	}
	Session struct {
		ID                        SessionID           `json:"id"`
		WorkspaceID               WorkspaceID         `json:"workspaceId,omitempty"`
		WorkspaceRevisionSnapshot WorkspaceRevisionID `json:"workspaceRevisionSnapshot,omitempty"`
		ProjectID                 ProjectID           `json:"projectId,omitempty"`
		ProjectRootSnapshot       string              `json:"projectRootSnapshot,omitempty"`
		ParentSessionID           SessionID           `json:"parentSessionId,omitempty"`
		Title                     string              `json:"title"`
		Status                    SessionStatus       `json:"status"`
		CreatedAt                 time.Time           `json:"createdAt"`
		UpdatedAt                 time.Time           `json:"updatedAt"`
	}
	SessionListing struct {
		Session
		ActivityAt time.Time `json:"activityAt"`
	}
	ScopeContext struct {
		OwnerID           OwnerID
		SessionID         SessionID
		WorkspaceID       WorkspaceID
		WorkspaceRevision WorkspaceRevisionID
		ProjectID         ProjectID
		ProjectRoot       string
		ParentSessionID   SessionID
	}
)

// WorkspaceFolder is an operational working location, not a memory scope or
// a change to a session's pinned Workspace configuration.
type WorkspaceFolder struct {
	Path     string `json:"path"`
	Revision int64  `json:"revision"`
}

// WorkspaceExploreSetting is a Workspace revision's choice for read-only
// explore workers (2026-10-03): the default, which allows them exactly while
// the Workspace has a local folder, or the owner's explicit on or off.
type WorkspaceExploreSetting string

const (
	WorkspaceExploreDefault WorkspaceExploreSetting = "default"
	WorkspaceExploreOn      WorkspaceExploreSetting = "on"
	WorkspaceExploreOff     WorkspaceExploreSetting = "off"
)

// Valid reports whether s is one of the three settings.
func (s WorkspaceExploreSetting) Valid() bool {
	return s == WorkspaceExploreDefault || s == WorkspaceExploreOn || s == WorkspaceExploreOff
}

// WorkspaceExplore is the current revision's explore setting and whether it
// allows explore workers now, given the Workspace's current folder.
type WorkspaceExplore struct {
	Setting WorkspaceExploreSetting `json:"setting"`
	Allowed bool                    `json:"allowed"`
}

// WorkspaceBuildSetting is a Workspace revision's choice for build workers,
// which write code in their own git worktrees (Stage 11, 2026-10-03). Writing
// is a bigger grant than exploring, so the default is off: only the owner's
// explicit on allows them.
type WorkspaceBuildSetting string

const (
	WorkspaceBuildDefault WorkspaceBuildSetting = "default"
	WorkspaceBuildOn      WorkspaceBuildSetting = "on"
	WorkspaceBuildOff     WorkspaceBuildSetting = "off"
)

// Valid reports whether s is one of the three settings.
func (s WorkspaceBuildSetting) Valid() bool {
	return s == WorkspaceBuildDefault || s == WorkspaceBuildOn || s == WorkspaceBuildOff
}

// WorkspaceBuild is the current revision's build setting and whether it
// allows build workers.
type WorkspaceBuild struct {
	Setting WorkspaceBuildSetting `json:"setting"`
	Allowed bool                  `json:"allowed"`
}

func (s Session) ScopeContext() ScopeContext {
	return ScopeContext{
		OwnerID:           LocalOwnerID,
		SessionID:         s.ID,
		WorkspaceID:       s.WorkspaceID,
		WorkspaceRevision: s.WorkspaceRevisionSnapshot,
		ProjectID:         s.ProjectID,
		ProjectRoot:       s.ProjectRootSnapshot,
		ParentSessionID:   s.ParentSessionID,
	}
}

const (
	LocalOwnerID      OwnerID        = "local"
	SessionActive     SessionStatus  = "active"
	SessionClosed     SessionStatus  = "closed"
	WorkspaceActive   WorkspaceState = "active"
	WorkspaceArchived WorkspaceState = "archived"
)

func CanonicalProjectRoot(root string) (string, error) {
	if root == "" {
		return "", errors.New("project root must not be empty")
	}

	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("make project root absolute: %w", err)
	}

	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve project root %q: %w", absolute, err)
	}

	info, err := os.Stat(canonical)
	if err != nil {
		return "", fmt.Errorf("stat project root %q: %w", canonical, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project root %q is not a directory", canonical)
	}

	return canonical, nil
}
