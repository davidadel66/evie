package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
)

var (
	ErrCompositionReceiptExists   = errors.New("eviedb: Composition Receipt already exists")
	ErrCompositionReceiptNotFound = errors.New("eviedb: Composition Receipt not found")
)

type compositionReceiptExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type compositionReceiptQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SaveCompositionReceipt appends the one immutable composition identity for a
// session. Validation occurs before SQLite so rejected secret-shaped content
// is never written even transiently.
func (s *Store) SaveCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
	receipt composition.Receipt,
) error {
	if sessionID == "" {
		return errors.New("session ID must not be empty")
	}
	encoded, err := composition.Marshal(receipt)
	if err != nil {
		return fmt.Errorf("validate Composition Receipt: %w", err)
	}
	err = insertCompositionReceipt(ctx, s.db, sessionID, encoded, s.now())
	if err == nil {
		return nil
	}
	var exists bool
	if lookupErr := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM session_composition_receipts WHERE session_id = ?)
	`, sessionID).Scan(&exists); lookupErr == nil && exists {
		return fmt.Errorf("%w for session %q", ErrCompositionReceiptExists, sessionID)
	}
	return fmt.Errorf("save Composition Receipt for session %q: %w", sessionID, err)
}

func insertCompositionReceipt(
	ctx context.Context,
	executor compositionReceiptExecutor,
	sessionID memory.SessionID,
	encoded []byte,
	recordedAt time.Time,
) error {
	_, err := executor.ExecContext(ctx, `
		INSERT INTO session_composition_receipts (session_id, receipt_json, recorded_at)
		VALUES (?, ?, ?)
	`, sessionID, string(encoded), recordedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) GetCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
) (composition.Receipt, error) {
	return getCompositionReceipt(ctx, s.db, sessionID)
}

func getCompositionReceipt(
	ctx context.Context,
	queryer compositionReceiptQueryer,
	sessionID memory.SessionID,
) (composition.Receipt, error) {
	if sessionID == "" {
		return composition.Receipt{}, errors.New("session ID must not be empty")
	}
	var encoded string
	err := queryer.QueryRowContext(ctx, `
		SELECT receipt_json FROM session_composition_receipts WHERE session_id = ?
	`, sessionID).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return composition.Receipt{}, fmt.Errorf("%w for session %q", ErrCompositionReceiptNotFound, sessionID)
	}
	if err != nil {
		return composition.Receipt{}, fmt.Errorf("read Composition Receipt for session %q: %w", sessionID, err)
	}
	receipt, err := composition.Unmarshal([]byte(encoded))
	if err != nil {
		return composition.Receipt{}, fmt.Errorf("decode Composition Receipt for session %q: %w", sessionID, err)
	}
	return receipt, nil
}

// AppendCompatibilityResolutions durably records substitution evidence before
// a resumed session can execute. Identical evidence is idempotent even when
// separate Evie processes race to resume the same session.
func (s *Store) AppendCompatibilityResolutions(
	ctx context.Context,
	sessionID memory.SessionID,
	resolutions []composition.CompatibilityResolution,
) error {
	if sessionID == "" {
		return errors.New("session ID must not be empty")
	}
	if len(resolutions) == 0 {
		return nil
	}
	err := s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		receipt, err := getCompositionReceipt(ctx, conn, sessionID)
		if err != nil {
			return fmt.Errorf("bind Compatibility Resolution to Composition Receipt: %w", err)
		}
		type validatedResolution struct {
			resolution composition.CompatibilityResolution
			encoded    []byte
			key        string
		}
		validated := make([]validatedResolution, 0, len(resolutions))
		for _, resolution := range resolutions {
			encoded, err := composition.MarshalCompatibilityResolution(resolution)
			if err != nil {
				return fmt.Errorf("validate Compatibility Resolution for session %q: %w", sessionID, err)
			}
			key, err := composition.CompatibilityResolutionKey(resolution)
			if err != nil {
				return fmt.Errorf("identify Compatibility Resolution for session %q: %w", sessionID, err)
			}
			if err := validateCompatibilityResolutionReceiptBinding(receipt, resolution); err != nil {
				return fmt.Errorf("bind Compatibility Resolution for session %q: %w", sessionID, err)
			}
			validated = append(validated, validatedResolution{resolution: resolution, encoded: encoded, key: key})
		}
		for _, item := range validated {
			if _, err := conn.ExecContext(ctx, `
				INSERT INTO session_compatibility_resolutions (
					session_id, resolution_key, resolution_json, resolved_at
				) VALUES (?, ?, ?, ?)
				ON CONFLICT (session_id, resolution_key) DO NOTHING
			`, sessionID, item.key, string(item.encoded), item.resolution.ResolvedAt.UTC().Format(time.RFC3339Nano)); err != nil {
				return fmt.Errorf("append Compatibility Resolution for session %q: %w", sessionID, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("append Compatibility Resolutions for session %q: %w", sessionID, err)
	}
	return nil
}

func validateCompatibilityResolutionReceiptBinding(
	receipt composition.Receipt,
	resolution composition.CompatibilityResolution,
) error {
	var pinnedProvider *composition.Provider
	for i := range receipt.Providers {
		if receipt.Providers[i].ID == resolution.OriginalProvider.ID {
			pinnedProvider = &receipt.Providers[i]
			break
		}
	}
	if pinnedProvider == nil {
		return fmt.Errorf(
			"original provider %q version %q is not pinned by the Composition Receipt",
			resolution.OriginalProvider.ID, resolution.OriginalProvider.ImplementationVersion,
		)
	}
	if pinnedProvider.ImplementationVersion != resolution.OriginalProvider.ImplementationVersion {
		return fmt.Errorf(
			"original provider %q version %q does not match pinned version %q",
			resolution.OriginalProvider.ID, resolution.OriginalProvider.ImplementationVersion,
			pinnedProvider.ImplementationVersion,
		)
	}
	pinnedCapabilities := make(map[string]composition.Capability)
	for _, capability := range receipt.Capabilities {
		if capability.ProviderID == resolution.OriginalProvider.ID {
			pinnedCapabilities[capability.ID] = capability
		}
	}
	seen := make(map[string]struct{}, len(resolution.Capabilities))
	for _, evidence := range resolution.Capabilities {
		if _, duplicate := seen[evidence.ID]; duplicate {
			return fmt.Errorf("Compatibility Resolution repeats Capability %q", evidence.ID)
		}
		seen[evidence.ID] = struct{}{}
		pinned, exists := pinnedCapabilities[evidence.ID]
		if !exists {
			return fmt.Errorf("Compatibility Resolution contains unpinned Capability %q", evidence.ID)
		}
		if evidence.ContractVersion != pinned.ContractVersion {
			return fmt.Errorf(
				"Compatibility Resolution Capability %q contract %q does not match pinned contract %q",
				evidence.ID, evidence.ContractVersion, pinned.ContractVersion,
			)
		}
		if evidence.SchemaSHA256 != pinned.SchemaSHA256 {
			return fmt.Errorf(
				"Compatibility Resolution Capability %q schema %s does not match pinned schema %s",
				evidence.ID, evidence.SchemaSHA256, pinned.SchemaSHA256,
			)
		}
	}
	missing := make([]string, 0)
	for id := range pinnedCapabilities {
		if _, exists := seen[id]; !exists {
			missing = append(missing, id)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("Compatibility Resolution is missing pinned Capability %q", missing[0])
	}
	return nil
}

func (s *Store) GetCompatibilityResolutions(
	ctx context.Context,
	sessionID memory.SessionID,
) ([]composition.CompatibilityResolution, error) {
	if sessionID == "" {
		return nil, errors.New("session ID must not be empty")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT resolution_json
		FROM session_compatibility_resolutions
		WHERE session_id = ?
		ORDER BY resolved_at, resolution_key
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read Compatibility Resolutions for session %q: %w", sessionID, err)
	}
	defer rows.Close()
	resolutions := make([]composition.CompatibilityResolution, 0)
	for rows.Next() {
		var encoded string
		if err := rows.Scan(&encoded); err != nil {
			return nil, fmt.Errorf("read Compatibility Resolution for session %q: %w", sessionID, err)
		}
		resolution, err := composition.UnmarshalCompatibilityResolution([]byte(encoded))
		if err != nil {
			return nil, fmt.Errorf("decode Compatibility Resolution for session %q: %w", sessionID, err)
		}
		resolutions = append(resolutions, resolution)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Compatibility Resolutions for session %q: %w", sessionID, err)
	}
	return resolutions, nil
}
