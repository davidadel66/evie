package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func advancedPost(t *testing.T, f *webReviewFixture, route string, input any) *httptest.ResponseRecorder {
	t.Helper()
	return reviewPost(t, f.handler, route, map[string]any{"scope_key": "global", "input": input})
}

func advancedError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	out := reviewResponse[struct {
		Code string `json:"code"`
	}](t, w, status)
	if out.Code != code || strings.Contains(w.Body.String(), "café") || strings.Contains(w.Body.String(), "Maya") {
		t.Fatalf("unsafe or unexpected error: %s", w.Body)
	}
}

func TestCandidateAdvancedHTTPGuardsAndTypedBounds(t *testing.T) {
	f := newWebReviewFixture(t)
	routes := []struct {
		path  string
		limit int
	}{
		{"identity/options", 8192}, {"identity/choose", 8192}, {"identity/revision", 8192},
		{"temporal/options", 8192}, {"temporal/choose", 8192}, {"temporal/revision", 8192},
		{"edit", 264 * 1024}, {"edit/revision", 8192}, {"batch/prepare", 64 * 1024},
		{"batch/inspect", 8192}, {"batch/resolve", 32 * 1024},
	}
	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			const scopeProbe = `{"scope_key":"workspace:missing","input":{}}`
			for _, test := range []struct {
				name, method, host, origin, content, body string
				status                                    int
			}{
				{"origin", "POST", "127.0.0.1", "https://evil.example", "application/json", `{}`, 403},
				{"host", "POST", "evil.example", "", "application/json", `{}`, 403},
				{"method", "GET", "127.0.0.1", "", "application/json", `{}`, 405},
				{"content type", "POST", "127.0.0.1", "", "text/plain", `{}`, 403},
				{"null input", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":null}`, 400},
				{"missing input", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global"}`, 400},
				{"array input", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":[]}`, 400},
				{"duplicate input", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{},"input":{}}`, 400},
				{"nested owner", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{"owner_id":"local"}}`, 400},
				{"outer authority", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{},"authority":{}}`, 400},
				{"foreign scope", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"workspace:missing","input":{}}`, 403},
				{"trailing", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{}}{}`, 400},
				{"invalid UTF-8", "POST", "127.0.0.1", "", "application/json", "{\"scope_key\":\"global\",\"input\":{\"id\":\"\xff\"}}", 400},
				{"excessive nesting", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":` + strings.Repeat("[", 34) + "0" + strings.Repeat("]", 34) + "}", 400},
				{"oversize", "POST", "127.0.0.1", "", "application/json", strings.Repeat(" ", route.limit+1), 413},
				{"inclusive limit", "POST", "127.0.0.1", "", "application/json", scopeProbe + strings.Repeat(" ", route.limit-len(scopeProbe)), 403},
			} {
				t.Run(test.name, func(t *testing.T) {
					r := httptest.NewRequest(test.method, "http://"+test.host+"/api/memory/candidates/"+route.path, strings.NewReader(test.body))
					r.Header.Set("Origin", test.origin)
					r.Header.Set("Content-Type", test.content)
					w := httptest.NewRecorder()
					f.handler.ServeHTTP(w, r)
					if w.Code != test.status || strings.Contains(w.Body.String(), "café") {
						t.Fatalf("guard %d: %s", w.Code, w.Body)
					}
				})
			}
		})
	}
	// Nested duplicate fields must fail before an edit can advance either revision.
	ref, err := json.Marshal(f.candidate.Ref)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/memory/candidates/edit", strings.NewReader(`{"scope_key":"global","input":{"candidate":`+string(ref)+`,"candidate":`+string(ref)+`}}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	advancedError(t, w, 400, "invalid_review_request")
	tooMany := memory.ReviewBatchRequest{}
	for i := 0; i < 21; i++ {
		tooMany.Groups = append(tooMany.Groups, memory.ReviewBatchGroupRequest{ID: fmt.Sprintf("group%d", i), Action: "accept", Candidates: []memory.CandidateRef{{ID: fmt.Sprintf("candidate%d", i)}}})
	}
	advancedError(t, advancedPost(t, f, "batch/prepare", tooMany), 413, "review_too_large")
	var edits, batches int
	if err = f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_edit_revisions),(SELECT count(*) FROM memory_review_batch_previews)`).Scan(&edits, &batches); err != nil || edits+batches != 0 {
		t.Fatalf("malformed request mutated review: %d/%d %v", edits, batches, err)
	}
}

type advancedExtractor struct {
	webReviewExtractor
	adapt func(memory.ExtractorCandidate) []memory.ExtractorCandidate
}

func (x advancedExtractor) Extract(ctx context.Context, g memory.CompilerGeneration, r memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	base, err := x.webReviewExtractor.Extract(ctx, g, r)
	if err != nil {
		return base, err
	}
	var response memory.CompilerResponse
	if err = json.Unmarshal(base.Raw, &response); err != nil {
		return base, err
	}
	response.Candidates = x.adapt(response.Candidates[0])
	base.Raw, err = json.Marshal(response)
	return base, err
}

// Compilation occurs in a real source episode; every review call below occurs
// only after releasing its lease and closing that source session again.
func advancedCompile(t *testing.T, f *webReviewFixture, content, policy string, adapt func(memory.ExtractorCandidate) []memory.ExtractorCandidate) []memory.OwnerCandidate {
	t.Helper()
	ctx := context.Background()
	if _, err := f.db.Exec(`UPDATE sessions SET status='active' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	var err error
	f.lease, err = f.store.AcquireTurnLease(ctx, f.session.ID, "advanced-web-source", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root := f.append(t, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content})
	last := f.append(t, memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded."})
	g := webReviewGeneration()
	g.EntityPolicy, g.PredicatePolicy, g.ValidationPolicy, g.EquivalencePolicy, g.EffectPolicy = policy, policy, policy, policy, policy
	out, err := f.store.CompileCandidateUnit(ctx, f.session.ScopeContext(), memory.CompilationSelection{SessionID: f.session.ID, RootID: root.ID, Cutoff: last.Sequence, Destination: "global"}, g, advancedExtractor{webReviewExtractor{f.subject, f.predicate}, adapt})
	if err != nil || out.State != "completed_candidates" {
		t.Fatalf("compile advanced %s/%s: %v", out.State, out.Reason, err)
	}
	if err = f.store.ReleaseTurnLease(ctx, f.session.ID, f.lease.HolderID, f.lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	items := []memory.OwnerCandidate{}
	for _, c := range out.Candidates {
		items = append(items, reviewResponse[memory.OwnerCandidate](t, reviewPost(t, f.handler, "inspect", map[string]any{"scope_key": "global", "id": c.ID}), 200))
	}
	return items
}

func advancedChooseIdentity(t *testing.T, f *webReviewFixture, item memory.OwnerCandidate) memory.OwnerCandidate {
	t.Helper()
	o := reviewResponse[memory.ReviewIdentityOptions](t, advancedPost(t, f, "identity/options", item.Ref), 200)
	d := memory.ReviewIdentityDecision{Candidate: item.Ref, OptionsSHA256: o.SHA256, Choices: memory.ReviewIdentityChoices{Subject: &memory.ReviewEntityChoice{Create: true}, Predicate: &memory.ReviewPredicateChoice{Create: true}}}
	bad := d
	bad.OptionsSHA256 = strings.Repeat("0", 64)
	advancedError(t, advancedPost(t, f, "identity/choose", bad), 409, "stale_preview")
	chosen := reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, "identity/choose", d), 200)
	history := reviewResponse[memory.ReviewIdentityRevision](t, advancedPost(t, f, "identity/revision", map[string]any{"id": item.Ref.ID, "revision": chosen.Ref.InterpretationRevision}), 200)
	if chosen.Identity == nil || !reflect.DeepEqual(history, *chosen.Identity) || !reflect.DeepEqual(history.Options, o) {
		t.Fatal("identity route lost exact options or immutable revision")
	}
	return chosen
}

