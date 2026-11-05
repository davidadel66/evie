package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureData = "docs/architecture/explorer/data/"

// checkFixture runs check on root and returns its exit code and output.
func checkFixture(t *testing.T, root string) (int, string) {
	t.Helper()
	code, stdout, stderr := runTool(t, root, "check")
	return code, stdout + stderr
}

// writeFile writes a file under root, creating its directory.
func writeFile(t *testing.T, root, name, text string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// editData rewrites one wrapped data file through a JSON edit.
func editData(t *testing.T, root, name string, edit func(map[string]any)) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	var value map[string]any
	if err := json.Unmarshal([]byte(strings.Join(lines[1:len(lines)-1], "\n")), &value); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	edit(value)
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, name, lines[0]+"\n"+string(body)+"\n"+lines[len(lines)-1]+"\n")
}

// node returns the content node with id from a decoded content file.
func node(t *testing.T, content map[string]any, id string) map[string]any {
	t.Helper()
	for _, n := range content["nodes"].([]any) {
		if m := n.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("no node %s", id)
	return nil
}

func statement(text string, refs ...string) map[string]any {
	list := []any{}
	for _, ref := range refs {
		list = append(list, ref)
	}
	return map[string]any{"text": text, "refs": list}
}

func appendTo(m map[string]any, key string, items ...any) {
	list, _ := m[key].([]any)
	m[key] = append(list, items...)
}

func TestCheckPassesOnConsistentData(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	code, out := checkFixture(t, root)
	if code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

func TestCheckReportsStaleFactsWithTheExtractCommand(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	writeFile(t, root, "docs/adr/0003-third-decision.md", "# Make a third decision\n")

	code, out := checkFixture(t, root)
	if code == 0 {
		t.Fatalf("check exit 0, want non-zero:\n%s", out)
	}
	for _, want := range []string{factsPath, "differ from a fresh extraction", "go run ./scripts/explorer extract"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// checkCase breaks the consistent fixture in one way and names what check
// must report.
type checkCase struct {
	name   string
	break_ func(t *testing.T, root string)
	want   []string
}

// runCheckCases runs check cases on the consistent fixture.
func runCheckCases(t *testing.T, cases []checkCase) {
	t.Helper()
	runCheckCasesOn(t, func(t *testing.T) string {
		root := newFixture(t)
		extractFixture(t, root)
		return root
	}, cases)
}

// runCheckCasesOn runs each case on a fresh repository from newRoot, which
// check passes, and wants check to fail with every one of the case's texts.
func runCheckCasesOn(t *testing.T, newRoot func(*testing.T) string, cases []checkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRoot(t)
			tc.break_(t, root)
			code, out := checkFixture(t, root)
			if code != 1 {
				t.Fatalf("check exit %d, want 1:\n%s", code, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}

func editCore(edit func(core map[string]any)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		editData(t, root, fixtureData+"core.js", edit)
	}
}

func TestCheckReportsManifestAndFormatProblems(t *testing.T) {
	runCheckCases(t, []checkCase{
		{
			name: "data file not listed in the manifest",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, fixtureData+"extra.js",
					"evieExplorer.register(\n{\"format\": 1, \"kind\": \"content\", \"owner\": \"extra\"}\n);\n")
			},
			want: []string{fixtureData + "extra.js", "not listed in the manifest"},
		},
		{
			name: "data file listed but missing",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, fixtureData+"manifest.js",
					"evieExplorer.manifest(\n[\"facts.js\", \"system.js\", \"core.js\", \"proposals/sample.js\", \"missing.js\"]\n);\n")
			},
			want: []string{manifestPath, "missing.js", "does not exist"},
		},
		{
			name: "manifest path outside the data directory",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, fixtureData+"manifest.js",
					"evieExplorer.manifest(\n[\"facts.js\", \"system.js\", \"core.js\", \"proposals/sample.js\", \"../escape.js\"]\n);\n")
			},
			want: []string{manifestPath, "../escape.js", "malformed"},
		},
		{
			name: "body is not strict JSON",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, fixtureData+"core.js",
					"evieExplorer.register(\n{\"format\": 1, \"kind\": \"content\", \"owner\": \"core\",}\n);\n")
			},
			want: []string{fixtureData + "core.js", "malformed"},
		},
		{
			name: "wrong first line",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, fixtureData+"core.js",
					"register(\n{\"format\": 1, \"kind\": \"content\", \"owner\": \"core\"}\n);\n")
			},
			want: []string{fixtureData + "core.js", "malformed", "evieExplorer.register("},
		},
		{
			name: "unknown field",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/gamma")["sumary"] = "typo"
			}),
			want: []string{fixtureData + "core.js", "malformed", "sumary"},
		},
		{
			name: "field with the wrong case",
			break_: editCore(func(core map[string]any) {
				n := node(t, core, "pkg:internal/gamma")
				n["Title"] = n["title"]
				delete(n, "title")
			}),
			want: []string{fixtureData + "core.js", "malformed", "Title"},
		},
		{
			name: "unsupported format version",
			break_: editCore(func(core map[string]any) {
				core["format"] = 2
			}),
			want: []string{fixtureData + "core.js", "format 2"},
		},
		{
			name: "unknown kind",
			break_: editCore(func(core map[string]any) {
				core["kind"] = "notes"
			}),
			want: []string{fixtureData + "core.js", `"notes"`},
		},
		{
			name: "value outside its enumeration",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/gamma")["level"] = "galaxy"
			}),
			want: []string{fixtureData + "core.js", "node pkg:internal/gamma", `"galaxy"`},
		},
		{
			name: "missing required field",
			break_: editCore(func(core map[string]any) {
				delete(node(t, core, "pkg:internal/gamma"), "title")
			}),
			want: []string{fixtureData + "core.js", "node pkg:internal/gamma", "title"},
		},
	})
}

