package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/skill"
)

// foundSkill is one skill folder the migration found: a folder holding a
// SKILL.md, named by its last path element, and where it came from.
type foundSkill struct {
	name   string
	dir    string
	source eviedb.SkillSource
}

// claudeSkills lists the skill folders of David's Claude Code setup under
// home, in the migration's precedence (skills.spec.md): personal skills in
// ~/.claude/skills, the synced skills under ~/.claude/skills/synced/*/,
// ~/.agents/skills, then, with plugins, the installed plugins' skills.
// Notes say what could not be read.
func claudeSkills(home string, plugins bool) ([]foundSkill, []string) {
	var found []foundSkill
	var notes []string
	add := func(dir string, source eviedb.SkillSource) {
		f, n := skillFolders(home, dir, source)
		found, notes = append(found, f...), append(notes, n...)
	}
	claude := filepath.Join(home, ".claude", "skills")
	add(claude, eviedb.SkillSource{Kind: eviedb.SkillSourceClaudePersonal})
	synced := filepath.Join(claude, "synced")
	buckets, n := listFolder(home, synced)
	notes = append(notes, n...)
	for _, bucket := range buckets {
		if info, err := os.Stat(filepath.Join(synced, bucket)); err == nil && info.IsDir() {
			add(filepath.Join(synced, bucket), eviedb.SkillSource{Kind: eviedb.SkillSourceClaudeSynced})
		}
	}
	add(filepath.Join(home, ".agents", "skills"), eviedb.SkillSource{Kind: eviedb.SkillSourceAgents})
	if plugins {
		installs, n := pluginInstalls(home)
		notes = append(notes, n...)
		for _, p := range installs {
			add(filepath.Join(p.InstallPath, "skills"), eviedb.SkillSource{Kind: eviedb.SkillSourceClaudePlugin, Plugin: p.name, PluginVersion: p.Version})
		}
	}
	return found, notes
}

// pluginInstall is one installed Claude Code plugin, as
// installed_plugins.json records it.
type pluginInstall struct {
	name        string
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
}

// pluginInstalls reads the installed Claude Code plugins, by plugin name,
// from ~/.claude/plugins/installed_plugins.json: {"plugins": {"<name>":
// [install records]}}, where an install record has the plugin's
// installPath and version (an older form has one record per plugin). The
// file's format is Claude Code's own and undocumented, so anything else is
// reported in the notes and skipped.
func pluginInstalls(home string) ([]pluginInstall, []string) {
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	shown := tildePath(home, path)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, []string{"found no " + shown + "; no plugin skills were read"}
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s: %v; no plugin skills were read", shown, err)}
	}
	var file struct {
		Plugins map[string]json.RawMessage `json:"plugins"`
	}
	if err := json.Unmarshal(data, &file); err != nil || file.Plugins == nil {
		return nil, []string{shown + ` is not the expected JSON ({"plugins": {...}}); no plugin skills were read`}
	}
	names := make([]string, 0, len(file.Plugins))
	for name := range file.Plugins {
		names = append(names, name)
	}
	sort.Strings(names)
	var installs []pluginInstall
	var notes []string
	for _, name := range names {
		var records []pluginInstall
		if err := json.Unmarshal(file.Plugins[name], &records); err != nil {
			var one pluginInstall
			if err := json.Unmarshal(file.Plugins[name], &one); err != nil {
				notes = append(notes, fmt.Sprintf("plugin %s in %s is not an install record or a list of them; its skills were not read", name, shown))
				continue
			}
			records = []pluginInstall{one}
		}
		for _, r := range records {
			if !filepath.IsAbs(r.InstallPath) {
				notes = append(notes, fmt.Sprintf("plugin %s in %s has no absolute install path; its skills were not read", name, shown))
				continue
			}
			r.name = name
			installs = append(installs, r)
		}
	}
	return installs, notes
}

// listFolder lists dir's entries, by name, without hidden ones (such as
// the synced folder's .bucket-* marker). A missing dir has none.
func listFolder(home, dir string) ([]string, []string) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s: %v", tildePath(home, dir), err)}
	}
	var names []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// skillFolders lists the skill folders in dir: each folder, or link to a
// folder, that holds a SKILL.md, from source. Files such as the synced
// buckets' manifest.json are not skills.
func skillFolders(home, dir string, source eviedb.SkillSource) ([]foundSkill, []string) {
	names, notes := listFolder(home, dir)
	var found []foundSkill
	for _, name := range names {
		folder := filepath.Join(dir, name)
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Lstat(filepath.Join(folder, skill.InstructionsFile)); err != nil {
			continue
		}
		s := source
		s.Path = folder
		found = append(found, foundSkill{name: name, dir: folder, source: s})
	}
	return found, notes
}

