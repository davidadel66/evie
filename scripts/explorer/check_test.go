package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// byID returns the object with id in a list of objects, such as the map's
// parts or a part's diagrams.
func byID(t *testing.T, list any, id string) map[string]any {
	t.Helper()
	items, _ := list.([]any)
	for _, item := range items {
		if object := item.(map[string]any); object["id"] == id {
			return object
		}
	}
	t.Fatalf("no %s in %d objects", id, len(items))
	return nil
}

// breakPart edits one part of the map and rebuilds its page, so only the
// edit is wrong.
func breakPart(id string, edit func(t *testing.T, part map[string]any)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		m := visionFixtureMap()
		edit(t, byID(t, m["parts"], id))
		writeVision(t, root, m)
	}
}

// breakDiagram edits one of alpha's or beta's diagrams.
func breakDiagram(partID, id string, edit func(diagram map[string]any)) func(*testing.T, string) {
	return breakPart(partID, func(t *testing.T, part map[string]any) { edit(byID(t, part["diagrams"], id)) })
}

// breakSequence edits the map's run sequence.
func breakSequence(edit func(sequence map[string]any)) func(*testing.T, string) {
	return breakMap(func(m, _ map[string]any) { edit(m["sequences"].([]any)[0].(map[string]any)) })
}

// nth returns item i of a list of objects.
func nth(list any, i int) map[string]any {
	return list.([]any)[i].(map[string]any)
}

func TestCheckReportsPackagesWithoutAPart(t *testing.T) {
	runCheckCases(t, []checkCase{
		{
			name: "package with no part",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "internal/zeta/zeta.go", "package zeta\n")
				extractFixture(t, root)
			},
			want: []string{visionMapPath + ": package internal/zeta", `no part has "package": "internal/zeta"`, "a group and a cited summary", "vision.py build", "1 problem(s)"},
		},
		{
			name: "part for a package that no longer exists",
			break_: func(t *testing.T, root string) {
				if err := os.RemoveAll(filepath.Join(root, "internal", "delta")); err != nil {
					t.Fatal(err)
				}
				extractFixture(t, root)
			},
			want: []string{visionMapPath + ": part pkg:internal/delta", "the facts have no package internal/delta", "remove the part"},
		},
	})
}

// citeFromAlpha adds a gotcha on alpha that cites ref.
func citeFromAlpha(ref string) func(*testing.T, string) {
	return breakPart("pkg:internal/alpha", func(_ *testing.T, alpha map[string]any) {
		appendTo(alpha, "gotchas", statement("Cites something.", ref))
	})
}

