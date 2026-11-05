package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The system map (system-page.decisions.md, 2026-11-04). Evie's System view
// opens on a page the vision skill builds from .vision/map.json and that the
// repository commits beside the explorer. CI cannot run the skill, which
// lives outside the repository, so check holds the map to the code itself:
// every reference in the map resolves against the code and the facts, and
// the committed page carries the current map's hash. Building the page runs
// the skill's own check first, so a page built from the current map also
// means the map passed that check. All of this runs only when the map
// exists.
const (
	visionMapPath = ".vision/map.json"
	// visionPagePath is where the page is built: the snapshot package
	// embeds it, and the System view frames it.
	visionPagePath = "docs/architecture/explorer/system.html"
	visionBuild    = "python3 ~/.claude/skills/vision/scripts/vision.py build"
	visionRebuild  = "rebuild with the vision skill: " + visionBuild + ", and commit the page with the map"
	visionFormat   = "the vision skill's references/map-format.md"
)

// visionMap is the part of a map the check reads: where it says its facts
// and page are, its flows (or one bare flow), its code references, and its
// library references. Other fields belong to the page and are not read.
type visionMap struct {
	Meta struct {
		Facts string `json:"facts"`
		Page  string `json:"page"`
	} `json:"meta"`
	Flows []visionFlow               `json:"flows"`
	Flow  []visionStep               `json:"flow"`
	Code  map[string]visionCode      `json:"code"`
	Refs  map[string]json.RawMessage `json:"refs"`
}

type visionFlow struct {
	ID    string       `json:"id"`
	Steps []visionStep `json:"steps"`
}

// visionStep is one step and the references it carries.
type visionStep struct {
	ID   string `json:"id"`
	Code *struct {
		Key string `json:"key"`
	} `json:"code"`
	Refs   []string      `json:"refs"`
	Facts  [][]any       `json:"facts"`
	Flags  []visionFlag  `json:"flags"`
	Values []visionValue `json:"values"`
	Files  []struct {
		Path string `json:"path"`
	} `json:"files"`
	Children []visionStep `json:"children"`
}

type visionFlag struct {
	Text string   `json:"text"`
	Refs []string `json:"refs"`
}

// visionValue is a value panel: its own reference, and for a kv panel the
// rows that may each carry one.
type visionValue struct {
	Type  string          `json:"type"`
	Title string          `json:"title"`
	Ref   string          `json:"ref"`
	Rows  json.RawMessage `json:"rows"`
}

// visionCode is a code entry: a definition read from a file by symbol,
// line range or notebook cell, or inline source with no file.
type visionCode struct {
	File   string          `json:"file"`
	Symbol *string         `json:"symbol"`
	Lines  json.RawMessage `json:"lines"`
	Cell   *int            `json:"cell"`
	Src    *string         `json:"src"`
}

func (e visionCode) readsRepo() bool { return e.Symbol != nil || e.Lines != nil || e.Cell != nil }

// visionRefKinds are the reference kinds Evie's map may use: code and docs,
// resolved against the repository, and facts, resolved against the facts
// this check extracts. The skill's notebook and text-source kinds (undef:,
// out:, sec: and the rest) need sources Evie's map does not have.
var visionRefKinds = map[string]bool{
	"code": true, "doc": true, "pkg": true, "route": true, "table": true, "command": true, "adr": true, "term": true,
}

// validateVisionMap checks the system map when the repository has one.
func (c *checker) validateVisionMap() {
	data, err := os.ReadFile(c.path(visionMapPath))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		c.add(visionMapPath, "map", fmt.Sprintf("the map cannot be read: %v", err), "make the file readable")
		return
	}
	var m visionMap
	if err := json.Unmarshal(data, &m); err != nil {
		c.add(visionMapPath, "map", "malformed map: "+err.Error(), "make the map match "+visionFormat)
		return
	}
	c.visionMeta(m)
	v := &visionChecker{checker: c, m: m}
	for _, key := range slices.Sorted(maps.Keys(m.Code)) {
		v.codeEntry(key, m.Code[key])
	}
	flows := m.Flows
	if flows == nil && m.Flow != nil {
		flows = []visionFlow{{ID: "main", Steps: m.Flow}}
	}
	for _, f := range flows {
		for _, step := range f.Steps {
			v.step(step)
		}
	}
	c.visionPageHash(data, m.Meta.Page)
}

