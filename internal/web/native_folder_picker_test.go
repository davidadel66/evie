package web

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeFolderPickerRunnerPreservesExactPathAndRejectsInvalidOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), " chosen \"folder\"\n\t")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	want := workspaceFolderChoice{Path: path}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	choice, err := chooseWorkspaceFolderWithRunner(context.Background(), func(ctx context.Context, script string) ([]byte, error) {
		if script != workspaceFolderPickerScript {
			t.Fatal("script was not static")
		}
		return encoded, nil
	})
	if err != nil || choice != want {
		t.Fatalf("choice=%+v err=%v", choice, err)
	}
	for _, output := range []string{`{}`, `{"path":"relative"}`, `{"path":"/missing/does-not-exist"}`, `{"path":"/tmp","cancelled":true}`, `not json`} {
		if _, err := chooseWorkspaceFolderWithRunner(context.Background(), func(context.Context, string) ([]byte, error) { return []byte(output), nil }); err == nil {
			t.Fatalf("invalid output accepted: %s", output)
		}
	}
	choice, err = chooseWorkspaceFolderWithRunner(context.Background(), func(context.Context, string) ([]byte, error) { return []byte(`{"path":"","cancelled":true}`), nil })
	if err != nil || !choice.Cancelled || choice.Path != "" {
		t.Fatalf("cancel=%+v %v", choice, err)
	}
}

func TestNativeFolderPickerCommandCancellationReapsProcess(t *testing.T) {
	if os.Getenv("EVIE_FOLDER_PICKER_TEST_CHILD") == "1" {
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestNativeFolderPickerCommandCancellationReapsProcess$")
	command.Env = append(os.Environ(), "EVIE_FOLDER_PICKER_TEST_CHILD=1")
	started := time.Now()
	_, err = runWorkspaceFolderPickerCommand(command, workspaceFolderPickerScript)
	if err == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) || command.ProcessState == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("cancelled process not reaped: state=%v err=%v context=%v", command.ProcessState, err, ctx.Err())
	}
}
