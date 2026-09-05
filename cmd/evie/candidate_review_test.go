package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type reviewCLIExtractor struct{ subject, predicate memory.SemanticID }

func (reviewCLIExtractor) ServerIdentity() string { return "scripted:review-cli" }
func (e reviewCLIExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	var source memory.EvidenceLocator
	for _, s := range r.Window.Sources {
		if s.Usage == "new_support" {
			source = s.Locator
			break
		}
	}
	candidate := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: e.subject, PredicateID: e.predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "coffee"}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{source}, Context: []memory.EvidenceLocator{}}
	raw, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{candidate}})
	return eviedb.CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, err
}
func reviewCLIGeneration() memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "scripted:test", ModelSHA256: strings.Repeat("1", 64), Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64), Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract owner assertions.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
	g.ModelManifest = json.RawMessage(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	g.EvidencePolicy = memory.CompilerPolicyVersion
	g.SecretPolicy = memory.CompilerPolicyVersion
	g.ClosurePolicy = memory.CompilerPolicyVersion
	g.WindowPolicy = memory.CompilerPolicyVersion
	g.PredicatePolicy = memory.CompilerPolicyVersion
	g.EntityPolicy = memory.CompilerPolicyVersion
	g.ValidationPolicy = memory.CompilerPolicyVersion
	g.EquivalencePolicy = memory.CompilerPolicyVersion
	g.EffectPolicy = memory.CompilerPolicyVersion
	return g
}

func TestOwnerReviewCLIClosedSessionAcceptance(t *testing.T) {
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
	lease, err := store.AcquireTurnLease(ctx, session.ID, "review-cli", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		e, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	seed := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember drink tea"})
	explicit, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:91000000-0000-4000-8000-000000000140", SourceEventID: seed.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ApplyRememberLiteral(ctx, lease, explicit); err != nil {
		t.Fatal(err)
	}
	root := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer coffee."})
	last := appendEvent(memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	compiled, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, reviewCLIGeneration(), reviewCLIExtractor{explicit.Subject.ID, explicit.Predicate.ID})
	if err != nil || compiled.State != "completed_candidates" {
		t.Fatalf("compile %+v %v", compiled, err)
	}
	if err = store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) []byte {
		t.Helper()
		var out bytes.Buffer
		handled, err := runOwnerReviewManagement(ctx, append([]string{"memory-review"}, args...), &out, store)
		if !handled || err != nil {
			t.Fatalf("CLI %v: %v", args, err)
		}
		return out.Bytes()
	}
	var page memory.OwnerCandidatePage
	if err = json.Unmarshal(run("inbox", "--scope", "global"), &page); err != nil || len(page.Candidates) != 1 {
		t.Fatalf("inbox %+v %v", page, err)
	}
	var item memory.OwnerCandidate
	if err = json.Unmarshal(run("inspect", "--scope", "global", "--id", page.Candidates[0].Ref.ID), &item); err != nil {
		t.Fatal(err)
	}
	var preview memory.ReviewPreview
	if err = json.Unmarshal(run("prepare", "--scope", "global", "--id", item.Ref.ID, "--revision", fmt.Sprint(item.Ref.ReviewRevision), "--interpretation", fmt.Sprint(item.Ref.InterpretationRevision), "--action", "accept"), &preview); err != nil {
		t.Fatal(err)
	}
	args := []string{"resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000141", "--action", "accept"}
	var result memory.ReviewResult
	if err = json.Unmarshal(run(args...), &result); err != nil || result.Operation == nil {
		t.Fatalf("resolve %+v %v", result, err)
	}
	repeated := run(args...)
	var same memory.ReviewResult
	if err = json.Unmarshal(repeated, &same); err != nil || same.AuditID != result.AuditID {
		t.Fatalf("duplicate delivery %+v %v", same, err)
	}
	var op memory.OwnerReviewOperation
	if err = json.Unmarshal(run("operation", "--scope", "global", "--id", string(result.Operation.OperationID)), &op); err != nil || op.Preview.Effect.Claims[0].Sources[0].Evidence != "I prefer coffee." {
		t.Fatalf("operation %+v %v", op, err)
	}
	var status string
	db.QueryRow(`SELECT status FROM sessions WHERE id=?`, session.ID).Scan(&status)
	if status != "closed" {
		t.Fatal("CLI resumed source session")
	}
	for _, bad := range [][]string{{"inbox"}, {"prepare", "--scope", "global", "--id", item.Ref.ID}, {"resolve", "--scope", "global", "--preview", preview.ID, "--digest", preview.SHA256, "--delivery", "idem:v1:91000000-0000-4000-8000-000000000142"}, {"inbox", "--scope", "global", "--action", "accept"}} {
		var out bytes.Buffer
		if _, err = runOwnerReviewManagement(ctx, append([]string{"memory-review"}, bad...), &out, store); err == nil || out.Len() != 0 {
			t.Fatalf("unbound CLI action %v accepted", bad)
		}
	}
}
