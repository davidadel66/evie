package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/skill"
)

const skillsUsage = "usage: evie skills propose <folder> | evie skills import-claude [--plugins] | evie skills list" +
	" | evie skills show <name> [--proposal <id>] | evie skills approve <name> --hash <version-hash> --expect-version <n>" +
	" | evie skills approve --import <id> | evie skills deactivate <name>"

// skillCommands are the owner's skill commands' store and procedural
// repository, and the home folder import-claude reads.
type skillCommands struct {
	store *eviedb.Store
	repo  *procedural.Repo
	home  string
}

// runSkillManagement answers the owner's skill commands
// (cmd/evie/docs/active/skills.spec.md): propose checks a skill folder,
// commits it to procedural Git under global/skills/<name> and records a
// proposal; import-claude proposes David's Claude Code skills as one
// recorded import; list shows active skills and pending proposals; show
// prints a proposal's text, files, hash, warnings and diff against the
// active version; approve activates exactly the reviewed hash against the
// reviewed active version, or every proposal of one import at the hash its
// report showed; deactivate ends the active version. Approval is the
// owner's only: these records accept only the CLI surface, and no model
// tool reaches them.
func runSkillManagement(ctx context.Context, args []string, out io.Writer, c skillCommands) (bool, error) {
	if len(args) == 0 || args[0] != "skills" {
		return false, nil
	}
	usage := errors.New(skillsUsage)
	if len(args) < 2 {
		return true, usage
	}
	switch args[1] {
	case "propose":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || a.json || len(a.positional) != 1 {
			return true, usage
		}
		return true, c.propose(ctx, out, a.positional[0])
	case "import-claude":
		switch {
		case len(args) == 2:
			return true, c.importClaude(ctx, out, false)
		case len(args) == 3 && args[2] == "--plugins":
			return true, c.importClaude(ctx, out, true)
		}
		return true, usage
	case "list":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || a.json || len(a.positional) != 0 {
			return true, usage
		}
		return true, c.list(ctx, out)
	case "show":
		a, ok := parseWorkflowArgs(args[2:], "proposal")
		if !ok || a.json || len(a.positional) != 1 {
			return true, usage
		}
		return true, c.show(ctx, out, a.positional[0], a.flags["proposal"])
	case "approve":
		a, ok := parseWorkflowArgs(args[2:], "hash", "expect-version", "import")
		id, byImport := a.flags["import"]
		switch {
		case !ok || a.json:
			return true, usage
		case byImport && len(a.positional) == 0 && len(a.flags) == 1:
			return true, c.approveImport(ctx, out, id)
		case byImport || len(a.positional) != 1 || len(a.flags) != 2:
			return true, usage
		}
		expected, err := strconv.ParseInt(a.flags["expect-version"], 10, 64)
		if err != nil || expected < 0 {
			return true, usage
		}
		if !skill.ValidHash(a.flags["hash"]) {
			return true, fmt.Errorf("--hash %q is not a version hash (64 lowercase hex digits, as evie skills show prints it)", a.flags["hash"])
		}
		activation, err := c.store.ActivateSkill(ctx, eviedb.SkillApproval{Name: a.positional[0], VersionHash: a.flags["hash"],
			ExpectedVersion: expected, Actor: "owner", Surface: "cli"})
		if errors.Is(err, eviedb.ErrSkillHashMismatch) || errors.Is(err, eviedb.ErrSkillChanged) {
			return true, fmt.Errorf("%w; review it again with evie skills show %s", err, a.positional[0])
		}
		if err != nil {
			return true, err
		}
		_, err = fmt.Fprintf(out, "Activated skill %s version %d (version hash %s).\n", activation.Name, activation.Version, activation.VersionHash)
		return true, err
	case "deactivate":
		a, ok := parseWorkflowArgs(args[2:])
		if !ok || a.json || len(a.positional) != 1 {
			return true, usage
		}
		activation, err := c.store.DeactivateSkill(ctx, a.positional[0], "owner", "cli")
		if err != nil {
			return true, err
		}
		_, err = fmt.Fprintf(out, "Deactivated skill %s version %d. No version of %s is active; approve a proposal to activate one.\n",
			activation.Name, activation.Version, activation.Name)
		return true, err
	}
	return true, usage
}

