package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A build worker's bash runs only under an OS sandbox (Stage 11, 2026-10-03):
// Seatbelt (sandbox-exec) on macOS, as Claude Code does, and bubblewrap on
// Linux when it is installed and usable. The sandbox lets the shell write
// only inside the attempt's worktree (never its .git) and private temp
// folder, denies every network connection (including loopback and Unix
// sockets), and allows reads, so the toolchain works, except of locations
// that may hold secrets. Where no sandbox is available the build worker has
// no bash at all; Evie never hands a worker a write-capable shell without
// one. EVIE_WORKER_SANDBOX=off turns build-worker bash off on any host.

// WorkerSandbox confines one command of a build worker's bash.
type WorkerSandbox interface {
	Name() string
	// Command returns the confined command for program and args, to be run
	// with its Dir inside fence.Worktree and the environment buildEnv makes.
	Command(ctx context.Context, fence BuildFence, program string, args ...string) (*exec.Cmd, error)
}

// workerSandboxEnv lets an operator turn build-worker bash off.
const workerSandboxEnv = "EVIE_WORKER_SANDBOX"

var (
	probeOnce     sync.Once
	probedSandbox WorkerSandbox
	probeErr      error
)

// DetectWorkerSandbox returns this host's sandbox, or why there is none. The
// OS probe runs once per process; the operator switch is read every time.
func DetectWorkerSandbox() (WorkerSandbox, error) {
	if setting := strings.TrimSpace(os.Getenv(workerSandboxEnv)); setting != "" && setting != "auto" {
		if setting == "off" {
			return nil, fmt.Errorf("build-worker bash is turned off by %s=off", workerSandboxEnv)
		}
		return nil, fmt.Errorf("%s=%q is not a setting; use off or auto", workerSandboxEnv, setting)
	}
	probeOnce.Do(func() { probedSandbox, probeErr = probeWorkerSandbox() })
	return probedSandbox, probeErr
}

// WorkerSandboxAvailable reports whether build workers get a bash.
func WorkerSandboxAvailable() bool {
	_, err := DetectWorkerSandbox()
	return err == nil
}

func probeWorkerSandbox() (WorkerSandbox, error) {
	switch runtime.GOOS {
	case "darwin":
		path := "/usr/bin/sandbox-exec"
		if _, err := os.Stat(path); err != nil {
			return nil, errors.New("sandbox-exec is not installed")
		}
		s := seatbelt{path: path}
		return s, probeRun(exec.Command(path, "-p", "(version 1)(allow default)", "/usr/bin/true"))
	case "linux":
		path, err := exec.LookPath("bwrap")
		if err != nil {
			return nil, errors.New("bubblewrap (bwrap) is not installed")
		}
		s := bubblewrap{path: path}
		return s, probeRun(exec.Command(path, "--ro-bind", "/", "/", "--dev", "/dev", "--unshare-all", "--die-with-parent", "--", "/bin/true"))
	}
	return nil, fmt.Errorf("no OS sandbox is supported on %s", runtime.GOOS)
}

