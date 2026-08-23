package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// snapshotTimeout bounds capturing the user's shell environment. A shell
// whose rc file hangs — waiting on a network call, a slow version manager
// — must not hold evie up; past this we fall back to a login shell.
const snapshotTimeout = 10 * time.Second

// A login shell (`-l`) loads .zprofile and .profile, which is enough for
// PATH edits, but aliases and functions live in .zshrc / .bashrc and those
// are only read by an *interactive* shell. Running an interactive shell per
// command would be slow and noisy, so instead we run one once, dump what it
// defined into a file, and source that file before every command.
//
// snapshotCapture keeps at most one capture active even if Warm and tool calls
// race. Waiters cancel independently; when the final waiter leaves, the lazy
// capture is cancelled and remains retryable. Ordinary capture failures retain
// the prior sticky one-attempt policy.
var (
	snapshotMu        sync.Mutex
	snapshotPath      string
	snapshotAttempted bool
	snapshotCapture   *shellCapture
	captureShellFunc  = captureShell
)

type shellCapture struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	path    string
	err     error
}

// Warm captures the user's shell environment ahead of the first command.
// Called from main at startup so the cost lands during boot rather than in
// the middle of the model's first bash call; everything still works if it
// is never called, because runBash triggers the same capture lazily.
func Warm() {
	go snapshot(context.Background())
}

// snapshot returns the path to a sourceable file holding the user's shell
// functions, aliases, options and PATH — or "" if capture failed, in which
// case commands run under a plain login shell and lose only the
// interactive-rc pieces. Failure is never surfaced to the model: a missing
// alias is a degraded shell, not a broken tool.
func snapshot(ctx context.Context) string {
	if ctx.Err() != nil {
		return ""
	}
	snapshotMu.Lock()
	if snapshotPath != "" {
		path := snapshotPath
		snapshotMu.Unlock()
		return path
	}
	if snapshotAttempted {
		snapshotMu.Unlock()
		return ""
	}
	capture := snapshotCapture
	if capture == nil {
		captureCtx, cancel := context.WithCancel(context.Background())
		capture = &shellCapture{done: make(chan struct{}), cancel: cancel}
		snapshotCapture = capture
		go func(c *shellCapture) {
			path, err := captureShellFunc(captureCtx, shellPath())
			snapshotMu.Lock()
			if cancelErr := captureCtx.Err(); cancelErr != nil {
				if path != "" {
					_ = os.Remove(path)
				}
				path = ""
				err = cancelErr
			} else {
				snapshotAttempted = true
			}
			if err == nil && path != "" {
				snapshotPath = path
				c.path = path
			}
			c.err = err
			if snapshotCapture == c {
				snapshotCapture = nil
			}
			close(c.done)
			snapshotMu.Unlock()
		}(capture)
	}
	capture.waiters++
	snapshotMu.Unlock()

	select {
	case <-capture.done:
		if errors.Is(capture.err, context.Canceled) && ctx.Err() == nil {
			return snapshot(ctx)
		}
		return capture.path
	case <-ctx.Done():
		snapshotMu.Lock()
		capture.waiters--
		if capture.waiters == 0 {
			capture.cancel()
		}
		snapshotMu.Unlock()
		return ""
	}
}

// captureShell runs one interactive login shell and has it write its own
// definitions to a file. Interactive is the point — it is the only mode
// that reads .zshrc / .bashrc — and also why stderr is discarded: prompt
// setup and job-control warnings are normal noise in that mode.
func captureShell(parent context.Context, shell string) (string, error) {
	if err := parent.Err(); err != nil {
		return "", err
	}
	out, err := os.CreateTemp("", "evie-shell-*.sh")
	if err != nil {
		return "", fmt.Errorf("create snapshot file: %w", err)
	}
	path := out.Name()
	out.Close()

	ctx, cancel := context.WithTimeout(parent, snapshotTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shell, "-i", "-l", "-c", dumpScript(shell, path))
	cmd.WaitDelay = 2 * time.Second

	// An interactive shell does job control: it tries to take ownership of
	// the controlling terminal. Run from a background goroutine that is not
	// the foreground process group, that raises SIGTTOU and suspends evie
	// itself — the whole REPL freezes at startup. Setsid puts the snapshot
	// shell in a fresh session with no controlling terminal, so it has no
	// terminal to fight over. Stdio goes nowhere for the same reason.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("capture shell environment: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		os.Remove(path)
		return "", fmt.Errorf("shell environment capture was empty")
	}

	return path, nil
}