func TestCheckReportsPartReferencesThatNoLongerResolve(t *testing.T) {
	cases := []checkCase{}
	for _, tc := range []struct{ name, ref, why string }{
		{"file", "code:internal/alpha/missing.go", "does not exist"},
		{"directory escape", "code:../outside.go", "inside the repository"},
		{"Go symbol", "code:internal/alpha/alpha.go#Missing", "declares no Missing"},
		{"Go method", "code:internal/alpha/alpha.go#Server.Stop", "declares no Server.Stop"},
		{"symbol in a file check cannot read", "code:docs/guide.md#Guide", "a Go, JavaScript or TypeScript file, not a .md file"},
		{"symbol in a folder", "code:internal/alpha#Name", "is a folder, so it declares nothing"},
		{"heading", "doc:docs/guide.md#no-such-heading", "no heading"},
		{"heading in a code fence", "doc:docs/guide.md#not-a-heading", "no heading"},
		{"heading in a fence closed only by a longer fence", "doc:docs/guide.md#inside-a-longer-fence", "no heading"},
		{"heading in a tilde fence closed only by a longer fence", "doc:docs/guide.md#inside-a-four-tilde-fence", "no heading"},
		{"heading in a tilde fence holding backticks", "doc:docs/guide.md#inside-a-tilde-fence", "no heading"},
		{"list item above a thematic break", "doc:docs/guide.md#a-list-item", "no heading"},
		{"table row above a thematic break", "doc:docs/guide.md#-a--b-", "no heading"},
		{"front matter", "doc:docs/adr/0001-first-decision.md#status-accepted", "no heading"},
		{"heading in a folder", "doc:docs#guide", "not a file"},
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
		{"part", "part:ghost", "there is no part ghost"},
		{"code key", "Server.Stop", "neither a code key of the map nor a kind:target reference"},
	} {
		cases = append(cases, checkCase{
			name:   tc.name,
			break_: citeFromAlpha(tc.ref),
			want:   []string{visionMapPath + ": part pkg:internal/alpha gotchas 2", "dead reference " + jsonQuote(tc.ref), tc.why, "vision.py build"},
		})
	}
	edit := func(id string, edit func(part map[string]any)) func(*testing.T, string) {
		return breakPart(id, func(_ *testing.T, part map[string]any) { edit(part) })
	}
	cases = append(cases,
		checkCase{
			name:   "unknown kind",
			break_: citeFromAlpha("url:https://example.com"),
			want:   []string{"part pkg:internal/alpha gotchas 2", "url: references", "only the vision skill's check reads"},
		},
		checkCase{
			name:   "summary",
			break_: edit("pkg:internal/beta", func(beta map[string]any) { beta["summary"] = statement("Beta.", "code:internal/beta/beta.go#Title") }),
			want:   []string{"part pkg:internal/beta summary", "declares no Title"},
		},
		checkCase{
			name: "what it does",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) {
				appendTo(alpha, "does", statement("Stops.", "code:internal/alpha/alpha.go#Server.Stop"))
			}),
			want: []string{"part pkg:internal/alpha does 2", "declares no Server.Stop"},
		},
		checkCase{
			name:   "data it owns",
			break_: edit("pkg:internal/beta", func(beta map[string]any) { appendTo(beta, "owns", statement("Gadgets.", "table:fixture.gadgets")) }),
			want:   []string{"part pkg:internal/beta owns 2", "no table fixture.gadgets"},
		},
		checkCase{
			name:   "interfaces",
			break_: edit("pkg:cmd/tool", func(tool map[string]any) { appendTo(tool, "interfaces", statement("Flies.", "command:tool fly")) }),
			want:   []string{"part pkg:cmd/tool interfaces 2", "no command tool fly"},
		},
		checkCase{
			name:   "part references",
			break_: edit("pkg:internal/gamma", func(gamma map[string]any) { appendTo(gamma, "refs", "code:internal/gamma/gone.go") }),
			want:   []string{"part pkg:internal/gamma refs", "internal/gamma/gone.go does not exist"},
		},
		checkCase{
			name:   "glossary term",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) { appendTo(alpha, "terms", "Gizmo") }),
			want:   []string{"part pkg:internal/alpha terms", `dead reference "term:Gizmo"`, "use a term exactly as GLOSSARY.md spells it"},
		},
		checkCase{
			name:   "link",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) { appendTo(alpha, "links", "adr:0042") }),
			want:   []string{"part pkg:internal/alpha links", `dead reference "adr:0042"`},
		},
		checkCase{
			name:   "contained part",
			break_: edit("tool-cli", func(cli map[string]any) { appendTo(cli, "contains", "pkg:internal/ghost") }),
			want:   []string{"part tool-cli contains", `"pkg:internal/ghost" is not a part`, "vision.py build"},
		},
		checkCase{
			name:   "path",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) { appendTo(alpha, "paths", "docs/gone.md") }),
			want:   []string{"part pkg:internal/alpha paths", `path "docs/gone.md": docs/gone.md does not exist`},
		},
		checkCase{
			name: "fact",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) {
				appendTo(alpha, "facts", []any{"route", "GET /gadgets", "route:GET /gadgets"})
			}),
			want: []string{`part pkg:internal/alpha fact "route"`, `dead reference "route:GET /gadgets"`},
		},
	)
	runCheckCases(t, cases)
}