func TestCheckReportsPackageCoverageProblems(t *testing.T) {
	runCheckCases(t, []checkCase{
		{
			name: "package with no node",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "internal/zeta/zeta.go", "package zeta\n")
				extractFixture(t, root)
			},
			want: []string{factsPath, "package internal/zeta", `"pkg:internal/zeta"`},
		},
		{
			name: "node for a package that no longer exists",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/gamma")["id"] = "pkg:internal/ghost"
			}),
			want: []string{fixtureData + "core.js", "node pkg:internal/ghost", "no package internal/ghost"},
		},
		{
			name: "duplicate node id across files",
			break_: func(t *testing.T, root string) {
				editData(t, root, fixtureData+"system.js", func(system map[string]any) {
					node(t, system, "owner")["id"] = "pkg:internal/beta"
				})
			},
			want: []string{fixtureData + "system.js", "node pkg:internal/beta", "duplicate", fixtureData + "core.js"},
		},
		{
			name: "duplicate child id",
			break_: editCore(func(core map[string]any) {
				alpha := node(t, core, "pkg:internal/alpha")
				child := alpha["children"].([]any)[0].(map[string]any)
				child["id"] = "pkg:internal/delta"
			}),
			want: []string{"node pkg:internal/delta", "duplicate"},
		},
		{
			name: "component node without a group",
			break_: editCore(func(core map[string]any) {
				delete(node(t, core, "pkg:internal/delta"), "group")
			}),
			want: []string{fixtureData + "core.js", "node pkg:internal/delta", "missing group"},
		},
		{
			name: "group outside its enumeration",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/beta")["alsoIn"] = []any{"plumbing"}
			}),
			want: []string{"node pkg:internal/beta", `alsoIn "plumbing"`},
		},
		{
			name: "package node at the wrong level",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/delta")["level"] = "container"
			}),
			want: []string{"node pkg:internal/delta", "component"},
		},
	})
}

// citeFromAlpha adds a gotcha on alpha that cites ref.
func citeFromAlpha(t *testing.T, ref string) func(*testing.T, string) {
	return editCore(func(core map[string]any) {
		appendTo(node(t, core, "pkg:internal/alpha"), "gotchas", statement("Cites something.", ref))
	})
}

