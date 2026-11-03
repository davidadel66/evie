package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/skill"
	"github.com/davidadel66/evie/internal/subagents"
	"github.com/davidadel66/evie/internal/tools"
)

// skillCLI is the CLI approval harness of skills.spec.md: the owner's
// `evie skills` commands over a temporary procedural repository and a real
// SQLite store.
type skillCLI struct {
	t     *testing.T
	db    *sql.DB
	store *eviedb.Store
	repo  *procedural.Repo
}

func newSkillCLI(t *testing.T) *skillCLI {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &skillCLI{t: t, db: db, store: eviedb.NewStore(db), repo: procedural.New(filepath.Join(t.TempDir(), "procedural"))}
}

func (c *skillCLI) run(args ...string) (string, error) {
	c.t.Helper()
	var out bytes.Buffer
	handled, err := runSkillManagement(context.Background(), args, &out, skillCommands{store: c.store, repo: c.repo})
	if !handled {
		c.t.Fatalf("%q was not handled", args)
	}
	return out.String(), err
}

func (c *skillCLI) mustRun(args ...string) string {
	c.t.Helper()
	out, err := c.run(args...)
	if err != nil {
		c.t.Fatalf("%q: %v\n%s", args, err, out)
	}
	return out
}

// writeSkillFolder writes files (slash paths) into a new folder named name
// and returns the folder.
func writeSkillFolder(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	for rel, text := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const grillingSkill = `---
name: grilling
description: Grill the owner about a plan until every branch is resolved.
---
# Grilling

Ask one hard question at a time.
`

var (
	versionHashLine = regexp.MustCompile(`Version hash: ([0-9a-f]{64})`)
	proposalLine    = regexp.MustCompile(`proposal ([0-9a-f-]{36})`)
)

func mustContain(t *testing.T, what, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("%s lacks %q:\n%s", what, want, text)
		}
	}
}