func advancedSharedBatch(t *testing.T) (*webReviewFixture, memory.ReviewBatchRequest) {
	t.Helper()
	f := newWebReviewFixture(t)
	items := advancedCompile(t, f, "Maya likes tea and coffee.", memory.CompilerIdentityPolicyV2, func(base memory.ExtractorCandidate) []memory.ExtractorCandidate {
		out := []memory.ExtractorCandidate{}
		for _, value := range []string{"tea", "coffee"} {
			c := base
			c.Proposition.SubjectEntityID, c.Proposition.PredicateID = "", ""
			c.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}
			c.Identity = &memory.CandidateIdentityProposal{Subject: &memory.EntityMention{Name: "Maya", EntityType: "person", Support: c.Support[0]}, Predicate: &memory.PredicateDefinition{Token: "likes", Label: "likes", ObjectConstraint: memory.PredicateObjectConstraint(memory.LiteralText), Cardinality: memory.CardinalityMany}}
			out = append(out, c)
		}
		return out
	})
	if len(items) != 2 {
		t.Fatalf("shared candidates: %d", len(items))
	}
	refs := []memory.CandidateRef{}
	for _, item := range items {
		refs = append(refs, advancedChooseIdentity(t, f, item).Ref)
	}
	return f, memory.ReviewBatchRequest{Groups: []memory.ReviewBatchGroupRequest{
		{ID: "shared", Action: "accept", Candidates: refs, Dependencies: []memory.ReviewDependency{
			{CandidateID: refs[1].ID, Field: "subject", FromCandidateID: refs[0].ID, FromField: "subject"},
			{CandidateID: refs[1].ID, Field: "predicate", FromCandidateID: refs[0].ID, FromField: "predicate"},
		}},
		{ID: "independent", Action: "accept", Candidates: []memory.CandidateRef{f.candidate.Ref}, Dependencies: []memory.ReviewDependency{}},
	}}
}

