package main

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
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
)

func TestBindSessionCompositionPinsLegacySessionOnce(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	manager := sessionCompositionManager(t)
	standard, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	bound, err := bindSessionComposition(ctx, store, manager, session.ID, standard, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bound.Resolved.Receipt, standard.Receipt) || !reflect.DeepEqual(stored, standard.Receipt) {
		t.Fatalf("bound=%#v stored=%#v, want %#v", bound.Resolved.Receipt, stored, standard.Receipt)
	}
	boundAgain, err := bindSessionComposition(ctx, store, manager, session.ID, standard, nil)
	if err != nil || !reflect.DeepEqual(boundAgain.Resolved.Receipt, stored) {
		t.Fatalf("second bind = %#v, %v; want pinned %#v", boundAgain.Resolved.Receipt, err, stored)
	}
}

func TestReceiptBoundREPLStoreCreatesSessionAndReceiptAtomically(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	manager := sessionCompositionManager(t)
	standard, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	boundStore := receiptBoundREPLStore{Store: store, composition: standard}
	session, err := boundStore.CreateGlobalSessionForChooser(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(stored, standard.Receipt) {
		t.Fatalf("stored receipt = %#v, %v; want %#v", stored, err, standard.Receipt)
	}
	selection := boundStore.selection(session)
	if selection.createdComposition == nil || !reflect.DeepEqual(selection.createdComposition.Receipt, standard.Receipt) {
		t.Fatalf("creation handoff = %#v, want original resolved composition", selection.createdComposition)
	}
}

func TestNewlyCreatedSessionUsesOriginalSnapshotWithoutSecondResolution(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	resolver := &countingSessionCompositionResolver{manager: sessionCompositionManager(t)}
	original, err := resolver.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateGlobalSessionWithComposition(ctx, original.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := resolver.manager.SetEnabled(plugins.FinancePluginID, false); err != nil {
		t.Fatal(err)
	}

	bound, err := bindSessionComposition(ctx, store, resolver, session.ID, original, &original)
	if err != nil {
		t.Fatalf("newly created bind re-resolved after lifecycle change: %v", err)
	}
	if resolver.presetCalls != 1 || resolver.resumeCalls != 0 {
		t.Fatalf("resolution calls after new bind: preset=%d resume=%d, want 1 and 0", resolver.presetCalls, resolver.resumeCalls)
	}
	if !reflect.DeepEqual(bound.Resolved.Receipt, original.Receipt) ||
		!containsSchema(bound.Resolved.Toolset, "finance_sync") {
		t.Fatalf("new bind lost original snapshot: receipt=%#v schemas=%v", bound.Resolved.Receipt, schemaNames(bound.Resolved.Toolset))
	}

	if _, err := bindSessionComposition(ctx, store, resolver, session.ID, original, nil); err == nil ||
		!strings.Contains(err.Error(), `required Capability "finance.sync"`) {
		t.Fatalf("actual resume after provider disable error = %v", err)
	}
	if resolver.resumeCalls != 1 {
		t.Fatalf("resume calls = %d, want one", resolver.resumeCalls)
	}
}

func TestBindSessionCompositionReturnsPersistedWarningsOnCreateAndResume(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &countingCompositionStore{Store: eviedb.NewStore(db)}
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	standard, err := sessionCompositionManager(t).ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	standard.Receipt.Warnings = []composition.Warning{{
		Code: composition.WarningProviderDisabled, CapabilityID: "optional.echo", ProviderID: "optional",
	}}
	wantWarnings := []visibleCompositionWarning{{
		Code:    composition.WarningProviderDisabled,
		Message: `optional Capability "optional.echo" from provider "optional" is unavailable`,
	}}

	created, err := bindSessionComposition(ctx, store, echoCompositionResolver{}, session.ID, standard, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(created.Warnings, wantWarnings) || store.saves != 1 {
		t.Fatalf("created warnings=%#v saves=%d, want %#v and one save", created.Warnings, store.saves, wantWarnings)
	}
	resumed, err := bindSessionComposition(ctx, store, echoCompositionResolver{}, session.ID, standard, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resumed.Warnings, wantWarnings) || store.saves != 1 {
		t.Fatalf("resumed warnings=%#v saves=%d, want %#v and no receipt mutation", resumed.Warnings, store.saves, wantWarnings)
	}
	stored, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(stored.Warnings, standard.Receipt.Warnings) {
		t.Fatalf("stored warnings=%#v, %v; want immutable %#v", stored.Warnings, err, standard.Receipt.Warnings)
	}
	losingCandidate := standard
	losingCandidate.Receipt = composition.Clone(standard.Receipt)
	losingCandidate.Receipt.Warnings = []composition.Warning{{
		Code: composition.WarningProviderDisabled, CapabilityID: "optional.loser", ProviderID: "optional",
	}}
	winner, err := bindSessionComposition(
		ctx, store, echoCompositionResolver{}, session.ID, losingCandidate, &losingCandidate,
	)
	if err != nil || !reflect.DeepEqual(winner.Resolved.Receipt, stored) {
		t.Fatalf("mismatched creation candidate = %#v, %v; want durable winner %#v", winner.Resolved.Receipt, err, stored)
	}
	if store.saves != 1 {
		t.Fatalf("mismatched candidate mutated receipt: saves=%d, want one", store.saves)
	}
}

func TestConcurrentFirstBindUsesOneDurableSQLiteWinner(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	dbA, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbA.Close()
	storeA := eviedb.NewStore(dbA)
	session, err := storeA.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dbB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer dbB.Close()
	storeB := eviedb.NewStore(dbB)
	standard, err := sessionCompositionManager(t).ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	defaults := []plugins.ResolvedComposition{standard, standard}
	for i := range defaults {
		defaults[i].Receipt = composition.Clone(standard.Receipt)
		defaults[i].Receipt.Warnings = []composition.Warning{{
			Code:         composition.WarningProviderDisabled,
			CapabilityID: compositionWarningCapability(i),
			ProviderID:   "optional",
		}}
	}
	barrier := &missingReceiptBarrier{remaining: 2, ready: make(chan struct{})}
	stores := []sessionCompositionStore{
		barrierCompositionStore{Store: storeA, barrier: barrier},
		barrierCompositionStore{Store: storeB, barrier: barrier},
	}
	results := make([]boundSessionComposition, 2)
	errs := make([]error, 2)
	var group sync.WaitGroup
	for i := range stores {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errs[index] = bindSessionComposition(
				ctx, stores[index], echoCompositionResolver{}, session.ID, defaults[index], nil,
			)
		}(i)
	}
	group.Wait()
	stored, err := storeA.GetCompositionReceipt(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for i, err := range errs {
		if err != nil || !reflect.DeepEqual(results[i].Resolved.Receipt, stored) {
			t.Fatalf("bind %d = %#v, %v; want durable winner %#v", i, results[i], err, stored)
		}
	}
	var receipts int
	if err := dbA.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_composition_receipts WHERE session_id = ?`, session.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("durable receipts = %d, want one", receipts)
	}
}

func compositionWarningCapability(index int) string {
	if index == 0 {
		return "optional.first"
	}
	return "optional.second"
}

func TestPreReceiptDatabaseReopensAndBindsLegacySessionOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	legacyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := legacyDB.ExecContext(ctx, `
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY NOT NULL,
			project_id TEXT,
			project_root_snapshot TEXT,
			parent_session_id TEXT,
			title TEXT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO sessions (id, status, created_at, updated_at)
		VALUES ('legacy-session', 'active', ?, ?);
	`, now, now); err != nil {
		t.Fatal(err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	legacy, err := store.GetSession(ctx, "legacy-session")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetCompositionReceipt(ctx, legacy.ID); !errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
		t.Fatalf("legacy receipt before bind = %v, want not found", err)
	}
	manager := sessionCompositionManager(t)
	standard, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bindSessionComposition(ctx, store, manager, legacy.ID, standard, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = eviedb.NewStore(db)
	stored, err := store.GetCompositionReceipt(ctx, legacy.ID)
	if err != nil || !reflect.DeepEqual(stored, standard.Receipt) {
		t.Fatalf("legacy receipt after reopen = %#v, %v; want %#v", stored, err, standard.Receipt)
	}
	if _, err := bindSessionComposition(ctx, store, manager, legacy.ID, standard, nil); err != nil {
		t.Fatal(err)
	}
	var receipts int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM session_composition_receipts WHERE session_id = ?`, legacy.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if receipts != 1 {
		t.Fatalf("legacy receipts after second bind = %d, want one", receipts)
	}
}

type countingCompositionStore struct {
	*eviedb.Store
	saves int
}

func (s *countingCompositionStore) SaveCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
	receipt composition.Receipt,
) error {
	s.saves++
	return s.Store.SaveCompositionReceipt(ctx, sessionID, receipt)
}

type echoCompositionResolver struct{}

func (echoCompositionResolver) ResumeComposition(receipt composition.Receipt) (plugins.ResolvedComposition, error) {
	return plugins.ResolvedComposition{Receipt: receipt}, nil
}

type countingSessionCompositionResolver struct {
	manager     *plugins.Manager
	presetCalls int
	resumeCalls int
}

func (r *countingSessionCompositionResolver) ResolvePreset(id plugins.PresetID) (plugins.ResolvedComposition, error) {
	r.presetCalls++
	return r.manager.ResolvePreset(id)
}

func (r *countingSessionCompositionResolver) ResumeComposition(
	receipt composition.Receipt,
) (plugins.ResolvedComposition, error) {
	r.resumeCalls++
	return r.manager.ResumeComposition(receipt)
}

type missingReceiptBarrier struct {
	mu        sync.Mutex
	remaining int
	ready     chan struct{}
}

func (b *missingReceiptBarrier) wait() {
	b.mu.Lock()
	b.remaining--
	if b.remaining == 0 {
		close(b.ready)
	}
	b.mu.Unlock()
	<-b.ready
}

type barrierCompositionStore struct {
	*eviedb.Store
	barrier *missingReceiptBarrier
}

func (s barrierCompositionStore) GetCompositionReceipt(
	ctx context.Context,
	sessionID memory.SessionID,
) (composition.Receipt, error) {
	receipt, err := s.Store.GetCompositionReceipt(ctx, sessionID)
	if errors.Is(err, eviedb.ErrCompositionReceiptNotFound) {
		s.barrier.wait()
	}
	return receipt, err
}

func sessionCompositionManager(t *testing.T) *plugins.Manager {
	t.Helper()
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

func containsSchema(toolset tools.Toolset, name string) bool {
	for _, schema := range toolset.Schemas() {
		if schema.Function.Name == name {
			return true
		}
	}
	return false
}

func schemaNames(toolset tools.Toolset) []string {
	schemas := toolset.Schemas()
	names := make([]string, len(schemas))
	for i, schema := range schemas {
		names[i] = schema.Function.Name
	}
	return names
}
