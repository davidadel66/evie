package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemoryEvidenceActivitySummarizesRecordedHistoryAndConflicts(t *testing.T) {
	conflict := memory.ClaimConflictWarning{Code: memory.ConflictOneCardinality, PredicateToken: "home", ClaimIDs: []memory.SemanticID{"claim-boston", "claim-denver"}}
	payload, err := json.Marshal(memory.ContextSnapshotPayload{Memory: &memory.RetrievalReceipt{
		Version: "memory-retrieval-v1", Status: memory.RetrievalSuccess,
		Evidence: []memory.RetrievalReference{
			{ID: "claim-boston", Kind: memory.RetrievalAcceptedMemory, Intent: memory.RetrievalHistorical, Status: memory.SemanticStatusActive, CurrentStatus: memory.SemanticStatusRetired, Conflicts: []memory.ClaimConflictWarning{conflict}},
			{ID: "claim-denver", Kind: memory.RetrievalAcceptedMemory, Intent: memory.RetrievalHistorical, Status: memory.SemanticStatusActive, CurrentStatus: memory.SemanticStatusActive, Conflicts: []memory.ClaimConflictWarning{conflict}},
			{ID: "excerpt-chicago", Kind: memory.RetrievalConversationExcerpt, Intent: memory.RetrievalCurrent, Status: memory.SemanticStatusActive, CurrentStatus: memory.SemanticStatusActive, RelatedClaimIDs: []memory.SemanticID{"claim-boston"}},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	stream, err := newSSEEvents(recorder)
	if err != nil {
		t.Fatal(err)
	}
	var activity agent.MemoryActivityEvents = stream
	activity.MemoryRetrieved(memory.Event{ID: "original-request", Type: memory.EventContextSnapshot, Payload: payload})
	for _, expected := range []string{`"snapshotId":"original-request"`, `"acceptedCount":2`, `"excerptCount":1`, `"historicalCount":2`, `"retiredCount":1`, `"conflictCount":2`} {
		if !strings.Contains(recorder.Body.String(), expected) {
			t.Fatalf("original evidence annotation %s missing: %s", expected, recorder.Body.String())
		}
	}
	for _, forbidden := range []string{"claim-boston", "claim-denver", "excerpt-chicago", "home"} {
		if strings.Contains(recorder.Body.String(), forbidden) {
			t.Fatalf("compact activity disclosed source metadata: %s", recorder.Body.String())
		}
	}
}

func TestHistoricalMemoryEvidenceHTTPRetainsOriginalStateAfterRestoration(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	source, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimID := seedWebMemoryLiteral(t, store, source, "089", "Boston")
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	lifecycle := func(action string) {
		t.Helper()
		args, err := json.Marshal(map[string]string{"idempotency_key": "idem:v1:" + uuid.NewString(), "object_kind": "claim", "object_id": string(claimID)})
		if err != nil {
			t.Fatal(err)
		}
		client := &fakeClient{steps: []fakeStep{
			{toolCalls: []openrouter.ToolCall{{ID: "lifecycle", Type: "function", Function: openrouter.FunctionCall{Name: "memory_" + action, Arguments: string(args)}}}},
			{content: "Lifecycle change recorded."},
		}}
		runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(source.ID, "historical-source"), source.ScopeContext(), store.BindTurnOwner(source.ID, "historical-source"), tools.NewToolset(definitions))
		stream, err := newSSEEvents(httptest.NewRecorder())
		if err != nil {
			t.Fatal(err)
		}
		if err = runtime.Send(ctx, "Apply the requested saved-memory lifecycle change.", stream, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
			t.Fatal(err)
		}
		for _, message := range client.reqs[len(client.reqs)-1].Messages {
			if message.Role == "tool" && strings.Contains(message.Content, "Error:") {
				t.Fatalf("fixture lifecycle failed: %s", message.Content)
			}
		}
	}
	lifecycle("retire")
	coverage, err := store.RefreshMemoryIndex(ctx, 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("index=%+v: %v", coverage, err)
	}
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "historical-lookup", Type: "function", Function: openrouter.FunctionCall{Name: "memory_search", Arguments: `{"query":"Boston","intent":"historical"}`}}}},
		{content: "Historical statement received."},
	}}
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(reader.ID, "historical-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "historical-reader"), tools.NewToolset(definitions))
	stream := httptest.NewRecorder()
	streamEvents, err := newSSEEvents(stream)
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "What had I recorded before retiring that memory?", streamEvents, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stream.Body.String(), `"historicalCount":1`) || !strings.Contains(stream.Body.String(), `"retiredCount":1`) {
		t.Fatalf("historical retired activity missing: %s", stream.Body.String())
	}
	history, err := runtime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotID memory.EventID
	var original memory.RetrievalReference
	for _, event := range history {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil && len(snapshot.Memory.Evidence) == 1 && snapshot.Memory.Evidence[0].ClaimID == claimID {
			snapshotID, original = event.ID, snapshot.Memory.Evidence[0]
		}
	}
	if snapshotID == "" || original.Intent != memory.RetrievalHistorical || original.CurrentStatus != memory.SemanticStatusRetired {
		t.Fatalf("historical request did not preserve its original retired reference: %+v", original)
	}
	lifecycle("restore")
	server := NewContextMemoryServer(runtime, nil, nil, &fakeContextSessionController{}, store)
	server.activeSession = reader
	body, err := json.Marshal(map[string]string{"sessionId": string(reader.ID), "snapshotId": string(snapshotID)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("inspection status=%d body=%s", rr.Code, rr.Body.String())
	}
	var inspected struct {
		Evidence []memory.RetrievalInspection `json:"evidence"`
	}
	if err = json.Unmarshal(rr.Body.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if len(inspected.Evidence) != 1 || !inspected.Evidence[0].Available || inspected.Evidence[0].Evidence == nil {
		t.Fatalf("historical reference unavailable after restoration: %s", rr.Body.String())
	}
	item := inspected.Evidence[0]
	if !reflect.DeepEqual(item.Reference, original) || item.CurrentStatus != memory.SemanticStatusActive || item.Evidence.CurrentStatus != memory.SemanticStatusActive {
		t.Fatalf("inspection conflated original retired state with current restored state: %+v", item)
	}
	if item.Evidence.Claim == nil || item.Evidence.Claim.TransactionTime.IsZero() || item.Evidence.EffectiveValidTime == nil || item.Evidence.EffectiveValidTime.From != nil || item.Evidence.EffectiveValidTime.To != nil {
		t.Fatalf("historical HTTP response invented validity or lost accepted transaction time: %+v", item.Evidence)
	}
	replayed := httptest.NewRecorder()
	server.Handler().ServeHTTP(replayed, managementRequest("/api/context-sessions/history", `{"sessionId":"`+string(reader.ID)+`"}`))
	if replayed.Code != http.StatusOK || !strings.Contains(replayed.Body.String(), `"historicalCount":1`) || !strings.Contains(replayed.Body.String(), `"retiredCount":1`) {
		t.Fatalf("restoration rewrote the original request's activity: %d %s", replayed.Code, replayed.Body.String())
	}
}

func TestMemoryEvidenceHTTPReconstructsOriginalConflictsAndNewerRelationsAfterRestart(t *testing.T) {
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "evie.db")
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	store := eviedb.NewStore(db)
	source, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := agent.New(nil, webTestContextProfile("test"), store.BindHistory(source.ID, "conflict-owner"), source.ScopeContext(), store.BindTurnOwner(source.ID, "conflict-owner"))
	var accepted []memory.SemanticID
	for _, city := range []string{"Boston", "Portland"} {
		proposal, err := owner.PrepareRememberLiteral(ctx, store, "Remember that I live in "+city+".", memory.RememberLiteralRequest{
			Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "live_in", PredicateLabel: "live in", PredicateCardinality: memory.CardinalityOne,
			Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: city}, Polarity: memory.PolarityAffirmed,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, err := owner.ResolveRememberLiteral(ctx, store, proposal, tools.Approved)
		if err != nil {
			t.Fatal(err)
		}
		accepted = append(accepted, result.ClaimID)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	writer := agent.NewWithToolset(&fakeClient{steps: []fakeStep{{content: "Recorded."}}}, webTestContextProfile("test"), store.BindHistory(source.ID, "conflict-writer"), source.ScopeContext(), store.BindTurnOwner(source.ID, "conflict-writer"), tools.NewToolset(definitions))
	writerEvents, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Send(ctx, "I live in Chicago now.", writerEvents, nil); err != nil {
		t.Fatal(err)
	}
	if coverage, err := store.RefreshMemoryIndex(ctx, 256); err != nil || coverage.State != "active" {
		t.Fatalf("index=%+v: %v", coverage, err)
	}
	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "lookup", Type: "function", Function: openrouter.FunctionCall{Name: "memory_search", Arguments: `{"query":"Boston"}`}}}},
		{content: "Conflicting accepted memories and newer owner wording received."},
	}}
	runtime := agent.NewWithToolset(client, webTestContextProfile("test"), store.BindHistory(reader.ID, "conflict-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "conflict-reader"), tools.NewToolset(definitions))
	streamEvents, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		t.Fatal(err)
	}
	if err = runtime.Send(ctx, "Look up the saved residence and its original evidence.", streamEvents, nil); err != nil {
		t.Fatal(err)
	}
	history, err := runtime.HistoryEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotID memory.EventID
	var refs []memory.RetrievalReference
	for _, event := range history {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err = json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil && len(snapshot.Memory.Evidence) == 3 {
			snapshotID, refs = event.ID, snapshot.Memory.Evidence
		}
	}
	if snapshotID == "" {
		t.Fatal("reader turn did not retain the conflicting pair and newer excerpt")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = eviedb.OpenDBAt(path)
	if err != nil {
		t.Fatal(err)
	}
	store = eviedb.NewStore(db)
	runtime = agent.New(nil, webTestContextProfile("test"), store.BindHistory(reader.ID, "reopened-conflict-reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "reopened-conflict-reader"))
	server := NewContextMemoryServer(runtime, nil, nil, nil, store)
	server.activeSession = reader
	body, err := json.Marshal(map[string]string{"sessionId": string(reader.ID), "snapshotId": string(snapshotID)})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("inspection status=%d body=%s", rr.Code, rr.Body.String())
	}
	var inspected struct {
		Evidence []memory.RetrievalInspection `json:"evidence"`
	}
	if err = json.Unmarshal(rr.Body.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if len(inspected.Evidence) != len(refs) {
		t.Fatalf("original receipt lost evidence: %s", rr.Body.String())
	}
	for i, item := range inspected.Evidence {
		if !item.Available || item.Evidence == nil || !reflect.DeepEqual(item.Reference, refs[i]) {
			t.Fatalf("original eligible evidence or reference changed: %+v", item)
		}
		if item.Evidence.Kind == memory.RetrievalAcceptedMemory {
			if len(refs[i].Conflicts) != 1 || !reflect.DeepEqual(item.Evidence.Conflicts, refs[i].Conflicts) {
				t.Fatalf("HTTP inspection lost reconstructed accepted conflict metadata: %+v", item)
			}
		} else if item.Evidence.Text != "I live in Chicago now." || len(item.Evidence.Conflicts) != 0 || !slices.Contains(item.Evidence.RelatedClaimIDs, accepted[0]) || !reflect.DeepEqual(item.Evidence.RelatedClaimIDs, refs[i].RelatedClaimIDs) {
			t.Fatalf("HTTP inspection lost the attributed possible-discrepancy relationship: %+v", item)
		}
	}
	var sourceLink memory.SemanticID
	for _, ref := range refs {
		if ref.ClaimID == accepted[0] && len(ref.Sources) == 1 {
			sourceLink = ref.Sources[0].SourceLinkID
		}
	}
	if sourceLink == "" {
		t.Fatal("accepted Boston receipt lacks its original Source Link")
	}
	args, err := json.Marshal(map[string]string{"idempotency_key": "idem:v1:" + uuid.NewString(), "source_link_id": string(sourceLink)})
	if err != nil {
		t.Fatal(err)
	}
	retractClient := &fakeClient{steps: []fakeStep{
		{toolCalls: []openrouter.ToolCall{{ID: "retract", Type: "function", Function: openrouter.FunctionCall{Name: "memory_retract_source", Arguments: string(args)}}}},
		{content: "Original source retracted."},
	}}
	definitions = nil
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	retractor := agent.NewWithToolset(retractClient, webTestContextProfile("test"), store.BindHistory(source.ID, "source-retractor"), source.ScopeContext(), store.BindTurnOwner(source.ID, "source-retractor"), tools.NewToolset(definitions))
	retractEvents, err := newSSEEvents(httptest.NewRecorder())
	if err != nil {
		t.Fatal(err)
	}
	if err = retractor.Send(ctx, "Retract the original accepted source.", retractEvents, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	server.Handler().ServeHTTP(rr, managementRequest("/api/memory/evidence", string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("restricted inspection status=%d body=%s", rr.Code, rr.Body.String())
	}
	inspected.Evidence = nil
	if err = json.Unmarshal(rr.Body.Bytes(), &inspected); err != nil {
		t.Fatal(err)
	}
	if len(inspected.Evidence) != len(refs) {
		t.Fatal("source revocation removed original request references")
	}
	for i, item := range inspected.Evidence {
		if !reflect.DeepEqual(item.Reference, refs[i]) {
			t.Fatalf("source revocation rewrote original relation metadata: %+v", item.Reference)
		}
		if item.Reference.ClaimID == accepted[0] {
			if item.Available || item.Evidence != nil {
				t.Fatal("retracted Boston source remained available")
			}
			continue
		}
		if !item.Available || item.Evidence == nil || len(item.Evidence.Conflicts) != 0 || slices.Contains(item.Evidence.RelatedClaimIDs, accepted[0]) {
			t.Fatalf("inspection treated an unavailable source as an eligible conflict or relation: %+v", item)
		}
	}
}
