package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var errNativeFolderPickerUnsupported = errors.New("native folder picker requires macOS")

// Static JXA only: no owner input is inserted into executable source or args.
// NSOpenPanel's New Folder button creates directories inside this same chooser.
const workspaceFolderPickerScript = `ObjC.import('AppKit');
var app = $.NSApplication.sharedApplication;
app.setActivationPolicy($.NSApplicationActivationPolicyAccessory);
app.activateIgnoringOtherApps(true);
var panel = $.NSOpenPanel.openPanel;
panel.title = 'Choose a workspace folder';
panel.prompt = 'Choose folder';
panel.canChooseFiles = false;
panel.canChooseDirectories = true;
panel.canCreateDirectories = true;
panel.allowsMultipleSelection = false;
var response = panel.runModal;
JSON.stringify(Number(response) === Number($.NSModalResponseOK)
  ? {path: ObjC.unwrap(panel.URL.path), cancelled: false}
  : {path: '', cancelled: true});`

func chooseNativeWorkspaceFolder(ctx context.Context) (workspaceFolderChoice, error) {
	if runtime.GOOS != "darwin" {
		return workspaceFolderChoice{}, errNativeFolderPickerUnsupported
	}
	return chooseWorkspaceFolderWithRunner(ctx, runWorkspaceFolderPickerScript)
}

func runWorkspaceFolderPickerScript(ctx context.Context, script string) ([]byte, error) {
	command := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-s", "h", "-")
	return runWorkspaceFolderPickerCommand(command, script)
}

func runWorkspaceFolderPickerCommand(command *exec.Cmd, script string) ([]byte, error) {
	command.Stdin = strings.NewReader(script)
	command.Stderr = io.Discard
	// CommandContext kills the dialog's owning process on cancellation; bound
	// pipe draining too, so a child cannot keep shutdown waiting indefinitely.
	command.WaitDelay = 2 * time.Second
	return command.Output()
}

func chooseWorkspaceFolderWithRunner(ctx context.Context, run func(context.Context, string) ([]byte, error)) (workspaceFolderChoice, error) {
	output, err := run(ctx, workspaceFolderPickerScript)
	if err != nil {
		return workspaceFolderChoice{}, err
	}
	var choice workspaceFolderChoice
	if len(output) > 64*1024 || json.Unmarshal(output, &choice) != nil {
		return workspaceFolderChoice{}, errors.New("invalid native folder choice")
	}
	if choice.Cancelled {
		if choice.Path != "" {
			return workspaceFolderChoice{}, errors.New("cancelled folder choice contains a path")
		}
		return choice, nil
	}
	if !filepath.IsAbs(choice.Path) {
		return workspaceFolderChoice{}, errors.New("native folder choice must be absolute")
	}
	stat, err := os.Stat(choice.Path)
	if err != nil || !stat.IsDir() {
		return workspaceFolderChoice{}, errors.New("native folder choice is unavailable")
	}
	return choice, nil
}