func TestCheckHoldsTheMapsArchitectureToItsContentRules(t *testing.T) {
	// As the skill's check has them: every statement of a part and every
	// edge but an import edge cites a source, and ids are unique among parts,
	// among sequences, and across every part's diagrams.
	uncited := func(text string) map[string]any { return map[string]any{"text": text} }
	cites := []string{"cites no source", "cite at least one reference", "vision.py build", "1 problem(s)"}
	edit := func(id string, edit func(part map[string]any)) func(*testing.T, string) {
		return breakPart(id, func(_ *testing.T, part map[string]any) { edit(part) })
	}
	runCheckCases(t, []checkCase{
		{
			name:   "a part's summary without a source",
			break_: edit("pkg:internal/beta", func(beta map[string]any) { beta["summary"] = uncited("Beta names the fixture.") }),
			want:   append([]string{visionMapPath + ": part pkg:internal/beta summary"}, cites...),
		},
		{
			name:   "what a part does, with an empty list of sources",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) { appendTo(alpha, "does", statement("Stops a server.")) }),
			want:   append([]string{"part pkg:internal/alpha does 2"}, cites...),
		},
		{
			name:   "data a part owns, without a source",
			break_: edit("pkg:internal/beta", func(beta map[string]any) { appendTo(beta, "owns", uncited("Gadgets.")) }),
			want:   append([]string{"part pkg:internal/beta owns 2"}, cites...),
		},
		{
			name:   "an interface without a source",
			break_: edit("pkg:cmd/tool", func(tool map[string]any) { appendTo(tool, "interfaces", uncited("Flies.")) }),
			want:   append([]string{"part pkg:cmd/tool interfaces 2"}, cites...),
		},
		{
			name:   "a gotcha without a source",
			break_: edit("pkg:internal/alpha", func(alpha map[string]any) { appendTo(alpha, "gotchas", uncited("Watch out.")) }),
			want:   append([]string{"part pkg:internal/alpha gotchas 2"}, cites...),
		},
		{
			name: "an edge other than an import edge without a source",
			break_: breakMap(func(m, _ map[string]any) {
				appendTo(m, "edges", map[string]any{"from": "pkg:internal/alpha", "to": "pkg:internal/delta", "label": "reads", "via": "call"})
			}),
			want: []string{visionMapPath + ": edge pkg:internal/alpha -> pkg:internal/delta (call)", "the edge cites no source", "cite the code", "1 problem(s)"},
		},
		{
			name: "a part id used twice",
			break_: breakMap(func(m, _ map[string]any) {
				appendTo(m, "parts", map[string]any{"id": "owner", "level": "context", "kind": "person", "title": "Owner again",
					"summary": statement("The owner again.", "doc:docs/guide.md")})
			}),
			want: []string{visionMapPath + ": part owner: the id is used by an earlier part", "give each part its own id", "1 problem(s)"},
		},
		{
			name: "a sequence id used twice",
			break_: breakMap(func(m, _ map[string]any) {
				again := map[string]any{}
				for k, v := range m["sequences"].([]any)[0].(map[string]any) {
					again[k] = v
				}
				again["title"] = "Run the tool again"
				appendTo(m, "sequences", again)
			}),
			want: []string{visionMapPath + ": sequence run: the id is used by an earlier sequence", "give each sequence its own id", "1 problem(s)"},
		},
		{
			name: "a diagram id another part's diagram uses",
			break_: edit("pkg:internal/beta", func(beta map[string]any) {
				appendTo(beta, "diagrams", map[string]any{"id": "alpha-graph", "title": "Beta alone", "kind": "graph",
					"summary": statement("Beta.", "code:internal/beta/beta.go#Name"), "nodes": []any{map[string]any{"id": "b", "label": "beta"}}})
			}),
			want: []string{"part pkg:internal/beta diagram alpha-graph: the id is used by an earlier diagram", "1 problem(s)"},
		},
	})
}

func TestCheckHoldsPartFactValuesToWhatTheyCite(t *testing.T) {
	runCheckCases(t, []checkCase{
		{
			name: "a Go constant's value changes",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "internal/alpha/alpha.go", strings.Replace(readFile(t, root, "internal/alpha/alpha.go"), "const Limit = 3", "const Limit = 4", 1))
			},
			want: []string{`part pkg:internal/alpha fact "limit"`, `says "3", which "code:internal/alpha/alpha.go#Limit" does not`},
		},
		{
			name: "a part that is not the one the value names",
			break_: breakPart("pkg:internal/alpha", func(_ *testing.T, alpha map[string]any) {
				alpha["facts"].([]any)[1] = []any{"owner", "Owner", "part:fixture"}
			}),
			want: []string{`part pkg:internal/alpha fact "owner"`, `says "Owner", which "part:fixture" does not`},
		},
	})
}

