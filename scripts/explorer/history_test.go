package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// changelog joins entries under a preamble into a CHANGELOG.md.
func changelog(entries ...string) string {
	return "# Changelog\n\nNewest first.\n\n" + strings.Join(entries, "\n")
}

// validEntry is a well-formed entry that needs no git.
const validEntry = "## 2026-10-20 — Add the run command\n" +
	"Branch: feat/run\n" +
	"Touches: cmd/tool\n" +
	"Sources: doc:docs/guide.md, #12\n" +
	"\n" +
	"The tool gains a run command.\n"

// withChangelog replaces the fixture's changelog and re-extracts, so check
// reports the changelog's own problems rather than stale facts.
func withChangelog(text string) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		writeFile(t, root, changelogPath, text)
		extractFixture(t, root)
	}
}

// entryProblem is what check prints for a problem with the named entry.
func entryProblem(heading, message string) string {
	return changelogPath + ": entry \"" + heading + "\": " + message
}

func TestExtractParsesTheChangelogIntoHistory(t *testing.T) {
	root := newFixture(t)
	facts := decodeFacts(t, extractFixture(t, root))

	// Entries keep the file's order; lines of a paragraph join with a space;
	// the preamble and a heading inside a fence are not entries.
	want := `[` +
		`{"branch":"feat/run","date":"2026-10-20","sources":["doc:docs/guide.md#getting-started","#12"],` +
		`"text":["The tool gains a run command that starts alpha. It reads its widgets first.","A second paragraph says why."],` +
		`"title":"Add the tool's run command","touches":["cmd/tool","internal/alpha"]},` +
		`{"branch":"chore/retire-store","date":"2026-10-20","sources":["adr:0001"],` +
		`"text":["The old store is removed; its history stays readable."],` +
		`"title":"Retire the old store","touches":["internal/gone"]},` +
		`{"branch":"master","date":"2026-10-02","sources":["#1","code:internal/alpha/alpha.go"],` +
		`"text":["The first entry."],` +
		`"title":"Start the fixture","touches":["internal/alpha","scripts/build"]}` +
		`]`
	if got := asJSON(t, facts["history"]); got != want {
		t.Fatalf("history =\n%s\nwant\n%s", got, want)
	}
}

func TestExtractFailsClearlyWithoutAChangelog(t *testing.T) {
	root := newFixture(t)
	if err := os.Remove(filepath.Join(root, changelogPath)); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runTool(t, root, "extract")
	if code != 2 || !strings.Contains(stderr, "read CHANGELOG.md") {
		t.Fatalf("extract exit %d, stderr %q; want 2 naming CHANGELOG.md", code, stderr)
	}
}

func TestCheckReportsMalformedChangelogEntries(t *testing.T) {
	const heading = "2026-10-20 — Add the run command"
	without := func(line string) string { return strings.Replace(validEntry, line+"\n", "", 1) }
	runCheckCases(t, []checkCase{
		{
			name:   "missing branch",
			break_: withChangelog(changelog(without("Branch: feat/run"))),
			want:   []string{entryProblem(heading, "missing Branch")},
		},
		{
			name:   "missing touches",
			break_: withChangelog(changelog(without("Touches: cmd/tool"))),
			want:   []string{entryProblem(heading, "missing Touches")},
		},
		{
			name:   "missing sources",
			break_: withChangelog(changelog(without("Sources: doc:docs/guide.md, #12"))),
			want:   []string{entryProblem(heading, "missing Sources")},
		},
		{
			name:   "missing text",
			break_: withChangelog(changelog(without("The tool gains a run command."))),
			want:   []string{entryProblem(heading, "missing text")},
		},
		{
			name:   "heading with a hyphen for the em dash",
			break_: withChangelog(changelog(strings.Replace(validEntry, " — ", " - ", 1))),
			want:   []string{entryProblem("2026-10-20 - Add the run command", "the heading is not YYYY-MM-DD — Title")},
		},
		{
			name:   "heading with no title",
			break_: withChangelog(changelog(strings.Replace(validEntry, " — Add the run command", " — ", 1))),
			want:   []string{entryProblem("2026-10-20 —", "missing title")},
		},
		{
			name:   "unknown metadata field",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Branch:", "Brnach:", 1))),
			want:   []string{entryProblem(heading, `metadata line "Brnach: feat/run" is not a Branch, Merge, Commits, Touches, or Sources line`), entryProblem(heading, "missing Branch")},
		},
		{
			name:   "text with no blank line after the metadata",
			break_: withChangelog(changelog(strings.Replace(validEntry, "#12\n\n", "#12\n", 1))),
			want:   []string{entryProblem(heading, `metadata line "The tool gains a run command." is not`), "put a blank line between the metadata and the text"},
		},
		{
			name:   "field given twice",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Branch: feat/run\n", "Branch: feat/run\nBranch: feat/other\n", 1))),
			want:   []string{entryProblem(heading, "Branch is given twice")},
		},
		{
			name:   "both merge and commits",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Branch: feat/run\n", "Branch: feat/run\nMerge: abc1234\nCommits: abc1234..def5678\n", 1))),
			want:   []string{entryProblem(heading, "the entry has both Merge and Commits")},
		},
		{
			name:   "empty list item",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Touches: cmd/tool", "Touches: cmd/tool,", 1))),
			want:   []string{entryProblem(heading, "Touches has an empty item")},
		},
		{
			name:   "touched path that is not a clean relative path",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Touches: cmd/tool", "Touches: cmd/tool/, ../outside", 1))),
			want:   []string{entryProblem(heading, `Touches item "cmd/tool/" is not a clean relative path`), entryProblem(heading, `Touches item "../outside"`)},
		},
		{
			name:   "merge that is not a commit SHA",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Branch: feat/run\n", "Branch: feat/run\nMerge: main\n", 1))),
			want:   []string{entryProblem(heading, `Merge "main" is not a commit SHA`)},
		},
		{
			name:   "commits that are not a range",
			break_: withChangelog(changelog(strings.Replace(validEntry, "Branch: feat/run\n", "Branch: feat/run\nCommits: abc1234\n", 1))),
			want:   []string{entryProblem(heading, `Commits "abc1234" is not <from>..<to>`)},
		},
	})
}

