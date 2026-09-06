package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type fixtureHarness struct {
	directory string
	cleanup   []func()
}

func (t *fixtureHarness) Helper()                   {}
func (t *fixtureHarness) TempDir() string           { return t.directory }
func (t *fixtureHarness) Cleanup(f func())          { t.cleanup = append(t.cleanup, f) }
func (t *fixtureHarness) Fatal(v ...any)            { panic(fmt.Sprint(v...)) }
func (t *fixtureHarness) Fatalf(s string, v ...any) { panic(fmt.Sprintf(s, v...)) }

type webReviewFixture struct {
	db                 *sql.DB
	store              *eviedb.Store
	path               string
	session            memory.Session
	lease              memory.TurnLease
	subject, predicate memory.SemanticID
	candidate          memory.OwnerCandidate
	handler            http.Handler
}

func newWebReviewFixture(t *fixtureHarness) *webReviewFixture {
	t.Helper()
	f := &webReviewFixture{path: filepath.Join(t.TempDir(), "review.db")}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = eviedb.NewStore(f.db)
	ctx := context.Background()
	f.session, err = f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "web-review-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "/remember drink tea"})
	p, err := f.store.PrepareRememberLiteral(ctx, f.session.ScopeContext(), memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:90000000-0000-4000-8000-000000000145", SourceEventID: source.ID, Predicate: "drink", PredicateLabel: "drink", Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ApplyRememberLiteral(ctx, f.lease, p); err != nil {
		t.Fatal(err)
	}
	f.subject = p.Subject.ID
	f.predicate = p.Predicate.ID
	f.candidate = f.compile(t, "global")
	if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed',title='Tea preferences' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	f.handler = web.WithCandidateReview(web.NewServer(nil), f.store).Handler()
	return f
}

func (f *webReviewFixture) append(t *fixtureHarness, input memory.EventInput) memory.Event {
	t.Helper()
	e, err := f.store.AppendEventWithLease(context.Background(), f.session.ID, f.lease.HolderID, f.lease.FencingToken, input)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func webReviewGeneration() memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "scripted:web-review", ModelSHA256: strings.Repeat("1", 64), Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: strings.Repeat("2", 64), Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract owner assertions only.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
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

type webReviewExtractor struct{ subject, predicate memory.SemanticID }

func (webReviewExtractor) ServerIdentity() string { return "scripted:web-review" }
func (x webReviewExtractor) Extract(_ context.Context, _ memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	c := memory.ExtractorCandidate{Proposition: memory.ClaimProposition{SubjectEntityID: x.subject, PredicateID: x.predicate, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "café"}}, Polarity: memory.PolarityAffirmed}, Support: []memory.EvidenceLocator{}, Context: []memory.EvidenceLocator{}}
	for _, s := range r.Window.Sources {
		if s.Usage == "new_support" {
			c.Support = append(c.Support, s.Locator)
		}
		if s.Usage == "context" {
			c.Context = append(c.Context, s.Locator)
		}
	}
	b, err := json.Marshal(memory.CompilerResponse{RequestID: r.ID, Candidates: []memory.ExtractorCandidate{c}})
	return eviedb.CompilerExtraction{Raw: b, ReleaseEvidence: "completed"}, err
}
func (f *webReviewFixture) compile(t *fixtureHarness, scope string) memory.OwnerCandidate {
	t.Helper()
	root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer café."})
	last := f.append(t, memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded."})
	r, err := f.store.CompileCandidateUnit(context.Background(), f.session.ScopeContext(), memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: scope}, webReviewGeneration(), webReviewExtractor{f.subject, f.predicate})
	if err != nil || len(r.Candidates) != 1 {
		t.Fatalf("compile %+v: %v", r, err)
	}
	a, err := f.store.LocalOwnerReviewContext(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.store.InspectOwnerCandidate(context.Background(), a, r.Candidates[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func main() {
	directory, err := os.MkdirTemp("", "evie-advanced-review-browser-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(directory)
	harness := &fixtureHarness{directory: directory}
	defer func() {
		for _, f := range harness.cleanup {
			f()
		}
	}()
	f := newWebReviewFixture(harness)
	if _, err = f.db.Exec(`UPDATE sessions SET status='active' WHERE id=?`, f.session.ID); err != nil {
		panic(err)
	}
	f.lease, err = f.store.AcquireTurnLease(context.Background(), f.session.ID, "browser-fixture", time.Minute)
	if err != nil {
		panic(err)
	}
	seedAdvancedBrowserFixture(harness, f)
	f.compile(harness, "session:"+string(f.session.ID))
	if err = f.store.ReleaseTurnLease(context.Background(), f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		panic(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	server := &http.Server{Handler: f.handler, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("BROWSER_FIXTURE_URL=http://%s\nBROWSER_FIXTURE_DB=%s\n", listener.Addr(), f.path)
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(shutdown)
}