func TestCheckReportsEdgeAndSequenceProblems(t *testing.T) {
	edge := func(e map[string]any) func(*testing.T, string) {
		return breakMap(func(m, _ map[string]any) { appendTo(m, "edges", e) })
	}
	runCheckCases(t, []checkCase{
		{
			name:   "import edge the facts lack",
			break_: edge(map[string]any{"from": "pkg:internal/alpha", "to": "pkg:internal/delta", "label": "reads", "via": "import"}),
			want:   []string{visionMapPath + ": edge pkg:internal/alpha -> pkg:internal/delta (import)", "internal/alpha does not import internal/delta", "remove it, reverse it"},
		},
		{
			name:   "edge to a part that is not there",
			break_: edge(map[string]any{"from": "pkg:internal/alpha", "to": "pkg:internal/ghost", "label": "calls", "via": "call", "refs": []any{"code:internal/alpha/alpha.go"}}),
			want:   []string{"edge pkg:internal/alpha -> pkg:internal/ghost (call)", `"pkg:internal/ghost" is not a part`},
		},
		{
			name: "edge reference",
			break_: breakMap(func(m, _ map[string]any) {
				nth(m["edges"], 1)["refs"] = []any{"code:internal/alpha/alpha_linux.go"}
			}),
			want: []string{"edge pkg:internal/alpha -> pkg:internal/gamma (call)", "internal/alpha/alpha_linux.go does not exist"},
		},
		{
			name: "sequence step via an import the facts lack",
			break_: breakSequence(func(run map[string]any) {
				appendTo(run, "steps", map[string]any{"from": "pkg:internal/alpha", "to": "pkg:cmd/tool", "label": "calls back", "kind": "call", "via": "import", "refs": []any{"Name"}})
			}),
			want: []string{"sequence run step 4", "internal/alpha does not import cmd/tool"},
		},
		{
			name:   "sequence step that cites no code",
			break_: breakSequence(func(run map[string]any) { nth(run["steps"], 2)["refs"] = []any{"doc:docs/guide.md"} }),
			want:   []string{"sequence run step 3", "the step cites no code", "a code: reference or a code key"},
		},
		{
			name:   "sequence step reference",
			break_: breakSequence(func(run map[string]any) { appendTo(nth(run["steps"], 0), "refs", "code:internal/alpha/alpha.go#Gone") }),
			want:   []string{"sequence run step 1", "declares no Gone"},
		},
		{
			name:   "sequence participant that is not a part",
			break_: breakSequence(func(run map[string]any) { appendTo(run, "participants", "pkg:internal/ghost") }),
			want:   []string{"sequence run participants", `"pkg:internal/ghost" is not a part`},
		},
		{
			name:   "sequence step to a part that is not a participant",
			break_: breakSequence(func(run map[string]any) { nth(run["steps"], 2)["to"] = "pkg:internal/beta" }),
			want:   []string{visionMapPath + ": sequence run step 3", `to "pkg:internal/beta" is not a participant`, "add it to participants", "vision.py build", "1 problem(s)"},
		},
		{
			name:   "sequence step from something that is not a part",
			break_: breakSequence(func(run map[string]any) { nth(run["steps"], 2)["from"] = "pkg:internal/ghost" }),
			want:   []string{"sequence run step 3", `from "pkg:internal/ghost" is neither a participant nor a part`, "1 problem(s)"},
		},
		{
			name:   "sequence step without an end",
			break_: breakSequence(func(run map[string]any) { delete(nth(run["steps"], 2), "from") }),
			want:   []string{"sequence run step 3", "the step has no from", "name the participant", "1 problem(s)"},
		},
		{
			name: "sequence step via import from a part with no package",
			break_: breakSequence(func(run map[string]any) {
				appendTo(run, "participants", "tool-cli")
				appendTo(run, "steps", map[string]any{"from": "tool-cli", "to": "pkg:internal/alpha", "label": "Name()", "kind": "call", "via": "import", "refs": []any{"Name"}})
			}),
			want: []string{"sequence run step 4", `part "tool-cli" has no package, but via import joins two package parts`, "use a via other than import", "1 problem(s)"},
		},
		{
			name:   "import edge from a part with no package",
			break_: edge(map[string]any{"from": "owner", "to": "pkg:internal/alpha", "label": "runs", "via": "import"}),
			want:   []string{"edge owner -> pkg:internal/alpha (import)", `part "owner" has no package, but via import joins two package parts`, "1 problem(s)"},
		},
		{
			name:   "sequence summary",
			break_: breakSequence(func(run map[string]any) { run["summary"] = statement("Runs.", "code:cmd/tool/main.go#start") }),
			want:   []string{"sequence run summary", "declares no start"},
		},
		{
			name:   "sequence trigger",
			break_: breakSequence(func(run map[string]any) { run["trigger"] = statement("Nobody runs it.", "part:nobody") }),
			want:   []string{"sequence run trigger", `dead reference "part:nobody"`, "there is no part nobody", "use the id of a part of the map"},
		},
	})
}

