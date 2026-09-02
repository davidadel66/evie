package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

type stage3ContextController struct {
	workspace memory.Workspace
	session   memory.Session
	agent     *agent.Session
}

func (c *stage3ContextController) Snapshot(context.Context) (web.ContextSessionSnapshot, error) {
	return web.ContextSessionSnapshot{
		Workspaces: []memory.Workspace{c.workspace},
		Sessions:   []memory.SessionListing{{Session: c.session}},
	}, nil
}

func (*stage3ContextController) RegisterWorkspace(context.Context, string) (memory.Workspace, error) {
	return memory.Workspace{}, fmt.Errorf("registration is outside the acceptance path")
}

func (c *stage3ContextController) SelectSession(context.Context, web.ContextSessionSelection) (web.OpenedContextSession, error) {
	return web.OpenedContextSession{Session: c.session, Agent: c.agent}, nil
}

func stage3MemoryTool(t *testing.T, plugin *plugins.Memory, name string) tools.Tool {
	t.Helper()
	for _, capability := range plugin.ToolCapabilities() {
		if capability.Tool.Schema.Function.Name == name {
			return capability.Tool
		}
	}
	t.Fatalf("Memory Capability %q is unavailable", name)
	return tools.Tool{}
}

func stage3HTTPRequest(path, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6687"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return request
}

type stage3AcceptanceHarness struct {
	t       *testing.T
	ctx     context.Context
	store   *eviedb.Store
	session memory.Session
	holder  memory.LeaseHolderID
}