// The owner proposes a skill folder, reviews it, approves exactly the
// version shown, approves an update over it (one version is active at a
// time) and deactivates it; list and show follow every step, and stale
// hashes and versions are refused.
func TestSkillCLIProposeApproveDeactivate(t *testing.T) {
	c := newSkillCLI(t)
	dir := writeSkillFolder(t, "grilling", map[string]string{
		"SKILL.md":                grillingSkill,
		"references/questions.md": "What breaks first?\n",
	})

	proposed := c.mustRun("skills", "propose", dir)
	mustContain(t, "propose output", proposed, "Proposed skill grilling", "SKILL.md", "references/questions.md", "Warnings: none", "Nothing is active until you approve it")
	m := versionHashLine.FindStringSubmatch(proposed)
	p := proposalLine.FindStringSubmatch(proposed)
	if m == nil || p == nil {
		t.Fatalf("propose output has no version hash or proposal:\n%s", proposed)
	}
	first, firstProposal := m[1], p[1]

	mustContain(t, "list before approval", c.mustRun("skills", "list"), "Pending proposals", "grilling", first[:12])
	mustContain(t, "show before approval", c.mustRun("skills", "show", "grilling"),
		"Status: not active", first, "Ask one hard question at a time.", "references/questions.md", "What breaks first?",
		"evie skills approve grilling --hash "+first+" --expect-version 0")

	for _, args := range [][]string{
		{"skills", "approve", "grilling"},
		{"skills", "approve", "grilling", "--hash", first},
		{"skills", "approve", "--hash", first, "--expect-version", "0"},
		{"skills", "approve", "grilling", "--hash", first, "--expect-version", "none"},
		{"skills", "approve", "grilling", "extra", "--hash", first, "--expect-version", "0"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if _, err := c.run("skills", "approve", "grilling", "--hash", strings.Repeat("a", 64), "--expect-version", "0"); err == nil {
		t.Fatal("a hash the owner did not review was approved")
	}
	if _, err := c.run("skills", "approve", "grilling", "--hash", first, "--expect-version", "1"); err == nil {
		t.Fatal("a stale expected version was approved")
	}
	mustContain(t, "approve output", c.mustRun("skills", "approve", "grilling", "--hash", first, "--expect-version", "0"),
		"Activated skill grilling version 1", first)
	listed := c.mustRun("skills", "list")
	mustContain(t, "list after approval", listed, "Active skills", "grilling  version 1  "+first[:12])
	if strings.Contains(listed, "Pending proposals") {
		t.Fatalf("an approved proposal is still pending:\n%s", listed)
	}

	// An edited folder is a new version, reviewed as a diff against the
	// active one; approving it supersedes version 1.
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(grillingSkill, "one hard question", "two hard questions", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	m = versionHashLine.FindStringSubmatch(c.mustRun("skills", "propose", dir))
	if m == nil || m[1] == first {
		t.Fatalf("the edited folder has hash %v", m)
	}
	second := m[1]
	mustContain(t, "show of the update", c.mustRun("skills", "show", "grilling"),
		"Status: active, version 1", second, "-Ask one hard question at a time.", "+Ask two hard questions at a time.",
		"evie skills approve grilling --hash "+second+" --expect-version 1")
	if _, err := c.run("skills", "approve", "grilling", "--hash", first, "--expect-version", "1"); err == nil ||
		!strings.Contains(err.Error(), "review it again with evie skills show grilling") || strings.Contains(err.Error(), second) {
		t.Fatalf("approving the superseded hash: %v", err)
	}
	if _, err := c.run("skills", "approve", "grilling", "--hash", second, "--expect-version", "0"); err == nil {
		t.Fatal("an approval that missed version 1 was accepted")
	}
	mustContain(t, "approve of the update", c.mustRun("skills", "approve", "grilling", "--hash", second, "--expect-version", "1"),
		"Activated skill grilling version 2")
	listed = c.mustRun("skills", "list")
	mustContain(t, "list after the update", listed, "grilling  version 2  "+second[:12])
	if strings.Contains(listed, "version 1") {
		t.Fatalf("two versions are active:\n%s", listed)
	}
	mustContain(t, "show of the first proposal", c.mustRun("skills", "show", "grilling", "--proposal", firstProposal),
		"approved as version 1", first, "Ask one hard question at a time.")

	mustContain(t, "deactivate output", c.mustRun("skills", "deactivate", "grilling"), "Deactivated skill grilling version 2")
	listed = c.mustRun("skills", "list")
	if strings.Contains(listed, "Active skills") || strings.Contains(listed, "grilling") {
		t.Fatalf("list after deactivation:\n%s", listed)
	}
	mustContain(t, "show after deactivation", c.mustRun("skills", "show", "grilling"), "Status: not active (version 2 was deactivated")
	if _, err := c.run("skills", "deactivate", "grilling"); err == nil {
		t.Fatal("a skill with no active version was deactivated")
	}
}

// A skill written for Claude Code is proposed with warnings for what Evie
// will not honour: frontmatter Evie ignores, inline shell commands,
// argument placeholders and mentions of Claude Code's tools. Recognised
// fields are recorded with the version.
func TestSkillCLIProposeReportsClaudeCodeOnlyFeatures(t *testing.T) {
	c := newSkillCLI(t)
	dir := writeSkillFolder(t, "review-pr", map[string]string{
		"SKILL.md": "---\n" +
			"name: review-pr\n" +
			"description: Review a pull request.\n" +
			"argument-hint: \"[pr-number]\"\n" +
			"license: MIT\n" +
			"disable-model-invocation: true\n" +
			"allowed-tools: Bash(gh pr diff:*)\n" +
			"context: fork\n" +
			"---\n" +
			"Current diff: !`gh pr diff`\n" +
			"\n" +
			"Review pull request $ARGUMENTS using the Read tool.\n",
		"references/checklist.md": "Run the tests with `Bash` before approving.\n",
	})
	proposed := c.mustRun("skills", "propose", dir)
	mustContain(t, "propose output", proposed,
		`"allowed-tools" is kept but has no effect in Evie`,
		`"context" is kept but has no effect in Evie`,
		"inline shell commands, which Evie never runs: !`gh pr diff` (line 10)",
		"argument placeholders, which Evie never substitutes: $ARGUMENTS (line 12)",
		"mentions Claude Code tools (Bash, Read in SKILL.md, references/checklist.md)")
	for _, unwanted := range []string{`"license"`, `"argument-hint"`, `"disable-model-invocation"`, "Warnings: none"} {
		if strings.Contains(proposed, unwanted) {
			t.Fatalf("propose output has %q:\n%s", unwanted, proposed)
		}
	}
	mustContain(t, "show", c.mustRun("skills", "show", "review-pr"),
		"Argument hint: [pr-number]", "License: MIT", "Owner-only: invoked with /review-pr", "inline shell commands")
}

func skillMD(name, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\n---\nDo the thing.\n"
}

// Each refused folder names its problem, and nothing it holds reaches the
// procedural repository or the store.
func TestSkillCLIProposeRefusesUnsafeOrInvalidFolders(t *testing.T) {
	c := newSkillCLI(t)
	valid := skillMD("grilling", "Grill a plan.")
	many := map[string]string{"SKILL.md": valid}
	for i := 0; i < skill.MaxFiles; i++ {
		many[fmt.Sprintf("references/%02d.md", i)] = "note\n"
	}
	large := map[string]string{"SKILL.md": valid}
	for i := 0; i < 40; i++ {
		large[fmt.Sprintf("references/%02d.md", i)] = strings.Repeat("x", 30<<10)
	}
	cases := []struct {
		name   string
		folder string
		files  map[string]string
		setup  func(t *testing.T, dir string)
		want   string
	}{
		{name: "scripts folder", folder: "grilling", files: map[string]string{"SKILL.md": valid, "scripts/run.md": "notes\n"},
			want: "scripts: a scripts/ folder is refused"},
		{name: "executable file", folder: "grilling", files: map[string]string{"SKILL.md": valid, "references/run.md": "echo hi\n"},
			setup: func(t *testing.T, dir string) { chmod(t, filepath.Join(dir, "references", "run.md"), 0o755) },
			want:  "references/run.md: is executable"},
		{name: "link to a file outside", folder: "grilling", files: map[string]string{"SKILL.md": valid},
			setup: func(t *testing.T, dir string) {
				outside := filepath.Join(filepath.Dir(dir), "secret.md")
				if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, "notes.md")); err != nil {
					t.Fatal(err)
				}
			},
			want: "notes.md: is a symbolic link"},
		{name: "linked SKILL.md", folder: "grilling", files: map[string]string{"notes.md": valid},
			setup: func(t *testing.T, dir string) {
				if err := os.Symlink("notes.md", filepath.Join(dir, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			},
			want: "SKILL.md: is a symbolic link"},
		{name: "binary file", folder: "grilling", files: map[string]string{"SKILL.md": valid, "logo.png": "\x89PNG\xff\xfe"},
			want: "logo.png: is not UTF-8 text"},
		{name: "NUL byte", folder: "grilling", files: map[string]string{"SKILL.md": valid, "notes.md": "a\x00b"},
			want: "notes.md: contains a NUL byte"},
		{name: "no SKILL.md", folder: "grilling", files: map[string]string{"README.md": "hello\n"},
			want: "SKILL.md: is missing"},
		{name: "no frontmatter", folder: "grilling", files: map[string]string{"SKILL.md": "# Grilling\n"},
			want: "SKILL.md: must start with YAML frontmatter"},
		{name: "missing name", folder: "grilling", files: map[string]string{"SKILL.md": "---\ndescription: Grill a plan.\n---\nGo.\n"},
			want: "SKILL.md: name is missing"},
		{name: "invalid name", folder: "Grilling", files: map[string]string{"SKILL.md": skillMD("Grilling", "Grill a plan.")},
			want: `SKILL.md: name "Grilling" must be 1 to 64 lowercase letters, digits and hyphens`},
		{name: "name not the folder's", folder: "grilling", files: map[string]string{"SKILL.md": skillMD("grill", "Grill a plan.")},
			want: `SKILL.md: name "grill" does not match the folder name "grilling"`},
		{name: "missing description", folder: "grilling", files: map[string]string{"SKILL.md": "---\nname: grilling\n---\nGo.\n"},
			want: "SKILL.md: description is missing"},
		{name: "long description", folder: "grilling", files: map[string]string{"SKILL.md": skillMD("grilling", strings.Repeat("é", 1025))},
			want: "SKILL.md: description is 1025 characters; at most 1024"},
		{name: "clashes with /model", folder: "model", files: map[string]string{"SKILL.md": skillMD("model", "Pick a model.")},
			want: `SKILL.md: name "model" clashes with Evie's built-in /model command`},
		{name: "clashes with /compact", folder: "compact", files: map[string]string{"SKILL.md": skillMD("compact", "Compact.")},
			want: `SKILL.md: name "compact" clashes with Evie's built-in /compact command`},
		{name: "SKILL.md over 32 KiB", folder: "grilling", files: map[string]string{"SKILL.md": valid + strings.Repeat("x", 32<<10)},
			want: fmt.Sprintf("SKILL.md: is %d bytes; each file is at most 32 KiB", len(valid)+32<<10)},
		{name: "reference over 32 KiB", folder: "grilling", files: map[string]string{"SKILL.md": valid, "references/big.md": strings.Repeat("x", 32<<10+1)},
			want: "references/big.md: is 32769 bytes; each file is at most 32 KiB"},
		{name: "more than 64 files", folder: "grilling", files: many, want: ".: has more than 64 files"},
		{name: "more than 1 MiB", folder: "grilling", files: large, want: ".: is more than 1024 KiB in total"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeSkillFolder(t, tc.folder, tc.files)
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			out, err := c.run("skills", "propose", dir)
			if err == nil || !strings.Contains(err.Error(), "nothing was proposed") {
				t.Fatalf("propose: %v\n%s", err, out)
			}
			mustContain(t, "refusal", out, "is not a valid skill:", tc.want)
			if tc.want != "SKILL.md: is missing" && strings.Contains(out, "SKILL.md: is missing") {
				t.Fatalf("a refused SKILL.md is also reported missing:\n%s", out)
			}
		})
	}
	if head, err := c.repo.Head(context.Background()); err != nil || head != "" {
		t.Fatalf("a refused skill reached the procedural repository: head %q, %v", head, err)
	}
	mustContain(t, "list", c.mustRun("skills", "list"), "No skills are active or pending")
}

// A skill folder that is itself a link (as ~/.claude/skills entries can
// be) is followed to its target.
func TestSkillCLIProposeFollowsALinkedSkillFolder(t *testing.T) {
	c := newSkillCLI(t)
	target := writeSkillFolder(t, "grilling", map[string]string{"SKILL.md": grillingSkill})
	link := filepath.Join(t.TempDir(), "grilling")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	mustContain(t, "propose through a link", c.mustRun("skills", "propose", link), "Proposed skill grilling", "Source: "+link)
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// The four skill tables refuse updates and deletes, and accept only the
// owner on the CLI surface, so no other surface can propose, approve or
// deactivate a skill even by writing the database.
func TestSkillRecordsAreImmutableAndCLIOnly(t *testing.T) {
	ctx := context.Background()
	c := newSkillCLI(t)
	dir := writeSkillFolder(t, "grilling", map[string]string{"SKILL.md": grillingSkill})
	first := versionHashLine.FindStringSubmatch(c.mustRun("skills", "propose", dir))[1]
	c.mustRun("skills", "approve", "grilling", "--hash", first, "--expect-version", "0")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(grillingSkill+"Then summarize.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := versionHashLine.FindStringSubmatch(c.mustRun("skills", "propose", dir))[1]
	c.mustRun("skills", "approve", "grilling", "--hash", second, "--expect-version", "1")
	c.mustRun("skills", "deactivate", "grilling")
	if _, err := c.db.ExecContext(ctx, `INSERT INTO skill_imports(id,report,actor,surface,imported_at) VALUES('import-1','{}','owner','cli','2026-11-03T00:00:00Z')`); err != nil {
		t.Fatalf("the owner's CLI import was refused: %v", err)
	}

	refused := func(what, statement, want string) {
		t.Helper()
		if _, err := c.db.ExecContext(ctx, statement); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v (want %q)", what, err, want)
		}
	}
	for _, table := range []string{"skill_imports", "skill_proposals", "skill_activations", "skill_deactivations"} {
		refused(table+" update", `UPDATE `+table+` SET surface = 'cli'`, "immutable")
		refused(table+" delete", `DELETE FROM `+table, "immutable")
	}
	const check = "CHECK constraint failed"
	refused("web import", `INSERT INTO skill_imports(id,report,actor,surface,imported_at) VALUES('import-2','{}','owner','web','2026-11-03T00:00:00Z')`, check)
	refused("model import", `INSERT INTO skill_imports(id,report,actor,surface,imported_at) VALUES('import-3','{}','evie','cli','2026-11-03T00:00:00Z')`, check)
	refused("web proposal", `INSERT INTO skill_proposals SELECT 'p-web',import_id,name,commit_id,tree_path,version_hash,manifest,metadata,source,warnings,actor,'web',proposed_at FROM skill_proposals LIMIT 1`, check)
	refused("tool proposal", `INSERT INTO skill_proposals SELECT 'p-tool',import_id,name,commit_id,tree_path,version_hash,manifest,metadata,source,warnings,actor,'tool',proposed_at FROM skill_proposals LIMIT 1`, check)
	refused("model proposal", `INSERT INTO skill_proposals SELECT 'p-evie',import_id,name,commit_id,tree_path,version_hash,manifest,metadata,source,warnings,'evie',surface,proposed_at FROM skill_proposals LIMIT 1`, check)
	refused("web activation", `INSERT INTO skill_activations SELECT name,3,proposal_id,commit_id,tree_path,version_hash,manifest,metadata,actor,'web',approved_at FROM skill_activations WHERE version = 2`, check)
	refused("model activation", `INSERT INTO skill_activations SELECT name,3,proposal_id,commit_id,tree_path,version_hash,manifest,metadata,'evie',surface,approved_at FROM skill_activations WHERE version = 2`, check)
	refused("activation unlike its proposal", `INSERT INTO skill_activations SELECT name,3,proposal_id,commit_id,tree_path,'`+strings.Repeat("0", 64)+`',manifest,metadata,actor,surface,approved_at FROM skill_activations WHERE version = 2`, "copies its proposal exactly")
	refused("web deactivation", `INSERT INTO skill_deactivations(name,version,actor,surface,deactivated_at) VALUES('grilling',1,'owner','web','2026-11-03T00:00:00Z')`, check)
	refused("model deactivation", `INSERT INTO skill_deactivations(name,version,actor,surface,deactivated_at) VALUES('grilling',1,'evie','cli','2026-11-03T00:00:00Z')`, check)

	// The store refuses the same outside the owner's CLI.
	if _, err := c.store.ActivateSkill(ctx, eviedb.SkillApproval{Name: "grilling", VersionHash: second, Actor: "owner", Surface: "web"}); !errors.Is(err, eviedb.ErrSkillOwnerOnly) {
		t.Fatalf("a web approval: %v", err)
	}
	if _, err := c.store.DeactivateSkill(ctx, "grilling", "evie", "cli"); !errors.Is(err, eviedb.ErrSkillOwnerOnly) {
		t.Fatalf("a model deactivation: %v", err)
	}
}

// Only the owner proposes, approves and deactivates skills: no
// model-facing tool of the kernel or of any compiled plugin proposes,
// approves, activates or deactivates anything, and the skills plugin
// exposes exactly the read-only skill tool, which takes name and path.
// Every plugin Evie enables by default must be listed here, so the skills
// plugin cannot be compiled into Evie without this test checking it.
func TestSkillApprovalHasNoModelTool(t *testing.T) {
	const skillsPluginID = plugins.PluginID("skills") // the first-party skills plugin of skills.spec.md
	store := backgroundTestStore(t)
	policy, err := configuredSubagentPolicy(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := subagents.New(store, policy)
	if err != nil {
		t.Fatal(err)
	}
	compiled := []plugins.Plugin{
		plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store), plugins.NewMemory(store),
		plugins.NewSubagents(supervisor), plugins.NewWorkflows(&plugins.WorkflowLink{}), plugins.NewGitHub(nil),
		plugins.NewMCP(&plugins.MCPLink{}), plugins.NewWorkspaceTabs(store), plugins.NewSkills(store, procedural.New(t.TempDir())),
	}
	checked := map[plugins.PluginID]plugins.Plugin{}
	for _, p := range compiled {
		checked[p.Manifest().ID] = p
	}
	for id := range defaultPluginEnabled {
		if _, ok := checked[id]; !ok {
			t.Fatalf("plugin %s is enabled by default but this test does not check its tools; add it to compiled", id)
		}
	}

	type modelTool struct {
		plugin           plugins.PluginID
		name, capability string
		parameters       []string
	}
	var all []modelTool
	for _, schema := range tools.KernelToolset().Schemas() {
		all = append(all, modelTool{plugin: "kernel", name: schema.Function.Name})
	}
	for _, p := range compiled {
		provider, ok := p.(plugins.ToolProvider)
		if !ok {
			continue
		}
		for _, capability := range provider.ToolCapabilities() {
			var parameters []string
			for name := range capability.Tool.Schema.Function.Parameters.Properties {
				parameters = append(parameters, name)
			}
			slices.Sort(parameters)
			all = append(all, modelTool{plugin: p.Manifest().ID, name: capability.Tool.Schema.Function.Name, capability: string(capability.ID), parameters: parameters})
		}
	}
	if len(all) < 10 {
		t.Fatalf("only %d tools found", len(all))
	}
	// notSkills are the model-facing tools whose names hold one of the
	// verbs but reach no skill record, each with what it does.
	notSkills := map[string]string{
		"propose_build_merge": "proposes a Build worker's change for the owner's merge review",
	}
	var skillTools []modelTool
	for _, tool := range all {
		text := strings.ToLower(tool.name + " " + tool.capability)
		for _, verb := range []string{"propose", "approve", "activate"} { // activate also covers deactivate
			if _, other := notSkills[tool.name]; !other && strings.Contains(text, verb) {
				t.Fatalf("model-facing tool %s (%s, plugin %s) can %s", tool.name, tool.capability, tool.plugin, verb)
			}
		}
		if tool.plugin == skillsPluginID || strings.Contains(text, "skill") {
			skillTools = append(skillTools, tool)
		}
	}
	for _, tool := range skillTools {
		if tool.plugin != skillsPluginID || tool.name != "skill" || !slices.Equal(tool.parameters, []string{"name", "path"}) {
			t.Fatalf("skill tool %+v; only the skills plugin's skill tool, taking name and path, reaches skills", tool)
		}
	}
	if _, ok := checked[skillsPluginID]; ok && len(skillTools) != 1 {
		t.Fatalf("the skills plugin exposes %d tools; want exactly skill", len(skillTools))
	}
}

func TestSkillCLIUsage(t *testing.T) {
	c := newSkillCLI(t)
	var out bytes.Buffer
	if handled, err := runSkillManagement(context.Background(), []string{"workflows", "list"}, &out, skillCommands{store: c.store, repo: c.repo}); handled || err != nil {
		t.Fatalf("another command: handled %v, err %v", handled, err)
	}
	for _, args := range [][]string{
		{"skills"},
		{"skills", "import"},
		{"skills", "propose"},
		{"skills", "propose", "a", "b"},
		{"skills", "propose", "--json", "a"},
		{"skills", "list", "extra"},
		{"skills", "show"},
		{"skills", "show", "grilling", "--proposal"},
		{"skills", "deactivate"},
		{"skills", "deactivate", "grilling", "--version", "1"},
	} {
		if _, err := c.run(args...); err == nil || !strings.Contains(err.Error(), "usage: evie skills") {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if _, err := c.run("skills", "approve", "grilling", "--hash", "sha256:abc", "--expect-version", "0"); err == nil || !strings.Contains(err.Error(), "not a version hash") {
		t.Fatalf("a malformed hash: %v", err)
	}
	if _, err := c.run("skills", "show", "grilling"); !errors.Is(err, eviedb.ErrSkillNotFound) {
		t.Fatalf("show of an unknown skill: %v", err)
	}
}
