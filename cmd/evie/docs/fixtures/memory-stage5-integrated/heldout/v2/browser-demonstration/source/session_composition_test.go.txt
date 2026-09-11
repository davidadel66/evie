package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
	"github.com/davidadel66/evie/internal/openrouter"
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

func TestResumeAndRecordSessionCompositionPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := cancellationAwareCompositionResolver{}

	_, err := resumeAndRecordSessionComposition(
		ctx,
		panicCompositionStore{},
		resolver,
		"session",
		composition.Receipt{},
		"resume",
		"record",
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resume error = %v, want context canceled", err)
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

	workspace, err := store.RegisterWorkspace(ctx, "Cairo's Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	workspaceSession, err := boundStore.CreateWorkspaceSessionForChooser(ctx, workspace.ID, workspace.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceReceipt, err := store.GetCompositionReceipt(ctx, workspaceSession.ID)
	if err != nil || !reflect.DeepEqual(workspaceReceipt, standard.Receipt) {
		t.Fatalf("Workspace receipt = %#v, %v; want %#v", workspaceReceipt, err, standard.Receipt)
	}
	if workspaceSession.WorkspaceID != workspace.ID || workspaceSession.WorkspaceRevisionSnapshot != workspace.CurrentRevisionID {
		t.Fatalf("Workspace session=%+v, want %q at %q", workspaceSession, workspace.ID, workspace.CurrentRevisionID)
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
		!strings.Contains(err.Error(), `pinned provider plugin "finance" is disabled`) {
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

func TestSessionCompositionReopenAuditsOnlyDeclaredCompatibleReplacement(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	pinned := seedPinnedStandardCompositionWithEvieVersion(t, ctx, path, "0.9.0")
	exactDB, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	exactStore := eviedb.NewStore(exactDB)
	session := onlySession(t, ctx, exactDB)
	exactManager := sessionCompositionManager(t)
	if _, err := bindSessionComposition(
		ctx, exactStore, exactManager, session.ID, plugins.ResolvedComposition{}, nil,
	); err != nil {
		t.Fatalf("unchanged composition after Evie upgrade did not resume: %v", err)
	}
	exactResolutions, err := exactStore.GetCompatibilityResolutions(ctx, session.ID)
	if err != nil || len(exactResolutions) != 0 {
		t.Fatalf("unchanged resume resolutions = %#v, %v; want none", exactResolutions, err)
	}
	if err := exactDB.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session = onlySession(t, ctx, db)
	manager := replacementSessionCompositionManager(t, pinned.Receipt, replacementMutation{})
	bound, err := bindSessionComposition(ctx, store, manager, session.ID, plugins.ResolvedComposition{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bound.Resolved.Receipt, pinned.Receipt) {
		t.Fatalf("resumed receipt = %#v, want original %#v", bound.Resolved.Receipt, pinned.Receipt)
	}
	assertCompositionToolResult(t, bound.Resolved.Toolset, "finance_sync", "replacement finance.sync")
	resolutions, err := store.GetCompatibilityResolutions(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolutions) != 1 || resolutions[0].OriginalProvider.ID != "finance" ||
		resolutions[0].OriginalProvider.ImplementationVersion != "1.0.0" ||
		resolutions[0].ReplacementImplementationVersion != "1.1.0" ||
		resolutions[0].KernelAPIVersion != plugins.KernelAPIVersion ||
		len(resolutions[0].Capabilities) != 3 || resolutions[0].ResolvedAt.IsZero() {
		t.Fatalf("Compatibility Resolutions = %#v, want exact Finance substitution evidence", resolutions)
	}
	if _, err := bindSessionComposition(ctx, store, manager, session.ID, plugins.ResolvedComposition{}, nil); err != nil {
		t.Fatal(err)
	}
	resolutions, err = store.GetCompatibilityResolutions(ctx, session.ID)
	if err != nil || len(resolutions) != 1 {
		t.Fatalf("resolutions after repeated resume = %#v, %v; want one", resolutions, err)
	}
	stored, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(stored, pinned.Receipt) {
		t.Fatalf("stored receipt after substitution = %#v, %v; want immutable %#v", stored, err, pinned.Receipt)
	}
}

func TestSessionCompositionRealSQLiteReopenBlocksEveryIncompatibleReplacement(t *testing.T) {
	tests := []struct {
		name       string
		mutation   replacementMutation
		want       string
		managerErr bool
	}{
		{name: "undeclared", mutation: replacementMutation{undeclared: true}, want: `loaded version "1.1.0" does not declare it resumable`},
		{name: "schema changing", mutation: replacementMutation{changeSchema: true}, want: `pinned Capability "finance.sync" requires schema`},
		{name: "contract changing", mutation: replacementMutation{changeContract: true}, want: `pinned Capability "finance.sync" requires contract version "1.0.0"`},
		{name: "Kernel incompatible", mutation: replacementMutation{kernelIncompatible: true}, want: `plugin "finance" is incompatible with Kernel API 1.0.0`, managerErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "evie.db")
			pinned := seedPinnedStandardComposition(t, ctx, path)
			db, err := eviedb.OpenDBAt(path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store := eviedb.NewStore(db)
			session := onlySession(t, ctx, db)
			manager, managerErr := newReplacementSessionCompositionManager(t, pinned.Receipt, tc.mutation)
			if tc.managerErr {
				if managerErr == nil || !strings.Contains(managerErr.Error(), tc.want) {
					t.Fatalf("replacement Manager error = %v, want containing %q", managerErr, tc.want)
				}
				return
			}
			if managerErr != nil {
				t.Fatal(managerErr)
			}
			for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
				if err := manager.SetEnabled(id, true); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := bindSessionComposition(ctx, store, manager, session.ID, plugins.ResolvedComposition{}, nil); err == nil ||
				!strings.Contains(err.Error(), tc.want) {
				t.Fatalf("incompatible reopened resume error = %v, want containing %q", err, tc.want)
			}
			resolutions, err := store.GetCompatibilityResolutions(ctx, session.ID)
			if err != nil || len(resolutions) != 0 {
				t.Fatalf("rejected resume resolutions = %#v, %v; want none", resolutions, err)
			}
		})
	}
}

func TestSessionCompositionMissingPinnedProviderNeverFallsBackAfterSQLiteReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	pinned := seedPinnedStandardComposition(t, ctx, path)
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session := onlySession(t, ctx, db)
	alternate := compatibilityTestPlugin{
		manifest: plugins.Manifest{
			ID: "alternate", ImplementationVersion: "1.0.0",
			KernelCompatibility: plugins.VersionRange{Minimum: "1.0.0", MaximumExclusive: "2.0.0"},
			Capabilities:        []plugins.CapabilityContract{{ID: "alternate.sync", Version: "1.0.0"}},
		},
		capabilities: []plugins.ToolCapability{{
			ID: "alternate.sync", ContractVersion: "1.0.0",
			Tool: plugins.NewFinance().ToolCapabilities()[0].Tool,
		}},
	}
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), alternate)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, "alternate"} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := bindSessionComposition(ctx, store, manager, session.ID, plugins.ResolvedComposition{}, nil); err == nil ||
		!strings.Contains(err.Error(), `pinned provider plugin "finance" version "1.0.0" is not compiled into Evie`) {
		t.Fatalf("missing pinned provider error = %v", err)
	}
	stored, err := store.GetCompositionReceipt(ctx, session.ID)
	if err != nil || !reflect.DeepEqual(stored, pinned.Receipt) {
		t.Fatalf("missing-provider receipt = %#v, %v; want original %#v", stored, err, pinned.Receipt)
	}
}