// propose checks the folder, commits exactly its files, reads them back
// from the commit (they must check and hash the same) and records the
// proposal. A refused folder never reaches the procedural repository.
func (c skillCommands) propose(ctx context.Context, out io.Writer, folder string) error {
	abs, err := filepath.Abs(folder)
	if err != nil {
		return err
	}
	local, err := skill.ReadFolder(abs)
	var invalid *skill.InvalidError
	if errors.As(err, &invalid) {
		fmt.Fprintf(out, "%s is not a valid skill:\n", folder)
		for _, p := range invalid.Problems {
			fmt.Fprintf(out, "  %s: %s\n", p.Path, p.Message)
		}
		return fmt.Errorf("%s is invalid (%d problems); nothing was proposed", folder, len(invalid.Problems))
	}
	if err != nil {
		return err
	}
	// A version that is already pending or active is not proposed again,
	// as the migration skips it: a new proposal would supersede a pending
	// one, and an import's approval with it.
	standing, err := c.standing(ctx)
	if err != nil {
		return err
	}
	if a, ok := standing.active[local.Name]; ok && a.VersionHash == local.Hash {
		_, err := fmt.Fprintf(out, "Skill %s is already active as version %d (version hash %s); nothing was proposed.\n", local.Name, a.Version, local.Hash)
		return err
	}
	if p, ok := standing.pending[local.Name]; ok && p.VersionHash == local.Hash {
		_, err := fmt.Fprintf(out, "Skill %s is already pending as proposal %s (version hash %s); nothing was proposed. Review it with: evie skills show %s\n",
			local.Name, p.ID, local.Hash, local.Name)
		return err
	}
	committed, commit, err := c.commitSkill(ctx, local, abs)
	if err != nil {
		return err
	}
	p, err := c.store.RecordSkillProposal(ctx, eviedb.SkillProposal{Name: committed.Name, Commit: commit, Path: eviedb.GlobalSkillPath(committed.Name),
		VersionHash: committed.Hash, Manifest: committed.Manifest, Metadata: committed.Metadata,
		Source:   eviedb.SkillSource{Kind: eviedb.SkillSourceFolder, Path: abs},
		Warnings: committed.Warnings, Actor: "owner", Surface: "cli"})
	if err != nil {
		return err
	}
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Proposed skill %s as proposal %s.", p.Name, p.ID)
	line("Version hash: %s", p.VersionHash)
	line("Description: %s", p.Metadata.Description)
	line("Source: %s", p.Source.Path)
	skillFilesText(&b, p.Manifest)
	skillWarningsText(&b, p.Warnings)
	if p.Pending {
		line("Nothing is active until you approve it. Review it with: evie skills show %s", p.Name)
	} else {
		line("It is the active version of %s; there is nothing to approve.", p.Name)
	}
	_, err = io.WriteString(out, b.String())
	return err
}

// commitSkill commits exactly a checked skill's files to procedural Git at
// global/skills/<name> and reads them back from the commit, where they
// must check and hash the same.
func (c skillCommands) commitSkill(ctx context.Context, local skill.Version, source string) (skill.Version, string, error) {
	path := eviedb.GlobalSkillPath(local.Name)
	message := fmt.Sprintf("Propose skill %s\n\nVersion-Hash: %s\nSource: %s\nProposed-By: owner (cli)\n", local.Name, local.Hash, source)
	commit, _, err := c.repo.Commit(ctx, path, local.Files, message)
	if err != nil {
		return skill.Version{}, "", err
	}
	committed, err := skill.ReadVersion(local.Name, c.repo.Source(ctx, commit, path), local.Manifest)
	if err != nil {
		return skill.Version{}, "", fmt.Errorf("the committed skill did not read back: %w", err)
	}
	// The committed files are exactly the checked ones, so their warnings
	// are too, and the folder's say what its walk skipped.
	committed.Warnings = local.Warnings
	return committed, commit, nil
}

// skillStanding is each name's active version and pending proposal: a
// folder whose version is one of them is not proposed again.
type skillStanding struct {
	active  map[string]eviedb.SkillActivation
	pending map[string]eviedb.SkillProposal
}

func (c skillCommands) standing(ctx context.Context) (skillStanding, error) {
	s := skillStanding{active: map[string]eviedb.SkillActivation{}, pending: map[string]eviedb.SkillProposal{}}
	active, err := c.store.ActiveSkills(ctx)
	if err != nil {
		return s, err
	}
	for _, a := range active {
		s.active[a.Name] = a
	}
	pending, err := c.store.PendingSkillProposals(ctx)
	if err != nil {
		return s, err
	}
	for _, p := range pending {
		s.pending[p.Name] = p
	}
	return s, nil
}

func skillFilesText(b *strings.Builder, m skill.Manifest) {
	b.WriteString("Files:\n")
	for _, f := range m.Files {
		fmt.Fprintf(b, "  %s (%d bytes)\n", f.Path, f.Bytes)
	}
}

func skillWarningsText(b *strings.Builder, warnings []skill.Warning) {
	if len(warnings) == 0 {
		b.WriteString("Warnings: none\n")
		return
	}
	b.WriteString("Warnings:\n")
	for _, w := range warnings {
		fmt.Fprintf(b, "  - %s\n", w.Message)
	}
}

func shortVersionHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

