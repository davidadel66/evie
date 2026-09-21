package eviedb

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/davidadel66/evie/internal/memory"
)

var ErrWorkspaceFolderInvalid = errors.New("Choose an existing absolute folder, or a new folder inside an existing parent")

// Creation never makes intermediate parents or overwrites an existing entry.
// Cleanup removes only the same newly created, still-empty directory.
func prepareWorkspaceFolder(path string, create bool) (memory.WorkspaceFolder, func(bool), error) {
	noop := func(bool) {}
	if path == "" && !create {
		return memory.WorkspaceFolder{}, noop, nil
	}
	if !filepath.IsAbs(path) {
		return memory.WorkspaceFolder{}, noop, ErrWorkspaceFolderInvalid
	}
	if !create {
		canonical, err := memory.CanonicalProjectRoot(path)
		if err != nil {
			return memory.WorkspaceFolder{}, noop, fmt.Errorf("%w: %v", ErrWorkspaceFolderInvalid, err)
		}
		return memory.WorkspaceFolder{Path: canonical, Revision: 1}, noop, nil
	}
	path = filepath.Clean(path)
	parentPath, err := memory.CanonicalProjectRoot(filepath.Dir(path))
	if err != nil {
		return memory.WorkspaceFolder{}, noop, fmt.Errorf("%w: %v", ErrWorkspaceFolderInvalid, err)
	}
	parent, err := os.OpenRoot(parentPath)
	if err != nil {
		return memory.WorkspaceFolder{}, noop, fmt.Errorf("%w: %v", ErrWorkspaceFolderInvalid, err)
	}
	name := filepath.Base(path)
	if err := parent.Mkdir(name, 0755); err != nil {
		parent.Close()
		return memory.WorkspaceFolder{}, noop, fmt.Errorf("%w: %v", ErrWorkspaceFolderInvalid, err)
	}
	created, err := parent.Lstat(name)
	if err != nil {
		parent.Close()
		return memory.WorkspaceFolder{}, noop, fmt.Errorf("%w: %v", ErrWorkspaceFolderInvalid, err)
	}
	canonical := filepath.Join(parentPath, name)
	cleanup := func(committed bool) {
		defer parent.Close()
		if committed {
			return
		}
		current, err := parent.Lstat(name)
		if err == nil && os.SameFile(created, current) {
			_ = parent.Remove(name)
		}
	}
	return memory.WorkspaceFolder{Path: canonical, Revision: 1}, cleanup, nil
}
