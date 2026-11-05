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
	"strings"
	"unicode"
	"unicode/utf8"
)

// The system map (system-page.decisions.md, 2026-11-04). Evie's System view
// opens on a page the vision skill builds from .vision/map.json and that the
// repository commits beside the explorer. CI cannot run the skill, which
// lives outside the repository, so check holds the map to the code itself:
// every reference in the map resolves against the code and the facts, and
// the committed page carries the current map's hash. Building the page runs
// the skill's own check first, so a page built from the current map also
// means the map passed that check. Check reads only the forms Evie's map
// uses, and refuses the skill's others rather than passing them unread.
const (
	visionMapPath = ".vision/map.json"
	// visionPagePath is where the page is built: the snapshot package
	// embeds it, and the System view frames it.
	visionPagePath = "docs/architecture/explorer/system.html"
	visionBuild    = "python3 ~/.claude/skills/vision/scripts/vision.py build"
	visionRebuild  = "rebuild with the vision skill: " + visionBuild + ", and commit the page with the map"
	visionCheck    = "python3 ~/.claude/skills/vision/scripts/vision.py check .vision/map.json"
	visionFormat   = "the vision skill's references/map-format.md"
)

// visionMap is the part of a map the check reads: where it says its facts
// and page are, its flows, and its code entries. A top-level flow and
// library definitions (refs) are read only to refuse them. Other fields
// belong to the page and are not read.
type visionMap struct {
	Meta struct {
		Facts string `json:"facts"`
		Page  string `json:"page"`
	} `json:"meta"`
	Flows []visionFlow               `json:"flows"`
	Flow  json.RawMessage            `json:"flow"`
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

// visionValue is a value panel. Its ref, and a kv row's reference, are read
// only to refuse them.
type visionValue struct {
	Type  string          `json:"type"`
	Title string          `json:"title"`
	Ref   string          `json:"ref"`
	Rows  json.RawMessage `json:"rows"`
}

// visionCode is a code entry: a declaration read from a file by symbol.
// Line ranges, notebook cells and inline source are read only to refuse
// them; src beside a symbol is the copy resolving it fills in.
type visionCode struct {
	File   string          `json:"file"`
	Symbol string          `json:"symbol"`
	Lines  json.RawMessage `json:"lines"`
	Cell   json.RawMessage `json:"cell"`
	Src    *string         `json:"src"`
}

// visionRefFixes are the reference kinds a map may use, with how to fix
// each: the explorer's kinds but node: (a map cites code, docs and facts,
// not explorer nodes), with code: and doc: in the map's forms.
var visionRefFixes = func() map[string]string {
	fixes := maps.Clone(refFixes)
	delete(fixes, "node")
	fixes["code"] = "point it at an existing repository file or folder, with #Name or #Type.Method naming a declaration in that Go, JavaScript or TypeScript file"
	fixes["doc"] = "point it at an existing repository file or folder, with #anchor naming one of a Markdown file's headings (GitHub slug)"
	return fixes
}()

// validateVisionMap checks the system map when the repository has one, and
// that it has one when it commits the page built from it.
func (c *checker) validateVisionMap() {
	data, err := os.ReadFile(c.path(visionMapPath))
	if errors.Is(err, fs.ErrNotExist) {
		if _, err := os.Stat(c.path(visionPagePath)); err == nil {
			c.add(visionMapPath, "map", "the file does not exist, but "+visionPagePath+", the page built from it, does",
				"restore the map the page was built from (git log -- "+visionMapPath+" finds it) and commit it with the page")
		}
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
	v := &visionChecker{checker: c, m: m, facts: factTextsOf(c.facts)}
	if m.Flow != nil && string(m.Flow) != "null" {
		v.unsupported("flow", "a top-level flow", "give the map a flows list")
	}
	if len(m.Refs) > 0 {
		v.unsupported("refs", "library definitions ("+strings.Join(slices.Sorted(maps.Keys(m.Refs)), ", ")+")",
			"remove them, and cite the code that calls the library")
	}
	for _, key := range slices.Sorted(maps.Keys(m.Code)) {
		v.codeEntry(key, m.Code[key])
	}
	for _, f := range m.Flows {
		for _, step := range f.Steps {
			v.step(step)
		}
	}
	c.visionPageHash(data)
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

// visionPageHash checks that the page Evie embeds was built from this map,
// whatever meta.page says.
func (c *checker) visionPageHash(mapData []byte) {
	want, err := mapHash(mapData)
	if err != nil {
		c.add(visionMapPath, "map hash", "the map's hash cannot be computed: "+err.Error(), "make the map valid JSON that "+visionFormat+" describes")
		return
	}
	html, err := os.ReadFile(c.path(visionPagePath))
	if err != nil {
		c.add(visionPagePath, "page", "the built page does not exist", visionRebuild)
		return
	}
	switch m := pageHashMeta.FindSubmatch(html); {
	case m == nil:
		c.add(visionPagePath, "page", `the page carries no <meta name="vision-map-hash">, so it was not built by the vision skill`, visionRebuild)
	case string(m[1]) != want:
		c.add(visionPagePath, "page", fmt.Sprintf("the page was built from an older map: it carries %s, and %s hashes to %s", m[1], visionMapPath, want), visionRebuild)
	}
}

// pageHashMeta finds the map hash in a built page, as the skill reads it.
var pageHashMeta = regexp.MustCompile(`<meta name="vision-map-hash" content="([^"]+)"`)

// visionChecker checks the references of one map.
type visionChecker struct {
	*checker
	m     visionMap
	facts factTexts
}

// unsupported refuses a form of the skill's map format that Evie's map does
// not use, naming the check that reads it.
func (v *visionChecker) unsupported(item, form, instead string) {
	v.add(visionMapPath, item, form+" is a form only the vision skill's check reads ("+visionCheck+"); the explorer check reads the forms Evie's map uses",
		instead+", or teach scripts/explorer/visionmap.go the form, then "+visionRebuild)
}

// mapRef reports ref when it does not resolve, and returns whether it does.
// A reference is a key of the map's code, or kind:target.
func (v *visionChecker) mapRef(item, ref string) bool {
	if _, ok := v.m.Code[ref]; ok {
		return true
	}
	kind, target, ok := strings.Cut(ref, ":")
	fix, known := visionRefFixes[kind]
	var why string
	switch {
	case !ok || target == "":
		why = "it is neither a code key of the map nor a kind:target reference"
		fix = "use a code key of the map, or one of " + strings.Join(slices.Sorted(maps.Keys(visionRefFixes)), ":, ") + ":"
	case !known:
		v.unsupported(item, kind+": references", "cite code, docs or facts instead")
		return false
	default:
		why = v.resolveIn(ref, nil, mapForms)
	}
	if why != "" {
		v.add(visionMapPath, item, fmt.Sprintf("dead reference %q: %s", ref, why), fix+", then "+visionRebuild)
		return false
	}
	return true
}

// chip reads a fact or kv row, [label, value] or [label, value, reference].
func chip(row []any) (label string, value any, ref string) {
	if len(row) > 0 {
		label = fmt.Sprint(row[0])
		if s, ok := row[0].(string); ok {
			label = s
		}
	}
	if len(row) > 1 {
		value = row[1]
	}
	if len(row) > 2 {
		ref, _ = row[2].(string)
	}
	return label, value, ref
}

func (v *visionChecker) step(s visionStep) {
	item := "step " + s.ID
	if s.Code != nil {
		if _, ok := v.m.Code[s.Code.Key]; !ok {
			v.add(visionMapPath, item+" code", fmt.Sprintf("code key %q is not in the map's code", s.Code.Key),
				"add it to code, or correct the key, then "+visionRebuild)
		}
	}
	for _, ref := range s.Refs {
		v.mapRef(item+" refs", ref)
	}
	for _, fact := range s.Facts {
		label, value, ref := chip(fact)
		if chip := fmt.Sprintf("%s fact %q", item, label); ref != "" && v.mapRef(chip, ref) {
			v.backed(chip, value, ref)
		}
	}
	for _, flag := range s.Flags {
		for _, ref := range flag.Refs {
			v.mapRef(fmt.Sprintf("%s flag %q", item, clip(flag.Text, 40)), ref)
		}
	}
	for _, value := range s.Values {
		panel := fmt.Sprintf("%s %s panel %q", item, value.Type, value.Title)
		if value.Ref != "" {
			v.unsupported(panel, "a value panel's ref", "cite the value in a fact chip, [label, value, reference]")
		}
		var rows [][]any
		if value.Type == "kv" && json.Unmarshal(value.Rows, &rows) == nil {
			for _, row := range rows {
				if label, _, ref := chip(row); ref != "" {
					v.unsupported(fmt.Sprintf("%s row %q", panel, label), "a kv row's reference", "cite the value in a fact chip, [label, value, reference]")
				}
			}
		}
	}
	for _, file := range s.Files {
		if _, why := v.stat(file.Path); why != "" {
			v.add(visionMapPath, item+" files", fmt.Sprintf("file %q: %s", file.Path, why),
				"point it at an existing repository file, or remove it, then "+visionRebuild)
		}
	}
	for _, child := range s.Children {
		v.step(child)
	}
}

// clip shortens s to n characters for a problem's item.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

// codeEntry checks one code entry: it names a file and a symbol, which the
// file declares, as code:file#symbol would.
func (v *visionChecker) codeEntry(key string, e visionCode) {
	item := "code." + key
	switch {
	case e.Lines != nil:
		v.unsupported(item, "a line range", "name the declaration with symbol instead")
	case e.Cell != nil:
		v.unsupported(item, "a notebook cell", "name the declaration with symbol instead")
	case e.Symbol == "" && e.Src != nil:
		v.unsupported(item, "inline src", "cite code in the repository by file and symbol")
	case e.Symbol == "":
		v.add(visionMapPath, item, "the entry names no file and symbol", "give it the file and symbol of the declaration it shows, then "+visionRebuild)
	case e.File == "":
		v.add(visionMapPath, item, "the entry gives a symbol but no file", "add the file that declares it, then "+visionRebuild)
	default:
		if why := v.resolveCode(e.File+"#"+e.Symbol, mapForms); why != "" {
			v.add(visionMapPath, item, why, "point file at an existing file and symbol at a declaration in it, as the code now spells it (Type.Method for a method), then "+visionRebuild)
		}
	}
}

// backed reports a fact chip whose value is not in what its reference
// points at, as the skill's check does (references/map-format.md, "backed
// values"): a declaration's lines, a whole file, or the fact's own fields.
func (v *visionChecker) backed(item string, value any, ref string) {
	text, ok := value.(string)
	if !ok {
		v.unsupported(item, "a value that is not a string", "write the value as text")
		return
	}
	if region, ok := v.region(item, ref, text); ok && !backs(region, text) {
		v.add(visionMapPath, item, fmt.Sprintf("says %q, which %q does not", clip(text, 60), ref),
			"re-read the source and write the value as it now says it, or re-point the reference, then "+visionRebuild)
	}
}

// region returns the text a value backed by ref, which resolves, is looked
// for in. It returns false when there is none: reported here, or for a code
// entry that does not resolve, at the entry.
func (v *visionChecker) region(item, ref, value string) (string, bool) {
	if e, ok := v.m.Code[ref]; ok {
		text, why := v.declaration(e.File, e.Symbol, mapForms)
		return text, why == ""
	}
	kind, target, _ := strings.Cut(ref, ":")
	switch kind {
	case "code":
		file, symbol, hasSymbol := strings.Cut(target, "#")
		if hasSymbol {
			text, why := v.declaration(file, symbol, mapForms)
			return text, why == ""
		}
		if info, _ := v.stat(file); info.IsDir() {
			v.add(visionMapPath, item, fmt.Sprintf("%q names a folder, which has no text to find %q in", ref, clip(value, 60)),
				"cite a file or a declaration in it, then "+visionRebuild)
			return "", false
		}
		data, err := os.ReadFile(v.path(file))
		return string(data), err == nil
	case "doc":
		v.unsupported(item, "a value backed by a doc: reference", "cite the code or the fact that holds the value")
		return "", false
	}
	return v.facts.lookup(kind, target), true
}

// factTexts holds, by reference kind and then target, the text a value
// backed by a facts reference is looked for in, as the skill's facts_index
// builds it: the target and the fact's own fields. Targets keep the order
// they were first seen in, and a repeated target keeps its last text.
type factTexts map[string]*factKind

type factKind struct {
	order []string
	text  map[string]string
}

func factTextsOf(facts Facts) factTexts {
	f := factTexts{}
	for _, p := range facts.Packages {
		f.put("pkg", p.Path, p.Name, p.Doc)
	}
	for _, r := range facts.Routes {
		target := r.Pattern
		if r.Method != "" {
			target = r.Method + " " + r.Pattern
		}
		f.put("route", target, r.Guard, r.Package, r.Source.Function)
	}
	for _, d := range facts.Databases {
		for _, t := range d.Tables {
			fields := []string{t.Name}
			for _, col := range t.Columns {
				fields = append(fields, col.Name)
			}
			f.put("table", d.ID+"."+t.Name, fields...)
		}
	}
	for _, cmd := range facts.Commands {
		f.put("command", strings.Join(append([]string{cmd.Binary}, cmd.Words...), " "), cmd.Package)
	}
	for _, a := range facts.ADRs {
		f.put("adr", a.Number, a.Title, a.Path)
	}
	for _, g := range facts.Glossary {
		f.put("term", g.Term, g.Definition)
	}
	return f
}

func (f factTexts) put(kind, target string, fields ...string) {
	if target == "" {
		return
	}
	k := f[kind]
	if k == nil {
		k = &factKind{text: map[string]string{}}
		f[kind] = k
	}
	if _, ok := k.text[target]; !ok {
		k.order = append(k.order, target)
	}
	parts := []string{target}
	for _, field := range fields {
		if field != "" {
			parts = append(parts, field)
		}
	}
	k.text[target] = strings.Join(parts, " ")
}

// lookup returns a target's text, as the skill's lookup_fact finds it:
// exactly, or for a bare route pattern the first route with that pattern,
// or for a bare binary its first command.
func (f factTexts) lookup(kind, target string) string {
	k := f[kind]
	if k == nil {
		return ""
	}
	if text, ok := k.text[target]; ok {
		return text
	}
	for _, key := range k.order {
		head, tail, spaced := strings.Cut(key, " ")
		if !spaced {
			tail = key
		}
		switch method, _, ok := splitRoute(target); {
		case kind == "route" && ok && method == "" && tail == target,
			kind == "command" && !strings.Contains(target, " ") && head == target:
			return k.text[key]
		}
	}
	return ""
}

// backs reports whether value appears in region as whole words, as the
// skill's check finds a backed value (vision.py's contains and norm). Both
// are compared ignoring case, runs of whitespace, digit-group commas
// ("100,000" is 100000) and the kind of minus sign, with a word hyphenated
// across a line end joined; "64" is not found in "640" or "0.64". An
// ellipsis (…) elides: the pieces around it must appear in that order.
// Unlike the skill, the check applies no Unicode compatibility
// normalization (NFKC) or full case folding, which Go's standard library
// lacks, so it can only be stricter.
func backs(region, value string) bool {
	text, at := normText(region), 0
	for _, piece := range strings.Split(value, "…") {
		want := normText(piece)
		if want == "" {
			continue
		}
		if at = findWords(text, want, at); at < 0 {
			return false
		}
	}
	return true
}

// lineEndHyphen is a word hyphenated across a line end.
var lineEndHyphen = regexp.MustCompile(`-[ \t]*\n[\s\p{Z}\x{85}]*`)

// normText is text as backs compares it.
func normText(s string) string {
	s = strings.NewReplacer("\u2212", "-", "\f", "\n").Replace(s)
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		// A comma between a digit and exactly three more digits groups them.
		if r == ',' && i > 0 && unicode.IsDigit(runes[i-1]) && i+3 < len(runes) &&
			unicode.IsDigit(runes[i+1]) && unicode.IsDigit(runes[i+2]) && unicode.IsDigit(runes[i+3]) &&
			(i+4 == len(runes) || !unicode.IsDigit(runes[i+4])) {
			continue
		}
		b.WriteRune(r)
	}
	s = lineEndHyphen.ReplaceAllString(b.String(), "-")
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// findWords returns where the first whole-word want in text at or after
// from ends, or -1. A want that starts with a letter or digit must not
// follow a word character or a dot, and one that ends with one must not run
// on into a word character.
func findWords(text, want string, from int) int {
	first, _ := utf8.DecodeRuneInString(want)
	last, _ := utf8.DecodeLastRuneInString(want)
	for from <= len(text) {
		i := strings.Index(text[from:], want)
		if i < 0 {
			return -1
		}
		start, end := from+i, from+i+len(want)
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if (!alnum(first) || start == 0 || !wordRune(before) && before != '.') && (!alnum(last) || end == len(text) || !wordRune(after)) {
			return end
		}
		_, size := utf8.DecodeRuneInString(text[start:])
		from = start + size
	}
	return -1
}

func alnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }

func wordRune(r rune) bool { return alnum(r) || r == '_' }
