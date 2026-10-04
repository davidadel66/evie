package workflow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testEnvironment is the review environment the notes-digest example needs:
// a read and an idempotent effect on one connection with two resources.
func testEnvironment() Environment {
	note := ObjectType(Field{Name: "id", Type: StringType()}, Field{Name: "text", Type: StringType()})
	return Environment{
		Capabilities: NewCatalog(
			CapabilityContract{
				ID: "notes.list", Contract: "1.0.0",
				Input:  ObjectType(Field{Name: "day", Type: StringType()}),
				Output: ObjectType(Field{Name: "notes", Type: ArrayType(note)}),
			},
			CapabilityContract{
				ID: "notes.append", Contract: "1.0.0", Effect: true, Idempotent: true,
				Input:  ObjectType(Field{Name: "day", Type: StringType()}, Field{Name: "text", Type: StringType()}),
				Output: ObjectType(Field{Name: "entry_id", Type: StringType()}),
			},
		),
		Access: WorkspaceAccess{Connections: []ConnectionAccess{
			{ID: "conn-notes", Resources: []string{"notebook:inbox", "notebook:digest"}},
		}},
		Workers: WorkerCeiling{Deadline: 15 * time.Minute, Tokens: 1_000_000},
	}
}

// copyDefinition copies testdata/<name> into a fresh directory so a test can
// edit it.
func copyDefinition(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", name)
	dst := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// edit replaces exactly one occurrence of old in dir/rel.
func edit(t *testing.T, dir, rel, old, replacement string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data), old); n != 1 {
		t.Fatalf("%s: %q occurs %d times, want 1", rel, old, n)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validateDir(dir string, env Environment) (*Validated, error) {
	return Validate(DirSource{Dir: dir}, env)
}

func mustValidate(t *testing.T, dir string, env Environment) *Validated {
	t.Helper()
	v, err := validateDir(dir, env)
	if err != nil {
		t.Fatalf("validate %s: %v", dir, err)
	}
	return v
}

// wantProblem asserts err lists a problem whose path or message contains
// every fragment.
func wantProblem(t *testing.T, err error, fragments ...string) {
	t.Helper()
	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("got error %v, want an InvalidError mentioning %q", err, fragments)
	}
	for _, p := range invalid.Problems {
		text := p.Path + ": " + p.Message
		matched := true
		for _, f := range fragments {
			if !strings.Contains(text, f) {
				matched = false
				break
			}
		}
		if matched {
			t.Logf("matched %s", text)
			return
		}
	}
	t.Fatalf("no problem mentions %q; problems:\n%s", fragments, invalid.Error())
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