func TestCheckReportsDiagramProblems(t *testing.T) {
	alpha := func(id string, edit func(diagram map[string]any)) func(*testing.T, string) {
		return breakDiagram("pkg:internal/alpha", id, edit)
	}
	runCheckCases(t, []checkCase{
		{
			name:   "graph node naming a part that is not there",
			break_: alpha("alpha-graph", func(g map[string]any) { nth(g["nodes"], 1)["node"] = "pkg:internal/ghost" }),
			want:   []string{"part pkg:internal/alpha diagram alpha-graph node b", `"pkg:internal/ghost" is not a part`},
		},
		{
			name:   "graph edge reference",
			break_: alpha("alpha-graph", func(g map[string]any) { nth(g["edges"], 0)["refs"] = []any{"code:internal/beta/beta.go#Title"} }),
			want:   []string{"part pkg:internal/alpha diagram alpha-graph edge 1", "declares no Title"},
		},
		{
			name:   "diagram summary",
			break_: alpha("alpha-graph", func(g map[string]any) { g["summary"] = statement("Alpha calls beta.", "doc:docs/gone.md") }),
			want:   []string{"part pkg:internal/alpha diagram alpha-graph summary", "docs/gone.md does not exist"},
		},
		{
			name:   "sequence diagram step that cites no code",
			break_: alpha("alpha-sequence", func(g map[string]any) { nth(g["steps"], 0)["refs"] = []any{"adr:0001"} }),
			want:   []string{"part pkg:internal/alpha diagram alpha-sequence step 1", "the step cites no code"},
		},
		{
			name: "sequence diagram step via an import the facts lack",
			break_: alpha("alpha-sequence", func(g map[string]any) {
				step := nth(g["steps"], 0)
				step["from"], step["to"] = "pkg:internal/alpha", "pkg:cmd/tool"
			}),
			want: []string{"part pkg:internal/alpha diagram alpha-sequence step 1", "internal/alpha does not import cmd/tool"},
		},
		{
			name: "sequence diagram step to a part that is not a participant",
			break_: alpha("alpha-sequence", func(g map[string]any) {
				step := nth(g["steps"], 0)
				step["to"] = "pkg:internal/delta"
				delete(step, "via")
			}),
			want: []string{"part pkg:internal/alpha diagram alpha-sequence step 1", `to "pkg:internal/delta" is not a participant`, "1 problem(s)"},
		},
		{
			name:   "sequence diagram participant that is not a part",
			break_: alpha("alpha-sequence", func(g map[string]any) { appendTo(g, "participants", "pkg:internal/ghost") }),
			want:   []string{"part pkg:internal/alpha diagram alpha-sequence participants", `"pkg:internal/ghost" is not a part`},
		},
		{
			name:   "sequence diagram trigger",
			break_: alpha("alpha-sequence", func(g map[string]any) { g["trigger"] = statement("Nobody.", "part:nobody") }),
			want:   []string{"part pkg:internal/alpha diagram alpha-sequence trigger", "there is no part nobody"},
		},
		{
			name:   "state reference",
			break_: alpha("alpha-states", func(g map[string]any) { nth(g["states"], 0)["refs"] = []any{"code:internal/alpha/alpha.go#Idle"} }),
			want:   []string{"part pkg:internal/alpha diagram alpha-states state idle", "declares no Idle"},
		},
		{
			name: "transition reference",
			break_: alpha("alpha-states", func(g map[string]any) {
				nth(g["transitions"], 0)["refs"] = []any{"code:internal/alpha/alpha.go#Server.Run"}
			}),
			want: []string{"part pkg:internal/alpha diagram alpha-states transition 1", "declares no Server.Run"},
		},
		{
			name:   "er diagram over a database the facts lack",
			break_: breakDiagram("pkg:internal/beta", "beta-tables", func(g map[string]any) { g["database"] = "ghost" }),
			want:   []string{"part pkg:internal/beta diagram beta-tables", "there is no database ghost in the facts", "use database.table from the facts databases"},
		},
		{
			name:   "er diagram over a table the facts lack",
			break_: breakDiagram("pkg:internal/beta", "beta-tables", func(g map[string]any) { appendTo(g, "tables", "gadgets") }),
			want:   []string{"part pkg:internal/beta diagram beta-tables", `dead reference "table:fixture.gadgets"`},
		},
	})
}

