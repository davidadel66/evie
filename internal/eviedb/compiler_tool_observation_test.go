package eviedb_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

func clockGeneration() memory.CompilerGeneration {
	g := compilerGeneration()
	g.EvidencePolicy = "owner-clock-observations-v2"
	return g
}

type clockFixtureOptions struct{ args, approval string }

func (f *compilerFixture) clockSelection(t *testing.T, content string, options ...clockFixtureOptions) (memory.CompilationSelection, memory.Event) {
	t.Helper()
	args, approval := "{}", ""
	if len(options) > 0 {
		args = options[0].args
		approval = options[0].approval
	}
	call := memory.ToolCall{ID: "clock-call", Name: "get_time", Arguments: args}
	assistantPayload, _ := json.Marshal(memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{call}})
	intentPayload, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
	root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Using the checked local date, today I adopted tea as my standing drink."})
	assistant := f.append(t, memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Payload: assistantPayload})
	intent := f.append(t, memory.EventInput{ParentID: assistant.ID, Type: memory.EventToolIntent, ExecutionID: "clock-execution", Payload: intentPayload})
	parent := intent.ID
	if approval != "" {
		payload, _ := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalDecision(approval)})
		approved := f.append(t, memory.EventInput{ParentID: intent.ID, Type: memory.EventApproval, ExecutionID: "clock-execution", Payload: payload})
		parent = approved.ID
	}
	outcome := f.append(t, memory.EventInput{ParentID: parent, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: "clock-execution", Content: content, Payload: json.RawMessage(`{"tool_call_id":"clock-call","is_error":false}`)})
	last := f.append(t, memory.EventInput{ParentID: outcome.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded the local calendar date without a timezone."})
	return memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, outcome
}

func (f *compilerFixture) clockCandidate(r memory.CompilerRequest) memory.ExtractorCandidate {
	c := f.candidate(r)
	c.Proposition.Object.Literal.Value = "tea adopted on the checked local date 2026-09-04"
	for _, source := range r.Window.Sources {
		if source.SourceType == "tool_succeeded" {
			c.Support = append(c.Support, memory.EvidenceLocator{EventID: source.Locator.EventID, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: "0:10", EvidenceSHA256: "ec00d6c3e1a390cb687d96168d38fbb1c79e6fcd9e3d1193448e5bc2dea06efa"})
		}
	}
	return c
}

func TestCompilerClockObservationAcceptedAuthorityAndReplay(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		c := f.clockCandidate(r)
		if len(c.Support) != 2 {
			t.Fatalf("contracted clock missing from input: %+v", r.Window)
		}
		return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
	}}
	result, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), extractor)
	if err != nil || result.State != "completed_candidates" {
		t.Fatalf("clock compilation: %s %s %v", result.State, result.Reason, err)
	}
	if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	item, err := f.store.InspectOwnerCandidate(ctx, a, result.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, a, item.Ref, "accept")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Version != "owner-review-preview-v4" {
		t.Fatalf("version %s", preview.Version)
	}
	for _, source := range preview.Effect.Claims[0].Sources {
		if source.EventID == outcome.ID && (source.Authority != "tool_observation" || source.Actor != "tool" || source.SourceType != "tool_succeeded" || source.Evidence != "2026-09-04") {
			t.Fatalf("clock authority lost: %+v", source)
		}
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, a, memory.ReviewDecision{PreviewID: preview.ID, PreviewSHA256: preview.SHA256, DeliveryKey: "idem:v1:91000000-0000-4000-8000-000000000443", Action: "accept"})
	if err != nil {
		t.Fatal(err)
	}
	op, err := f.store.InspectOwnerReviewOperation(ctx, a, accepted.Operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Preview.Candidates[0].Candidate.Support) != 2 {
		t.Fatal("accepted provenance incomplete")
	}
	inspecting, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := f.store.InspectSemanticObject(ctx, inspecting.ScopeContext(), memory.SemanticObjectClaim, accepted.Operation.ClaimIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range inspection.Sources {
		source := entry.Source
		if source.EventID == outcome.ID {
			found = true
			if source.Evidence != "2026-09-04" || source.Authority != "tool_observation" {
				t.Fatalf("accepted rendering: %+v", source)
			}
		}
	}
	if !found {
		t.Fatal("missing accepted clock source")
	}
	if replay, err := f.store.VerifySemanticProjection(ctx); err != nil || !replay.Valid {
		t.Fatalf("replay %+v %v", replay, err)
	}
	if extractor.calls.Load() != 1 {
		t.Fatal("replay invoked extraction")
	}
}

