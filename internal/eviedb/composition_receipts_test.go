package eviedb

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
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

func TestCompatibilityResolutionAppendsOnceAcrossConcurrentResumeAndSQLiteReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	storeA := NewStore(dbA)
	session, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	if err := storeA.SaveCompositionReceipt(ctx, session.ID, receipt); err != nil {
		t.Fatal(err)
	}
	dbB, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	storeB := NewStore(dbB)
	resolution := composition.CompatibilityResolution{
		OriginalProvider:                 composition.Provider{ID: "fixture", ImplementationVersion: "1.0.0"},
		ReplacementImplementationVersion: "1.1.0",
		KernelAPIVersion:                 "1.0.0",
		Capabilities: []composition.CompatibilityCapability{{
			ID: "fixture.echo", ContractVersion: "1.0.0",
			SchemaSHA256: strings.Repeat("0", 64),
		}},
		ResolvedAt: time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC),
	}

	stores := []*Store{storeA, storeB}
	errs := make([]error, len(stores))
	readReceipts := make([]composition.Receipt, len(stores))
	readErrs := make([]error, len(stores))
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			errs[index] = stores[index].AppendCompatibilityResolutions(ctx, session.ID, []composition.CompatibilityResolution{resolution})
		}(i)
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			readReceipts[index], readErrs[index] = stores[index].GetCompositionReceipt(ctx, session.ID)
		}(i)
	}
	close(start)
	group.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent append %d: %v", i, err)
		}
	}
	for i, err := range readErrs {
		if err != nil || !reflect.DeepEqual(readReceipts[i], receipt) {
			t.Fatalf("concurrent receipt read %d = %#v, %v; want exact %#v", i, readReceipts[i], err, receipt)
		}
	}
	if err := storeA.AppendCompatibilityResolutions(ctx, session.ID, []composition.CompatibilityResolution{resolution}); err != nil {
		t.Fatal(err)
	}
	if err := dbA.Close(); err != nil {
		t.Fatal(err)
	}
	if err := dbB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewStore(db)
	got, err := store.GetCompatibilityResolutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []composition.CompatibilityResolution{resolution}) {
		t.Fatalf("reopened resolutions = %#v, want one exact %#v", got, resolution)
	}
	storedReceipt, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(storedReceipt, receipt) {
		t.Fatalf("original receipt after resolution = %#v, %v; want immutable %#v", storedReceipt, err, receipt)
	}
	if _, err := db.ExecContext(ctx, `UPDATE session_compatibility_resolutions SET resolution_json = '{}' WHERE session_id = ?`, session.ID); err == nil ||
		!strings.Contains(err.Error(), "compatibility resolutions are append-only") {
		t.Fatalf("resolution update error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM session_compatibility_resolutions WHERE session_id = ?`, session.ID); err == nil ||
		!strings.Contains(err.Error(), "compatibility resolutions are append-only") {
		t.Fatalf("resolution delete error = %v", err)
	}
}

func TestCompatibilityResolutionMustMatchPinnedReceiptBeforeAppend(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*composition.Receipt, *composition.CompatibilityResolution)
		want   string
	}{
		{
			name: "unrelated provider",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.OriginalProvider.ID = "unrelated"
				resolution.Capabilities[0].ID = "unrelated.echo"
			},
			want: `original provider "unrelated" version "1.0.0" is not pinned`,
		},
		{
			name: "wrong original version",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.OriginalProvider.ImplementationVersion = "1.0.1"
				resolution.ReplacementImplementationVersion = "1.1.1"
			},
			want: `original provider "fixture" version "1.0.1" does not match pinned version "1.0.0"`,
		},
		{
			name: "missing capability",
			mutate: func(receipt *composition.Receipt, _ *composition.CompatibilityResolution) {
				receipt.Capabilities = append(receipt.Capabilities, composition.Capability{
					ID: "fixture.second", ProviderID: "fixture", ContractVersion: "1.0.0",
					SchemaSHA256: strings.Repeat("1", 64),
				})
				receipt.ToolSchemas = append(receipt.ToolSchemas, composition.ToolSchema{
					Name: "fixture_second", SHA256: strings.Repeat("1", 64),
				})
			},
			want: `is missing pinned Capability "fixture.second"`,
		},
		{
			name: "extra capability",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.Capabilities = append(resolution.Capabilities, composition.CompatibilityCapability{
					ID: "fixture.extra", ContractVersion: "1.0.0", SchemaSHA256: strings.Repeat("1", 64),
				})
			},
			want: `contains unpinned Capability "fixture.extra"`,
		},
		{
			name: "cross-provider capability",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.Capabilities = append(resolution.Capabilities, composition.CompatibilityCapability{
					ID: "unrelated.echo", ContractVersion: "1.0.0", SchemaSHA256: strings.Repeat("1", 64),
				})
			},
			want: `Capability "unrelated.echo" has an invalid provider identity`,
		},
		{
			name: "wrong contract",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.Capabilities[0].ContractVersion = "1.0.1"
			},
			want: `Capability "fixture.echo" contract "1.0.1" does not match pinned contract "1.0.0"`,
		},
		{
			name: "wrong schema",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.Capabilities[0].SchemaSHA256 = strings.Repeat("1", 64)
			},
			want: `Capability "fixture.echo" schema`,
		},
		{
			name: "duplicate capability",
			mutate: func(_ *composition.Receipt, resolution *composition.CompatibilityResolution) {
				resolution.Capabilities = append(resolution.Capabilities, resolution.Capabilities[0])
			},
			want: `repeats Capability "fixture.echo"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			resolution := standardCompatibilityResolution()
			tc.mutate(&receipt, &resolution)
			if err := store.SaveCompositionReceipt(ctx, session.ID, receipt); err != nil {
				t.Fatal(err)
			}
			err = store.AppendCompatibilityResolutions(
				ctx, session.ID,
				[]composition.CompatibilityResolution{standardCompatibilityResolution(), resolution},
			)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("append error = %v, want containing %q", err, tc.want)
			}
			resolutions, readErr := store.GetCompatibilityResolutions(ctx, session.ID)
			if readErr != nil || len(resolutions) != 0 {
				t.Fatalf("rows after contradictory batch = %#v, %v; want none", resolutions, readErr)
			}
			stored, readErr := store.GetCompositionReceipt(ctx, session.ID)
			if readErr != nil || !reflect.DeepEqual(stored, receipt) {
				t.Fatalf("receipt after rejected append = %#v, %v; want unchanged %#v", stored, readErr, receipt)
			}
		})
	}
}

func standardCompatibilityResolution() composition.CompatibilityResolution {
	return composition.CompatibilityResolution{
		OriginalProvider:                 composition.Provider{ID: "fixture", ImplementationVersion: "1.0.0"},
		ReplacementImplementationVersion: "1.1.0",
		KernelAPIVersion:                 "1.0.0",
		Capabilities: []composition.CompatibilityCapability{{
			ID: "fixture.echo", ContractVersion: "1.0.0", SchemaSHA256: strings.Repeat("0", 64),
		}},
		ResolvedAt: time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestCompatibilityResolutionCommitAndRollbackFailuresDoNotPoisonPool(t *testing.T) {
	for _, tc := range []struct {
		name         string
		failRollback bool
	}{
		{name: "commit failure rolls back"},
		{name: "rollback failure discards connection", failRollback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, sessionID, receipt := compatibilityResolutionRecoveryFixture(t)
			var statements []string
			store.resolveImmediateTransaction = func(
				ctx context.Context,
				conn *sql.Conn,
				statement string,
			) (sql.Result, error) {
				statements = append(statements, statement)
				switch statement {
				case `COMMIT`:
					return nil, errors.New("forced Compatibility Resolution commit failure")
				case `ROLLBACK`:
					if tc.failRollback {
						return nil, errors.New("forced Compatibility Resolution rollback failure")
					}
				}
				return executeImmediateTransactionStatement(ctx, conn, statement)
			}

			err := store.AppendCompatibilityResolutions(
				context.Background(), sessionID,
				[]composition.CompatibilityResolution{standardCompatibilityResolution()},
			)
			if err == nil || !strings.Contains(err.Error(), "forced Compatibility Resolution commit failure") {
				t.Fatalf("append error = %v, want forced commit failure", err)
			}
			if !reflect.DeepEqual(statements, []string{`COMMIT`, `ROLLBACK`}) {
				t.Fatalf("transaction resolution statements = %v, want COMMIT then ROLLBACK", statements)
			}

			store.resolveImmediateTransaction = executeImmediateTransactionStatement
			assertCompatibilityResolutionRecovery(t, store, sessionID, receipt)
		})
	}
}

func TestCompatibilityResolutionCommitAndRollbackStallsAreBoundedAndDiscarded(t *testing.T) {
	store, sessionID, receipt := compatibilityResolutionRecoveryFixture(t)
	deadline := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	observed, commitEntered, commitCtx, rollbackEntered, rollbackCtx := scriptStoreResolutionDeadline(t, store, deadline)
	ctx := newTriggeredDeadlineContext(deadline)
	done := make(chan error, 1)
	go func() {
		done <- store.AppendCompatibilityResolutions(
			ctx, sessionID, []composition.CompatibilityResolution{standardCompatibilityResolution()},
		)
	}()
	select {
	case <-commitEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("Compatibility Resolution append did not reach COMMIT resolution")
	}
	requireStillBlocked(t, done, "Compatibility Resolution COMMIT")
	commitCtx.trigger()
	select {
	case <-rollbackEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("Compatibility Resolution append did not reach ROLLBACK resolution")
	}
	requireStillBlocked(t, done, "Compatibility Resolution ROLLBACK")
	rollbackCtx.trigger()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("append error = %v, want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Compatibility Resolution transaction outlived cleanup deadline")
	}
	requireBoundedTransactionResolution(t, observed, deadline)

	store.resolveImmediateTransaction = executeImmediateTransactionStatement
	store.newResolutionContext = transactionResolutionContext
	assertCompatibilityResolutionRecovery(t, store, sessionID, receipt)
}

func compatibilityResolutionRecoveryFixture(
	t *testing.T,
) (*Store, memory.SessionID, composition.Receipt) {
	t.Helper()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	store := NewStore(db)
	session, err := store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receipt := standardReceipt(t)
	if err := store.SaveCompositionReceipt(context.Background(), session.ID, receipt); err != nil {
		t.Fatal(err)
	}
	return store, session.ID, receipt
}

func assertCompatibilityResolutionRecovery(
	t *testing.T,
	store *Store,
	sessionID memory.SessionID,
	receipt composition.Receipt,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stored, err := store.GetCompositionReceipt(ctx, sessionID)
	if err != nil || !reflect.DeepEqual(stored, receipt) {
		t.Fatalf("receipt after failed transaction = %#v, %v; want unchanged %#v", stored, err, receipt)
	}
	resolutions, err := store.GetCompatibilityResolutions(ctx, sessionID)
	if err != nil || len(resolutions) != 0 {
		t.Fatalf("audit rows after failed transaction = %#v, %v; want none", resolutions, err)
	}
	resolution := standardCompatibilityResolution()
	if err := store.AppendCompatibilityResolutions(ctx, sessionID, []composition.CompatibilityResolution{resolution}); err != nil {
		t.Fatalf("later append after transaction recovery: %v", err)
	}
	resolutions, err = store.GetCompatibilityResolutions(ctx, sessionID)
	if err != nil || !reflect.DeepEqual(resolutions, []composition.CompatibilityResolution{resolution}) {
		t.Fatalf("later audit rows = %#v, %v; want exact successful append", resolutions, err)
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
