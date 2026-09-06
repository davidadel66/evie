package web_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

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

func newWebReviewFixture(t *testing.T) *webReviewFixture {
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

func (f *webReviewFixture) append(t *testing.T, input memory.EventInput) memory.Event {
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
func (f *webReviewFixture) compile(t *testing.T, scope string) memory.OwnerCandidate {
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
func reviewPost(t *testing.T, h http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/memory/candidates/"+path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "http://localhost:5173")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func reviewResponse[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
	}
	var out T
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func webDecision(p memory.ReviewPreview) memory.ReviewDecision {
	return memory.ReviewDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000000146", PreviewID: p.ID, PreviewSHA256: p.SHA256, Action: p.Action}
}
func webPrepare(t *testing.T, f *webReviewFixture, action string) memory.ReviewPreview {
	return reviewResponse[memory.ReviewPreview](t, reviewPost(t, f.handler, "prepare", map[string]any{"scope_key": "global", "candidate": f.candidate.Ref, "action": action}), 200)
}

func TestCandidateReviewHTTPClosedSourceExactApprovalAndReopen(t *testing.T) {
	f := newWebReviewFixture(t)
	ctx := context.Background()
	page := reviewResponse[memory.OwnerCandidatePage](t, reviewPost(t, f.handler, "list", map[string]any{"scope_key": "global", "limit": 1}), 200)
	if len(page.Candidates) != 1 || page.Candidates[0].Ref != f.candidate.Ref {
		t.Fatalf("page %+v", page)
	}
	detail := reviewResponse[memory.OwnerCandidate](t, reviewPost(t, f.handler, "inspect", map[string]any{"scope_key": "global", "id": f.candidate.Ref.ID}), 200)
	if !reflect.DeepEqual(detail, f.candidate) {
		t.Fatal("HTTP inspection differs from Kernel")
	}
	p := webPrepare(t, f, "accept")
	if p.Effect == nil || p.Effect.Claims[0].Sources[0].Evidence != "I prefer café." || p.Effect.Claims[0].Context[0].Authority != "none" {
		t.Fatalf("preview %+v", p)
	}
	var before int
	if err := f.db.QueryRow(`SELECT count(*) FROM semantic_claims`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatal("preparation wrote accepted memory")
	}
	decision := webDecision(p)
	body := map[string]any{"scope_key": "global", "decision": decision}
	w := reviewPost(t, f.handler, "resolve", body)
	result := reviewResponse[memory.ReviewResult](t, w, 200)
	if w.Header().Get("Cache-Control") != "no-store" || result.Operation == nil {
		t.Fatalf("result %+v headers %+v", result, w.Header())
	}
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	operation, err := f.store.InspectOwnerReviewOperation(ctx, a, result.Operation.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(operation.Preview, p) {
		t.Fatal("persisted reviewed effect differs")
	}
	httpOperation := reviewResponse[memory.OwnerReviewOperation](t, reviewPost(t, f.handler, "operation", map[string]any{"scope_key": "global", "id": result.Operation.OperationID}), 200)
	if !reflect.DeepEqual(operation, httpOperation) {
		t.Fatal("HTTP provenance differs from Kernel/CLI boundary")
	}
	if err = f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	f.handler = web.WithCandidateReview(web.NewServer(nil), f.store).Handler()
	retry := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", body), 200)
	if !reflect.DeepEqual(result, retry) {
		t.Fatal("response delivery retry changed persisted result")
	}
	page = reviewResponse[memory.OwnerCandidatePage](t, reviewPost(t, f.handler, "list", map[string]any{"scope_key": "global"}), 200)
	if len(page.Candidates) != 0 {
		t.Fatal("accepted candidate remains in inbox")
	}
	decision.DeliveryKey = "idem:v1:90000000-0000-4000-8000-000000000147"
	resolved := reviewResponse[struct {
		Code   string              `json:"code"`
		Result memory.ReviewResult `json:"result"`
	}](t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": decision}), 409)
	if resolved.Code != "already_resolved" || !reflect.DeepEqual(resolved.Result, result) {
		t.Fatalf("resolved %+v", resolved)
	}
	var status string
	var leases int
	f.db.QueryRow(`SELECT status FROM sessions WHERE id=?`, f.session.ID).Scan(&status)
	f.db.QueryRow(`SELECT count(*) FROM session_turn_leases WHERE holder_id IS NOT NULL`).Scan(&leases)
	if status != "closed" || leases != 0 {
		t.Fatal("review reopened the source")
	}
	v, err := f.store.VerifySemanticProjection(ctx)
	if err != nil || !v.Valid {
		t.Fatalf("replay %+v %v", v, err)
	}
	if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='new-policy-after-acceptance'`); err != nil {
		t.Fatal(err)
	}
	w = reviewPost(t, f.handler, "operation", map[string]any{"scope_key": "global", "id": result.Operation.OperationID})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "source_ineligible") || strings.Contains(w.Body.String(), "I prefer café.") || strings.Contains(w.Body.String(), "Recorded.") {
		t.Fatalf("protected accepted provenance %d %s", w.Code, w.Body)
	}
}

func TestCandidateReviewHTTPProtections(t *testing.T) {
	f := newWebReviewFixture(t)
	for _, test := range []struct {
		name, method, host, origin, content, body string
		status                                    int
	}{
		{"origin", "POST", "127.0.0.1", "https://evil.example", "application/json", `{"scope_key":"global"}`, 403},
		{"host", "POST", "evil.example", "", "application/json", `{"scope_key":"global"}`, 403},
		{"form", "POST", "127.0.0.1", "", "text/plain", `{"scope_key":"global"}`, 403},
		{"method", "GET", "127.0.0.1", "", "application/json", `{}`, 405},
		{"unknown authority", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","authority":{"owner":"local"}}`, 400},
		{"duplicate scope", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","scope_key":"workspace:missing"}`, 400},
		{"null", "POST", "127.0.0.1", "", "application/json", `null`, 400},
		{"trailing", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global"}{}`, 400},
		{"oversize", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"` + strings.Repeat("x", 70*1024) + `"}`, 413},
		{"missing scope", "POST", "127.0.0.1", "", "application/json", `{}`, 400},
		{"foreign scope", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"workspace:missing"}`, 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, "http://"+test.host+"/api/memory/candidates/list", strings.NewReader(test.body))
			r.Header.Set("Origin", test.origin)
			r.Header.Set("Content-Type", test.content)
			w := httptest.NewRecorder()
			f.handler.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status %d: %s", w.Code, w.Body)
			}
			if strings.Contains(w.Body.String(), "café") {
				t.Fatal("protected bytes leaked")
			}
		})
	}
	for _, scope := range []string{"session:" + string(f.session.ID), "workspace:missing"} {
		w := reviewPost(t, f.handler, "inspect", map[string]any{"scope_key": scope, "id": f.candidate.Ref.ID})
		if w.Code != 403 || strings.Contains(w.Body.String(), "café") {
			t.Fatalf("foreign inspection %d %s", w.Code, w.Body)
		}
	}
	ref := f.candidate.Ref
	ref.ReviewRevision++
	w := reviewPost(t, f.handler, "prepare", map[string]any{"scope_key": "global", "candidate": ref, "action": "accept"})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "stale_preview") {
		t.Fatalf("stale %d %s", w.Code, w.Body)
	}
	p := webPrepare(t, f, "accept")
	decision := webDecision(p)
	decision.PreviewSHA256 = strings.Repeat("0", 64)
	w = reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": decision})
	if w.Code == 200 {
		t.Fatal("forged digest accepted")
	}
}

