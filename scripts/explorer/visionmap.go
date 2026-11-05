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

// The system map (system-page.decisions.md, 2026-11-04 and 2026-11-05).
// Evie's System view is a page the vision skill builds from .vision/map.json
// and that the repository commits beside the facts: a Map of Evie's flows,
// and Architecture and History tabs drawn from the map's parts, edges and
// sequences and from CHANGELOG.md. CI cannot run the skill, which lives
// outside the repository, so check holds the map to the code itself: every
// package has a part, every reference in the map resolves against the code
// and the facts, the map's import edges and sequence steps follow the facts'
// imports, and the committed page carries the current map's hash and the
// hash of the other files it embeds (the facts, the changelog and the design
// proposals). Building the page runs the skill's own check first, so a page
// built from the current map also means the map passed that check. Check
// reads only the forms Evie's map uses, and refuses the skill's others
// rather than passing them unread.
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

// visionRebuildInputs is visionRebuild for a page older than the other
// files it embeds.
const visionRebuildInputs = "rebuild with the vision skill: " + visionBuild + ", and commit the page with the facts, the changelog and the proposals"

// visionMap is the part of a map the check reads: its settings, its flows,
// its code entries, and its parts, edges and sequences. A top-level flow and
// library definitions (refs) are read only to refuse them. Other fields
// belong to the page and are not read.
type visionMap struct {
	Meta struct {
		Facts      string `json:"facts"`
		Page       string `json:"page"`
		Parts      string `json:"parts"`
		Changelog  string `json:"changelog"`
		Repository string `json:"repository"`
	} `json:"meta"`
	Flows     []visionFlow               `json:"flows"`
	Flow      json.RawMessage            `json:"flow"`
	Code      map[string]visionCode      `json:"code"`
	Refs      map[string]json.RawMessage `json:"refs"`
	Parts     []visionPart               `json:"parts"`
	Edges     []visionEdge               `json:"edges"`
	Sequences []visionSequence           `json:"sequences"`
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

// given reports whether a field read only to refuse it holds a value: it
// is there and not null.
func given(raw json.RawMessage) bool {
	return raw != nil && string(raw) != "null"
}

// visionStatement is a sentence of the Architecture tab and the references
// it rests on.
type visionStatement struct {
	Text string   `json:"text"`
	Refs []string `json:"refs"`
}

// visionPart is one part the system is built from: a package, a container,
// or a person or system around Evie.
type visionPart struct {
	ID         string            `json:"id"`
	Package    string            `json:"package"`
	Paths      []string          `json:"paths"`
	Refs       []string          `json:"refs"`
	Summary    *visionStatement  `json:"summary"`
	Does       []visionStatement `json:"does"`
	Owns       []visionStatement `json:"owns"`
	Interfaces []visionStatement `json:"interfaces"`
	Gotchas    []visionStatement `json:"gotchas"`
	Facts      [][]any           `json:"facts"`
	Terms      []string          `json:"terms"`
	Contains   []string          `json:"contains"`
	Links      []string          `json:"links"`
	Diagrams   []visionDiagram   `json:"diagrams"`
}

// visionDiagram is a drawing of a part. Which fields it has depends on its
// kind: graph, sequence, state, or er. Every kind has a summary; a sequence
// diagram has a sequence's other fields too.
type visionDiagram struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	visionMessages
	// graph
	Nodes []struct {
		ID   string `json:"id"`
		Node string `json:"node"`
	} `json:"nodes"`
	Edges []struct {
		Refs []string `json:"refs"`
	} `json:"edges"`
	// state
	States []struct {
		ID   string   `json:"id"`
		Refs []string `json:"refs"`
	} `json:"states"`
	Transitions []struct {
		Refs []string `json:"refs"`
	} `json:"transitions"`
	// er
	Database string   `json:"database"`
	Tables   []string `json:"tables"`
}

// visionEdge is a connection between two parts that the facts' imports do
// not draw, or a label on one they do.
type visionEdge struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Via  string   `json:"via"`
	Refs []string `json:"refs"`
}

// visionSequence is a runtime flow drawn as a sequence diagram.
type visionSequence struct {
	ID string `json:"id"`
	visionMessages
}

// visionMessages is what a sequence and a part's sequence diagram share:
// a summary, what triggers it, its participants, and the messages between
// them.
type visionMessages struct {
	Summary      *visionStatement `json:"summary"`
	Trigger      *visionStatement `json:"trigger"`
	Participants []string         `json:"participants"`
	Steps        []visionMessage  `json:"steps"`
}

