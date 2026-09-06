package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type scriptedExtractor struct {
	Subject, Predicate memory.SemanticID
	DelayMS            int
	AuditPath          string
}

type dispatchObservation struct {
	RequestID  string `json:"request_id"`
	PID        int    `json:"pid"`
	StartedNS  int64  `json:"started_unix_nanos"`
	FinishedNS int64  `json:"finished_unix_nanos"`
}

func (scriptedExtractor) ServerIdentity() string { return "scripted:stage4-infrastructure-pilot-v1" }
func (scriptedExtractor) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}
func (x scriptedExtractor) Extract(ctx context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (out eviedb.CompilerExtraction, outErr error) {
	observation := dispatchObservation{RequestID: r.ID, PID: os.Getpid(), StartedNS: time.Now().UnixNano()}
	defer func() {
		observation.FinishedNS = time.Now().UnixNano()
		file, err := os.OpenFile(x.AuditPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			outErr = errors.Join(outErr, err)
			return
		}
		b, err := json.Marshal(observation)
		if err == nil {
			_, err = file.Write(append(b, '\n'))
		}
		outErr = errors.Join(outErr, err, file.Close())
	}()
	timer := time.NewTimer(time.Duration(x.DelayMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, ctx.Err()
	case <-timer.C:
	}
	var support, interpretation []memory.EvidenceLocator
	var content string
	for _, source := range r.Window.Sources {
		if source.Usage == "new_support" {
			support = append(support, source.Locator)
			content += source.Evidence
		}
		if source.Usage == "context" {
			interpretation = append(interpretation, source.Locator)
		}
	}
	if strings.Contains(content, "PILOT_FAILED_GAP") {
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, eviedb.ErrCompilerTerminalOutput
	}
	candidates := []memory.ExtractorCandidate{}
	if len(support) > 0 && !strings.Contains(content, "PILOT_ZERO_CANDIDATES") {
		candidates = append(candidates, memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: x.Subject, PredicateID: x.Predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "fixture " + memory.CompilerHash([]byte(content))[:16]}}, Polarity: memory.PolarityAffirmed}, Support: support, Context: interpretation})
	}
	b, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: candidates})
	return eviedb.CompilerExtraction{Raw: b, ReleaseEvidence: "completed"}, err
}

func generation() memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "scripted:stage4-infrastructure-pilot-v1", ModelSHA256: strings.Repeat("1", 64), Quantization: "scripted-not-a-model", RuntimeVersion: "scripted-v1", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64), Template: "{{.System}}\n{{.Prompt}}", Prompt: "Infrastructure fixture only; no quality conclusions.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
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
	g.EquivalencePolicy = memory.CompilerEquivalencePolicyV2
	g.EffectPolicy = memory.CompilerPolicyVersion
	return g
}

// Bulk insertion is deliberately confined to synthetic archived fixture data,
// outside all measured intervals. No compiler result or accepted graph record is
// inserted here. The experiment does not measure loading a million-event active
// conversational context: the foreground session is separate and small.
func seedRetained(ctx context.Context, db *sql.DB, store *eviedb.Store, count int) error {
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		return err
	}
	for first := 0; first < count; first += 1000 {
		last := min(first+1000, count)
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO events(id,session_id,sequence,event_type,role,content,payload_json,recorded_at,format_version) VALUES(?,?,?,'user_message','user','Synthetic archived filler.','{}','2026-01-01T00:00:00Z',1)`)
		if err != nil {
			tx.Rollback()
			return err
		}
		for n := first; n < last; n++ {
			if _, err = stmt.ExecContext(ctx, fmt.Sprintf("pilot-archive-%08d", n), session.ID, n+1); err != nil {
				stmt.Close()
				tx.Rollback()
				return err
			}
		}
		stmt.Close()
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	_, err = db.ExecContext(ctx, `UPDATE sessions SET status='closed' WHERE id=?`, session.ID)
	return err
}

func appendTurn(ctx context.Context, store *eviedb.Store, session memory.Session, text string) (memory.Event, memory.Event, error) {
	lease, err := store.AcquireTurnLease(ctx, session.ID, "pilot-fixture", time.Minute)
	if err != nil {
		return memory.Event{}, memory.Event{}, err
	}
	defer store.ReleaseTurnLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken)
	first, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	if err != nil {
		return first, memory.Event{}, err
	}
	last, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: first.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Acknowledged.", Payload: json.RawMessage(`{"tool_calls":[]}`)})
	return first, last, err
}

func seedGraph(ctx context.Context, store *eviedb.Store, count int) (scriptedExtractor, error) {
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		return scriptedExtractor{}, err
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "pilot-graph", time.Minute)
	if err != nil {
		return scriptedExtractor{}, err
	}
	defer store.ReleaseTurnLease(context.Background(), session.ID, lease.HolderID, lease.FencingToken)
	var x scriptedExtractor
	for n := 0; n < count; n++ {
		e, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: fmt.Sprintf("I prefer fixture value %d.", n)})
		if err != nil {
			return x, err
		}
		p, err := store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: fmt.Sprintf("idem:v1:90000000-0000-4000-8000-%012d", n+150000), SourceEventID: e.ID, Predicate: "pilot_preference", PredicateLabel: "pilot preference", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: fmt.Sprintf("fixture value %d", n)}})
		if err != nil {
			return x, err
		}
		if _, err = store.ApplyRememberLiteral(ctx, lease, p); err != nil {
			return x, err
		}
		x.Subject = p.Subject.ID
		x.Predicate = p.Predicate.ID
	}
	return x, nil
}

type foregroundClient struct{}

func (foregroundClient) ChatStream(_ context.Context, _ openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: "Acknowledged."}}}}, nil
}

type foregroundEvents struct{ output strings.Builder }

func (e *foregroundEvents) Delta(s string)                              { e.output.WriteString(s) }
func (*foregroundEvents) Reasoning(string)                              {}
func (*foregroundEvents) ReasoningDone()                                {}
func (e *foregroundEvents) AssistantDone(s string)                      { e.output.WriteString(s) }
func (*foregroundEvents) ToolCall(string, string, string)               {}
func (*foregroundEvents) ToolResult(string, string, bool)               {}
func (*foregroundEvents) ResponseDiscarded(agent.DiscardReason, string) {}

func fixtureInput(n, bytes int) string {
	prefix := fmt.Sprintf("I prefer fixture choice %03d. ", n)
	return prefix + strings.Repeat("x", bytes-len(prefix))
}
func checkedID(g memory.CompilerGeneration) (string, error) {
	id, _, err := memory.CompilerGenerationIdentity(g)
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", errors.New("missing generation identity")
	}
	return id, nil
}
