package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

// This opt-in helper serves only disposable scripted data to the external
// browser driver. Ordinary go test runs do not launch a long-lived HTTP server.
// SIGUSR1 requests final assertions; SIGINT/SIGTERM only clean up an aborted run.
func TestStage4BrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_STAGE4_BROWSER_FIXTURE") != "1" {
		return
	}
	f := newStage4ConformanceFixture(t)
	type scopeCase struct {
		Name      string                `json:"name"`
		Scope     string                `json:"scope"`
		Session   memory.Session        `json:"session"`
		Candidate memory.OwnerCandidate `json:"candidate"`
	}
	cases := []scopeCase{{Name: "global", Scope: "global", Session: f.session}}
	manager := sessionCompositionManager(t)
	resolved, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 2; i++ {
		workspace, err := f.store.RegisterWorkspace(f.ctx, fmt.Sprintf("Browser workspace %d", i))
		if err != nil {
			t.Fatal(err)
		}
		session, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{Name: fmt.Sprintf("workspace-%d", i), Scope: "workspace:" + string(workspace.ID), Session: session})
	}
	for i := 1; i <= 2; i++ {
		project, err := f.store.RegisterProject(f.ctx, fmt.Sprintf("Browser project %d", i), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		session, err := f.store.CreateProjectSession(f.ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{Name: fmt.Sprintf("project-%d", i), Scope: "project:" + string(project.ID), Session: session})
	}
	for i := 1; i <= 2; i++ {
		session, err := f.store.CreateGlobalSession(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{Name: fmt.Sprintf("session-%d", i), Scope: "session:" + string(session.ID), Session: session})
	}
	for i := range cases {
		f.session = cases[i].Session
		selection := f.foreground("I prefer café in this exact context.")
		selection.Destination = cases[i].Scope
		compiled := f.compile(selection, f.generation, f.extractor("café"))
		f.closeSource()
		cases[i].Candidate = f.inspectCandidate(compiled, cases[i].Scope)
		cases[i].Session, err = f.store.GetSession(f.ctx, cases[i].Session.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	observer, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime := agent.NewWithToolset(stage4ConversationClient{&f.modelCalls}, evieTestContextProfile("stage4-browser"), f.store.BindHistory(observer.ID, "browser-observer"), observer.ScopeContext(), f.store.BindTurnOwner(observer.ID, "browser-observer"), tools.NewToolset(nil))
	controller := &stage4BrowserController{session: observer, runtime: runtime}
	handler := web.WithCandidateReview(web.NewContextMemoryServer(nil, nil, nil, controller, f.store), f.store).Handler()
	selected := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/context-sessions/select", strings.NewReader(fmt.Sprintf(`{"sessionId":%q}`, observer.ID)))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(selected, request)
	if selected.Code != http.StatusOK {
		t.Fatalf("select real global observer: %d %s", selected.Code, selected.Body.String())
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	raw, err := json.Marshal(map[string]any{"version": "memory-stage-4-browser-fixture-v1", "url": server.URL, "database": f.path, "cases": cases})
	if err != nil {
		t.Fatal(err)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	defer signal.Stop(signals)
	fmt.Println("STAGE4_BROWSER_READY=" + string(raw))
	if <-signals != syscall.SIGUSR1 {
		return
	}
	server.Close()
	// Bind final checks to the actual operation IDs and hashes returned by the
	// browser. Candidate lineage only describes recurrence origins, so it is
	// not the source of a primary candidate's own accepted operation.
	var receipts []struct {
		Name          string            `json:"name"`
		CandidateID   string            `json:"candidate_id"`
		OperationID   memory.SemanticID `json:"operation_id"`
		PreviewSHA256 string            `json:"preview_sha256"`
		EffectSHA256  string            `json:"effect_sha256"`
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, 65537))
	if err != nil || len(input) > 65536 || json.Unmarshal(input, &receipts) != nil || len(receipts) != len(cases) {
		t.Fatalf("invalid browser operation receipts: %v", err)
	}
	modelCalls := f.modelCalls.Load()
	proofs := []map[string]any{}
	for i, entry := range cases {
		receipt := receipts[i]
		if receipt.Name != entry.Name || receipt.CandidateID != entry.Candidate.Ref.ID {
			t.Fatalf("browser receipt identity mismatch for %s", entry.Name)
		}
		item, err := f.store.InspectOwnerCandidate(f.ctx, f.authority(entry.Scope), entry.Candidate.Ref.ID)
		if err != nil || item.Candidate.ReviewState != "accepted" {
			t.Fatalf("browser did not accept %s: %+v %v", entry.Name, item, err)
		}
		operation, err := f.store.InspectOwnerReviewOperation(f.ctx, f.authority(entry.Scope), receipt.OperationID)
		if err != nil || operation.Preview.ScopeKey != entry.Scope || operation.Preview.SHA256 != receipt.PreviewSHA256 || operation.Preview.EffectSHA256 != receipt.EffectSHA256 || len(operation.Preview.Candidates) != 1 || operation.Preview.Candidates[0].Ref.ID != receipt.CandidateID || operation.Preview.Effect == nil || len(operation.Preview.Effect.Claims) != 1 {
			t.Fatalf("accepted operation %s: %v", entry.Name, err)
		}
		reader := observer
		switch {
		case entry.Session.WorkspaceID != "":
			reader, err = f.store.CreateWorkspaceSessionWithComposition(f.ctx, entry.Session.WorkspaceID, entry.Session.WorkspaceRevisionSnapshot, resolved.Receipt)
		case entry.Session.ProjectID != "":
			reader, err = f.store.CreateProjectSession(f.ctx, entry.Session.ProjectID)
		}
		if err != nil {
			t.Fatal(err)
		}
		effect := operation.Preview.Effect.Claims[0]
		if strings.HasPrefix(entry.Scope, "session:") {
			// Exact closed-session destinations have no active read context.
			// Check the disposable projection against the owner-visible operation;
			// canonical replay below independently verifies accepted state.
			var count int
			err = f.db.QueryRow(`SELECT COUNT(*) FROM semantic_claims c JOIN semantic_scopes s ON s.scope_id=c.scope_id WHERE c.claim_id=? AND s.scope_key=? AND c.created_operation_id=? AND c.literal_value='café' AND c.lifecycle='active'`, effect.Claim.ID, entry.Scope, operation.OperationID).Scan(&count)
			if err != nil || count != 1 || len(effect.Sources) == 0 {
				t.Fatalf("accepted closed-session projection %s: %d %v", entry.Name, count, err)
			}
			for _, source := range effect.Sources {
				err = f.db.QueryRow(`SELECT COUNT(*) FROM semantic_source_links l JOIN semantic_scopes s ON s.scope_id=l.scope_id WHERE l.source_link_id=? AND l.claim_id=? AND s.scope_key=? AND l.created_operation_id=? AND l.source_session_id=? AND l.event_id=? AND l.evidence_sha256=? AND l.authority='owner_statement' AND l.eligibility='eligible'`, source.ID, effect.Claim.ID, entry.Scope, operation.OperationID, entry.Session.ID, source.EventID, source.EvidenceSHA256).Scan(&count)
				if err != nil || count != 1 {
					t.Fatalf("accepted closed-session provenance %s: %d %v", entry.Name, count, err)
				}
			}
		} else {
			claim, err := f.store.InspectSemanticObjectAt(f.ctx, reader.ScopeContext(), memory.SemanticObjectClaim, effect.Claim.ID, memory.ClaimQuery{ScopeKey: entry.Scope})
			if err != nil || claim.Scope.Key != entry.Scope || len(claim.Sources) == 0 {
				t.Fatalf("accepted graph/provenance %s: %+v %v", entry.Name, claim, err)
			}
		}
		proofs = append(proofs, map[string]any{"name": entry.Name, "scope": entry.Scope, "operation_id": operation.OperationID, "preview_sha256": operation.Preview.SHA256, "effect_sha256": operation.Preview.EffectSHA256, "accepted_graph": true})
	}
	global, err := f.store.InspectLiteralClaims(f.ctx, observer.ScopeContext())
	if err != nil || len(global.Claims) != 2 {
		t.Fatalf("private browser acceptances widened global graph: %+v %v", global, err)
	}
	var closed int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status='closed'`).Scan(&closed); err != nil || closed != len(cases) {
		t.Fatalf("source sessions reopened: %d %v", closed, err)
	}
	verified, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || !verified.Valid || f.modelCalls.Load() != modelCalls {
		t.Fatalf("browser acceptance replay: %+v %v", verified, err)
	}
	raw, _ = json.Marshal(map[string]any{"version": "memory-stage-4-browser-kernel-v1", "status": "passed", "cases": proofs, "closed_sources": closed, "global_claims": len(global.Claims), "canonical_replay": true, "replay_model_calls": 0})
	fmt.Println("STAGE4_BROWSER_VERIFIED=" + string(raw))
}

type stage4BrowserController struct {
	session memory.Session
	runtime *agent.Session
}

func (c *stage4BrowserController) Snapshot(context.Context) (web.ContextSessionSnapshot, error) {
	return web.ContextSessionSnapshot{Workspaces: []memory.Workspace{}, Projects: []memory.Project{}, Sessions: []memory.SessionListing{{Session: c.session}}}, nil
}
func (*stage4BrowserController) RegisterWorkspace(context.Context, string) (memory.Workspace, error) {
	return memory.Workspace{}, fmt.Errorf("browser observer does not register scopes")
}
func (c *stage4BrowserController) SelectSession(_ context.Context, selection web.ContextSessionSelection) (web.OpenedContextSession, error) {
	if selection.SessionID != c.session.ID {
		return web.OpenedContextSession{}, fmt.Errorf("browser observer session mismatch")
	}
	return web.OpenedContextSession{Session: c.session, Agent: c.runtime}, nil
}
