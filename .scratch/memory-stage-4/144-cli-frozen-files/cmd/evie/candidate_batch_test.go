package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

// Count through the existing scripted extractors so review and replay cannot
// accidentally invoke extraction while the fixture still reports success.
type batchCLIExtractor struct {
	eviedb.CompilerExtractor
	calls int
}

func (e *batchCLIExtractor) Extract(ctx context.Context, g memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	e.calls++
	return e.CompilerExtractor.Extract(ctx, g, r)
}

func batchCLIJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestOwnerReviewBatchCLIMixedEditsClosedSessionAndReplay(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := eviedb.NewStore(db)
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "batch-cli-seed", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	seedSource := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I drink tea. I record local dates separately."})
	seed, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000001440", SourceEventID: seedSource.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, seed); err != nil {
		t.Fatal(err)
	}
	clockSeed, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000001441", SourceEventID: seedSource.ID, Predicate: "local_date_note", PredicateLabel: "local date note", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "local dates recorded separately"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, clockSeed); err != nil {
		t.Fatal(err)
	}
	compile := func(content string, generation memory.CompilerGeneration, extractor eviedb.CompilerExtractor) memory.Compilation {
		t.Helper()
		root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
		last := appendEvent(memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
		compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, generation, extractor)
		if err != nil || compiled.State != "completed_candidates" || len(compiled.Candidates) != 1 {
			t.Fatalf("compile state=%s reason=%s candidates=%d: %v", compiled.State, compiled.Reason, len(compiled.Candidates), err)
		}
		return compiled
	}
	generation := reviewCLIGeneration()
	generation.EntityPolicy = memory.CompilerIdentityPolicyV2
	generation.PredicatePolicy = generation.EntityPolicy
	generation.ValidationPolicy = generation.EntityPolicy
	generation.EquivalencePolicy = generation.EntityPolicy
	generation.EffectPolicy = generation.EntityPolicy
	identityExtractor := &batchCLIExtractor{CompilerExtractor: identityCLIExtractor{subject: seed.Subject.ID}}
	relationship := compile("I work with Maya Chen.", generation, identityExtractor)
	generation.EntityPolicy = memory.CompilerTemporalPolicyV3
	generation.PredicatePolicy = generation.EntityPolicy
	generation.ValidationPolicy = generation.EntityPolicy
	generation.EquivalencePolicy = generation.EntityPolicy
	generation.EffectPolicy = generation.EntityPolicy
	temporalExtractor := &batchCLIExtractor{CompilerExtractor: temporalCLIExtractor{reviewCLIExtractor: reviewCLIExtractor{subject: seed.Subject.ID, predicate: seed.Predicate.ID}, adapt: func(c *memory.ExtractorCandidate) {
		c.Temporal.Correction = &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{memory.CorrectionError}}
	}}}
	correction := compile("I was mistaken about tea. I drink espresso.", generation, temporalExtractor)
	rejectExtractor := &batchCLIExtractor{CompilerExtractor: reviewCLIExtractor{subject: seed.Subject.ID, predicate: seed.Predicate.ID}}
	dismissed := compile("I tried coffee once.", reviewCLIGeneration(), rejectExtractor)
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	client := &clockCLIClient{}
	toolCalls := 0
	clock := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "get_time"}}, Execute: func(_ context.Context, args string) (string, error) {
		toolCalls++
		if args != "{}" {
			t.Fatalf("unexpected clock arguments %q", args)
		}
		return "2026-09-04 11:42:00", nil
	}}
	profile, err := openrouter.NewExplicitContextProfile("batch-clock-fixture", 300000, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.NewWithToolset(client, profile, store.BindHistory(session.ID, "batch-cli-runtime"), session.ScopeContext(), store.BindTurnOwner(session.ID, "batch-cli-runtime"), tools.NewToolset([]tools.Tool{clock}))
	if err = runtime.Send(ctx, "Check the local date. On that checked local date, I adopted tea as my standing drink.", clockCLIEvents{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadEvents(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var clockRoot, clockOutcome memory.Event
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			clockRoot = event
		}
		if event.Type == memory.EventToolSucceeded {
			clockOutcome = event
		}
	}
	if clockOutcome.ID == "" || toolCalls != 1 || client.calls != 2 {
		t.Fatal("fixture did not commit the actual runtime clock path")
	}
	clockGeneration := reviewCLIGeneration()
	clockGeneration.EvidencePolicy = "owner-clock-observations-v2"
	clockExtractor := &clockCLIExtractor{subject: clockSeed.Subject.ID, predicate: clockSeed.Predicate.ID}
	observation, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: clockRoot.ID, Cutoff: events[len(events)-1].Sequence, Destination: "global"}, clockGeneration, clockExtractor)
	if err != nil || observation.State != "completed_candidates" || len(observation.Candidates) != 1 {
		t.Fatalf("clock compile state=%s reason=%s: %v", observation.State, observation.Reason, err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	run := func(target any, args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
		if !handled || err != nil {
			t.Fatalf("CLI %v: %v", args, err)
		}
		if err = json.Unmarshal(out.Bytes(), target); err != nil {
			t.Fatalf("CLI %v output %q: %v", args, out.String(), err)
		}
		return append([]byte{}, out.Bytes()...)
	}
	fails := func(want error, args ...string) {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
		if !handled || err == nil || out.Len() != 0 || want != nil && !errors.Is(err, want) {
			t.Fatalf("CLI %v: handled=%v output=%q error=%v want=%v", args, handled, out.String(), err, want)
		}
	}
	var items [4]memory.OwnerCandidate
	compiled := []memory.Compilation{relationship, correction, observation, dismissed}
	for i := range items {
		run(&items[i], "inspect", "--scope", "global", "--id", compiled[i].Candidates[0].ID)
	}
	var originals []memory.OwnerCandidate
	if err = json.Unmarshal([]byte(batchCLIJSON(t, items)), &originals); err != nil {
		t.Fatal(err)
	}
	for i := range items[:3] {
		var proposal memory.ExtractorCandidate
		if err = json.Unmarshal([]byte(batchCLIJSON(t, items[i].Candidate.Proposal)), &proposal); err != nil {
			t.Fatal(err)
		}
		switch i {
		case 0:
			proposal.Identity.Object.Name = "Maya Chen"
		case 1:
			proposal.Proposition.Object.Literal.Value = "espresso"
		case 2:
			proposal.Proposition.Object.Literal.Value = "tea adopted on local date 2026-09-04"
		}
		old := items[i].Ref
		run(&items[i], "edit", "--scope", "global", "--id", old.ID, "--revision", fmt.Sprint(old.ReviewRevision), "--interpretation", fmt.Sprint(old.InterpretationRevision), "--proposal", batchCLIJSON(t, proposal), "--reason", "Correct the interpretation using its original sources.")
		item := items[i]
		if item.Ref.ID != old.ID || item.Ref.InterpretationRevision != 1 || item.Ref.ReviewRevision != old.ReviewRevision+1 || item.Edit == nil || item.Original == nil || item.Edit.ParentRevision != 0 || item.Edit.OwnerID != memory.LocalOwnerID || item.Edit.AuditID == "" {
			t.Fatalf("edit lineage %d: %+v", i, item)
		}
		// Original retains extraction order; the earlier inspection and edit
		// before/after disclosures use canonical source-set order.
		if !reflect.DeepEqual(item.Original.Proposal, compiled[i].Candidates[0].Proposal) || !reflect.DeepEqual(item.Original.Support, compiled[i].Candidates[0].Support) || item.GenerationID != originals[i].GenerationID || item.JobID != originals[i].JobID || item.Destination != "global" {
			t.Fatalf("edit changed original extraction or scope %d", i)
		}
		var revision memory.ReviewEditRevision
		run(&revision, "edit-revision", "--scope", "global", "--id", item.Ref.ID, "--interpretation", "1")
		if !reflect.DeepEqual(revision, *item.Edit) || !reflect.DeepEqual(revision.Before.Proposal, originals[i].Candidate.Proposal) || !reflect.DeepEqual(revision.After.Proposal, proposal) {
			t.Fatalf("CLI edit history differs from current lineage %d", i)
		}
		fails(eviedb.ErrReviewStale, "edit", "--scope", "global", "--id", old.ID, "--revision", fmt.Sprint(old.ReviewRevision), "--interpretation", fmt.Sprint(old.InterpretationRevision), "--proposal", batchCLIJSON(t, proposal))
	}
	var identities memory.ReviewIdentityOptions
	run(&identities, "alternatives", "--scope", "global", "--id", items[0].Ref.ID, "--revision", "1", "--interpretation", "1")
	run(&items[0], "choose", "--scope", "global", "--id", items[0].Ref.ID, "--revision", "1", "--interpretation", "1", "--options", identities.SHA256, "--choices", batchCLIJSON(t, memory.ReviewIdentityChoices{Object: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}))
	var times memory.ReviewTemporalOptions
	run(&times, "temporal-options", "--scope", "global", "--id", items[1].Ref.ID, "--revision", "1", "--interpretation", "1")
	run(&items[1], "temporal-choose", "--scope", "global", "--id", items[1].Ref.ID, "--revision", "1", "--interpretation", "1", "--options", times.SHA256, "--choices", batchCLIJSON(t, memory.ReviewTemporalChoice{OldClaimID: seed.ClaimID, Mode: memory.CorrectionError}))
	request := memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{}}
	for i, id := range []string{"relationship", "correction", "local-date", "one-time-drink"} {
		action := "accept"
		if i == 3 {
			action = "reject"
		}
		request.Groups = append(request.Groups, memory.ReviewBatchGroupRequest{ID: id, Action: action, Candidates: []memory.CandidateRef{items[i].Ref}, Dependencies: []memory.ReviewDependency{}})
	}
	var invalidDependency memory.ReviewBatchRequest
	if err = json.Unmarshal([]byte(batchCLIJSON(t, request)), &invalidDependency); err != nil {
		t.Fatal(err)
	}
	invalidDependency.Groups[1].Dependencies = []memory.ReviewDependency{{CandidateID: items[1].Ref.ID, Field: "subject", FromCandidateID: items[0].Ref.ID, FromField: "object"}}
	fails(eviedb.ErrReviewDependencies, "batch-prepare", "--scope", "global", "--request", batchCLIJSON(t, invalidDependency))
	var preview memory.ReviewBatchPreview
	rawPreview := run(&preview, "batch-prepare", "--scope", "global", "--request", batchCLIJSON(t, request))
	if preview.Version != "owner-review-batch-v1" || preview.ScopeKey != "global" || len(preview.Groups) != 4 || len(preview.SHA256) != 71 || preview.FailureBehavior != "atomic_groups_independent_failures; committed_failures_are_not_retried" {
		t.Fatalf("batch disclosure %+v", preview)
	}
	var inspected memory.ReviewBatchPreview
	if got := run(&inspected, "batch-inspect", "--scope", "global", "--id", preview.ID); !bytes.Equal(got, rawPreview) {
		t.Fatal("CLI inspection changed the durable complete preview bytes")
	}
	for i, group := range preview.Groups {
		p := group.Preview
		if group.ID != request.Groups[i].ID || p.BatchID != preview.ID || p.Version != "owner-review-preview-v5" || p.Action != request.Groups[i].Action || len(p.SHA256) != 71 || len(p.EffectSHA256) != 71 || p.Candidates[0].Ref != items[i].Ref {
			t.Fatalf("bound group %d: %+v", i, group)
		}
		if p.Action == "reject" {
			if p.Effect != nil {
				t.Fatal("rejection preview authorized a semantic effect")
			}
			continue
		}
		if p.Effect == nil || p.Effect.Version != "owner-review-effect-v5" || len(p.Effect.Members) != 1 || len(p.Effect.Claims) != 1 || !reflect.DeepEqual(p.Effect.PriorRevisions, preview.PriorRevisions) || p.Candidates[0].Edit == nil {
			t.Fatalf("accepted group disclosure %d: %+v", i, group)
		}
	}
	relationshipEffect := preview.Groups[0].Preview.Effect.Members[0]
	if relationshipEffect.Identity == nil || !relationshipEffect.Identity.Revision.Choices.Object.Create || relationshipEffect.Claims[0].ObjectEntity == nil || relationshipEffect.Claims[0].ObjectEntity.CanonicalName != "Maya Chen" || !relationshipEffect.Claims[0].Predicate.Create || relationshipEffect.Claims[0].Predicate.Token != "works_with" {
		t.Fatal("relationship preview lost exact edited identity or Predicate creation")
	}
	correctionEffect := preview.Groups[1].Preview.Effect.Members[0]
	if correctionEffect.Correction == nil || correctionEffect.Correction.OldClaim.ID != seed.ClaimID || correctionEffect.Correction.Mode != memory.CorrectionError || correctionEffect.Correction.EffectiveTime != nil || correctionEffect.Claims[0].Claim.Object.Literal.Value != "espresso" {
		t.Fatalf("correction preview %+v", correctionEffect)
	}
	clockEffect := preview.Groups[2].Preview.Effect.Members[0]
	if clockEffect.Version != "owner-review-effect-v4" || clockEffect.Claims[0].Claim.ValidTime.From != nil || clockEffect.Claims[0].Claim.ValidTime.To != nil || !strings.Contains(clockEffect.Claims[0].TemporalQualification, "timezone and effective instant remain unknown") {
		t.Fatalf("clock preview invented an effective instant %+v", clockEffect)
	}
	if len(clockEffect.Claims[0].Sources) != 2 {
		t.Fatal("clock preview lost its owner assertion and contracted observation")
	}
	ownerSource, clockSource := false, false
	for _, source := range clockEffect.Claims[0].Sources {
		if source.EventID == clockOutcome.ID {
			clockSource = true
			if source.Evidence != "2026-09-04" || source.LocatorValue != "0:10" || source.Authority != "tool_observation" || source.Actor != "tool" || source.SourceType != "tool_succeeded" {
				t.Fatalf("edit or approval upgraded clock authority: %+v", source)
			}
		} else if source.EventID != clockRoot.ID || source.Authority != memory.AuthorityOwnerStatement || source.Evidence != clockRoot.Content {
			t.Fatalf("unexpected owner evidence %+v", source)
		} else {
			ownerSource = true
		}
	}
	if !ownerSource || !clockSource {
		t.Fatal("clock effect must retain both distinct source authorities")
	}
	decision := memory.ReviewBatchDecision{DeliveryKey: "idem:v1:91000000-0000-4000-8000-000000001442", PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Actions: []memory.ReviewBatchAction{}, Reason: "Accept the disclosed edits and original source authority."}
	for _, group := range preview.Groups {
		decision.Actions = append(decision.Actions, memory.ReviewBatchAction{GroupID: group.ID, Action: group.Preview.Action})
	}
	badDigest := decision
	badDigest.PreviewSHA256 = "sha256:" + strings.Repeat("0", 64)
	fails(eviedb.ErrReviewStale, "batch-resolve", "--scope", "global", "--decision", batchCLIJSON(t, badDigest))
	badAction := decision
	badAction.Actions = append([]memory.ReviewBatchAction{}, decision.Actions...)
	badAction.Actions[0].Action = "reject"
	fails(eviedb.ErrReviewDependencies, "batch-resolve", "--scope", "global", "--decision", batchCLIJSON(t, badAction))
	for _, args := range [][]string{
		{"edit", "--scope", "global", "--id", items[2].Ref.ID, "--proposal", batchCLIJSON(t, items[2].Candidate.Proposal), "--request", "{}"},
		{"edit-revision", "--scope", "global", "--id", items[2].Ref.ID, "--interpretation", "0"},
		{"batch-prepare", "--scope", "global", "--request", `{"groups":[],"scope":"global"}`},
		{"batch-prepare", "--scope", "global", "--request", `{"groups":[],"groups":[]}`},
		{"batch-prepare", "--scope", "global", "--request", batchCLIJSON(t, request) + " {}"},
		{"batch-prepare", "--request", batchCLIJSON(t, request)},
		{"batch-inspect", "--scope", "global"},
		{"batch-resolve", "--scope", "global", "--decision", `{"delivery_key":"x","effects":[]}`},
	} {
		fails(nil, args...)
	}
	var result memory.ReviewBatchResult
	rawResult := run(&result, "batch-resolve", "--scope", "global", "--decision", batchCLIJSON(t, decision))
	if result.DeliveryKey != decision.DeliveryKey || result.PreviewID != preview.ID || len(result.Groups) != 4 {
		t.Fatalf("batch receipt %+v", result)
	}
	for i, group := range result.Groups {
		action := request.Groups[i].Action
		if group.GroupID != preview.Groups[i].ID || group.Outcome != action+"ed" || group.FailureCode != "" || group.Result == nil || group.Result.Action != action {
			t.Fatalf("group outcome %d: %+v", i, group)
		}
		if action == "accept" {
			if group.Result.Operation == nil || group.Result.Operation.OperationID != preview.Groups[i].Preview.Effect.OperationID {
				t.Fatal("missing or substituted operation")
			}
			var operation memory.OwnerReviewOperation
			run(&operation, "operation", "--scope", "global", "--id", string(group.Result.Operation.OperationID))
			if operation.Batch == nil || operation.Batch.PreviewID != preview.ID || operation.Batch.PreviewSHA256 != preview.SHA256 || operation.Batch.GroupID != group.GroupID || operation.Batch.GroupIndex != i || len(operation.Batch.PriorGroups) != i || operation.Actor != memory.SemanticActorOwner || operation.SessionID != session.ID || !reflect.DeepEqual(operation.Preview, preview.Groups[i].Preview) {
				t.Fatalf("operation lost exact batch approval or origin %d: %+v", i, operation)
			}
			if len(group.Result.Operation.ClaimIDs) != 1 || group.Result.Operation.ClaimIDs[0] != preview.Groups[i].Preview.Effect.Claims[0].Claim.ID {
				t.Fatal("receipt substituted a Claim ID")
			}
			wantRevisions := append([]memory.ScopeRevision{}, preview.PriorRevisions...)
			for j := range wantRevisions {
				wantRevisions[j].Revision += int64(i + 1)
			}
			if !reflect.DeepEqual(group.Result.Operation.ResultingRevisions, wantRevisions) {
				t.Fatalf("group %d lost the enumerated own revision advance", i)
			}
		} else if group.Result.Operation != nil {
			t.Fatal("rejection wrote a semantic operation")
		}
		var resolved memory.OwnerCandidate
		run(&resolved, "inspect", "--scope", "global", "--id", items[i].Ref.ID)
		if resolved.Candidate.ReviewState != action+"ed" || resolved.Ref.InterpretationRevision != items[i].Ref.InterpretationRevision || resolved.Ref.ReviewRevision != items[i].Ref.ReviewRevision+1 {
			t.Fatalf("terminal interpretation %d: %+v", i, resolved)
		}
		fails(eviedb.ErrReviewResolved, "edit", "--scope", "global", "--id", items[i].Ref.ID, "--revision", fmt.Sprint(items[i].Ref.ReviewRevision), "--interpretation", fmt.Sprint(items[i].Ref.InterpretationRevision), "--proposal", batchCLIJSON(t, items[i].Candidate.Proposal))
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	var retry memory.ReviewBatchResult
	if raw := run(&retry, "batch-resolve", "--scope", "global", "--decision", batchCLIJSON(t, decision)); !bytes.Equal(raw, rawResult) {
		t.Fatal("CLI duplicate delivery after reopening did not return the exact stored receipt")
	}
	changed := decision
	changed.Reason = "Changed approval request."
	fails(eviedb.ErrIdempotencyConflict, "batch-resolve", "--scope", "global", "--decision", batchCLIJSON(t, changed))
	if verified, err := store.VerifySemanticProjection(ctx); err != nil || !verified.Valid {
		t.Fatalf("canonical mixed edited batch replay %+v: %v", verified, err)
	}
	if toolCalls != 1 || client.calls != 2 || identityExtractor.calls != 1 || temporalExtractor.calls != 1 || clockExtractor.calls != 1 || rejectExtractor.calls != 1 {
		t.Fatalf("review/replay invoked inference or tools: tool=%d model=%d identity=%d temporal=%d clock=%d rejection=%d", toolCalls, client.calls, identityExtractor.calls, temporalExtractor.calls, clockExtractor.calls, rejectExtractor.calls)
	}
}
