package localfiles_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/localfiles"
)

// changes.spec.md story 6a: a Change's diff is a comparison of two
// revisions of its Repository, from where the branch left its base: each
// file's status, line counts, blob and patch, with what review never shows
// (secrets such as .env) listed but withheld.
func TestCompareListsEachFileOfABranchWithItsPatch(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Test", "-c", "user.email=t@example.invalid"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("a.txt", "one\ntwo\nthree\n")
	write("gone.txt", "bye\n")
	write("docs/old name.md", "moved\nas is\n")
	git("add", ".")
	git("commit", "-qm", "first")
	git("switch", "-q", "-c", "evie/1a2b3c4d-fix")
	write("a.txt", "one\n2\nthree\nfour\n")
	write("new.txt", "new\n")
	write(".env", "TOKEN=secret\n")
	write("logo.png", "\x89PNG\x00\x01\x02")
	git("rm", "-q", "gone.txt")
	git("mv", "docs/old name.md", "docs/new name.md")
	git("add", ".")
	git("commit", "-qm", "fix")
	// The base moved after the branch left it: that is not the Change's.
	git("switch", "-q", "main")
	write("main-only.txt", "later\n")
	git("add", ".")
	git("commit", "-qm", "later")

	got, err := localfiles.Compare(context.Background(), root, "refs/heads/main", "refs/heads/evie/1a2b3c4d-fix")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]localfiles.ComparedFile{}
	for _, f := range got.Files {
		files[f.Path] = f
	}
	if len(files) != 6 || got.Truncated {
		t.Fatalf("files = %+v", got)
	}
	a := files["a.txt"]
	if a.Status != "modified" || a.Additions != 2 || a.Deletions != 1 || len(a.Blob) != 40 || !strings.HasPrefix(a.Patch, "@@ ") ||
		!strings.Contains(a.Patch, "-two\n+2\n") || !strings.Contains(a.Patch, "+four") {
		t.Fatalf("a.txt = %+v", a)
	}
	if f := files["new.txt"]; f.Status != "added" || f.Additions != 1 || !strings.Contains(f.Patch, "+new") {
		t.Fatalf("new.txt = %+v", f)
	}
	if f := files["gone.txt"]; f.Status != "deleted" || f.Deletions != 1 || f.Blob != "" || !strings.Contains(f.Patch, "-bye") {
		t.Fatalf("gone.txt = %+v", f)
	}
	if f := files["docs/new name.md"]; f.Status != "renamed" || f.OldPath != "docs/old name.md" || f.Additions != 0 || f.Deletions != 0 {
		t.Fatalf("renamed = %+v", f)
	}
	if f := files["logo.png"]; !f.Binary || f.Patch != "" {
		t.Fatalf("logo.png = %+v", f)
	}
	if f := files[".env"]; !f.Excluded || f.Patch != "" || f.Additions != 0 {
		t.Fatalf(".env = %+v", f)
	}
	if _, ok := files["main-only.txt"]; ok {
		t.Fatal("the base's later commit is in the Change")
	}
	for _, refs := range [][2]string{{"--output=/tmp/x", "HEAD"}, {"main", "-p"}, {"main\n", "HEAD"}, {"", "HEAD"}} {
		if _, err := localfiles.Compare(context.Background(), root, refs[0], refs[1]); err == nil {
			t.Fatalf("%q compared", refs)
		}
	}
}