// Corruption injection bypasses append-only storage only to exercise admission
// and acceptance against malformed historical databases; public writes remain
// append-only. Assertions observe compiler/review results, never private state.
func mutateClockEvent(t *testing.T, f *compilerFixture, event memory.EventID, assignment string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(`DROP TRIGGER IF EXISTS events_append_only_update`); err != nil {
		t.Fatal(err)
	}
	args = append(args, event)
	if _, err := f.db.Exec(`UPDATE events SET `+assignment+` WHERE id=?`, args...); err != nil {
		t.Fatal(err)
	}
}
func clockControlID(t *testing.T, f *compilerFixture, kind string) memory.EventID {
	t.Helper()
	var id memory.EventID
	if err := f.db.QueryRow(`SELECT id FROM events WHERE event_type=? AND (execution_id='clock-execution' OR event_type='assistant_message' AND payload_json LIKE '%clock-call%') ORDER BY sequence LIMIT 1`, kind).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCompilerClockObservationRejectsMalformedDurableSources(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *compilerFixture, memory.Event)
	}{
		{"missing content", func(t *testing.T, f *compilerFixture, e memory.Event) { mutateClockEvent(t, f, e.ID, "content='' ") }},
		{"invalid calendar date", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", "2026-02-30 11:42:00")
		}},
		{"invalid hour", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", "2026-09-04 24:42:00")
		}},
		{"extra timezone", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", "2026-09-04 11:42:00Z")
		}},
		{"json envelope", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", `{"date":"2026-09-04"}`)
		}},
		{"truncation envelope", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", "2026-09-04 11:42:00 [truncated]")
		}},
		{"nonascii", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "content=?", "２０２６-09-04 11:42:00")
		}},
		{"is error", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "payload_json=?", `{"tool_call_id":"clock-call","is_error":true}`)
		}},
		{"missing is error", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "payload_json=?", `{"tool_call_id":"clock-call"}`)
		}},
		{"duplicate result key", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "payload_json=?", `{"tool_call_id":"clock-call","is_error":true,"is_error":false}`)
		}},
		{"wrong role", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "role='assistant'")
		}},
		{"wrong call", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "payload_json=?", `{"tool_call_id":"other","is_error":false}`)
		}},
		{"missing execution", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "execution_id=NULL")
		}},
		{"mismatched execution", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "execution_id='other'")
		}},
		{"changed argument contract", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, clockControlID(t, f, "tool_intent"), "payload_json=?", `{"call":{"id":"clock-call","name":"get_time","arguments":"{\"zone\":\"UTC\"}"}}`)
		}},
		{"wrong assistant call", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, clockControlID(t, f, "assistant_message"), "payload_json=?", `{"tool_calls":[{"id":"clock-call","name":"other","arguments":"{}"}]}`)
		}},
		{"duplicate assistant calls", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, clockControlID(t, f, "assistant_message"), "payload_json=?", `{"tool_calls":[{"id":"clock-call","name":"get_time","arguments":"{}"},{"id":"clock-call","name":"get_time","arguments":"{}"}]}`)
		}},
		{"missing intent parent", func(t *testing.T, f *compilerFixture, e memory.Event) {
			mutateClockEvent(t, f, e.ID, "parent_id=?", clockControlID(t, f, "assistant_message"))
		}},
		{"duplicate terminal", func(t *testing.T, f *compilerFixture, e memory.Event) {
			f.append(t, memory.EventInput{ParentID: e.ParentID, Type: memory.EventToolFailed, Role: memory.RoleTool, ExecutionID: e.ExecutionID, Content: "failed", Payload: json.RawMessage(`{"tool_call_id":"clock-call","is_error":true}`)})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
			tc.mutate(t, f, outcome)
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				t.Fatal("invalid observation reached extraction")
				return eviedb.CompilerExtraction{}, nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, clockGeneration(), extractor)
			if err == nil && result.State != "failed" {
				t.Fatalf("malformed clock admitted %s %s", result.State, result.Reason)
			}
			if extractor.calls.Load() != 0 {
				t.Fatal("malformed clock dispatched")
			}
		})
	}
}

