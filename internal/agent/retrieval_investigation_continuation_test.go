package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

func investigationReceipts(t *testing.T, f *retrievalFixture, session memory.Session) []memory.RetrievalReceipt {
	t.Helper()
	events, err := f.store.LoadEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipts []memory.RetrievalReceipt
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil {
			receipts = append(receipts, *snapshot.Memory)
		}
	}
	return receipts
}

func TestMemoryInvestigationReusesUnchangedEvidenceWithExactDeliveryAccounting(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := f.remember(source, memory.MemoryEverywhere, "tourmaline envelope")
	f.refresh()
	check := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "continue_check", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) { return "Checked the current request.", nil }}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("search", "memory_search", `{"query":"tourmaline"}`)),
		assistantStep("", nil, toolCall("check-1", "continue_check", `{}`)),
		assistantStep("", nil, toolCall("check-2", "continue_check", `{}`)),
		assistantStep("The original tourmaline source remains valid.", nil),
	}}
	if err := f.session(reader, client, check).Send(context.Background(), "Check the envelope, then verify the current request twice.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	receipts := investigationReceipts(t, f, reader)
	if len(receipts) != 3 {
		t.Fatalf("receipts=%d", len(receipts))
	}
	cumulative := 0
	for i, receipt := range receipts {
		if len(receipt.Evidence) != 1 || receipt.Evidence[0].ClaimID != saved.ClaimID {
			t.Fatalf("original source lost: %+v", receipt)
		}
		if !reflect.DeepEqual(receipt.Evidence, receipts[0].Evidence) {
			t.Fatal("unchanged continuation repinned or changed immutable references")
		}
		for _, message := range client.reqs[i+1].Messages {
			if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") || message.Role == "tool" && message.ToolCallID == "search" {
				raw, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				cumulative += len(raw)
			}
		}
		accounting := receipt.Investigation
		reused := 0
		if i > 0 {
			reused = 1
		}
		if accounting == nil || accounting.SearchAttempts != 1 || accounting.RefreshAttempts != 0 || accounting.ReusedEvidence != reused || accounting.CumulativeMemoryBytes != cumulative || accounting.KernelWorkNanoseconds <= 0 || accounting.KernelWorkNanoseconds > int64(3*time.Second) {
			t.Fatalf("receipt %d has inaccurate accounting: %+v, actual bytes=%d", i, accounting, cumulative)
		}
	}
}

func TestMemoryInvestigationCancellationAndLeaseLossHaveNoLateEffects(t *testing.T) {
	for _, mode := range []string{"cancel", "lease loss"} {
		t.Run(mode, func(t *testing.T) {
			f := newRetrievalFixture(t)
			source, reader := f.global(), f.global()
			saved := f.remember(source, memory.MemoryEverywhere, "spinel evidence")
			f.refresh()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var lease memory.TurnLease
			client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
				if call == 0 {
					return assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"spinel"}`))
				}
				if len(expansionBoundaryEvidence(t, request)) != 1 {
					t.Fatal("fixture lacks previously delivered evidence")
				}
				if mode == "cancel" {
					cancel()
				} else {
					current, err := f.store.GetTurnLease(context.Background(), reader.ID)
					if err != nil {
						t.Fatal(err)
					}
					if err := f.store.ReleaseTurnLease(context.Background(), reader.ID, current.HolderID, current.FencingToken); err != nil {
						t.Fatal(err)
					}
					lease, err = f.store.AcquireTurnLease(context.Background(), reader.ID, "replacement-investigator", time.Minute)
					if err != nil {
						t.Fatal(err)
					}
				}
				return assistantStep("", nil, toolCall("late-search", "memory_search_conversations", `{"query":"spinel"}`), toolCall("late-retire", "memory_retire", `{"object_kind":"claim","object_id":"`+string(saved.ClaimID)+`","idempotency_key":"idem:v1:unused"}`))
			}}
			approvals := 0
			err := f.session(reader, client).Send(ctx, "Investigate the spinel evidence.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
				approvals++
				return tools.Approved
			})
			if err == nil || mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("lost turn returned %v", err)
			}
			if len(client.reqs) != 2 || approvals != 0 {
				t.Fatalf("late effects: provider requests=%d approvals=%d", len(client.reqs), approvals)
			}
			receipts := investigationReceipts(t, f, reader)
			if len(receipts) != 1 || len(receipts[0].Evidence) != 1 || receipts[0].Evidence[0].ClaimID != saved.ClaimID || receipts[0].Investigation.SearchAttempts != 1 {
				t.Fatalf("late receipt or altered original: %+v", receipts)
			}
			claim, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, saved.ClaimID)
			if err != nil || claim.Status != memory.SemanticStatusActive {
				t.Fatalf("late memory mutation: %+v %v", claim, err)
			}
			if mode == "lease loss" {
				current, err := f.store.GetTurnLease(context.Background(), reader.ID)
				if err != nil || current != lease {
					t.Fatalf("stale cleanup changed replacement lease: %+v %v", current, err)
				}
				if err := f.store.ReleaseTurnLease(context.Background(), reader.ID, lease.HolderID, lease.FencingToken); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
