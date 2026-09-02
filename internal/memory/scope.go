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
		ID                WorkspaceID         `json:"id"`
		DisplayName       string              `json:"displayName"`
		State             WorkspaceState      `json:"state"`
		CurrentRevisionID WorkspaceRevisionID `json:"currentRevisionId"`
		CreatedAt         time.Time           `json:"createdAt"`
		UpdatedAt         time.Time           `json:"updatedAt"`
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
