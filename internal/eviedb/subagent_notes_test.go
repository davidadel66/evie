package eviedb_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/memory"
)

// Harness notes name read_subagent_report only to a parent whose pinned
// composition has it: a session pinned before the tool existed is told only
// that the inline summary was cut.
func TestCutSummaryNoteNamesTheReportToolOnlyWhenTheParentHasIt(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capabilities []string
		names        bool
	}{
		{"pinned_before_reports", []string{delegation.CapabilityID}, false},
		{"with_reports", []string{delegation.CapabilityID, delegation.ReportCapabilityID}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, _, _, parent, receipt := newSubagentAdmissionWith(t, tc.capabilities...)
			policy := delegation.DefaultPolicy()
			policy.ResultBytes = 2048
			attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, policy, "dispatch")
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
			report := "## Summary\n" + strings.Repeat("A long finding. ", 400)
			if _, err = store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: report}); err != nil {
				t.Fatal(err)
			}
			finished, err := store.FinishSubagent(ctx, a.ID, "succeeded", "")
			if err != nil {
				t.Fatal(err)
			}
			notes := strings.Join(finished.Result.Notes, "\n")
			if !finished.Result.SummaryTruncated || !strings.Contains(notes, "inline summary was cut") ||
				strings.Contains(notes, delegation.ReportToolName) != tc.names {
				t.Fatalf("truncated=%t notes=%q", finished.Result.SummaryTruncated, notes)
			}
		})
	}
}
