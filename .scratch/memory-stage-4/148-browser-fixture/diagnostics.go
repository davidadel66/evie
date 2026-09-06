package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

type diagnosticScenarioExtractor struct {
	webReviewExtractor
	failRoot memory.EventID
}

func (diagnosticScenarioExtractor) VerifyCompilerConfiguration(context.Context, memory.CompilerGeneration) error {
	return nil
}
func (x diagnosticScenarioExtractor) Extract(ctx context.Context, generation memory.CompilerGeneration, request memory.CompilerRequest) (eviedb.CompilerExtraction, error) {
	if request.Window.Selection.RootID == x.failRoot {
		return eviedb.CompilerExtraction{ReleaseEvidence: "completed"}, eviedb.ErrCompilerTerminalOutput
	}
	return x.webReviewExtractor.Extract(ctx, generation, request)
}

func seedDiagnosticBrowserFixture(t *fixtureHarness, f *webReviewFixture) {
	ctx := context.Background()
	session, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, "diagnostic-browser", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent := func(input memory.EventInput) memory.Event {
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
	for i := 0; i < 18; i++ {
		r := appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Synthetic outside-selection message."})
		appendEvent(memory.EventInput{ParentID: r.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Noted."})
	}
	if err = f.store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	x := diagnosticScenarioExtractor{webReviewExtractor: webReviewExtractor{f.subject, f.predicate}, failRoot: firstRoot.ID}
	g1, g2, g3 := webReviewGeneration(), webReviewGeneration(), webReviewGeneration()
	g1.Prompt += " Earlier diagnostic generation."
	g2.Prompt += " Later diagnostic generation."
	g3.Prompt += " Unavailable diagnostic generation."
	selectUnit := func(root, end memory.Event, g memory.CompilerGeneration, x eviedb.CompilerExtractor) memory.Compilation {
		r, err := f.store.QueueCandidateUnit(ctx, session.ScopeContext(), memory.CompilationSelection{SessionID: session.ID, RootID: root.ID, Cutoff: end.Sequence, Destination: "global"}, g, x)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	earlier := selectUnit(firstRoot, firstEnd, g1, x)
	later := selectUnit(laterRoot, laterEnd, g2, x)
	config := eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{earlier.GenerationID: x, later.GenerationID: x}}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || !errors.Is(err, eviedb.ErrCompilerTerminalOutput) {
		t.Fatal("earlier failure: ", worked, " ", err)
	}
	if worked, err := f.store.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatal("later completion: ", worked, " ", err)
	}
	endpoint := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint.Close()
	runtime, err := localextractor.New(localextractor.Config{Endpoint: endpoint.URL, Generation: g3})
	if err != nil {
		t.Fatal(err)
	}
	unavailable := selectUnit(firstRoot, firstEnd, g3, runtime)
	if worked, err := f.store.RunCompilerStep(ctx, eviedb.CompilerSupervisorConfig{Extractors: map[string]eviedb.CompilerExtractor{unavailable.GenerationID: runtime}}); !worked || !errors.Is(err, eviedb.ErrCompilerEndpointUnavailable) {
		t.Fatal("endpoint failure: ", worked, " ", err)
	}
	_, err = f.store.SelectCompilerHistory(ctx, []memory.ScopeContext{session.ScopeContext()}, memory.CompilerHistoryRequest{RequestID: "browser-history", Ranges: []memory.CompilerHistoryRange{{SourceScope: "global", Destination: "global", SessionID: session.ID, FirstSequence: firstRoot.Sequence, LastSequence: firstEnd.Sequence, FirstEventID: firstRoot.ID, LastEventID: firstEnd.ID}}}, g1, x)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.store.ActivateCompiler(ctx, session.ScopeContext(), memory.CompilerActivationRequest{RequestID: "browser-activation", Selector: memory.CompilerLiveSelector{SourceScope: "global", Destination: "global", SessionID: session.ID}}, g1, x)
	if err != nil {
		t.Fatal(err)
	}
	lease, err = f.store.AcquireTurnLease(ctx, session.ID, "diagnostic-browser-live", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	appendEvent(memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "An unfinished synthetic live turn."})
	if _, err = f.store.ReconcileCompilerEvidence(ctx, config); err != nil {
		t.Fatal(err)
	}
	if err = f.store.ReleaseTurnLease(ctx, session.ID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE sessions SET status='closed' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"session_id": session.ID, "earlier_generation": earlier.GenerationID, "later_generation": later.GenerationID, "earlier_job": earlier.JobID, "later_job": later.JobID, "unavailable_job": unavailable.JobID, "candidate_session": f.session.ID})
	fmt.Println("BROWSER_DIAGNOSTIC=" + string(raw))
}
