package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type (
	ProjectID     string
	OwnerID       string
	SessionID     string
	SessionStatus string
	Project       struct {
		ID            ProjectID
		DisplayName   string
		CanonicalRoot string
		Archived      bool
		CreatedAt     time.Time
		UpdatedAt     time.Time
	}
	Session struct {
		ID                  SessionID
		ProjectID           ProjectID
		ProjectRootSnapshot string
		ParentSessionID     SessionID
		Status              SessionStatus
		CreatedAt           time.Time
		UpdatedAt           time.Time
	}
	ScopeContext struct {
		OwnerID         OwnerID
		SessionID       SessionID
		ProjectID       ProjectID
		ProjectRoot     string
		ParentSessionID SessionID
	}
)

func (s Session) ScopeContext() ScopeContext {
	return ScopeContext{
		OwnerID:         LocalOwnerID,
		SessionID:       s.ID,
		ProjectID:       s.ProjectID,
		ProjectRoot:     s.ProjectRootSnapshot,
		ParentSessionID: s.ParentSessionID,
	}
}

const (
	LocalOwnerID  OwnerID       = "local"
	SessionActive SessionStatus = "active"
	SessionClosed SessionStatus = "closed"
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
