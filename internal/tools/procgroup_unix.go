//go:build unix

package tools

import (
	"os/exec"
	"syscall"
)

// inProcessGroup runs cmd as the leader of its own process group, so a
// timeout or cancellation kills everything the command started, not only
// its shell.
func inProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return nil
	}
}

// killProcessGroup kills cmd's process group, including background jobs the
// command left behind.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil && cmd.Process.Pid > 0 {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
