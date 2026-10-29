package main

import (
	"bufio"
	"fmt"
	"go/doc"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Facts is the generated half of the explorer data. It is sorted and holds no
// timestamps or line numbers, so a fresh extraction is byte-identical on every
// platform.
type Facts struct {
	Format    int        `json:"format"`
	Kind      string     `json:"kind"`
	Packages  []Package  `json:"packages"`
	Imports   []Import   `json:"imports"`
	Commands  []Command  `json:"commands"`
	Routes    []Route    `json:"routes"`
	Databases []Database `json:"databases"`
	Glossary  []Term     `json:"glossary"`
	ADRs      []ADR      `json:"adrs"`
}

// Package is one directory under cmd/ or internal/ with a non-test Go file.
type Package struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	Command     bool   `json:"command"`
	Doc         string `json:"doc"`
	TestSupport bool   `json:"testSupport"`
}

// Import is an in-module import edge between two packages.
type Import struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Source locates a declaration by file and function, never by line.
type Source struct {
	File     string `json:"file"`
	Function string `json:"function"`
}

// Command is one CLI word path of a binary.
type Command struct {
	Binary  string   `json:"binary"`
	Words   []string `json:"words"`
	Package string   `json:"package"`
	Source  Source   `json:"source"`
}

// Route is one HTTP route registered on a ServeMux.
type Route struct {
	Method  string `json:"method,omitempty"`
	Pattern string `json:"pattern"`
	Package string `json:"package"`
	Guard   string `json:"guard,omitempty"`
	Source  Source `json:"source"`
}

// Database is one SQLite database and the schema its migrations create.
type Database struct {
	ID      string  `json:"id"`
	Package string  `json:"package"`
	Tables  []Table `json:"tables"`
}

// Table is one table of a database.
type Table struct {
	Name        string       `json:"name"`
	Columns     []Column     `json:"columns"`
	ForeignKeys []ForeignKey `json:"foreignKeys"`
}

// Column is one table column. PrimaryKey is the column's 1-based position in
// the primary key, or 0.
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	NotNull    bool   `json:"notNull"`
	PrimaryKey int    `json:"primaryKey"`
}

// ForeignKey links columns of a table to columns of another table.
type ForeignKey struct {
	Columns    []string `json:"columns"`
	Table      string   `json:"table"`
	References []string `json:"references"`
}

// Term is one glossary entry.
type Term struct {
	Term       string   `json:"term"`
	Definition string   `json:"definition"`
	Avoid      []string `json:"avoid"`
}

// ADR is one architecture decision record.
type ADR struct {
	Number string `json:"number"`
	Title  string `json:"title"`
	Path   string `json:"path"`
}

// writeFacts extracts the facts from the repository at root and writes them.
func writeFacts(root string) error {
	data, err := renderFacts(root)
	if err != nil {
		return err
	}
	file := filepath.Join(root, filepath.FromSlash(factsPath))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, data, 0o644)
}

// renderFacts extracts the facts and renders them as the facts data file.
func renderFacts(root string) ([]byte, error) {
	facts, err := extractFacts(root)
	if err != nil {
		return nil, err
	}
	return wrap(registerOpen, facts)
}

func extractFacts(root string) (Facts, error) {
	facts := Facts{
		Format:    formatVersion,
		Kind:      "facts",
		Packages:  []Package{},
		Imports:   []Import{},
		Commands:  []Command{},
		Routes:    []Route{},
		Databases: []Database{},
		Glossary:  []Term{},
		ADRs:      []ADR{},
	}
	module, err := modulePath(root)
	if err != nil {
		return facts, err
	}
	dirs, err := packageDirs(root)
	if err != nil {
		return facts, err
	}
	isPackage := map[string]bool{}
	for _, dir := range dirs {
		isPackage[dir] = true
	}
	for _, dir := range dirs {
		pkg, err := readPackage(root, dir)
		if err != nil {
			return facts, err
		}
		facts.Packages = append(facts.Packages, pkg.Package)
		facts.Imports = append(facts.Imports, moduleImports(module, pkg, isPackage)...)
	}
	if facts.Glossary, err = readGlossary(root); err != nil {
		return facts, err
	}
	if facts.ADRs, err = readADRs(root); err != nil {
		return facts, err
	}
	return facts, nil
}

// readADRs lists the numbered Markdown files in docs/adr with their first
// heading as title, sorted by number.
func readADRs(root string) ([]ADR, error) {
	entries, err := os.ReadDir(filepath.Join(root, "docs", "adr"))
	if err != nil {
		return nil, fmt.Errorf("read ADRs: %w", err)
	}
	adrs := []ADR{}
	for _, entry := range entries { // ReadDir sorts by name, so by number
		name := entry.Name()
		if entry.IsDir() || !isADRName(name) {
			continue
		}
		file := "docs/adr/" + name
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			return nil, err
		}
		title := ""
		for _, line := range strings.Split(string(data), "\n") {
			if heading, ok := strings.CutPrefix(line, "# "); ok {
				title = strings.TrimSpace(heading)
				break
			}
		}
		if title == "" {
			return nil, fmt.Errorf("%s has no top-level heading", file)
		}
		adrs = append(adrs, ADR{Number: name[:4], Title: title, Path: file})
	}
	return adrs, nil
}