func TestCompilerClockObservationExactProjectionAndOwnerRequirement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*memory.ExtractorCandidate)
		valid  bool
	}{
		{"whole", func(c *memory.ExtractorCandidate) {
			c.Support[1].LocatorKind = memory.LocatorWhole
			c.Support[1].LocatorValue = ""
			c.Support[1].EvidenceSHA256 = memory.CompilerHash([]byte("2026-09-04 11:42:00"))
		}, true},
		{"date", func(*memory.ExtractorCandidate) {}, true},
		{"whole as range", func(c *memory.ExtractorCandidate) {
			c.Support[1].LocatorValue = "0:19"
			c.Support[1].EvidenceSHA256 = memory.CompilerHash([]byte("2026-09-04 11:42:00"))
		}, false},
		{"year", func(c *memory.ExtractorCandidate) {
			c.Support[1].LocatorValue = "0:4"
			c.Support[1].EvidenceSHA256 = memory.CompilerHash([]byte("2026"))
		}, false},
		{"time", func(c *memory.ExtractorCandidate) {
			c.Support[1].LocatorValue = "11:19"
			c.Support[1].EvidenceSHA256 = memory.CompilerHash([]byte("11:42:00"))
		}, false},
		{"payload", func(c *memory.ExtractorCandidate) { c.Support[1].EventPart = memory.EvidencePayload }, false},
		{"json pointer", func(c *memory.ExtractorCandidate) {
			c.Support[1].LocatorKind = memory.LocatorJSONPointer
			c.Support[1].LocatorValue = "/date"
		}, false},
		{"mismatched hash", func(c *memory.ExtractorCandidate) {
			c.Support[1].EvidenceSHA256 = memory.CompilerHash([]byte("2026-09-05"))
		}, false},
		{"bare clock", func(c *memory.ExtractorCandidate) { c.Support = c.Support[1:] }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel, _ := f.clockSelection(t, "2026-09-04 11:42:00")
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				c := f.clockCandidate(r)
				tc.mutate(&c)
				return compilerOutput(r, []memory.ExtractorCandidate{c}), nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, clockGeneration(), extractor)
			if (err == nil && result.State == "completed_candidates") != tc.valid {
				t.Fatalf("valid=%v state=%s reason=%s error=%v", tc.valid, result.State, result.Reason, err)
			}
		})
	}
}

func TestCompilerClockObservationOlderPolicyAndNonOutcomesStayExcluded(t *testing.T) {
	for _, kind := range []string{"old policy", "tool_failed", "tool_cancelled", "execution_resolved", "other tool", "secret"} {
		t.Run(kind, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel, e := f.clockSelection(t, "2026-09-04 11:42:00")
			g := clockGeneration()
			switch kind {
			case "old policy":
				g = compilerGeneration()
			case "other tool":
				mutateClockEvent(t, f, clockControlID(t, f, "tool_intent"), "payload_json=?", `{"call":{"id":"clock-call","name":"shell","arguments":"{}"}}`)
				mutateClockEvent(t, f, clockControlID(t, f, "assistant_message"), "payload_json=?", `{"tool_calls":[{"id":"clock-call","name":"shell","arguments":"{}"}]}`)
			case "secret":
				mutateClockEvent(t, f, e.ID, "content=?", "password=synthetic-secret-value")
			default:
				mutateClockEvent(t, f, e.ID, "event_type=?", kind)
			}
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				for _, s := range r.Window.Sources {
					if s.Locator.EventID == e.ID {
						t.Fatalf("prohibited evidence exposed: %+v", s)
					}
				}
				return compilerOutput(r, []memory.ExtractorCandidate{}), nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, g, extractor)
			if err != nil || result.State != "completed_empty" {
				t.Fatalf("exclusion %s %s %v", result.State, result.Reason, err)
			}
		})
	}
}

func TestCompilerClockObservationEmptyObjectArgumentsAndApproval(t *testing.T) {
	for _, tc := range []struct {
		name, args, approval string
		valid                bool
	}{
		{"whitespace object", " { \n\t } ", "", true},
		{"approved", "{}", "approved", true},
		{"declined", "{}", "declined", false},
		{"expired", "{}", "expired", false},
		{"null", "null", "", false}, {"array", "[]", "", false}, {"missing", "", "", false},
		{"nonempty", "{\"zone\":\"UTC\"}", "", false},
		{"duplicate keys", "{\"zone\":1,\"zone\":2}", "", false},
		{"trailing", "{} {}", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCompilerFixture(t)
			sel, _ := f.clockSelection(t, "2026-09-04 11:42:00", clockFixtureOptions{tc.args, tc.approval})
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				return compilerOutput(r, []memory.ExtractorCandidate{f.clockCandidate(r)}), nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, clockGeneration(), extractor)
			if (err == nil && result.State == "completed_candidates") != tc.valid {
				t.Fatalf("valid=%v state=%s reason=%s error=%v", tc.valid, result.State, result.Reason, err)
			}
		})
	}
}

