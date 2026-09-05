package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type compilerCommitExtractor struct{ beforeReturn func() error }

func (compilerCommitExtractor) ServerIdentity() string { return "scripted:commit-cancellation" }
func (e compilerCommitExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (CompilerExtraction, error) {
	if e.beforeReturn != nil {
		if err := e.beforeReturn(); err != nil {
			return CompilerExtraction{}, err
		}
	}
	return CompilerExtraction{Raw: compilerJSON(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{}}), ReleaseEvidence: "completed"}, nil
}
func compilerCommitGeneration() memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "scripted:commit", ModelSHA256: strings.Repeat("1", 64), Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64), Template: "{{.Prompt}}", Prompt: "Extract owner assertions.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
	g.ModelManifest = []byte(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
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

func TestCompilerCancellationAfterExtractionCannotCommit(t *testing.T) {
	for _, phase := range []string{"stage_wait", "publish_wait", "stage_commit", "publish_commit"} {
		t.Run(phase, func(t *testing.T) {
			db := newTestDB(t)
			store := NewStore(db)
			base := context.Background()
			session, err := store.CreateGlobalSession(base)
			if err != nil {
				t.Fatal(err)
			}
			lease, err := store.AcquireTurnLease(base, session.ID, "compiler-commit-test", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			root, err := store.AppendEventWithLease(base, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
			if err != nil {
				t.Fatal(err)
			}
			last, err := store.AppendEventWithLease(base, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(base)
			defer cancel()
			locked := make(chan *sql.Conn, 1)
			var triggered atomic.Bool
			acquireLock := func() error {
				conn, err := db.Conn(base)
				if err != nil {
					return err
				}
				if _, err := conn.ExecContext(base, `BEGIN IMMEDIATE`); err != nil {
					conn.Close()
					return err
				}
				locked <- conn
				return nil
			}
			extractor := compilerCommitExtractor{}
			if phase == "stage_wait" {
				extractor.beforeReturn = acquireLock
			}
			store.resolveImmediateTransaction = func(resolveCtx context.Context, conn *sql.Conn, statement string) (sql.Result, error) {
				if statement == "COMMIT" && !triggered.Load() {
					var stages, groups int
					if err := conn.QueryRowContext(resolveCtx, `SELECT COUNT(*) FROM memory_compiler_stages`).Scan(&stages); err != nil {
						return nil, err
					}
					if err := conn.QueryRowContext(resolveCtx, `SELECT COUNT(*) FROM memory_compiler_candidate_groups`).Scan(&groups); err != nil {
						return nil, err
					}
					atStage := stages == 1 && groups == 0
					atPublish := groups == 1
					if ((phase == "stage_commit" || phase == "publish_wait") && atStage) || (phase == "publish_commit" && atPublish) {
						triggered.Store(true)
						if phase == "publish_wait" {
							result, err := executeImmediateTransactionStatement(resolveCtx, conn, statement)
							if err != nil {
								return result, err
							}
							return result, acquireLock()
						}
						cancel()
					}
				}
				return executeImmediateTransactionStatement(resolveCtx, conn, statement)
			}
			type outcome struct {
				result memory.Compilation
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := store.CompileCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, compilerCommitGeneration(), extractor)
				done <- outcome{result, err}
			}()
			if strings.HasSuffix(phase, "_wait") {
				var held *sql.Conn
				select {
				case held = <-locked:
				case <-time.After(2 * time.Second):
					t.Fatal("did not establish writer lock")
				}
				deadline := time.Now().Add(2 * time.Second)
				for db.Stats().InUse < 2 && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if db.Stats().InUse < 2 {
					held.ExecContext(base, `ROLLBACK`)
					held.Close()
					t.Fatal("publication did not wait on SQLite")
				}
				cancel()
				if _, err := held.ExecContext(base, `ROLLBACK`); err != nil {
					t.Fatal(err)
				}
				if err := held.Close(); err != nil {
					t.Fatal(err)
				}
			}
			var got outcome
			select {
			case got = <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("cancelled compilation did not return")
			}
			if !errors.Is(got.err, context.Canceled) || got.result.State != "cancelled" || got.result.Attempts != 1 || got.result.CapacityState != "" {
				t.Fatalf("outcome %+v err=%v", got.result, got.err)
			}
			for _, table := range []string{"memory_compiler_candidate_groups", "memory_compiler_candidates", "memory_compiler_coverage", "memory_compiler_capacity"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("cancelled %s count=%d err=%v", table, count, err)
				}
			}
			var stages, consumed int
			if err := db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(consumed),0) FROM memory_compiler_stages`).Scan(&stages, &consumed); err != nil {
				t.Fatal(err)
			}
			wantStages := 0
			if strings.HasPrefix(phase, "publish_") {
				wantStages = 1
			}
			if stages != wantStages || consumed != 0 {
				t.Fatalf("stage disposition %d consumed %d", stages, consumed)
			}
			var fence int
			if err := db.QueryRow(`SELECT fence FROM memory_compiler_jobs`).Scan(&fence); err != nil || fence != 2 {
				t.Fatalf("fence=%d err=%v", fence, err)
			}
		})
	}
}

func TestCompilerEquivalenceV1GoldenEncoding(t *testing.T) {
	source := func(id memory.EventID, sequence int64, usage, text string) memory.CompilerSource {
		actor := memory.SemanticActorOwner
		authority := memory.AuthorityOwnerStatement
		kind := memory.SourceTypeUserMessage
		if usage == "context" {
			actor = "assistant"
			authority = "none"
			kind = "assistant_message"
		}
		return memory.CompilerSource{SourceType: kind, Locator: memory.EvidenceLocator{EventID: id, EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorWhole, EvidenceSHA256: memory.CompilerHash([]byte(text))}, SessionID: "session-fixture", ScopeKey: "global", Sequence: sequence, FormatVersion: 1, ObservedAt: "2026-09-05T00:00:00Z", Actor: actor, Authority: authority, Usage: usage, Evidence: text}
	}
	older := source("event-a", 1, "overlap", "Tea.")
	newer := source("event-b", 3, "new_support", "I prefer tea.")
	contextSource := source("event-c", 2, "context", "Which drink?")
	candidate := memory.MemoryCandidate{Proposal: memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: "owner-fixture", PredicateID: "predicate-fixture", Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{newer.Locator, older.Locator}, Context: []memory.EvidenceLocator{contextSource.Locator}}, Support: []memory.CompilerSource{newer, older}, Context: []memory.CompilerSource{contextSource}, ReviewState: "unresolved"}
	selection := memory.CompilationSelection{SessionID: "session-fixture", RootID: "event-b", Cutoff: 3, Destination: "global"}
	original := string(compilerJSON(candidate))
	encoded := compilerEquivalenceEncoding(selection, candidate)
	if got, want := memory.CompilerHash(encoded), "5133050318a6bd0fc9e725f0a49663961f97032ede271cbf899e893755d4df41"; got != want {
		t.Fatalf("equivalence-v1 hash=%s", got)
	}
	if string(compilerJSON(candidate)) != original {
		t.Fatal("canonical encoding rewrote original extraction")
	}
	candidate.Proposal.Support[0], candidate.Proposal.Support[1] = candidate.Proposal.Support[1], candidate.Proposal.Support[0]
	candidate.Support[0], candidate.Support[1] = candidate.Support[1], candidate.Support[0]
	if string(compilerEquivalenceEncoding(selection, candidate)) != string(encoded) {
		t.Fatal("reference reordering changed equivalence")
	}
	candidate.Proposal.Proposition.Polarity = memory.PolarityDenied
	if string(compilerEquivalenceEncoding(selection, candidate)) == string(encoded) {
		t.Fatal("polarity was omitted from equivalence")
	}
}
