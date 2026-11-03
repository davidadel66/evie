package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
)

// writeFiles writes files (slash paths) under dir.
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// homeSkill writes the skill folder name under dir (a slash path relative
// to the fake home) with a valid SKILL.md and any extra files.
func (c *skillCLI) homeSkill(dir, name, description string, extra map[string]string) string {
	c.t.Helper()
	folder := filepath.Join(c.home, filepath.FromSlash(dir), name)
	files := map[string]string{"SKILL.md": skillMD(name, description)}
	for rel, text := range extra {
		files[rel] = text
	}
	writeFiles(c.t, folder, files)
	return folder
}

// importSection is the lines of one section ("Proposed", "Refused", ...)
// of an import report, without its heading; "" when the report has none.
func importSection(report, heading string) string {
	_, rest, found := strings.Cut(report, "\n"+heading+" (")
	if !found {
		return ""
	}
	_, rest, _ = strings.Cut(rest, "\n")
	var b strings.Builder
	for _, line := range strings.SplitAfter(rest, "\n") {
		if !strings.HasPrefix(line, "  ") {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

// A personal Claude Code skill is proposed by the migration, recorded with
// its source, and is not active until the owner approves it.
func TestSkillCLIImportClaudeProposesAPersonalSkill(t *testing.T) {
	c := newSkillCLI(t)
	c.homeSkill(".claude/skills", "grilling", "Grill a plan.", nil)

	report := c.mustRun("skills", "import-claude")
	importID := importLine.FindStringSubmatch(report)
	if importID == nil {
		t.Fatalf("the report names no import:\n%s", report)
	}
	proposed := importSection(report, "Proposed")
	mustContain(t, "proposed", proposed, "grilling", "Claude personal (~/.claude/skills/grilling)", "Warnings: none")
	mustContain(t, "report", report, "Nothing is active until you approve it", "evie skills approve --import "+importID[1])

	listed := c.mustRun("skills", "list")
	mustContain(t, "list after import", listed, "Pending proposals", "grilling")
	if strings.Contains(listed, "Active skills") {
		t.Fatalf("an imported skill is active:\n%s", listed)
	}
	mustContain(t, "show", c.mustRun("skills", "show", "grilling"), "Source: claude-personal "+filepath.Join(c.home, ".claude", "skills", "grilling"))
}

var importLine = regexp.MustCompile(`Import ([0-9a-f-]{36})`)

// The migration proposes each name once, from its first copy in Claude
// Code's precedence (personal, synced, ~/.agents), and reports later
// copies as shadowed: a personal skill that is a link to a ~/.agents skill
// is proposed as Claude personal and shadows its own target. The synced
// folder's marker and bookkeeping files are not skills, a folder without
// SKILL.md is not a skill, and a skill with scripts is refused with its
// reason while still shadowing later copies of its name.
func TestSkillCLIImportClaudeFollowsPrecedence(t *testing.T) {
	c := newSkillCLI(t)
	c.homeSkill(".claude/skills", "grilling", "Grill a plan (personal).", nil)
	target := c.homeSkill(".agents/skills", "frontend-design", "Design a frontend.", nil)
	if err := os.Symlink(target, filepath.Join(c.home, ".claude", "skills", "frontend-design")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Join(c.home, ".claude", "skills"), map[string]string{
		"notes/README.md":                         "Not a skill.\n",
		"synced/.bucket-1234":                     "",
		"synced/bucket-a/.last-complete-round":    "7\n",
		"synced/bucket-a/manifest.json":           "{}\n",
		"synced/bucket-b/manifest.json":           "{}\n",
		"synced/bucket-a/docx/SKILL.md":           skillMD("docx", "Edit Word documents."),
		"synced/bucket-a/docx/scripts/convert.md": "Convert it.\n",
		"synced/bucket-a/pdf/SKILL.md":            skillMD("pdf", "Read PDFs."),
		"synced/bucket-a/grilling/SKILL.md":       skillMD("grilling", "Grill a plan (synced)."),
		"synced/bucket-b/pdf/SKILL.md":            skillMD("pdf", "Read PDFs (second bucket)."),
	})
	c.homeSkill(".agents/skills", "grilling", "Grill a plan (agents).", nil)
	c.homeSkill(".agents/skills", "docx", "Edit Word documents (agents).", nil)
	c.homeSkill(".agents/skills", "tdd", "Test first.", nil)
	c.homeSkill(".agents/skills", "diagnosing-bugs", "Diagnose a bug.", map[string]string{"scripts/hook.md": "Hook.\n"})

	report := c.mustRun("skills", "import-claude")
	proposed := importSection(report, "Proposed")
	for _, want := range []string{
		"grilling  ", "from Claude personal (~/.claude/skills/grilling)",
		"frontend-design  ", "from Claude personal (~/.claude/skills/frontend-design)",
		"pdf  ", "from Claude synced (~/.claude/skills/synced/bucket-a/pdf)",
		"tdd  ", "from ~/.agents (~/.agents/skills/tdd)",
	} {
		mustContain(t, "proposed", proposed, want)
	}
	if n := strings.Count(proposed, "Warnings: none"); n != 4 {
		t.Fatalf("%d skills proposed; want 4:\n%s", n, report)
	}
	refused := importSection(report, "Refused")
	mustContain(t, "refused", refused,
		"docx  from Claude synced (~/.claude/skills/synced/bucket-a/docx)",
		"diagnosing-bugs  from ~/.agents (~/.agents/skills/diagnosing-bugs)",
		"scripts: a scripts/ folder is refused: Evie does not run skill scripts")
	shadowed := importSection(report, "Shadowed")
	mustContain(t, "shadowed", shadowed,
		"grilling  from Claude synced (~/.claude/skills/synced/bucket-a/grilling), shadowed by Claude personal (~/.claude/skills/grilling)",
		"pdf  from Claude synced (~/.claude/skills/synced/bucket-b/pdf), shadowed by Claude synced (~/.claude/skills/synced/bucket-a/pdf)",
		"frontend-design  from ~/.agents (~/.agents/skills/frontend-design), shadowed by Claude personal (~/.claude/skills/frontend-design)",
		"grilling  from ~/.agents (~/.agents/skills/grilling), shadowed by Claude personal (~/.claude/skills/grilling)",
		"docx  from ~/.agents (~/.agents/skills/docx), shadowed by Claude synced (~/.claude/skills/synced/bucket-a/docx)")
	if n := strings.Count(shadowed, "shadowed by"); n != 5 {
		t.Fatalf("%d skills shadowed; want 5:\n%s", n, report)
	}
	for _, unwanted := range []string{"notes", "manifest", "bucket-1234", "last-complete-round", "Unchanged", "Notes"} {
		if strings.Contains(report, unwanted) {
			t.Fatalf("the report mentions %q:\n%s", unwanted, report)
		}
	}

	// The winning copy is the one proposed.
	mustContain(t, "show grilling", c.mustRun("skills", "show", "grilling"), "Grill a plan (personal).")
	mustContain(t, "show pdf", c.mustRun("skills", "show", "pdf"), "Description: Read PDFs.\n")
	if _, err := c.run("skills", "show", "docx"); err == nil {
		t.Fatal("a shadowed copy of a refused skill was proposed")
	}
	if listed := c.mustRun("skills", "list"); strings.Contains(listed, "Active skills") {
		t.Fatalf("an imported skill is active:\n%s", listed)
	}
}

// Running the migration again with nothing changed proposes nothing: each
// skill's version is already pending or active. Only a skill edited since
// is proposed again, as an update of its pending proposal.
func TestSkillCLIImportClaudeAgainProposesOnlyChangedSkills(t *testing.T) {
	c := newSkillCLI(t)
	grilling := c.homeSkill(".claude/skills", "grilling", "Grill a plan.", nil)
	c.homeSkill(".agents/skills", "tdd", "Test first.", nil)
	c.homeSkill(".agents/skills", "pdf", "Read PDFs.", nil)
	c.homeSkill(".agents/skills", "diagnosing-bugs", "Diagnose a bug.", map[string]string{"scripts/hook.md": "Hook.\n"})
	first := c.mustRun("skills", "import-claude")
	if n := strings.Count(importSection(first, "Proposed"), "Warnings: none"); n != 3 {
		t.Fatalf("%d skills proposed; want 3:\n%s", n, first)
	}
	pdf := regexp.MustCompile(`pdf  ([0-9a-f]{64})`).FindStringSubmatch(first)[1]
	c.mustRun("skills", "approve", "pdf", "--hash", pdf, "--expect-version", "0")

	again := c.mustRun("skills", "import-claude")
	if section := importSection(again, "Proposed"); section != "" {
		t.Fatalf("a second run proposed:\n%s", again)
	}
	mustContain(t, "second run", again, "Nothing new was proposed; there is nothing to approve.")
	unchanged := importSection(again, "Unchanged")
	mustContain(t, "unchanged", unchanged,
		"grilling  from Claude personal (~/.claude/skills/grilling): the same as its pending proposal ",
		"tdd  from ~/.agents (~/.agents/skills/tdd): the same as its pending proposal ",
		"pdf  from ~/.agents (~/.agents/skills/pdf): the same as its active version 1")
	mustContain(t, "refused again", importSection(again, "Refused"), "diagnosing-bugs")
	if strings.Contains(again, "evie skills approve --import") {
		t.Fatalf("a run that proposed nothing offers an approval:\n%s", again)
	}

	writeFiles(t, grilling, map[string]string{"SKILL.md": skillMD("grilling", "Grill a plan, harder.")})
	third := c.mustRun("skills", "import-claude")
	proposed := importSection(third, "Proposed")
	if !strings.Contains(proposed, "grilling  ") || strings.Count(proposed, "Warnings: none") != 1 {
		t.Fatalf("the third run should propose only the edited grilling:\n%s", third)
	}
	mustContain(t, "show the update", c.mustRun("skills", "show", "grilling"), "Grill a plan, harder.", "pending approval")
}

// proposedHash is the version hash an import report shows for name.
func proposedHash(t *testing.T, report, name string) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^  ` + regexp.QuoteMeta(name) + `  ([0-9a-f]{64})  from `).FindStringSubmatch(importSection(report, "Proposed"))
	if m == nil {
		t.Fatalf("the report proposes no %s:\n%s", name, report)
	}
	return m[1]
}

// The owner approves a whole import in one command: each of its proposals
// is activated at the version hash its report showed, against whatever
// version is active then. A proposal superseded since, by a later import
// of an edited skill, is refused and reported, and the rest are still
// activated; one already active is reported as such.
func TestSkillCLIApproveImportActivatesTheReportedVersions(t *testing.T) {
	c := newSkillCLI(t)
	grilling := c.homeSkill(".claude/skills", "grilling", "Grill a plan.", nil)
	tdd := c.homeSkill(".agents/skills", "tdd", "Test first.", nil)
	c.homeSkill(".agents/skills", "pdf", "Read PDFs.", nil)
	first := c.mustRun("skills", "import-claude")
	firstID := importLine.FindStringSubmatch(first)[1]
	grillingHash, tddHash, pdfHash := proposedHash(t, first, "grilling"), proposedHash(t, first, "tdd"), proposedHash(t, first, "pdf")

	c.mustRun("skills", "approve", "pdf", "--hash", pdfHash, "--expect-version", "0")
	writeFiles(t, tdd, map[string]string{"SKILL.md": skillMD("tdd", "Test first, always.")})
	second := c.mustRun("skills", "import-claude")
	secondID := importLine.FindStringSubmatch(second)[1]
	newTDDHash := proposedHash(t, second, "tdd")

	out, err := c.run("skills", "approve", "--import", firstID)
	if err == nil || !strings.Contains(err.Error(), "1 of 3") {
		t.Fatalf("approving an import with a superseded proposal: %v\n%s", err, out)
	}
	mustContain(t, "approve the first import", out,
		"Activated grilling version 1 (version hash "+grillingHash+")",
		"Already active: pdf version 1 (version hash "+pdfHash+")",
		"Refused tdd: ", "superseded", "review it with evie skills show tdd")
	if strings.Contains(out, "Activated tdd") || strings.Contains(out, tddHash+")") {
		t.Fatalf("the superseded tdd proposal was activated:\n%s", out)
	}
	listed := c.mustRun("skills", "list")
	mustContain(t, "list after the first approval", listed, "grilling  version 1  "+grillingHash[:12], "pdf  version 1  "+pdfHash[:12],
		"Pending proposals", "tdd  "+newTDDHash[:12])

	mustContain(t, "approve the second import", c.mustRun("skills", "approve", "--import", secondID),
		"Activated tdd version 1 (version hash "+newTDDHash+")")

	// An update is approved over the active version the owner has now.
	writeFiles(t, grilling, map[string]string{"SKILL.md": skillMD("grilling", "Grill a plan, harder.")})
	third := c.mustRun("skills", "import-claude")
	newGrillingHash := proposedHash(t, third, "grilling")
	mustContain(t, "approve the update", c.mustRun("skills", "approve", "--import", importLine.FindStringSubmatch(third)[1]),
		"Activated grilling version 2 (version hash "+newGrillingHash+")")
	mustContain(t, "list after the update", c.mustRun("skills", "list"), "grilling  version 2  "+newGrillingHash[:12])

	nothing := importLine.FindStringSubmatch(c.mustRun("skills", "import-claude"))[1]
	mustContain(t, "approve an import that proposed nothing", c.mustRun("skills", "approve", "--import", nothing), "proposed nothing")
	if _, err := c.run("skills", "approve", "--import", "4f9d3c1e-0000-4000-8000-000000000000"); !errors.Is(err, eviedb.ErrSkillNotFound) {
		t.Fatalf("approving an unknown import: %v", err)
	}
}

// A plugin can list its skill folders in .claude-plugin/plugin.json, as
// mattpocock-skills does with folders grouped under skills/<group>/. The
// migration reads those as well as skills/<name>/, and a listed path that
// leaves the plugin's folder, or names no skill, is reported, not read.
func TestSkillCLIImportClaudeReadsSkillsAPluginLists(t *testing.T) {
	c := newSkillCLI(t)
	root := ".claude/plugins/cache/market/eng-skills/1.3.1"
	install := filepath.Join(c.home, filepath.FromSlash(root))
	c.homeSkill(root+"/skills/engineering", "tdd", "Test first (plugin).", nil)
	c.homeSkill(root+"/skills/productivity", "grilling", "Grill a plan.", nil)
	c.homeSkill(root+"/skills", "plain", "A skill in the default folder.", nil)
	c.homeSkill(".claude/plugins/cache/market", "outside", "Not the plugin's own.", nil)
	writeFiles(t, install, map[string]string{".claude-plugin/plugin.json": `{
  "name": "eng-skills",
  "version": "1.3.1",
  "skills": ["./skills/engineering/tdd", "./skills/productivity/grilling", "./skills/plain", "../../outside", "./skills/engineering/missing"]
}`})
	writeFiles(t, filepath.Join(c.home, ".claude", "plugins"), map[string]string{"installed_plugins.json": `{"plugins": {"eng-skills@market": [{"installPath": "` + install + `", "version": "1.3.1"}]}}`})

	report := c.mustRun("skills", "import-claude", "--plugins")
	proposed := importSection(report, "Proposed")
	mustContain(t, "listed plugin skills", proposed,
		"tdd  ", "from plugin eng-skills@market 1.3.1 (~/"+root+"/skills/engineering/tdd)",
		"grilling  ", "from plugin eng-skills@market 1.3.1 (~/"+root+"/skills/productivity/grilling)",
		"plain  ", "from plugin eng-skills@market 1.3.1 (~/"+root+"/skills/plain)")
	if strings.Contains(report, "Not the plugin's own") || strings.Contains(proposed, "outside") || strings.Count(proposed, "plain  ") != 1 {
		t.Fatalf("a path outside the plugin was read, or a skill was proposed twice:\n%s", report)
	}
	mustContain(t, "notes", importSection(report, "Notes"),
		"plugin eng-skills@market lists ../../outside, which is outside its folder; it was not read",
		"plugin eng-skills@market lists ./skills/engineering/missing, which holds no SKILL.md; it was not read")
}

// Installed Claude Code plugin skills are read only with --plugins, from
// each install path that ~/.claude/plugins/installed_plugins.json lists,
// last in precedence, and recorded with the plugin's name and version.
// Entries the migration cannot use are reported, not fatal.
func TestSkillCLIImportClaudeReadsPluginSkillsOnlyWhenAsked(t *testing.T) {
	c := newSkillCLI(t)
	c.homeSkill(".agents/skills", "tdd", "Test first.", nil)
	review := filepath.Join(c.home, ".claude", "plugins", "cache", "market", "review-tools", "1.2.0")
	c.homeSkill(".claude/plugins/cache/market/review-tools/1.2.0/skills", "review-pr", "Review a pull request.", nil)
	c.homeSkill(".claude/plugins/cache/market/review-tools/1.2.0/skills", "tdd", "Test first (plugin).", nil)
	legacy := filepath.Join(c.home, ".claude", "plugins", "cache", "market", "docs-kit")
	c.homeSkill(".claude/plugins/cache/market/docs-kit/skills", "changelog", "Write a changelog.", nil)
	installed := `{
  "version": 2,
  "plugins": {
    "review-tools@market": [{"scope": "user", "installPath": "` + review + `", "version": "1.2.0", "installedAt": "2026-10-01T00:00:00Z"}],
    "docs-kit@market": {"installPath": "` + legacy + `", "version": "0.3.1"},
    "no-path@market": [{"scope": "user", "version": "1.0.0"}],
    "odd@market": "unexpected"
  }
}`
	writeFiles(t, filepath.Join(c.home, ".claude", "plugins"), map[string]string{"installed_plugins.json": installed})

	without := c.mustRun("skills", "import-claude")
	for _, unwanted := range []string{"review-pr", "changelog", "plugin", "Notes"} {
		if strings.Contains(without, unwanted) {
			t.Fatalf("an import without --plugins mentions %q:\n%s", unwanted, without)
		}
	}

	with := c.mustRun("skills", "import-claude", "--plugins")
	mustContain(t, "proposed plugin skills", importSection(with, "Proposed"),
		"review-pr  ", "from plugin review-tools@market 1.2.0 (~/.claude/plugins/cache/market/review-tools/1.2.0/skills/review-pr)",
		"changelog  ", "from plugin docs-kit@market 0.3.1 (~/.claude/plugins/cache/market/docs-kit/skills/changelog)")
	mustContain(t, "shadowed plugin skill", importSection(with, "Shadowed"),
		"tdd  from plugin review-tools@market 1.2.0 (~/.claude/plugins/cache/market/review-tools/1.2.0/skills/tdd), shadowed by ~/.agents (~/.agents/skills/tdd)")
	mustContain(t, "unchanged", importSection(with, "Unchanged"), "tdd  from ~/.agents")
	mustContain(t, "notes", importSection(with, "Notes"),
		"plugin no-path@market in ~/.claude/plugins/installed_plugins.json has no absolute install path; its skills were not read",
		"plugin odd@market in ~/.claude/plugins/installed_plugins.json is not an install record or a list of them; its skills were not read")
	mustContain(t, "show a plugin skill", c.mustRun("skills", "show", "review-pr"), "Source: claude-plugin "+filepath.Join(review, "skills", "review-pr"))

	// A missing or unreadable plugin list is reported, and the other
	// sources are still read.
	for _, tc := range []struct{ name, file, want string }{
		{"missing", "", "found no ~/.claude/plugins/installed_plugins.json; no plugin skills were read"},
		{"not JSON", "{not json", "~/.claude/plugins/installed_plugins.json is not the expected JSON"},
		{"no plugins", `{"version": 2}`, "~/.claude/plugins/installed_plugins.json is not the expected JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newSkillCLI(t)
			c.homeSkill(".agents/skills", "tdd", "Test first.", nil)
			if tc.file != "" {
				writeFiles(t, filepath.Join(c.home, ".claude", "plugins"), map[string]string{"installed_plugins.json": tc.file})
			}
			report := c.mustRun("skills", "import-claude", "--plugins")
			mustContain(t, "notes", importSection(report, "Notes"), tc.want)
			mustContain(t, "proposed", importSection(report, "Proposed"), "tdd  ")
		})
	}
}
