package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// changelogPath is the written history of merges to master
// (system-page.spec.md, "Changelog format").
const changelogPath = "CHANGELOG.md"

// HistoryEntry is one changelog entry as written: a merge to master, or a
// run of direct-to-master commits. The page's History tab reads the
// changelog itself; check validates it.
type HistoryEntry struct {
	Date    string
	Title   string
	Branch  string
	Merge   string
	Commits *CommitRange
	Touches []string
	Sources []string
	Text    []string
}

// CommitRange names the first and last commit of direct-to-master work,
// both included.
type CommitRange struct {
	From string
	To   string
}

// changelogFile is CHANGELOG.md parsed: its entries, and what is wrong with
// the file outside any entry.
type changelogFile struct {
	entries  []changelogEntry
	problems []problem
}

// changelogEntry is a parsed entry with what the check needs beyond the
// facts: its heading as written, the metadata fields it gives, and what is
// malformed in it.
type changelogEntry struct {
	HistoryEntry
	heading  string
	seen     map[string]bool
	problems []problem
}

// item names the entry in a check problem.
func (e *changelogEntry) item() string {
	return fmt.Sprintf("entry %q", e.heading)
}

func (e *changelogEntry) add(message, fix string) {
	e.problems = append(e.problems, problem{File: changelogPath, Item: e.item(), Message: message, Fix: fix})
}

// metadataFields are the metadata lines an entry may have, in the order
// the README lists them.
var metadataFields = []string{"Branch", "Merge", "Commits", "Touches", "Sources"}

// changelogFix is how to write a missing changelog.
const changelogFix = "create it in the format in docs/architecture/explorer/README.md"

// readChangelog reads and parses CHANGELOG.md. A missing file is an error
// that says how to create it and matches fs.ErrNotExist.
func readChangelog(root string) (changelogFile, error) {
	data, err := os.ReadFile(filepath.Join(root, changelogPath))
	if errors.Is(err, fs.ErrNotExist) {
		return changelogFile{}, fmt.Errorf("read %s: %w. Fix: %s", changelogPath, err, changelogFix)
	}
	if err != nil {
		return changelogFile{}, fmt.Errorf("read %s: %w", changelogPath, err)
	}
	return parseChangelog(string(data)), nil
}

// parseChangelog reads every entry of a changelog: a level-two heading
// "YYYY-MM-DD — Title" outside a code fence, the metadata block (the first
// block after the heading), then paragraphs. Text before the first entry is
// a preamble. It never fails; each entry records what is malformed in it,
// and the changelog what is wrong outside any entry.
func parseChangelog(text string) changelogFile {
	var log changelogFile
	var current *changelogEntry
	var paragraph []string
	const (
		afterHeading = iota
		inMetadata
		inText
	)
	state := afterHeading
	flush := func() {
		if current != nil && len(paragraph) > 0 {
			current.Text = append(current.Text, strings.Join(paragraph, " "))
		}
		paragraph = nil
	}
	finish := func() {
		flush()
		if current != nil {
			validateEntry(current)
			log.entries = append(log.entries, *current)
		}
	}
	var open codeFence
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if open.size > 0 {
			if f, info, ok := fenceLine(line); ok && info == "" && f.char == open.char && f.size >= open.size {
				open = codeFence{}
			}
			if current != nil {
				paragraph = append(paragraph, trimmed)
			}
			continue
		}
		if heading, ok := strings.CutPrefix(line, "## "); ok {
			finish()
			current = &changelogEntry{heading: strings.TrimSpace(heading), seen: map[string]bool{}}
			current.Touches, current.Sources, current.Text = []string{}, []string{}, []string{}
			state = afterHeading
			continue
		}
		if f, _, ok := fenceLine(line); ok {
			open = f
		}
		if current == nil {
			continue
		}
		switch {
		case trimmed == "" && state == inMetadata:
			state = inText
		case trimmed == "":
			flush()
		case state == inText:
			paragraph = append(paragraph, trimmed)
		default:
			state = inMetadata
			metadataLine(current, trimmed)
		}
	}
	// An unclosed fence reads every later heading as text, so the entries
	// below it would vanish silently.
	if open.size > 0 {
		const fix = "close the fence with a line of at least as many of the same fence characters"
		if current != nil {
			current.add("a code fence opened in the entry is never closed, so any entry below it is read as its text", fix)
		} else {
			log.problems = append(log.problems, problem{File: changelogPath, Item: "preamble",
				Message: "a code fence opened before the first entry is never closed, so no entry after it is read", Fix: fix})
		}
	}
	finish()
	return log
}

