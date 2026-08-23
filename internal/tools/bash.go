package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

const (
	// defaultBashTimeout bounds a command that would otherwise hang forever
	// — a prompt waiting on stdin, a server that never exits. The model can
	// raise it per call up to maxBashTimeout. Two and ten minutes match
	// Claude Code's defaults, which are sized for real builds and test
	// suites rather than for one-liners.
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second

	// maxBashOutput caps what comes back inline. Past it the full output is
	// spilled to a file and the model is told where — so nothing is lost,
	// but one `find /` can't blow the context window.
	maxBashOutput = 30_000
)

// sessionCwd is the working directory the next command starts in, updated
// after every call from the shell's own `pwd`. This is deliberate shared
// state: the model works in a project for a long stretch, and making it
// re-pass cwd on every call is friction with no safety benefit — a shell
// that can `cd` can reach anywhere regardless. Empty means "wherever
// evie was launched", which is what a subprocess inherits by default.
//
// It reverses the "internal/tools is stateless" decision recorded in
// file-tools.decisions.md; see bash.decisions.md for why.
var (
	sessionCwdMu sync.Mutex
	sessionCwd   string
)

// bashTool describes bash to the model: a real shell, ungated. There is no
// approval prompt and no path denylist — a shell walks around any fence
// worth the name (`cat ~/.ssh/id_rsa` is one command), so pretending
// otherwise would buy false confidence rather than safety. The defense is
// that David reads what evie is doing.
var bashTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "bash",
		Description: `Run a shell command and get its output. This is a real login shell: pipes, globs, redirects, && and ||, heredocs, multi-line scripts, and anything on your PATH all work.

Output is stdout and stderr combined in the order they were written, followed by the exit status. A non-zero exit is NOT an error — it is information. Read the output and the status and act on them; a failing test suite or a grep that found nothing both come back this way.

The working directory persists between calls: if you cd somewhere, the next command starts there. Pass cwd to start somewhere else for one call — that directory then becomes the new working directory too. Run pwd if you are unsure where you are.

Output larger than 30000 characters is trimmed, and the full output is written to a file whose path is reported at the end. Read it with head, tail, or grep rather than cat.

Long-running commands are killed after 2 minutes by default; raise timeout_seconds (up to 600) for builds or test suites. Do not start servers or anything that waits for input — they will simply time out.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"command"},
			Properties: map[string]openrouter.Property{
				"command": {
					Type:        "string",
					Description: "The shell command to run. May be a multi-line script.",
				},
				"cwd": {
					Type:        "string",
					Description: "Directory to start in for this call. Absolute, or home-relative starting with ~/. Defaults to the working directory left behind by the previous command.",
				},
				"timeout_seconds": {
					Type:        "integer",
					Description: "Seconds before the command is killed. Defaults to 120, maximum 600.",
				},
			},
		},
	},
}

// shellPath returns the user's login shell, falling back to bash. Running
// their actual shell as a login shell is what makes PATH edits and profile
// setup visible to commands — a bare `bash -c` inherits none of it. The
// interactive-only pieces (.zshrc aliases and functions) come from the
// snapshot in shellsnapshot.go.
func shellPath() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "bash"
}

// startDir decides where this command begins: an explicit cwd wins, then
// whatever the previous command left behind, then evie's own directory.
// A remembered directory that has since been deleted is discarded rather
// than passed to exec, which would fail every subsequent command.
func startDir(explicit string) (string, error) {
	if explicit != "" {
		dir, err := expandHome(strings.TrimSpace(explicit))
		if err != nil {
			return "", err
		}
		return dir, nil
	}

	sessionCwdMu.Lock()
	defer sessionCwdMu.Unlock()

	if sessionCwd == "" {
		return "", nil
	}
	if info, err := os.Stat(sessionCwd); err != nil || !info.IsDir() {
		sessionCwd = ""
		return "", nil
	}
	return sessionCwd, nil
}

// runBash executes one shell command and returns its combined output plus
// the exit status as plain text. A command that fails is a normal result,
// not a Go error: the model needs the stderr and the status to correct
// itself, and returning an error would bury both behind the dispatcher's
// wrapper. Go errors are reserved for "could not run it at all" — bad
// arguments, an unusable cwd, or a timeout.
func runBash(parent context.Context, args string) (string, error) {
	var params struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
		Timeout int    `json:"timeout_seconds"`
	}
	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}

	if strings.TrimSpace(params.Command) == "" {
		return "", errors.New("command must not be empty")
	}

	timeout := defaultBashTimeout
	if params.Timeout > 0 {
		timeout = time.Duration(params.Timeout) * time.Second
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	dir, err := startDir(params.Cwd)
	if err != nil {
		return "", err
	}
	if err := parent.Err(); err != nil {
		return "", err
	}

	// The shell reports where it ended up through a temp file rather than
	// through stdout, so a `cd` is observable without polluting the output
	// the model reads.
	pwdFile, err := os.CreateTemp("", "evie-pwd-*")
	if err != nil {
		return "", fmt.Errorf("create pwd file: %w", err)
	}
	pwdPath := pwdFile.Name()
	pwdFile.Close()
	defer os.Remove(pwdPath)

	// Newline-separated rather than `;`-joined so a multi-line script or a
	// trailing comment in the command can't swallow the epilogue. The exit
	// status is captured first and re-raised last, so recording the
	// directory never changes what the model sees.
	var script strings.Builder
	if snap := snapshot(parent); snap != "" {
		// `|| true` so a snapshot deleted mid-session degrades to a plain
		// shell instead of failing the command.
		fmt.Fprintf(&script, "source %s 2>/dev/null || true\n", shellQuote(snap))
		// eval, not the command inline: a shell parses an entire line before
		// running any of it, so an alias the sourced snapshot just defined
		// would not expand. eval forces the second parse where it does.
		fmt.Fprintf(&script, "eval %s\n", shellQuote(params.Command))
	} else {
		script.WriteString(params.Command + "\n")
	}
	fmt.Fprintf(&script, "__evie_status=$?\npwd -P > %s 2>/dev/null\nexit $__evie_status\n",
		shellQuote(pwdPath))

	if err := parent.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, shellPath(), "-l", "-c", script.String())
	cmd.Dir = dir

	// Killing the shell on timeout is not enough: a child it spawned
	// inherits the output pipe and keeps it open, so CombinedOutput would
	// block until that child finished on its own — a 1-second timeout on
	// `sleep 60` would take 60 seconds to return. WaitDelay bounds that wait
	// and closes the pipes.
	cmd.WaitDelay = 2 * time.Second

	// CombinedOutput interleaves stderr with stdout in the order they were
	// actually written, which is how a human reads a failing build. Separate
	// streams would put every error after every line of normal output.
	out, err := cmd.CombinedOutput()

	// Checked before the error: a timed-out command still produced whatever
	// it wrote before the kill, and that output is usually the diagnosis.
	if parent.Err() != nil {
		return "", parent.Err()
	}

	rememberCwd(pwdPath)

	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s and was killed. Partial output:\n%s", timeout, out)
	}

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		// Exit status 0.
	case errors.As(err, &exitErr):
		// The command ran and failed. Normal result, not a Go error.
	default:
		return "", fmt.Errorf("run command: %w", err)
	}

	return fmt.Sprintf("%s\nexit status: %d\n", capOutput(out), cmd.ProcessState.ExitCode()), nil
}

// rememberCwd stores wherever the command ended up as the starting point
// for the next one. A failure to read it is not worth reporting: the model
// asked for a command, not for directory bookkeeping, and the next call
// simply starts where this one did.
func rememberCwd(pwdPath string) {
	data, err := os.ReadFile(pwdPath)
	if err != nil {
		return
	}
	dir := strings.TrimSpace(string(data))
	if dir == "" {
		return
	}

	sessionCwdMu.Lock()
	defer sessionCwdMu.Unlock()
	sessionCwd = dir
}

// capOutput trims oversized output and spills the whole of it to a file the
// model can slice with head/tail/grep. Truncating in silence would have the
// model reason confidently about a result it only half saw; truncating with
// a pointer to the rest costs one line and loses nothing.
func capOutput(out []byte) string {
	if len(out) <= maxBashOutput {
		return string(out)
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("evie-output-%d.txt", os.Getpid()))
	note := fmt.Sprintf("\n\n[output trimmed: %d of %d characters shown", maxBashOutput, len(out))
	if err := os.WriteFile(path, out, 0o600); err != nil {
		note += "; the rest could not be saved]"
	} else {
		note += fmt.Sprintf("; full output saved to %s — read it with head, tail, or grep]", path)
	}

	return string(out[:maxBashOutput]) + note
}

// shellQuote wraps a path in single quotes for safe interpolation into the
// generated epilogue. Only ever used on paths we produced ourselves, but a
// temp directory can contain spaces and that is enough to matter.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