func TestCompilerClockObservationSurvivesFailedOrCancelledTurn(t *testing.T) {
	for _, kind := range []memory.EventType{memory.EventTurnFailed, memory.EventTurnInterrupted} {
		t.Run(string(kind), func(t *testing.T) {
			f := newCompilerFixture(t)
			sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
			classification := memory.ClassificationProviderError
			if kind == memory.EventTurnInterrupted {
				classification = memory.ClassificationCallerCancelled
			}
			terminal := memory.TurnTerminalPayload{TurnID: sel.RootID, Classification: classification, Stage: memory.StageProvider}
			payload, _ := json.Marshal(terminal)
			last := f.append(t, memory.EventInput{ParentID: outcome.ID, Type: kind, Content: terminal.SafeContent(), Payload: payload})
			sel.Cutoff = last.Sequence
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				return compilerOutput(r, []memory.ExtractorCandidate{f.clockCandidate(r)}), nil
			}}
			result, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), sel, clockGeneration(), extractor)
			if err != nil || result.State != "completed_candidates" {
				t.Fatalf("completed observation lost in failed turn %s %v", result.State, err)
			}
		})
	}
}

func TestCompilerClockObservationRevalidatesBytesAndControlAncestry(t *testing.T) {
	for _, mutation := range []string{"display", "paired metadata", "new terminal", "foreign session"} {
		t.Run(mutation, func(t *testing.T) {
			f := newCompilerFixture(t)
			ctx := context.Background()
			sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
			extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
				return compilerOutput(r, []memory.ExtractorCandidate{f.clockCandidate(r)}), nil
			}}
			compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), extractor)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := f.store.LocalOwnerReviewContext(ctx, "global")
			if err != nil {
				t.Fatal(err)
			}
			preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "display":
				mutateClockEvent(t, f, outcome.ID, "content=?", "2026-09-05 11:42:00")
			case "paired metadata":
				call := memory.ToolCall{ID: "clock-call", Name: "get_time", Arguments: "{ }"}
				assistant, _ := json.Marshal(memory.AssistantMessagePayload{ToolCalls: []memory.ToolCall{call}})
				intent, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
				mutateClockEvent(t, f, clockControlID(t, f, "tool_intent"), "payload_json=?", string(intent))
				mutateClockEvent(t, f, clockControlID(t, f, "assistant_message"), "payload_json=?", string(assistant))
			case "new terminal":
				f.append(t, memory.EventInput{ParentID: outcome.ParentID, Type: memory.EventToolCancelled, Role: memory.RoleTool, ExecutionID: outcome.ExecutionID, Payload: json.RawMessage(`{"tool_call_id":"clock-call","is_error":false}`)})
			case "foreign session":
				other, err := f.store.CreateGlobalSession(ctx)
				if err != nil {
					t.Fatal(err)
				}
				conn, err := f.db.Conn(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = conn.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
					t.Fatal(err)
				}
				if _, err = conn.ExecContext(ctx, `DROP TRIGGER IF EXISTS events_append_only_update`); err != nil {
					t.Fatal(err)
				}
				_, err = conn.ExecContext(ctx, `UPDATE events SET session_id=?,sequence=100 WHERE id=?`, other.ID, outcome.ID)
				_, restoreErr := conn.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
				conn.Close()
				if err != nil || restoreErr != nil {
					t.Fatalf("inject foreign event: %v %v", err, restoreErr)
				}
			}
			_, err = f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "91000000-0000-4000-8000-000000000444"))
			if !errors.Is(err, eviedb.ErrReviewInvalidSource) {
				t.Fatalf("mutated source accepted: %v", err)
			}
			item, err := f.store.InspectOwnerCandidate(ctx, authority, compiled.Candidates[0].ID)
			if err != nil || !item.Redacted {
				t.Fatalf("mutated source disclosed: %+v %v", item, err)
			}
			if extractor.calls.Load() != 1 {
				t.Fatal("source revalidation reinvoked extraction")
			}
		})
	}
}