func (h stage3AcceptanceHarness) exactApproval(
	lease memory.TurnLease,
	parent memory.EventID,
	operation memory.SemanticID,
	proposalHash string,
	preparedHash string,
) {
	h.t.Helper()
	payload, err := json.Marshal(memory.ApprovalPayload{
		Decision: memory.ApprovalApproved, ProposalSHA256: proposalHash, PreparedSHA256: preparedHash,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.store.AppendEventWithLease(h.ctx, lease.SessionID, lease.HolderID, lease.FencingToken, memory.EventInput{
		ParentID: parent, Type: memory.EventApproval, ExecutionID: memory.ExecutionID(operation), Payload: payload,
	}); err != nil {
		h.t.Fatal(err)
	}
}

func (h stage3AcceptanceHarness) lifecycle(
	key string,
	command string,
	action memory.MemoryLifecycleAction,
	kind memory.SemanticObjectKind,
	id memory.SemanticID,
) memory.MemoryLifecycleResult {
	h.t.Helper()
	lease, err := h.store.AcquireTurnLease(h.ctx, h.session.ID, h.holder, time.Minute)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		if err := h.store.ReleaseTurnLease(h.ctx, h.session.ID, h.holder, lease.FencingToken); err != nil {
			h.t.Fatal(err)
		}
	}()
	event, err := h.store.AppendEventWithLease(h.ctx, h.session.ID, h.holder, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: command,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	proposal, err := h.store.PrepareMemoryLifecycle(h.ctx, h.session.ScopeContext(), memory.MemoryLifecycleRequest{
		IdempotencyKey: key, SourceEventID: event.ID, Action: action, ObjectKind: kind, ObjectID: id,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	h.exactApproval(lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	result, err := h.store.ApplyMemoryLifecycle(h.ctx, lease, proposal)
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}

func (h stage3AcceptanceHarness) graphLink(
	key string,
	source memory.SemanticID,
	target memory.SemanticID,
) memory.CreateGraphLinkResult {
	h.t.Helper()
	lease, err := h.store.AcquireTurnLease(h.ctx, h.session.ID, h.holder, time.Minute)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		if err := h.store.ReleaseTurnLease(h.ctx, h.session.ID, h.holder, lease.FencingToken); err != nil {
			h.t.Fatal(err)
		}
	}()
	event, err := h.store.AppendEventWithLease(h.ctx, h.session.ID, h.holder, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Record the explicit structural derivation",
	})
	if err != nil {
		h.t.Fatal(err)
	}
	proposal, err := h.store.PrepareCreateGraphLink(h.ctx, h.session.ScopeContext(), memory.CreateGraphLinkRequest{
		IdempotencyKey: key, SourceEventID: event.ID, Relation: memory.GraphRelationDerivation,
		Source: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: source},
		Target: memory.GraphEndpoint{Kind: memory.SemanticObjectClaim, ID: target},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	h.exactApproval(lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	result, err := h.store.ApplyCreateGraphLink(h.ctx, lease, proposal)
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}

func (h stage3AcceptanceHarness) promotion(
	key string,
	claimID memory.SemanticID,
) memory.PromotionResult {
	h.t.Helper()
	lease, err := h.store.AcquireTurnLease(h.ctx, h.session.ID, h.holder, time.Minute)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() {
		if err := h.store.ReleaseTurnLease(h.ctx, h.session.ID, h.holder, lease.FencingToken); err != nil {
			h.t.Fatal(err)
		}
	}()
	event, err := h.store.AppendEventWithLease(h.ctx, h.session.ID, h.holder, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Promote the source-linked Claim globally",
	})
	if err != nil {
		h.t.Fatal(err)
	}
	proposal, err := h.store.PreparePromotion(h.ctx, h.session.ScopeContext(), memory.PromotionRequest{
		IdempotencyKey: key, SourceEventID: event.ID, SourceClaimID: claimID, DestinationScopeKey: "global",
	})
	if err != nil {
		h.t.Fatal(err)
	}
	h.exactApproval(lease, event.ID, proposal.OperationID, proposal.ProposalSHA256, proposal.PreparedSHA256)
	result, err := h.store.ApplyPromotion(h.ctx, lease, proposal)
	if err != nil {
		h.t.Fatal(err)
	}
	return result
}

func stage3DecodeInspection(t *testing.T, raw string) memory.SemanticObjectInspection {
	t.Helper()
	start, end := strings.IndexByte(raw, '{'), strings.LastIndexByte(raw, '}')
	if start < 0 || end < start {
		t.Fatalf("Semantic Memory inspection did not contain JSON: %s", raw)
	}
	var result memory.SemanticObjectInspection
	if err := json.Unmarshal([]byte(raw[start:end+1]), &result); err != nil {
		t.Fatalf("decode Semantic Memory inspection: %v\n%s", err, raw)
	}
	return result
}

func stage3HasOperation(inspection memory.SemanticObjectInspection, operationID memory.SemanticID) bool {
	for _, operation := range inspection.Operations {
		if operation.OperationID == operationID {
			return true
		}
	}
	return false
}

func stage3JSONDifference(left, right memory.SemanticObjectInspection) string {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	index := 0
	for index < limit && a[index] == b[index] {
		index++
	}
	start := index - 80
	if start < 0 {
		start = 0
	}
	endA, endB := index+160, index+160
	if endA > len(a) {
		endA = len(a)
	}
	if endB > len(b) {
		endB = len(b)
	}
	return fmt.Sprintf("offset=%d left=%s right=%s", index, a[start:endA], b[start:endB])
}

func stage3CrossSurfaceInspection(
	t *testing.T,
	ctx context.Context,
	session *agent.Session,
	store *eviedb.Store,
	handler http.Handler,
	plugin *plugins.Memory,
	scope memory.ScopeContext,
	scopeKey string,
	claimID memory.SemanticID,
	validAt time.Time,
	asKnownAt time.Time,
) memory.SemanticObjectInspection {
	t.Helper()
	valid, known := validAt.UTC().Format(time.RFC3339Nano), asKnownAt.UTC().Format(time.RFC3339Nano)

	var cli bytes.Buffer
	command := fmt.Sprintf("/memory inspect claim %s --scope %s --history --valid-at %s --as-known-at %s", claimID, scopeKey, valid, known)
	if !handleMemoryCommand(ctx, session, store, command, &cli) {
		t.Fatalf("CLI did not handle %q", command)
	}
	cliResult := stage3DecodeInspection(t, cli.String())

	httpRead := httptest.NewRecorder()
	handler.ServeHTTP(httpRead, stage3HTTPRequest("/api/memory/inspect", fmt.Sprintf(
		`{"scopeKey":%q,"kind":"claim","id":%q,"validAt":%q,"asKnownAt":%q}`, scopeKey, claimID, valid, known,
	)))
	if httpRead.Code != http.StatusOK {
		t.Fatalf("HTTP inspection status=%d body=%s", httpRead.Code, httpRead.Body.String())
	}
	httpResult := stage3DecodeInspection(t, httpRead.Body.String())

	modelRead, modelReadError, err := tools.NewToolset([]tools.Tool{stage3MemoryTool(t, plugin, "memory_inspect_object")}).ExecuteWithApprovalAuthorizedCompletion(
		tools.WithInvocationContext(ctx, tools.InvocationContext{Scope: scope}),
		openrouter.ToolCall{ID: "stage3-parity-read", Type: "function", Function: openrouter.FunctionCall{
			Name: "memory_inspect_object", Arguments: fmt.Sprintf(`{"object_kind":"claim","object_id":%q,"valid_at":%q,"as_known_at":%q}`, claimID, valid, known),
		}}, nil, nil, nil, nil,
	)
	if err != nil || modelReadError {
		t.Fatalf("model-facing inspection = content %q, tool_error=%v, error=%v", modelRead.Content, modelReadError, err)
	}
	modelResult := stage3DecodeInspection(t, modelRead.Content)

	if cliResult.Metadata.SelectedScope != scopeKey || httpResult.Metadata.SelectedScope != scopeKey ||
		len(cliResult.Metadata.AllowedScopes) != 1 || cliResult.Metadata.AllowedScopes[0] != scopeKey ||
		len(httpResult.Metadata.AllowedScopes) != 1 || httpResult.Metadata.AllowedScopes[0] != scopeKey ||
		len(modelResult.Metadata.AllowedScopes) != 3 {
		t.Fatalf("surface-specific authorization bounds were not explicit: CLI=%+v HTTP=%+v model=%+v",
			cliResult.Metadata, httpResult.Metadata, modelResult.Metadata)
	}
	// CLI and HTTP carry the user's explicit selection while the focused model
	// Capability carries the full invocation Context. Compare the accepted
	// object, full provenance, conflicts, and operations independently of that
	// intentionally surface-specific authorization envelope.
	cliResult.Metadata = memory.ExactReadMetadata{}
	httpResult.Metadata = memory.ExactReadMetadata{}
	modelResult.Metadata = memory.ExactReadMetadata{}
	cliJSON, _ := json.Marshal(cliResult)
	httpJSON, _ := json.Marshal(httpResult)
	modelJSON, _ := json.Marshal(modelResult)
	if !bytes.Equal(cliJSON, httpJSON) || !bytes.Equal(cliJSON, modelJSON) {
		t.Fatalf("cross-surface structured parity mismatch: CLI/HTTP %s; CLI/model %s",
			stage3JSONDifference(cliResult, httpResult), stage3JSONDifference(cliResult, modelResult))
	}
	return cliResult
}

func TestSemanticMemoryStage3CrossSurfaceAcceptance(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	store := eviedb.NewStore(db)
	plugin := plugins.NewMemory(store)
	manager, err := plugins.NewManager(
		tools.NewToolset(nil), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugin,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{
		plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.MemoryPluginID,
	} {
		if err := manager.Enable(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := manager.ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "Stage 3 acceptance")
	if err != nil {
		t.Fatal(err)
	}
	storedSession, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	holder := memory.LeaseHolderID("stage3-cross-surface")
	harness := stage3AcceptanceHarness{t: t, ctx: ctx, store: store, session: storedSession, holder: holder}
	model := &noSemanticModelClient{}
	session := agent.New(model, evieTestContextProfile("stage3-acceptance"),
		store.BindHistory(storedSession.ID, holder), storedSession.ScopeContext(), store.BindTurnOwner(storedSession.ID, holder))

	var cliMutation bytes.Buffer
	runREPLContextIOWithMemory(ctx, session, bufio.NewScanner(strings.NewReader(
		"/remember timezone_name Detroit\ny\n",
	)), &cliMutation, store)
	if !strings.Contains(cliMutation.String(), "Remembered Claim") || !strings.Contains(cliMutation.String(), "workspace:") {
		t.Fatalf("CLI literal mutation output = %q", cliMutation.String())
	}
	literals, err := store.InspectClaims(ctx, storedSession.ScopeContext(), memory.ClaimQuery{PredicateToken: "timezone_name"})
	if err != nil || len(literals.Claims) != 1 {
		t.Fatalf("literal Claim after CLI mutation = %+v, %v", literals, err)
	}
	ownerID := literals.Claims[0].Subject.ID

	lease, err := store.AcquireTurnLease(ctx, storedSession.ID, holder, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.AppendEventWithLease(ctx, storedSession.ID, holder, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "Remember that I work with Acme",
	})
	if err != nil {
		t.Fatal(err)
	}
	toolCtx := tools.WithInvocationContext(ctx, tools.InvocationContext{Scope: storedSession.ScopeContext(), Lease: lease, SourceEventID: source.ID})
	approvedProposal := ""
	approvalObserved := false
	resultMessage, isToolError, err := tools.NewToolset([]tools.Tool{stage3MemoryTool(t, plugin, "memory_remember_entity")}).ExecuteWithApprovalAuthorizedCompletion(
		toolCtx,
		openrouter.ToolCall{ID: "stage3-entity", Type: "function", Function: openrouter.FunctionCall{
			Name:      "memory_remember_entity",
			Arguments: fmt.Sprintf(`{"idempotency_key":"idem:v1:ba000000-0000-4000-8000-000000000001","predicate":"works_with","predicate_label":"works with","cardinality":"many","polarity":"affirmed","subject_entity_id":%q,"object_create":true,"object_name":"Acme","object_type":"organization","object_alias":"ACME"}`, ownerID),
		}},
		func(_ context.Context, name, arguments string, _ *tools.FileChangePreview) tools.Decision {
			if name != "memory_remember_entity" || !strings.Contains(arguments, `"operation_id"`) {
				t.Fatalf("Action Approval did not receive the exact focused proposal: name=%q arguments=%s", name, arguments)
			}
			approvedProposal = arguments
			return tools.Approved
		},
		func(observeCtx context.Context, decision tools.Decision, metadata tools.ApprovalMetadata) error {
			if decision != tools.Approved || metadata.Arguments != approvedProposal || metadata.ProposalSHA256 == "" || metadata.PreparedSHA256 == "" {
				return fmt.Errorf("approval metadata did not bind the exact prepared proposal")
			}
			payload, err := json.Marshal(memory.ApprovalPayload{Decision: memory.ApprovalApproved, ProposalSHA256: metadata.ProposalSHA256, PreparedSHA256: metadata.PreparedSHA256})
			if err != nil {
				return err
			}
			_, err = store.AppendEventWithLease(observeCtx, storedSession.ID, holder, lease.FencingToken, memory.EventInput{
				ParentID: metadata.ParentEventID, Type: memory.EventApproval, ExecutionID: metadata.ExecutionID, Payload: payload,
			})
			approvalObserved = err == nil
			return err
		},
		func(authorizeCtx context.Context, boundary tools.AuthorizationBoundary) error {
			if boundary == tools.AuthorizeExecution && !approvalObserved {
				return fmt.Errorf("plugin execution started before durable Action Approval")
			}
			return store.AuthorizeTurnLease(authorizeCtx, storedSession.ID, holder, lease.FencingToken)
		},
		nil,
	)
	if err != nil || isToolError {
		t.Fatalf("model-facing Entity mutation = content %q, tool_error=%v, error=%v", resultMessage.Content, isToolError, err)
	}
	var entityResult memory.RememberEntityResult
	if err := json.Unmarshal([]byte(resultMessage.Content), &entityResult); err != nil {
		t.Fatal(err)
	}
	if entityResult.ClaimID == "" || entityResult.OperationID == "" || !approvalObserved {
		t.Fatalf("model-facing Entity result = %+v, approval observed=%v", entityResult, approvalObserved)
	}
	if err := store.ReleaseTurnLease(ctx, storedSession.ID, holder, lease.FencingToken); err != nil {
		t.Fatal(err)
	}

	var cliRead bytes.Buffer
	command := fmt.Sprintf("/memory inspect claim %s --scope context --history", entityResult.ClaimID)
	if !handleMemoryCommand(ctx, session, store, command, &cliRead) {
		t.Fatalf("CLI did not handle %q", command)
	}
	for _, identity := range []string{string(entityResult.ClaimID), string(entityResult.OperationID)} {
		if !strings.Contains(cliRead.String(), identity) {
			t.Fatalf("CLI inspection omitted shared identity %q: %s", identity, cliRead.String())
		}
	}

	controller := &stage3ContextController{workspace: workspace, session: storedSession, agent: session}
	handler := web.NewContextMemoryServer(nil, nil, nil, controller, store).Handler()
	selected := httptest.NewRecorder()
	handler.ServeHTTP(selected, stage3HTTPRequest("/api/context-sessions/select", fmt.Sprintf(`{"sessionId":%q}`, storedSession.ID)))
	if selected.Code != http.StatusOK {
		t.Fatalf("HTTP session selection status=%d body=%s", selected.Code, selected.Body.String())
	}
	webRead := httptest.NewRecorder()
	handler.ServeHTTP(webRead, stage3HTTPRequest("/api/memory/inspect", fmt.Sprintf(
		`{"scopeKey":%q,"kind":"claim","id":%q}`, "workspace:"+string(workspace.ID), entityResult.ClaimID,
	)))
	if webRead.Code != http.StatusOK {
		t.Fatalf("HTTP inspection status=%d body=%s", webRead.Code, webRead.Body.String())
	}
	for _, identity := range []string{string(entityResult.ClaimID), string(entityResult.OperationID)} {
		if !strings.Contains(webRead.Body.String(), identity) {
			t.Fatalf("HTTP inspection omitted shared identity %q: %s", identity, webRead.Body.String())
		}
	}

	modelCtx := tools.WithInvocationContext(ctx, tools.InvocationContext{Scope: storedSession.ScopeContext()})
	modelRead, modelReadError, err := tools.NewToolset([]tools.Tool{stage3MemoryTool(t, plugin, "memory_inspect_object")}).ExecuteWithApprovalAuthorizedCompletion(
		modelCtx,
		openrouter.ToolCall{ID: "stage3-read", Type: "function", Function: openrouter.FunctionCall{
			Name: "memory_inspect_object", Arguments: fmt.Sprintf(`{"object_kind":"claim","object_id":%q}`, entityResult.ClaimID),
		}}, nil, nil, nil, nil,
	)
	if err != nil || modelReadError {
		t.Fatalf("model-facing exact read = content %q, tool_error=%v, error=%v", modelRead.Content, modelReadError, err)
	}
	for _, identity := range []string{string(entityResult.ClaimID), string(entityResult.OperationID)} {
		if !strings.Contains(modelRead.Content, identity) {
			t.Fatalf("model-facing inspection omitted shared identity %q: %s", identity, modelRead.Content)
		}
	}
	if !strings.HasPrefix(modelRead.Content, "[begin untrusted semantic memory") || strings.Contains(modelRead.Content, "reasoning_details") || strings.Contains(modelRead.Content, "provider_payload") {
		t.Fatalf("model-facing rendering crossed its egress boundary: %s", modelRead.Content)
	}

	// The remainder of the same accepted object path exercises temporal,
	// provenance, lifecycle, structural, and Promotion operations before the
	// release script runs the exhaustive failure matrices around it.
	corroborating, err := session.PrepareRememberLiteral(ctx, store, "Confirm that my timezone is Detroit", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:ba000000-0000-4000-8000-000000000002",
		Predicate:      "timezone_name", PredicateLabel: "timezone name", PredicateCardinality: memory.CardinalityOne,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Detroit"}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	corroborated, err := session.ResolveRememberLiteral(ctx, store, corroborating, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	if corroborated.ClaimID != literals.Claims[0].ID || corroborated.SourceLinkID == literals.Claims[0].Sources[0].ID {
		t.Fatalf("corroboration did not reuse the Claim with new evidence: %+v", corroborated)
	}

	errorProposal, err := session.PrepareCorrectClaim(ctx, store, "Correction: Detroit was an error; use Chicago", memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:ba000000-0000-4000-8000-000000000003",
		OldClaimID:     corroborated.ClaimID,
		Mode:           memory.CorrectionError,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: literals.Claims[0].SubjectEntityID,
			PredicateID:     literals.Claims[0].Predicate.ID,
			Object:          memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Chicago"}},
			Polarity:        memory.PolarityAffirmed,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	errorResult, err := session.ResolveCorrectClaim(ctx, store, errorProposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	effective := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	changedProposal, err := session.PrepareCorrectClaim(ctx, store, "Chicago changed to Grand Rapids on April 1", memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:ba000000-0000-4000-8000-000000000004",
		OldClaimID:     errorResult.ReplacementClaimID,
		Mode:           memory.CorrectionChanged,
		EffectiveTime:  &effective,
		Replacement: memory.ClaimProposition{
			SubjectEntityID: errorProposal.ReplacementClaim.SubjectEntityID,
			PredicateID:     errorProposal.ReplacementClaim.Predicate.ID,
			Object:          memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "Grand Rapids"}},
			Polarity:        memory.PolarityAffirmed,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	changedResult, err := session.ResolveCorrectClaim(ctx, store, changedProposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	oldKnownAt := corroborated.TransactionTime
	oldView, err := store.InspectClaims(ctx, storedSession.ScopeContext(), memory.ClaimQuery{AsKnownAt: &oldKnownAt, PredicateToken: "timezone_name"})
	if err != nil || len(oldView.Claims) != 1 || oldView.Claims[0].Object.Literal.Value != "Detroit" {
		t.Fatalf("as-known-at temporal answer = %+v, %v", oldView, err)
	}
	currentView, err := store.InspectClaims(ctx, storedSession.ScopeContext(), memory.ClaimQuery{PredicateToken: "timezone_name"})
	if err != nil || len(currentView.Claims) != 1 || currentView.Claims[0].ID != changedResult.ReplacementClaimID {
		t.Fatalf("current temporal answer = %+v, %v", currentView, err)
	}
	conflictProposal, err := session.PrepareRememberLiteral(ctx, store, "There is conflicting evidence that my timezone is not Grand Rapids", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:ba000000-0000-4000-8000-000000000011",
		Predicate:      "timezone_name", PredicateLabel: "timezone name", PredicateCardinality: memory.CardinalityOne,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "Grand Rapids"}, Polarity: memory.PolarityDenied,
		ValidTime: memory.ValidTime{From: &effective},
	})
	if err != nil {
		t.Fatal(err)
	}
	conflicting, err := session.ResolveRememberLiteral(ctx, store, conflictProposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	currentKnownAt := conflicting.TransactionTime.Add(time.Nanosecond)
	entityParity := stage3CrossSurfaceInspection(t, ctx, session, store, handler, plugin,
		storedSession.ScopeContext(), "workspace:"+string(workspace.ID), entityResult.ClaimID,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), currentKnownAt)
	if entityParity.Claim == nil || entityParity.Claim.Object.EntityID == "" || len(entityParity.Sources) != 1 ||
		!stage3HasOperation(entityParity, entityResult.OperationID) {
		t.Fatalf("Entity Claim parity omitted object, provenance, or operation identity: %+v", entityParity)
	}
	historicalParity := stage3CrossSurfaceInspection(t, ctx, session, store, handler, plugin,
		storedSession.ScopeContext(), "workspace:"+string(workspace.ID), corroborated.ClaimID,
		time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), oldKnownAt)
	if historicalParity.Claim == nil || historicalParity.Claim.Object.Literal == nil || historicalParity.Claim.Object.Literal.Value != "Detroit" ||
		len(historicalParity.Sources) != 2 || !stage3HasOperation(historicalParity, corroborated.OperationID) {
		t.Fatalf("historical parity omitted answer, full provenance, or operation identity: %+v", historicalParity)
	}
	currentParity := stage3CrossSurfaceInspection(t, ctx, session, store, handler, plugin,
		storedSession.ScopeContext(), "workspace:"+string(workspace.ID), changedResult.ReplacementClaimID,
		time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), currentKnownAt)
	if currentParity.Claim == nil || currentParity.Claim.Object.Literal == nil || currentParity.Claim.Object.Literal.Value != "Grand Rapids" ||
		len(currentParity.Sources) != 1 || !stage3HasOperation(currentParity, changedResult.OperationID) || len(currentParity.Conflicts) == 0 {
		t.Fatalf("current parity omitted answer, provenance, operation identity, or conflict diagnostics: %+v", currentParity)
	}
	conflictFound := false
	for _, warning := range currentParity.Conflicts {
		if warning.Code == memory.ConflictOppositePolarity && len(warning.ClaimIDs) == 2 &&
			((warning.ClaimIDs[0] == changedResult.ReplacementClaimID && warning.ClaimIDs[1] == conflicting.ClaimID) ||
				(warning.ClaimIDs[1] == changedResult.ReplacementClaimID && warning.ClaimIDs[0] == conflicting.ClaimID)) {
			conflictFound = true
		}
	}
	if !conflictFound {
		t.Fatalf("cross-surface conflict diagnostics omitted exact accepted Claim identities: %+v", currentParity.Conflicts)
	}

	harness.lifecycle(
		"idem:v1:ba000000-0000-4000-8000-000000000005", "Retract the Entity Claim source",
		memory.LifecycleRetractSource, memory.SemanticObjectSourceLink, entityResult.SourceLinkID)
	unsupported, err := store.InspectSemanticObject(ctx, storedSession.ScopeContext(), memory.SemanticObjectClaim, entityResult.ClaimID)
	if err != nil || unsupported.Status != memory.SemanticStatusUnsupported {
		t.Fatalf("source retraction did not make the Claim unsupported: %+v, %v", unsupported, err)
	}
	harness.lifecycle(
		"idem:v1:ba000000-0000-4000-8000-000000000006", "Restore the Entity Claim source",
		memory.LifecycleRestoreSource, memory.SemanticObjectSourceLink, entityResult.SourceLinkID)
	harness.lifecycle(
		"idem:v1:ba000000-0000-4000-8000-000000000007", "Retire the Entity Claim",
		memory.LifecycleRetire, memory.SemanticObjectClaim, entityResult.ClaimID)
	harness.lifecycle(
		"idem:v1:ba000000-0000-4000-8000-000000000008", "Restore the Entity Claim",
		memory.LifecycleRestore, memory.SemanticObjectClaim, entityResult.ClaimID)
	link := harness.graphLink(
		"idem:v1:ba000000-0000-4000-8000-000000000009", entityResult.ClaimID, changedResult.ReplacementClaimID)
	promotion := harness.promotion(
		"idem:v1:ba000000-0000-4000-8000-000000000010", entityResult.ClaimID)
	if link.GraphLinkID == "" || promotion.DestinationClaimID == "" || promotion.DestinationClaimID == promotion.SourceClaimID {
		t.Fatalf("structural Link or source-linked Promotion result = %+v / %+v", link, promotion)
	}

	page, err := store.ListSemanticObjects(ctx, storedSession.ScopeContext(), memory.SemanticObjectListQuery{
		ClaimQuery: memory.ClaimQuery{ScopeKey: "workspace:" + string(workspace.ID)}, PageSize: 1,
	})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("pre-restart paginated inspection = %+v, %v", page, err)
	}
	eventsBeforeRestart, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	pinnedReceipt, err := store.GetCompositionReceipt(ctx, storedSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	reopenedReceipt, err := store.GetCompositionReceipt(ctx, storedSession.ID)
	if err != nil || !reflect.DeepEqual(reopenedReceipt, pinnedReceipt) {
		t.Fatalf("Composition Receipt changed across restart: got=%+v want=%+v error=%v", reopenedReceipt, pinnedReceipt, err)
	}
	restartedPlugin := plugins.NewMemory(store)
	restartedManager, err := plugins.NewManager(
		tools.NewToolset(nil), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), restartedPlugin,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{
		plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.MemoryPluginID,
	} {
		if err := restartedManager.Enable(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	resumed, err := restartedManager.ResumeComposition(reopenedReceipt)
	if err != nil || !reflect.DeepEqual(resumed.Receipt, reopenedReceipt) {
		t.Fatalf("Memory Capability identities did not resume exactly: receipt=%+v error=%v", resumed.Receipt, err)
	}
	nextPage, err := store.ListSemanticObjects(ctx, storedSession.ScopeContext(), memory.SemanticObjectListQuery{
		ClaimQuery: memory.ClaimQuery{ScopeKey: "workspace:" + string(workspace.ID)}, PageSize: 1, Cursor: page.NextCursor,
	})
	if err != nil || len(nextPage.Objects) != 1 || !reflect.DeepEqual(nextPage.Metadata.ScopeRevisions, page.Metadata.ScopeRevisions) {
		t.Fatalf("restart did not preserve snapshot-pinned pagination: first=%+v next=%+v error=%v", page, nextPage, err)
	}
	restartedClaim, err := store.InspectSemanticObject(ctx, storedSession.ScopeContext(), memory.SemanticObjectClaim, entityResult.ClaimID)
	if err != nil || restartedClaim.Status != memory.SemanticStatusActive || len(restartedClaim.Sources) != 1 || len(restartedClaim.Lifecycle) != 3 {
		t.Fatalf("restart did not preserve accepted lifecycle/provenance: %+v, %v", restartedClaim, err)
	}
	verification, err := store.VerifySemanticProjection(ctx)
	if err != nil || !verification.Valid {
		t.Fatalf("read-only Semantic Memory verification = %+v, %v", verification, err)
	}

	if _, err := db.ExecContext(ctx, `DROP TRIGGER semantic_claims_append_only_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE semantic_claims SET literal_value = 'damaged projection' WHERE claim_id = ?`, changedResult.ReplacementClaimID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER semantic_claims_append_only_update BEFORE UPDATE ON semantic_claims BEGIN SELECT RAISE(ABORT, 'semantic claims are append-only'); END`); err != nil {
		t.Fatal(err)
	}
	damaged, err := store.VerifySemanticProjection(ctx)
	if err != nil || damaged.Valid {
		t.Fatalf("deliberate projection damage was not detected: %+v, %v", damaged, err)
	}
	quarantinedScopes := 0
	for _, scope := range damaged.Scopes {
		if scope.Quarantined {
			quarantinedScopes++
			if scope.ScopeKey != "workspace:"+string(workspace.ID) {
				t.Fatalf("projection damage quarantined unrelated scope %q", scope.ScopeKey)
			}
		}
	}
	if quarantinedScopes != 1 {
		t.Fatalf("projection damage quarantined %d scopes: %+v", quarantinedScopes, damaged.Scopes)
	}
	if _, err := store.InspectSemanticObject(ctx, storedSession.ScopeContext(), memory.SemanticObjectClaim, entityResult.ClaimID); !errors.Is(err, eviedb.ErrSemanticScopeQuarantined) {
		t.Fatalf("damaged scope did not fail closed: %v", err)
	}
	globalSession, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if globalClaim, err := store.InspectSemanticObject(ctx, globalSession.ScopeContext(), memory.SemanticObjectClaim, promotion.DestinationClaimID); err != nil || globalClaim.Status != memory.SemanticStatusActive {
		t.Fatalf("unrelated global scope did not remain operational: %+v, %v", globalClaim, err)
	}
	rebuild, err := store.OwnerRebuildSemanticProjection(ctx, "stage3-acceptance-owner")
	if err != nil || !rebuild.Valid || rebuild.FencingToken <= 0 {
		t.Fatalf("owner rebuild did not restore verified projection: %+v, %v", rebuild, err)
	}
	restored, err := store.InspectSemanticObject(ctx, storedSession.ScopeContext(), memory.SemanticObjectClaim, changedResult.ReplacementClaimID)
	if err != nil || restored.Claim == nil || restored.Claim.Object.Literal == nil || restored.Claim.Object.Literal.Value != "Grand Rapids" {
		t.Fatalf("owner rebuild did not restore canonical Claim: %+v, %v", restored, err)
	}
	eventsAfterRebuild, err := store.LoadEvents(ctx, storedSession.ID)
	if err != nil || !reflect.DeepEqual(eventsAfterRebuild, eventsBeforeRestart) {
		t.Fatalf("verify/rebuild changed Episodic Memory: before=%d after=%d error=%v", len(eventsBeforeRestart), len(eventsAfterRebuild), err)
	}
	var laterStageObjects int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%'
		  AND (lower(name) LIKE '%candidate%' OR lower(name) LIKE '%fts%' OR
		       lower(name) LIKE '%vector%' OR lower(name) LIKE '%ranking%' OR
		       lower(name) LIKE '%cache%' OR lower(name) LIKE '%hard_eras%')
	`).Scan(&laterStageObjects); err != nil || laterStageObjects != 0 {
		t.Fatalf("Stage 3 created later-stage schema objects: count=%d error=%v", laterStageObjects, err)
	}
	for _, capability := range restartedPlugin.ToolCapabilities() {
		name := capability.Tool.Schema.Function.Name
		for _, forbidden := range []string{"extract", "candidate", "search", "rank", "context", "cache", "forget", "erase", "verify", "rebuild", "visual"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("Stage 3 exposed later-stage model Capability %q", name)
			}
		}
	}
	if model.calls != 0 {
		t.Fatalf("explicit Semantic Memory acceptance path made %d provider calls", model.calls)
	}
}
