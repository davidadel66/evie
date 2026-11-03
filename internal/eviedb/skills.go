package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/skill"
	"github.com/google/uuid"
)

// Skills (cmd/evie/docs/active/skills.spec.md). A skill's files live in the
// procedural repository at global/skills/<name>; SQLite records, immutably,
// what the owner did with them from the CLI:
//
//   - skill_imports: one migration run's report (import-claude), which its
//     proposals reference so they can be approved together;
//   - skill_proposals: one checked version of a skill, committed to
//     procedural Git, with its version hash, canonical manifest, recognised
//     frontmatter, source and warnings;
//   - skill_activations: the owner approving one proposal as the name's next
//     version, a copy of the proposal's commit, path, hash, manifest and
//     metadata;
//   - skill_deactivations: the owner ending an approved version.
//
// Every row records the owner as actor and the CLI as surface; no other
// value is accepted, so no chat tool can write one. A name's active version
// is its latest activation while it has no deactivation, so at most one
// version of a name is active. A name's pending proposal is its newest
// proposal while that proposal was never approved and differs from the
// active version; a newer proposal supersedes an older one.

// Skill source kinds: where a proposal's folder came from.
const (
	SkillSourceFolder         = "folder"
	SkillSourceClaudePersonal = "claude-personal"
	SkillSourceClaudeSynced   = "claude-synced"
	SkillSourceAgents         = "agents"
	SkillSourceClaudePlugin   = "claude-plugin"
)

var (
	// ErrSkillNotFound: no proposal or version matches.
	ErrSkillNotFound = errors.New("skill not found")
	// ErrSkillOwnerOnly refuses a record that is not the owner's, from the
	// CLI.
	ErrSkillOwnerOnly = errors.New("only the owner records skill proposals, approvals and deactivations, from the CLI")
	// ErrSkillNoPending: the name has no proposal waiting for approval.
	ErrSkillNoPending = errors.New("no pending proposal")
	// ErrSkillHashMismatch: the hash is not the pending proposal's.
	ErrSkillHashMismatch = errors.New("the version hash is not the pending proposal's")
	// ErrSkillChanged: the active version is not the one the owner
	// reviewed against.
	ErrSkillChanged = errors.New("the active version changed since review")
	// ErrSkillNotActive: the name has no active version.
	ErrSkillNotActive = errors.New("no active version")
)

// GlobalSkillPath is where a Global skill's files live in the procedural
// repository.
func GlobalSkillPath(name string) string { return "global/skills/" + name }

// SkillSource is where a proposal's folder came from: a plain folder, a
// Claude Code personal or synced skill, ~/.agents/skills, or a Claude Code
// plugin with its name and version.
type SkillSource struct {
	Kind          string `json:"kind"`
	Path          string `json:"path"`
	Plugin        string `json:"plugin,omitempty"`
	PluginVersion string `json:"plugin_version,omitempty"`
}

// SkillProposal is one checked version of a skill, committed to procedural
// Git. Pending and ApprovedVersions are read-side: what became of it.
type SkillProposal struct {
	ID          string          `json:"id"`
	ImportID    string          `json:"import_id,omitempty"`
	Name        string          `json:"name"`
	Commit      string          `json:"commit"`
	Path        string          `json:"path"`
	VersionHash string          `json:"version_hash"`
	Manifest    skill.Manifest  `json:"manifest"`
	Metadata    skill.Metadata  `json:"metadata"`
	Source      SkillSource     `json:"source"`
	Warnings    []skill.Warning `json:"warnings"`
	Actor       string          `json:"actor"`
	Surface     string          `json:"surface"`
	ProposedAt  time.Time       `json:"proposed_at"`

	Pending          bool    `json:"pending"`
	ApprovedVersions []int64 `json:"approved_versions,omitempty"`
}

// SkillActivation is one approved version of a skill: what the owner
// approved and where its files are.
type SkillActivation struct {
	Name        string             `json:"name"`
	Version     int64              `json:"version"`
	ProposalID  string             `json:"proposal_id"`
	Commit      string             `json:"commit"`
	Path        string             `json:"path"`
	VersionHash string             `json:"version_hash"`
	Manifest    skill.Manifest     `json:"manifest"`
	Metadata    skill.Metadata     `json:"metadata"`
	Actor       string             `json:"actor"`
	Surface     string             `json:"surface"`
	ApprovedAt  time.Time          `json:"approved_at"`
	Deactivated *SkillDeactivation `json:"deactivated,omitempty"`
}

// SkillDeactivation is the owner ending an approved version.
type SkillDeactivation struct {
	Actor   string    `json:"actor"`
	Surface string    `json:"surface"`
	At      time.Time `json:"at"`
}

