package tools

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// lanAddress is one of this host's non-loopback IPv4 addresses, or "".
func lanAddress() string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addresses {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() && !ip.IP.IsLinkLocalUnicast() {
			return ip.IP.String()
		}
	}
	return ""
}

// remoteHost is a public host the test process itself can reach, or "" when
// this machine is offline.
func remoteHost() string {
	conn, err := net.DialTimeout("tcp", "example.com:443", 3*time.Second)
	if err != nil {
		return ""
	}
	conn.Close()
	return "example.com"
}

// listen accepts connections on address and reports whether any arrived.
func listen(t *testing.T, address string) (port int, connected func() bool) {
	t.Helper()
	listener, err := net.Listen("tcp", net.JoinHostPort(address, "0"))
	if err != nil {
		t.Skipf("cannot listen on %s: %v", address, err)
	}
	t.Cleanup(func() { listener.Close() })
	var hit atomic.Bool
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			hit.Store(true)
			conn.Close()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, func() bool {
		time.Sleep(50 * time.Millisecond)
		return hit.Load()
	}
}

func gate(t *testing.T, f buildFixtureT, network bool, command string) GateResult {
	t.Helper()
	result, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: command, Network: network, Timeout: 30 * time.Second,
		Log: filepath.Join(f.temp, fmt.Sprintf("gate-%d.log", time.Now().UnixNano()))})
	if err != nil {
		t.Fatalf("gate %q: %v", command, err)
	}
	return result
}

// Stage 12: the setup phase runs the owner's setup command in the sandbox
// variant that allows outbound network, so dependencies can be fetched, but
// it still writes only in the worktree and the private temp folder and
// cannot read credentials or Evie's environment. On macOS it cannot reach
// loopback (Evie's own server, local services) either. The offline variant
// the worker and the verify step use reaches nothing.
func TestSetupSandboxHasNetworkButStaysConfined(t *testing.T) {
	requireSandbox(t)
	f := buildFixture(t)
	home := filepath.Join(filepath.Dir(f.repo), "home")
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "id_ed25519"), []byte("SSH-SECRET-SENTINEL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENROUTER_API_KEY", "sk-ENV-SECRET-SENTINEL")

	// Local listeners: on loopback, and on this host's own interface
	// address. The offline gate reaches neither; on macOS setup reaches
	// neither either (Evie's server and local services stay closed).
	locals := []string{"127.0.0.1"}
	if lan := lanAddress(); lan != "" {
		locals = append(locals, lan)
	}
	for _, address := range locals {
		port, connected := listen(t, address)
		probe := fmt.Sprintf("echo hi > /dev/tcp/%s/%d", address, port)
		if got := gate(t, f, false, probe); got.ExitStatus == 0 || connected() {
			t.Fatalf("the offline gate reached %s: exit %d %q", address, got.ExitStatus, got.Tail)
		}
		if runtime.GOOS == "darwin" {
			if got := gate(t, f, true, probe); got.ExitStatus == 0 || connected() {
				t.Fatalf("setup reached this host at %s: exit %d %q", address, got.ExitStatus, got.Tail)
			}
		}
	}
	// A remote host, through DNS: setup reaches it, the offline gate does
	// not. Skipped only when this machine itself is offline.
	_, curlErr := exec.LookPath("curl")
	if host := remoteHost(); host == "" || curlErr != nil {
		t.Log("offline or no curl: the positive network check was skipped")
	} else {
		probe := "curl -sS -m 8 -o /dev/null https://" + host + "/ && echo CONNECTED"
		if got := gate(t, f, false, probe); got.ExitStatus == 0 || strings.Contains(got.Tail, "CONNECTED") {
			t.Fatalf("the offline gate reached %s: %q", host, got.Tail)
		}
		if got := gate(t, f, true, probe); got.ExitStatus != 0 || !strings.Contains(got.Tail, "CONNECTED") {
			t.Fatalf("setup could not reach %s: exit %d %q", host, got.ExitStatus, got.Tail)
		}
	}

	if got := gate(t, f, true, "echo built > node_modules.txt && echo cached > \"$TMPDIR/cache.txt\" && pwd -P"); got.ExitStatus != 0 || !strings.Contains(got.Tail, f.worktree) {
		t.Fatalf("setup could not write its worktree and temp folder: exit %d %q", got.ExitStatus, got.Tail)
	}
	tmpProbe := filepath.Join("/tmp", fmt.Sprintf("evie-setup-probe-%d", time.Now().UnixNano()))
	for name, command := range map[string]string{
		"owner checkout": "echo pwned > " + filepath.Join(f.repo, "a.txt"),
		"outside folder": "echo pwned > " + filepath.Join(f.outside, "new.txt"),
		"home":           "echo pwned > \"$HOME/evil\"",
		"tmp":            "echo pwned > " + tmpProbe,
		"worktree .git":  "echo 'gitdir: /tmp/x' > .git",
		"main repo refs": "git -C " + f.repo + " update-ref refs/heads/evil HEAD",
	} {
		if got := gate(t, f, true, command); got.ExitStatus == 0 {
			t.Fatalf("setup %s: %q succeeded", name, command)
		}
	}
	if got := fileContent(t, filepath.Join(f.repo, "a.txt")); got != "alpha\nneedle one\nomega\n" {
		t.Fatalf("owner's checkout changed: %q", got)
	}
	for _, path := range []string{filepath.Join(f.outside, "new.txt"), filepath.Join(home, "evil"), tmpProbe} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("%s was created", path)
		}
	}
	if got := gate(t, f, true, "cat \"$HOME/.ssh/id_ed25519\"; env"); strings.Contains(got.Tail, "SSH-SECRET-SENTINEL") || strings.Contains(got.Tail, "sk-ENV-SECRET-SENTINEL") {
		t.Fatalf("setup read a secret:\n%s", got.Tail)
	}
}

