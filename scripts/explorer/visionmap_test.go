package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The system map rules run only when .vision/map.json exists; every other
// fixture has none, so they are unaffected. These tests write a map whose
// every reference resolves in the fixture repository, and a page carrying
// its hash, then break one thing.

const fixtureScript = "web/stream.ts"

// visionFixtureJSON is a consistent map: it reaches every place a map
// holds a reference, and every reference form check resolves.
const visionFixtureJSON = `{
  "meta": {"title": "Fixture", "facts": "docs/architecture/explorer/data/facts.js", "page": "docs/architecture/explorer/system.html"},
  "kinds": [{"id": "serve", "label": "Serve"}],
  "flows": [{"id": "main", "name": "Main", "steps": [{
    "id": "start", "name": "Start the server", "kind": "serve",
    "code": {"key": "Server.Start", "highlight": ["return nil"]},
    "refs": ["code:internal/alpha/routes.go#Server.routes", "code:internal/alpha", "doc:docs/guide.md", "doc:docs/guide.md#getting-started-1", "Name", "http.HandleFunc"],
    "facts": [
      ["route", "GET /widgets", "route:GET /widgets"], ["guarded by", "requireOwner", "route:GET /widgets"], ["route", "/health", "route:/health"],
      ["package", "internal/alpha", "pkg:internal/alpha"], ["writes", "widgets", "table:fixture.widgets"], ["command", "tool run", "command:tool run"],
      ["limit", "3"]
    ],
    "flags": [{"text": "A decision and a term back this flag.", "refs": ["adr:0001", "term:Widget"]}],
    "values": [
      {"type": "kv", "title": "Limits", "ref": "code:internal/alpha/alpha.go#Limit", "rows": [["limit", "3", "code:internal/alpha/alpha.go#Limit"], ["name", "beta"]]},
      {"type": "big", "title": "Fields", "value": "1", "ref": "code:internal/alpha/alpha.go#Server.Addr"}
    ],
    "files": [{"path": "internal/alpha/alpha.go", "note": "The server"}],
    "children": [{"id": "follow", "name": "Follow", "code": {"key": "http.HandleFunc"}, "refs": ["code:web/stream.ts#followChat"]}]
  }]}],
  "code": {
    "Server.Start": {"file": "internal/alpha/alpha.go", "symbol": "Server.Start", "title": "func (s *Server) Start"},
    "Name": {"file": "internal/alpha/alpha.go", "symbol": "Name"},
    "Handler.Serve": {"file": "internal/alpha/alpha.go", "symbol": "Handler.Serve"},
    "head": {"file": "internal/alpha/alpha.go", "lines": "1-2"},
    "sketch": {"src": "start, then serve", "label": "a sketch"},
    "sendDetached": {"file": "web/stream.ts", "symbol": "sendDetached"},
    "Feed.follow": {"file": "web/stream.ts", "symbol": "Feed.follow"}
  },
  "refs": {"http.HandleFunc": {"lib": "net/http", "sig": "http.HandleFunc(pattern, handler)", "text": "Registers a handler."}},
  "history": [{"date": "2026-11-04", "summary": "Map created"}]
}`

func visionFixtureMap() map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(visionFixtureJSON), &m); err != nil {
		panic(err)
	}
	return m
}