// SkillApproval names exactly what the owner reviewed: the name, the
// pending proposal's version hash, and the active version it was compared
// with (0 when none).
type SkillApproval struct {
	Name            string
	VersionHash     string
	ExpectedVersion int64
	Actor           string
	Surface         string
}

// SkillState is one name's versions and proposals, newest first.
type SkillState struct {
	Name      string            `json:"name"`
	Versions  []SkillActivation `json:"versions"`
	Proposals []SkillProposal   `json:"proposals"`
}

// Active is the name's active version, if any.
func (s SkillState) Active() (SkillActivation, bool) {
	if len(s.Versions) == 0 || s.Versions[0].Deactivated != nil {
		return SkillActivation{}, false
	}
	return s.Versions[0], true
}

func ownerCLI(actor, surface string) bool { return actor == "owner" && surface == "cli" }

func ensureSkillSchema(ctx context.Context, db *sql.DB) error {
	immutable := func(table, what string) string {
		return fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS %[1]s_no_update BEFORE UPDATE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;
 CREATE TRIGGER IF NOT EXISTS %[1]s_no_delete BEFORE DELETE ON %[1]s
 BEGIN SELECT RAISE(ABORT,'%[2]s are immutable'); END;`, table, what)
	}
	const owner = `actor TEXT NOT NULL CHECK(actor = 'owner'),
 surface TEXT NOT NULL CHECK(surface = 'cli'),`
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS skill_imports (
 id TEXT PRIMARY KEY,
 report TEXT NOT NULL CHECK(json_valid(report)),
 `+owner+`
 imported_at TEXT NOT NULL);
 `+immutable("skill_imports", "skill imports")+`
 CREATE TABLE IF NOT EXISTS skill_proposals (
 id TEXT PRIMARY KEY,
 import_id TEXT REFERENCES skill_imports(id),
 name TEXT NOT NULL CHECK(length(name) BETWEEN 1 AND 64 AND name GLOB '[a-z]*' AND name NOT GLOB '*[^a-z0-9-]*'),
 commit_id TEXT NOT NULL CHECK(length(commit_id) IN (40,64) AND commit_id NOT GLOB '*[^0-9a-f]*'),
 tree_path TEXT NOT NULL CHECK(tree_path = 'global/skills/' || name),
 version_hash TEXT NOT NULL CHECK(length(version_hash) = 64 AND version_hash NOT GLOB '*[^0-9a-f]*'),
 manifest TEXT NOT NULL CHECK(json_valid(manifest) AND json_extract(manifest,'$.format') = '`+skill.ManifestFormat+`'),
 metadata TEXT NOT NULL CHECK(json_valid(metadata) AND json_type(metadata) = 'object'),
 source TEXT NOT NULL CHECK(json_valid(source) AND json_extract(source,'$.kind') IN ('folder','claude-personal','claude-synced','agents','claude-plugin')),
 warnings TEXT NOT NULL CHECK(json_valid(warnings) AND json_type(warnings) = 'array'),
 `+owner+`
 proposed_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS skill_proposals_by_name ON skill_proposals(name);
 CREATE INDEX IF NOT EXISTS skill_proposals_by_import ON skill_proposals(import_id) WHERE import_id IS NOT NULL;
 `+immutable("skill_proposals", "skill proposals")+`
 CREATE TABLE IF NOT EXISTS skill_activations (
 name TEXT NOT NULL,
 version INTEGER NOT NULL CHECK(version > 0),
 proposal_id TEXT NOT NULL REFERENCES skill_proposals(id),
 commit_id TEXT NOT NULL,
 tree_path TEXT NOT NULL,
 version_hash TEXT NOT NULL,
 manifest TEXT NOT NULL,
 metadata TEXT NOT NULL,
 `+owner+`
 approved_at TEXT NOT NULL,
 PRIMARY KEY(name, version));
 CREATE INDEX IF NOT EXISTS skill_activations_by_hash ON skill_activations(name, version_hash);
 CREATE TRIGGER IF NOT EXISTS skill_activations_copy_proposal BEFORE INSERT ON skill_activations
 WHEN NOT EXISTS(SELECT 1 FROM skill_proposals p WHERE p.id = NEW.proposal_id AND p.name = NEW.name AND p.commit_id = NEW.commit_id
 AND p.tree_path = NEW.tree_path AND p.version_hash = NEW.version_hash AND p.manifest = NEW.manifest AND p.metadata = NEW.metadata)
 BEGIN SELECT RAISE(ABORT,'a skill activation copies its proposal exactly'); END;
 `+immutable("skill_activations", "skill activations")+`
 CREATE TABLE IF NOT EXISTS skill_deactivations (
 name TEXT NOT NULL,
 version INTEGER NOT NULL,
 `+owner+`
 deactivated_at TEXT NOT NULL,
 PRIMARY KEY(name, version),
 FOREIGN KEY(name, version) REFERENCES skill_activations(name, version));
 `+immutable("skill_deactivations", "skill deactivations"))
	if err != nil {
		return fmt.Errorf("create skill schema: %w", err)
	}
	return nil
}

