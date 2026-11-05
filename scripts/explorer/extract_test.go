package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// newFixture copies testdata/repo into a temporary directory so extract and
// check never write into testdata, and writes the page built from its system
// map, as the vision skill's build would.
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
	hash, err := mapHash([]byte(readFile(t, root, visionMapPath)))
	if err != nil {
		t.Fatal(err)
	}
	writeVisionPage(t, root, hash)
	return root
}

// runTool runs the explorer command against root and returns its exit code
// and output.
func runTool(t *testing.T, root string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, root, fixtureDatabases, &stdout, &stderr)
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
}

func TestExtractMarksTestSupportOnlyForAPackagesOwnHelpers(t *testing.T) {
	root := newFixture(t)
	// A name ending in "test" is test support only where it sits directly
	// inside the package it helps, as internal/alpha/alphatest does.
	writeFile(t, root, "internal/latest/latest.go", "// Package latest picks the newest.\npackage latest\n")
	writeFile(t, root, "internal/contest/contest.go", "// Package contest runs contests.\npackage contest\n")
	facts := decodeFacts(t, extractFixture(t, root))

	got := map[string]bool{}
	for _, p := range facts["packages"].([]any) {
		pkg := p.(map[string]any)
		got[pkg["path"].(string)] = pkg["testSupport"].(bool)
	}
	want := map[string]bool{"internal/alpha/alphatest": true, "internal/latest": false, "internal/contest": false, "internal/alpha": false}
	for path, support := range want {
		if got[path] != support {
			t.Errorf("%s testSupport = %v, want %v", path, got[path], support)
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

func TestExtractIncludesTheExplorerSnapshotPackage(t *testing.T) {
	root := newFixture(t)
	// The package that embeds the explorer page sits beside its files, the
	// one package outside cmd/ and internal/ (system-page.spec.md). Other
	// Go files under docs/, and below the explorer directory, are not
	// packages of the system.
	writeFile(t, root, "docs/architecture/explorer/snapshot.go", "// Package explorer carries the explorer page.\npackage explorer\n")
	writeFile(t, root, "docs/architecture/explorer/test/helper.go", "// Package helper is not part of the system.\npackage helper\n")
	writeFile(t, root, "docs/other/other.go", "// Package other is not part of the system.\npackage other\n")
	writeFile(t, root, "internal/beta/snapshot.go", "package beta\n\nimport _ \"example.com/fixture/docs/architecture/explorer\"\n")
	facts := decodeFacts(t, extractFixture(t, root))

	var paths []string
	for _, p := range facts["packages"].([]any) {
		paths = append(paths, p.(map[string]any)["path"].(string))
	}
	want := "cmd/tool docs/architecture/explorer internal/alpha internal/alpha/alphatest internal/beta internal/delta internal/gamma"
	if got := strings.Join(paths, " "); got != want {
		t.Fatalf("package paths =\n%s\nwant\n%s", got, want)
	}
	if got := asJSON(t, facts["imports"]); !strings.Contains(got, `{"from":"internal/beta","to":"docs/architecture/explorer"}`) {
		t.Fatalf("imports lack internal/beta -> docs/architecture/explorer:\n%s", got)
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

// routeFacts returns the extracted routes as JSON objects keyed by
// "[METHOD ]pattern".
func routeFacts(t *testing.T, facts map[string]any) map[string]string {
	t.Helper()
	routes := map[string]string{}
	for _, r := range facts["routes"].([]any) {
		route := r.(map[string]any)
		key, _ := route["pattern"].(string)
		if method, ok := route["method"].(string); ok {
			key = method + " " + key
		}
		routes[key] = asJSON(t, route)
	}
	return routes
}

func wantRoutes(t *testing.T, got map[string]string, want map[string]string) {
	t.Helper()
	for key, route := range want {
		if got[key] != route {
			t.Errorf("route %s =\n%s\nwant\n%s", key, got[key], route)
		}
	}
}

func TestExtractLiteralRoutesWithMethodGuardAndSource(t *testing.T) {
	root := newFixture(t)
	routes := routeFacts(t, decodeFacts(t, extractFixture(t, root)))

	// The method prefix is split from the pattern; the guard is the server
	// method wrapping the handler; a conversion such as http.HandlerFunc is
	// not a guard.
	wantRoutes(t, routes, map[string]string{
		"GET /widgets": `{"guard":"requireOwner","method":"GET","package":"internal/alpha","pattern":"/widgets","source":{"file":"internal/alpha/routes.go","function":"Server.routes"}}`,
		"/health":      `{"package":"internal/alpha","pattern":"/health","source":{"file":"internal/alpha/routes.go","function":"Server.routes"}}`,
	})
}

func TestExtractRoutesExpandedFromRangesOverLiterals(t *testing.T) {
	root := newFixture(t)
	routes := routeFacts(t, decodeFacts(t, extractFixture(t, root)))

	// A literal prefix joined to the key of a range over a map literal, or
	// to the element of a range over a slice literal, gives one route per
	// key or element.
	source := `"source":{"file":"internal/alpha/routes.go","function":"Server.routes"}`
	wantRoutes(t, routes, map[string]string{
		"/files/a":         `{"package":"internal/alpha","pattern":"/files/a",` + source + `}`,
		"/files/b":         `{"package":"internal/alpha","pattern":"/files/b",` + source + `}`,
		"POST /jobs/start": `{"guard":"requireOwner","method":"POST","package":"internal/alpha","pattern":"/jobs/start",` + source + `}`,
		"POST /jobs/stop":  `{"guard":"requireOwner","method":"POST","package":"internal/alpha","pattern":"/jobs/stop",` + source + `}`,
	})
}

func TestExtractRefusesARoutePatternItCannotRead(t *testing.T) {
	for _, tc := range []struct{ name, src, function, want string }{
		{
			name:     "pattern built from a parameter",
			function: "Server.prefixed",
			src:      "package alpha\n\nimport \"net/http\"\n\nfunc (s *Server) prefixed(mux *http.ServeMux, prefix string) {\n\tmux.Handle(prefix+\"/x\", nil)\n}\n",
			want:     `prefix + "/x"`,
		},
		{
			name:     "prefix joined to the value of a range over a map",
			function: "mapped",
			src:      "package alpha\n\nimport \"net/http\"\n\nfunc mapped(mux *http.ServeMux) {\n\tfor _, p := range map[string]string{\"a\": \"/a\"} {\n\t\tmux.Handle(\"/x\"+p, nil)\n\t}\n}\n",
			want:     `"/x" + p`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newFixture(t)
			writeFile(t, root, "internal/alpha/bad_routes.go", tc.src)
			code, _, stderr := runTool(t, root, "extract")
			if code != 2 {
				t.Fatalf("extract exit %d, want 2:\n%s", code, stderr)
			}
			for _, want := range []string{"internal/alpha/bad_routes.go", tc.function, tc.want} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
		})
	}
}

func TestExtractRouteGuardsThroughLocalsAndDelegatingClosures(t *testing.T) {
	root := newFixture(t)
	routes := routeFacts(t, decodeFacts(t, extractFixture(t, root)))

	// The guard wraps the handler through a local variable, or inside a
	// closure that delegates to a guarded handler with ServeHTTP, even when
	// the closure delegates to the same local on two branches; a closure
	// that delegates to nothing has no guard.
	source := `"source":{"file":"internal/alpha/routes.go","function":"Server.routes"}`
	wantRoutes(t, routes, map[string]string{
		"/local":     `{"guard":"requireOwner","package":"internal/alpha","pattern":"/local",` + source + `}`,
		"/delegated": `{"guard":"requireOwner","package":"internal/alpha","pattern":"/delegated",` + source + `}`,
		"/twice":     `{"guard":"requireOwner","package":"internal/alpha","pattern":"/twice",` + source + `}`,
		"/open":      `{"package":"internal/alpha","pattern":"/open",` + source + `}`,
	})
}

func TestExtractRoutesOnlyFromServeMuxes(t *testing.T) {
	root := newFixture(t)
	routes := routeFacts(t, decodeFacts(t, extractFixture(t, root)))

	// A mux from NewServeMux and the package's default mux count, under any
	// import name; a Handle method on another type does not.
	wantRoutes(t, routes, map[string]string{
		"/ping":    `{"package":"internal/alpha","pattern":"/ping","source":{"file":"internal/alpha/mux.go","function":"Mux"}}`,
		"/default": `{"package":"internal/alpha","pattern":"/default","source":{"file":"internal/alpha/mux.go","function":"Default"}}`,
	})
	if route, ok := routes["not-a-route"]; ok {
		t.Errorf("a Handle call on a non-mux type was extracted: %s", route)
	}
}

// commandFacts returns the extracted commands as "binary words… (file
// function)" lines in facts order.
func commandFacts(t *testing.T, facts map[string]any) []string {
	t.Helper()
	var lines []string
	for _, c := range facts["commands"].([]any) {
		cmd := c.(map[string]any)
		words := []string{cmd["binary"].(string)}
		for _, w := range cmd["words"].([]any) {
			words = append(words, w.(string))
		}
		source := cmd["source"].(map[string]any)
		lines = append(lines, fmt.Sprintf("%s (%s %s %s)", strings.Join(words, " "), cmd["package"], source["file"], source["function"]))
	}
	return lines
}

func TestExtractCommandWordPaths(t *testing.T) {
	root := newFixture(t)
	got := commandFacts(t, decodeFacts(t, extractFixture(t, root)))

	// Earlier words come from enclosing branches, a guard that returns, the
	// left side of && and ||, a word kept in a variable, and a re-sliced
	// argument list, through calls to the package's functions and methods.
	// A path found in several functions takes its source from the one
	// nearest main. Flags and empty strings are not words.
	want := []string{
		"tool admin (cmd/tool cmd/tool/main.go run)",
		"tool admin purge (cmd/tool cmd/tool/main.go admin)",
		"tool admin reset (cmd/tool cmd/tool/main.go admin)",
		"tool admin users (cmd/tool cmd/tool/main.go admin)",
		"tool admin users list (cmd/tool cmd/tool/main.go admin)",
		"tool help (cmd/tool cmd/tool/main.go run)",
		"tool jobs (cmd/tool cmd/tool/main.go run)",
		"tool jobs start (cmd/tool cmd/tool/main.go tool.jobs)",
		"tool jobs stop (cmd/tool cmd/tool/main.go tool.jobs)",
		"tool run (cmd/tool cmd/tool/main.go run)",
		"tool run now (cmd/tool cmd/tool/main.go run)",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestExtractRefusesACommandComparisonItCannotPlace(t *testing.T) {
	for _, tc := range []struct{ name, body, function, want string }{
		{
			name:     "word at an unknown position",
			body:     "func main() {\n\tfor _, a := range os.Args[1:] {\n\t\tif a == \"go\" {\n\t\t\treturn\n\t\t}\n\t}\n}\n",
			function: "main",
			want:     `"go"`,
		},
		{
			name:     "no word known before it",
			body:     "func main() { sub(os.Args[1:]) }\n\nfunc sub(args []string) {\n\tif len(args) > 1 && args[1] == \"deep\" {\n\t\treturn\n\t}\n}\n",
			function: "sub",
			want:     `"deep"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newFixture(t)
			writeFile(t, root, "cmd/bad/main.go", "package main\n\nimport \"os\"\n\n"+tc.body)
			code, _, stderr := runTool(t, root, "extract")
			if code != 2 {
				t.Fatalf("extract exit %d, want 2:\n%s", code, stderr)
			}
			for _, want := range []string{"cmd/bad/main.go " + tc.function + ":", tc.want, "cannot place"} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr lacks %q:\n%s", want, stderr)
				}
			}
		})
	}
}