// writeVision writes m as the fixture's map, and the page built from it.
func writeVision(t *testing.T, root string, m map[string]any) {
	t.Helper()
	data := writeVisionMap(t, root, m)
	hash, err := mapHash(data)
	if err != nil {
		t.Fatal(err)
	}
	writeVisionPage(t, root, hash)
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

func writeVisionPage(t *testing.T, root, hash string) {
	t.Helper()
	writeFile(t, root, visionPagePath, "<!doctype html>\n<meta charset=\"utf-8\">\n<meta name=\"vision-map-hash\" content=\""+hash+"\">\n<title>Fixture</title>\n")
}

// newVisionFixture is the consistent fixture with a consistent map.
func newVisionFixture(t *testing.T) string {
	t.Helper()
	root := newFixture(t)
	extractFixture(t, root)
	writeFile(t, root, fixtureScript, "export function followChat() {}\nexport const sendDetached = () => {};\nexport class Feed {\n  follow() {}\n}\n")
	writeVision(t, root, visionFixtureMap())
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
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newVisionFixture(t)
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

func TestCheckReportsSystemMapCodeEntriesThatNoLongerResolve(t *testing.T) {
	entry := func(key string, e map[string]any) func(*testing.T, string) {
		return breakMap(func(m, _ map[string]any) { m["code"].(map[string]any)[key] = e })
	}
	runVisionCases(t, []checkCase{
		{
			name:   "Go method renamed",
			break_: entry("Server.Start", map[string]any{"file": "internal/alpha/alpha.go", "symbol": "Server.Begin"}),
			want:   []string{visionMapPath, "code.Server.Start", "declares no Server.Begin", "Type.Method", "vision.py build"},
		},
		{
			name:   "Go method named without its type",
			break_: entry("Server.Start", map[string]any{"file": "internal/alpha/alpha.go", "symbol": "Start"}),
			want:   []string{"code.Server.Start", "declares no Start"},
		},
		{
			name:   "file removed",
			break_: entry("Name", map[string]any{"file": "internal/alpha/gone.go", "symbol": "Name"}),
			want:   []string{"code.Name", "internal/alpha/gone.go does not exist", "point file at an existing repository file"},
		},
		{
			name:   "file outside the repository",
			break_: entry("Name", map[string]any{"file": "../alpha.go", "symbol": "Name"}),
			want:   []string{"code.Name", "inside the repository"},
		},
		{
			name:   "TypeScript function renamed",
			break_: entry("sendDetached", map[string]any{"file": fixtureScript, "symbol": "sendLater"}),
			want:   []string{"code.sendDetached", fixtureScript + " declares no sendLater"},
		},
		{
			name:   "TypeScript method renamed",
			break_: entry("Feed.follow", map[string]any{"file": fixtureScript, "symbol": "Feed.unfollow"}),
			want:   []string{"code.Feed.follow", "declares no Feed.unfollow"},
		},
		{
			name:   "symbol in a language check cannot read",
			break_: entry("Name", map[string]any{"file": "docs/guide.md", "symbol": "Guide"}),
			want:   []string{"code.Name", "not .md files", "cite lines instead"},
		},
		{
			name:   "lines past the end of the file",
			break_: entry("head", map[string]any{"file": "internal/alpha/alpha.go", "lines": "20-400"}),
			want:   []string{"code.head", "lines 20-400 are outside internal/alpha/alpha.go"},
		},
		{
			name:   "lines that are not a range",
			break_: entry("head", map[string]any{"file": "internal/alpha/alpha.go", "lines": "top"}),
			want:   []string{"code.head", `lines "top" is not a range`},
		},
		{
			name:   "cell of a file that is not a notebook",
			break_: entry("head", map[string]any{"file": "internal/alpha/alpha.go", "cell": 2}),
			want:   []string{"code.head", "is not a notebook"},
		},
		{
			name:   "inline entry without source",
			break_: entry("sketch", map[string]any{"label": "a sketch"}),
			want:   []string{"code.sketch", "neither src nor a file reference"},
		},
		{
			name:   "symbol without a file",
			break_: entry("sketch", map[string]any{"symbol": "Name"}),
			want:   []string{"code.sketch", "no file"},
		},
	})
}

func TestCheckReportsSystemMapNotebookCells(t *testing.T) {
	runVisionCases(t, []checkCase{{
		name: "cell that is not a code cell",
		break_: func(t *testing.T, root string) {
			writeFile(t, root, "notes/train.ipynb", `{"cells": [{"cell_type": "markdown", "source": "# Train"}, {"cell_type": "code", "source": "x = 1"}]}`)
			m := visionFixtureMap()
			m["code"].(map[string]any)["cell"] = map[string]any{"file": "notes/train.ipynb", "cell": 0}
			m["code"].(map[string]any)["ok"] = map[string]any{"file": "notes/train.ipynb", "cell": 1}
			writeVision(t, root, m)
		},
		want: []string{"code.cell", "cell 0 is not a code cell in notes/train.ipynb"},
	}})
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
		{"kind the explorer check does not resolve", "sec:3.1", "does not resolve sec: references"},
		{"explorer node", "node:pkg:internal/alpha", "does not resolve node: references"},
		{"unknown key", "NoSuchKey", "neither a code or refs key"},
	} {
		cases = append(cases, checkCase{
			name:   tc.name,
			break_: breakMap(func(_, step map[string]any) { appendTo(step, "refs", tc.ref) }),
			want:   []string{visionMapPath, "step start refs", "dead reference " + jsonQuote(tc.ref), tc.why, "vision.py build"},
		})
	}
	runVisionCases(t, cases)
}

func jsonQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func TestCheckReportsSystemMapFactChipsFlagsAndPanelsThatNoLongerResolve(t *testing.T) {
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
			name: "value panel",
			break_: breakMap(func(_, step map[string]any) {
				step["values"].([]any)[1].(map[string]any)["ref"] = "code:internal/alpha/alpha.go#Server.Port"
			}),
			want: []string{`step start big panel "Fields"`, "declares no Server.Port"},
		},
		{
			name: "kv panel row",
			break_: breakMap(func(_, step map[string]any) {
				row := step["values"].([]any)[0].(map[string]any)["rows"].([]any)[1].([]any)
				step["values"].([]any)[0].(map[string]any)["rows"].([]any)[1] = append(row, "code:internal/alpha/alpha.go#Title")
			}),
			want: []string{`step start kv panel "Limits" row "name"`, "declares no Title"},
		},
		{
			name: "step code key",
			break_: breakMap(func(_, step map[string]any) {
				step["code"] = map[string]any{"key": "Server.Stop"}
			}),
			want: []string{"step start code", `code key "Server.Stop" is not in the map's code or refs`},
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
		{
			name: "a single top-level flow",
			break_: breakMap(func(m, _ map[string]any) {
				flow := m["flows"].([]any)[0].(map[string]any)
				delete(m, "flows")
				m["flow"] = flow["steps"]
				appendTo(visionStepOfFlow(m), "refs", "doc:docs/missing.md")
			}),
			want: []string{"step start refs", `dead reference "doc:docs/missing.md"`},
		},
	})
}