func (c skillCommands) list(ctx context.Context, out io.Writer) error {
	active, err := c.store.ActiveSkills(ctx)
	if err != nil {
		return err
	}
	pending, err := c.store.PendingSkillProposals(ctx)
	if err != nil {
		return err
	}
	var b strings.Builder
	if len(active) == 0 && len(pending) == 0 {
		b.WriteString("No skills are active or pending. Propose one with evie skills propose <folder>.\n")
	}
	if len(active) > 0 {
		b.WriteString("Active skills\n")
	}
	for _, a := range active {
		owner := ""
		if a.Metadata.DisableModelInvocation {
			owner = "  (owner-only)"
		}
		fmt.Fprintf(&b, "  %s  version %d  %s%s  %s\n", a.Name, a.Version, shortVersionHash(a.VersionHash), owner, a.Metadata.Description)
	}
	if len(pending) > 0 {
		b.WriteString("Pending proposals\n")
	}
	for _, p := range pending {
		fmt.Fprintf(&b, "  %s  %s  proposal %s  (evie skills show %s)\n", p.Name, shortVersionHash(p.VersionHash), p.ID, p.Name)
	}
	_, err = io.WriteString(out, b.String())
	return err
}

// show prints one proposal of name (its newest, or proposalID) for review:
// the name's status, the proposal's state, hash, files, warnings, every
// file's text as committed, and the diff against the active version.
func (c skillCommands) show(ctx context.Context, out io.Writer, name, proposalID string) error {
	state, err := c.store.SkillState(ctx, name)
	if err != nil {
		return err
	}
	p := state.Proposals[0]
	if proposalID != "" {
		if p, err = c.store.SkillProposal(ctx, proposalID); err != nil {
			return err
		}
		if p.Name != name {
			return fmt.Errorf("proposal %s is of skill %s, not %s", p.ID, p.Name, name)
		}
	}
	version, err := skill.ReadVersion(p.Name, c.repo.Source(ctx, p.Commit, p.Path), p.Manifest)
	if err != nil {
		return fmt.Errorf("read proposal %s from the procedural repository: %w", p.ID, err)
	}
	active, isActive := state.Active()
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	line("Skill %s", name)
	switch {
	case isActive:
		line("Status: active, version %d (version hash %s, approved by %s via %s at %s)", active.Version, active.VersionHash,
			active.Actor, active.Surface, active.ApprovedAt.Format("2006-01-02 15:04:05Z07:00"))
	case len(state.Versions) > 0:
		d := state.Versions[0].Deactivated
		line("Status: not active (version %d was deactivated by %s via %s at %s)", state.Versions[0].Version, d.Actor, d.Surface,
			d.At.Format("2006-01-02 15:04:05Z07:00"))
	default:
		line("Status: not active")
	}
	proposalState := "superseded by a newer proposal without approval"
	switch {
	case len(p.ApprovedVersions) > 0:
		versions := make([]string, len(p.ApprovedVersions))
		for i, v := range p.ApprovedVersions {
			versions[i] = strconv.FormatInt(v, 10)
		}
		proposalState = "approved as version " + strings.Join(versions, ", ")
	case p.Pending:
		proposalState = "pending approval"
	case isActive && p.VersionHash == active.VersionHash:
		proposalState = "the same as the active version; nothing to approve"
	}
	line("Proposal %s, proposed by %s via %s at %s: %s", p.ID, p.Actor, p.Surface, p.ProposedAt.Format("2006-01-02 15:04:05Z07:00"), proposalState)
	line("Source: %s %s", p.Source.Kind, p.Source.Path)
	line("Version hash: %s", p.VersionHash)
	line("Description: %s", p.Metadata.Description)
	if p.Metadata.ArgumentHint != "" {
		line("Argument hint: %s", p.Metadata.ArgumentHint)
	}
	if p.Metadata.License != "" {
		line("License: %s", p.Metadata.License)
	}
	if p.Metadata.DisableModelInvocation {
		line("Owner-only: invoked with /%s, never listed for Evie or loaded by Evie's tool", name)
	}
	skillFilesText(&b, p.Manifest)
	skillWarningsText(&b, p.Warnings)
	for _, f := range p.Manifest.Files {
		line("===== %s =====", f.Path)
		text := string(version.Files[f.Path])
		b.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			b.WriteString("\n")
		}
	}
	switch {
	case !isActive:
		line("===== No active version to compare with; the files above are the whole change. =====")
	default:
		diff, err := c.repo.Diff(ctx, active.Commit, p.Commit, p.Path)
		if err != nil {
			return err
		}
		if diff == "" {
			line("===== No difference from active version %d. =====", active.Version)
		} else {
			line("===== Diff against active version %d =====", active.Version)
			b.WriteString(diff)
		}
	}
	if p.Pending {
		expected := int64(0)
		if isActive {
			expected = active.Version
		}
		line("To approve: evie skills approve %s --hash %s --expect-version %d", name, p.VersionHash, expected)
	}
	_, err = io.WriteString(out, b.String())
	return err
}
