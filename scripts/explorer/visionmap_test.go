package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every fixture has a consistent system map, testdata/repo/.vision/map.json,
// whose every reference resolves in the fixture repository, and the page
// built from it (newFixture). These tests break one thing.

const fixtureScript = "web/stream.ts"

// visionFixtureMap is the fixture's map, decoded.
func visionFixtureMap() map[string]any {
	data, err := os.ReadFile(filepath.Join("testdata", "repo", filepath.FromSlash(visionMapPath)))
	if err != nil {
		panic(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		panic(err)
	}
	return m
}

// writeVision writes m as the fixture's map, and the page built from it.
func writeVision(t *testing.T, root string, m map[string]any) {
	t.Helper()
	writeVisionMap(t, root, m)
	buildVisionPage(t, root)
}

func writeVisionMap(t *testing.T, root string, m map[string]any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, visionMapPath, string(data)+"\n")
	return data
}

// buildVisionPage writes the page the vision skill's build would write from
// the fixture's map and the inputs it names: the map's hash and the inputs
// hash in its head.
func buildVisionPage(t *testing.T, root string) {
	t.Helper()
	writeVisionPage(t, root, fixtureMapHash(t, root), fixtureInputsHash(t, root))
}

func fixtureMapHash(t *testing.T, root string) string {
	t.Helper()
	hash, err := mapHash([]byte(readFile(t, root, visionMapPath)))
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

// fixtureInputsHash hashes the facts file and changelog the fixture's map
// names, and its design proposals.
func fixtureInputsHash(t *testing.T, root string) string {
	t.Helper()
	var m struct {
		Meta struct{ Facts, Changelog string } `json:"meta"`
	}
	if err := json.Unmarshal([]byte(readFile(t, root, visionMapPath)), &m); err != nil {
		t.Fatal(err)
	}
	proposals, err := readProposals(root)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := inputsHash(root, m.Meta.Facts, m.Meta.Changelog, proposals)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

// writeVisionPage writes a page carrying the given hashes; an empty inputs
// hash leaves its meta out, as on a page built before the skill wrote one.
func writeVisionPage(t *testing.T, root, mapDigest, inputsDigest string) {
	t.Helper()
	head := "<!doctype html>\n<meta charset=\"utf-8\">\n<meta name=\"vision-map-hash\" content=\"" + mapDigest + "\">\n"
	if inputsDigest != "" {
		head += "<meta name=\"vision-inputs-hash\" content=\"" + inputsDigest + "\">\n"
	}
	writeFile(t, root, visionPagePath, head+"<title>Fixture</title>\n")
}

// newVisionFixture is the consistent fixture with its facts extracted.
func newVisionFixture(t *testing.T) string {
	t.Helper()
	root := newFixture(t)
	extractFixture(t, root)
	return root
}

func TestCheckPassesWithAConsistentSystemMap(t *testing.T) {
	root := newVisionFixture(t)
	code, out := checkFixture(t, root)
	if code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

// visionStepOf returns the first top-level step of the map's first flow.
func visionStepOf(m map[string]any) map[string]any {
	return m["flows"].([]any)[0].(map[string]any)["steps"].([]any)[0].(map[string]any)
}

// breakMap edits the map and rebuilds its page, so only the edit is wrong.
func breakMap(edit func(m, step map[string]any)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		m := visionFixtureMap()
		edit(m, visionStepOf(m))
		writeVision(t, root, m)
	}
}

// runVisionCases runs check cases on the fixture with a consistent map.
func runVisionCases(t *testing.T, cases []checkCase) {
	t.Helper()
	runCheckCasesOn(t, newVisionFixture, cases)
}

// codeEntry replaces one entry of the map's code.
func codeEntry(key string, e map[string]any) func(*testing.T, string) {
	return breakMap(func(m, _ map[string]any) { m["code"].(map[string]any)[key] = e })
}

func TestCheckReportsSystemMapCodeEntriesThatNoLongerResolve(t *testing.T) {
	runVisionCases(t, []checkCase{
		{
			name:   "Go method renamed",
			break_: codeEntry("Server.Start", map[string]any{"file": "internal/alpha/alpha.go", "symbol": "Server.Begin"}),
			want:   []string{visionMapPath, "code.Server.Start", "declares no Server.Begin", "Type.Method", "vision.py build"},
		},
		{
			name:   "Go method named without its type",
			break_: codeEntry("Server.Start", map[string]any{"file": "internal/alpha/alpha.go", "symbol": "Start"}),
			want:   []string{"code.Server.Start", "declares no Start"},
		},
		{
			name:   "file removed",
			break_: codeEntry("Name", map[string]any{"file": "internal/alpha/gone.go", "symbol": "Name"}),
			want:   []string{"code.Name", "internal/alpha/gone.go does not exist", "point file at an existing file"},
		},
		{
			name:   "file outside the repository",
			break_: codeEntry("Name", map[string]any{"file": "../alpha.go", "symbol": "Name"}),
			want:   []string{"code.Name", "inside the repository"},
		},
		{
			name:   "TypeScript function renamed",
			break_: codeEntry("sendDetached", map[string]any{"file": fixtureScript, "symbol": "sendLater"}),
			want:   []string{"code.sendDetached", fixtureScript + " declares no sendLater"},
		},
		{
			name:   "TypeScript method renamed",
			break_: codeEntry("Feed.follow", map[string]any{"file": fixtureScript, "symbol": "Feed.unfollow"}),
			want:   []string{"code.Feed.follow", "declares no Feed.unfollow"},
		},
		{
			name:   "symbol in a language check cannot read",
			break_: codeEntry("Name", map[string]any{"file": "docs/guide.md", "symbol": "Guide"}),
			want:   []string{"code.Name", "a Go, JavaScript or TypeScript file, not a .md file"},
		},
		{
			name:   "symbol without a file",
			break_: codeEntry("Name", map[string]any{"symbol": "Name"}),
			want:   []string{"code.Name", "no file"},
		},
		{
			name:   "entry with neither file nor symbol",
			break_: codeEntry("Name", map[string]any{"title": "Name"}),
			want:   []string{"code.Name", "names no file and symbol"},
		},
	})
}

func TestCheckRefusesSystemMapFormsOnlyTheSkillReads(t *testing.T) {
	// Evie's map uses none of these, so the explorer check does not read
	// them; it names the skill's check, which does, rather than passing them.
	refused := func(item string, more ...string) []string {
		return append([]string{visionMapPath, item, "only the vision skill's check reads", "vision.py check .vision/map.json"}, more...)
	}
	runVisionCases(t, []checkCase{
		{
			name:   "line range",
			break_: codeEntry("Name", map[string]any{"file": "internal/alpha/alpha.go", "symbol": "Name", "lines": "1-2"}),
			want:   refused("code.Name", "a line range"),
		},
		{
			name: "notebook cell",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "notes/train.ipynb", `{"cells": [{"cell_type": "code", "source": "x = 1"}]}`)
				codeEntry("cell", map[string]any{"file": "notes/train.ipynb", "cell": 0})(t, root)
			},
			want: refused("code.cell", "a notebook cell"),
		},
		{
			name:   "inline source",
			break_: codeEntry("sketch", map[string]any{"src": "start, then serve", "label": "a sketch"}),
			want:   refused("code.sketch", "inline src"),
		},
		{
			name: "a single top-level flow",
			break_: breakMap(func(m, _ map[string]any) {
				m["flow"] = m["flows"].([]any)[0].(map[string]any)["steps"]
				delete(m, "flows")
			}),
			want: refused("flow", "a top-level flow"),
		},
		{
			name: "library definitions",
			break_: breakMap(func(m, _ map[string]any) {
				m["refs"] = map[string]any{"http.HandleFunc": map[string]any{"lib": "net/http", "sig": "http.HandleFunc(pattern, handler)"}}
			}),
			want: refused("refs", "library definitions"),
		},
		{
			name: "value panel reference",
			break_: breakMap(func(_, step map[string]any) {
				appendTo(step, "values", map[string]any{"type": "big", "title": "Fields", "value": "1", "ref": "code:internal/alpha/alpha.go#Server.Addr"})
			}),
			want: refused(`step start big panel "Fields"`, "a value panel's ref"),
		},
		{
			name: "kv row reference",
			break_: breakMap(func(_, step map[string]any) {
				panel := step["values"].([]any)[0].(map[string]any)
				panel["rows"].([]any)[0] = []any{"limit", "3", "code:internal/alpha/alpha.go#Limit"}
			}),
			want: refused(`step start kv panel "Limits" row "limit"`, "a kv row's reference"),
		},
		{
			name:   "reference kind",
			break_: breakMap(func(_, step map[string]any) { appendTo(step, "refs", "sec:3.1") }),
			want:   refused("step start refs", "sec: references"),
		},
		{
			name:   "explorer node",
			break_: breakMap(func(_, step map[string]any) { appendTo(step, "refs", "node:pkg:internal/alpha") }),
			want:   refused("step start refs", "node: references"),
		},
	})
}

