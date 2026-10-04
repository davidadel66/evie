package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// buildFixture is a build worker's fence: a repository (the owner's
// checkout) with one commit, a linked worktree on its own branch inside the
// repository's .evie folder, the attempt's private temp folder, a bare
// remote, and a folder outside everything. The worktree holds links that
// escape it and one that points at its .git file.
type buildFixtureT struct {
	repo, worktree, temp, outside, remote string
	fence                                 BuildFence
}

func gitCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not installed")
	}
	cmd := exec.Command(git, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func buildFixture(t *testing.T) buildFixtureT {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := buildFixtureT{repo: filepath.Join(base, "repo"), outside: filepath.Join(base, "outside"), remote: filepath.Join(base, "remote.git")}
	for _, dir := range []string{f.repo, f.outside} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(f.repo, "a.txt"):         "alpha\nneedle one\nomega\n",
		filepath.Join(f.repo, "sub", "b.txt"):  "beta\n",
		filepath.Join(f.outside, "victim.txt"): "original\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitCommand(t, f.repo, "init", "-q", "-b", "master")
	gitCommand(t, f.repo, "add", "a.txt", "sub/b.txt")
	gitCommand(t, f.repo, "commit", "-q", "-m", "first commit")
	gitCommand(t, base, "init", "-q", "--bare", f.remote)
	gitCommand(t, f.repo, "remote", "add", "origin", f.remote)
	f.worktree = filepath.Join(f.repo, ".evie", "worktrees", "w1")
	f.temp = filepath.Join(f.repo, ".evie", "tmp", "w1")
	gitCommand(t, f.repo, "worktree", "add", "-q", "-b", "evie/w1-test", f.worktree, "HEAD")
	if err := os.MkdirAll(f.temp, 0o700); err != nil {
		t.Fatal(err)
	}
	for link, target := range map[string]string{
		filepath.Join(f.worktree, "link-dir-out"):  f.outside,
		filepath.Join(f.worktree, "link-file-out"): filepath.Join(f.outside, "victim.txt"),
		filepath.Join(f.worktree, "link-git"):      filepath.Join(f.worktree, ".git"),
		filepath.Join(f.worktree, "link-in"):       filepath.Join(f.worktree, "a.txt"),
	} {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(f.worktree, ".env"), []byte("TOKEN=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.fence = BuildFence{Worktree: f.worktree, Temp: f.temp}
	return f
}

func buildCall(t *testing.T, fence BuildFence, name string, params map[string]any) (string, error) {
	t.Helper()
	args, _ := json.Marshal(params)
	ctx := withBuildFence(context.Background(), fence)
	for _, tool := range BuildTools(true) {
		if tool.Schema.Function.Name == name {
			if tool.NeedsApproval {
				t.Fatalf("build %s asks for approval inside the worktree", name)
			}
			return tool.Execute(ctx, string(args))
		}
	}
	t.Fatalf("no build tool %s", name)
	return "", nil
}

func fileContent(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Stage 11: the build worker's file tools write freely inside its worktree
// and nowhere else. Paths are resolved with symlinks evaluated; .., absolute
// paths, links out, the worktree's .git (in any case or with invisible
// characters), nested .git folders and secret files are refused.
func TestBuildFileToolsStayInsideTheWorktree(t *testing.T) {
	f := buildFixture(t)
	gitFile := fileContent(t, filepath.Join(f.worktree, ".git"))

	for _, path := range []string{"a.txt", "link-in", filepath.Join(f.worktree, "sub", "b.txt"), "sub/../a.txt"} {
		if got, err := buildCall(t, f.fence, "read_file", map[string]any{"path": path}); err != nil || !strings.Contains(got, "\t") {
			t.Fatalf("read %q = %q, %v", path, got, err)
		}
	}
	if _, err := buildCall(t, f.fence, "write_file", map[string]any{"path": "new/dir/c.txt", "content": "gamma\n"}); err != nil {
		t.Fatal(err)
	}
	if got := fileContent(t, filepath.Join(f.worktree, "new", "dir", "c.txt")); got != "gamma\n" {
		t.Fatalf("written file = %q", got)
	}
	if _, err := buildCall(t, f.fence, "write_file", map[string]any{"path": filepath.Join(f.worktree, "a.txt"), "content": "replaced\n"}); err != nil {
		t.Fatal(err)
	}
	if _, err := buildCall(t, f.fence, "edit_file", map[string]any{"path": "sub/b.txt", "old_string": "beta", "new_string": "BETA"}); err != nil {
		t.Fatal(err)
	}
	if got := fileContent(t, filepath.Join(f.worktree, "sub", "b.txt")); got != "BETA\n" {
		t.Fatalf("edited file = %q", got)
	}

	refused := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"read_file", map[string]any{"path": "../../../a.txt"}, "outside"},
		{"read_file", map[string]any{"path": filepath.Join(f.outside, "victim.txt")}, "outside"},
		{"read_file", map[string]any{"path": "link-file-out"}, "outside"},
		{"read_file", map[string]any{"path": "link-dir-out/victim.txt"}, "outside"},
		{"read_file", map[string]any{"path": "/etc/hosts"}, "outside"},
		{"read_file", map[string]any{"path": "~/.zshrc"}, "~"},
		{"read_file", map[string]any{"path": ".env"}, "off-limits"},
		{"read_file", map[string]any{"path": ".git"}, ".git"},
		{"write_file", map[string]any{"path": "../../../a.txt", "content": "x"}, "outside"},
		{"write_file", map[string]any{"path": filepath.Join(f.repo, "a.txt"), "content": "x"}, "outside"},
		{"write_file", map[string]any{"path": filepath.Join(f.outside, "new.txt"), "content": "x"}, "outside"},
		{"write_file", map[string]any{"path": "link-dir-out/new.txt", "content": "x"}, "outside"},
		{"write_file", map[string]any{"path": "link-file-out", "content": "x"}, "outside"},
		{"write_file", map[string]any{"path": "link-git", "content": "gitdir: /tmp/elsewhere"}, ".git"},
		{"write_file", map[string]any{"path": ".git", "content": "gitdir: /tmp/elsewhere"}, ".git"},
		{"write_file", map[string]any{"path": ".GIT", "content": "gitdir: /tmp/elsewhere"}, ".git"},
		{"write_file", map[string]any{"path": ".g​it", "content": "gitdir: /tmp/elsewhere"}, ".git"},
		{"write_file", map[string]any{"path": "sub/.git/config", "content": "[core]\n\tfsmonitor = /bin/evil\n"}, ".git"},
		{"write_file", map[string]any{"path": ".env.local", "content": "x"}, "off-limits"},
		{"write_file", map[string]any{"path": "~/evil", "content": "x"}, "~"},
		{"write_file", map[string]any{"path": "sub", "content": "x"}, "directory"},
		{"write_file", map[string]any{"path": "", "content": "x"}, "empty"},
		{"edit_file", map[string]any{"path": "link-file-out", "old_string": "original", "new_string": "pwned"}, "outside"},
		{"edit_file", map[string]any{"path": filepath.Join(f.repo, "a.txt"), "old_string": "alpha", "new_string": "pwned"}, "outside"},
		{"edit_file", map[string]any{"path": "link-git", "old_string": "gitdir", "new_string": "x"}, ".git"},
		{"edit_file", map[string]any{"path": ".git", "old_string": "gitdir", "new_string": "x"}, ".git"},
	}
	for _, c := range refused {
		if got, err := buildCall(t, f.fence, c.tool, c.args); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s %v = %q, %v; want refusal naming %q", c.tool, c.args, got, err, c.want)
		}
	}
	if got := fileContent(t, filepath.Join(f.outside, "victim.txt")); got != "original\n" {
		t.Fatalf("outside file changed: %q", got)
	}
	if got := fileContent(t, filepath.Join(f.repo, "a.txt")); got != "alpha\nneedle one\nomega\n" {
		t.Fatalf("owner's checkout changed: %q", got)
	}
	if got := fileContent(t, filepath.Join(f.worktree, ".git")); got != gitFile {
		t.Fatalf("worktree .git changed: %q", got)
	}
	for _, path := range []string{filepath.Join(f.outside, "new.txt"), filepath.Join(f.worktree, "sub", ".git"), filepath.Join(f.worktree, ".g​it")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s was created", path)
		}
	}
	// Without a bound fence the tools refuse to run at all.
	for _, tool := range BuildTools(true) {
		if _, err := tool.Execute(context.Background(), `{"path":"a.txt","command":"true"}`); err == nil || !strings.Contains(err.Error(), "build worker") {
			t.Fatalf("%s ran without a fence: %v", tool.Schema.Function.Name, err)
		}
	}
}