func TestCheckReportsDeadReferences(t *testing.T) {
	cases := []checkCase{}
	for _, tc := range []struct{ name, ref, why string }{
		{"file", "code:internal/alpha/missing.go", "does not exist"},
		{"directory escape", "code:../outside.go", "inside the repository"},
		{"Go symbol", "code:internal/alpha/alpha.go#Missing", "declares no Missing"},
		{"Go method", "code:internal/alpha/alpha.go#Server.Stop", "declares no Server.Stop"},
		{"symbol in a non-Go file", "code:docs/guide.md#Guide", "Go file"},
		{"heading", "doc:docs/guide.md#no-such-heading", "no heading"},
		{"heading in a code fence", "doc:docs/guide.md#not-a-heading", "no heading"},
		{"heading in a fence closed only by a longer fence", "doc:docs/guide.md#inside-a-longer-fence", "no heading"},
		{"heading in a tilde fence closed only by a longer fence", "doc:docs/guide.md#inside-a-four-tilde-fence", "no heading"},
		{"heading in a tilde fence holding backticks", "doc:docs/guide.md#inside-a-tilde-fence", "no heading"},
		{"list item above a thematic break", "doc:docs/guide.md#a-list-item", "no heading"},
		{"table row above a thematic break", "doc:docs/guide.md#-a--b-", "no heading"},
		{"front matter", "doc:docs/adr/0001-first-decision.md#status-accepted", "no heading"},
		{"doc that is a directory", "doc:docs", "not a file"},
		{"ADR", "adr:0099", "no ADR 0099"},
		{"glossary term", "term:Gadget", "no glossary term"},
		{"table", "table:fixture.gadgets", "no table"},
		{"table in another database", "table:ghost.widgets", "no table"},
		{"route", "route:GET /gadgets", "no route"},
		{"route with the wrong method", "route:POST /widgets", "no route"},
		{"route with two spaces after its method", "route:GET  /widgets", "want route:/pattern or route:METHOD /pattern"},
		{"route with a lower-case method", "route:get /widgets", "want route:/pattern or route:METHOD /pattern"},
		{"route without a leading slash", "route:widgets", "want route:/pattern or route:METHOD /pattern"},
		{"command", "command:tool fly", "no command"},
		{"command with an extra word", "command:tool run fast", "no command"},
		{"command without a binary", "command: ", "names no binary"},
		{"package", "pkg:internal/ghost", "no package"},
		{"node", "node:missing-node", "no node"},
		{"unknown kind", "url:https://example.com", "unknown reference kind"},
	} {
		cases = append(cases, checkCase{
			name:   tc.name,
			break_: citeFromAlpha(t, tc.ref),
			want:   []string{fixtureData + "core.js", "node pkg:internal/alpha gotchas 2", tc.ref, tc.why},
		})
	}
	cases = append(cases,
		checkCase{
			name: "node glossary term",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/alpha"), "terms", "Gizmo")
			}),
			want: []string{"node pkg:internal/alpha terms", "term:Gizmo"},
		},
		checkCase{
			name: "node link",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/alpha"), "links", "adr:0042")
			}),
			want: []string{"node pkg:internal/alpha links", "adr:0042"},
		},
		checkCase{
			name: "contained node",
			break_: func(t *testing.T, root string) {
				editData(t, root, fixtureData+"system.js", func(system map[string]any) {
					appendTo(node(t, system, "tool-cli"), "contains", "pkg:internal/ghost")
				})
			},
			want: []string{fixtureData + "system.js", "node tool-cli contains", "node:pkg:internal/ghost"},
		},
		checkCase{
			name: "statement with no source",
			break_: editCore(func(core map[string]any) {
				node(t, core, "pkg:internal/delta")["summary"] = statement("Delta, unsourced.")
			}),
			want: []string{fixtureData + "core.js", "node pkg:internal/delta summary", "no source"},
		},
		checkCase{
			name: "child statement with no source",
			break_: editCore(func(core map[string]any) {
				alpha := node(t, core, "pkg:internal/alpha")
				child := alpha["children"].([]any)[0].(map[string]any)
				child["summary"] = statement("Server, unsourced.")
			}),
			want: []string{"node part:alpha-server summary", "no source"},
		},
	)
	runCheckCases(t, cases)
}

func editSystem(edit func(system map[string]any)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		editData(t, root, fixtureData+"system.js", edit)
	}
}

