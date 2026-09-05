package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type diagnosticCLIEmpty struct{}

func (diagnosticCLIEmpty) ServerIdentity() string { return "scripted:diagnostic-cli" }
func (diagnosticCLIEmpty) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	raw, e := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, e
}
func TestCompilerDiagnosticsCLIUsesSameSafeProjection(t *testing.T) {
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "cli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "diagnostic-cli", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "PRIVATE source"})
	if err != nil {
		t.Fatal(err)
	}
	end, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "noted"})
	if err != nil {
		t.Fatal(err)
	}
	g := reviewCLIGeneration()
	compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}, g, diagnosticCLIEmpty{})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	handled, err := runCompilerDiagnostics(ctx, []string{"memory-health", "--scope", "global", "--session", string(session.ID)}, &out, store)
	if !handled || err != nil {
		t.Fatalf("CLI %v %v", handled, err)
	}
	var got memory.CompilerDiagnostics
	if err = json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Jobs) != 1 || got.Jobs[0].JobID != compiled.JobID || got.Counts["jobs_completed_empty"] != 1 || strings.Contains(out.String(), "PRIVATE") {
		t.Fatalf("projection %s", out.String())
	}
	out.Reset()
	if handled, err = runCompilerDiagnostics(ctx, []string{"memory-health", "--scope", "global"}, &out, store); !handled || err != nil || !strings.Contains(out.String(), string(session.ID)) {
		t.Fatalf("navigation %v %v %s", handled, err, out.String())
	}
	for _, args := range [][]string{{"memory-health"}, {"memory-health", "--scope", "global", "--session", string(session.ID), "--limit", "33"}, {"memory-health", "--scope", "global", "--view", "raw"}} {
		out.Reset()
		if handled, err = runCompilerDiagnostics(ctx, args, &out, store); !handled || err == nil || out.Len() != 0 {
			t.Fatalf("invalid %+v %v %v %s", args, handled, err, out.String())
		}
	}
}
