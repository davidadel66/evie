package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
)

func TestSessionWorkingFoldersIsolateCommandsAndRelativeFiles(t *testing.T) {
	a, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "note.txt"), []byte("from A"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, "note.txt"), []byte("from B"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(a, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	first, second := &Directory{}, &Directory{}
	first.SetRoot(a)
	second.SetRoot(b)
	ca := WithInvocationContext(context.Background(), InvocationContext{Directory: first})
	cb := WithInvocationContext(context.Background(), InvocationContext{Directory: second})
	execute := func(ctx context.Context, name, args string) (string, error) {
		message, failed, err := BuiltinToolset().ExecuteWithApprovalAuthorizedCompletion(ctx, openrouter.ToolCall{ID: "directory-test", Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: args}}, func(context.Context, string, string, *FileChangePreview) Decision { return Approved }, nil, nil, nil)
		if err == nil && failed {
			err = fmt.Errorf("tool failed: %s", message.Content)
		}
		return message.Content, err
	}
	for _, tc := range []struct {
		ctx  context.Context
		want string
	}{{ca, "from A"}, {cb, "from B"}} {
		got, err := execute(tc.ctx, "read_file", `{"path":"note.txt"}`)
		if err != nil || !strings.Contains(got, tc.want) {
			t.Fatalf("relative file = %q, %v", got, err)
		}
	}
	if _, err := execute(ca, "bash", `{"command":"cd nested"}`); err != nil {
		t.Fatal(err)
	}
	got, err := execute(cb, "bash", `{"command":"pwd"}`)
	if err != nil || !strings.Contains(got, b) {
		t.Fatalf("other session cwd = %q, %v", got, err)
	}
	first.SetRoot(b)
	got, err = execute(ca, "read_file", `{"path":"note.txt"}`)
	if err != nil || !strings.Contains(got, "from B") {
		t.Fatalf("replacement root = %q, %v", got, err)
	}
	first.SetRoot(filepath.Join(a, "missing"))
	if _, err := execute(ca, "read_file", `{"path":"note.txt"}`); err == nil {
		t.Fatal("missing attachment silently fell back")
	}
}

func TestWorkingFolderRejectsReplacementSymlink(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	directory := &Directory{}
	directory.SetRoot(root)
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := directory.Start(""); err == nil {
		t.Fatal("followed replaced attachment")
	}
}
