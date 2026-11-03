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

// SkillImport is one import-claude run: its report and the proposals it
// recorded, which reference it so the owner can approve them together.
type SkillImport struct {
	ID         string            `json:"id"`
	Report     SkillImportReport `json:"report"`
	Proposals  []SkillProposal   `json:"proposals"`
	Actor      string            `json:"actor"`
	Surface    string            `json:"surface"`
	ImportedAt time.Time         `json:"imported_at"`
}

// SkillImportReport is what one import found, in precedence order within
// each list: the skills it proposed, refused (with the problems), found
// shadowed by an earlier copy of the same name, and left unchanged because
// their version is already active or pending; Notes are what it could not
// read.
type SkillImportReport struct {
	Plugins   bool               `json:"plugins"`
	Proposed  []SkillImportEntry `json:"proposed"`
	Refused   []SkillImportEntry `json:"refused"`
	Shadowed  []SkillImportEntry `json:"shadowed"`
	Unchanged []SkillImportEntry `json:"unchanged"`
	Notes     []string           `json:"notes"`
}

// SkillImportEntry is one skill folder an import found.
type SkillImportEntry struct {
	Name   string      `json:"name"`
	Source SkillSource `json:"source"`
	// ProposalID, VersionHash and Warnings: a proposed skill.
	ProposalID  string          `json:"proposal_id,omitempty"`
	VersionHash string          `json:"version_hash,omitempty"`
	Warnings    []skill.Warning `json:"warnings,omitempty"`
	// Problems: why a refused skill was refused.
	Problems []skill.Problem `json:"problems,omitempty"`
	// ShadowedBy: the earlier copy that shadows this one.
	ShadowedBy *SkillSource `json:"shadowed_by,omitempty"`
	// SameAs: what an unchanged skill's version already is, for example
	// "active version 2".
	SameAs string `json:"same_as,omitempty"`
}

// RecordSkillImport records the owner's import and its proposals together:
// each proposal (of a different name, checked as RecordSkillProposal
// checks one) references the import, and the report's Proposed list is
// made from the proposals as recorded.
func (s *Store) RecordSkillImport(ctx context.Context, imp SkillImport) (SkillImport, error) {
	if !ownerCLI(imp.Actor, imp.Surface) {
		return SkillImport{}, ErrSkillOwnerOnly
	}
	imp.ID, imp.ImportedAt = uuid.NewString(), s.now().UTC()
	rows := make([]skillProposalRow, len(imp.Proposals))
	names := map[string]bool{}
	imp.Report.Proposed = []SkillImportEntry{}
	for i := range imp.Proposals {
		p := &imp.Proposals[i]
		if names[p.Name] {
			return SkillImport{}, fmt.Errorf("record skill import: %s is proposed twice", p.Name)
		}
		names[p.Name] = true
		if !ownerCLI(p.Actor, p.Surface) {
			return SkillImport{}, ErrSkillOwnerOnly
		}
		p.ImportID = imp.ID
		var err error
		if rows[i], err = encodeSkillProposal(p, imp.ImportedAt); err != nil {
			return SkillImport{}, err
		}
		imp.Report.Proposed = append(imp.Report.Proposed, SkillImportEntry{Name: p.Name, Source: p.Source,
			ProposalID: p.ID, VersionHash: p.VersionHash, Warnings: p.Warnings})
	}
	for _, list := range []*[]SkillImportEntry{&imp.Report.Refused, &imp.Report.Shadowed, &imp.Report.Unchanged} {
		if *list == nil {
			*list = []SkillImportEntry{}
		}
	}
	if imp.Report.Notes == nil {
		imp.Report.Notes = []string{}
	}
	report, err := json.Marshal(imp.Report)
	if err != nil {
		return SkillImport{}, err
	}
	err = s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, `INSERT INTO skill_imports(id,report,actor,surface,imported_at) VALUES(?,?,?,?,?)`,
			imp.ID, string(report), imp.Actor, imp.Surface, formatTime(imp.ImportedAt)); err != nil {
			return fmt.Errorf("record skill import: %w", err)
		}
		for i := range imp.Proposals {
			if err := insertSkillProposal(ctx, conn, &imp.Proposals[i], rows[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SkillImport{}, err
	}
	return imp, nil
}

// SkillImport reads one import: its report and its proposals, in the order
// they were recorded, each with what became of it.
func (s *Store) SkillImport(ctx context.Context, id string) (SkillImport, error) {
	imp := SkillImport{ID: id}
	var report, imported string
	err := s.db.QueryRowContext(ctx, `SELECT report,actor,surface,imported_at FROM skill_imports WHERE id = ?`, id).
		Scan(&report, &imp.Actor, &imp.Surface, &imported)
	if errors.Is(err, sql.ErrNoRows) {
		return imp, fmt.Errorf("%w: import %s", ErrSkillNotFound, id)
	}
	if err != nil {
		return imp, err
	}
	if err := json.Unmarshal([]byte(report), &imp.Report); err != nil {
		return imp, fmt.Errorf("decode skill import %s: %w", id, err)
	}
	if imp.ImportedAt, err = parseTime(imported); err != nil {
		return imp, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+skillProposalColumns+` FROM skill_proposals p WHERE p.import_id = ? ORDER BY p.rowid`, id)
	if err != nil {
		return imp, err
	}
	defer rows.Close()
	imp.Proposals = []SkillProposal{}
	for rows.Next() {
		p, err := scanSkillProposal(rows)
		if err != nil {
			return imp, err
		}
		imp.Proposals = append(imp.Proposals, p)
	}
	if err := rows.Err(); err != nil {
		return imp, err
	}
	rows.Close()
	for i := range imp.Proposals {
		if err := fillSkillProposalState(ctx, s.db, &imp.Proposals[i]); err != nil {
			return imp, err
		}
	}
	return imp, nil
}