type replacementMutation struct {
	undeclared         bool
	changeSchema       bool
	changeContract     bool
	kernelIncompatible bool
}

type compatibilityTestPlugin struct {
	manifest     plugins.Manifest
	capabilities []plugins.ToolCapability
}

func (p compatibilityTestPlugin) Manifest() plugins.Manifest { return p.manifest }

func (compatibilityTestPlugin) Start(context.Context) error { return nil }

func (compatibilityTestPlugin) Stop(context.Context) error { return nil }

func (p compatibilityTestPlugin) ToolCapabilities() []plugins.ToolCapability {
	return append([]plugins.ToolCapability(nil), p.capabilities...)
}

func replacementSessionCompositionManager(
	t *testing.T,
	receipt composition.Receipt,
	mutation replacementMutation,
) *plugins.Manager {
	t.Helper()
	manager, err := newReplacementSessionCompositionManager(t, receipt, mutation)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

func newReplacementSessionCompositionManager(
	t *testing.T,
	receipt composition.Receipt,
	mutation replacementMutation,
) (*plugins.Manager, error) {
	t.Helper()
	finance := plugins.NewFinance()
	manifest := finance.Manifest()
	manifest.ImplementationVersion = "1.1.0"
	capabilities := finance.ToolCapabilities()
	for i := range capabilities {
		capabilityID := capabilities[i].ID
		capabilities[i].Tool.Execute = func(context.Context, string) (string, error) {
			return "replacement " + string(capabilityID), nil
		}
	}
	if mutation.changeSchema {
		capabilities[0].Tool.Schema.Function.Description = "changed across incompatible upgrade"
	}
	if mutation.changeContract {
		manifest.Capabilities[0].Version = "1.1.0"
		capabilities[0].ContractVersion = "1.1.0"
	}
	if mutation.kernelIncompatible {
		manifest.KernelCompatibility = plugins.VersionRange{Minimum: "2.0.0", MaximumExclusive: "3.0.0"}
	}
	if !mutation.undeclared {
		declaration := plugins.ImplementationCompatibility{ImplementationVersion: "1.0.0"}
		for _, capability := range capabilities {
			declaration.Capabilities = append(declaration.Capabilities, plugins.CapabilityCompatibility{
				ID: capability.ID, ContractVersion: capability.ContractVersion,
				SchemaSHA256: testToolSchemaHash(capability.Tool.Schema),
			})
		}
		manifest.ResumableFrom = []plugins.ImplementationCompatibility{declaration}
	}
	return plugins.NewManager(
		tools.KernelToolset(), plugins.NewWeb(), plugins.NewYouTube(), plugins.NewTodo(testTaskStore(t)),
		compatibilityTestPlugin{manifest: manifest, capabilities: capabilities},
	)
}

func seedPinnedStandardComposition(t *testing.T, ctx context.Context, path string) plugins.ResolvedComposition {
	return seedPinnedStandardCompositionWithEvieVersion(t, ctx, path, plugins.EvieVersion)
}

func seedPinnedStandardCompositionWithEvieVersion(
	t *testing.T,
	ctx context.Context,
	path string,
	evieVersion string,
) plugins.ResolvedComposition {
	t.Helper()
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store := eviedb.NewStore(db)
	pinned, err := sessionCompositionManager(t).ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	pinned.Receipt.EvieVersion = evieVersion
	if _, err := store.CreateGlobalSessionWithComposition(ctx, pinned.Receipt); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return pinned
}

func onlySession(t *testing.T, ctx context.Context, db *sql.DB) memory.Session {
	t.Helper()
	var id memory.SessionID
	if err := db.QueryRowContext(ctx, `SELECT id FROM sessions`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return memory.Session{ID: id}
}

func testToolSchemaHash(schema openrouter.Tool) string {
	encoded, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func assertCompositionToolResult(t *testing.T, toolset tools.Toolset, name, want string) {
	t.Helper()
	message, isErr, err := toolset.ExecuteWithApprovalAuthorizedCompletion(
		context.Background(),
		openrouter.ToolCall{ID: "call", Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: `{}`}},
		nil, nil, nil, nil,
	)
	if err != nil || isErr || message.Content != want {
		t.Fatalf("execute %q = (%+v, %v, %v), want %q", name, message, isErr, err, want)
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

func (echoCompositionResolver) ResumeCompositionContext(
	_ context.Context,
	receipt composition.Receipt,
) (plugins.ResolvedComposition, error) {
	return plugins.ResolvedComposition{Receipt: receipt}, nil
}

type cancellationAwareCompositionResolver struct{}

func (cancellationAwareCompositionResolver) ResumeCompositionContext(
	ctx context.Context,
	_ composition.Receipt,
) (plugins.ResolvedComposition, error) {
	return plugins.ResolvedComposition{}, ctx.Err()
}

type panicCompositionStore struct{}

func (panicCompositionStore) SaveCompositionReceipt(context.Context, memory.SessionID, composition.Receipt) error {
	panic("unexpected receipt save")
}

func (panicCompositionStore) GetCompositionReceipt(context.Context, memory.SessionID) (composition.Receipt, error) {
	panic("unexpected receipt read")
}

func (panicCompositionStore) AppendCompatibilityResolutions(
	context.Context,
	memory.SessionID,
	[]composition.CompatibilityResolution,
) error {
	panic("canceled resolution must not be recorded")
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

func (r *countingSessionCompositionResolver) ResumeCompositionContext(
	ctx context.Context,
	receipt composition.Receipt,
) (plugins.ResolvedComposition, error) {
	r.resumeCalls++
	return r.manager.ResumeCompositionContext(ctx, receipt)
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
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(testTaskStore(t)))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
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
