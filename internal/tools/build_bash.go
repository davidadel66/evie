package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/openrouter"
)

// sandboxedBashTool is a build worker's bash: a real shell, confined by the
// OS sandbox to writing in its worktree and private temp folder, with no
// network. It is part of the build toolset only where a sandbox works.
var sandboxedBashTool = openrouter.Tool{
	Type: "function",
	Function: openrouter.Function{
		Name: "bash",
		Description: `Run a shell command in your worktree, inside an OS sandbox: it can write only in the worktree and in $TMPDIR (your private temp folder), it has no network access, and it cannot read credential files. Use it to build, run tests and linters, and inspect the code (rg, ls, git status/diff/log).

Do not run git commands that write (commit, add, checkout, switch, branch, reset, stash, merge, rebase, push): the worktree's git data is read-only to you, and Evie's harness commits your changes on your branch when you finish. Dependencies that are not already installed or cached cannot be downloaded.

Every call starts in the worktree root, or in cwd, which must be inside the worktree. Output is stdout and stderr followed by the exit status; a non-zero exit is information, not an error. Output past 30000 characters is cut and the rest saved to a file in $TMPDIR; read it with head, tail or rg. Commands are killed after 2 minutes by default; raise timeout_seconds (up to 600) for builds and test suites. Do not start servers or anything that waits for input.`,
		Parameters: openrouter.Parameter{
			Type:     "object",
			Required: []string{"command"},
			Properties: map[string]openrouter.Property{
				"command":         {Type: "string", Description: "The shell command to run. May be a multi-line script."},
				"cwd":             {Type: "string", Description: "Folder to start in, inside the worktree: relative to the worktree root, or absolute. Defaults to the worktree root."},
				"timeout_seconds": {Type: "integer", Description: "Seconds before the command is killed. Defaults to 120, maximum 600."},
			},
		},
	},
}

// runSandboxedBash runs one command under the host's OS sandbox, or refuses:
// there is never an unsandboxed fallback.
func runSandboxedBash(parent context.Context, args string) (string, error) {
	fence, err := buildFenceFrom(parent)
	if err != nil {
		return "", err
	}
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
	sandbox, err := DetectWorkerSandbox()
	if err != nil {
		return "", fmt.Errorf("refused: build workers run bash only inside an OS sandbox, and none is available (%v), so nothing ran", err)
	}
	if info, err := os.Stat(fence.Temp); err != nil || !info.IsDir() {
		return "", fmt.Errorf("the worker's temp folder %q is unavailable", fence.Temp)
	}
	cwd := fence.Worktree
	if strings.TrimSpace(params.Cwd) != "" {
		if cwd, err = workerDirectory(fence.Worktree, fence.Worktree, strings.TrimSpace(params.Cwd)); err != nil {
			return "", fmt.Errorf("cwd: %w", err)
		}
	}
	timeout := defaultBashTimeout
	if params.Timeout > 0 {
		timeout = min(time.Duration(params.Timeout)*time.Second, maxBashTimeout)
	}
	if err := parent.Err(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd, err := sandbox.Command(ctx, fence, "/bin/bash", "--noprofile", "--norc", "-c", params.Command)
	if err != nil {
		return "", fmt.Errorf("refused: the sandbox could not be prepared, so nothing ran: %w", err)
	}
	cmd.Dir = cwd
	cmd.Env = buildEnv(fence)
	cmd.Stdin = nil
	cmd.WaitDelay = 2 * time.Second
	inProcessGroup(cmd)
	out := &boundedOutput{dir: fence.Temp}
	cmd.Stdout = out
	cmd.Stderr = out
	err = cmd.Run()
	// Background jobs the command left behind end with it.
	killProcessGroup(cmd)
	out.finish()
	if parent.Err() != nil {
		out.discard()
		return "", parent.Err()
	}
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %s and was killed. Partial output:\n%s", timeout, out.render())
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
	default:
		out.discard()
		return "", fmt.Errorf("run command: %w", err)
	}
	return fmt.Sprintf("%s\nexit status: %d\n", out.render(), cmd.ProcessState.ExitCode()), nil
}