// Stage 12: a gate command's whole output goes to its log file within a
// bound, the result keeps only the tail, a non-zero exit is a result rather
// than an error, and a command past its time limit is killed as a group.
func TestGateCommandBoundsOutputAndTime(t *testing.T) {
	requireSandbox(t)
	f := buildFixture(t)
	log := filepath.Join(f.temp, "verify.log")
	result, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: "for i in $(seq 1 20000); do echo line-$i; done; echo LAST-LINE; exit 3", Timeout: 30 * time.Second, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitStatus != 3 || result.TimedOut || !strings.HasSuffix(strings.TrimSpace(result.Tail), "LAST-LINE") || len(result.Tail) > MaxGateTailBytes+200 || strings.Contains(result.Tail, "line-1\n") {
		t.Fatalf("result = exit %d timeout %v tail %d bytes %q", result.ExitStatus, result.TimedOut, len(result.Tail), result.Tail[:min(len(result.Tail), 80)])
	}
	if logged := fileContent(t, log); !strings.HasPrefix(logged, "line-1\n") || !strings.Contains(logged, "LAST-LINE") {
		t.Fatalf("log = %d bytes", len(logged))
	}
	started := time.Now()
	slow, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: "echo starting; sleep 30 & sleep 30", Timeout: 500 * time.Millisecond, Log: filepath.Join(f.temp, "slow.log")})
	if err != nil {
		t.Fatal(err)
	}
	if !slow.TimedOut || time.Since(started) > 10*time.Second || !strings.Contains(slow.Tail, "starting") {
		t.Fatalf("slow = %+v after %s", slow, time.Since(started))
	}
	// A link the worker left at the log's name is replaced, never followed.
	victim := filepath.Join(f.outside, "victim.txt")
	linked := filepath.Join(f.temp, "linked.log")
	if err := os.Symlink(victim, linked); err != nil {
		t.Fatal(err)
	}
	if _, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: "echo through-the-link", Timeout: 30 * time.Second, Log: linked}); err != nil {
		t.Fatal(err)
	}
	if got := fileContent(t, victim); got != "original\n" {
		t.Fatalf("the log followed a link out: %q", got)
	}
	if info, err := os.Lstat(linked); err != nil || !info.Mode().IsRegular() || fileContent(t, linked) != "through-the-link\n" {
		t.Fatalf("log = %v %v", info, err)
	}
	// A log outside the temp folder is refused before anything runs.
	if _, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: "true", Timeout: time.Second, Log: filepath.Join(f.repo, "x.log")}); err == nil {
		t.Fatal("a log outside the temp folder was accepted")
	}
	t.Setenv("EVIE_WORKER_SANDBOX", "off")
	if _, err := RunGateCommand(context.Background(), GateCommand{Fence: f.fence, Command: "touch unsandboxed", Timeout: time.Second, Log: filepath.Join(f.temp, "x.log")}); err == nil || !strings.Contains(err.Error(), "sandbox") {
		t.Fatalf("a gate command ran without a sandbox: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.worktree, "unsandboxed")); !os.IsNotExist(err) {
		t.Fatal("an unsandboxed gate command wrote a file")
	}
}