// tildePath shows p under home as ~/...
func tildePath(home, p string) string {
	if rel, err := filepath.Rel(home, p); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return p
}

// sourceText names where a skill came from, for the owner.
func sourceText(home string, s eviedb.SkillSource) string {
	label := s.Kind
	switch s.Kind {
	case eviedb.SkillSourceClaudePersonal:
		label = "Claude personal"
	case eviedb.SkillSourceClaudeSynced:
		label = "Claude synced"
	case eviedb.SkillSourceAgents:
		label = "~/.agents"
	case eviedb.SkillSourceClaudePlugin:
		version := s.PluginVersion
		if version == "" {
			version = "version unknown"
		}
		label = "plugin " + s.Plugin + " " + version
	}
	return label + " (" + tildePath(home, s.Path) + ")"
}

// importClaude migrates David's Claude Code skills: each name found is
// proposed once, from its first copy in precedence order; later copies are
// reported as shadowed, refused folders with their problems, and a skill
// whose version is already active or pending as unchanged. The import and
// its proposals are recorded together, and nothing is activated.
func (c skillCommands) importClaude(ctx context.Context, out io.Writer, plugins bool) error {
	if c.home == "" {
		return errors.New("no home folder to import Claude Code skills from (HOME is not set)")
	}
	found, notes := claudeSkills(c.home, plugins)
	active, err := c.store.ActiveSkills(ctx)
	if err != nil {
		return err
	}
	activeByName := map[string]eviedb.SkillActivation{}
	for _, a := range active {
		activeByName[a.Name] = a
	}
	pending, err := c.store.PendingSkillProposals(ctx)
	if err != nil {
		return err
	}
	pendingByName := map[string]eviedb.SkillProposal{}
	for _, p := range pending {
		pendingByName[p.Name] = p
	}

	report := eviedb.SkillImportReport{Plugins: plugins, Notes: notes}
	var proposals []eviedb.SkillProposal
	winners := map[string]eviedb.SkillSource{}
	for _, f := range found {
		if winner, ok := winners[f.name]; ok {
			report.Shadowed = append(report.Shadowed, eviedb.SkillImportEntry{Name: f.name, Source: f.source, ShadowedBy: &winner})
			continue
		}
		winners[f.name] = f.source
		local, err := skill.ReadFolder(f.dir)
		var invalid *skill.InvalidError
		switch {
		case errors.As(err, &invalid):
			report.Refused = append(report.Refused, eviedb.SkillImportEntry{Name: f.name, Source: f.source, Problems: invalid.Problems})
			continue
		case err != nil:
			report.Refused = append(report.Refused, eviedb.SkillImportEntry{Name: f.name, Source: f.source,
				Problems: []skill.Problem{{Path: ".", Message: err.Error()}}})
			continue
		}
		if a, ok := activeByName[local.Name]; ok && a.VersionHash == local.Hash {
			report.Unchanged = append(report.Unchanged, eviedb.SkillImportEntry{Name: f.name, Source: f.source,
				VersionHash: local.Hash, SameAs: fmt.Sprintf("active version %d", a.Version)})
			continue
		}
		if p, ok := pendingByName[local.Name]; ok && p.VersionHash == local.Hash {
			report.Unchanged = append(report.Unchanged, eviedb.SkillImportEntry{Name: f.name, Source: f.source,
				VersionHash: local.Hash, SameAs: "pending proposal " + p.ID})
			continue
		}
		committed, commit, err := c.commitSkill(ctx, local, sourceText(c.home, f.source))
		if err != nil {
			return fmt.Errorf("propose %s from %s: %w", f.name, f.dir, err)
		}
		proposals = append(proposals, eviedb.SkillProposal{Name: committed.Name, Commit: commit, Path: eviedb.GlobalSkillPath(committed.Name),
			VersionHash: committed.Hash, Manifest: committed.Manifest, Metadata: committed.Metadata, Source: f.source,
			Warnings: committed.Warnings, Actor: "owner", Surface: "cli"})
	}
	imp, err := c.store.RecordSkillImport(ctx, eviedb.SkillImport{Report: report, Proposals: proposals, Actor: "owner", Surface: "cli"})
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, c.importReportText(imp))
	return err
}