func TestCheckReportsEveryProblemNotJustTheFirst(t *testing.T) {
	root := newFixture(t)
	extractFixture(t, root)
	m := visionFixtureMap()
	byID(t, m["parts"], "pkg:internal/delta")["summary"] = statement("Delta.", "code:internal/delta/delta.go#Gone")
	appendTo(byID(t, m["parts"], "owner"), "links", "adr:0098")
	writeVision(t, root, m)
	writeFile(t, root, "internal/zeta/zeta.go", "package zeta\n")

	code, out := checkFixture(t, root)
	if code != 1 {
		t.Fatalf("check exit %d, want 1:\n%s", code, out)
	}
	for _, want := range []string{
		factsPath + ": facts:",
		visionMapPath + ": part pkg:internal/delta summary:",
		visionMapPath + ": part owner links:",
		visionMapPath + ": package internal/zeta:",
		"4 problem(s)",
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
	// Code entries are a JSON object, read in no order; every other list
	// keeps the map's. The problems come out in one order every time.
	root := newFixture(t)
	extractFixture(t, root)
	m := visionFixtureMap()
	code := m["code"].(map[string]any)
	for _, key := range []string{"Zeta", "Alpha", "Mu", "Beta", "Omega", "Delta"} {
		code[key] = map[string]any{"file": "internal/alpha/alpha.go", "symbol": key}
	}
	appendTo(byID(t, m["parts"], "pkg:internal/gamma"), "refs", "pkg:internal/ghost", "adr:0099")
	writeVision(t, root, m)

	_, first := checkFixture(t, root)
	if alpha, zeta := strings.Index(first, "code.Alpha:"), strings.Index(first, "code.Zeta:"); alpha < 0 || zeta < 0 || alpha > zeta {
		t.Fatalf("want code entries reported in key order:\n%s", first)
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
	// A checkout that turned LF into CRLF. The facts are named with the
	// line endings as the problem rather than as a facts mismatch.
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(factsPath)))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, factsPath, strings.ReplaceAll(string(data), "\n", "\r\n"))

	code, out := checkFixture(t, root)
	if code != 1 {
		t.Fatalf("check exit %d, want 1:\n%s", code, out)
	}
	if !strings.Contains(out, factsPath+": file: the file has CRLF line endings") {
		t.Errorf("output does not name the facts' line endings:\n%s", out)
	}
	if strings.Contains(out, "differ from a fresh extraction") {
		t.Errorf("output reports a facts mismatch for a line-ending problem:\n%s", out)
	}
	if !strings.Contains(out, "1 problem(s)") {
		t.Errorf("want only the line-ending problem:\n%s", out)
	}
}