func TestCompilerClockObservationPromotionRetainsAuthorityAndPolicy(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
	sel.Destination = "session:" + string(f.session.ID)
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{f.clockCandidate(r)}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), extractor)
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile %s %v", compiled.State, err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, sel.Destination)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "91000000-0000-4000-8000-000000000445"))
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := f.store.InspectSemanticObject(ctx, outsider.ScopeContext(), memory.SemanticObjectClaim, accepted.Operation.ClaimIDs[0])
	if err == nil && hidden.Claim != nil {
		t.Fatal("narrow claim leaked before promotion")
	}
	event := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote the reviewed dated adoption to global."})
	promotion, err := f.store.PreparePromotion(ctx, f.session.ScopeContext(), memory.PromotionRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000446", SourceEventID: event.ID, SourceClaimID: accepted.Operation.ClaimIDs[0], DestinationScopeKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: promotion.ProposalSHA256, PreparedSHA256: promotion.PreparedSHA256})
	f.append(t, memory.EventInput{ParentID: promotion.Evidence.EventID, Type: memory.EventApproval, ExecutionID: memory.ExecutionID(promotion.OperationID), Payload: payload})
	result, err := f.store.ApplyPromotion(ctx, f.lease, promotion)
	if err != nil {
		t.Fatal(err)
	}
	inspected, err := f.store.InspectSemanticObject(ctx, outsider.ScopeContext(), memory.SemanticObjectClaim, result.DestinationClaimID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range inspected.Sources {
		s := entry.Source
		if s.EventID == outcome.ID {
			found = true
			if s.Evidence != "2026-09-04" || s.Actor != "tool" || s.Authority != "tool_observation" || s.SourceType != "tool_succeeded" {
				t.Fatalf("promoted authority lost: %+v", s)
			}
		}
	}
	if !found {
		t.Fatal("promoted clock missing")
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='stricter-policy'`); err != nil {
		t.Fatal(err)
	}
	inspected, err = f.store.InspectSemanticObject(ctx, outsider.ScopeContext(), memory.SemanticObjectClaim, result.DestinationClaimID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range inspected.Sources {
		if entry.Source.Evidence != "" {
			t.Fatal("new policy leaked accepted evidence")
		}
	}
	if verified, err := f.store.VerifySemanticProjection(ctx); err != nil || !verified.Valid {
		t.Fatalf("historical replay %+v %v", verified, err)
	}
	if extractor.calls.Load() != 1 {
		t.Fatal("promotion or replay reinvoked extraction")
	}
}

func TestCompilerClockObservationUnfinishedPrefixAndDisabledExtraction(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
	if _, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), nil); !errors.Is(err, eviedb.ErrCompilerNotConfigured) {
		t.Fatalf("disabled extraction: %v", err)
	}
	if err := f.db.QueryRow(`SELECT sequence FROM events WHERE id=?`, outcome.ParentID).Scan(&sel.Cutoff); err != nil {
		t.Fatal(err)
	}
	if err := f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		for _, source := range r.Window.Sources {
			if source.Observation != nil || source.SourceType == "tool_succeeded" {
				t.Fatal("unfinished intent supplied an observation")
			}
		}
		return compilerOutput(r, []memory.ExtractorCandidate{}), nil
	}}
	result, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), extractor)
	if err != nil || result.State != "completed_empty" || result.Window.Closure != "no_live_lease" {
		t.Fatalf("incomplete prefix: %+v %v", result, err)
	}
	if verified, err := f.store.VerifySemanticProjection(ctx); err != nil || !verified.Valid {
		t.Fatalf("disabled replay: %+v %v", verified, err)
	}
}

func TestCompilerClockObservationReplayRejectsChangedAcceptedBinding(t *testing.T) {
	f := newCompilerFixture(t)
	ctx := context.Background()
	sel, outcome := f.clockSelection(t, "2026-09-04 11:42:00")
	extractor := &scriptedCompiler{run: func(_ context.Context, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
		return compilerOutput(r, []memory.ExtractorCandidate{f.clockCandidate(r)}), nil
	}}
	compiled, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), sel, clockGeneration(), extractor)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.PrepareOwnerCandidateReview(ctx, authority, candidateRef(compiled), "accept")
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := f.store.ResolveOwnerCandidateReview(ctx, authority, decisionFor(preview, "91000000-0000-4000-8000-000000000447"))
	if err != nil {
		t.Fatal(err)
	}
	mutateClockEvent(t, f, outcome.ID, "content=?", "2026-09-05 11:42:00")
	if _, err = f.store.InspectOwnerReviewOperation(ctx, authority, accepted.Operation.OperationID); !errors.Is(err, eviedb.ErrReviewInvalidSource) {
		t.Fatalf("changed accepted source disclosed: %v", err)
	}
	if verified, err := f.store.VerifySemanticProjection(ctx); err == nil && verified.Valid {
		t.Fatal("replay trusted changed accepted evidence")
	}
	if extractor.calls.Load() != 1 {
		t.Fatal("historical source check invoked extraction")
	}
}