// visionMeta checks that the map's facts and page are the ones Evie's check
// and binary use.
func (c *checker) visionMeta(m visionMap) {
	if m.Meta.Facts != factsPath {
		c.add(visionMapPath, "meta.facts", fmt.Sprintf("meta.facts is %q, but the map's fact references resolve against %s", m.Meta.Facts, factsPath),
			fmt.Sprintf("set meta.facts to %q, then %s", factsPath, visionRebuild))
	}
	if m.Meta.Page != visionPagePath {
		c.add(visionMapPath, "meta.page", fmt.Sprintf("meta.page is %q, but Evie embeds and serves %s", m.Meta.Page, visionPagePath),
			fmt.Sprintf("set meta.page to %q, then %s", visionPagePath, visionRebuild))
	}
}

// visionPageHash checks that the committed page was built from this map.
func (c *checker) visionPageHash(mapData []byte, page string) {
	if !cleanRelative(page) {
		return // reported by visionMeta
	}
	want, err := mapHash(mapData)
	if err != nil {
		c.add(visionMapPath, "map hash", "the map's hash cannot be computed: "+err.Error(), "make the map valid JSON that "+visionFormat+" describes")
		return
	}
	html, err := os.ReadFile(c.path(page))
	if err != nil {
		c.add(page, "page", "the built page does not exist", visionRebuild)
		return
	}
	m := pageHashMeta.FindSubmatch(html)
	switch {
	case m == nil:
		c.add(page, "page", `the page carries no <meta name="vision-map-hash">, so it was not built by the vision skill`, visionRebuild)
	case string(m[1]) != want:
		c.add(page, "page", fmt.Sprintf("the page was built from an older map: it carries %s, and %s hashes to %s", m[1], visionMapPath, want), visionRebuild)
	}
}

// pageHashMeta finds the map hash in a built page, as the skill reads it.
var pageHashMeta = regexp.MustCompile(`<meta name="vision-map-hash" content="([^"]+)"`)

// visionChecker checks the references of one map.
type visionChecker struct {
	*checker
	m visionMap
}

func (v *visionChecker) dead(item, ref, why string) {
	kind, _, _ := strings.Cut(ref, ":")
	fix, ok := refFixes[kind]
	if !ok {
		fix = "use a code or refs key of the map, or a code:, doc:, pkg:, route:, table:, command:, adr: or term: reference"
	}
	v.add(visionMapPath, item, fmt.Sprintf("dead reference %q: %s", ref, why), fix+", then "+visionRebuild)
}

// ref reports ref when it does not resolve. A reference is a key of the
// map's code or refs, or kind:target.
func (v *visionChecker) ref(item, ref string) {
	if _, ok := v.m.Code[ref]; ok {
		return
	}
	if _, ok := v.m.Refs[ref]; ok {
		return
	}
	kind, target, ok := strings.Cut(ref, ":")
	var why string
	switch {
	case !ok || target == "":
		why = "it is neither a code or refs key of the map nor a kind:target reference"
	case !visionRefKinds[kind]:
		why = "the explorer check does not resolve " + kind + ": references; Evie's map cites code, docs and facts"
	case kind == "code":
		why = v.resolveMapCode(target)
	default:
		why = v.resolve(ref, nil)
	}
	if why != "" {
		v.dead(item, ref, why)
	}
}

