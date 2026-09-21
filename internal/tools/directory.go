package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Directory belongs to one live agent session. The root can change between
// turns when its owner replaces the Workspace attachment; cd never leaks to
// another session. This is a working location, not a shell sandbox.
type Directory struct {
	mu        sync.Mutex
	root, cwd string
}

func (d *Directory) SetRoot(root string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.root != root {
		d.root, d.cwd = root, root
	}
}

func (d *Directory) Start(explicit string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.root != "" && (explicit == "" || !filepath.IsAbs(explicit)) {
		canonical, err := filepath.EvalSymlinks(d.root)
		if err != nil || canonical != d.root {
			return "", fmt.Errorf("Attached folder %q is unavailable or has moved; update it in the Workspace", d.root)
		}
	}
	dir := d.cwd
	if dir == "" {
		dir = d.root
	}
	if explicit != "" {
		expanded, err := expandHome(strings.TrimSpace(explicit))
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(expanded) {
			expanded = filepath.Join(dir, expanded)
		}
		dir = expanded
	}
	if dir == "" {
		return os.Getwd()
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("Working folder %q is unavailable; attach an existing folder or choose a working directory", dir)
	}
	return dir, nil
}

func (d *Directory) remember(pwdPath string) {
	data, err := os.ReadFile(pwdPath)
	if err != nil {
		return
	}
	if dir := strings.TrimSpace(string(data)); dir != "" {
		d.mu.Lock()
		d.cwd = dir
		d.mu.Unlock()
	}
}

func invocationDirectory(ctx context.Context) *Directory {
	invocation, _ := InvocationFromContext(ctx)
	return invocation.Directory
}

func resolveToolPath(ctx context.Context, path string) (string, error) {
	path = strings.TrimSpace(path)
	if d := invocationDirectory(ctx); d != nil && path != "" && !filepath.IsAbs(path) && !strings.HasPrefix(path, "~") {
		cwd, err := d.Start("")
		if err != nil {
			return "", err
		}
		path = filepath.Join(cwd, path)
	}
	return resolvePath(path)
}