// visionMessage is one step of a sequence: a message between two
// participants.
type visionMessage struct {
	From string   `json:"from"`
	To   string   `json:"to"`
	Kind string   `json:"kind"`
	Via  string   `json:"via"`
	Refs []string `json:"refs"`
}

// loadedMap is the system map as read: its bytes, what check decodes of
// it, and its parts, by id and by the package each package part is.
type loadedMap struct {
	data        []byte
	m           visionMap
	parts       map[string]bool   // part ids, which part: references name
	partPackage map[string]string // part id -> the package it is
}

// readVisionMap reads and decodes the system map. A map that is missing,
// unreadable or malformed is reported once, and it returns nil.
func (c *checker) readVisionMap() *loadedMap {
	data, err := os.ReadFile(c.path(visionMapPath))
	if errors.Is(err, fs.ErrNotExist) {
		c.add(visionMapPath, "map", "the file does not exist, but it holds the system map, every package's part, and what "+visionPagePath+" is built from",
			"restore it (git log -- "+visionMapPath+" finds it), then "+visionRebuild)
		return nil
	}
	if err != nil {
		c.add(visionMapPath, "map", fmt.Sprintf("the map cannot be read: %v", err), "make the file readable")
		return nil
	}
	vm := &loadedMap{data: data, parts: map[string]bool{}, partPackage: map[string]string{}}
	if err := json.Unmarshal(data, &vm.m); err != nil {
		c.add(visionMapPath, "map", "malformed map: "+err.Error(), "make the map match "+visionFormat)
		return nil
	}
	for _, p := range vm.m.Parts {
		if p.ID == "" {
			continue
		}
		vm.parts[p.ID] = true
		if p.Package != "" {
			vm.partPackage[p.ID] = p.Package
		}
	}
	return vm
}

// validateVisionMap checks the system map, which holds every package's part
// and is what the page Evie embeds is built from, and the design proposals
// and the page beside it. Without a map (vm nil, reported by readVisionMap)
// it still checks the proposals and the page's inputs.
func (c *checker) validateVisionMap(vm *loadedMap, inputs pageInputs) {
	if vm != nil {
		c.visionMapContent(vm)
	}
	if inputs.proposalsErr != nil {
		c.add(visionProposalsDir, "folder", "the design proposals cannot be read: "+inputs.proposalsErr.Error(), "make the folder and its files readable")
	}
	for _, p := range inputs.proposals {
		c.validateProposal(p)
	}
	c.visionPageHash(vm, inputs)
}