func TestCheckReportsChangelogDatesThatAreNotRealCalendarDates(t *testing.T) {
	cases := []checkCase{}
	for _, date := range []string{"2026-02-30", "2026-13-01", "2026-10-2", "26-10-20", "2026/10/20", "someday"} {
		heading := date + " — Add the run command"
		cases = append(cases, checkCase{
			name:   date,
			break_: withChangelog(changelog(strings.Replace(validEntry, "2026-10-20", date, 1))),
			want:   []string{entryProblem(heading, fmt.Sprintf("date %q is not a real calendar date", date)), "YYYY-MM-DD"},
		})
	}
	runCheckCases(t, cases)
}

// entryOn is validEntry with another date and title.
func entryOn(date, title string) string {
	return strings.Replace(validEntry, "2026-10-20 — Add the run command", date+" — "+title, 1)
}

func TestCheckReportsChangelogEntriesOutOfNewestFirstOrder(t *testing.T) {
	runCheckCases(t, []checkCase{{
		name: "older entry above a newer one",
		break_: withChangelog(changelog(
			entryOn("2026-10-20", "Newest"),
			entryOn("2026-10-02", "Older"),
			entryOn("2026-10-05", "Out of place"),
		)),
		want: []string{
			entryProblem("2026-10-05 — Out of place", `the entry is newer than "2026-10-02 — Older" above it`),
			"keep entries newest first",
			"1 problem(s)",
		},
	}})
}

func TestCheckAcceptsChangelogEntriesOnTheSameDay(t *testing.T) {
	root := newFixture(t)
	withChangelog(changelog(entryOn("2026-10-20", "Second merge that day"), entryOn("2026-10-20", "First merge that day")))(t, root)
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

func TestCheckReportsChangelogSourcesThatDoNotResolve(t *testing.T) {
	const heading = "2026-10-20 — Add the run command"
	withSources := func(sources string) func(*testing.T, string) {
		return withChangelog(changelog(strings.Replace(validEntry, "Sources: doc:docs/guide.md, #12", "Sources: "+sources, 1)))
	}
	cases := []checkCase{}
	for _, tc := range []struct{ name, source, why string }{
		{"missing doc", "doc:docs/missing.md", "docs/missing.md does not exist"},
		{"missing heading", "doc:docs/guide.md#no-such-heading", "no heading"},
		{"missing code symbol", "code:internal/alpha/alpha.go#Missing", "declares no Missing"},
		{"missing package", "pkg:internal/ghost", "no package"},
		{"missing ADR", "adr:0099", "no ADR 0099"},
		{"unknown kind", "https://example.com", "unknown reference kind"},
	} {
		cases = append(cases, checkCase{
			name:   tc.name,
			break_: withSources("#12, " + tc.source),
			want:   []string{entryProblem(heading, fmt.Sprintf("dead reference %q: ", tc.source)), tc.why},
		})
	}
	for _, number := range []string{"#", "#0", "#012", "#12a", "# 12", "#-3"} {
		cases = append(cases, checkCase{
			name:   "malformed number " + number,
			break_: withSources("doc:docs/guide.md, " + number),
			want:   []string{entryProblem(heading, fmt.Sprintf("malformed ticket or pull-request number %q", number)), "such as #179", "1 problem(s)"},
		})
	}
	runCheckCases(t, cases)
}

// git runs git in root, ignoring the owner's configuration, and returns its
// trimmed output.
func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// commits are the SHAs of the fixture's git history.
type commits struct {
	first, second string // the main line, oldest first
	branch        string // the merged branch's commit
	merge         string // the merge commit
}

// fixtureHistory makes the fixture a git repository: two commits on the
// main line, then a branch with one commit merged into it.
func fixtureHistory(t *testing.T, root string) commits {
	t.Helper()
	commit := func(name string) string {
		writeFile(t, root, "notes/"+name+".txt", name+"\n")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", name)
		return git(t, root, "rev-parse", "HEAD")
	}
	var h commits
	git(t, root, "init", "-q")
	h.first = commit("first")
	h.second = commit("second")
	git(t, root, "checkout", "-q", "-b", "feat/run")
	h.branch = commit("branch")
	git(t, root, "checkout", "-q", "-")
	git(t, root, "merge", "-q", "--no-ff", "-m", "Merge feat/run", "feat/run")
	h.merge = git(t, root, "rev-parse", "HEAD")
	return h
}

// withMetadata is validEntry with one more metadata line.
func withMetadata(line string) string {
	return strings.Replace(validEntry, "Branch: feat/run\n", "Branch: feat/run\n"+line+"\n", 1)
}

// retitled is an entry with another date and title.
func retitled(entry, date, title string) string {
	return strings.Replace(entry, "2026-10-20 — Add the run command", date+" — "+title, 1)
}

// assertCheckReports runs check on root and requires exit 1 with every want.
func assertCheckReports(t *testing.T, root string, want ...string) {
	t.Helper()
	code, out := checkFixture(t, root)
	if code != 1 {
		t.Fatalf("check exit %d, want 1:\n%s", code, out)
	}
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("output lacks %q:\n%s", w, out)
		}
	}
}