func TestCheckFindsTypeScriptMembersOnlyInsideTheirOwnersBody(t *testing.T) {
	// Feed.follow is gone from Feed, but the file still names follow after
	// Feed: on a later line, and inside one of Feed's own method bodies.
	gone := func(src string) func(*testing.T, string) {
		return func(t *testing.T, root string) { writeFile(t, root, fixtureScript, src) }
	}
	rest := "export function followChat() {}\nexport const sendDetached = () => {};\n"
	runVisionCases(t, []checkCase{
		{
			name:   "member deleted, named later in the file",
			break_: gone("export class Feed {\n  unfollow() {}\n}\n" + rest + "export const feed = new Feed().follow;\n"),
			want:   []string{"code.Feed.follow", fixtureScript + " declares no Feed.follow"},
		},
		{
			name:   "member deleted, called inside another member",
			break_: gone("export class Feed {\n  unfollow() {\n    follow();\n  }\n}\n" + rest),
			want:   []string{"code.Feed.follow", fixtureScript + " declares no Feed.follow"},
		},
	})
}

func TestCheckAcceptsSystemMapFolderReferences(t *testing.T) {
	// As the skill reads them: code: and doc: name a file or a folder.
	root := newVisionFixture(t)
	m := visionFixtureMap()
	appendTo(visionStepOf(m), "refs", "doc:docs/adr", "code:internal/alpha")
	writeVision(t, root, m)
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

// chipAt replaces the start step's fact chip at i.
func chipAt(i int, chip ...any) func(*testing.T, string) {
	return breakMap(func(_, step map[string]any) { step["facts"].([]any)[i] = chip })
}

func TestCheckHoldsSystemMapFactValuesToWhatTheyCite(t *testing.T) {
	// As the skill's check does ("backed values" in references/map-format.md):
	// once a chip's reference resolves, its value must appear in what the
	// reference points at.
	runVisionCases(t, []checkCase{
		{
			name: "a route's guard changes",
			break_: func(t *testing.T, root string) {
				routes := readFile(t, root, "internal/alpha/routes.go")
				routes = strings.Replace(routes, `mux.HandleFunc("GET /widgets", s.requireOwner(s.listWidgets))`, `mux.HandleFunc("GET /widgets", s.requireAdmin(s.listWidgets))`, 1)
				writeFile(t, root, "internal/alpha/routes.go", routes+"\nfunc (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc { return next }\n")
				extractFixture(t, root)
			},
			want: []string{visionMapPath, `step start fact "guarded by"`, `says "requireOwner", which "route:GET /widgets" does not`, "vision.py build"},
		},
		{
			name: "a Go constant's value changes",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "internal/alpha/alpha.go", strings.Replace(readFile(t, root, "internal/alpha/alpha.go"), "const Limit = 3", "const Limit = 4", 1))
			},
			want: []string{`step start fact "limit"`, `says "3", which "code:internal/alpha/alpha.go#Limit" does not`},
		},
		{
			name: "a value found only inside a longer number",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, "internal/alpha/alpha.go", strings.Replace(readFile(t, root, "internal/alpha/alpha.go"), "const Limit = 3", "const Limit = 30", 1))
			},
			want: []string{`step start fact "limit"`, `says "3"`},
		},
		{
			name:   "a TypeScript declaration that does not say the value",
			break_: chipAt(7, "follows", "unfollowChat", "code:web/stream.ts#followChat"),
			want:   []string{`step start fact "follows"`, `says "unfollowChat", which "code:web/stream.ts#followChat" does not`},
		},
		{
			name:   "a code key whose declaration does not say the value",
			break_: chipAt(8, "names", "alpha.Name", "Name"),
			want:   []string{`step start fact "names"`, `says "alpha.Name", which "Name" does not`},
		},
		{
			name:   "a fact that does not say the value",
			break_: chipAt(3, "package", "internal/beta", "pkg:internal/alpha"),
			want:   []string{`step start fact "package"`, `says "internal/beta", which "pkg:internal/alpha" does not`},
		},
		{
			name:   "a value in a later piece's place",
			break_: chipAt(0, "route", "widgets … GET", "route:GET /widgets"),
			want:   []string{`step start fact "route"`, `says "widgets … GET"`},
		},
		{
			name:   "a folder, which has no text",
			break_: chipAt(3, "package", "alpha", "code:internal/alpha"),
			want:   []string{`step start fact "package"`, `"code:internal/alpha" names a folder, which has no text to find "alpha" in`},
		},
		{
			name:   "a document",
			break_: chipAt(3, "guide", "Text", "doc:docs/guide.md"),
			want:   []string{visionMapPath, `step start fact "guide"`, "a value backed by a doc: reference", "only the vision skill's check reads"},
		},
		{
			name:   "a value that is not text",
			break_: chipAt(6, "limit", 3, "code:internal/alpha/alpha.go#Limit"),
			want:   []string{`step start fact "limit"`, "a value that is not a string", "only the vision skill's check reads"},
		},
	})
}