// visionMapContent checks what the map says against the code and the
// facts: its settings, code entries, flows, parts, edges and sequences, and
// that every package has a part.
func (c *checker) visionMapContent(vm *loadedMap) {
	m := vm.m
	c.visionMeta(m)
	v := &visionChecker{checker: c, loadedMap: vm, facts: factTextsOf(c.facts)}
	if given(m.Flow) {
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
	for _, p := range m.Parts {
		v.part(p)
	}
	for _, e := range m.Edges {
		v.edge(e)
	}
	for _, q := range m.Sequences {
		v.sequence("sequence "+q.ID, q.visionMessages)
	}
	v.packageCoverage()
}

// visionMeta checks that the map's settings are the ones Evie's check,
// binary and changelog use.
func (c *checker) visionMeta(m visionMap) {
	setting := func(item, got, want, why string) {
		if got != want {
			c.add(visionMapPath, item, fmt.Sprintf("%s is %q, but %s", item, got, why),
				fmt.Sprintf("set %s to %q, then %s", item, want, visionRebuild))
		}
	}
	setting("meta.facts", m.Meta.Facts, factsPath, "the map's fact references resolve against "+factsPath)
	setting("meta.page", m.Meta.Page, visionPagePath, "Evie embeds and serves "+visionPagePath)
	setting("meta.parts", m.Meta.Parts, "complete", "check requires that every package has a part")
	setting("meta.changelog", m.Meta.Changelog, changelogPath, "the History tab draws the file it names, and check validates "+changelogPath)
	// History's commits, tickets and pull requests link into the repository
	// the module path names, as the address Go's build stamp carries.
	if module, err := modulePath(c.root); err == nil {
		setting("meta.repository", m.Meta.Repository, "https://"+module,
			"History's commit, ticket and pull-request links must open the module "+module)
	}
}

// visionPageHash checks that the page Evie embeds was built from this map
// and from the files it embeds beside it, whatever meta.page says. Without
// a map (vm nil) only the inputs can be compared.
func (c *checker) visionPageHash(vm *loadedMap, inputs pageInputs) {
	var want string
	if vm != nil {
		var err error
		if want, err = mapHash(vm.data); err != nil {
			c.add(visionMapPath, "map hash", "the map's hash cannot be computed: "+err.Error(), "make the map valid JSON that "+visionFormat+" describes")
			return
		}
	}
	html, ok := c.readPage()
	if !ok {
		return
	}
	switch got := pageHashMeta.FindSubmatch(html); {
	case got == nil:
		c.add(visionPagePath, "page", `the page carries no <meta name="vision-map-hash">, so it was not built by the vision skill`, visionRebuild)
		return
	case vm != nil && string(got[1]) != want:
		c.add(visionPagePath, "page", fmt.Sprintf("the page was built from an older map: it carries %s, and %s hashes to %s", got[1], visionMapPath, want), visionRebuild)
	}
	c.visionInputsHash(html, vm, inputs)
}

// readPage reads the built page, or reports why it cannot: there is none,
// a folder is in its place, or the file cannot be read.
func (c *checker) readPage() ([]byte, bool) {
	page := c.path(visionPagePath)
	html, err := os.ReadFile(page)
	if err == nil {
		return html, true
	}
	switch info, statErr := os.Stat(page); {
	case errors.Is(err, fs.ErrNotExist):
		c.add(visionPagePath, "page", "the built page does not exist", visionRebuild)
	case statErr == nil && info.IsDir():
		c.add(visionPagePath, "page", "a folder is where the built page belongs", "remove the folder, then "+visionRebuild)
	default:
		c.add(visionPagePath, "page", "the built page cannot be read: "+err.Error(), "make the file readable, or remove it and "+visionRebuild)
	}
	return nil, false
}

// visionInputsHash checks that the page was built from the facts file and
// the changelog and from the design proposals as they are now, by the
// inputs hash the skill writes beside the map's. It hashes the files Evie's
// settings name, the only ones a map that passes check names: a map that
// names others is reported at its settings. Without the proposals there is
// nothing to compare.
func (c *checker) visionInputsHash(html []byte, vm *loadedMap, inputs pageInputs) {
	if vm != nil && (vm.m.Meta.Facts != factsPath || vm.m.Meta.Changelog != changelogPath) || inputs.proposalsErr != nil {
		return
	}
	want := inputsHash(inputs.facts, inputs.changelog, inputs.proposals)
	switch got := pageInputsMeta.FindSubmatch(html); {
	case got == nil:
		c.add(visionPagePath, "page", `the page carries no <meta name="vision-inputs-hash">, so it was built before the vision skill wrote one`, visionRebuildInputs)
	case string(got[1]) != want:
		c.add(visionPagePath, "page", fmt.Sprintf("the page was built from an older facts file, changelog or design proposals: it carries inputs hash %s, and they now hash to %s", got[1], want), visionRebuildInputs)
	}
}

// pageHashMeta and pageInputsMeta find the map hash and the inputs hash in
// a built page, as the skill reads them.
var (
	pageHashMeta   = regexp.MustCompile(`<meta name="vision-map-hash" content="([^"]+)"`)
	pageInputsMeta = regexp.MustCompile(`<meta name="vision-inputs-hash" content="([^"]+)"`)
)

// visionChecker checks the references of one map.
type visionChecker struct {
	*checker
	*loadedMap
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
	fix, known := refFixes[kind]
	var why string
	switch {
	case !ok || target == "":
		why = "it is neither a code key of the map nor a kind:target reference"
		fix = "use a code key of the map, or one of " + refKinds()
	case !known:
		v.unsupported(item, kind+": references", "cite code, docs, facts or parts instead")
		return false
	default:
		why = v.resolve(ref, v.parts)
	}
	if why != "" {
		v.add(visionMapPath, item, fmt.Sprintf("dead reference %q: %s", ref, why), fix+", then "+visionRebuild)
		return false
	}
	return true
}

func (v *visionChecker) mapRefs(item string, refs []string) {
	for _, ref := range refs {
		v.mapRef(item, ref)
	}
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

// factChips checks fact chips: a chip's reference resolves, and its value
// is in what the reference points at.
func (v *visionChecker) factChips(item string, facts [][]any) {
	for _, fact := range facts {
		label, value, ref := chip(fact)
		if where := fmt.Sprintf("%s fact %q", item, label); ref != "" && v.mapRef(where, ref) {
			v.backed(where, value, ref)
		}
	}
}

func (v *visionChecker) step(s visionStep) {
	item := "step " + s.ID
	if s.Code != nil {
		if _, ok := v.m.Code[s.Code.Key]; !ok {
			v.add(visionMapPath, item+" code", fmt.Sprintf("code key %q is not in the map's code", s.Code.Key),
				"add it to code, or correct the key, then "+visionRebuild)
		}
	}
	v.mapRefs(item+" refs", s.Refs)
	v.factChips(item, s.Facts)
	for _, flag := range s.Flags {
		v.mapRefs(fmt.Sprintf("%s flag %q", item, clip(flag.Text, 40)), flag.Refs)
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

// statement checks the references of a statement, if there is one.
func (v *visionChecker) statement(item string, s *visionStatement) {
	if s != nil {
		v.mapRefs(item, s.Refs)
	}
}

func (v *visionChecker) statements(item string, list []visionStatement) {
	for i := range list {
		v.statement(fmt.Sprintf("%s %d", item, i+1), &list[i])
	}
}

// isPart reports an id that names no part of the map.
func (v *visionChecker) isPart(item, id string) {
	if !v.parts[id] {
		v.add(visionMapPath, item, fmt.Sprintf("%q is not a part", id), "name a part of the map, or remove it, then "+visionRebuild)
	}
}

// part checks one part: the package it is, the files it names, and every
// reference its statements, facts, terms, links and diagrams make.
func (v *visionChecker) part(p visionPart) {
	item := "part " + p.ID
	if p.Package != "" && !v.packages[p.Package] {
		v.add(visionMapPath, item, "the facts have no package "+p.Package,
			"remove the part, or correct its package to an existing package, then "+visionRebuild)
	}
	for _, path := range p.Paths {
		if _, why := v.stat(path); why != "" {
			v.add(visionMapPath, item+" paths", fmt.Sprintf("path %q: %s", path, why),
				"point it at an existing repository file or folder, or remove it, then "+visionRebuild)
		}
	}
	v.mapRefs(item+" refs", p.Refs)
	v.statement(item+" summary", p.Summary)
	v.statements(item+" does", p.Does)
	v.statements(item+" owns", p.Owns)
	v.statements(item+" interfaces", p.Interfaces)
	v.statements(item+" gotchas", p.Gotchas)
	v.factChips(item, p.Facts)
	for _, term := range p.Terms {
		v.mapRef(item+" terms", "term:"+term)
	}
	for _, id := range p.Contains {
		v.isPart(item+" contains", id)
	}
	v.mapRefs(item+" links", p.Links)
	for _, d := range p.Diagrams {
		v.diagram(item+" diagram "+d.ID, d)
	}
}

// diagram checks a part's diagram: what its nodes, states and steps cite,
// that the parts it names exist, and that an er diagram's tables are in the
// facts.
func (v *visionChecker) diagram(item string, d visionDiagram) {
	if d.Kind == "sequence" {
		v.sequence(item, d.visionMessages)
		return
	}
	v.statement(item+" summary", d.Summary)
	for _, n := range d.Nodes {
		if n.Node != "" {
			v.isPart(item+" node "+n.ID, n.Node)
		}
	}
	for i, e := range d.Edges {
		v.mapRefs(fmt.Sprintf("%s edge %d", item, i+1), e.Refs)
	}
	for _, s := range d.States {
		v.mapRefs(item+" state "+s.ID, s.Refs)
	}
	for i, t := range d.Transitions {
		v.mapRefs(fmt.Sprintf("%s transition %d", item, i+1), t.Refs)
	}
	if d.Kind != "er" {
		return
	}
	if !slices.ContainsFunc(v.checker.facts.Databases, func(db Database) bool { return db.ID == d.Database }) {
		v.add(visionMapPath, item, "there is no database "+d.Database+" in the facts", refFixes["table"]+", then "+visionRebuild)
		return
	}
	for _, table := range d.Tables {
		v.mapRef(item, "table:"+d.Database+"."+table)
	}
}

// edge checks an edge between two parts: both are parts, its references
// resolve, and an import edge is one of the facts' imports.
func (v *visionChecker) edge(e visionEdge) {
	item := fmt.Sprintf("edge %s -> %s (%s)", e.From, e.To, e.Via)
	v.isPart(item, e.From)
	v.isPart(item, e.To)
	v.mapRefs(item, e.Refs)
	if e.Via == "import" {
		v.importEdge(item, e.From, e.To)
	}
}

// importEdge checks that the package of part from imports the package of
// part to, as the skill's check does: an import joins two package parts. An
// end that is not a part at all is reported where the ends are checked.
func (v *visionChecker) importEdge(item, from, to string) {
	fromPkg, toPkg := v.partPackage[from], v.partPackage[to]
	for _, end := range []string{from, to} {
		if v.parts[end] && v.partPackage[end] == "" {
			v.add(visionMapPath, item, fmt.Sprintf("part %q has no package, but via import joins two package parts", end),
				"join the package parts whose import carries it, or use a via other than import with a cited source, then "+visionRebuild)
		}
	}
	if fromPkg == "" || toPkg == "" || v.imports[Import{From: fromPkg, To: toPkg}] {
		return
	}
	v.add(visionMapPath, item, fmt.Sprintf("%s does not import %s", fromPkg, toPkg),
		"remove it, reverse it, or use a via other than import with a cited source, then "+visionRebuild)
}

// sequence checks a sequence, or a part's sequence diagram: its statements
// resolve, its participants are parts, and every step joins two of its
// participants, cites the code that carries it and, via import, follows one
// of the facts' imports.
func (v *visionChecker) sequence(item string, q visionMessages) {
	v.statement(item+" summary", q.Summary)
	v.statement(item+" trigger", q.Trigger)
	for _, id := range q.Participants {
		v.isPart(item+" participants", id)
	}
	for i, s := range q.Steps {
		stepItem := fmt.Sprintf("%s step %d", item, i+1)
		v.stepEnd(stepItem, "from", s.From, q.Participants)
		v.stepEnd(stepItem, "to", s.To, q.Participants)
		v.mapRefs(stepItem, s.Refs)
		if !slices.ContainsFunc(s.Refs, v.citesCode) {
			v.add(visionMapPath, stepItem, "the step cites no code",
				"add a code: reference or a code key of the map naming the code that carries the step, then "+visionRebuild)
		}
		if s.Via == "import" {
			from, to := s.From, s.To
			if s.Kind == "return" {
				from, to = to, from
			}
			v.importEdge(stepItem, from, to)
		}
	}
}

// stepEnd reports a sequence step's end, from or to, that is not one of
// the sequence's participants. A participant that is not a part is reported
// with the participants.
func (v *visionChecker) stepEnd(item, end, id string, participants []string) {
	switch {
	case id == "":
		v.add(visionMapPath, item, "the step has no "+end, "name the participant it goes "+end+", then "+visionRebuild)
	case slices.Contains(participants, id):
	case v.parts[id]:
		v.add(visionMapPath, item, fmt.Sprintf("%s %q is not a participant", end, id),
			"add it to participants, or correct the step, then "+visionRebuild)
	default:
		v.add(visionMapPath, item, fmt.Sprintf("%s %q is neither a participant nor a part", end, id),
			"name a participant of the sequence, or correct the step, then "+visionRebuild)
	}
}

// citesCode reports whether a reference names code: a code key of the map,
// or a code: reference.
func (v *visionChecker) citesCode(ref string) bool {
	_, key := v.m.Code[ref]
	return key || strings.HasPrefix(ref, "code:")
}

// packageCoverage reports every package in the facts without a part
// (meta.parts "complete").
func (v *visionChecker) packageCoverage() {
	covered := map[string]bool{}
	for _, pkg := range v.partPackage {
		covered[pkg] = true
	}
	for _, pkg := range v.checker.facts.Packages {
		if !covered[pkg.Path] {
			v.add(visionMapPath, "package "+pkg.Path, fmt.Sprintf(`no part has "package": %q`, pkg.Path),
				fmt.Sprintf(`add a component part with "package": %q, a group and a cited summary, then %s`, pkg.Path, visionRebuild))
		}
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
	case given(e.Lines):
		v.unsupported(item, "a line range", "name the declaration with symbol instead")
	case given(e.Cell):
		v.unsupported(item, "a notebook cell", "name the declaration with symbol instead")
	case e.Symbol == "" && e.Src != nil:
		v.unsupported(item, "inline src", "cite code in the repository by file and symbol")
	case e.Symbol == "":
		v.add(visionMapPath, item, "the entry names no file and symbol", "give it the file and symbol of the declaration it shows, then "+visionRebuild)
	case e.File == "":
		v.add(visionMapPath, item, "the entry gives a symbol but no file", "add the file that declares it, then "+visionRebuild)
	default:
		if why := v.resolveCode(e.File + "#" + e.Symbol); why != "" {
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
		text, why := v.declaration(e.File, e.Symbol)
		return text, why == ""
	}
	kind, target, _ := strings.Cut(ref, ":")
	switch kind {
	case "code":
		file, symbol, hasSymbol := strings.Cut(target, "#")
		if hasSymbol {
			text, why := v.declaration(file, symbol)
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
	case "part":
		// As the skill reads it: a part's text is its id.
		return target, true
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
