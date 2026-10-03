package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// workerRootFixture is an explore worker's root with ordinary files, a
// secret file, a database, links that stay inside the root and links that
// escape it, plus a sibling folder outside the root.
func workerRootFixture(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "root")
	outside = filepath.Join(base, "outside")
	for _, dir := range []string{root, filepath.Join(root, "sub"), outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, "a.txt"):           "alpha\nneedle one\nomega\n",
		filepath.Join(root, "notes.txt"):       "notes\n",
		filepath.Join(root, "sub", "b.txt"):    "beta needle\n",
		filepath.Join(root, ".env"):            "TOKEN=secret\n",
		filepath.Join(root, "sub", ".env.dev"): "TOKEN=secret\n",
		filepath.Join(outside, "secret.txt"):   "outside secret\n",
		filepath.Join(base, "sibling.txt"):     "sibling\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		filepath.Join(root, "link-in"):      filepath.Join(root, "a.txt"),
		filepath.Join(root, "link-out"):     filepath.Join(outside, "secret.txt"),
		filepath.Join(root, "link-dir-out"): outside,
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "data.db"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, outside
}

// Stage 7 (2026-10-03): the explore worker's bash runs only commands that
// are provably read-only. Every rejected form is refused before anything
// runs; every allowed form is a read inside the worker's root.
func TestReadOnlyBashVetting(t *testing.T) {
	root, _ := workerRootFixture(t)
	allowed := []string{
		"ls",
		"ls -la",
		"ls -la sub",
		"ls sub/../sub",
		"ls -R",
		"cat a.txt",
		"cat a.txt sub/b.txt",
		"cat link-in",
		"cat 'a.txt'",
		`cat "a.txt"`,
		`cat a\.txt`,
		"cat " + filepath.Join(root, "a.txt"),
		"cat *.txt",
		"cat sub/*",
		"head -n 5 a.txt",
		"head -5 a.txt",
		"head -n5 a.txt",
		"tail -n 2 a.txt",
		"tail -n +2 a.txt",
		"wc -l a.txt sub/b.txt",
		"cat a.txt | head -n 1",
		"cat a.txt|wc -l",
		"cat a.txt | rg needle | wc -l",
		"rg -n TODO",
		"rg -n 'foo|bar' sub",
		"rg -e /api/v1 .",
		`rg "/api/v1"`,
		"rg 'x$'",
		"rg --files -g '*.go'",
		"rg -i -C 2 needle sub",
		"rg --hidden -uu needle",
		"rg -tgo needle",
		"rg --type=go --max-count=3 needle -- sub",
		"grep -rn needle .",
		"grep -E 'a|b' a.txt",
		"grep -c needle a.txt sub/b.txt",
		"grep --include='*.go' -rn needle .",
		"find . -name '*.go' -type f",
		"find sub -maxdepth 2 -print",
		`find . \( -name a -o -name b \) -print`,
		"find . ! -name '*.txt'",
		"find . -newer a.txt",
		"find . -type d -empty",
		"git log --oneline -n 5",
		"git log -5 --stat",
		"git log --since='2 weeks ago' --author=David --format='%h %s'",
		"git log -p -1 -- sub",
		"git diff",
		"git diff --cached --name-only",
		"git diff HEAD~1 HEAD --stat",
		"git diff -U1 --word-diff",
		"git show HEAD:a.txt",
		"git show --stat HEAD",
		"git status",
		"git status --porcelain -b",
		"git status -s -uno",
		"git blame -L 1,5 a.txt",
		"git ls-files -z",
		"git ls-files --others --exclude-standard",
		"git --no-pager log -p -1",
		"sqlite3 -readonly data.db 'SELECT * FROM t LIMIT 5'",
		`sqlite3 -readonly -json data.db "select count(*) from t"`,
		"sqlite3 -readonly -header -column data.db 'select 1'",
		"sqlite3 -readonly data.db .tables",
		"sqlite3 -readonly data.db '.schema t'",
		"pwd",
		"cd sub && ls",
		"cd sub; cat b.txt",
		"cd sub && cat ../a.txt",
		"cd",
		"ls && wc -l a.txt",
		"ls || ls sub",
		"ls; ls sub;",
	}
	for _, command := range allowed {
		t.Run("allowed/"+command, func(t *testing.T) {
			if _, err := planReadOnly(root, root, command); err != nil {
				t.Fatalf("vetted read-only command refused: %v", err)
			}
		})
	}
	rejected := []struct{ command, want string }{
		{"", "empty"},
		{"   ", "empty"},
		{"rm a.txt", `"rm" is not an allowed`},
		{"sudo ls", `"sudo" is not an allowed`},
		{"env ls", `"env" is not an allowed`},
		{"xargs cat", `"xargs" is not an allowed`},
		{"/bin/ls", "by name"},
		{"./ls", "by name"},
		{"touch new.txt", `"touch" is not an allowed`},
		{"echo hi", `"echo" is not an allowed`},
		// Option-embedded execution and writes in git.
		{"git -c core.pager=x log", `"-c"`},
		{"git -c core.fsmonitor=x status", `"-c"`},
		{"git -C .. log", `"-C"`},
		{"git --exec-path=/tmp log", `"--exec-path"`},
		{"git --git-dir=/tmp/x log", `"--git-dir"`},
		{"git --work-tree=/ status", `"--work-tree"`},
		{"git --config-env=core.pager=X log", `"--config-env"`},
		{"git log --output=/tmp/x", `"--output"`},
		{"git log --output /tmp/x", `"--output"`},
		{"git diff --output=x.patch", `"--output"`},
		{"git diff --ext-diff", `"--ext-diff"`},
		{"git diff --textconv", `"--textconv"`},
		{"git show --textconv HEAD:a.txt", `"--textconv"`},
		{"git log --show-signature", `"--show-signature"`},
		{"git diff --no-index /etc/passwd a.txt", `"--no-index"`},
		{"git diff -O/etc/passwd", `"-O"`},
		{"git blame --contents /etc/passwd a.txt", `"--contents"`},
		{"git blame --ignore-revs-file /etc/passwd a.txt", `"--ignore-revs-file"`},
		{"git push", `git "push" is not allowed`},
		{"git fetch --upload-pack=touch", `git "fetch" is not allowed`},
		{"git clone --upload-pack=touch x y", `git "clone" is not allowed`},
		{"git config core.pager x", `git "config" is not allowed`},
		{"git checkout -- a.txt", `git "checkout" is not allowed`},
		{"git commit -m x", `git "commit" is not allowed`},
		{"git", "subcommand"},
		{"git -p log", `"-p"`},
		// find's write and exec primaries.
		{"find . -exec rm '{}' ';'", `"-exec"`},
		{"find . -exec rm {} ;", "{"},
		{"find . -execdir ls ';'", `"-execdir"`},
		{"find . -ok rm '{}' ';'", `"-ok"`},
		{"find . -delete", `"-delete"`},
		{"find . -fprint out.txt", `"-fprint"`},
		{"find . -fls out.txt", `"-fls"`},
		{"find -L .", `"-L"`},
		{"find . -follow", `"-follow"`},
		{"find / -name x", "outside"},
		{"find . -newer /etc/passwd", "outside"},
		// Redirections, sequencing into writers, substitutions, assignments.
		{"cat a.txt > b.txt", "redirection"},
		{"cat a.txt >> b.txt", "redirection"},
		{"cat < a.txt", "redirection"},
		{"ls 2>&1", "redirection"},
		{"ls &>/dev/null", "&"},
		{"ls >|x", "redirection"},
		{"ls; rm x", `"rm" is not an allowed`},
		{"ls && rm -rf .", `"rm" is not an allowed`},
		{"ls&&rm x", `"rm" is not an allowed`},
		{"ls | xargs rm", `"xargs" is not an allowed`},
		{"cat a.txt | tee b.txt", `"tee" is not an allowed`},
		{"cat a.txt | sh", `"sh" is not an allowed`},
		{"head -n 5 a.txt | sort -o b.txt", `"sort" is not an allowed`},
		{"cat a.txt|rm b", `"rm" is not an allowed`},
		{"echo $(rm a.txt)", "$"},
		{"cat $(ls)", "$"},
		{`cat "$(ls)"`, "$"},
		{"cat `ls`", "backtick"},
		{"cat \"`ls`\"", "backtick"},
		{`cat "$HOME/.ssh/id_rsa"`, "$"},
		{"cat ${FOO}", "$"},
		{"cat $FOO", "$"},
		{"FOO=1 ls", "environment assignment"},
		{"PATH=/tmp ls", "environment assignment"},
		{"GIT_DIR=/tmp git log", "environment assignment"},
		// Paths outside the root, directly, by .., by ~ or through symlinks.
		{"cat ../outside/secret.txt", "outside"},
		{"cat ../sibling.txt", "outside"},
		{"cat sub/../../sibling.txt", "outside"},
		{"cat /etc/passwd", "outside"},
		{"cat link-out", "outside"},
		{"cat sub/../link-out", "outside"},
		{"ls link-dir-out/", "outside"},
		{"cat link-dir-out/secret.txt", "outside"},
		{"cat link-dir-out/*", "outside"},
		{"cat ~/.ssh/id_rsa", "~"},
		{"ls ~", "~"},
		{"cat ../*", "outside"},
		{"rg needle ..", "outside"},
		{"rg needle /etc", "outside"},
		{"rg -f /etc/passwd a.txt", "outside"},
		{"grep needle /etc/passwd", "outside"},
		{"grep -f ../sibling.txt a.txt", "outside"},
		{"wc -l ../sibling.txt", "outside"},
		{"head -n 1 /etc/hosts", "outside"},
		{"git blame -L 1,5 ../sibling.txt", "outside"},
		{"cat .env", "off-limits"},
		{"cat sub/.env.dev", "off-limits"},
		// cd stays inside the root and only leads the line.
		{"cd .. && ls", "outside"},
		{"cd /; ls", "outside"},
		{"cd link-dir-out && ls", "outside"},
		{"ls && cd sub", "cd"},
		{"cd sub | ls", "cd"},
		{"cd sub sub", "cd"},
		// Shell syntax that has no read-only meaning here.
		{"ls &", "&"},
		{"(ls)", "subshell"},
		{"{ ls; }", "{"},
		{"ls # comment", "comment"},
		{"ls\nrm a.txt", "newline"},
		{"ls |", "operator"},
		{"| ls", "operator"},
		{"ls ;; ls", "operator"},
		{"ls |& cat", "&"},
		{"cat 'unterminated", "unterminated"},
		{`cat "unterminated`, "unterminated"},
		{"cat a.txt\x00", "control"},
		// Flags that follow, follow forever or execute.
		{"ls -L", `"-L"`},
		{"tail -f a.txt", `"-f"`},
		{"tail --follow a.txt", `"--follow"`},
		{"rg --pre cat needle", `"--pre"`},
		{"rg --pre=cat needle", `"--pre"`},
		{"rg -z needle", `"-z"`},
		{"rg --search-zip needle", `"--search-zip"`},
		{"rg -L needle", `"-L"`},
		{"rg --follow needle", `"--follow"`},
		{"rg --hostname-bin=x needle", `"--hostname-bin"`},
		{"grep -R needle .", `"-R"`},
		{"grep -S -r needle .", `"-S"`},
		{"grep --dereference-recursive needle .", `"--dereference-recursive"`},
		{"wc --files0-from=list", `"--files0-from"`},
		{"cat --unknown a.txt", `"--unknown"`},
		// sqlite3 stays read-only and single-statement.
		{"sqlite3 data.db 'select 1'", "-readonly"},
		{"sqlite3 -readonly data.db 'delete from t'", "SELECT"},
		{"sqlite3 -readonly data.db 'select 1; drop table t'", "exactly one"},
		{"sqlite3 -readonly data.db '.shell rm a.txt'", "SELECT"},
		{"sqlite3 -readonly data.db '.output x'", "SELECT"},
		{"sqlite3 -readonly -cmd '.shell ls' data.db 'select 1'", `"-cmd"`},
		{"sqlite3 -readonly -init x.sql data.db 'select 1'", `"-init"`},
		{"sqlite3 -readonly -A data.db 'select 1'", `"-A"`},
		{"sqlite3 -readonly data.db \"attach 'x' as y\"", "ATTACH"},
		{"sqlite3 -readonly data.db \"select * from t; attach 'x' as y\"", "exactly one"},
		{"sqlite3 -readonly /etc/x.db 'select 1'", "outside"},
		{"sqlite3 -readonly 'file:data.db?mode=rw' 'select 1'", "file:"},
		{"sqlite3 -readonly missing.db 'select 1'", "missing.db"},
		{"sqlite3 -readonly data.db", "SQL"},
		{"sqlite3 -readonly data.db 'select 1' 'select 2'", "SQL"},
	}
	for _, c := range rejected {
		t.Run("rejected/"+c.command, func(t *testing.T) {
			_, err := planReadOnly(root, root, c.command)
			if err == nil {
				t.Fatal("command that is not provably read-only was accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("refusal %q does not name %q", err, c.want)
			}
		})
	}
}

func workerBash(t *testing.T, root string, params map[string]any) (string, error) {
	t.Helper()
	args, _ := json.Marshal(params)
	ctx := withWorkerRoot(context.Background(), root)
	return runReadOnlyBash(ctx, string(args))
}

func TestReadOnlyBashRunsVettedCommandsWithoutAShell(t *testing.T) {
	root, _ := workerRootFixture(t)
	for _, c := range []struct {
		command, cwd string
		want         []string
	}{
		{"cat a.txt | head -n 1", "", []string{"alpha\n", "exit status: 0"}},
		{"wc -l a.txt", "", []string{"3 a.txt"}},
		{"cd sub && cat b.txt", "", []string{"beta needle"}},
		{"cat b.txt", "sub", []string{"beta needle"}},
		{"pwd", "sub", []string{filepath.Join(root, "sub")}},
		{"cat *.txt", "", []string{"alpha", "notes"}},
		{"ls", "", []string{"a.txt", "notes.txt", "sub"}},
		{"cat missing.txt", "", []string{"exit status: 1"}},
		{"cat missing.txt || cat notes.txt", "", []string{"notes", "exit status: 0"}},
		{"cat missing.txt && cat notes.txt", "", []string{"exit status: 1"}},
		{"find . -name 'b.txt'", "", []string{"./sub/b.txt"}},
		{"grep -rn needle .", "", []string{"a.txt:2:needle one", "b.txt:1:beta needle"}},
	} {
		t.Run(c.command, func(t *testing.T) {
			params := map[string]any{"command": c.command}
			if c.cwd != "" {
				params["cwd"] = c.cwd
			}
			got, err := workerBash(t, root, params)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Fatalf("output %q lacks %q", got, want)
				}
			}
		})
	}
	// Secrets stay out of recursive searches even when the search could
	// reach them: grep -r gets the harness's exclusions.
	got, err := workerBash(t, root, map[string]any{"command": "grep -rn TOKEN ."})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "secret") {
		t.Fatalf("recursive grep read a secret file: %q", got)
	}
	// A pipeline whose reader stops early still ends cleanly.
	got, err = workerBash(t, root, map[string]any{"command": "cat a.txt a.txt a.txt | head -n 1"})
	if err != nil || !strings.Contains(got, "exit status: 0") {
		t.Fatalf("pipeline = %q, %v", got, err)
	}
}