func TestCheckMatchesSystemMapFactValuesAsTheSkillDoes(t *testing.T) {
	// Case, runs of whitespace and digit-group commas are ignored, and an
	// ellipsis elides between pieces that appear in order.
	root := newVisionFixture(t)
	writeFile(t, root, fixtureScript, readFile(t, root, fixtureScript)+"export const pageSize = 1000;\n")
	m := visionFixtureMap()
	appendTo(visionStepOf(m), "facts",
		[]any{"guarded by", "REQUIREOWNER", "route:GET /widgets"},
		[]any{"limit", "const  Limit\n= 3", "code:internal/alpha/alpha.go#Limit"},
		[]any{"page size", "1,000", "code:web/stream.ts#pageSize"},
		[]any{"route", "GET … requireOwner", "route:GET /widgets"},
		[]any{"server", "Server", "code:internal/alpha/alpha.go"},
		[]any{"routes", "Server.routes", "route:/health"},
		[]any{"columns", "name", "table:fixture.widgets"},
		[]any{"command", "cmd/tool", "command:tool"},
		[]any{"part", "PKG:internal/alpha", "part:pkg:internal/alpha"},
	)
	writeVision(t, root, m)
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

func TestCheckReportsSystemMapReferencesThatNoLongerResolve(t *testing.T) {
	cases := []checkCase{}
	for _, tc := range []struct{ name, ref, why string }{
		{"Go symbol", "code:internal/alpha/alpha.go#Stop", "declares no Stop"},
		{"Go method", "code:internal/alpha/routes.go#Server.serve", "declares no Server.serve"},
		{"TypeScript symbol", "code:web/stream.ts#unfollowChat", "declares no unfollowChat"},
		{"file", "code:internal/alpha/gone.go", "does not exist"},
		{"doc", "doc:docs/missing.md", "does not exist"},
		{"doc heading", "doc:docs/guide.md#no-such-heading", "no heading"},
		{"table", "table:fixture.gadgets", "no table fixture.gadgets"},
		{"route", "route:GET /gadgets", "no route GET /gadgets"},
		{"route with the wrong method", "route:POST /widgets", "no route"},
		{"command", "command:tool fly", "no command tool fly"},
		{"package", "pkg:internal/ghost", "no package internal/ghost"},
		{"ADR", "adr:0099", "no ADR 0099"},
		{"term", "term:Gadget", "no glossary term Gadget"},
		{"part", "part:ghost", "there is no part ghost"},
		{"unknown key", "NoSuchKey", "neither a code key of the map nor a kind:target reference"},
	} {
		cases = append(cases, checkCase{
			name:   tc.name,
			break_: breakMap(func(_, step map[string]any) { appendTo(step, "refs", tc.ref) }),
			want:   []string{visionMapPath, "step start refs", "dead reference " + jsonQuote(tc.ref), tc.why, "vision.py build"},
		})
	}
	runVisionCases(t, cases)
}

func TestCheckNamesTheReferenceKindsASystemMapMayUse(t *testing.T) {
	runVisionCases(t, []checkCase{{
		name:   "unknown key",
		break_: breakMap(func(_, step map[string]any) { appendTo(step, "refs", "NoSuchKey") }),
		want:   []string{"use a code key of the map, or one of adr:, code:, command:, doc:, part:, pkg:, route:, table:, term:"},
	}})
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestCheckReportsSystemMapFactChipsAndFlagsThatNoLongerResolve(t *testing.T) {
	runVisionCases(t, []checkCase{
		{
			name: "route chip",
			break_: breakMap(func(_, step map[string]any) {
				step["facts"].([]any)[0] = []any{"route", "GET /gadgets", "route:GET /gadgets"}
			}),
			want: []string{`step start fact "route"`, `dead reference "route:GET /gadgets"`, "use a route from the facts", "vision.py build"},
		},
		{
			name: "table chip",
			break_: breakMap(func(_, step map[string]any) {
				step["facts"].([]any)[4] = []any{"writes", "gadgets", "table:fixture.gadgets"}
			}),
			want: []string{`step start fact "writes"`, `dead reference "table:fixture.gadgets"`, "use database.table from the facts databases"},
		},
		{
			name: "package chip",
			break_: breakMap(func(_, step map[string]any) {
				step["facts"].([]any)[3] = []any{"package", "internal/ghost", "pkg:internal/ghost"}
			}),
			want: []string{`step start fact "package"`, `dead reference "pkg:internal/ghost"`},
		},
		{
			name: "command chip",
			break_: breakMap(func(_, step map[string]any) {
				step["facts"].([]any)[5] = []any{"command", "tool fly", "command:tool fly"}
			}),
			want: []string{`step start fact "command"`, `dead reference "command:tool fly"`},
		},
		{
			name: "flag",
			break_: breakMap(func(_, step map[string]any) {
				appendTo(step["flags"].([]any)[0].(map[string]any), "refs", "code:internal/alpha/alpha.go#Server.Stop")
			}),
			want: []string{`step start flag "A decision and a term back this flag."`, "declares no Server.Stop"},
		},
		{
			name: "step code key",
			break_: breakMap(func(_, step map[string]any) {
				step["code"] = map[string]any{"key": "Server.Stop"}
			}),
			want: []string{"step start code", `code key "Server.Stop" is not in the map's code`},
		},
		{
			name: "file",
			break_: breakMap(func(_, step map[string]any) {
				appendTo(step, "files", map[string]any{"path": "internal/alpha/gone.go"})
			}),
			want: []string{"step start files", `file "internal/alpha/gone.go"`, "does not exist"},
		},
		{
			name: "nested step",
			break_: breakMap(func(_, step map[string]any) {
				child := step["children"].([]any)[0].(map[string]any)
				child["refs"] = []any{"code:web/stream.ts#leaveChat"}
			}),
			want: []string{"step follow refs", "declares no leaveChat"},
		},
	})
}

func TestCheckReportsSystemMapSettingsEvieDoesNotUse(t *testing.T) {
	runVisionCases(t, []checkCase{
		{
			name:   "facts file",
			break_: breakMap(func(m, _ map[string]any) { m["meta"].(map[string]any)["facts"] = "docs/facts.json" }),
			want:   []string{visionMapPath, "meta.facts", `"docs/facts.json"`, factsPath},
		},
		{
			name: "page",
			break_: func(t *testing.T, root string) {
				m := visionFixtureMap()
				m["meta"].(map[string]any)["page"] = "docs/system.html"
				writeVisionMap(t, root, m)
			},
			// The page Evie serves was built before the change, so it is stale too.
			want: []string{visionMapPath, "meta.page", `"docs/system.html"`, "Evie embeds and serves " + visionPagePath, visionPagePath + ": page", "older map"},
		},
		{
			name:   "parts that need not cover every package",
			break_: breakMap(func(m, _ map[string]any) { delete(m["meta"].(map[string]any), "parts") }),
			want:   []string{visionMapPath + ": meta.parts", `meta.parts is ""`, "every package has a part", `set meta.parts to "complete"`},
		},
		{
			name:   "changelog",
			break_: breakMap(func(m, _ map[string]any) { m["meta"].(map[string]any)["changelog"] = "docs/HISTORY.md" }),
			want:   []string{visionMapPath + ": meta.changelog", `"docs/HISTORY.md"`, "check validates " + changelogPath, `set meta.changelog to "CHANGELOG.md"`},
		},
		{
			name: "repository",
			break_: breakMap(func(m, _ map[string]any) {
				m["meta"].(map[string]any)["repository"] = "https://github.com/someone/fork"
			}),
			want: []string{visionMapPath + ": meta.repository", `"https://github.com/someone/fork"`, "the module example.com/fixture", `set meta.repository to "https://example.com/fixture"`},
		},
		{
			name: "malformed map",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionMapPath, `{"flows": [}`)
			},
			want: []string{visionMapPath, "malformed map", "map-format.md"},
		},
	})
}