func TestCheckAcceptsMergeCommitsAndRangesGitKnows(t *testing.T) {
	root := newFixture(t)
	h := fixtureHistory(t, root)
	withChangelog(changelog(
		withMetadata("Merge: "+h.merge[:7]),
		retitled(withMetadata("Commits: "+h.first+".."+h.second), "2026-10-19", "Two direct commits"),
		retitled(withMetadata("Commits: "+h.first[:7]+".."+h.first[:7]), "2026-10-18", "One direct commit"),
	))(t, root)

	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}

func TestCheckReportsCommitsGitDoesNotKnow(t *testing.T) {
	const heading = "2026-10-20 — Add the run command"
	for _, tc := range []struct {
		name string
		line func(h commits) string
		want func(h commits) []string
	}{
		{
			name: "unknown merge commit",
			line: func(commits) string { return "Merge: 1234567" },
			want: func(commits) []string {
				return []string{entryProblem(heading, "git does not know the merge commit 1234567"), "full history"}
			},
		},
		{
			name: "range ending in an unknown commit",
			line: func(h commits) string { return "Commits: " + h.first[:7] + "..abcdef0" },
			want: func(commits) []string {
				return []string{entryProblem(heading, "git does not know commit abcdef0 in Commits"), "1 problem(s)"}
			},
		},
		{
			name: "range written newest first",
			line: func(h commits) string { return "Commits: " + h.second[:7] + ".." + h.first[:7] },
			want: func(h commits) []string {
				return []string{entryProblem(heading, "commit "+h.second[:7]+" is not "+h.first[:7]+" or an ancestor of it"), "oldest first"}
			},
		},
		{
			name: "range across lines of history that diverge",
			line: func(h commits) string { return "Commits: " + h.branch[:7] + ".." + h.second[:7] },
			want: func(h commits) []string {
				return []string{entryProblem(heading, "commit "+h.branch[:7]+" is not "+h.second[:7]+" or an ancestor of it")}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newFixture(t)
			h := fixtureHistory(t, root)
			withChangelog(changelog(withMetadata(tc.line(h))))(t, root)
			assertCheckReports(t, root, tc.want(h)...)
		})
	}
}

func TestCheckFailsClearlyWhenGitCannotVerifyCommits(t *testing.T) {
	twoMerges := changelog(withMetadata("Merge: 1234567"), retitled(withMetadata("Merge: abcdef0"), "2026-10-19", "Another"))
	runCheckCases(t, []checkCase{
		{
			name: "not a git repository",
			break_: func(t *testing.T, root string) {
				t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
				withChangelog(twoMerges)(t, root)
			},
			want: []string{changelogPath + ": commits: git cannot verify the changelog's commits", "not a git repository", "1 problem(s)"},
		},
		{
			name: "git not installed",
			break_: func(t *testing.T, root string) {
				fixtureHistory(t, root)
				withChangelog(twoMerges)(t, root)
				t.Setenv("PATH", "")
			},
			want: []string{changelogPath + ": commits: git cannot verify the changelog's commits", "git on PATH", "1 problem(s)"},
		},
	})
}