func TestCandidateReviewHTTPPolicyRedactionAndExplicitReject(t *testing.T) {
	f := newWebReviewFixture(t)
	p := webPrepare(t, f, "accept")
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-policy'`); err != nil {
		t.Fatal(err)
	}
	w := reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": webDecision(p)})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "stale_preview") {
		t.Fatalf("stale %d %s", w.Code, w.Body)
	}
	item := reviewResponse[memory.OwnerCandidate](t, reviewPost(t, f.handler, "inspect", map[string]any{"scope_key": "global", "id": f.candidate.Ref.ID}), 200)
	if !item.Redacted || len(item.Candidate.Support) != 0 {
		t.Fatal("source not redacted")
	}
	w = reviewPost(t, f.handler, "prepare", map[string]any{"scope_key": "global", "candidate": f.candidate.Ref, "action": "accept"})
	if w.Code != 409 || strings.Contains(w.Body.String(), "café") {
		t.Fatal("ineligible acceptance disclosed source")
	}
	reject := webPrepare(t, f, "reject")
	decision := webDecision(reject)
	decision.DeliveryKey = "idem:v1:90000000-0000-4000-8000-000000000148"
	decision.Reason = strings.Repeat("a", 4096)
	out := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": decision}), 200)
	if out.Operation != nil {
		t.Fatal("rejection wrote graph")
	}
	decision.Reason = "different intent"
	w = reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": decision})
	if w.Code != 409 || !strings.Contains(w.Body.String(), "idempotency_conflict") {
		t.Fatal("changed retry was not rejected")
	}
}

func TestCandidateReviewScopeNavigationClosedSessionPagination(t *testing.T) {
	f := newWebReviewFixture(t)
	var err error
	if _, err = f.db.Exec(`UPDATE sessions SET status='active' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	f.lease, err = f.store.AcquireTurnLease(context.Background(), f.session.ID, "additional-candidate", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	f.compile(t, "session:"+string(f.session.ID))
	if err = f.store.ReleaseTurnLease(context.Background(), f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	first := reviewResponse[memory.OwnerCandidateScopes](t, reviewPost(t, f.handler, "scopes", map[string]any{"limit": 1}), 200)
	if len(first.Scopes) != 1 || first.Scopes[0].ScopeKey != "global" || first.NextCursor == "" {
		t.Fatalf("first %+v", first)
	}
	last := reviewResponse[memory.OwnerCandidateScopes](t, reviewPost(t, f.handler, "scopes", map[string]any{"limit": 1, "cursor": first.NextCursor}), 200)
	if len(last.Scopes) != 1 || last.Scopes[0].Label != "Tea preferences" || last.Scopes[0].Kind != "session" || last.NextCursor != "" {
		t.Fatalf("last %+v", last)
	}
	page := reviewResponse[memory.OwnerCandidatePage](t, reviewPost(t, f.handler, "list", map[string]any{"scope_key": last.Scopes[0].ScopeKey}), 200)
	if len(page.Candidates) != 1 || page.Candidates[0].Destination != last.Scopes[0].ScopeKey {
		t.Fatalf("exact destination %+v", page)
	}
	w := reviewPost(t, f.handler, "scopes", map[string]any{"cursor": first.NextCursor + "x"})
	if w.Code != 400 {
		t.Fatalf("forged cursor %d %s", w.Code, w.Body)
	}
}