func TestCheckReportsASystemMapPageNotBuiltFromTheMap(t *testing.T) {
	runVisionCases(t, []checkCase{
		{
			name: "the map changed after the page was built",
			break_: func(t *testing.T, root string) {
				m := visionFixtureMap()
				built, _ := mapHash(writeVisionMap(t, root, m))
				visionStepOf(m)["name"] = "Start the server again"
				now, _ := mapHash(writeVisionMap(t, root, m))
				if built == now {
					t.Fatal("renaming a step left the map's hash unchanged")
				}
				writeVisionPage(t, root, built, fixtureInputsHash(t, root))
			},
			want: []string{visionPagePath + ": page", "built from an older map", "vision.py build", "commit the page with the map"},
		},
		{
			name: "the page is missing",
			break_: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(visionPagePath))); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{visionPagePath + ": page", "does not exist", "rebuild with the vision skill"},
		},
		{
			// The map holds every package's part, and the binary embeds the
			// page built from it, so a repository without it fails once.
			name: "the map is missing",
			break_: func(t *testing.T, root string) {
				if err := os.Remove(filepath.Join(root, filepath.FromSlash(visionMapPath))); err != nil {
					t.Fatal(err)
				}
			},
			want: []string{visionMapPath + ": map", "does not exist", "every package's part", visionPagePath, "git log -- " + visionMapPath, "1 problem(s)"},
		},
		{
			name: "the page carries no hash",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionPagePath, "<!doctype html>\n<title>Hand-made</title>\n")
			},
			want: []string{visionPagePath + ": page", `carries no <meta name="vision-map-hash">`, "rebuild with the vision skill"},
		},
	})
}

