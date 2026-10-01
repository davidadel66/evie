package eviedb

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
)

// C8: after a provider context-length rejection, one compact-and-retry is a
// valid durable shape: trigger, rejected request snapshot, automatic
// compaction, retry snapshot. A retry without compaction or a second retry
// for the same trigger stays rejected.
func TestContextSnapshotRetryAfterRejectionRequiresAutomaticCompaction(t *testing.T) {
	store := NewStore(newTestDB(t))
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := store.appendEventForTest(ctx, session.ID, input)
		if err != nil {
			t.Fatalf("append %s: %v", input.Type, err)
		}
		return event
	}
	snapshotInput := func(payload memory.ContextSnapshotPayload, parent memory.EventID) memory.EventInput {
		t.Helper()
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		return memory.EventInput{ParentID: parent, Type: memory.EventContextSnapshot, Payload: encoded}
	}
	firstRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "first"})
	firstAssistant := appendEvent(memory.EventInput{
		ParentID: firstRoot.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant,
		Content: "answer", Payload: json.RawMessage(`{}`),
	})
	active := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "active"})
	appendEvent(snapshotInput(validContextSnapshotPayload(firstRoot, active), active.ID))

	summary := validContextCompactionSummary()
	digest := sha256.Sum256([]byte(summary))
	compactionJSON, err := json.Marshal(memory.ContextCompactedPayload{
		SchemaVersion: memory.ContextCompactedSchemaVersion, Generation: 1,
		Trigger:             memory.ContextCompactionAutomatic,
		CoveredFirstEventID: firstRoot.ID, CoveredFirstSequence: firstRoot.Sequence,
		CoveredLastEventID: firstAssistant.ID, CoveredLastSequence: firstAssistant.Sequence,
		FirstRetainedEventID: active.ID, CanonicalModel: "test/model",
		PromptVersion: memory.ContextCompactionPromptVersion, SummaryBytes: int64(len(summary)),
		SummarySHA256: fmt.Sprintf("%x", digest),
	})
	if err != nil {
		t.Fatal(err)
	}
	compacted := appendEvent(memory.EventInput{Type: memory.EventContextCompacted, Content: summary, Payload: compactionJSON})

	retry := validContextSnapshotPayload(active, active)
	retry.ActiveCompactionEventID = compacted.ID
	retry.SummaryMessageBytes = int64(len(summary))
	appendEvent(snapshotInput(retry, active.ID))

	if _, err := store.appendEventForTest(ctx, session.ID, snapshotInput(retry, active.ID)); err == nil {
		t.Fatal("a second retry snapshot for the same trigger was accepted")
	}
}
