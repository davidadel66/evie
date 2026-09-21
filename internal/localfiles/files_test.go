package localfiles_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/localfiles"
)

func TestFolderBrowsingBoundsReadsAndFindsFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("notes.md", "# Learning\n")
	write(".env", "secret")
	write("binary", "a\x00b")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "other"), []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "other"), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	listing, err := localfiles.List(context.Background(), root, ".", "note")
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Path != "notes.md" {
		t.Fatalf("list=%+v, %v", listing, err)
	}
	file, err := localfiles.Read(root, "notes.md")
	if err != nil || file.Text != "# Learning\n" {
		t.Fatalf("read=%+v, %v", file, err)
	}
	for _, path := range []string{"../other", "/etc/passwd", ".env", "binary", "escape"} {
		if _, err := localfiles.Read(root, path); err == nil {
			t.Errorf("read allowed %q", path)
		}
	}
}

func TestDirectoryListingRejectsFIFOWithoutBlocking(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := localfiles.List(context.Background(), root, "pipe", ""); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as a directory")
		}
	case <-time.After(time.Second):
		// Release a regressed blocking open so the test itself does not leak.
		f, _ := os.OpenFile(fifo, os.O_RDWR|syscall.O_NONBLOCK, 0)
		if f != nil {
			defer f.Close()
		}
		t.Fatal("directory listing blocked on a FIFO")
	}
}

func TestGitReviewDoesNotExecuteConfiguredFilters(t *testing.T) {
	for _, filter := range []string{"clean", "process"} {
		t.Run(filter, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			run := func(args ...string) {
				t.Helper()
				out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
				if err != nil {
					t.Fatalf("git: %s %v", out, err)
				}
			}
			write := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			run("init", "-q")
			write("a.txt", "before\n")
			write(".gitattributes", "a.txt filter=marker\n")
			run("add", ".")
			run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "initial")
			run("config", "filter.marker."+filter, "touch filter-ran; cat")
			run("config", "filter.marker.required", "true")
			write("a.txt", "after\n")
			if _, err := localfiles.Review(context.Background(), root, "working", ""); err != nil {
				t.Fatal(err)
			}
			patch, err := localfiles.Diff(context.Background(), root, "a.txt", "working", "")
			if err != nil || !strings.Contains(patch.Text, "+after") {
				t.Fatalf("diff=%+v %v", patch, err)
			}
			if _, err := os.Stat(filepath.Join(root, "filter-ran")); !os.IsNotExist(err) {
				t.Fatalf("filter executed: %v", err)
			}
		})
	}
}
func TestReviewReportsWorkingStagedAndBranchChanges(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	run("init", "-q")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.invalid")
	path := filepath.Join(root, "a.txt")
	os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0600)
	run("add", ".")
	run("commit", "-qm", "initial")
	os.WriteFile(path, []byte("first\ntwo\nlast\n"), 0600)
	os.WriteFile(filepath.Join(root, "new.txt"), []byte("new\n"), 0600)
	review, err := localfiles.Review(context.Background(), root, "working", "")
	if err != nil || !review.Available || len(review.Files) != 2 {
		t.Fatalf("review=%+v %v", review, err)
	}
	diff, err := localfiles.Diff(context.Background(), root, "a.txt", "working", "")
	if err != nil || !strings.Contains(diff.Text, "+first") || !strings.Contains(diff.Text, "-three") {
		t.Fatalf("diff=%+v %v", diff, err)
	}
	run("add", "a.txt")
	review, err = localfiles.Review(context.Background(), root, "staged", "")
	if err != nil || len(review.Files) != 1 {
		t.Fatalf("staged=%+v %v", review, err)
	}
	run("commit", "-qm", "change")
	review, err = localfiles.Review(context.Background(), root, "branch", "HEAD~1")
	if err != nil || len(review.Files) != 1 {
		t.Fatalf("branch=%+v %v", review, err)
	}
}

func TestReviewSubfolderUsesLiteralPathsAndShowsStagedDeletion(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	run("init", "-q")
	run("config", "user.name", "Test")
	run("config", "user.email", "test@example.invalid")
	child := filepath.Join(root, "sub")
	os.Mkdir(child, 0700)
	os.WriteFile(filepath.Join(root, ".env"), []byte("PRIVATE=old\n"), 0600)
	os.WriteFile(filepath.Join(child, "a.txt"), []byte("before\n"), 0600)
	run("add", ".")
	run("commit", "-qm", "initial")
	os.WriteFile(filepath.Join(root, ".env"), []byte("PRIVATE=new\n"), 0600)
	os.WriteFile(filepath.Join(child, "a.txt"), []byte("after\n"), 0600)
	review, err := localfiles.Review(context.Background(), child, "working", "")
	if err != nil || len(review.Files) != 1 || review.Files[0].Path != "a.txt" {
		t.Fatalf("subfolder review=%+v %v", review, err)
	}
	patch, err := localfiles.Diff(context.Background(), child, "a.txt", "working", "")
	if err != nil || !strings.Contains(patch.Text, "+after") {
		t.Fatalf("subfolder diff=%+v %v", patch, err)
	}
	patch, _ = localfiles.Diff(context.Background(), child, ":(top,glob)**", "working", "")
	if strings.Contains(patch.Text, "PRIVATE") {
		t.Fatal("pathspec escaped attachment")
	}
	run("rm", "-f", "sub/a.txt")
	// Git removes an empty parent directory; the attached folder must exist.
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	patch, err = localfiles.Diff(context.Background(), child, "a.txt", "working", "")
	if err != nil || !strings.Contains(patch.Text, "-before") {
		t.Fatalf("deleted diff=%+v %v", patch, err)
	}
	for _, path := range []string{".", "sub"} {
		if patch, err := localfiles.Diff(context.Background(), root, path, "working", ""); err == nil || patch.Text != "" {
			t.Fatalf("directory %q selected: %+v %v", path, patch, err)
		}
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if patch, err := localfiles.Diff(context.Background(), root, "sub", "staged", ""); err == nil || patch.Text != "" {
		t.Fatalf("historical directory selected: %+v %v", patch, err)
	}
}

func TestWorkingDiffInNewRepositoryShowsCurrentContents(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("init: %s %v", out, err)
	}
	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", root, "add", "a.txt").CombinedOutput(); err != nil {
		t.Fatalf("add: %s %v", out, err)
	}
	if err := os.WriteFile(path, []byte("current\n"), 0600); err != nil {
		t.Fatal(err)
	}
	patch, err := localfiles.Diff(context.Background(), root, "a.txt", "working", "")
	if err != nil || !strings.Contains(patch.Text, "+current") || strings.Contains(patch.Text, "+staged") {
		t.Fatalf("new repository diff=%+v %v", patch, err)
	}
}
