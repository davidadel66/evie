package repoinstructions

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

func TestRootInstructionsPrecedenceAndBoundaries(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	folder := memory.WorkspaceFolder{Path: root, Revision: 1}
	settings := memory.RepositoryInstructionSettings{Enabled: true}
	load := func() memory.RepositoryInstructionSnapshot { return Load("workspace", folder, settings) }
	if got := load(); got.Status != "missing" {
		t.Fatalf("missing=%+v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("fallback"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := load(); got.File != "CLAUDE.md" || got.Text != "fallback" || len(got.SHA256) != 64 {
		t.Fatalf("fallback=%+v", got)
	}
	agents := filepath.Join(root, "AGENTS.md")
	for _, value := range []string{"root instructions", "", strings.Repeat("x", MaxBytes)} {
		if err := os.WriteFile(agents, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		got := load()
		if got.Status != "loaded" || got.File != "AGENTS.md" || got.Text != value {
			t.Fatalf("priority/status=%s %s", got.Status, got.File)
		}
		if !strings.Contains(Render(got), "explicit request takes precedence") {
			t.Fatal("missing precedence contract")
		}
	}
	for _, value := range []string{strings.Repeat("x", MaxBytes+1), "bad\x00text", "\xff"} {
		os.WriteFile(agents, []byte(value), 0600)
		got := load()
		if got.Status != "error" || got.Text != "" || got.File != "AGENTS.md" {
			t.Fatal("invalid AGENTS fell back or was supplied")
		}
	}
	os.Remove(agents)
	if err := os.Symlink(filepath.Join(root, "CLAUDE.md"), agents); err != nil {
		t.Fatal(err)
	}
	if got := load(); got.Status != "error" {
		t.Fatal("followed instruction symlink")
	}
	os.Remove(agents)
	if err := syscall.Mkfifo(agents, 0600); err != nil {
		t.Fatal(err)
	}
	if got := load(); got.Status != "error" {
		t.Fatal("accepted special file")
	}
	settings.Enabled = false
	if got := load(); got.Status != "disabled" || got.Text != "" {
		t.Fatal("disabled read")
	}
	settings.Enabled = true
	folder.Path = filepath.Join(root, "gone")
	if got := load(); got.Status != "error" {
		t.Fatal("accepted missing root")
	}
}

func TestRootInstructionsRejectSymlinkedRootComponents(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(base, "directory")
	if err := os.MkdirAll(filepath.Join(directory, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	// The anchored opener must reject both a final and an ancestor symlink,
	// even if the path became a symlink after initial canonical validation.
	for _, path := range []string{link, filepath.Join(link, "nested")} {
		root, err := openRootWithoutSymlinks(path)
		if err == nil {
			root.Close()
			t.Fatalf("opened symlinked component: %s", path)
		}
	}
	root, err := openRootWithoutSymlinks(filepath.Join(directory, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	root.Close()
}