// Stage 11: a host without a usable OS sandbox composes build workers
// without bash, never with an unsandboxed shell.
func TestBuildToolsetHasBashOnlyWithASandbox(t *testing.T) {
	names := func(set Toolset) []string {
		var out []string
		for _, schema := range set.Schemas() {
			out = append(out, schema.Function.Name)
		}
		return out
	}
	if got := names(BuildToolset(true)); !slices.Equal(got, []string{"read_file", "edit_file", "write_file", "bash"}) {
		t.Fatalf("build tools with a sandbox = %v", got)
	}
	if got := names(BuildToolset(false)); !slices.Equal(got, []string{"read_file", "edit_file", "write_file"}) {
		t.Fatalf("build tools without a sandbox = %v", got)
	}
	t.Setenv("EVIE_WORKER_SANDBOX", "off")
	if WorkerSandboxAvailable() {
		t.Fatal("EVIE_WORKER_SANDBOX=off left the sandbox available")
	}
	if _, err := DetectWorkerSandbox(); err == nil || !strings.Contains(err.Error(), "EVIE_WORKER_SANDBOX") {
		t.Fatalf("disabled sandbox reason = %v", err)
	}
	f := buildFixture(t)
	if _, err := buildCall(t, f.fence, "bash", map[string]any{"command": "touch made.txt"}); err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("bash ran without a sandbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.worktree, "made.txt")); !os.IsNotExist(err) {
		t.Fatal("unsandboxed bash wrote a file")
	}
}

