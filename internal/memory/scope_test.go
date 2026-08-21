package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalProjectRootResolvesDirectoryAndSymlink(t *testing.T) {
	root := t.TempDir()
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize expected root: %v", err)
	}

	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatalf("create project symlink: %v", err)
	}

	for _, path := range []string{root, alias} {
		got, err := CanonicalProjectRoot(path)
		if err != nil {
			t.Fatalf("CanonicalProjectRoot(%q): %v", path, err)
		}
		if got != want {
			t.Errorf("CanonicalProjectRoot(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCanonicalProjectRootRejectsInvalidPaths(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatalf("create regular file: %v", err)
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "empty", path: ""},
		{name: "missing", path: filepath.Join(t.TempDir(), "missing")},
		{name: "regular file", path: file},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := CanonicalProjectRoot(tt.path); err == nil {
				t.Fatalf("CanonicalProjectRoot(%q) = %q, want an error", tt.path, got)
			}
		})
	}
}