// RecordSkillProposal records the owner's proposal of a checked skill
// version whose files are at p.Commit, at GlobalSkillPath(p.Name), in
// procedural Git.
func (s *Store) RecordSkillProposal(ctx context.Context, p SkillProposal) (SkillProposal, error) {
	switch {
	case !ownerCLI(p.Actor, p.Surface):
		return SkillProposal{}, ErrSkillOwnerOnly
	case !skill.ValidName(p.Name):
		return SkillProposal{}, fmt.Errorf("record skill proposal: invalid name %q", p.Name)
	case p.Path != GlobalSkillPath(p.Name):
		return SkillProposal{}, fmt.Errorf("record skill proposal: path %q is not %s", p.Path, GlobalSkillPath(p.Name))
	case p.Manifest.Format != skill.ManifestFormat || p.Manifest.Hash() != p.VersionHash:
		return SkillProposal{}, fmt.Errorf("record skill proposal: version hash %s is not its manifest's", p.VersionHash)
	}
	if p.Warnings == nil {
		p.Warnings = []skill.Warning{}
	}
	manifest, metadata, err := encodeSkillVersion(p.Manifest, p.Metadata)
	if err != nil {
		return SkillProposal{}, err
	}
	source, err := json.Marshal(p.Source)
	if err != nil {
		return SkillProposal{}, err
	}
	warnings, err := json.Marshal(p.Warnings)
	if err != nil {
		return SkillProposal{}, err
	}
	var importID any
	if p.ImportID != "" {
		importID = p.ImportID
	}
	p.ID, p.ProposedAt, p.Pending, p.ApprovedVersions = uuid.NewString(), s.now().UTC(), false, nil
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `INSERT INTO skill_proposals(id,import_id,name,commit_id,tree_path,version_hash,manifest,metadata,source,warnings,actor,surface,proposed_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, importID, p.Name, p.Commit, p.Path, p.VersionHash, manifest, metadata, string(source), string(warnings),
			p.Actor, p.Surface, formatTime(p.ProposedAt)); err != nil {
			return fmt.Errorf("record skill proposal: %w", err)
		}
		p.Pending, err = isPendingSkillProposal(ctx, conn, p.ID)
		return err
	})
	if err != nil {
		return SkillProposal{}, err
	}
	return p, nil
}

func encodeSkillVersion(m skill.Manifest, md skill.Metadata) (string, string, error) {
	manifest, err := json.Marshal(m)
	if err != nil {
		return "", "", err
	}
	metadata, err := json.Marshal(md)
	if err != nil {
		return "", "", err
	}
	return string(manifest), string(metadata), nil
}

const skillProposalColumns = `p.id,p.import_id,p.name,p.commit_id,p.tree_path,p.version_hash,p.manifest,p.metadata,p.source,p.warnings,p.actor,p.surface,p.proposed_at`

// pendingSkillProposals selects each name's pending proposal: its newest,
// never approved, and not the active version's hash.
const pendingSkillProposals = ` FROM skill_proposals p
 WHERE p.rowid = (SELECT MAX(q.rowid) FROM skill_proposals q WHERE q.name = p.name)
 AND NOT EXISTS(SELECT 1 FROM skill_activations a WHERE a.proposal_id = p.id)
 AND NOT EXISTS(SELECT 1 FROM skill_activations a WHERE a.name = p.name AND a.version_hash = p.version_hash
 AND a.version = (SELECT MAX(version) FROM skill_activations WHERE name = p.name)
 AND NOT EXISTS(SELECT 1 FROM skill_deactivations d WHERE d.name = a.name AND d.version = a.version))`

func scanSkillProposal(scanner rowScanner) (SkillProposal, error) {
	var p SkillProposal
	var importID sql.NullString
	var manifest, metadata, source, warnings, proposed string
	if err := scanner.Scan(&p.ID, &importID, &p.Name, &p.Commit, &p.Path, &p.VersionHash, &manifest, &metadata, &source, &warnings,
		&p.Actor, &p.Surface, &proposed); err != nil {
		return p, err
	}
	p.ImportID = importID.String
	for _, field := range []struct {
		text string
		into any
	}{{manifest, &p.Manifest}, {metadata, &p.Metadata}, {source, &p.Source}, {warnings, &p.Warnings}} {
		if err := json.Unmarshal([]byte(field.text), field.into); err != nil {
			return p, fmt.Errorf("decode skill proposal %s: %w", p.ID, err)
		}
	}
	var err error
	p.ProposedAt, err = parseTime(proposed)
	return p, err
}

func isPendingSkillProposal(ctx context.Context, q queryRower, id string) (bool, error) {
	var pending bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1`+pendingSkillProposals+` AND p.id = ?)`, id).Scan(&pending)
	return pending, err
}

