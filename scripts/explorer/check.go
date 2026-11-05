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

// problem is one drift or format failure: the data file it is in, the item
// that is wrong, what is wrong, and how to fix it.
type problem struct {
	File    string
	Item    string
	Message string
	Fix     string
}

func (p problem) String() string {
	return fmt.Sprintf("%s: %s: %s. Fix: %s.", p.File, p.Item, p.Message, p.Fix)
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
	fmt.Fprintln(stdout, "explorer check: the explorer data matches the code")
	return 0
}

// checkRepo re-extracts the facts, compares them with the facts file, and
// validates every data file against them. It returns every problem found; an
// error means the check itself could not run.
func checkRepo(root string, databases []databaseSource) ([]problem, error) {
	log, err := readChangelog(root)
	if errors.Is(err, fs.ErrNotExist) {
		// Without the changelog there is no fresh extraction to compare.
		return []problem{{File: changelogPath, Item: "file", Message: "the file does not exist", Fix: changelogFix}}, nil
	}
	if err != nil {
		return nil, err
	}
	facts, err := factsWith(root, databases, log)
	if err != nil {
		return nil, err
	}
	fresh, err := wrap(registerOpen, facts)
	if err != nil {
		return nil, err
	}
	c := &checker{root: root, facts: facts}
	current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(factsPath)))
	switch {
	case err == nil && bytes.Equal(current, fresh):
	case err == nil && hasCRLF(current) && bytes.Equal(bytes.ReplaceAll(current, []byte("\r\n"), []byte("\n")), fresh):
		c.lineEndings(factsPath, current)
	default:
		c.add(factsPath, "facts", "the facts differ from a fresh extraction of the code",
			"run "+extractCommand+" and commit the result")
	}
	c.validate(c.loadData())
	c.validateHistory(log)
	c.validateVisionMap()
	return c.problems, nil
}