// proposalJSON is a design proposal file as the vision skill writes one.
func proposalJSON(id, status string) string {
	return `{"id": "` + id + `", "feature": "` + id + `", "title": "Add ` + id + `", "status": "` + status + `",` + "\n" +
		`  "summary": {"text": "Adds ` + id + `.", "refs": ["doc:docs/guide.md"]}}` + "\n"
}

// keepPage changes the fixture's inputs with edit, then puts back the page
// built before it, so the page is older than its inputs and nothing else is
// wrong.
func keepPage(edit func(t *testing.T, root string)) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		built := readFile(t, root, visionPagePath)
		edit(t, root)
		writeFile(t, root, visionPagePath, built)
	}
}

func TestCheckReportsASystemMapPageOlderThanItsInputs(t *testing.T) {
	// The page embeds the facts file, the changelog and the design
	// proposals, so it must be rebuilt after any of them changes, as the
	// map hash requires after the map changes.
	stale := []string{visionPagePath + ": page", "built from an older facts file, changelog or design proposals",
		"vision.py build", "commit the page with the facts, the changelog and the proposals", "1 problem(s)"}
	runVisionCases(t, []checkCase{
		{
			name: "the facts changed",
			break_: keepPage(func(t *testing.T, root string) {
				writeFile(t, root, "docs/adr/0003-third-decision.md", "# Make a third decision\n")
				extractFixture(t, root)
			}),
			want: stale,
		},
		{
			name: "the changelog changed",
			break_: keepPage(func(t *testing.T, root string) {
				writeFile(t, root, changelogPath, readFile(t, root, changelogPath)+"\nA last paragraph.\n")
				extractFixture(t, root)
			}),
			want: stale,
		},
		{
			name: "a design proposal was added",
			break_: keepPage(func(t *testing.T, root string) {
				writeFile(t, root, visionProposalsDir+"/top-k.json", proposalJSON("top-k", "proposed"))
			}),
			want: stale,
		},
		{
			name: "a design proposal changed",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionProposalsDir+"/top-k.json", proposalJSON("top-k", "proposed"))
				buildVisionPage(t, root)
				keepPage(func(t *testing.T, root string) {
					writeFile(t, root, visionProposalsDir+"/top-k.json", proposalJSON("top-k", "building"))
				})(t, root)
			},
			want: stale,
		},
		{
			// /flow deletes a proposal once its feature merges, and rebuilds.
			name: "a design proposal was retired",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionProposalsDir+"/top-k.json", proposalJSON("top-k", "building"))
				buildVisionPage(t, root)
				keepPage(func(t *testing.T, root string) {
					if err := os.Remove(filepath.Join(root, filepath.FromSlash(visionProposalsDir), "top-k.json")); err != nil {
						t.Fatal(err)
					}
				})(t, root)
			},
			want: stale,
		},
		{
			name: "the page carries no inputs hash",
			break_: func(t *testing.T, root string) {
				writeVisionPage(t, root, fixtureMapHash(t, root), "")
			},
			want: []string{visionPagePath + ": page", `carries no <meta name="vision-inputs-hash">`, "rebuild with the vision skill", "1 problem(s)"},
		},
	})
}