// metadataLine records one line of an entry's metadata block.
func metadataLine(e *changelogEntry, line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || !slices.Contains(metadataFields, key) {
		e.add(fmt.Sprintf("metadata line %q is not a Branch, Merge, Commits, Touches, or Sources line", line),
			"put a blank line between the metadata and the text, or correct the field name")
		return
	}
	if e.seen[key] {
		e.add(key+" is given twice", "give each metadata field once")
		return
	}
	e.seen[key] = true
	value = strings.TrimSpace(value)
	switch key {
	case "Branch":
		e.Branch = value
	case "Merge":
		e.Merge = value
	case "Commits":
		from, to, ok := strings.Cut(value, "..")
		from, to = strings.TrimSpace(from), strings.TrimSpace(to)
		if !ok || !isSHA(from) || !isSHA(to) {
			e.add(fmt.Sprintf("Commits %q is not <from>..<to>", value),
				"write the first and last commit of the work as <from>..<to>, each a SHA of at least 7 lowercase hex digits")
		}
		e.Commits = &CommitRange{From: from, To: to}
	case "Touches":
		e.Touches = splitList(e, key, value)
	case "Sources":
		e.Sources = splitList(e, key, value)
	}
}

// validateEntry records what is missing or malformed in a finished entry,
// beyond its metadata lines.
func validateEntry(e *changelogEntry) {
	date, title, ok := strings.Cut(e.heading, " —")
	if !ok {
		e.Title = e.heading
		e.add("the heading is not YYYY-MM-DD — Title", "write it as ## YYYY-MM-DD — Title, with an em dash between the date and the title")
	} else {
		e.Date, e.Title = strings.TrimSpace(date), strings.TrimSpace(title)
		if !realDate(e.Date) {
			e.add(fmt.Sprintf("date %q is not a real calendar date", e.Date), "write the date the change merged as YYYY-MM-DD")
		}
		if e.Title == "" {
			e.add("missing title", "add a title after the em dash")
		}
	}
	if e.Branch == "" {
		e.add("missing Branch", "add a Branch: line naming the merged branch")
	}
	if e.Merge != "" && !isSHA(e.Merge) {
		e.add(fmt.Sprintf("Merge %q is not a commit SHA", e.Merge), "use the merge commit's SHA, full or at least 7 lowercase hex digits")
	}
	if e.seen["Merge"] && e.seen["Commits"] {
		e.add("the entry has both Merge and Commits", "give Merge for a merged branch or Commits for direct-to-master work, not both")
	}
	if len(e.Touches) == 0 {
		e.add("missing Touches", "add a Touches: line listing the package paths the change touched")
	}
	for _, p := range e.Touches {
		if !cleanRelative(p) {
			e.add(fmt.Sprintf("Touches item %q is not a clean relative path", p), "write repository paths such as internal/agent")
		}
	}
	if len(e.Sources) == 0 {
		e.add("missing Sources", "add a Sources: line with at least one reference or #n number")
	}
	for _, source := range e.Sources {
		if strings.HasPrefix(source, "#") && !issueNumber.MatchString(source) {
			e.add(fmt.Sprintf("malformed ticket or pull-request number %q", source), "write # and the number, such as #179")
		}
	}
	if len(e.Text) == 0 {
		e.add("missing text", "add a paragraph after a blank line saying what changed and why")
	}
}

// realDate reports whether date is a real calendar date written YYYY-MM-DD.
func realDate(date string) bool {
	t, err := time.Parse(time.DateOnly, date)
	return err == nil && t.Format(time.DateOnly) == date
}

// isSHA reports whether s looks like a commit SHA: 7 to 40 lowercase hex
// digits. Only such values reach git.
func isSHA(s string) bool {
	return shaPattern.MatchString(s)
}

var shaPattern = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

// issueNumber is a ticket or pull-request source: # and a number. It is
// checked for form only, since GitHub cannot be asked offline.
var issueNumber = regexp.MustCompile(`^#[1-9][0-9]*$`)

// splitList splits a comma-separated metadata value into its trimmed
// items, recording an empty one.
func splitList(e *changelogEntry, key, value string) []string {
	items := []string{}
	empty := false
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		} else {
			empty = true
		}
	}
	if empty && value != "" {
		e.add(key+" has an empty item", "separate items with single commas and no trailing comma")
	}
	return items
}

// validateHistory checks the parsed changelog and reports each problem
// against the entry it is in.
func (c *checker) validateHistory(log changelogFile) {
	c.problems = append(c.problems, log.problems...)
	repo := c.askGit(log.entries)
	var above *changelogEntry // the nearest entry above with a real date
	for i := range log.entries {
		e := &log.entries[i]
		item := e.item()
		c.problems = append(c.problems, e.problems...)
		for _, source := range e.Sources {
			if !strings.HasPrefix(source, "#") {
				c.source(item, source)
			}
		}
		c.verifyCommits(repo, item, e)
		if !realDate(e.Date) {
			continue
		}
		// Entries on the same day may stand in either order.
		if above != nil && e.Date > above.Date {
			c.add(changelogPath, item, fmt.Sprintf("the entry is newer than %q above it", above.heading),
				"keep entries newest first: move it above every older entry")
		}
		above = e
	}
}

