package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

type clockCLIClient struct{ calls int }

func (c *clockCLIClient) ChatStream(context.Context, openrouter.ChatRequest, openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.calls++
	message := openrouter.Message{Role: "assistant", Content: "The runtime clock displayed the checked local date."}
	if c.calls == 1 {
		message.Content = ""
		message.ToolCalls = []openrouter.ToolCall{{ID: "clock-call", Type: "function", Function: openrouter.FunctionCall{Name: "get_time", Arguments: "{}"}}}
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: message}}}, nil
}

type clockCLIEvents struct{}

func (clockCLIEvents) Delta(string)                                  {}
func (clockCLIEvents) Reasoning(string)                              {}
func (clockCLIEvents) ReasoningDone()                                {}
func (clockCLIEvents) AssistantDone(string)                          {}
func (clockCLIEvents) ToolCall(string, string, string)               {}
func (clockCLIEvents) ToolResult(string, string, bool)               {}
func (clockCLIEvents) ResponseDiscarded(agent.DiscardReason, string) {}

type clockCLIExtractor struct {
	subject, predicate memory.SemanticID
	calls              int
}

func (e *clockCLIExtractor) ServerIdentity() string { return "scripted-clock-cli" }
func (e *clockCLIExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	e.calls++
	c := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: e.subject, PredicateID: e.predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea adopted on the checked local date 2026-09-04"}}, Polarity: memory.PolarityAffirmed}, ValidTime: memory.ValidTime{}, TemporalQualification: "The runtime local calendar date; timezone and effective instant remain unknown.", Support: []memory.EvidenceLocator{}, Context: []memory.EvidenceLocator{}}
	for _, s := range r.Window.Sources {
		if s.Usage != "new_support" {
			continue
		}
		if s.SourceType == memory.SourceTypeUserMessage {
			c.Support = append(c.Support, s.Locator)
		}
		if s.SourceType == "tool_succeeded" {
			c.Support = append(c.Support, memory.EvidenceLocator{EventID: s.Locator.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: "0:10", EvidenceSHA256: "ec00d6c3e1a390cb687d96168d38fbb1c79e6fcd9e3d1193448e5bc2dea06efa"})
		}
	}
	raw, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{c}})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, err
}

func TestOwnerReviewClockCLIActualCommittedToolPath(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "clock-seed", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink tea."})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000441", SourceEventID: seed.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, explicit); err != nil {
		t.Fatal(err)
	}
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	client := &clockCLIClient{}
	toolCalls := 0
	clock := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "get_time"}}, Execute: func(_ context.Context, args string) (string, error) {
		toolCalls++
		if args != "{}" {
			t.Fatalf("arguments %s", args)
		}
		return "2026-09-04 11:42:00", nil
	}}
	profile, err := openrouter.NewExplicitContextProfile("clock-fixture", 300000, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.NewWithToolset(client, profile, store.BindHistory(session.ID, "clock-runtime"), session.ScopeContext(), store.BindTurnOwner(session.ID, "clock-runtime"), tools.NewToolset([]tools.Tool{clock}))
	if err = runtime.Send(ctx, "Check the local date. Using that checked date, today I adopted tea as my standing drink.", clockCLIEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var root, outcome memory.Event
	approvals := 0
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			root = event
		}
		if event.Type == memory.EventToolSucceeded {
			outcome = event
		}
		if event.Type == memory.EventApproval {
			approvals++
		}
	}
	if outcome.ID == "" || toolCalls != 1 || approvals != 0 {
		t.Fatalf("committed tool path outcome=%s calls=%d approvals=%d", outcome.ID, toolCalls, approvals)
	}
	g := reviewCLIGeneration()
	g.EvidencePolicy = "owner-clock-observations-v2"
	extractor := &clockCLIExtractor{subject: explicit.Subject.ID, predicate: explicit.Predicate.ID}
	compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: events[len(events)-1].Sequence, Destination: "global"}, g, extractor)
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile %s %s %v", compiled.State, compiled.Reason, err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	run := func(target any, args ...string) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
		if !handled || err != nil {
			t.Fatalf("CLI %v: %v", args, err)
		}
		if err = json.Unmarshal(out.Bytes(), target); err != nil {
			t.Fatal(err)
		}
	}
	var page memory.OwnerCandidatePage
	run(&page, "inbox", "--scope", "global")
	if len(page.Candidates) != 1 {
		t.Fatal("missing clock candidate")
	}
	item := page.Candidates[0]
	var preview memory.ReviewPreview
	run(&preview, "prepare", "--scope", "global", "--id", item.Ref.ID, "--revision", fmt.Sprint(item.Ref.ReviewRevision), "--action", "accept")
	if preview.Version != "owner-review-preview-v4" || len(preview.Effect.Claims[0].Sources) != 2 {
		t.Fatalf("preview %+v", preview)
	}
	var result memory.ReviewResult
	run(&result, "resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000442", "--action", "accept")
	var operation memory.OwnerReviewOperation
	run(&operation, "operation", "--scope", "global", "--id", string(result.Operation.OperationID))
	found := false
	for _, source := range operation.Preview.Effect.Claims[0].Sources {
		if source.EventID == outcome.ID {
			found = true
			if source.Evidence != "2026-09-04" || source.Authority != "tool_observation" || source.Actor != "tool" || source.SourceType != "tool_succeeded" {
				t.Fatalf("original clock authority: %+v", source)
			}
		}
	}
	if !found {
		t.Fatal("missing clock provenance")
	}
	if verified, err := store.VerifySemanticProjection(ctx); err != nil || !verified.Valid {
		t.Fatalf("replay %+v %v", verified, err)
	}
	if toolCalls != 1 || client.calls != 2 || extractor.calls != 1 {
		t.Fatalf("review/replay invoked model or tool: tool=%d model=%d extractor=%d", toolCalls, client.calls, extractor.calls)
	}
}