// A refused line runs nothing: not even its allowed first command.
func TestReadOnlyBashRefusesBeforeRunningAnything(t *testing.T) {
	root, _ := workerRootFixture(t)
	for _, command := range []string{
		"cat a.txt && rm a.txt",
		"cat a.txt > copy.txt",
		"cat a.txt | tee copy.txt",
		"find . -name a.txt -delete",
		"git log --output=copy.txt",
		"ls; touch copy.txt",
	} {
		t.Run(command, func(t *testing.T) {
			out, err := workerBash(t, root, map[string]any{"command": command})
			if err == nil {
				t.Fatalf("write accepted: %q", out)
			}
			if !strings.Contains(err.Error(), "read-only") {
				t.Fatalf("refusal does not explain the read-only rule: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
				t.Fatal("a.txt was removed")
			}
			if _, err := os.Stat(filepath.Join(root, "copy.txt")); !os.IsNotExist(err) {
				t.Fatal("copy.txt was written")
			}
		})
	}
}

func TestReadOnlyBashConfinesItsWorkingDirectory(t *testing.T) {
	root, outside := workerRootFixture(t)
	for _, cwd := range []string{"..", outside, "link-dir-out", "/", "~"} {
		if out, err := workerBash(t, root, map[string]any{"command": "ls", "cwd": cwd}); err == nil {
			t.Fatalf("cwd %q outside the root accepted: %q", cwd, out)
		}
	}
	// Without a pinned root the tool refuses to run at all.
	args, _ := json.Marshal(map[string]any{"command": "ls"})
	if _, err := runReadOnlyBash(context.Background(), string(args)); err == nil || !strings.Contains(err.Error(), "explore worker") {
		t.Fatalf("unpinned worker bash = %v", err)
	}
	// A root that was swapped for a symlink after pinning is refused.
	moved := filepath.Join(filepath.Dir(root), "moved")
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, root); err != nil {
		t.Fatal(err)
	}
	if _, err := workerBash(t, root, map[string]any{"command": "ls"}); err == nil || !strings.Contains(err.Error(), "moved") {
		t.Fatalf("swapped root = %v", err)
	}
}