func runFlow(t *testing.T, system map[string]any) map[string]any {
	t.Helper()
	return system["flows"].([]any)[0].(map[string]any)
}

func TestCheckReportsEdgeAndFlowProblems(t *testing.T) {
	runCheckCases(t, []checkCase{
		{
			name: "import edge the import graph does not contain",
			break_: editCore(func(core map[string]any) {
				appendTo(core, "edges", map[string]any{"from": "pkg:internal/beta", "to": "pkg:internal/alpha", "label": "backwards", "via": "import"})
			}),
			want: []string{fixtureData + "core.js", "edge pkg:internal/beta -> pkg:internal/alpha (import)", "internal/beta does not import internal/alpha"},
		},
		{
			name: "import edge between nodes that are not packages",
			break_: editSystem(func(system map[string]any) {
				appendTo(system, "edges", map[string]any{"from": "owner", "to": "tool-cli", "label": "imports?", "via": "import"})
			}),
			want: []string{fixtureData + "system.js", "edge owner -> tool-cli (import)", "two package nodes"},
		},
		{
			name: "edge with no source",
			break_: editCore(func(core map[string]any) {
				core["edges"].([]any)[1].(map[string]any)["refs"] = []any{}
			}),
			want: []string{"edge pkg:internal/alpha -> pkg:internal/gamma (call)", "no source"},
		},
		{
			name: "edge to a missing node",
			break_: editCore(func(core map[string]any) {
				appendTo(core, "edges", map[string]any{"from": "pkg:internal/alpha", "to": "nowhere", "label": "x", "via": "call", "refs": []any{"code:internal/alpha"}})
			}),
			want: []string{"edge pkg:internal/alpha -> nowhere (call)", "node:nowhere"},
		},
		{
			name: "edge via outside its enumeration",
			break_: editCore(func(core map[string]any) {
				core["edges"].([]any)[1].(map[string]any)["via"] = "telepathy"
			}),
			want: []string{"edge pkg:internal/alpha -> pkg:internal/gamma (telepathy)", `via "telepathy"`},
		},
		{
			name: "flow step the import graph does not contain",
			break_: editSystem(func(system map[string]any) {
				appendTo(runFlow(t, system), "steps", map[string]any{"from": "pkg:internal/alpha", "to": "pkg:cmd/tool", "label": "calls back", "kind": "call", "via": "import", "refs": []any{"code:internal/alpha/alpha.go"}})
			}),
			want: []string{fixtureData + "system.js", "flow run step 4", "internal/alpha does not import cmd/tool"},
		},
		{
			name: "flow step with no source",
			break_: editSystem(func(system map[string]any) {
				runFlow(t, system)["steps"].([]any)[2].(map[string]any)["refs"] = []any{}
			}),
			want: []string{"flow run step 3", "no source"},
		},
		{
			name: "flow step that cites no code",
			break_: editSystem(func(system map[string]any) {
				runFlow(t, system)["steps"].([]any)[2].(map[string]any)["refs"] = []any{"command:tool run"}
			}),
			want: []string{fixtureData + "system.js", "flow run step 3", "the step cites no code", "code:"},
		},
		{
			name: "flow step between non-participants",
			break_: editSystem(func(system map[string]any) {
				appendTo(runFlow(t, system), "steps", map[string]any{"from": "pkg:cmd/tool", "to": "owner", "label": "reports", "kind": "async", "refs": []any{"code:cmd/tool/main.go"}})
			}),
			want: []string{"flow run step 4", "owner is not a participant"},
		},
		{
			name: "flow participant that is not a node",
			break_: editSystem(func(system map[string]any) {
				appendTo(runFlow(t, system), "participants", "ghost-node")
			}),
			want: []string{"flow run participants", "node:ghost-node"},
		},
		{
			name: "flow without a trigger",
			break_: editSystem(func(system map[string]any) {
				delete(runFlow(t, system), "trigger")
			}),
			want: []string{"flow run", "missing trigger"},
		},
		{
			name: "flow step kind outside its enumeration",
			break_: editSystem(func(system map[string]any) {
				runFlow(t, system)["steps"].([]any)[0].(map[string]any)["kind"] = "shout"
			}),
			want: []string{"flow run step 1", `kind "shout"`},
		},
		{
			name: "duplicate flow id",
			break_: editCore(func(core map[string]any) {
				appendTo(core, "flows", map[string]any{
					"id": "run", "title": "Again",
					"summary":      statement("Again.", "code:cmd/tool/main.go"),
					"trigger":      statement("Again.", "code:cmd/tool/main.go"),
					"participants": []any{"pkg:cmd/tool"},
					"steps":        []any{map[string]any{"from": "pkg:cmd/tool", "to": "pkg:cmd/tool", "label": "x", "kind": "note", "refs": []any{"code:cmd/tool/main.go"}}},
				})
			}),
			want: []string{fixtureData + "core.js", "flow run", "duplicate flow id"},
		},
	})
}