func visionStepOfFlow(m map[string]any) map[string]any {
	return m["flow"].([]any)[0].(map[string]any)
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
				writeVisionPage(t, root, built)
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
			name: "the page carries no hash",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionPagePath, "<!doctype html>\n<title>Hand-made</title>\n")
			},
			want: []string{visionPagePath + ": page", `carries no <meta name="vision-map-hash">`, "rebuild with the vision skill"},
		},
	})
}

func TestCheckAcceptsASystemMapStampedOrReformattedAfterItsPageWasBuilt(t *testing.T) {
	// The hash covers the map's content: stamp, publishing, key order and
	// layout leave it unchanged, so a page built before them stays current.
	// The fixture's page was built from the map written with sorted keys and
	// indentation; this is the map as written by hand, then stamped.
	root := newVisionFixture(t)
	stamped := strings.NewReplacer(
		`"page": "docs/architecture/explorer/system.html"}`,
		`"page": "docs/architecture/explorer/system.html", "commit": "3bcc2db0000000000000000000000000000000000", "stamped": "2026-11-04 (with uncommitted work)", "artifact": "https://claude.ai/artifact/abc"}`,
		`"symbol": "Server.Start",`,
		`"symbol": "Server.Start", "hash": "a1b2c3d4e5f6", "src": "func (s *Server) Start() error { return nil }", "startLine": 18, "where": "internal/alpha/alpha.go · lines 18–18",`,
	).Replace(visionFixtureJSON)
	if strings.Count(stamped, `"hash"`) != 1 || !strings.Contains(stamped, `"artifact"`) {
		t.Fatal("the stamp fields were not added to the map")
	}
	writeFile(t, root, visionMapPath, stamped)
	code, out := checkFixture(t, root)
	if code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}