// SkillProposal reads one proposal.
func (s *Store) SkillProposal(ctx context.Context, id string) (SkillProposal, error) {
	p, err := scanSkillProposal(s.db.QueryRowContext(ctx, `SELECT `+skillProposalColumns+` FROM skill_proposals p WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("%w: proposal %s", ErrSkillNotFound, id)
	}
	if err != nil {
		return p, err
	}
	if p.Pending, err = isPendingSkillProposal(ctx, s.db, id); err != nil {
		return p, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT version FROM skill_activations WHERE proposal_id = ? ORDER BY version`, id)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return p, err
		}
		p.ApprovedVersions = append(p.ApprovedVersions, version)
	}
	return p, rows.Err()
}

// PendingSkillProposals lists each name's pending proposal, by name.
func (s *Store) PendingSkillProposals(ctx context.Context) ([]SkillProposal, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+skillProposalColumns+pendingSkillProposals+` ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillProposal{}
	for rows.Next() {
		p, err := scanSkillProposal(rows)
		if err != nil {
			return nil, err
		}
		p.Pending = true
		out = append(out, p)
	}
	return out, rows.Err()
}

const skillActivationColumns = `a.name,a.version,a.proposal_id,a.commit_id,a.tree_path,a.version_hash,a.manifest,a.metadata,a.actor,a.surface,a.approved_at,
 d.actor,d.surface,d.deactivated_at`

const skillActivationFrom = ` FROM skill_activations a LEFT JOIN skill_deactivations d ON d.name = a.name AND d.version = a.version`

// activeSkillVersions keeps each name's latest activation while it has no
// deactivation.
const activeSkillVersions = ` WHERE a.version = (SELECT MAX(version) FROM skill_activations WHERE name = a.name) AND d.version IS NULL`

func scanSkillActivation(scanner rowScanner) (SkillActivation, error) {
	var a SkillActivation
	var manifest, metadata, approved string
	var actor, surface, deactivated sql.NullString
	if err := scanner.Scan(&a.Name, &a.Version, &a.ProposalID, &a.Commit, &a.Path, &a.VersionHash, &manifest, &metadata,
		&a.Actor, &a.Surface, &approved, &actor, &surface, &deactivated); err != nil {
		return a, err
	}
	if err := json.Unmarshal([]byte(manifest), &a.Manifest); err != nil {
		return a, fmt.Errorf("decode skill %s version %d manifest: %w", a.Name, a.Version, err)
	}
	if err := json.Unmarshal([]byte(metadata), &a.Metadata); err != nil {
		return a, fmt.Errorf("decode skill %s version %d metadata: %w", a.Name, a.Version, err)
	}
	var err error
	if a.ApprovedAt, err = parseTime(approved); err != nil {
		return a, err
	}
	if deactivated.Valid {
		at, err := parseTime(deactivated.String)
		if err != nil {
			return a, err
		}
		a.Deactivated = &SkillDeactivation{Actor: actor.String, Surface: surface.String, At: at}
	}
	return a, nil
}

func querySkillActivations(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, where string, args ...any) ([]SkillActivation, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+skillActivationColumns+skillActivationFrom+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillActivation{}
	for rows.Next() {
		a, err := scanSkillActivation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActiveSkills lists every name's active version, by name.
func (s *Store) ActiveSkills(ctx context.Context) ([]SkillActivation, error) {
	return querySkillActivations(ctx, s.db, activeSkillVersions+` ORDER BY a.name`)
}

// SkillActivationByHash reads the newest approved version of name with
// exactly that version hash, active or not, as a chat that pinned it
// resolves it.
func (s *Store) SkillActivationByHash(ctx context.Context, name, hash string) (SkillActivation, error) {
	found, err := querySkillActivations(ctx, s.db, ` WHERE a.name = ? AND a.version_hash = ? ORDER BY a.version DESC LIMIT 1`, name, hash)
	if err != nil {
		return SkillActivation{}, err
	}
	if len(found) == 0 {
		return SkillActivation{}, fmt.Errorf("%w: %s at %s was never approved", ErrSkillNotFound, name, hash)
	}
	return found[0], nil
}

// SkillState reads one name's versions and proposals, newest first.
func (s *Store) SkillState(ctx context.Context, name string) (SkillState, error) {
	state := SkillState{Name: name}
	var err error
	if state.Versions, err = querySkillActivations(ctx, s.db, ` WHERE a.name = ? ORDER BY a.version DESC`, name); err != nil {
		return state, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+skillProposalColumns+` FROM skill_proposals p WHERE p.name = ? ORDER BY p.rowid DESC`, name)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	state.Proposals = []SkillProposal{}
	for rows.Next() {
		p, err := scanSkillProposal(rows)
		if err != nil {
			return state, err
		}
		for i := len(state.Versions) - 1; i >= 0; i-- {
			if state.Versions[i].ProposalID == p.ID {
				p.ApprovedVersions = append(p.ApprovedVersions, state.Versions[i].Version)
			}
		}
		state.Proposals = append(state.Proposals, p)
	}
	if err := rows.Err(); err != nil {
		return state, err
	}
	if len(state.Proposals) == 0 {
		return state, fmt.Errorf("%w: %s has never been proposed", ErrSkillNotFound, name)
	}
	state.Proposals[0].Pending, err = isPendingSkillProposal(ctx, s.db, state.Proposals[0].ID)
	return state, err
}

// ActivateSkill records the owner's approval of the name's pending
// proposal, at exactly the version hash reviewed and against exactly the
// active version reviewed (0 when none), as the name's next version. It
// replaces any active version, so at most one is active.
func (s *Store) ActivateSkill(ctx context.Context, approval SkillApproval) (SkillActivation, error) {
	if !ownerCLI(approval.Actor, approval.Surface) {
		return SkillActivation{}, ErrSkillOwnerOnly
	}
	var out SkillActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		p, err := scanSkillProposal(conn.QueryRowContext(ctx, `SELECT `+skillProposalColumns+pendingSkillProposals+` AND p.name = ?`, approval.Name))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w for skill %s", ErrSkillNoPending, approval.Name)
		}
		if err != nil {
			return err
		}
		if p.VersionHash != approval.VersionHash {
			return fmt.Errorf("%w: %s's pending proposal is %s", ErrSkillHashMismatch, p.Name, p.ID)
		}
		versions, err := querySkillActivations(ctx, conn, ` WHERE a.name = ? ORDER BY a.version DESC LIMIT 1`, p.Name)
		if err != nil {
			return err
		}
		var latest, active int64
		if len(versions) > 0 {
			latest = versions[0].Version
			if versions[0].Deactivated == nil {
				active = latest
			}
		}
		if active != approval.ExpectedVersion {
			return fmt.Errorf("%w: %s's active version is %d, not %d", ErrSkillChanged, p.Name, active, approval.ExpectedVersion)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO skill_activations(name,version,proposal_id,commit_id,tree_path,version_hash,manifest,metadata,actor,surface,approved_at)
 SELECT name,?,id,commit_id,tree_path,version_hash,manifest,metadata,?,?,? FROM skill_proposals WHERE id = ?`,
			latest+1, approval.Actor, approval.Surface, formatTime(s.now()), p.ID); err != nil {
			return fmt.Errorf("record skill activation: %w", err)
		}
		found, err := querySkillActivations(ctx, conn, ` WHERE a.name = ? AND a.version = ?`, p.Name, latest+1)
		if err != nil {
			return err
		}
		out = found[0]
		return nil
	})
	return out, err
}

// DeactivateSkill records the owner ending the name's active version.
func (s *Store) DeactivateSkill(ctx context.Context, name, actor, surface string) (SkillActivation, error) {
	if !ownerCLI(actor, surface) {
		return SkillActivation{}, ErrSkillOwnerOnly
	}
	var out SkillActivation
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		active, err := querySkillActivations(ctx, conn, activeSkillVersions+` AND a.name = ?`, name)
		if err != nil {
			return err
		}
		if len(active) == 0 {
			return fmt.Errorf("%w for skill %s", ErrSkillNotActive, name)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO skill_deactivations(name,version,actor,surface,deactivated_at) VALUES(?,?,?,?,?)`,
			name, active[0].Version, actor, surface, formatTime(s.now())); err != nil {
			return fmt.Errorf("record skill deactivation: %w", err)
		}
		found, err := querySkillActivations(ctx, conn, ` WHERE a.name = ? AND a.version = ?`, name, active[0].Version)
		if err != nil {
			return err
		}
		out = found[0]
		return nil
	})
	return out, err
}
