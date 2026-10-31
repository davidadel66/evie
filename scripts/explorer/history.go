package main

import (
	"errors"
	"fmt"
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

// HistoryEntry is one changelog entry: a merge to master, or a run of
// direct-to-master commits.
type HistoryEntry struct {
	Date    string       `json:"date"`
	Title   string       `json:"title"`
	Branch  string       `json:"branch"`
	Merge   string       `json:"merge,omitempty"`
	Commits *CommitRange `json:"commits,omitempty"`
	Touches []string     `json:"touches"`
	Sources []string     `json:"sources"`
	Text    []string     `json:"text"`
}

// CommitRange names the first and last commit of direct-to-master work,
// both included.
type CommitRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// changelogEntry is a parsed entry with what the check needs beyond the
// facts: its heading as written and what is malformed in it.
type changelogEntry struct {
	HistoryEntry
	heading string
	issues  []issue
}

// issue is one thing wrong with an entry and how to fix it.
type issue struct {
	message, fix string
}

func (e *changelogEntry) add(message, fix string) {
	e.issues = append(e.issues, issue{message, fix})
}

// metadataFields are the metadata lines an entry may have, in the order
// the README lists them.
var metadataFields = []string{"Branch", "Merge", "Commits", "Touches", "Sources"}

// readHistory parses CHANGELOG.md into the history facts.
func readHistory(root string) ([]HistoryEntry, error) {
	entries, err := readChangelog(root)
	if err != nil {
		return nil, err
	}
	history := make([]HistoryEntry, 0, len(entries))
	for _, e := range entries {
		history = append(history, e.HistoryEntry)
	}
	return history, nil
}

func readChangelog(root string) ([]changelogEntry, error) {
	data, err := os.ReadFile(filepath.Join(root, changelogPath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", changelogPath, err)
	}
	return parseChangelog(string(data)), nil
}

// parseChangelog reads every entry of a changelog: a level-two heading
// "YYYY-MM-DD — Title" outside a code fence, the metadata block (the first
// block after the heading), then paragraphs. Text before the first entry is
// a preamble. It never fails; each entry records what is malformed in it.
func parseChangelog(text string) []changelogEntry {
	var entries []changelogEntry
	var current *changelogEntry
	var paragraph []string
	var seen map[string]bool
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
			validateEntry(current, seen)
			entries = append(entries, *current)
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
			current = &changelogEntry{heading: strings.TrimSpace(heading)}
			current.Touches, current.Sources, current.Text = []string{}, []string{}, []string{}
			seen = map[string]bool{}
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
			metadataLine(current, seen, trimmed)
		}
	}
	finish()
	return entries
}

// metadataLine records one line of an entry's metadata block.
func metadataLine(e *changelogEntry, seen map[string]bool, line string) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || !slices.Contains(metadataFields, key) {
		e.add(fmt.Sprintf("metadata line %q is not a Branch, Merge, Commits, Touches, or Sources line", line),
			"put a blank line between the metadata and the text, or correct the field name")
		return
	}
	if seen[key] {
		e.add(key+" is given twice", "give each metadata field once")
		return
	}
	seen[key] = true
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
func validateEntry(e *changelogEntry, seen map[string]bool) {
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
	if seen["Merge"] && seen["Commits"] {
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
		e.add("missing Sources", "add a Sources: line with at least one explorer reference or #n number")
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

// validateHistory checks the changelog and reports each problem against the
// entry it is in.
func (c *checker) validateHistory() {
	entries, err := readChangelog(c.root)
	if err != nil {
		c.add(changelogPath, "file", err.Error(), "create it in the format in docs/architecture/explorer/README.md")
		return
	}
	repo := &gitRepo{root: c.root}
	var above *changelogEntry // the nearest entry above with a real date
	for i := range entries {
		e := &entries[i]
		item := fmt.Sprintf("entry %q", e.heading)
		for _, is := range e.issues {
			c.add(changelogPath, item, is.message, is.fix)
		}
		for _, source := range e.Sources {
			if !strings.HasPrefix(source, "#") {
				c.ref(changelogPath, item, source, nil)
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

// verifyCommits checks an entry's merge commit or commit range with git.
// When git cannot read the repository, that is reported once and no entry's
// commits are verified.
func (c *checker) verifyCommits(repo *gitRepo, item string, e *changelogEntry) {
	var shas []string
	if isSHA(e.Merge) {
		shas = append(shas, e.Merge)
	}
	r := e.Commits
	inRange := r != nil && isSHA(r.From) && isSHA(r.To)
	if inRange {
		shas = append(shas, r.From, r.To)
	}
	if len(shas) == 0 || repo.reported {
		return
	}
	if err := repo.usable(); err != nil {
		repo.reported = true
		c.add(changelogPath, "commits", "git cannot verify the changelog's commits: "+err.Error(),
			"run the check from a git checkout, with git on PATH and the full history (CI checks out with fetch-depth 0)")
		return
	}
	const fix = "use a commit SHA from this repository's history, long enough to be unambiguous; a shallow clone needs its full history"
	if !inRange {
		if !repo.hasCommit(e.Merge) {
			c.add(changelogPath, item, "git does not know the merge commit "+e.Merge, fix)
		}
		return
	}
	known := true
	for _, sha := range []string{r.From, r.To} {
		if !repo.hasCommit(sha) {
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
	root     string
	checked  bool
	err      error // why git cannot read the repository
	reported bool  // err has been reported
}

// usable returns why git cannot read the repository, or nil; it asks once.
func (g *gitRepo) usable() error {
	if !g.checked {
		g.checked = true
		if _, err := exec.LookPath("git"); err != nil {
			g.err = fmt.Errorf("git is not on PATH: %w", err)
		} else if out, err := g.run("rev-parse", "--git-dir"); err != nil {
			g.err = fmt.Errorf("%s (%v)", out, err)
		}
	}
	return g.err
}

func (g *gitRepo) run(args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", g.root}, args...)...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// hasCommit reports whether sha names exactly one commit git knows.
func (g *gitRepo) hasCommit(sha string) bool {
	_, err := g.run("cat-file", "-e", sha+"^{commit}")
	return err == nil
}

// isAncestor reports whether from is to or an ancestor of it.
func (g *gitRepo) isAncestor(from, to string) (bool, error) {
	out, err := g.run("merge-base", "--is-ancestor", from, to)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s (%v)", out, err)
	}
	return true, nil
}