func advancedBatchDecision(p memory.ReviewBatchPreview) memory.ReviewBatchDecision {
	d := memory.ReviewBatchDecision{DeliveryKey: "idem:v1:90000000-0000-4000-8000-000000004146", PreviewID: p.ID, PreviewSHA256: p.SHA256}
	for _, g := range p.Groups {
		d.Actions = append(d.Actions, memory.ReviewBatchAction{GroupID: g.ID, Action: g.Preview.Action})
	}
	return d
}

func advancedReopen(t *testing.T, f *webReviewFixture) {
	t.Helper()
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	f.handler = web.WithCandidateReview(web.NewServer(nil), f.store).Handler()
}

func advancedVerify(t *testing.T, f *webReviewFixture) {
	t.Helper()
	v, err := f.store.VerifySemanticProjection(context.Background())
	if err != nil || !v.Valid {
		t.Fatalf("advanced replay: %+v %v", v, err)
	}
	var status string
	var leases int
	if err = f.db.QueryRow(`SELECT status,(SELECT count(*) FROM session_turn_leases WHERE holder_id IS NOT NULL) FROM sessions WHERE id=?`, f.session.ID).Scan(&status, &leases); err != nil || status != "closed" || leases != 0 {
		t.Fatalf("advanced review altered source lifecycle: %s/%d %v", status, leases, err)
	}
}

type uncertainAdvancedKernel struct {
	*eviedb.Store
	failed bool
}

func (k *uncertainAdvancedKernel) ResolveOwnerCandidateBatch(ctx context.Context, a eviedb.OwnerReviewContext, d memory.ReviewBatchDecision) (memory.ReviewBatchResult, error) {
	r, err := k.Store.ResolveOwnerCandidateBatch(ctx, a, d)
	if err == nil && !k.failed {
		k.failed = true
		return memory.ReviewBatchResult{}, errors.New("lost commit acknowledgement: protected Maya source")
	}
	return r, err
}