func TestReadOnlyBashCapsOutputWithoutSpilling(t *testing.T) {
	root, _ := workerRootFixture(t)
	big := strings.Repeat("0123456789\n", 6000)
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := workerBash(t, root, map[string]any{"command": "cat big.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > maxWorkerBashOutput+512 || !strings.Contains(got, "[output trimmed: 30000 of 66000 bytes shown") || strings.Contains(got, "saved to") {
		t.Fatalf("output not capped as stated: %d bytes, tail %q", len(got), got[len(got)-300:])
	}
}

func TestReadOnlyBashGitAndSQLiteStayReadOnly(t *testing.T) {
	root, outside := workerRootFixture(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run(root, "init", "-q")
	run(root, "add", "a.txt", "notes.txt")
	run(root, "commit", "-q", "-m", "first commit")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The repository's own config asks for a pager and a fsmonitor hook
	// that would run a command; neither runs.
	marker := filepath.Join(outside, "ran")
	hook := filepath.Join(outside, "hook.sh")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	run(root, "config", "core.fsmonitor", hook)
	run(root, "config", "core.pager", hook)
	// Control: an ordinary git status does run the configured hook.
	run(root, "status", "--porcelain")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control: the fsmonitor hook did not run without the harness: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	index, err := os.Stat(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ command, want string }{
		{"git log --oneline", "first commit"},
		{"git status --porcelain", "M a.txt"},
		{"git diff --stat", "a.txt"},
		{"git show HEAD:a.txt", "needle one"},
		{"git ls-files", "notes.txt"},
		{"git blame -L 1,1 notes.txt", "notes"},
	} {
		got, err := workerBash(t, root, map[string]any{"command": c.command})
		if err != nil || !strings.Contains(got, c.want) {
			t.Fatalf("%s = %q, %v", c.command, got, err)
		}
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("repository config ran a command")
	}
	after, err := os.Stat(filepath.Join(root, ".git", "index"))
	if err != nil || !after.ModTime().Equal(index.ModTime()) {
		t.Fatal("git status rewrote the index")
	}
	// A repository above the root is not discovered: git sees only the root.
	nested := filepath.Join(root, "sub")
	if got, err := workerBash(t, nested, map[string]any{"command": "git log --oneline"}); err != nil || strings.Contains(got, "first commit") {
		t.Fatalf("worker rooted in a subfolder read the enclosing repository: %q %v", got, err)
	}

	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	db := filepath.Join(root, "data.db")
	if out, err := exec.Command(sqlite, db, "CREATE TABLE t(x); INSERT INTO t VALUES (42);").CombinedOutput(); err != nil {
		t.Fatalf("seed: %v %s", err, out)
	}
	got, err := workerBash(t, root, map[string]any{"command": "sqlite3 -readonly data.db 'SELECT x FROM t'"})
	if err != nil || !strings.Contains(got, "42") {
		t.Fatalf("sqlite3 read = %q, %v", got, err)
	}
	got, err = workerBash(t, root, map[string]any{"command": "sqlite3 -readonly data.db \"SELECT writefile('w.txt', 'x')\""})
	if err != nil || strings.Contains(got, "exit status: 0") {
		t.Fatalf("writefile ran: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "w.txt")); !os.IsNotExist(err) {
		t.Fatal("sqlite3 wrote a file")
	}
}

// rg runs without its user config, so a configured preprocessor (an exec
// vector) never runs, and secret files stay out of recursive searches.
func TestReadOnlyBashRipgrepIgnoresConfigAndSecrets(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed")
	}
	root, outside := workerRootFixture(t)
	marker := filepath.Join(outside, "pre-ran")
	pre := filepath.Join(outside, "pre.sh")
	if err := os.WriteFile(pre, []byte("#!/bin/sh\ntouch "+marker+"\ncat \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(outside, "rgrc")
	if err := os.WriteFile(config, []byte("--pre="+pre+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIPGREP_CONFIG_PATH", config)
	// Control: rg run directly honours the config and runs the program.
	control := exec.Command("rg", "needle", ".")
	control.Dir = root
	_ = control.Run()
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("control: the configured preprocessor did not run without the harness: %v", err)
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	got, err := workerBash(t, root, map[string]any{"command": "rg -uu -n 'needle|TOKEN'"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "needle one") || strings.Contains(got, "TOKEN") {
		t.Fatalf("rg output = %q", got)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("rg ran the configured preprocessor")
	}
}