func probeRun(cmd *exec.Cmd) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probe := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	if out, err := probe.CombinedOutput(); err != nil {
		return fmt.Errorf("the OS sandbox does not work here: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// secretReadLocations are what a sandboxed shell may not read: the file
// tools' denylist plus all of ~/.evie and a few other credential stores.
func secretReadLocations() (dirs, files []string) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	for _, rel := range []string{".ssh", ".aws", ".gnupg", ".evie", ".config/gh", ".kube", ".docker", "Library/Keychains"} {
		dirs = append(dirs, filepath.Join(home, rel))
	}
	for _, rel := range []string{".netrc", ".git-credentials", ".npmrc", ".pypirc", ".zshrc", ".bashrc", ".zprofile", ".bash_profile", ".profile", ".zsh_history", ".bash_history"} {
		files = append(files, filepath.Join(home, rel))
	}
	return withResolved(dirs), withResolved(files)
}

// withResolved adds each existing path's symlink-resolved form, since the
// sandbox matches real paths.
func withResolved(paths []string) []string {
	out := append([]string(nil), paths...)
	for _, p := range paths {
		if resolved, err := filepath.EvalSymlinks(p); err == nil && resolved != p {
			out = append(out, resolved)
		}
	}
	return out
}

// checkFence requires the fence's folders to be canonical existing
// directories, so the sandbox's path rules name exactly them.
func checkFence(fence BuildFence) error {
	for _, dir := range []string{fence.Worktree, fence.Temp} {
		if dir == "" || !filepath.IsAbs(dir) {
			return fmt.Errorf("sandbox folder %q is not an absolute path", dir)
		}
		canonical, err := filepath.EvalSymlinks(dir)
		if err != nil || canonical != dir {
			return fmt.Errorf("sandbox folder %q is unavailable or not canonical", dir)
		}
		if strings.ContainsAny(dir, "\x00\n\r") {
			return fmt.Errorf("sandbox folder %q has a control character", dir)
		}
	}
	return nil
}

// seatbelt is macOS's sandbox, driven by a profile generated per command.
type seatbelt struct{ path string }

func (seatbelt) Name() string { return "seatbelt" }

func (s seatbelt) Command(ctx context.Context, fence BuildFence, program string, args ...string) (*exec.Cmd, error) {
	profile, err := seatbeltProfile(fence)
	if err != nil {
		return nil, err
	}
	return exec.CommandContext(ctx, s.path, append([]string{"-p", profile, program}, args...)...), nil
}

// sbplString quotes s as a Seatbelt profile string literal.
func sbplString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// seatbeltProfile denies everything by default (so no network, no Mach
// services that could launch work outside the sandbox, no writes) and then
// allows what a build needs: running programs, reading files except
// secrets, and writing only inside the worktree, except its .git or any
// .git below it, and the private temp folder. Later rules win, which is how
// the denials carve exceptions out of the allowances.
func seatbeltProfile(fence BuildFence) (string, error) {
	if err := checkFence(fence); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("(version 1)\n(deny default)\n")
	b.WriteString("(allow process-exec)\n(allow process-fork)\n(allow signal (target same-sandbox))\n(allow process-info* (target same-sandbox))\n")
	b.WriteString("(allow sysctl-read)\n(allow file-ioctl)\n(allow ipc-posix-shm)\n(allow ipc-posix-sem)\n")
	b.WriteString("(allow mach-lookup (global-name \"com.apple.system.opendirectoryd.libinfo\"))\n")
	b.WriteString("(allow file-read*)\n")
	dirs, files := secretReadLocations()
	for _, dir := range dirs {
		fmt.Fprintf(&b, "(deny file-read* (subpath %s))\n", sbplString(dir))
	}
	for _, file := range files {
		fmt.Fprintf(&b, "(deny file-read* (literal %s))\n", sbplString(file))
	}
	b.WriteString("(deny file-read* (regex #\"/\\.env[^/]*$\"))\n")
	fmt.Fprintf(&b, "(allow file-write* (subpath %s) (subpath %s) (literal \"/dev/null\") (literal \"/dev/zero\") (literal \"/dev/dtracehelper\") (regex #\"^/dev/fd/[0-9]+$\"))\n",
		sbplString(fence.Worktree), sbplString(fence.Temp))
	fmt.Fprintf(&b, "(deny file-write* (require-all (subpath %s) (regex #\"/\\.[Gg][Ii][Tt](/|$)\")))\n", sbplString(fence.Worktree))
	return b.String(), nil
}

// bubblewrap is Linux's sandbox: the whole filesystem read-only, the
// worktree and temp folder writable, the worktree's .git read-only, secret
// folders hidden, and every namespace unshared, the network included.
// Unlike Seatbelt it cannot stop a nested .git from being created; the
// harness refuses to commit a worktree that has one.
type bubblewrap struct{ path string }

func (bubblewrap) Name() string { return "bubblewrap" }

func (s bubblewrap) Command(ctx context.Context, fence BuildFence, program string, args ...string) (*exec.Cmd, error) {
	if err := checkFence(fence); err != nil {
		return nil, err
	}
	argv := []string{"--die-with-parent", "--new-session", "--unshare-all", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc",
		"--bind", fence.Worktree, fence.Worktree, "--bind", fence.Temp, fence.Temp}
	if _, err := os.Lstat(filepath.Join(fence.Worktree, ".git")); err == nil {
		argv = append(argv, "--ro-bind", filepath.Join(fence.Worktree, ".git"), filepath.Join(fence.Worktree, ".git"))
	}
	dirs, files := secretReadLocations()
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			argv = append(argv, "--tmpfs", dir)
		}
	}
	for _, file := range files {
		if info, err := os.Stat(file); err == nil && info.Mode().IsRegular() {
			argv = append(argv, "--ro-bind", "/dev/null", file)
		}
	}
	argv = append(argv, "--chdir", fence.Worktree, "--", program)
	return exec.CommandContext(ctx, s.path, append(argv, args...)...), nil
}

// buildEnv is the sandboxed shell's environment, built rather than
// inherited so Evie's own secrets (API keys) never reach it: the path, home
// and locale, toolchain locations, and caches redirected into the private
// temp folder, the only writable place outside the worktree.
func buildEnv(fence BuildFence) []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	home, _ := os.UserHomeDir()
	env := []string{
		"PATH=" + path, "HOME=" + home, "SHELL=/bin/bash", "TERM=dumb", "PAGER=cat", "GIT_PAGER=cat",
		"TMPDIR=" + fence.Temp, "TMP=" + fence.Temp, "TEMP=" + fence.Temp,
		"XDG_CACHE_HOME=" + filepath.Join(fence.Temp, "cache"),
		"GOCACHE=" + filepath.Join(fence.Temp, "go-build"), "GOTMPDIR=" + fence.Temp, "GOTOOLCHAIN=local", "GOPROXY=off",
		"npm_config_cache=" + filepath.Join(fence.Temp, "npm"),
		"GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0",
	}
	for _, name := range []string{"USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE", "GOPATH", "GOROOT", "GOMODCACHE", "CARGO_HOME", "RUSTUP_HOME", "JAVA_HOME"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}