// rcFiles lists the startup files worth sourcing for a given shell, most
// general first. Sourcing is guarded by an existence check, so naming a
// file the user doesn't have costs nothing.
func rcFiles(shell string) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	names := []string{".profile", ".bash_profile", ".bashrc"}
	if strings.Contains(shell, "zsh") {
		names = []string{".zprofile", ".zshrc"}
	}

	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(home, name))
	}
	return paths
}

// dumpScript builds the shell program that writes the snapshot. zsh and
// bash disagree on every one of these introspection commands, so the two
// are written out separately rather than hidden behind a lowest common
// denominator.
func dumpScript(shell, path string) string {
	q := shellQuote(path)

	var b strings.Builder

	// Source the rc file by name rather than relying on the shell to read it
	// at startup. Startup semantics are the trap here: `bash -i -l` reads
	// .bash_profile and never touches .bashrc, so a snapshot that waited for
	// the shell to load it came back empty. `-i` is still passed so that rc
	// files guarding their contents on `[[ $- == *i* ]]` run anyway.
	for _, rc := range rcFiles(shell) {
		fmt.Fprintf(&b, "[ -f %s ] && . %s 2>/dev/null\n", shellQuote(rc), shellQuote(rc))
	}

	// PATH as the interactive shell resolved it, which is the main thing a
	// non-interactive shell gets wrong. %q rather than %s so a directory
	// containing a space survives being written out and sourced back in —
	// and "$PATH" unquoted here, or the shell would write the literal five
	// characters instead of expanding them.
	fmt.Fprintf(&b, "printf 'export PATH=%%q\\n' \"$PATH\" > %s\n", q)

	if strings.Contains(shell, "zsh") {
		// `typeset -f` with no arguments forces autoloaded functions to load
		// before we ask for their names; without it lazily-defined functions
		// from version managers are missing.
		fmt.Fprintf(&b, "typeset -f > /dev/null 2>&1\n")
		// Skip completion functions (a single leading underscore) but keep
		// double-underscore helpers, which tools like mise and pyenv rely on.
		fmt.Fprintf(&b, "typeset +f | grep -vE '^_[^_]' | while read -r f; do typeset -f \"$f\" >> %s; done\n", q)
		fmt.Fprintf(&b, "setopt | sed 's/^/setopt /' >> %s\n", q)
	} else {
		fmt.Fprintf(&b, "declare -f > /dev/null 2>&1\n")
		fmt.Fprintf(&b, "declare -F | cut -d' ' -f3 | grep -vE '^_[^_]' | while read -r f; do declare -f \"$f\" >> %s; done\n", q)
		fmt.Fprintf(&b, "shopt -p >> %s\n", q)
		// Aliases are disabled in non-interactive bash, so the snapshot has
		// to turn them back on for the shells that will source it.
		fmt.Fprintf(&b, "echo 'shopt -s expand_aliases' >> %s\n", q)
	}

	// `alias` prints `name=value`; `alias --` in front makes each line a
	// valid statement again, and tolerates alias names starting with a dash.
	fmt.Fprintf(&b, "alias | sed 's/^alias //' | sed 's/^/alias -- /' >> %s\n", q)

	// Never let a failing introspection command fail the whole capture: a
	// missing `setopt` is worth less than the functions we already wrote.
	b.WriteString("exit 0\n")

	return b.String()
}