// alphaDiagram returns one of alpha's diagrams by id.
func alphaDiagram(t *testing.T, core map[string]any, id string) map[string]any {
	t.Helper()
	for _, d := range node(t, core, "pkg:internal/alpha")["diagrams"].([]any) {
		if m := d.(map[string]any); m["id"] == id {
			return m
		}
	}
	t.Fatalf("no diagram %s", id)
	return nil
}

func TestCheckReportsDiagramProblems(t *testing.T) {
	const at = "node pkg:internal/alpha diagram "
	runCheckCases(t, []checkCase{
		{
			name: "graph node pointing at a missing explorer node",
			break_: editCore(func(core map[string]any) {
				appendTo(alphaDiagram(t, core, "alpha-graph"), "nodes", map[string]any{"id": "g", "label": "ghost", "node": "pkg:internal/ghost"})
			}),
			want: []string{fixtureData + "core.js", at + "alpha-graph node g", "node:pkg:internal/ghost"},
		},
		{
			name: "graph edge to an unknown local node",
			break_: editCore(func(core map[string]any) {
				appendTo(alphaDiagram(t, core, "alpha-graph"), "edges", map[string]any{"from": "a", "to": "zz", "label": "x"})
			}),
			want: []string{at + "alpha-graph edge 2", "no local node zz"},
		},
		{
			name: "graph node in an unknown lane",
			break_: editCore(func(core map[string]any) {
				appendTo(alphaDiagram(t, core, "alpha-graph"), "nodes", map[string]any{"id": "c", "label": "c", "lane": "nowhere"})
			}),
			want: []string{at + "alpha-graph node c", "no lane nowhere"},
		},
		{
			name: "graph edge with a dead reference",
			break_: editCore(func(core map[string]any) {
				appendTo(alphaDiagram(t, core, "alpha-graph"), "edges", map[string]any{"from": "a", "to": "b", "label": "x", "refs": []any{"adr:0077"}})
			}),
			want: []string{at + "alpha-graph edge 2", "adr:0077"},
		},
		{
			name: "sequence step with no source",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-sequence")["steps"].([]any)[0].(map[string]any)["refs"] = []any{}
			}),
			want: []string{at + "alpha-sequence step 1", "no source"},
		},
		{
			name: "sequence step that cites no code",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-sequence")["steps"].([]any)[0].(map[string]any)["refs"] = []any{"pkg:internal/alpha"}
			}),
			want: []string{at + "alpha-sequence step 1", "the step cites no code"},
		},
		{
			name: "state diagram initial state that is not a state",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-states")["initial"] = "asleep"
			}),
			want: []string{at + "alpha-states", "initial state asleep is not a state"},
		},
		{
			name: "state with no source",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-states")["states"].([]any)[0].(map[string]any)["refs"] = []any{}
			}),
			want: []string{at + "alpha-states state idle", "no source"},
		},
		{
			name: "transition with no source",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-states")["transitions"].([]any)[0].(map[string]any)["refs"] = []any{}
			}),
			want: []string{at + "alpha-states transition 1", "no source"},
		},
		{
			name: "transition to an unknown state",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-states")["transitions"].([]any)[0].(map[string]any)["to"] = "gone"
			}),
			want: []string{at + "alpha-states transition 1", "no state gone"},
		},
		{
			name: "er diagram over a database the facts lack",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/alpha"), "diagrams", map[string]any{
					"id": "alpha-er", "title": "Tables", "kind": "er",
					"summary":  statement("Alpha's tables.", "code:internal/alpha"),
					"database": "ghost", "tables": []any{"widgets"},
				})
			}),
			want: []string{at + "alpha-er", "no database ghost", "table:ghost.widgets"},
		},
		{
			name: "er diagram over a table the facts lack",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/alpha"), "diagrams", map[string]any{
					"id": "alpha-er", "title": "Tables", "kind": "er",
					"summary":  statement("Alpha's tables.", "code:internal/alpha"),
					"database": "fixture", "tables": []any{"widgets", "gadgets"},
				})
			}),
			want: []string{at + "alpha-er", "table:fixture.gadgets"},
		},
		{
			name: "diagram kind outside its enumeration",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-graph")["kind"] = "pie"
			}),
			want: []string{at + "alpha-graph", `kind "pie"`},
		},
		{
			name: "field of another diagram kind",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-graph")["initial"] = "idle"
			}),
			want: []string{at + "alpha-graph", "initial does not belong to a graph diagram"},
		},
		{
			name: "diagram summary with no source",
			break_: editCore(func(core map[string]any) {
				alphaDiagram(t, core, "alpha-states")["summary"] = statement("Unsourced.")
			}),
			want: []string{at + "alpha-states summary", "no source"},
		},
		{
			name: "duplicate diagram id",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/beta"), "diagrams", map[string]any{
					"id": "alpha-states", "title": "Again", "kind": "state",
					"summary": statement("Again.", "code:internal/beta"),
					"states":  []any{map[string]any{"id": "s", "refs": []any{"code:internal/beta"}}},
					"initial": "s",
				})
			}),
			want: []string{"node pkg:internal/beta diagram alpha-states", "duplicate diagram id"},
		},
	})
}

