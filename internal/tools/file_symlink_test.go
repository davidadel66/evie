package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// T4: edit_file through a symlink edits the file the link points at and
// leaves the link itself in place. Renaming the temp file over the link
// would swap the link for a regular file and leave the real file stale.
func TestEditFileThroughSymlinkEditsTargetAndKeepsLink(t *testing.T) {
	const before = "alpha\nbeta\ngamma\n"
	const after = "alpha\ndelta\ngamma\n"
	setup := func(t *testing.T) (target, link, linkText string) {
		t.Helper()
		targetDir, linkDir := t.TempDir(), t.TempDir()
		target = filepath.Join(targetDir, "real.md")
		if err := os.WriteFile(target, []byte(before), 0o640); err != nil {
			t.Fatal(err)
		}
		// A relative link from another directory: the temp file must be
		// written beside the target, not beside the link.
		linkText, err := filepath.Rel(linkDir, target)
		if err != nil {
			t.Fatal(err)
		}
		link = filepath.Join(linkDir, "link.md")
		if err := os.Symlink(linkText, link); err != nil {
			t.Fatal(err)
		}
		return target, link, linkText
	}
	check := func(t *testing.T, target, link, linkText string) {
		t.Helper()
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatalf("lstat link: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("edit replaced the symlink with a %v", info.Mode().Type())
		}
		if got, err := os.Readlink(link); err != nil || got != linkText {
			t.Fatalf("link now points at %q (%v), want %q", got, err, linkText)
		}
		data, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read target: %v", err)
		}
		if string(data) != after {
			t.Fatalf("target = %q, want %q", data, after)
		}
		if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("target permissions = %v (%v), want 640", info.Mode().Perm(), err)
		}
		for _, dir := range []string{filepath.Dir(target), filepath.Dir(link)} {
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".evie-") {
					t.Fatalf("leftover temp file %s in %s", entry.Name(), dir)
				}
			}
		}
	}
	args := func(path string) string {
		data, _ := json.Marshal(map[string]string{"path": path, "old_string": "beta", "new_string": "delta"})
		return string(data)
	}

	t.Run("approved preview", func(t *testing.T) {
		target, link, linkText := setup(t)
		prepared, err := prepareEditFileTool(context.Background(), args(link))
		if err != nil {
			t.Fatalf("prepare: %v", err)
		}
		// The approval preview names the file that will actually be written.
		resolvedTarget, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatal(err)
		}
		if p := prepared.Preview; p == nil || p.Path != resolvedTarget || p.OldText != before || p.NewText != after {
			t.Fatalf("preview = %+v, want path %s with the target's before/after", p, resolvedTarget)
		}
		if _, err := prepared.Execute(context.Background()); err != nil {
			t.Fatalf("execute: %v", err)
		}
		check(t, target, link, linkText)
	})

	t.Run("direct edit", func(t *testing.T) {
		target, link, linkText := setup(t)
		got, err := editFile(context.Background(), args(link))
		if err != nil {
			t.Fatalf("editFile: %v", err)
		}
		if !strings.Contains(got, "line 2") {
			t.Errorf("result %q does not report line 2", got)
		}
		check(t, target, link, linkText)
	})
}