// isADRName reports whether name looks like "0007-some-title.md".
func isADRName(name string) bool {
	if len(name) < 6 || name[4] != '-' || !strings.HasSuffix(name, ".md") {
		return false
	}
	for _, r := range name[:4] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// readGlossary parses the bold "**Term**:" entries of GLOSSARY.md: the
// definition is the rest of the paragraph and an "_Avoid_:" line lists the
// avoided synonyms. Entries are sorted by term.
func readGlossary(root string) ([]Term, error) {
	data, err := os.ReadFile(filepath.Join(root, "GLOSSARY.md"))
	if err != nil {
		return nil, fmt.Errorf("read glossary: %w", err)
	}
	terms := []Term{}
	var current *Term
	var definition []string
	flush := func() {
		if current != nil {
			current.Definition = strings.Join(definition, " ")
			terms = append(terms, *current)
		}
		current, definition = nil, nil
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case line == "":
			flush()
		case glossaryTerm(line) != "":
			flush()
			current = &Term{Term: glossaryTerm(line), Avoid: []string{}}
		case current == nil:
		case strings.HasPrefix(line, "_Avoid_:"):
			for _, word := range strings.Split(strings.TrimPrefix(line, "_Avoid_:"), ",") {
				if word = strings.TrimSpace(word); word != "" {
					current.Avoid = append(current.Avoid, word)
				}
			}
		default:
			definition = append(definition, line)
		}
	}
	flush()
	sort.Slice(terms, func(i, j int) bool { return terms[i].Term < terms[j].Term })
	return terms, nil
}

// glossaryTerm returns the term of a "**Term**:" or "**Term:**" heading line.
func glossaryTerm(line string) string {
	if !strings.HasPrefix(line, "**") {
		return ""
	}
	if term, ok := strings.CutSuffix(line, "**:"); ok {
		return strings.TrimSpace(strings.TrimPrefix(term, "**"))
	}
	if term, ok := strings.CutSuffix(line, ":**"); ok {
		return strings.TrimSpace(strings.TrimPrefix(term, "**"))
	}
	return ""
}

// moduleImports returns pkg's distinct imports of other extracted packages,
// sorted by target.
func moduleImports(module string, pkg packageInfo, isPackage map[string]bool) []Import {
	targets := map[string]bool{}
	for _, imp := range pkg.imports {
		rel, ok := strings.CutPrefix(imp, module+"/")
		if ok && isPackage[rel] && rel != pkg.Path {
			targets[rel] = true
		}
	}
	edges := make([]Import, 0, len(targets))
	for to := range targets {
		edges = append(edges, Import{From: pkg.Path, To: to})
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].To < edges[j].To })
	return edges
}

// modulePath reads the module path from root's go.mod.
func modulePath(root string) (string, error) {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read go.mod (run from the repository root): %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("go.mod has no module line")
}

// packageDirs lists every directory under cmd/ and internal/ with at least one
// non-test Go file, excluding testdata and node_modules, as sorted
// slash-separated paths relative to root.
func packageDirs(root string) ([]string, error) {
	seen := map[string]bool{}
	for _, top := range []string{"cmd", "internal"} {
		base := filepath.Join(root, top)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isSourceFile(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(root, filepath.Dir(file))
			if err != nil {
				return err
			}
			seen[filepath.ToSlash(rel)] = true
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	dirs := make([]string, 0, len(seen))
	for dir := range seen {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs, nil
}

func isSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
}

type packageInfo struct {
	Package
	imports []string // import paths from every non-test file
}

// readPackage parses every non-test Go file of dir, ignoring build
// constraints, so every platform reads the same package.
func readPackage(root, dir string) (packageInfo, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if err != nil {
		return packageInfo{}, err
	}
	info := packageInfo{Package: Package{Path: dir}}
	fset := token.NewFileSet()
	for _, entry := range entries { // ReadDir sorts by name
		if entry.IsDir() || !isSourceFile(entry.Name()) {
			continue
		}
		name := path.Join(dir, entry.Name())
		src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return packageInfo{}, err
		}
		file, err := parser.ParseFile(fset, name, src, parser.ImportsOnly|parser.ParseComments)
		if err != nil {
			return packageInfo{}, fmt.Errorf("parse %s: %w", name, err)
		}
		if info.Name == "" {
			info.Name = file.Name.Name
		}
		if info.Doc == "" && file.Doc != nil {
			info.Doc = new(doc.Package).Synopsis(file.Doc.Text())
		}
		for _, spec := range file.Imports {
			info.imports = append(info.imports, strings.Trim(spec.Path.Value, `"`))
		}
	}
	info.Command = info.Name == "main"
	info.TestSupport = strings.HasSuffix(info.Name, "test")
	return info, nil
}
