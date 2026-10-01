package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type changedRepositoryHistory struct{ fakeHistory }

func (*changedRepositoryHistory) RepositoryInstructions(context.Context, memory.TurnLease, memory.EventID) (memory.RepositoryInstructionSnapshot, error) {
	return memory.RepositoryInstructionSnapshot{}, memory.ErrRepositoryInstructionsChanged
}

func TestRepositoryInstructionConflictCompletesWithoutProvider(t *testing.T) {
	history := &changedRepositoryHistory{}
	client := &fakeClient{}
	session := New(client, testContextProfile("test-model"), history, memory.ScopeContext{OwnerID: memory.LocalOwnerID, SessionID: "test-session"}, newFakeTurnOwner())
	err := session.Send(context.Background(), "hello", &recorder{}, nil)
	if !errors.Is(err, memory.ErrRepositoryInstructionsChanged) || len(client.reqs) != 0 || len(history.events) != 2 {
		t.Fatalf("conflict=%v provider=%d events=%+v", err, len(client.reqs), history.events)
	}
	terminal := history.events[1]
	var payload memory.TurnTerminalPayload
	if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if terminal.Type != memory.EventTurnFailed || payload.Classification != memory.ClassificationRepositoryInstructions || payload.Stage != memory.StageContextCompose || terminal.Content != payload.SafeContent() {
		t.Fatalf("conflict terminal=%+v payload=%+v", terminal, payload)
	}
}

func TestRepositoryInstructionsCountedAndKeptOutsideConversation(t *testing.T) {
	profile := testContextProfile("test/model")
	input := ContextComposeInput{Profile: profile, Events: []memory.Event{{ID: "turn", Sequence: 1, Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "current owner request"}}, ActiveRootID: "turn", TriggerEventID: "turn", Iteration: 1, WorkingContext: "task data", RepositoryInstructions: "root guide", RepositoryInstructionsTurnID: "turn"}
	composer := NewContextComposer(CanonicalRequestEstimator{})
	result, err := composer.Compose(input)
	if err != nil {
		t.Fatal(err)
	}
	messages := result.Request.Messages
	// Repository guidance is stable and leads; Task Focus is volatile and trails.
	if len(messages) != 4 || messages[0].Content != systemPrompt || messages[1].Content != "root guide" || messages[2].Content != "current owner request" || messages[3].Content != "task data" {
		t.Fatalf("message ordering=%+v", messages)
	}
	estimate, err := (CanonicalRequestEstimator{}).Estimate(result.Request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.SerializedBytes != estimate.SerializedBytes || result.Snapshot.RequestSHA256 != estimate.RequestSHA256 {
		t.Fatal("instructions omitted from estimate/hash")
	}
	data, _ := json.Marshal(result.Snapshot)
	if strings.Contains(string(data), "root guide") || result.Snapshot.RepositoryInstructionsTurnID != "turn" {
		t.Fatal("manifest content/reference wrong")
	}
	input.RepositoryInstructions = strings.Repeat("repository guide ", 2000)
	input.Profile, err = openrouter.NewExplicitContextProfile("test/model", 13000, 13000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = composer.Compose(input); !IsContextOverflow(err) {
		t.Fatalf("expected overflow instead of omitted instructions: %v", err)
	}
}