// source reports a changelog source that does not resolve. Sources are
// references in the system map's forms, as the vision skill's History tab
// reads them; a part: source names a part of the map.
func (c *checker) source(item, ref string) {
	why := c.resolve(ref)
	if why == "" {
		return
	}
	kind, _, _ := strings.Cut(ref, ":")
	fix, ok := refFixes[kind]
	if !ok {
		fix = "use one of " + refKinds()
	}
	c.add(changelogPath, item, fmt.Sprintf("dead reference %q: %s", ref, why), fix)
}

// commitSHAs are the well-formed commits an entry names: its merge commit
// and the ends of its commit range. Only these reach git.
func (e *changelogEntry) commitSHAs() []string {
	var shas []string
	if isSHA(e.Merge) {
		shas = append(shas, e.Merge)
	}
	if r := e.Commits; r != nil && isSHA(r.From) && isSHA(r.To) {
		shas = append(shas, r.From, r.To)
	}
	return shas
}

// askGit asks git, in one process, which of the changelog's commits it
// knows. When git cannot read the repository, that is reported once and it
// returns nil, so no entry's commits are verified; it also returns nil, and
// runs no git, when the changelog names no commit.
func (c *checker) askGit(entries []changelogEntry) *gitRepo {
	var shas []string
	for i := range entries {
		shas = append(shas, entries[i].commitSHAs()...)
	}
	if len(shas) == 0 {
		return nil
	}
	repo := &gitRepo{root: c.root}
	known, err := repo.knownCommits(shas)
	if err != nil {
		c.add(changelogPath, "commits", "git cannot verify the changelog's commits: "+err.Error(),
			"run the check from a git checkout, with git on PATH and the full history (CI checks out with fetch-depth 0)")
		return nil
	}
	repo.known = known
	return repo
}

// verifyCommits checks an entry's merge commit or commit range against what
// git knows (askGit); a nil repo verifies nothing.
func (c *checker) verifyCommits(repo *gitRepo, item string, e *changelogEntry) {
	if repo == nil {
		return
	}
	const fix = "use a commit SHA from this repository's history, long enough to be unambiguous; a shallow clone needs its full history"
	r := e.Commits
	if r == nil || !isSHA(r.From) || !isSHA(r.To) {
		if isSHA(e.Merge) && !repo.known[e.Merge] {
			c.add(changelogPath, item, "git does not know the merge commit "+e.Merge, fix)
		}
		return
	}
	known := true
	for _, sha := range []string{r.From, r.To} {
		if !repo.known[sha] {
			c.add(changelogPath, item, fmt.Sprintf("git does not know commit %s in Commits %s..%s", sha, r.From, r.To), fix)
			known = false
		}
	}
	if !known {
		return
	}
	ancestor, err := repo.isAncestor(r.From, r.To)
	switch {
	case err != nil:
		c.add(changelogPath, item, fmt.Sprintf("git cannot compare %s with %s: %v", r.From, r.To, err), fix)
	case !ancestor:
		c.add(changelogPath, item, fmt.Sprintf("commit %s is not %s or an ancestor of it", r.From, r.To),
			"write the range oldest first as <first>..<last>, both on one line of history")
	}
}

// gitRepo asks git about the repository at root.
type gitRepo struct {
	root  string
	known map[string]bool // the changelog's commits git knows (knownCommits)
}

// knownCommits asks one git process which of shas each name exactly one
// commit. It fails when git is missing or cannot read the repository.
func (g *gitRepo) knownCommits(shas []string) (map[string]bool, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git is not on PATH: %w", err)
	}
	var names strings.Builder
	for _, sha := range shas {
		names.WriteString(sha + "^{commit}\n")
	}
	cmd := exec.Command("git", "-C", g.root, "cat-file", "--batch-check=%(objecttype)")
	cmd.Stdin = strings.NewReader(names.String())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s (%v)", strings.TrimSpace(stderr.String()), err)
	}
	// One answer per name, in order: "commit", or the name followed by
	// "missing" or "ambiguous".
	answers := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(answers) != len(shas) {
		return nil, fmt.Errorf("git cat-file answered %d of %d commits", len(answers), len(shas))
	}
	known := make(map[string]bool, len(shas))
	for i, sha := range shas {
		known[sha] = answers[i] == "commit"
	}
	return known, nil
}

// isAncestor reports whether from is to or an ancestor of it.
func (g *gitRepo) isAncestor(from, to string) (bool, error) {
	out, err := exec.Command("git", "-C", g.root, "merge-base", "--is-ancestor", from, to).CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s (%v)", strings.TrimSpace(string(out)), err)
	}
	return true, nil
}
