package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

func TestStage4ScopeAdapterConformance(t *testing.T) {
	f := newStage4ConformanceFixture(t)
	type scopeCase struct {
		name, scope string
		session     memory.Session
		candidate   memory.OwnerCandidate
	}
	cases := []scopeCase{{name: "global", scope: "global", session: f.session}}
	manager := sessionCompositionManager(t)
	resolved, err := manager.ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		workspace, err := f.store.RegisterWorkspace(f.ctx, fmt.Sprintf("Conformance workspace %d", i))
		if err != nil {
			t.Fatal(err)
		}
		session, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{name: fmt.Sprintf("workspace_%d", i), scope: "workspace:" + string(workspace.ID), session: session})
	}
	for i := 0; i < 2; i++ {
		project, err := f.store.RegisterProject(f.ctx, fmt.Sprintf("Conformance project %d", i), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		session, err := f.store.CreateProjectSession(f.ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{name: fmt.Sprintf("project_%d", i), scope: "project:" + string(project.ID), session: session})
	}
	for i := 0; i < 2; i++ {
		session, err := f.store.CreateGlobalSession(f.ctx)
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, scopeCase{name: fmt.Sprintf("session_%d", i), scope: "session:" + string(session.ID), session: session})
	}
	for i := range cases {
		f.session = cases[i].session
		selection := f.foreground("I prefer café in this exact context.")
		selection.Destination = cases[i].scope
		compiled := f.compile(selection, f.generation, f.extractor("café"))
		f.closeSource()
		cases[i].candidate = f.inspectCandidate(compiled, cases[i].scope)
	}
	server := httptest.NewServer(web.WithCandidateReview(web.NewServer(nil), f.store).Handler())
	defer server.Close()
	denials := 0
	receipts := []map[string]any{}
	for i, entry := range cases {
		a := f.authority(entry.scope)
		page, err := f.store.ListOwnerCandidates(f.ctx, a, memory.OwnerCandidateQuery{})
		if err != nil || len(page.Candidates) != 1 || page.Candidates[0].Ref.ID != entry.candidate.Ref.ID {
			t.Fatalf("exact Kernel inbox %s %+v %v", entry.name, page, err)
		}
		var cliPage, httpPage memory.OwnerCandidatePage
		stage4CLI(t, f.store, &cliPage, "inbox", "--scope", entry.scope)
		stage4HTTP(t, server, "list", map[string]any{"scope_key": entry.scope, "limit": 32, "cursor": ""}, 200, &httpPage)
		if !reflect.DeepEqual(page.Candidates, cliPage.Candidates) || !reflect.DeepEqual(page.Candidates, httpPage.Candidates) {
			t.Fatalf("scope %s adapter evidence mismatch", entry.name)
		}
		for j, foreign := range cases {
			if i == j {
				continue
			}
			hidden, err := f.store.InspectOwnerCandidate(f.ctx, a, foreign.candidate.Ref.ID)
			if !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) || len(hidden.Candidate.Support) != 0 {
				t.Fatalf("Kernel foreign source %s->%s %+v %v", entry.name, foreign.name, hidden, err)
			}
			var out bytes.Buffer
			if handled, err := runOwnerReviewManagement(f.ctx, []string{"memory-review", "inspect", "--scope", entry.scope, "--id", foreign.candidate.Ref.ID}, &out, f.store); !handled || !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) || out.Len() != 0 {
				t.Fatalf("CLI foreign source %s->%s %v %s", entry.name, foreign.name, err, out.String())
			}
			body := stage4HTTP(t, server, "inspect", map[string]any{"scope_key": entry.scope, "id": foreign.candidate.Ref.ID}, 403, nil)
			if bytes.Contains(body, []byte("café")) || bytes.Contains(body, []byte(foreign.candidate.Ref.ID)) {
				t.Fatal("HTTP denial disclosed source or candidate identity")
			}
			denials++
		}
		if entry.scope != "global" {
			if _, err = f.store.PrepareOwnerCandidateReview(f.ctx, f.authority("global"), entry.candidate.Ref, "accept"); !errors.Is(err, eviedb.ErrOwnerReviewUnauthorized) {
				t.Fatalf("implicit Promotion preparation %v", err)
			}
			stage4HTTP(t, server, "prepare", map[string]any{"scope_key": "global", "candidate": entry.candidate.Ref, "action": "accept"}, 403, nil)
		}
		var preview memory.ReviewPreview
		stage4CLI(t, f.store, &preview, "prepare", "--scope", entry.scope, "--id", entry.candidate.Ref.ID, "--revision", fmt.Sprint(entry.candidate.Ref.ReviewRevision), "--interpretation", fmt.Sprint(entry.candidate.Ref.InterpretationRevision), "--action", "accept")
		if preview.ScopeKey != entry.scope || preview.Effect == nil || preview.Effect.Scope.Key != entry.scope || preview.Effect.Claims[0].Claim.ScopeKey != entry.scope {
			t.Fatal("scope widened in effect")
		}
		decision := memory.ReviewDecision{DeliveryKey: stage4Key(100 + i), PreviewID: preview.ID, PreviewSHA256: preview.SHA256, Action: "accept"}
		var result memory.ReviewResult
		stage4HTTP(t, server, "resolve", map[string]any{"scope_key": entry.scope, "decision": decision}, 200, &result)
		kernel, err := f.store.ResolveOwnerCandidateReview(f.ctx, a, decision)
		if err != nil || !reflect.DeepEqual(kernel, result) {
			t.Fatalf("scope %s exact result %v", entry.name, err)
		}
		operation, err := f.store.InspectOwnerReviewOperation(f.ctx, a, result.Operation.OperationID)
		if err != nil || !reflect.DeepEqual(operation.Preview, preview) {
			t.Fatalf("scope %s preview/provenance %v", entry.name, err)
		}
		receipts = append(receipts, map[string]any{"scope_family": entry.name, "scope": entry.scope, "candidate_id": entry.candidate.Ref.ID, "preview_sha256": preview.SHA256, "effect_sha256": preview.EffectSHA256, "operation_id": result.Operation.OperationID})
	}
	observer, err := f.store.CreateGlobalSession(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := f.store.InspectLiteralClaims(f.ctx, observer.ScopeContext())
	if err != nil || len(claims.Claims) != 2 {
		t.Fatalf("private acceptance implicitly promoted %+v %v", claims, err)
	}
	for _, claim := range claims.Claims {
		if claim.Scope.Key != "global" {
			t.Fatal("private claim in global accepted read")
		}
	}
	verified, err := f.store.VerifySemanticProjection(f.ctx)
	if err != nil || !verified.Valid {
		t.Fatalf("scope canonical replay %+v %v", verified, err)
	}
	var closed int
	if err = f.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE status='closed'`).Scan(&closed); err != nil || closed != len(cases) {
		t.Fatal("source review changed session lifecycle", closed, err)
	}
	stage4Evidence(t, "scope_adapter_matrix", map[string]any{"scope_cases": receipts, "cross_scope_denials_per_surface": denials, "closed_sources": closed, "implicit_promotion_denied": true, "canonical_replay": true})
}

func TestStage4GenericStorageConformance(t *testing.T) {
	f := newStage4ConformanceFixture(t)
	selection := f.foreground("I prefer café.")
	compiled := f.compile(selection, f.generation, f.extractor("café"))
	if len(compiled.Candidates) != 1 {
		t.Fatal("missing protected candidate")
	}
	// Configure the actual query tool with a read-only opener bound to this temporary
	// SQLite database. No HOME changes and no access to the user's default DB.
	if _, err := f.db.Exec(`INSERT INTO jobs (name,schedule,command,created_at) VALUES ('stage4-public-control','* * * * *','true','2026-09-05T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	opens := 0
	tool := tools.QueryDBToolWithEvieReader(func(ctx context.Context) (*sql.DB, error) {
		opens++
		path := (&url.URL{Scheme: "file", Path: f.path, RawQuery: "mode=ro"}).String()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		if err = db.PingContext(ctx); err != nil {
			db.Close()
			return nil, err
		}
		return db, nil
	})
	set := tools.NewToolset([]tools.Tool{tool})
	queryTool := func(query string) (openrouter.Message, bool, error) {
		args, err := json.Marshal(map[string]string{"db": "evie", "query": query})
		if err != nil {
			t.Fatal(err)
		}
		return set.ExecuteWithApprovalAuthorizedCompletion(f.ctx, openrouter.ToolCall{ID: "conformance-query", Type: "function", Function: openrouter.FunctionCall{Name: "query_db", Arguments: string(args)}}, nil, nil, nil, nil)
	}
	for _, query := range []string{`SELECT name FROM jobs`, `SELECT (SELECT name FROM jobs LIMIT 1) AS name FROM jobs`} {
		message, isErr, err := queryTool(query)
		if err != nil || isErr || message.Content != "name\nstage4-public-control\n(1 rows)\n" {
			t.Fatalf("allowed temporary-database control failed: %t %v %q", isErr, err, message.Content)
		}
	}
	if opens != 2 {
		t.Fatalf("allowed control did not open the temporary database twice: %d", opens)
	}
	// Enumerate actual protected tables/views, then distinguish identifier denial
	// from syntax-form rejection. A missing table, unavailable DB or dispatch error
	// must never count as containment.
	rows, err := f.db.Query(`SELECT name FROM sqlite_schema WHERE type IN ('table','view') AND (name LIKE 'memory_compiler_%' OR name LIKE 'memory_review_%' OR name LIKE 'semantic_%') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	denied := 0
	for _, name := range tables {
		queries := []struct{ query, want string }{
			{fmt.Sprintf(`SELECT * FROM %s`, name), fmt.Sprintf("evie table %q is not available through query_db", name)},
			{fmt.Sprintf(`SELECT (SELECT COUNT(*) FROM %s) FROM jobs`, name), fmt.Sprintf("evie table %q is not available through query_db", name)},
			{fmt.Sprintf(`SELECT * FROM "%s"`, name), "evie queries may not use quoted table names"},
			{fmt.Sprintf(`SELECT (SELECT COUNT(*) FROM main."%s") FROM jobs`, name), `evie table "main" is not available through query_db`},
		}
		for _, query := range queries {
			message, isErr, err := queryTool(query.query)
			if err != nil || !isErr || message.Content != "tool call came back with error "+query.want || message.Role != "tool" || message.ToolCallID != "conformance-query" {
				t.Fatalf("generic storage fence %s: got error=%t transport=%v output=%q want=%q", query.query, isErr, err, message.Content, query.want)
			}
			if opens != 2 {
				t.Fatalf("protected query reached the SQLite opener: %s", query.query)
			}
			denied++
		}
	}
	claims, err := f.store.InspectLiteralClaims(f.ctx, f.session.ScopeContext())
	if err != nil || len(claims.Claims) != 1 {
		t.Fatalf("unaccepted candidate escaped into accepted reads: %+v %v", claims, err)
	}

	if len(tables) < 40 {
		t.Fatalf("protected schema inventory incomplete: %d", len(tables))
	}
	stage4Evidence(t, "generic_storage_containment", map[string]any{"protected_tables_and_views": len(tables), "denied_model_queries": denied, "allowed_fixture_queries": 2, "blocked_query_db_opens": 0, "exact_policy_rejections": true, "unquoted_direct_and_nested_queries": true, "unaccepted_candidate_isolated": true, "privileged_shell_claim": "unchanged documented limitation"})
}
