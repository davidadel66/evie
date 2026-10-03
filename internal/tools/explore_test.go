package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExploreReadFileStaysInsideTheWorkerRoot(t *testing.T) {
	root, outside := workerRootFixture(t)
	read := func(ctx context.Context, path string) (string, error) {
		args, _ := json.Marshal(map[string]string{"path": path})
		return workerReadFile(ctx, string(args))
	}
	ctx := withWorkerRoot(context.Background(), root)
	for _, path := range []string{"a.txt", "sub/b.txt", "link-in", filepath.Join(root, "notes.txt"), "sub/../a.txt"} {
		got, err := read(ctx, path)
		if err != nil || !strings.Contains(got, "\t") {
			t.Fatalf("read %q = %q, %v", path, got, err)
		}
	}
	for _, c := range []struct{ path, want string }{
		{"../sibling.txt", "outside"},
		{filepath.Join(outside, "secret.txt"), "outside"},
		{"link-out", "outside"},
		{"link-dir-out/secret.txt", "outside"},
		{"/etc/hosts", "outside"},
		{"~/.zshrc", "~"},
		{".env", "off-limits"},
		{"sub/.env.dev", "off-limits"},
		{"sub", "directory"},
		{"", "empty"},
	} {
		if got, err := read(ctx, c.path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("read %q = %q, %v; want refusal naming %q", c.path, got, err, c.want)
		}
	}
	if _, err := read(context.Background(), "a.txt"); err == nil || !strings.Contains(err.Error(), "explore worker") {
		t.Fatalf("unbound read_file = %v", err)
	}
}

func TestResolveWorkerRootStaysInsideTheParentFolder(t *testing.T) {
	root, outside := workerRootFixture(t)
	for _, c := range []struct{ base, declared, want string }{
		{root, "", root},
		{root, "sub", filepath.Join(root, "sub")},
		{root, filepath.Join(root, "sub"), filepath.Join(root, "sub")},
		{"", outside, outside},
		{"", root + "/sub/..", root},
	} {
		got, err := ResolveWorkerRoot(c.base, c.declared)
		if err != nil || got != c.want {
			t.Fatalf("ResolveWorkerRoot(%q, %q) = %q, %v; want %q", c.base, c.declared, got, err, c.want)
		}
	}
	for _, c := range []struct{ base, declared, want string }{
		{root, "..", "outside"},
		{root, outside, "outside"},
		{root, "link-dir-out", "outside"},
		{root, "a.txt", "not a directory"},
		{root, "missing", "unavailable"},
		{"", "", "declare an absolute path"},
		{"", "relative", "absolute"},
		{"", "~/.ssh", ""},
	} {
		if got, err := ResolveWorkerRoot(c.base, c.declared); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("ResolveWorkerRoot(%q, %q) = %q, %v; want refusal naming %q", c.base, c.declared, got, err, c.want)
		}
	}
}

// A recursive search could read anything below the root, so a root that
// contains a secret location (~/.ssh, ~/.aws, ~/.gnupg, Evie's own database
// and procedures) is refused outright, as is one inside such a location.
func TestResolveWorkerRootRefusesFoldersHoldingSecretLocations(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	for _, dir := range []string{".aws", ".evie", "code/project"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, declared := range []string{home, filepath.Join(home, ".evie"), filepath.Join(home, ".aws")} {
		if got, err := ResolveWorkerRoot("", declared); err == nil || !strings.Contains(err.Error(), "off-limits") {
			t.Fatalf("ResolveWorkerRoot(%q) = %q, %v; want a secrets refusal", declared, got, err)
		}
	}
	if got, err := ResolveWorkerRoot(home, ""); err == nil || !strings.Contains(err.Error(), "off-limits") {
		t.Fatalf("a parent folder holding secrets became a worker root: %q, %v", got, err)
	}
	project := filepath.Join(home, "code", "project")
	if got, err := ResolveWorkerRoot("", project); err != nil || got != project {
		t.Fatalf("project folder refused: %q, %v", got, err)
	}
}

// Binding a root changes execution only: the schemas, and so the receipt,
// stay exactly the explore preset's.
func TestExploreToolsetBindsARootWithoutChangingSchemas(t *testing.T) {
	root, _ := workerRootFixture(t)
	base := ExploreToolset()
	bound := base.WithWorkerRoot(root)
	if !reflect.DeepEqual(base.Schemas(), bound.Schemas()) {
		t.Fatal("binding a root changed the schemas")
	}
	var names []string
	for _, schema := range bound.Schemas() {
		names = append(names, schema.Function.Name)
	}
	if !reflect.DeepEqual(names, ExploreToolNames) {
		t.Fatalf("explore tools = %v", names)
	}
	for _, tool := range bound.tools {
		if tool.NeedsApproval || tool.Prepare != nil {
			t.Fatalf("explore tool %s is approval-gated or prepared", tool.Schema.Function.Name)
		}
	}
	got, err := bound.tools[2].Execute(context.Background(), `{"command":"cat a.txt"}`)
	if err != nil || !strings.Contains(got, "needle one") {
		t.Fatalf("bound bash = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "a.txt")); err != nil {
		t.Fatal(err)
	}
}
