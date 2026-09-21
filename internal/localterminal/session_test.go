package localterminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestNaturalShellExitCleansUpDetachedTerminalJobs(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(context.Background(), root, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	pidFile := filepath.Join(root, "child.pid")
	go func() {
		buffer := make([]byte, 8192)
		for {
			if _, err := s.Read(buffer); err != nil {
				return
			}
		}
	}()
	if err := s.Write("sh -c 'trap \"\" HUP; echo $$ > child.pid; exec sleep 60' &\n"); err != nil {
		t.Fatal(err)
	}
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		data, _ := os.ReadFile(pidFile)
		fmt.Sscanf(string(data), "%d", &pid)
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child did not start")
	}
	if err := s.Write("exit\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Exited():
	case <-time.After(3 * time.Second):
		t.Fatal("shell did not exit")
	}
	s.Close()
	deadline = time.Now().Add(3 * time.Second)
	for unix.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if unix.Kill(pid, 0) == nil {
		t.Fatalf("background job %d survived natural shell exit", pid)
	}
}

func TestTerminalStartsInFolderResizesAndStopsItsJobs(t *testing.T) {
	t.Setenv("SHELL", "/bin/bash")
	t.Setenv("HOME", t.TempDir())
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s, err := Start(ctx, root, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	output := make(chan string, 100)
	go func() {
		defer close(output)
		buffer := make([]byte, 8192)
		for {
			n, err := s.Read(buffer)
			if n > 0 {
				output <- string(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	readUntil := func(marker string) string {
		t.Helper()
		var text strings.Builder
		timer := time.NewTimer(4 * time.Second)
		defer timer.Stop()
		for {
			select {
			case part, ok := <-output:
				if !ok {
					t.Fatalf("terminal ended before %q: %s", marker, text.String())
				}
				text.WriteString(part)
				if strings.Contains(text.String(), marker) {
					return text.String()
				}
			case <-timer.C:
				t.Fatalf("missing %q: %s", marker, text.String())
			}
		}
	}
	if err := s.Write("PS1='__EVIE_PROMPT__ '; printf '\\n__ROOT__%s\\n' \"$PWD\"\n"); err != nil {
		t.Fatal(err)
	}
	readUntil("__ROOT__" + root + "\r\n__EVIE_PROMPT__ ")
	if err := s.Resize(101, 37); err != nil {
		t.Fatal(err)
	}
	if err := s.Write("stty size\n"); err != nil {
		t.Fatal(err)
	}
	readUntil("37 101")
	if err := s.Write("sh -c 'printf \"\\n__SLEEPING__\\n\"; exec sleep 60'\n"); err != nil {
		t.Fatal(err)
	}
	readUntil("\r\n__SLEEPING__\r\n")
	if err := s.Write("\x03"); err != nil {
		t.Fatal(err)
	}
	readUntil("__EVIE_PROMPT__ ")
	// Both jobs use their own process group and ignore HUP. Closing must still
	// stop them, rather than just killing the shell's original process group.
	if err := s.Write("set +H\nsh -c 'trap \"\" HUP; exec sleep 60' &\nprintf '\\n__BG__%s\\n' \"$!\"\nsh -c 'trap \"\" HUP; printf \"\\n__FG__%s\\n\" \"$$\"; exec sleep 60'\n"); err != nil {
		t.Fatal(err)
	}
	text := readUntil("__FG__")
	// Echoed command text also contains the markers; collect until numeric
	// standalone records from both actual processes have arrived.
	var background, foreground int
	deadline := time.After(4 * time.Second)
	for background == 0 || foreground == 0 {
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "__BG__") {
				fmt.Sscanf(line, "__BG__%d", &background)
			}
			if strings.HasPrefix(line, "__FG__") {
				fmt.Sscanf(line, "__FG__%d", &foreground)
			}
		}
		if background != 0 && foreground != 0 {
			break
		}
		select {
		case part := <-output:
			text += part
		case <-deadline:
			t.Fatalf("missing job pids: %s", text)
		}
	}
	cancel()
	select {
	case <-s.Closed():
	case <-time.After(4 * time.Second):
		t.Fatal("terminal cleanup did not finish")
	}
	for _, pid := range []int{s.cmd.Process.Pid, background, foreground} {
		deadline := time.Now().Add(3 * time.Second)
		for unix.Kill(pid, 0) == nil && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if err := unix.Kill(pid, 0); err == nil {
			t.Errorf("terminal process %d survived", pid)
		}
	}
}

func TestTerminalRejectsMissingOrReplacedRootAndInvalidSize(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{0, 24}, {80, 0}, {501, 24}, {80, 201}} {
		if s, err := Start(context.Background(), parent, size[0], size[1]); err == nil {
			s.Close()
			t.Fatalf("accepted size %v", size)
		}
	}
	root := filepath.Join(parent, "moved")
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	if s, err := Start(context.Background(), root, 80, 24); err == nil {
		s.Close()
		t.Fatal("accepted replaced root")
	}
}