// resolveMapCode resolves code:path or code:path#Name. Unlike the explorer's
// own references, a map may name declarations in TypeScript and JavaScript
// as well as Go.
func (v *visionChecker) resolveMapCode(target string) string {
	file, symbol, hasSymbol := strings.Cut(target, "#")
	if !hasSymbol {
		return v.resolveCode(target)
	}
	return v.resolveSymbol(file, symbol)
}

func (v *visionChecker) step(s visionStep) {
	item := "step " + s.ID
	if s.Code != nil {
		_, inCode := v.m.Code[s.Code.Key]
		_, inRefs := v.m.Refs[s.Code.Key]
		if !inCode && !inRefs {
			v.add(visionMapPath, item+" code", fmt.Sprintf("code key %q is not in the map's code or refs", s.Code.Key),
				"add it to code, or correct the key, then "+visionRebuild)
		}
	}
	for _, ref := range s.Refs {
		v.ref(item+" refs", ref)
	}
	for _, fact := range s.Facts {
		if len(fact) > 2 {
			if ref, ok := fact[2].(string); ok && ref != "" {
				v.ref(fmt.Sprintf("%s fact %q", item, factLabel(fact)), ref)
			}
		}
	}
	for _, flag := range s.Flags {
		for _, ref := range flag.Refs {
			v.ref(fmt.Sprintf("%s flag %q", item, clip(flag.Text, 40)), ref)
		}
	}
	for _, value := range s.Values {
		panel := fmt.Sprintf("%s %s panel %q", item, value.Type, value.Title)
		if value.Ref != "" {
			v.ref(panel, value.Ref)
		}
		if value.Type == "kv" {
			var rows [][]any
			if json.Unmarshal(value.Rows, &rows) == nil {
				for _, row := range rows {
					if len(row) > 2 {
						if ref, ok := row[2].(string); ok && ref != "" {
							v.ref(fmt.Sprintf("%s row %q", panel, factLabel(row)), ref)
						}
					}
				}
			}
		}
	}
	for _, file := range s.Files {
		if why := v.resolveCode(file.Path); why != "" {
			v.add(visionMapPath, item+" files", fmt.Sprintf("file %q: %s", file.Path, why),
				"point it at an existing repository file, or remove it, then "+visionRebuild)
		}
	}
	for _, child := range s.Children {
		v.step(child)
	}
}

func factLabel(fact []any) string {
	if label, ok := fact[0].(string); ok {
		return label
	}
	return fmt.Sprint(fact[0])
}

// clip shortens s to n characters for a problem's item.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// codeEntry checks one code entry: inline source needs no file; anything
// else names a file, and its symbol, line range or notebook cell resolves.
func (v *visionChecker) codeEntry(key string, e visionCode) {
	item := "code." + key
	fail := func(message, fix string) {
		v.add(visionMapPath, item, message, fix+", then "+visionRebuild)
	}
	if !e.readsRepo() {
		if e.Src == nil {
			fail("the entry has neither src nor a file reference (symbol, lines or cell)", "give it a file and a symbol, or inline src")
		}
		return
	}
	if e.File == "" {
		fail("the entry gives symbol, lines or cell but no file", "add the file it reads")
		return
	}
	if why := v.resolveCode(e.File); why != "" {
		fail(why, "point file at an existing repository file")
		return
	}
	if e.Symbol != nil {
		if why := v.resolveSymbol(e.File, *e.Symbol); why != "" {
			fail(why, "rename the symbol to the declaration as the code now spells it (Type.Method for a method), or re-point the entry")
		}
	}
	if e.Cell != nil {
		if why := v.resolveCell(e.File, *e.Cell); why != "" {
			fail(why, "point cell at a code cell of the notebook")
		}
	}
	if e.Lines != nil {
		from, to, ok := parseLineRange(e.Lines)
		switch {
		case !ok:
			fail(fmt.Sprintf("lines %s is not a range like \"10-42\"", e.Lines), "write lines as \"from-to\"")
		case e.Symbol == nil && e.Cell == nil:
			// Lines inside a symbol or cell are relative to where it
			// starts; on their own they must lie inside the file.
			if n := v.lineCount(e.File); from < 1 || from > to || to > n {
				fail(fmt.Sprintf("lines %d-%d are outside %s, which has %d lines", from, to, e.File, n), "cite lines the file has, or a symbol")
			}
		}
	}
}

