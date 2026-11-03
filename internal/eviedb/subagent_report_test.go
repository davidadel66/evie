package eviedb_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// The owner's report is the stored final report the parent model pages
// through read_subagent_report, byte for byte.
func TestSubagentReportIsTheReportTheParentPages(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	report := "## Summary\nHorizons double. <script>alert(1)</script>\n\n## Details\n" + strings.Repeat("détail ✓ ", 6000)
	a, _ := finishedChild(t, store, parent, receipt, report)

	var paged strings.Builder
	for offset := 0; ; {
		page, err := store.ReadSubagentReport(ctx, parent, a.ID, offset, 32*1024)
		if err != nil {
			t.Fatal(err)
		}
		paged.WriteString(page.Text)
		if page.NextOffset == nil {
			break
		}
		offset = *page.NextOffset
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text == nil || *got.Text != paged.String() || *got.Text != report {
		t.Fatalf("owner report differs from the paged report")
	}
	if got.ExecutionID != a.ID || got.Worker != delegation.WorkerResearch || got.Status != "succeeded" || got.Reason != "" ||
		got.TotalBytes != len(report) || got.OmittedBytes != 0 {
		t.Fatalf("report document = %+v", got)
	}
}

// Only the chat that delegated a child opens its report.
func TestSubagentReportStaysWithinTheParentSession(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	a, _ := finishedChild(t, store, parent, receipt, "## Summary\nsecret-sentinel")
	other, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		session memory.SessionID
		id      string
	}{
		{"another chat's worker", other.ID, a.ID},
		{"the worker's own session", a.Child.ID, a.ID},
		{"an unknown execution", parent.Scope.SessionID, "missing"},
	} {
		got, err := store.SubagentReport(ctx, tc.session, tc.id)
		if !errors.Is(err, sql.ErrNoRows) || got.Text != nil {
			t.Fatalf("%s: %+v %v", tc.name, got, err)
		}
	}
}

// A worker a Workflow Run owns is not the chat's to open, even if its record
// named the chat as its scope: run pages have their own views.
func TestSubagentReportOfARunOwnedWorkerIsNotFound(t *testing.T) {
	ctx := context.Background()
	store, db, _, parent, receipt := newSubagentAdmission(t)
	a, _ := finishedChild(t, store, parent, receipt, "## Summary\nrun-sentinel")
	if _, err := db.Exec(`UPDATE subagent_executions SET record_json=json_set(record_json,'$.run',json(?)) WHERE id=?`,
		`{"run_id":"run-1","workspace_id":"ws","workflow":"fix-bugs","step":"research","attempt_seq":1,"holder":"runtime","token":1}`, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if !errors.Is(err, sql.ErrNoRows) || got.Text != nil {
		t.Fatalf("run-owned worker read: %+v %v", got, err)
	}
}

// A child without a final report is not an error: the owner sees how it
// ended, or that it is still running, and no report text.
func TestSubagentReportWithoutAFinalReportStatesHowTheChildEnded(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	running, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || running.Status != "running" || running.Reason != "" || running.Text != nil || running.TotalBytes != 0 {
		t.Fatalf("running child: %+v %v", running, err)
	}
	if _, err = store.FinishSubagent(ctx, a.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	ended, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || ended.ExecutionID != a.ID || ended.Status != "interrupted" || ended.Reason != "no_accepted_final_answer" || ended.Text != nil || ended.TotalBytes != 0 {
		t.Fatalf("child without a report: %+v %v", ended, err)
	}
}

// A partial report opens with the reason its child stopped early.
func TestSubagentReportOfAPartialReportCarriesItsReason(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	lease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "## Summary\nhalf done"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FinishSubagent(ctx, a.ID, delegation.StatePartial, delegation.WrapUpTime); err != nil {
		t.Fatal(err)
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || got.Status != delegation.StatePartial || got.Reason != delegation.WrapUpTime || got.Text == nil || *got.Text != "## Summary\nhalf done" {
		t.Fatalf("partial report: %+v %v", got, err)
	}
}

// A report over 1 MiB is cut at a UTF-8 boundary and says how much is left
// out; the parent's pages are unaffected.
func TestSubagentReportOverOneMebibyteStatesTheBytesLeftOut(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	// Two-byte runes start at odd offsets after the 11-byte heading, so the
	// 1 MiB mark falls inside one and the cut steps back a byte.
	report := "## Summary\n" + strings.Repeat("é", 600_000)
	a, _ := finishedChild(t, store, parent, receipt, report)
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	const kept = 1<<20 - 1
	if got.Text == nil || len(*got.Text) != kept || *got.Text != report[:kept] || !utf8.ValidString(*got.Text) ||
		got.TotalBytes != 1_200_011 || got.OmittedBytes != 1_200_011-kept {
		t.Fatalf("cut report: text %d bytes, total %d, left out %d", len(*got.Text), got.TotalBytes, got.OmittedBytes)
	}
}
