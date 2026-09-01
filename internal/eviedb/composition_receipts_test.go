package eviedb

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/composition"
)

func TestCompositionReceiptRoundTripsExactlyAcrossSQLiteReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	if err := store.SaveCompositionReceipt(ctx, session.ID, receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	got, err := NewStore(db).GetCompositionReceipt(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, receipt) {
		t.Fatalf("receipt after reopen = %#v, want %#v", got, receipt)
	}

	if _, err := db.ExecContext(ctx, `UPDATE session_composition_receipts SET receipt_json = '{}' WHERE session_id = ?`, session.ID); err == nil ||
		!strings.Contains(err.Error(), "composition receipts are immutable") {
		t.Fatalf("receipt update error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM session_composition_receipts WHERE session_id = ?`, session.ID); err == nil ||
		!strings.Contains(err.Error(), "composition receipts are immutable") {
		t.Fatalf("receipt delete error = %v", err)
	}
}

func TestCompositionReceiptPersistenceRejectsCredentialsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	receipt.Configuration = []composition.ConfigurationReference{{
		Kind: composition.ConfigurationConnection, ID: "raw-access-token",
	}}
	if err := store.SaveCompositionReceipt(ctx, session.ID, receipt); err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("credential persistence error = %v", err)
	}

	receipt = standardReceipt(t)
	if err := store.SaveCompositionReceipt(ctx, session.ID, receipt); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCompositionReceipt(ctx, session.ID, receipt); !errors.Is(err, ErrCompositionReceiptExists) {
		t.Fatalf("duplicate receipt error = %v, want ErrCompositionReceiptExists", err)
	}
	if _, err := store.GetCompositionReceipt(ctx, "missing"); !errors.Is(err, ErrCompositionReceiptNotFound) {
		t.Fatalf("missing receipt error = %v, want ErrCompositionReceiptNotFound", err)
	}

	var stored string
	if err := db.QueryRowContext(ctx, `SELECT receipt_json FROM session_composition_receipts WHERE session_id = ?`, session.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, "raw-access-token") {
		t.Fatalf("stored receipt contains rejected credential: %s", stored)
	}
}

func TestCompositionReceiptForeignKeyRejectsMissingSession(t *testing.T) {
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	err = NewStore(db).SaveCompositionReceipt(context.Background(), "missing", standardReceipt(t))
	if err == nil || !strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		t.Fatalf("missing session receipt error = %v", err)
	}
}

func TestGlobalSessionAndCompositionReceiptAreCreatedAtomically(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	receipt := standardReceipt(t)
	session, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(got, receipt) {
		t.Fatalf("atomic receipt = %#v, %v; want %#v", got, err, receipt)
	}

	invalid := receipt
	invalid.Configuration[0].ID = "raw-token"
	if _, err := store.CreateGlobalSessionWithComposition(ctx, invalid); err == nil || !strings.Contains(err.Error(), "canonical UUID") {
		t.Fatalf("invalid atomic create error = %v", err)
	}
	var sessions int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("sessions after rejected composition = %d, want 1", sessions)
	}
}

func TestGlobalSessionCreationRollsBackWhenReceiptInsertFails(t *testing.T) {
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `
		CREATE TRIGGER reject_test_composition_receipt
		BEFORE INSERT ON session_composition_receipts
		FOR EACH ROW
		BEGIN
			SELECT RAISE(ABORT, 'forced receipt insertion failure');
		END;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(db).CreateGlobalSessionWithComposition(ctx, standardReceipt(t)); err == nil ||
		!strings.Contains(err.Error(), "forced receipt insertion failure") {
		t.Fatalf("forced receipt insertion error = %v", err)
	}
	var sessions, receipts int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_composition_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || receipts != 0 {
		t.Fatalf("rolled-back rows: sessions=%d receipts=%d, want zero", sessions, receipts)
	}
}

func standardReceipt(t *testing.T) composition.Receipt {
	t.Helper()
	const hash = "0000000000000000000000000000000000000000000000000000000000000000"
	return composition.Receipt{
		FormatVersion: composition.FormatVersion,
		Preset: composition.PresetIdentity{
			ID: "standard", Version: "sha256:" + hash,
		},
		EvieVersion: "1.0.0",
		Providers:   []composition.Provider{{ID: "fixture", ImplementationVersion: "1.0.0"}},
		Capabilities: []composition.Capability{{
			ID: "fixture.echo", ProviderID: "fixture", ContractVersion: "1.0.0", SchemaSHA256: hash,
		}},
		ToolSchemas:  []composition.ToolSchema{{Name: "fixture_echo", SHA256: hash}},
		Instructions: []composition.InstructionReference{{ID: "fixture-instructions", SHA256: hash}},
		Configuration: []composition.ConfigurationReference{{
			Kind: composition.ConfigurationConnection, ID: "da73b499-4df4-4a91-bbe8-4fd3a223e634",
		}},
		Warnings: []composition.Warning{{
			Code: composition.WarningProviderDisabled, CapabilityID: "optional.echo", ProviderID: "optional",
		}},
	}
}