func editSample(edit func(sample map[string]any)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		editData(t, root, fixtureData+"proposals/sample.js", edit)
	}
}

func TestCheckReportsProposalProblems(t *testing.T) {
	const sample = fixtureData + "proposals/sample.js"
	runCheckCases(t, []checkCase{
		{
			name: "change of a node that does not exist",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "nodes", map[string]any{"op": "change", "id": "pkg:internal/ghost", "title": "ghost"})
			}),
			want: []string{sample + ": design proposal sample change node pkg:internal/ghost:", "does not exist"},
		},
		{
			name: "change of a part node",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "nodes", map[string]any{"op": "change", "id": "part:alpha-server", "title": "Server, changed"})
			}),
			want: []string{sample, "proposal sample change node part:alpha-server", "a part of another node", "top-level nodes only"},
		},
		{
			name: "removal of a part node",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "nodes", map[string]any{"op": "remove", "id": "part:alpha-server"})
			}),
			want: []string{sample, "proposal sample remove node part:alpha-server", "top-level nodes only"},
		},
		{
			name: "added node that carries children",
			break_: editSample(func(p map[string]any) {
				p["nodes"].([]any)[0].(map[string]any)["children"] = []any{map[string]any{
					"id": "part:epsilon-core", "level": "part", "title": "Core",
					"summary": statement("The core.", "doc:docs/guide.md"),
				}}
			}),
			want: []string{sample, "proposal sample add node pkg:internal/epsilon", "children", "depth pass"},
		},
		{
			name: "changed node that sets children",
			break_: editSample(func(p map[string]any) {
				p["nodes"].([]any)[1].(map[string]any)["children"] = []any{map[string]any{
					"id": "part:alpha-client", "level": "part", "title": "Client",
					"summary": statement("The client.", "doc:docs/guide.md"),
				}}
			}),
			want: []string{sample, "proposal sample change node pkg:internal/alpha", "children", "depth pass"},
		},
		{
			name: "removal of a node that does not exist",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "nodes", map[string]any{"op": "remove", "id": "nobody"})
			}),
			want: []string{"proposal sample remove node nobody", "does not exist"},
		},
		{
			name: "added node that collides with an existing node",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "nodes", map[string]any{"op": "add", "id": "pkg:internal/beta", "level": "component", "kind": "package", "title": "beta", "group": "memory", "summary": statement("Beta again.", "doc:docs/guide.md")})
			}),
			want: []string{"proposal sample add node pkg:internal/beta", "already exists"},
		},
		{
			name: "added node with a statement that has no source",
			break_: editSample(func(p map[string]any) {
				p["nodes"].([]any)[0].(map[string]any)["summary"] = statement("Unsourced.")
			}),
			want: []string{"proposal sample add node pkg:internal/epsilon summary", "no source"},
		},
		{
			name: "added edge that collides with an import edge from the facts",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "edges", map[string]any{"op": "add", "from": "pkg:internal/alpha", "to": "pkg:internal/beta", "label": "again", "via": "import"})
			}),
			want: []string{"proposal sample add edge pkg:internal/alpha -> pkg:internal/beta (import)", "already exists"},
		},
		{
			name: "added edge to a node that neither exists nor is added",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "edges", map[string]any{"op": "add", "from": "pkg:internal/alpha", "to": "pkg:internal/omega", "label": "uses", "via": "import"})
			}),
			want: []string{"proposal sample add edge pkg:internal/alpha -> pkg:internal/omega (import)", "node:pkg:internal/omega"},
		},
		{
			name: "change of an edge that does not exist",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "edges", map[string]any{"op": "change", "from": "pkg:internal/beta", "to": "pkg:internal/alpha", "via": "call", "label": "x"})
			}),
			want: []string{"proposal sample change edge pkg:internal/beta -> pkg:internal/alpha (call)", "does not exist"},
		},
		{
			name: "removal of a flow that does not exist",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "flows", map[string]any{"op": "remove", "id": "ghost-flow"})
			}),
			want: []string{"proposal sample remove flow ghost-flow", "does not exist"},
		},
		{
			name: "added flow that collides with an existing flow",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "flows", map[string]any{"op": "add", "id": "run", "title": "Run again",
					"summary": statement("Again.", "doc:docs/guide.md"), "trigger": statement("Again.", "doc:docs/guide.md"),
					"participants": []any{"pkg:cmd/tool"},
					"steps":        []any{map[string]any{"from": "pkg:cmd/tool", "to": "pkg:cmd/tool", "label": "x", "kind": "note", "refs": []any{"doc:docs/guide.md"}}}})
			}),
			want: []string{"proposal sample add flow run", "already exists"},
		},
		{
			name: "operation outside its enumeration",
			break_: editSample(func(p map[string]any) {
				p["nodes"].([]any)[1].(map[string]any)["op"] = "rename"
			}),
			want: []string{"proposal sample", `op "rename"`},
		},
		{
			name: "status outside its enumeration",
			break_: editSample(func(p map[string]any) {
				p["status"] = "shipped"
			}),
			want: []string{"proposal sample", `status "shipped"`},
		},
		{
			name: "requirement with no source",
			break_: editSample(func(p map[string]any) {
				appendTo(p, "requirements", statement("Unsourced."))
			}),
			want: []string{"proposal sample requirements 2", "no source"},
		},
		{
			name: "dead spec reference",
			break_: editSample(func(p map[string]any) {
				p["spec"] = "doc:docs/missing.spec.md"
			}),
			want: []string{"proposal sample spec", "doc:docs/missing.spec.md"},
		},
		{
			name: "missing proposal id",
			break_: editSample(func(p map[string]any) {
				delete(p, "id")
			}),
			want: []string{sample, "missing id"},
		},
		{
			name: "duplicate proposal id",
			break_: func(t *testing.T, root string) {
				editData(t, root, "docs/architecture/explorer/examples/example.js", func(p map[string]any) {
					p["id"] = "sample"
				})
			},
			want: []string{"docs/architecture/explorer/examples/example.js", ": design proposal sample:", "duplicate design proposal id"},
		},
		{
			name: "example proposal that removes a missing node",
			break_: func(t *testing.T, root string) {
				editData(t, root, "docs/architecture/explorer/examples/example.js", func(p map[string]any) {
					p["nodes"] = []any{map[string]any{"op": "remove", "id": "pkg:internal/ghost"}}
				})
			},
			want: []string{"docs/architecture/explorer/examples/example.js", "proposal example remove node pkg:internal/ghost", "does not exist"},
		},
		{
			name: "content in the examples directory",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "docs/architecture/explorer/examples/content.js",
					"evieExplorer.register(\n{\"format\": 1, \"kind\": \"content\", \"owner\": \"x\"}\n);\n")
			},
			want: []string{"docs/architecture/explorer/examples/content.js", "examples are design proposals"},
		},
	})
}

