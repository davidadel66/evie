package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type cliHistoryScript struct{}

func (*cliHistoryScript) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}
func (*cliHistoryScript) ServerIdentity() string { return "scripted:history-cli" }
func (*cliHistoryScript) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	data, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}})
	return eviedb.CompilerExtraction{Raw: data, ReleaseEvidence: "completed"}, err
}

func TestCompilerHistoryCLIMultiScopeSelectionProgressCancellationAndReopen(t *testing.T) {
	f := newCompilerRuntimeFixture(t, nil)
	ctx := context.Background()
	global := f.appendOwnerEvent(t, "Global private source.")
	project, err := f.store.RegisterProject(ctx, "Selected project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, "history-cli", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Project private source."})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	sourceScope := "project:" + string(project.ID)
	req := memory.CompilerHistoryRequest{RequestID: "cli-history", Ranges: []memory.CompilerHistoryRange{
		{SourceScope: "global", Destination: "global", SessionID: f.session.ID, FirstSequence: global.Sequence, LastSequence: global.Sequence, FirstEventID: global.ID, LastEventID: global.ID},
		{SourceScope: sourceScope, Destination: sourceScope, SessionID: session.ID, FirstSequence: scoped.Sequence, LastSequence: scoped.Sequence, FirstEventID: scoped.ID, LastEventID: scoped.ID},
	}}
	path := filepath.Join(t.TempDir(), "selection.json")
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	command := func(action string, args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		all := append([]string{"memory-backfill", action, "--selection", path}, args...)
		handled, err := runCompilerHistoryManagement(ctx, all, &out, f.store)
		if !handled || err != nil {
			t.Fatalf("%s %v %v", action, handled, err)
		}
		for _, secret := range []string{"private source", f.generation.Prompt, f.server.URL} {
			if strings.Contains(out.String(), secret) {
				t.Fatal("metadata leaked source/configuration")
			}
		}
		return out.Bytes()
	}
	selected := command("select", "--config", f.configPath)
	if string(command("select", "--config", f.configPath)) != string(selected) {
		t.Fatal("CLI idempotent receipt changed")
	}
	var receipt memory.CompilerHistoryReceipt
	if err := json.Unmarshal(selected, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SelectedEvents != 2 || len(receipt.Request.Ranges) != 2 {
		t.Fatalf("receipt %+v", receipt)
	}
	// Each source authorization is independent; a global context cannot authorize
	// the project source even when both occur in an otherwise valid request.
	if _, err := f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{f.session.ScopeContext()}, req, f.generation, &cliHistoryScript{}); err == nil {
		t.Fatal("missing exact scope authorization")
	}
	id, _, err := memory.CompilerGenerationIdentity(f.generation)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{id: &cliHistoryScript{}}}
	for range 6 {
		if _, err := f.store.ReconcileCompilerHistory(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
			t.Fatal(worked, err)
		}
	}
	for index, arg := range []string{"0", "1"} {
		var progress memory.CompilerHistoryProgress
		if err := json.Unmarshal(command("status", "--range", arg, "--limit", "1"), &progress); err != nil {
			t.Fatal(err)
		}
		if progress.RangeIndex != index || len(progress.Intervals) != 1 || progress.Intervals[0].State != "completed_empty" || progress.ContiguousFrontier != 1 {
			t.Fatalf("scope progress %+v", progress)
		}
	}
	command("cancel", "--operation", "cli-cancel", "--revision", "1")
	command("resume", "--operation", "cli-resume", "--revision", "2", "--config", f.configPath)
	var after memory.CompilerHistoryProgress
	if err := json.Unmarshal(command("status"), &after); err != nil {
		t.Fatal(err)
	}
	if after.RequestState.Revision != 3 || after.RequestState.Cancelled || after.Intervals[0].State != "completed_empty" {
		t.Fatal("completion lost", after)
	}
	if f.inferences.Load() != 0 {
		t.Fatal("management command dispatched inference")
	}
	// This is a second Store over the same real SQLite file, not a mock Kernel.
	var dbPath string
	if err := f.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&dbPath); err != nil {
		t.Fatal(err)
	}
	db, err := eviedb.OpenDBAt(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f.store = eviedb.NewStore(db)
	if string(command("select", "--config", f.configPath)) != string(selected) {
		t.Fatal("reopen changed receipt")
	}
	t.Logf("real-SQLite CLI: %d explicit source ranges; two completed_empty frontiers; cancel/resume revision %d; original receipt retained after reopen", len(receipt.Request.Ranges), after.RequestState.Revision)
}
