package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// newFixture copies testdata/repo into a temporary directory so extract and
// check never write into testdata.
func newFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	src := filepath.Join("testdata", "repo")
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return root
}

// runTool runs the explorer command against root and returns its exit code
// and output.
func runTool(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, root, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func extractFixture(t *testing.T, root string) []byte {
	t.Helper()
	code, stdout, stderr := runTool(t, root, "extract")
	if code != 0 {
		t.Fatalf("extract exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	data, err := os.ReadFile(filepath.Join(root, factsPath))
	if err != nil {
		t.Fatalf("read facts: %v", err)
	}
	return data
}

// decodeFacts strips the fixed wrapper lines and decodes the JSON body.
func decodeFacts(t *testing.T, data []byte) map[string]any {
	t.Helper()
	text := string(data)
	first := "evieExplorer.register(\n"
	last := ");\n"
	if !strings.HasPrefix(text, first) || !strings.HasSuffix(text, last) {
		t.Fatalf("facts file is not wrapped in the registration call:\n%s", text)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(text, first), last)
	var facts map[string]any
	if err := json.Unmarshal([]byte(body), &facts); err != nil {
		t.Fatalf("facts body is not JSON: %v", err)
	}
	return facts
}

func asJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestExtractWritesPackageFacts(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	if facts["format"] != float64(1) || facts["kind"] != "facts" {
		t.Fatalf("format/kind = %v/%v, want 1/facts", facts["format"], facts["kind"])
	}
	// Keys appear sorted because the decoded objects are maps.
	want := `[` +
		`{"command":true,"doc":"Command tool drives the fixture.","name":"main","path":"cmd/tool","testSupport":false},` +
		`{"command":false,"doc":"Package alpha is the first fixture package.","name":"alpha","path":"internal/alpha","testSupport":false},` +
		`{"command":false,"doc":"Package alphatest fakes alpha for tests.","name":"alphatest","path":"internal/alpha/alphatest","testSupport":true},` +
		`{"command":false,"doc":"","name":"beta","path":"internal/beta","testSupport":false},` +
		`{"command":false,"doc":"Package delta is only imported by tests.","name":"delta","path":"internal/delta","testSupport":false},` +
		`{"command":false,"doc":"Package gamma is only imported on windows.","name":"gamma","path":"internal/gamma","testSupport":false}` +
		`]`
	if got := asJSON(t, facts["packages"]); got != want {
		t.Fatalf("packages =\n%s\nwant\n%s", got, want)
	}
	for _, key := range []string{"commands", "routes", "databases"} {
		if !reflect.DeepEqual(facts[key], []any{}) {
			t.Errorf("%s = %v, want an empty list", key, facts[key])
		}
	}
}

func TestExtractImportEdgesAreTheUnionOfNonTestFiles(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	// alpha_windows.go is build-tagged and still contributes alpha -> gamma;
	// alpha_test.go's import of delta is test-only and contributes nothing;
	// fmt is outside the module.
	want := `[` +
		`{"from":"cmd/tool","to":"internal/alpha"},` +
		`{"from":"internal/alpha","to":"internal/beta"},` +
		`{"from":"internal/alpha","to":"internal/gamma"},` +
		`{"from":"internal/alpha/alphatest","to":"internal/alpha"}` +
		`]`
	if got := asJSON(t, facts["imports"]); got != want {
		t.Fatalf("imports =\n%s\nwant\n%s", got, want)
	}
}

func TestExtractGlossaryEntries(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	want := `[` +
		`{"avoid":["Other"],"definition":"A term written with the colon inside the bold.","term":"Bold colon"},` +
		`{"avoid":[],"definition":"One recorded line.","term":"Ledger Entry"},` +
		`{"avoid":["Gadget","gizmo"],"definition":"A thing the fixture makes, spread across two lines.","term":"Widget"}` +
		`]`
	if got := asJSON(t, facts["glossary"]); got != want {
		t.Fatalf("glossary =\n%s\nwant\n%s", got, want)
	}
}

func TestExtractADRs(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	// Front matter is skipped, the first heading is the title, and files that
	// are not numbered (README.md) are not ADRs.
	want := `[` +
		`{"number":"0001","path":"docs/adr/0001-first-decision.md","title":"Make the first decision"},` +
		`{"number":"0002","path":"docs/adr/0002-second-decision.md","title":"Make the second decision"}` +
		`]`
	if got := asJSON(t, facts["adrs"]); got != want {
		t.Fatalf("adrs =\n%s\nwant\n%s", got, want)
	}
}

func TestExtractIsByteIdenticalWhenRepeated(t *testing.T) {
	root := newFixture(t)
	first := extractFixture(t, root)
	second := extractFixture(t, root)
	if !bytes.Equal(first, second) {
		t.Fatalf("second extraction differs:\n%s\n---\n%s", first, second)
	}
}