func TestCheckReportsEveryProblemNotJustTheFirst(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	editCore(func(core map[string]any) {
		node(t, core, "pkg:internal/delta")["summary"] = statement("Unsourced.")
	})(t, root)
	editSystem(func(system map[string]any) {
		appendTo(node(t, system, "owner"), "links", "adr:0098")
	})(t, root)
	writeFile(t, root, fixtureData+"stray.js", "evieExplorer.register(\n{\"format\": 1, \"kind\": \"content\", \"owner\": \"stray\"}\n);\n")
	writeFile(t, root, "internal/zeta/zeta.go", "package zeta\n")

	code, out := checkFixture(t, root)
	if code != 1 {
		t.Fatalf("check exit %d, want 1:\n%s", code, out)
	}
	for _, want := range []string{
		factsPath + ": facts:",
		fixtureData + "core.js: node pkg:internal/delta summary:",
		fixtureData + "system.js: node owner links:",
		fixtureData + "stray.js: manifest:",
		factsPath + ": package internal/zeta:",
		"5 problem(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if !strings.Contains(line, "problem(s)") && !strings.Contains(line, ". Fix: ") {
			t.Errorf("problem line has no fix hint: %q", line)
		}
	}
}

func TestCheckOutputIsTheSameOnEveryRun(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	editCore(func(core map[string]any) {
		appendTo(core, "edges", map[string]any{"label": "nowhere", "via": "call", "refs": []any{"code:internal/alpha"}})
	})(t, root)

	_, first := checkFixture(t, root)
	from, to := strings.Index(first, "missing from"), strings.Index(first, "missing to")
	if from < 0 || to < 0 || from > to {
		t.Fatalf("want missing from reported before missing to:\n%s", first)
	}
	for range 30 {
		if _, again := checkFixture(t, root); again != first {
			t.Fatalf("check output changed between runs:\n%s\n---\n%s", first, again)
		}
	}
}