func TestCandidateAdvancedHTTPCompoundBatchExactRecoveryAndPartialFailure(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprintf("partial_%t", partial), func(t *testing.T) {
			f, request := advancedSharedBatch(t)
			bad := request
			bad.Groups = append([]memory.ReviewBatchGroupRequest(nil), request.Groups...)
			bad.Groups[0].Dependencies = bad.Groups[0].Dependencies[:1]
			advancedError(t, advancedPost(t, f, "batch/prepare", bad), 409, "review_dependencies")
			p := reviewResponse[memory.ReviewBatchPreview](t, advancedPost(t, f, "batch/prepare", request), 200)
			inspected := reviewResponse[memory.ReviewBatchPreview](t, advancedPost(t, f, "batch/inspect", map[string]any{"id": p.ID}), 200)
			if !reflect.DeepEqual(p, inspected) || len(p.Groups) != 2 || p.FailureBehavior == "" {
				t.Fatal("batch inspection changed exact disclosure")
			}
			shared := p.Groups[0].Preview
			if shared.BatchID != p.ID || shared.Version != "owner-review-preview-v5" || len(shared.Effect.Members) != 2 || len(shared.Effect.Dependencies) != 2 || shared.Effect.Claims[0].Subject.ID != shared.Effect.Claims[1].Subject.ID || shared.Effect.Claims[0].Predicate.ID != shared.Effect.Claims[1].Predicate.ID {
				t.Fatalf("incomplete compound disclosure: %+v", shared)
			}
			// An exact group preview cannot bypass approval of its containing batch.
			advancedError(t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": webDecision(shared)}), 409, "review_dependencies")
			if partial {
				if _, err := f.db.Exec(fmt.Sprintf(`CREATE TRIGGER advanced_fail_source BEFORE INSERT ON semantic_source_links WHEN NEW.source_link_id='%s' BEGIN SELECT RAISE(ABORT,'protected Maya details'); END`, shared.Effect.Claims[1].Sources[0].ID)); err != nil {
					t.Fatal(err)
				}
			}
			d := advancedBatchDecision(p)
			// The Kernel's inclusive 4 KiB reason bound counts decoded bytes.
			// JSON escaping can require six transport bytes for each such byte.
			d.Reason = strings.Repeat("\x01", 4096)
			forged := d
			forged.PreviewSHA256 = strings.Repeat("0", 64)
			advancedError(t, advancedPost(t, f, "batch/resolve", forged), 409, "stale_preview")
			f.handler = web.WithCandidateReview(web.NewServer(nil), &uncertainAdvancedKernel{Store: f.store}).Handler()
			advancedError(t, advancedPost(t, f, "batch/resolve", d), 503, "review_retryable")
			// Reopen before retrying the same request, as a browser would after a lost response.
			advancedReopen(t, f)
			result := reviewResponse[memory.ReviewBatchResult](t, advancedPost(t, f, "batch/resolve", d), 200)
			receipt, err := json.Marshal(result)
			if err != nil || strings.Contains(string(receipt), "protected") || strings.Contains(string(receipt), "Maya") {
				t.Fatalf("group error exposed underlying source details: %s %v", receipt, err)
			}
			if len(result.Groups) != 2 || result.Groups[1].Outcome != "accepted" {
				t.Fatalf("ordered outcomes %+v", result)
			}
			wantShared := "accepted"
			if partial {
				wantShared = "failed"
			}
			if result.Groups[0].Outcome != wantShared {
				t.Fatalf("atomic outcome %+v", result)
			}
			var entities, predicates, claims, resolutions int
			if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM semantic_entities WHERE canonical_name='Maya'),(SELECT count(*) FROM semantic_predicates WHERE token='likes'),(SELECT count(*) FROM semantic_claims WHERE created_operation_id=?),(SELECT count(*) FROM memory_review_resolutions WHERE candidate_id IN (?,?))`, shared.Effect.OperationID, request.Groups[0].Candidates[0].ID, request.Groups[0].Candidates[1].ID).Scan(&entities, &predicates, &claims, &resolutions); err != nil {
				t.Fatal(err)
			}
			if partial {
				if entities+predicates+claims+resolutions != 0 || result.Groups[0].Result != nil || result.Groups[0].FailureCode == "" {
					t.Fatal("failed dependent group leaked effects or lost safe failure")
				}
				if _, err := f.db.Exec(`DROP TRIGGER advanced_fail_source`); err != nil {
					t.Fatal(err)
				}
			} else if entities != 1 || predicates != 1 || claims != 2 || resolutions != 2 {
				t.Fatalf("shared definitions were duplicated: %d/%d/%d/%d", entities, predicates, claims, resolutions)
			}
			a, err := f.store.LocalOwnerReviewContext(context.Background(), "global")
			if err != nil {
				t.Fatal(err)
			}
			kernel, err := f.store.ResolveOwnerCandidateBatch(context.Background(), a, d)
			if err != nil || !reflect.DeepEqual(kernel, result) {
				t.Fatalf("Kernel/HTTP delivery parity %+v %v", kernel, err)
			}
			retry := reviewResponse[memory.ReviewBatchResult](t, advancedPost(t, f, "batch/resolve", d), 200)
			if !reflect.DeepEqual(result, retry) {
				t.Fatal("immutable receipt retried its failed group")
			}
			for _, group := range result.Groups {
				if group.Result == nil || group.Result.Operation == nil {
					continue
				}
				op := reviewResponse[memory.OwnerReviewOperation](t, reviewPost(t, f.handler, "operation", map[string]any{"scope_key": "global", "id": group.Result.Operation.OperationID}), 200)
				if op.Batch == nil || op.Batch.PreviewID != p.ID || op.Batch.GroupID != group.GroupID {
					t.Fatal("durable operation lost batch lineage")
				}
			}
			d.Reason = "changed approval"
			advancedError(t, advancedPost(t, f, "batch/resolve", d), 409, "idempotency_conflict")
			advancedVerify(t, f)
			if _, err = f.db.Exec(`UPDATE memory_review_authorization SET source_policy='batch-redaction'`); err != nil {
				t.Fatal(err)
			}
			advancedError(t, advancedPost(t, f, "batch/inspect", map[string]any{"id": p.ID}), 409, "source_ineligible")
			advancedError(t, advancedPost(t, f, "identity/revision", map[string]any{"id": request.Groups[0].Candidates[0].ID, "revision": request.Groups[0].Candidates[0].InterpretationRevision}), 409, "source_ineligible")
		})
	}
}

func TestCandidateAdvancedHTTPBatchScopeAndWholePreviewStaleness(t *testing.T) {
	f, request := advancedSharedBatch(t)
	p := reviewResponse[memory.ReviewBatchPreview](t, advancedPost(t, f, "batch/prepare", request), 200)
	d := advancedBatchDecision(p)
	for _, call := range []struct {
		route string
		input any
	}{
		{"batch/inspect", map[string]any{"id": p.ID}},
		{"batch/resolve", d},
		{"identity/options", request.Groups[0].Candidates[0]},
	} {
		advancedError(t, reviewPost(t, f.handler, call.route, map[string]any{"scope_key": "session:" + string(f.session.ID), "input": call.input}), 403, "review_unauthorized")
	}
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='changed-before-batch'`); err != nil {
		t.Fatal(err)
	}
	advancedError(t, advancedPost(t, f, "batch/resolve", d), 409, "stale_preview")
	var deliveries, resolutions, entities int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM memory_review_batch_deliveries),(SELECT count(*) FROM memory_review_resolutions),(SELECT count(*) FROM semantic_entities WHERE canonical_name='Maya')`).Scan(&deliveries, &resolutions, &entities); err != nil || deliveries+resolutions+entities != 0 {
		t.Fatalf("whole-preview stale left mutations: %d/%d/%d %v", deliveries, resolutions, entities, err)
	}
}

func TestCandidateAdvancedHTTPEditLineageStalenessAndCurrentRedaction(t *testing.T) {
	f := newWebReviewFixture(t)
	old := webPrepare(t, f, "accept")
	proposal := f.candidate.Candidate.Proposal
	proposal.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "coffee"}
	d := memory.ReviewEditDecision{Candidate: f.candidate.Ref, Proposal: proposal, Reason: "Use the English drink name."}
	edited := reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, "edit", d), 200)
	if edited.Edit == nil || edited.Original == nil || edited.Edit.ParentRevision != 0 || edited.Ref.InterpretationRevision != 1 || edited.Ref.ReviewRevision != 1 || edited.Original.Proposal.Proposition.Object.Literal.Value != "café" || edited.Candidate.Proposal.Proposition.Object.Literal.Value != "coffee" {
		t.Fatalf("edit lineage %+v", edited)
	}
	advancedError(t, advancedPost(t, f, "edit", d), 409, "stale_preview")
	advancedError(t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": webDecision(old)}), 409, "stale_preview")
	history := reviewResponse[memory.ReviewEditRevision](t, advancedPost(t, f, "edit/revision", map[string]any{"id": edited.Ref.ID, "revision": 1}), 200)
	if !reflect.DeepEqual(history, *edited.Edit) || history.Before.Support[0].Authority != memory.AuthorityOwnerStatement || history.After.Support[0].Locator != history.Before.Support[0].Locator {
		t.Fatal("edit inspection lost immutable source authority")
	}
	foreign := reviewPost(t, f.handler, "edit/revision", map[string]any{"scope_key": "session:" + string(f.session.ID), "input": map[string]any{"id": edited.Ref.ID, "revision": 1}})
	advancedError(t, foreign, 403, "review_unauthorized")
	f.candidate = edited
	p := webPrepare(t, f, "accept")
	if p.Version != "owner-review-preview-v5" || p.Effect.Claims[0].Claim.Object.Literal.Value != "coffee" || p.Candidates[0].Edit == nil {
		t.Fatal("edited effect or original extraction omitted")
	}
	decision := webDecision(p)
	decision.Reason = strings.Repeat("\x01", 4096)
	result := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": decision}), 200)
	advancedReopen(t, f)
	persisted := reviewResponse[memory.ReviewEditRevision](t, advancedPost(t, f, "edit/revision", map[string]any{"id": edited.Ref.ID, "revision": 1}), 200)
	if !reflect.DeepEqual(history, persisted) || result.Operation == nil {
		t.Fatal("closed-source edit or operation did not persist")
	}
	advancedVerify(t, f)
	if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='advanced-redaction'`); err != nil {
		t.Fatal(err)
	}
	advancedError(t, advancedPost(t, f, "edit/revision", map[string]any{"id": edited.Ref.ID, "revision": 1}), 409, "source_ineligible")
}