func TestCheckAcceptsAPageBuiltFromItsInputs(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(t *testing.T, root string)
	}{
		{"with a design proposal", func(t *testing.T, root string) {
			writeFile(t, root, visionProposalsDir+"/top-k.json", proposalJSON("top-k", "proposed"))
			writeFile(t, root, visionProposalsDir+"/notes.md", "Not a proposal: only .json files are.\n")
		}},
		{"with an empty proposals folder", func(t *testing.T, root string) {
			if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(visionProposalsDir)), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newVisionFixture(t)
			tc.edit(t, root)
			buildVisionPage(t, root)
			if code, out := checkFixture(t, root); code != 0 {
				t.Fatalf("check exit %d, want 0:\n%s", code, out)
			}
		})
	}
	t.Run("after a checkout turned the changelog's line endings into CRLF", func(t *testing.T) {
		// The hash reads CRLF as LF, so a Windows checkout of the same
		// commit agrees with the page.
		root := newVisionFixture(t)
		writeFile(t, root, changelogPath, strings.ReplaceAll(readFile(t, root, changelogPath), "\n", "\r\n"))
		if code, out := checkFixture(t, root); code != 0 {
			t.Fatalf("check exit %d, want 0:\n%s", code, out)
		}
	})
}

func TestCheckAcceptsASystemMapStampedOrReformattedAfterItsPageWasBuilt(t *testing.T) {
	// The hash covers the map's content: stamp, publishing, key order and
	// layout leave it unchanged, so a page built before them stays current.
	// The fixture's page was built from the map as written by hand; this is
	// that map stamped, then rewritten with sorted keys and indentation.
	root := newVisionFixture(t)
	stamped := strings.NewReplacer(
		`"parts": "complete"`,
		`"parts": "complete", "commit": "3bcc2db0000000000000000000000000000000000", "stamped": "2026-11-04 (with uncommitted work)", "artifact": "https://claude.ai/artifact/abc"`,
		`"symbol": "Server.Start",`,
		`"symbol": "Server.Start", "hash": "a1b2c3d4e5f6", "src": "func (s *Server) Start() error { return nil }", "startLine": 18, "where": "internal/alpha/alpha.go · lines 18–18",`,
	).Replace(readFile(t, root, visionMapPath))
	if strings.Count(stamped, `"hash"`) != 1 || !strings.Contains(stamped, `"artifact"`) {
		t.Fatal("the stamp fields were not added to the map")
	}
	writeFile(t, root, visionMapPath, stamped)
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("stamped: check exit %d, want 0:\n%s", code, out)
	}
	writeVisionMap(t, root, visionFixtureMap())
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("reformatted: check exit %d, want 0:\n%s", code, out)
	}
}

// readFile reads a fixture file.
func readFile(t *testing.T, root, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