// importReportText is the import's report for the owner.
func (c skillCommands) importReportText(imp eviedb.SkillImport) string {
	r := imp.Report
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	read := "~/.claude/skills, ~/.claude/skills/synced, ~/.agents/skills"
	if r.Plugins {
		read += " and installed Claude Code plugins"
	}
	line("Import %s of Claude Code skills from %s.", imp.ID, read)
	if len(r.Proposed) > 0 {
		line("Proposed (%d):", len(r.Proposed))
	}
	for _, e := range r.Proposed {
		line("  %s  %s  from %s", e.Name, e.VersionHash, sourceText(c.home, e.Source))
		if len(e.Warnings) == 0 {
			line("    Warnings: none")
		}
		for _, w := range e.Warnings {
			line("    Warning: %s", w.Message)
		}
	}
	if len(r.Refused) > 0 {
		line("Refused (%d):", len(r.Refused))
	}
	for _, e := range r.Refused {
		line("  %s  from %s", e.Name, sourceText(c.home, e.Source))
		for _, p := range e.Problems {
			line("    %s: %s", p.Path, p.Message)
		}
	}
	if len(r.Shadowed) > 0 {
		line("Shadowed (%d):", len(r.Shadowed))
	}
	for _, e := range r.Shadowed {
		line("  %s  from %s, shadowed by %s", e.Name, sourceText(c.home, e.Source), sourceText(c.home, *e.ShadowedBy))
	}
	if len(r.Unchanged) > 0 {
		line("Unchanged (%d):", len(r.Unchanged))
	}
	for _, e := range r.Unchanged {
		line("  %s  from %s: the same as its %s", e.Name, sourceText(c.home, e.Source), e.SameAs)
	}
	if len(r.Notes) > 0 {
		line("Notes (%d):", len(r.Notes))
	}
	for _, n := range r.Notes {
		line("  %s", n)
	}
	if len(r.Proposed) == 0 {
		line("Nothing new was proposed; there is nothing to approve.")
		return b.String()
	}
	line("Nothing is active until you approve it. Review each with evie skills show <name>, then approve all %d at the hashes above with:", len(r.Proposed))
	line("  evie skills approve --import %s", imp.ID)
	return b.String()
}

// approveImport activates every proposal of one import at the version hash
// its report showed, each through the same per-skill approval as approve
// <name>: the proposal must still be its name's pending proposal, and it
// is approved against the version active now (the hash guards the
// content). A superseded or no longer pending proposal is refused and
// reported, and the others are still activated; one whose version is
// already active is reported as such.
func (c skillCommands) approveImport(ctx context.Context, out io.Writer, id string) error {
	imp, err := c.store.SkillImport(ctx, id)
	if err != nil {
		return err
	}
	if len(imp.Proposals) == 0 {
		_, err := fmt.Fprintf(out, "Import %s proposed nothing; there is nothing to approve.\n", id)
		return err
	}
	active, err := c.store.ActiveSkills(ctx)
	if err != nil {
		return err
	}
	activeByName := map[string]eviedb.SkillActivation{}
	for _, a := range active {
		activeByName[a.Name] = a
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Approving the %d proposals of import %s, each at the version hash its report showed:", len(imp.Proposals), id)
	var activated, already, refused int
	for _, p := range imp.Proposals {
		current, isActive := activeByName[p.Name]
		if isActive && current.VersionHash == p.VersionHash {
			already++
			line("  Already active: %s version %d (version hash %s).", p.Name, current.Version, p.VersionHash)
			continue
		}
		a, err := c.store.ActivateSkill(ctx, eviedb.SkillApproval{Name: p.Name, VersionHash: p.VersionHash, ProposalID: p.ID,
			ExpectedVersion: current.Version, Actor: "owner", Surface: "cli"})
		switch {
		case errors.Is(err, eviedb.ErrSkillSuperseded) || errors.Is(err, eviedb.ErrSkillNoPending) ||
			errors.Is(err, eviedb.ErrSkillHashMismatch) || errors.Is(err, eviedb.ErrSkillChanged):
			refused++
			line("  Refused %s: %v; review it with evie skills show %s", p.Name, err, p.Name)
		case err != nil:
			return err
		default:
			activated++
			line("  Activated %s version %d (version hash %s).", a.Name, a.Version, a.VersionHash)
		}
	}
	line("Activated %d, already active %d, refused %d.", activated, already, refused)
	if _, err := io.WriteString(out, b.String()); err != nil {
		return err
	}
	if refused > 0 {
		return fmt.Errorf("%d of %d proposals of import %s were refused; the others are active", refused, len(imp.Proposals), id)
	}
	return nil
}