func TestCandidateAdvancedHTTPTemporalCorrectionChoiceAndPersistence(t *testing.T) {
	for _, mode := range []memory.CorrectionMode{memory.CorrectionError, memory.CorrectionChanged} {
		t.Run(string(mode), func(t *testing.T) {
			f := newWebReviewFixture(t)
			effective := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
			content := "I was mistaken about tea. I have always drunk coffee."
			if mode == memory.CorrectionChanged {
				content = "I now drink coffee, beginning May 1 2025. Previously I drank tea."
			}
			items := advancedCompile(t, f, content, memory.CompilerTemporalPolicyV3, func(c memory.ExtractorCandidate) []memory.ExtractorCandidate {
				c.Proposition.Object.Literal = &memory.TypedLiteral{Kind: memory.LiteralText, Value: "coffee"}
				c.Temporal = &memory.CandidateTemporalProposal{Meaning: "assertion", Correction: &memory.CandidateCorrectionProposal{Modes: []memory.CorrectionMode{mode}}}
				if mode == memory.CorrectionChanged {
					c.Temporal.Correction.EffectiveTime, c.ValidTime.From = &effective, &effective
				}
				return []memory.ExtractorCandidate{c}
			})
			if len(items) != 1 {
				t.Fatal("missing correction")
			}
			item := items[0]
			o := reviewResponse[memory.ReviewTemporalOptions](t, advancedPost(t, f, "temporal/options", item.Ref), 200)
			if len(o.Alternatives) != 1 || o.Alternatives[0].Claim.Object.Literal.Value != "tea" {
				t.Fatalf("prior alternatives %+v", o)
			}
			d := memory.ReviewTemporalDecision{Candidate: item.Ref, OptionsSHA256: o.SHA256, Choice: memory.ReviewTemporalChoice{OldClaimID: o.Alternatives[0].Claim.ID, Mode: mode}}
			bad := d
			bad.OptionsSHA256 = strings.Repeat("0", 64)
			advancedError(t, advancedPost(t, f, "temporal/choose", bad), 409, "stale_preview")
			chosen := reviewResponse[memory.OwnerCandidate](t, advancedPost(t, f, "temporal/choose", d), 200)
			history := reviewResponse[memory.ReviewTemporalRevision](t, advancedPost(t, f, "temporal/revision", map[string]any{"id": item.Ref.ID, "revision": chosen.Ref.InterpretationRevision}), 200)
			if chosen.Temporal == nil || !reflect.DeepEqual(history, *chosen.Temporal) {
				t.Fatal("temporal revision mismatch")
			}
			advancedError(t, advancedPost(t, f, "temporal/choose", d), 409, "stale_preview")
			f.candidate = chosen
			p := webPrepare(t, f, "accept")
			if p.Effect.Correction == nil || p.Effect.Correction.Mode != mode || p.Effect.Correction.OldClaim.ID != d.Choice.OldClaimID {
				t.Fatal("correction disclosure changed")
			}
			if mode == memory.CorrectionError && p.Effect.Correction.EffectiveTime != nil {
				t.Fatal("error invented world time")
			}
			if mode == memory.CorrectionChanged && (p.Effect.Correction.ValidTimeEffect.OldAfter.To == nil || !p.Effect.Correction.ValidTimeEffect.OldAfter.To.Equal(effective)) {
				t.Fatal("change lost exact world date")
			}
			result := reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": webDecision(p)}), 200)
			advancedReopen(t, f)
			op := reviewResponse[memory.OwnerReviewOperation](t, reviewPost(t, f.handler, "operation", map[string]any{"scope_key": "global", "id": result.Operation.OperationID}), 200)
			if !reflect.DeepEqual(op.Preview, p) {
				t.Fatal("durable correction differs from HTTP approval")
			}
			advancedVerify(t, f)
			if _, err := f.db.Exec(`UPDATE memory_review_authorization SET source_policy='temporal-redaction'`); err != nil {
				t.Fatal(err)
			}
			advancedError(t, advancedPost(t, f, "temporal/revision", map[string]any{"id": item.Ref.ID, "revision": chosen.Ref.InterpretationRevision}), 409, "source_ineligible")
		})
	}
}
