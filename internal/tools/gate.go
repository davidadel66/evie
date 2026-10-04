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
	"time"
	"unicode/utf8"
)

// The merge gate's commands (Stage 12 of docs/memory-architecture.md,
// 2026-10-04). Evie's harness, not a worker, runs the owner's setup command
// in a fresh build worktree before the worker starts (the only phase with
// network), and the owner's verify command on the worker's branch before and
// after its rebase (offline, like the worker). Both run under the same OS
// sandbox as the worker's bash, confined to the worktree and its private
// temp folder; without a sandbox nothing runs.

const (
	// MaxGateTailBytes bounds the output tail a gate run keeps.
	MaxGateTailBytes = 3000
	// maxGateLogBytes bounds a gate run's log file.
	maxGateLogBytes = 4 << 20
)

// GateCommand is one run of the owner's setup or verify command.
type GateCommand struct {
	Fence   BuildFence
	Command string
	// Network allows outbound connections: the setup phase only.
	Network bool
	Timeout time.Duration
	// Log, a file inside Fence.Temp, receives the output up to a bound.
	Log string
}

// GateResult is how a gate command ended.
type GateResult struct {
	ExitStatus int
	TimedOut   bool
	// Tail is the last MaxGateTailBytes of the output, cut at a line start
	// when one is near.
	Tail     string
	Duration time.Duration
}

// RunGateCommand runs c under the host's OS sandbox, or refuses: there is
// never an unsandboxed fallback. A non-zero exit or a timeout is a result;
// an error means the command could not run at all.
func RunGateCommand(parent context.Context, c GateCommand) (GateResult, error) {
	if strings.TrimSpace(c.Command) == "" {
		return GateResult{}, errors.New("the command is empty")
	}
	if c.Timeout <= 0 {
		return GateResult{}, errors.New("a gate command needs a time limit")
	}
	if err := checkFence(c.Fence); err != nil {
		return GateResult{}, err
	}
	if c.Log == "" || filepath.Dir(c.Log) != c.Fence.Temp {
		return GateResult{}, fmt.Errorf("the log %q must be a file in the attempt's temp folder %s", c.Log, c.Fence.Temp)
	}
	sandbox, err := DetectWorkerSandbox()
	if err != nil {
		return GateResult{}, fmt.Errorf("refused: Evie runs setup and verify commands only inside an OS sandbox, and none is available (%v), so nothing ran", err)
	}
	if err := parent.Err(); err != nil {
		return GateResult{}, err
	}
	// The temp folder is the worker's: whatever it left at the log's name
	// (a link, say) is removed, never followed, and a fresh file created.
	if err := os.Remove(c.Log); err != nil && !errors.Is(err, os.ErrNotExist) {
		return GateResult{}, fmt.Errorf("replace the log: %w", err)
	}
	log, err := os.OpenFile(c.Log, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return GateResult{}, fmt.Errorf("open the log: %w", err)
	}
	ctx, cancel := context.WithTimeout(parent, c.Timeout)
	defer cancel()
	prepare := sandbox.Command
	env := buildEnv(c.Fence)
	if c.Network {
		prepare, env = sandbox.SetupCommand, setupEnv(c.Fence)
	}
	cmd, err := prepare(ctx, c.Fence, "/bin/bash", "--noprofile", "--norc", "-c", c.Command)
	if err != nil {
		log.Close()
		return GateResult{}, fmt.Errorf("refused: the sandbox could not be prepared, so nothing ran: %w", err)
	}
	cmd.Dir = c.Fence.Worktree
	cmd.Env = env
	cmd.Stdin = nil
	cmd.WaitDelay = 2 * time.Second
	inProcessGroup(cmd)
	out := &gateOutput{log: log}
	cmd.Stdout, cmd.Stderr = out, out
	started := time.Now()
	runErr := cmd.Run()
	killProcessGroup(cmd)
	result := GateResult{Duration: time.Since(started), Tail: out.tail()}
	if err := log.Close(); err != nil && runErr == nil {
		runErr = err
	}
	if parent.Err() != nil {
		return result, parent.Err()
	}
	if ctx.Err() == context.DeadlineExceeded {
		result.TimedOut, result.ExitStatus = true, -1
		return result, nil
	}
	var exitErr *exec.ExitError
	switch {
	case runErr == nil:
	case errors.As(runErr, &exitErr):
	default:
		return result, fmt.Errorf("run the command: %w", runErr)
	}
	result.ExitStatus = cmd.ProcessState.ExitCode()
	return result, nil
}

// gateOutput writes the output to the log up to maxGateLogBytes and keeps
// a tail of it in memory.
type gateOutput struct {
	mu     sync.Mutex
	log    *os.File
	logged int64
	ring   []byte
}

// Write never fails: a full or failing log must not kill the command.
func (o *gateOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if room := maxGateLogBytes - o.logged; room > 0 {
		chunk := p
		if int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		n, _ := o.log.Write(chunk)
		o.logged += int64(n)
	}
	o.ring = append(o.ring, p...)
	if len(o.ring) > 4*MaxGateTailBytes {
		o.ring = append([]byte(nil), o.ring[len(o.ring)-MaxGateTailBytes:]...)
	}
	return len(p), nil
}

// tail is the last MaxGateTailBytes of the output as valid text, starting
// at a line when one starts near the cut.
func (o *gateOutput) tail() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	b := o.ring
	if len(b) > MaxGateTailBytes {
		b = b[len(b)-MaxGateTailBytes:]
		if i := strings.IndexByte(string(b[:min(len(b), 200)]), '\n'); i >= 0 {
			b = b[i+1:]
		}
		for len(b) > 0 && !utf8.RuneStart(b[0]) {
			b = b[1:]
		}
	}
	return strings.ToValidUTF8(string(b), "�")
}
