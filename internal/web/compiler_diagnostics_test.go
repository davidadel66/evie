package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/web"
)

func diagnosticPost(t *testing.T, handler http.Handler, path string, scope string, input any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(map[string]any{"scope_key": scope, "input": input})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/memory/compiler/"+path, strings.NewReader(string(b)))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("diagnostics must not be cached")
	}
	return w
}

func TestCompilerDiagnosticsHTTPGuards(t *testing.T) {
	f := newWebReviewFixture(t)
	for _, route := range []string{"sessions", "diagnostics"} {
		t.Run(route, func(t *testing.T) {
			probe := `{"scope_key":"workspace:missing","input":{}}`
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
				{"outer authority", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{},"authority":{}}`, 400},
				{"nested authority", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{"owner_id":"local"}}`, 400},
				{"duplicate", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{"limit":1,"limit":2}}`, 400},
				{"trailing", "POST", "127.0.0.1", "", "application/json", `{"scope_key":"global","input":{}}{}`, 400},
				{"UTF8", "POST", "127.0.0.1", "", "application/json", "{\"scope_key\":\"global\",\"input\":{\"cursor\":\"\xff\"}}", 400},
				{"foreign scope", "POST", "127.0.0.1", "", "application/json", probe, 403},
				{"oversize", "POST", "127.0.0.1", "", "application/json", strings.Repeat(" ", 8193), 413},
				{"inclusive bound", "POST", "127.0.0.1", "", "application/json", probe + strings.Repeat(" ", 8192-len(probe)), 403},
			} {
				t.Run(test.name, func(t *testing.T) {
					r := httptest.NewRequest(test.method, "http://"+test.host+"/api/memory/compiler/"+route, strings.NewReader(test.body))
					r.Header.Set("Origin", test.origin)
					r.Header.Set("Content-Type", test.content)
					w := httptest.NewRecorder()
					f.handler.ServeHTTP(w, r)
					if w.Code != test.status || w.Header().Get("Cache-Control") != "no-store" {
						t.Fatalf("%d: %s", w.Code, w.Body)
					}
					if strings.Contains(w.Body.String(), "café") || strings.Contains(w.Body.String(), f.path) {
						t.Fatalf("unsafe diagnostics: %s", w.Body)
					}
				})
			}
		})
	}
}

func TestCompilerDiagnosticsHTTPRealClosedSourceParityAndBounds(t *testing.T) {
	f := newWebReviewFixture(t)
	ctx := context.Background()
	a, err := f.store.LocalOwnerReviewContext(ctx, "global")
	if err != nil {
		t.Fatal(err)
	}
	sessions := reviewResponse[memory.CompilerDiagnosticSessions](t, diagnosticPost(t, f.handler, "sessions", "global", memory.CompilerDiagnosticSessionQuery{Limit: 32}), 200)
	if len(sessions.SessionIDs) != 1 || sessions.SessionIDs[0] != f.session.ID {
		t.Fatalf("closed source session absent: %+v", sessions)
	}
	for _, view := range []string{"jobs", "candidates", "activations", "history", "selection", "selections", "live_roots", "foreground"} {
		q := memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: view, Limit: 32}
		if view == "selection" {
			q.GenerationID = f.candidate.GenerationID
		}
		// Finish the bounded legacy projection before exact metadata comparison.
		direct, err := f.store.InspectOwnerCompilerDiagnostics(ctx, a, q)
		if err != nil {
			t.Fatal(err)
		}
		response := diagnosticPost(t, f.handler, "diagnostics", "global", q)
		got := reviewResponse[memory.CompilerDiagnostics](t, response, 200)
		if got.AsOfUnixMS < direct.AsOfUnixMS {
			t.Fatal("snapshot moved backwards")
		}
		direct.AsOfUnixMS = got.AsOfUnixMS
		if !reflect.DeepEqual(direct, got) {
			t.Fatalf("%s adapter altered Kernel projection\nwant %+v\ngot %+v", view, direct, got)
		}
		for _, secret := range []string{"café", "Tea preferences", "Recorded.", "Extract owner assertions", f.path, "model_manifest", "continuation", "subject_entity_id"} {
			if strings.Contains(response.Body.String(), secret) {
				t.Fatalf("protected value %q in diagnostics: %s", secret, response.Body)
			}
		}
		if view == "candidates" && (len(got.Candidates) != 1 || got.Candidates[0].Ref != f.candidate.Ref || got.Candidates[0].DecidedAtUnixMS != nil) {
			t.Fatalf("candidate review lineage missing: %+v", got.Candidates)
		}
	}
	for _, input := range []memory.CompilerDiagnosticsQuery{
		{SessionID: f.session.ID, View: "jobs", Limit: 33}, {SessionID: f.session.ID, View: "jobs", Limit: -1}, {SessionID: f.session.ID, View: "unknown"},
		{SessionID: f.session.ID, View: "selection"}, {SessionID: f.session.ID, View: "jobs", GenerationID: f.candidate.GenerationID},
	} {
		advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "global", input), 400, "invalid_diagnostic_request")
	}
	for _, limit := range []int{-1, 33} {
		advancedError(t, diagnosticPost(t, f.handler, "sessions", "global", memory.CompilerDiagnosticSessionQuery{Limit: limit}), 400, "invalid_diagnostic_request")
	}
	advancedError(t, diagnosticPost(t, f.handler, "sessions", "global", memory.CompilerDiagnosticSessionQuery{Cursor: "forged"}), 400, "invalid_cursor")
	advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "global", memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "jobs", Cursor: "forged"}), 400, "invalid_cursor")
	// A currently valid session scope still cannot inspect that source as global
	// evidence in a different exact destination or expose another session's rows.
	scope := "session:" + string(f.session.ID)
	foreign, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	advancedError(t, diagnosticPost(t, f.handler, "diagnostics", scope, memory.CompilerDiagnosticsQuery{SessionID: foreign.ID, View: "jobs"}), 403, "review_unauthorized")
	page := reviewResponse[memory.CompilerDiagnostics](t, diagnosticPost(t, f.handler, "diagnostics", scope, memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "candidates"}), 200)
	if len(page.Candidates) != 0 {
		t.Fatalf("global candidates crossed destination: %+v", page)
	}
	// Decisions change review metadata, without returning the approved proposition.
	preview := webPrepare(t, f, "reject")
	reviewResponse[memory.ReviewResult](t, reviewPost(t, f.handler, "resolve", map[string]any{"scope_key": "global", "decision": webDecision(preview)}), 200)
	got := reviewResponse[memory.CompilerDiagnostics](t, diagnosticPost(t, f.handler, "diagnostics", "global", memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "candidates"}), 200)
	if len(got.Candidates) != 1 || got.Candidates[0].ReviewState != "rejected" || got.Candidates[0].DecidedAtUnixMS == nil {
		t.Fatalf("recorded outcome missing: %+v", got)
	}
}

type diagnosticFailureKernel struct {
	*eviedb.Store
	err error
}

func (k diagnosticFailureKernel) InspectOwnerCompilerDiagnostics(context.Context, eviedb.OwnerReviewContext, memory.CompilerDiagnosticsQuery) (memory.CompilerDiagnostics, error) {
	return memory.CompilerDiagnostics{}, k.err
}

func TestCompilerDiagnosticsHTTPContainsUnknownErrors(t *testing.T) {
	f := newWebReviewFixture(t)
	for _, test := range []struct {
		err    error
		status int
		code   string
	}{
		{errors.New("SQLite SELECT secret FROM private at /private/database.db"), 503, "diagnostics_retryable"},
		{eviedb.ErrOwnerReviewUnauthorized, 403, "review_unauthorized"},
		{eviedb.ErrInvalidCursor, 400, "invalid_cursor"},
		{eviedb.ErrReviewTooLarge, 413, "diagnostics_too_large"},
	} {
		h := web.WithCandidateReview(web.NewServer(nil), diagnosticFailureKernel{f.store, test.err}).Handler()
		w := diagnosticPost(t, h, "diagnostics", "global", memory.CompilerDiagnosticsQuery{SessionID: f.session.ID, View: "jobs"})
		advancedError(t, w, test.status, test.code)
		if strings.Contains(w.Body.String(), "SELECT") || strings.Contains(w.Body.String(), "/private/") {
			t.Fatalf("error escaped containment: %s", w.Body)
		}
	}
}

type diagnosticScenarioExtractor struct {
	webReviewExtractor
	failRoot memory.EventID
}

func (x diagnosticScenarioExtractor) Extract(ctx context.Context, generation memory.CompilerGeneration, request memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	if request.Window.Selection.RootID == x.failRoot {
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, eviedb.ErrCompilerTerminalOutput
	}
	return x.webReviewExtractor.Extract(ctx, generation, request)
}

func TestCompilerDiagnosticsHTTPFailedGapLaterSuccessAndCursorScope(t *testing.T) {
	f := newWebReviewFixture(t)
	ctx := context.Background()
	session, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, "diagnostic-http-scenario", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
		t.Helper()
		event, err := f.store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event
	}
	firstRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	firstEnd := appendEvent(memory.EventInput{ParentID: firstRoot.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Noted."})
	laterRoot := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer café."})
	laterEnd := appendEvent(memory.EventInput{ParentID: laterRoot.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Noted."})
	if err = f.store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	extractor := diagnosticScenarioExtractor{webReviewExtractor: webReviewExtractor{f.subject, f.predicate}, failRoot: firstRoot.ID}
	earlierGeneration := webReviewGeneration()
	earlierGeneration.Prompt += " Early diagnostic generation."
	laterGeneration := webReviewGeneration()
	laterGeneration.Prompt += " Later diagnostic generation."
	earlier, err := f.store.QueueCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: firstRoot.ID, Cutoff: firstEnd.Sequence, Destination: "global"}, earlierGeneration, extractor)
	if err != nil {
		t.Fatal(err)
	}
	later, err := f.store.QueueCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: laterRoot.ID, Cutoff: laterEnd.Sequence, Destination: "global"}, laterGeneration, extractor)
	if err != nil {
		t.Fatal(err)
	}
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{earlier.GenerationID: extractor, later.GenerationID: extractor}}
	queued := reviewResponse[memory.CompilerDiagnostics](t, diagnosticPost(t, f.handler, "diagnostics", "global", memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs"}), 200)
	if len(queued.Jobs) != 2 || queued.Counts["jobs_queued"] != 2 {
		t.Fatalf("queued progress missing: %+v", queued)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || !errors.Is(err, eviedb.ErrCompilerTerminalOutput) {
		t.Fatalf("earlier failure: %v %v", worked, err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatalf("later success: %v %v", worked, err)
	}
	q := memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs", Limit: 1}
	jobs := map[string]memory.CompilerDiagnosticJob{}
	for pages := 0; pages < 4; pages++ {
		page := reviewResponse[memory.CompilerDiagnostics](t, diagnosticPost(t, f.handler, "diagnostics", "global", q), 200)
		if len(page.Jobs) > 1 {
			t.Fatal("HTTP page exceeded requested bound")
		}
		if page.Counts["jobs_failed"] != 1 || page.Counts["jobs_completed_candidates"] != 1 {
			t.Fatalf("later completion swallowed failed gap: %+v", page.Counts)
		}
		for _, job := range page.Jobs {
			if _, exists := jobs[job.JobID]; exists {
				t.Fatal("repeated keyset row")
			}
			jobs[job.JobID] = job
		}
		if page.NextCursor == "" {
			break
		}
		crossView := q
		crossView.View = "candidates"
		crossView.Cursor = page.NextCursor
		advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "global", crossView), 400, "invalid_cursor")
		crossScope := q
		crossScope.Cursor = page.NextCursor
		advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "session:"+string(session.ID), crossScope), 400, "invalid_cursor")
		crossSession := q
		crossSession.SessionID = f.session.ID
		crossSession.Cursor = page.NextCursor
		advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "global", crossSession), 400, "invalid_cursor")
		q.Cursor = page.NextCursor
	}
	first, ok := jobs[earlier.JobID]
	if !ok || first.State != "failed" || first.CompletedNewEvents != 0 || first.SelectedNewEvents != int64(len(earlier.Window.NewEventIDs)) {
		t.Fatalf("failed gap concealed: %+v", first)
	}
	second, ok := jobs[later.JobID]
	if !ok || second.State != "completed_candidates" || second.CompletedNewEvents != int64(len(later.Window.NewEventIDs)) {
		t.Fatalf("later coverage missing: %+v", second)
	}
	if len(second.Measurements) != 1 || second.Measurements[0].InferenceNanos == nil || second.PublicationNanos == nil || second.PublishedAtUnixMS == nil || second.CandidateFreshnessNanos != nil {
		t.Fatalf("observed/missing timing semantics changed: %+v", second)
	}
	selectionQ := memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "selection", GenerationID: earlier.GenerationID, Limit: 1}
	selectionPage := reviewResponse[memory.CompilerDiagnostics](t, diagnosticPost(t, f.handler, "diagnostics", "global", selectionQ), 200)
	if selectionPage.NextCursor == "" {
		t.Fatal("selection cursor fixture too small")
	}
	selectionQ.GenerationID = later.GenerationID
	selectionQ.Cursor = selectionPage.NextCursor
	advancedError(t, diagnosticPost(t, f.handler, "diagnostics", "global", selectionQ), 400, "invalid_cursor")
	// An actual unavailable loopback endpoint is a safe distinct reason. Metadata
	// verification fails before sending evidence, so no model release is guessed.
	unavailableGeneration := webReviewGeneration()
	unavailableGeneration.Prompt += " Unavailable endpoint diagnostic generation."
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint.Close()
	runtime, err := localextractor.New(localextractor.Config{Endpoint: endpoint.URL, Generation: unavailableGeneration})
	if err != nil {
		t.Fatal(err)
	}
	unavailable, err := f.store.QueueCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: firstRoot.ID, Cutoff: firstEnd.Sequence, Destination: "global"}, unavailableGeneration, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{unavailable.GenerationID: runtime}}); !worked || !errors.Is(err, eviedb.ErrCompilerEndpointUnavailable) {
		t.Fatalf("missing endpoint failure: %v %v", worked, err)
	}
	endpointResponse := diagnosticPost(t, f.handler, "diagnostics", "global", memory.CompilerDiagnosticsQuery{SessionID: session.ID, View: "jobs"})
	endpointPage := reviewResponse[memory.CompilerDiagnostics](t, endpointResponse, 200)
	found := false
	for _, job := range endpointPage.Jobs {
		if job.JobID == unavailable.JobID {
			found = true
			if job.State != "retry_wait" || job.Reason != "endpoint_unavailable" || job.RetryAt == 0 || job.Recovery == "" || job.CompletedNewEvents != 0 {
				t.Fatalf("endpoint failure hidden: %+v", job)
			}
		}
	}
	if !found || endpointPage.CapacityState != "available" || strings.Contains(endpointResponse.Body.String(), endpoint.URL) {
		t.Fatalf("endpoint status leaked details or lost retry: %s", endpointResponse.Body)
	}
}