func requireSandbox(t *testing.T) {
	t.Helper()
	if _, err := DetectWorkerSandbox(); err != nil {
		t.Skipf("no OS sandbox on this host: %v", err)
	}
}

// Stage 11: the build worker's bash runs under the OS sandbox: it starts in
// the worktree and can write only there and in its private temp folder. It
// cannot write to the owner's checkout, its home, /tmp or the worktree's .git,
// reach the network (even loopback), update the main repository's refs, push,
// or read secrets, and it never sees Evie's environment.
func TestSandboxedBashConfinesWritesNetworkAndGit(t *testing.T) {
	requireSandbox(t)
	f := buildFixture(t)
	home := filepath.Join(filepath.Dir(f.repo), "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("SSH-SECRET-SENTINEL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An identity, so a refused commit is the sandbox's doing.
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[user]\n\tname = t\n\temail = t@example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENROUTER_API_KEY", "sk-ENV-SECRET-SENTINEL")
	bash := func(params map[string]any) string {
		t.Helper()
		got, err := buildCall(t, f.fence, "bash", params)
		if err != nil {
			t.Fatalf("bash %v: %v", params, err)
		}
		return got
	}
	exitOK := func(out string) bool { return strings.Contains(out, "exit status: 0\n") }

	if got := bash(map[string]any{"command": "pwd -P"}); !strings.HasPrefix(got, f.worktree+"\n") {
		t.Fatalf("bash starts in %q, want the worktree", got)
	}
	if got := bash(map[string]any{"command": "echo inside > made.txt && mkdir -p deep/er && echo t > \"$TMPDIR/t.txt\" && cat made.txt"}); !exitOK(got) || !strings.Contains(got, "inside") {
		t.Fatalf("write inside the worktree and temp folder failed: %s", got)
	}
	tmpProbe := filepath.Join("/tmp", fmt.Sprintf("evie-sandbox-probe-%d", time.Now().UnixNano()))
	writes := map[string]string{
		"owner checkout":      "echo pwned > ../../../a.txt",
		"owner checkout path": "echo pwned > " + filepath.Join(f.repo, "a.txt"),
		"outside folder":      "echo pwned > " + filepath.Join(f.outside, "new.txt"),
		"through a link":      "echo pwned >> link-file-out",
		"home":                "echo pwned > \"$HOME/evil\"",
		"tmp":                 "echo pwned > " + tmpProbe,
		"after cd /":          "cd / && touch evie-cd-probe",
		"worktree .git":       "echo 'gitdir: /tmp/x' > .git",
		"worktree .GIT":       "echo 'gitdir: /tmp/x' > .GIT",
		"nested .git":         "mkdir -p sub/.git",
		"move .git":           "mv .git moved",
		"hard link out":       "ln " + filepath.Join(f.outside, "victim.txt") + " hl && echo pwned >> hl",
		"main repo refs":      "git -C " + f.repo + " update-ref refs/heads/evil HEAD",
		"git add":             "git add -A",
		"git commit":          "git commit -q -m sneaky",
		"git push":            "git push -q origin HEAD:refs/heads/pushed",
	}
	for name, command := range writes {
		if got := bash(map[string]any{"command": command}); exitOK(got) {
			t.Fatalf("%s: %q succeeded:\n%s", name, command, got)
		}
	}
	if got := fileContent(t, filepath.Join(f.repo, "a.txt")); got != "alpha\nneedle one\nomega\n" {
		t.Fatalf("owner's checkout changed: %q", got)
	}
	if got := fileContent(t, filepath.Join(f.outside, "victim.txt")); got != "original\n" {
		t.Fatalf("outside file changed: %q", got)
	}
	for _, path := range []string{filepath.Join(f.outside, "new.txt"), filepath.Join(home, "evil"), tmpProbe, "/evie-cd-probe", filepath.Join(f.worktree, "sub", ".git")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s was created", path)
		}
	}
	if !strings.HasPrefix(fileContent(t, filepath.Join(f.worktree, ".git")), "gitdir: ") || strings.Contains(fileContent(t, filepath.Join(f.worktree, ".git")), "/tmp/x") {
		t.Fatal("worktree .git was rewritten")
	}
	for _, ref := range []string{"refs/heads/evil"} {
		if out, err := exec.Command("git", "-C", f.repo, "rev-parse", "--verify", "--quiet", ref).CombinedOutput(); err == nil {
			t.Fatalf("%s was created: %s", ref, out)
		}
	}
	if out, err := exec.Command("git", "-C", f.remote, "rev-parse", "--verify", "--quiet", "refs/heads/pushed").CombinedOutput(); err == nil {
		t.Fatalf("push reached the remote: %s", out)
	}
	if head := gitCommand(t, f.worktree, "rev-parse", "HEAD"); head != gitCommand(t, f.repo, "rev-parse", "master") {
		t.Fatalf("worktree HEAD moved to %s", head)
	}

	// Read-only git inside the worktree works.
	if got := bash(map[string]any{"command": "git status --short && git log --oneline -1"}); !exitOK(got) || !strings.Contains(got, "made.txt") || !strings.Contains(got, "first commit") {
		t.Fatalf("read-only git failed: %s", got)
	}
	// No secrets: neither the home's keys nor Evie's own environment.
	if got := bash(map[string]any{"command": "cat \"$HOME/.ssh/id_ed25519\"; env"}); strings.Contains(got, "SSH-SECRET-SENTINEL") || strings.Contains(got, "sk-ENV-SECRET-SENTINEL") {
		t.Fatalf("bash read a secret:\n%s", got)
	}

	// No network, not even loopback.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connected atomic.Bool
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connected.Store(true)
			conn.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	probes := []string{fmt.Sprintf("echo hi > /dev/tcp/127.0.0.1/%d", port)}
	if _, err := exec.LookPath("curl"); err == nil {
		probes = append(probes, fmt.Sprintf("curl -sS -m 3 http://127.0.0.1:%d/", port), "curl -sS -m 3 https://example.com/")
	}
	for _, probe := range probes {
		if got := bash(map[string]any{"command": probe}); exitOK(got) {
			t.Fatalf("network probe %q succeeded:\n%s", probe, got)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if connected.Load() {
		t.Fatal("a sandboxed command connected to a local listener")
	}

	// cwd outside the worktree is refused before anything runs.
	for _, cwd := range []string{"/", "..", f.repo, "link-dir-out"} {
		if got, err := buildCall(t, f.fence, "bash", map[string]any{"command": "touch cwd-probe", "cwd": cwd}); err == nil {
			t.Fatalf("cwd %q accepted: %s", cwd, got)
		}
	}
	if _, err := os.Stat(filepath.Join(f.repo, "cwd-probe")); !os.IsNotExist(err) {
		t.Fatal("a refused cwd ran its command")
	}
}
