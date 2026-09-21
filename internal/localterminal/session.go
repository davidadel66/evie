// Package localterminal owns an owner's interactive shell and its PTY lifetime.
// It is independent of agent tools, approvals, and conversation history.
package localterminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/davidadel66/evie/internal/memory"
	"golang.org/x/sys/unix"
)

type Session struct {
	cmd            *exec.Cmd
	master         *os.File
	exited, closed chan struct{}
	closeOnce      sync.Once
	inputMu        sync.Mutex
	exitCode       int
}

func ValidSize(columns, rows int) bool {
	return columns >= 2 && columns <= 500 && rows >= 1 && rows <= 200
}

func Start(ctx context.Context, root string, columns, rows int) (*Session, error) {
	if !ValidSize(columns, rows) {
		return nil, errors.New("Terminal size must be 2–500 columns and 1–200 rows")
	}
	canonical, err := memory.CanonicalProjectRoot(root)
	if err != nil || canonical != root {
		return nil, errors.New("The attached folder is unavailable or has moved")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-l", "-i")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "PWD="+root)
	master, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
	if err != nil {
		return nil, fmt.Errorf("start terminal: %w", err)
	}
	// Rewrap a nonblocking descriptor so Go's poller can interrupt reads and
	// enforce input deadlines. PTY ioctl helpers call Fd(), which would switch
	// it back to blocking mode; later resizes use SyscallConn instead.
	fd, err := unix.Dup(int(master.Fd()))
	if err == nil {
		unix.CloseOnExec(fd)
		err = unix.SetNonblock(fd, true)
	}
	if err != nil {
		if fd >= 0 {
			unix.Close(fd)
		}
		unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		master.Close()
		cmd.Wait()
		return nil, fmt.Errorf("prepare terminal IO: %w", err)
	}
	master.Close()
	s := &Session{cmd: cmd, master: os.NewFile(uintptr(fd), "evie-terminal"), exited: make(chan struct{}), closed: make(chan struct{}), exitCode: -1}
	go func() {
		err := cmd.Wait()
		s.exitCode = 0
		if err != nil && cmd.ProcessState != nil {
			s.exitCode = cmd.ProcessState.ExitCode()
		}
		close(s.exited)
	}()
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-s.closed:
		}
	}()
	return s, nil
}

func (s *Session) Read(data []byte) (int, error) { return s.master.Read(data) }
func (s *Session) Exited() <-chan struct{}       { return s.exited }
func (s *Session) Closed() <-chan struct{}       { return s.closed }
func (s *Session) ExitCode() int                 { <-s.exited; return s.exitCode }

func (s *Session) Write(data string) error {
	if len(data) > 3072 {
		return errors.New("Terminal input is too large")
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if err := s.master.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	n, err := io.WriteString(s.master, data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func (s *Session) Resize(columns, rows int) error {
	if !ValidSize(columns, rows) {
		return errors.New("Invalid terminal dimensions")
	}
	control, err := s.master.SyscallConn()
	if err != nil {
		return err
	}
	var resizeErr error
	err = control.Control(func(fd uintptr) {
		resizeErr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Col: uint16(columns), Row: uint16(rows)})
	})
	return errors.Join(err, resizeErr)
}

// Close stops ordinary foreground and background jobs, including job-control
// groups separate from the shell. Deliberately detached new sessions are not
// part of this terminal. The master stays open until process lookup completes,
// and every PID is checked against the shell's session before signaling.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		defer close(s.closed)
		members := s.members()
		s.signalGroups(unix.SIGHUP, members)
		time.Sleep(100 * time.Millisecond)
		// The shell exiting can detach surviving jobs from the tty. Retain
		// their PIDs and validate session membership again before escalation.
		for pid, group := range s.members() {
			members[pid] = group
		}
		s.signalGroups(unix.SIGKILL, members)
		s.master.Close()
		select {
		case <-s.exited:
		case <-time.After(time.Second):
			s.cmd.Process.Kill()
		}
	})
}

func (s *Session) members() map[int]int {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", "-e", "-o", "pid=,pgid=")
	cmd.WaitDelay = 100 * time.Millisecond
	out, _ := cmd.Output()
	members := make(map[int]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		pgid, err := strconv.Atoi(fields[1])
		if err != nil || pgid <= 1 {
			continue
		}
		if sid, err := unix.Getsid(pid); err == nil && sid == s.cmd.Process.Pid {
			members[pid] = pgid
		}
	}
	if sid, err := unix.Getsid(s.cmd.Process.Pid); err == nil && sid == s.cmd.Process.Pid {
		members[s.cmd.Process.Pid] = s.cmd.Process.Pid
	}
	return members
}

func (s *Session) signalGroups(signal syscall.Signal, members map[int]int) {
	groups := make(map[int]bool)
	for pid, group := range members {
		if sid, err := unix.Getsid(pid); err == nil && sid == s.cmd.Process.Pid {
			if current, err := unix.Getpgid(pid); err == nil && current == group {
				groups[group] = true
			}
		}
	}
	for group := range groups {
		_ = unix.Kill(-group, signal)
	}
}