func TestCheckReportsCRLFLineEndingsClearly(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	// A checkout that turned LF into CRLF. Each file still decodes and is
	// validated, and each is named with the line endings as the problem
	// rather than as a wrapper or facts mismatch.
	files := []string{factsPath, manifestPath, fixtureData + "core.js"}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, name, strings.ReplaceAll(string(data), "\n", "\r\n"))
	}

	code, out := checkFixture(t, root)
	if code != 1 {
		t.Fatalf("check exit %d, want 1:\n%s", code, out)
	}
	for _, name := range files {
		if !strings.Contains(out, name+": file: the file has CRLF line endings") {
			t.Errorf("output does not name %s's line endings:\n%s", name, out)
		}
	}
	for _, opaque := range []string{"first line is", "last line is", "differ from a fresh extraction"} {
		if strings.Contains(out, opaque) {
			t.Errorf("output reports %q for a line-ending problem:\n%s", opaque, out)
		}
	}
	if !strings.Contains(out, "3 problem(s)") {
		t.Errorf("want only the three line-ending problems:\n%s", out)
	}
}

func TestCheckAcceptsDesignProposalFlowStepsThatCiteTheSpecOnly(t *testing.T) {
	// A design proposal's flow describes code that does not exist yet, so
	// its steps cite the feature's spec rather than code.
	root := newFixture(t)
	extractFixture(t, root)
	editSample(func(p map[string]any) {
		appendTo(p, "flows", map[string]any{"op": "add", "id": "epsilon-run", "title": "Run epsilon",
			"summary": statement("Alpha will ask epsilon.", "doc:docs/guide.md"), "trigger": statement("A request.", "doc:docs/guide.md"),
			"participants": []any{"pkg:internal/alpha", "pkg:internal/epsilon"},
			"steps":        []any{map[string]any{"from": "pkg:internal/alpha", "to": "pkg:internal/epsilon", "label": "asks", "kind": "call", "refs": []any{"doc:docs/guide.md"}}}})
	})(t, root)

	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}
