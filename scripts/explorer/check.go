package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// problem is one drift or format failure: the file it is in, the item that
// is wrong, what is wrong, and how to fix it.
type problem struct {
	File    string
	Item    string
	Message string
	Fix     string
}

func (p problem) String() string {
	return fmt.Sprintf("%s: %s: %s. Fix: %s.", p.File, p.Item, p.Message, p.Fix)
}

// checker collects problems while checking the facts file, the system map
// and the changelog against a fresh extraction of the code.
type checker struct {
	root     string
	facts    Facts
	problems []problem
	packages map[string]bool // package paths from the facts
	imports  map[Import]bool // import edges from the facts

	declCache    map[string]*goFile         // Go file -> its declarations
	scriptCache  map[string]*scriptFile     // JavaScript or TypeScript file -> its declarations
	headingCache map[string]map[string]bool // Markdown file -> anchors
}

func newChecker(root string, facts Facts) *checker {
	c := &checker{root: root, facts: facts, packages: map[string]bool{}, imports: map[Import]bool{}}
	for _, pkg := range facts.Packages {
		c.packages[pkg.Path] = true
	}
	for _, imp := range facts.Imports {
		c.imports[imp] = true
	}
	return c
}

func (c *checker) add(file, item, message, fix string) {
	c.problems = append(c.problems, problem{File: file, Item: item, Message: message, Fix: fix})
}

func (c *checker) path(rel string) string {
	return filepath.Join(c.root, filepath.FromSlash(rel))
}

// runCheck prints every problem and returns the exit code.
func runCheck(root string, databases []databaseSource, stdout, stderr io.Writer) int {
	problems, err := checkRepo(root, databases)
	if err != nil {
		fmt.Fprintf(stderr, "explorer check: %v\n", err)
		return 2
	}
	for _, p := range problems {
		fmt.Fprintln(stdout, p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(stdout, "explorer check: %d problem(s)\n", len(problems))
		return 1
	}
	fmt.Fprintln(stdout, "explorer check: the facts, the system map and the changelog match the code")
	return 0
}

// checkRepo re-extracts the facts and compares them with the facts file,
// then checks the system map and the changelog against them. It returns
// every problem found; an error means the check itself could not run.
func checkRepo(root string, databases []databaseSource) ([]problem, error) {
	log, logData, err := readChangelog(root)
	missingLog := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missingLog {
		return nil, err
	}
	facts, err := extractFacts(root, databases)
	if err != nil {
		return nil, err
	}
	fresh, err := wrap(registerOpen, facts)
	if err != nil {
		return nil, err
	}
	c := newChecker(root, facts)
	current, factsErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(factsPath)))
	switch {
	case factsErr == nil && bytes.Equal(current, fresh):
	case factsErr == nil && hasCRLF(current) && bytes.Equal(bytes.ReplaceAll(current, []byte("\r\n"), []byte("\n")), fresh):
		c.lineEndings(factsPath, current)
	default:
		c.add(factsPath, "facts", "the facts differ from a fresh extraction of the code",
			"run "+extractCommand+" and commit the result")
	}
	// The page embeds the facts file and the changelog as they were just
	// read, and the design proposals.
	var inputs pageInputs
	if factsErr == nil {
		inputs.facts = &namedFile{Name: factsPath, Data: current}
	}
	if !missingLog {
		inputs.changelog = &namedFile{Name: changelogPath, Data: logData}
	}
	inputs.proposals, inputs.proposalsErr = readProposals(root)
	vm := c.readVisionMap()
	// part: references, in the map and in the changelog's sources, name
	// the map's parts; without a readable map there are none.
	var parts map[string]bool
	if vm != nil {
		parts = vm.parts
	}
	c.validateVisionMap(vm, inputs)
	if missingLog {
		c.add(changelogPath, "file", "the file does not exist", changelogFix)
	} else {
		c.validateHistory(log, parts)
	}
	return c.problems, nil
}
