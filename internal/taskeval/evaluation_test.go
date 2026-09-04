package taskeval

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/task"
)

func TestDeriveReportsDeterministicTaskAndEpisodicEvidenceCounts(t *testing.T) {
	input := Input{
		SessionScopes: map[memory.SessionID]task.Scope{"owner": task.ScopeGlobal},
		Tasks: []task.Task{
			{ID: "root", Status: task.StatusCompleted, Title: "release"},
			{ID: "verified-claim", Status: task.StatusCompleted, Title: "tests pass secret-marker"},
			{ID: "abandoned", Status: task.StatusCancelled, Title: "superseded"},
		},
		TaskEvents: []task.Event{
			{ID: "decompose-event", TaskID: "root", Operation: task.OperationDecompose, Outcome: task.MutationAccepted, SessionID: "owner", RunID: "decompose-run"},
			{ID: "stale-event", TaskID: "root", Operation: task.OperationUpdate, Outcome: task.MutationRejected, DiagnosticCode: task.DiagnosticRevisionConflict, SessionID: "owner", RunID: "stale-run"},
			{ID: "recovery-event", TaskID: "root", Operation: task.OperationRelease, Outcome: task.MutationAccepted, ClaimReason: "recovery", SessionID: "owner", RunID: "recovery-run"},
		},
		EpisodicEvents: []memory.Event{
			intentEvent(t, "intent-decompose", "owner", "decompose-run", "todo_decompose", `{"task_id":"root","idempotency_key":"decompose-key"}`),
			{ID: "approval-decompose", SessionID: "owner", ExecutionID: "decompose-run", Type: memory.EventApproval},
			{ID: "outcome-decompose", SessionID: "owner", ExecutionID: "decompose-run", Type: memory.EventToolSucceeded},
			intentEvent(t, "intent-retry-1", "owner", "retry-run-1", "todo_update", `{"task_id":"root","expected_revision":1,"title":"same","idempotency_key":"retry-key"}`),
			{ID: "outcome-retry-1", SessionID: "owner", ExecutionID: "retry-run-1", Type: memory.EventToolSucceeded},
			intentEvent(t, "intent-retry-2", "owner", "retry-run-2", "todo_update", `{"idempotency_key":"retry-key","title":"same","expected_revision":1,"task_id":"root"}`),
			{ID: "outcome-retry-2", SessionID: "owner", ExecutionID: "retry-run-2", Type: memory.EventToolSucceeded},
			intentEvent(t, "intent-conflict-1", "owner", "conflict-run-1", "todo_add", `{"title":"first","idempotency_key":"conflict-key"}`),
			{ID: "outcome-conflict-1", SessionID: "owner", ExecutionID: "conflict-run-1", Type: memory.EventToolSucceeded},
			intentEvent(t, "intent-conflict-2", "owner", "conflict-run-2", "todo_add", `{"title":"changed","idempotency_key":"conflict-key"}`),
			{ID: "outcome-conflict-2", SessionID: "owner", ExecutionID: "conflict-run-2", Type: memory.EventToolFailed},
			intentEvent(t, "intent-default-zero-1", "owner", "default-zero-run-1", "todo_add", `{"title":"equivalent","idempotency_key":"default-zero-key"}`),
			{ID: "outcome-default-zero-1", SessionID: "owner", ExecutionID: "default-zero-run-1", Type: memory.EventToolSucceeded},
			intentEvent(t, "intent-default-zero-2", "owner", "default-zero-run-2", "todo_add", `{"title":"equivalent","priority":0,"idempotency_key":"default-zero-key"}`),
			{ID: "outcome-default-zero-2", SessionID: "owner", ExecutionID: "default-zero-run-2", Type: memory.EventToolSucceeded},
			{ID: "hidden-reasoning", SessionID: "owner", Type: memory.EventAssistantMessage, Content: "private reasoning secret-marker"},
		},
	}

	report, err := Derive(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != ReportSchemaVersion || report.TaskCount != 3 || report.CompletionCount != 2 || report.AbandonmentCount != 1 ||
		report.DecompositionCount != 1 || report.RecoveryCount != 1 || report.DuplicateAttemptCount != 3 ||
		report.RetryCount != 2 || report.ConflictCount != 2 {
		t.Fatalf("report counts = %+v", report)
	}
	if report.EvidenceLinkCount != 1 || len(report.EvidenceLinks) != 1 || report.EvidenceLinks[0] != (EvidenceLink{
		TaskEventID: "decompose-event", SessionID: "owner", ExecutionID: "decompose-run",
		ToolName:      "todo_decompose",
		IntentEventID: "intent-decompose", ApprovalEventID: "approval-decompose",
		OutcomeEventID: "outcome-decompose", OutcomeType: memory.EventToolSucceeded,
	}) {
		t.Fatalf("evidence links = %+v", report.EvidenceLinks)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"secret-marker", "retry-key", "conflict-key", "private reasoning"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("report retained sensitive source content %q: %s", forbidden, encoded)
		}
	}
	reversed := input
	reversed.Tasks = reverse(input.Tasks)
	reversed.TaskEvents = reverse(input.TaskEvents)
	reversed.EpisodicEvents = reverse(input.EpisodicEvents)
	reordered, err := Derive(reversed)
	if err != nil || !reflect.DeepEqual(reordered, report) {
		t.Fatalf("reordered report = %+v, %v; want %+v", reordered, err, report)
	}
}

func reverse[T any](values []T) []T {
	reversed := append([]T(nil), values...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}

func intentEvent(t *testing.T, id, sessionID, executionID, name, arguments string) memory.Event {
	t.Helper()
	payload, err := json.Marshal(memory.ToolIntentPayload{Call: memory.ToolCall{
		ID: id + "-call", Name: name, Arguments: arguments,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return memory.Event{
		ID: memory.EventID(id), SessionID: memory.SessionID(sessionID), ExecutionID: memory.ExecutionID(executionID),
		Type: memory.EventToolIntent, Payload: payload,
	}
}