// lineRange is a lines value: "10-42", "10:42", "10–42", "10", or a number.
var lineRange = regexp.MustCompile(`^\s*(\d+)\s*(?:[-–:]\s*(\d+))?\s*$`)

func parseLineRange(raw json.RawMessage) (from, to int, ok bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		var n int
		if json.Unmarshal(raw, &n) != nil {
			return 0, 0, false
		}
		text = strconv.Itoa(n)
	}
	m := lineRange.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, false
	}
	from, _ = strconv.Atoi(m[1])
	to = from
	if m[2] != "" {
		to, _ = strconv.Atoi(m[2])
	}
	return from, to, true
}

func (v *visionChecker) lineCount(file string) int {
	data, err := os.ReadFile(v.path(file))
	if err != nil {
		return 0
	}
	return len(strings.Split(string(data), "\n"))
}

// resolveSymbol returns "" when file declares symbol, or why it does not:
// Go files by their declarations (Name, or Type.Name for a method, field or
// interface method), JavaScript and TypeScript files by a declaring line.
func (v *visionChecker) resolveSymbol(file, symbol string) string {
	if !cleanRelative(file) {
		return notInRepo
	}
	info, err := os.Stat(v.path(file))
	if err != nil {
		return file + " does not exist"
	}
	if info.IsDir() {
		return file + " is a directory, so it declares nothing"
	}
	switch ext := fileExt(file); ext {
	case ".go":
		decls, err := v.goDecls(file)
		if err != nil {
			return err.Error()
		}
		if !decls[symbol] {
			return file + " declares no " + symbol
		}
		return ""
	case ".js", ".jsx", ".mjs", ".ts", ".tsx":
		data, err := os.ReadFile(v.path(file))
		if err != nil {
			return err.Error()
		}
		if !scriptDeclares(string(data), symbol) {
			return file + " declares no " + symbol
		}
		return ""
	default:
		return "the explorer check reads declarations only in Go, JavaScript and TypeScript files, not " + ext + " files; cite lines instead"
	}
}

func fileExt(file string) string {
	if i := strings.LastIndex(file, "."); i > strings.LastIndex(file, "/") {
		return file[i:]
	}
	return ""
}

// scriptDeclares reports whether JavaScript or TypeScript source declares
// name on a line of its own: a function, class, interface, type, enum or
// variable. Owner.member needs Owner declared and member named after it.
func scriptDeclares(src, symbol string) bool {
	owner, member, dotted := strings.Cut(symbol, ".")
	decl := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(?:async\s+)?` +
		`(?:function\*?|class|interface|type|enum|const|let|var)\s+` + regexp.QuoteMeta(owner) + `\b`)
	loc := decl.FindStringIndex(src)
	if loc == nil {
		return false
	}
	if !dotted {
		return true
	}
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(member) + `\b`).MatchString(src[loc[1]:])
}

// resolveCell returns "" when the notebook has a code cell at index cell,
// counting from 0 with markdown cells included.
func (v *visionChecker) resolveCell(file string, cell int) string {
	if fileExt(file) != ".ipynb" {
		return file + " is not a notebook, so it has no cells"
	}
	data, err := os.ReadFile(v.path(file))
	if err != nil {
		return file + " does not exist"
	}
	var nb struct {
		Cells []struct {
			CellType string `json:"cell_type"`
		} `json:"cells"`
	}
	if err := json.Unmarshal(data, &nb); err != nil {
		return file + " is not a notebook: " + err.Error()
	}
	if cell < 0 || cell >= len(nb.Cells) || nb.Cells[cell].CellType != "code" {
		return fmt.Sprintf("cell %d is not a code cell in %s", cell, file)
	}
	return ""
}
