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

func runCheckCases(t *testing.T, cases []checkCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newFixture(t)
			extractFixture(t, root)
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
		{"doc that is a directory", "doc:docs", "not a file"},
		{"ADR", "adr:0099", "no ADR 0099"},
		{"glossary term", "term:Gadget", "no glossary term"},
		{"table", "table:fixture.widgets", "no table"},
		{"route", "route:GET /widgets", "no route"},
		{"command", "command:tool run", "no command"},
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
			name: "er diagram over a database and table the facts lack",
			break_: editCore(func(core map[string]any) {
				appendTo(node(t, core, "pkg:internal/alpha"), "diagrams", map[string]any{
					"id": "alpha-er", "title": "Tables", "kind": "er",
					"summary":  statement("Alpha's tables.", "code:internal/alpha"),
					"database": "fixture", "tables": []any{"widgets"},
				})
			}),
			want: []string{at + "alpha-er", "no database fixture", "table:fixture.widgets"},
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
