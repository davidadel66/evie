package agent

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemorySearchTurnSuppliesAcceptedEvidenceWithoutMutation(t *testing.T) {
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
	sourceSession := NewWithToolset(nil, testContextProfile("test-model"), store.BindHistory(source.ID, "source"), source.ScopeContext(), store.BindTurnOwner(source.ID, "source"), tools.NewToolset(nil))
	proposal, err := sourceSession.PrepareRememberLiteral(ctx, store, "Remember that I am vegetarian.", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:50000000-0000-4000-8000-000000000001", Predicate: "diet", PredicateLabel: "diet", PredicateCardinality: memory.CardinalityOne,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "vegetarian"}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := sourceSession.ResolveRememberLiteral(ctx, store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := store.RefreshMemoryIndex(ctx, 256)
	if err != nil || coverage.State != "active" {
		t.Fatalf("fixture index coverage = %+v: %v", coverage, err)
	}

	reader, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("lookup-diet", "memory_search", `{"query":"vegetarian"}`)),
		assistantStep("Evidence received.", nil),
	}}
	session := NewWithToolset(client, testContextProfile("test-model"), store.BindHistory(reader.ID, "reader"), reader.ScopeContext(), store.BindTurnOwner(reader.ID, "reader"), tools.NewToolset(definitions))
	if err := session.Send(ctx, "Look up my dietary preference.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(client.reqs) != 2 {
		t.Fatalf("provider requests = %d, want 2", len(client.reqs))
	}
	payload, err := json.Marshal(client.reqs[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), string(accepted.ClaimID)) || !strings.Contains(string(payload), "vegetarian") || !strings.Contains(string(payload), string(proposal.Source.EventID)) {
		t.Fatalf("provider did not receive accepted, source-bearing evidence for Claim %s", accepted.ClaimID)
	}
	after, err := store.InspectClaims(ctx, source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Claims) != len(before.Claims) || after.ScopeRevision != before.ScopeRevision {
		t.Fatalf("search changed accepted state: before=%+v after=%+v", before.ScopeRevisions, after.ScopeRevisions)
	}
}

type retrievalFixture struct {
	t     *testing.T
	path  string
	db    *sql.DB
	store *eviedb.Store
}

func newRetrievalFixture(t *testing.T) *retrievalFixture {
	t.Helper()
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	f := &retrievalFixture{t: t, path: filepath.Join(t.TempDir(), "evie.db")}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	t.Cleanup(func() { f.db.Close() })
	return f
}

func (f *retrievalFixture) global() memory.Session {
	f.t.Helper()
	s, err := f.store.CreateGlobalSession(context.Background())
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func (f *retrievalFixture) session(record memory.Session, client Client, extra ...tools.Tool) *Session {
	f.t.Helper()
	var definitions []tools.Tool
	for _, c := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, c.Tool)
	}
	definitions = append(definitions, extra...)
	holder := memory.LeaseHolderID("retrieval-" + string(record.ID))
	return NewWithToolset(client, testContextProfile("test-model"), f.store.BindHistory(record.ID, holder), record.ScopeContext(), f.store.BindTurnOwner(record.ID, holder), tools.NewToolset(definitions))
}

func (f *retrievalFixture) remember(record memory.Session, destination memory.MemoryDestination, value string) memory.RememberLiteralProposal {
	f.t.Helper()
	s := f.session(record, nil)
	p, err := s.PrepareRememberLiteral(context.Background(), f.store, "Remember my retrieval marker: "+value, memory.RememberLiteralRequest{
		Destination: destination, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "retrieval_marker", PredicateLabel: "retrieval marker", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = s.ResolveRememberLiteral(context.Background(), f.store, p, tools.Approved); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *retrievalFixture) refresh() {
	f.t.Helper()
	for i := 0; i < 100; i++ {
		coverage, err := f.store.RefreshMemoryIndex(context.Background(), 256)
		if err != nil {
			f.t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			return
		}
	}
	f.t.Fatal("fixture backfill did not complete")
}

func (f *retrievalFixture) search(record memory.Session, query string) (*fakeClient, []memory.Event) {
	f.t.Helper()
	args, _ := json.Marshal(map[string]string{"query": query})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("lookup", "memory_search", string(args))), assistantStep("Evidence received.", nil)}}
	if err := f.session(record, client).Send(context.Background(), "Look up my saved evidence.", &recorder{}, nil); err != nil {
		f.t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), record.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	return client, events
}

func retrievalData(t *testing.T, req openrouter.ChatRequest) string {
	t.Helper()
	for _, message := range req.Messages {
		if message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
			return message.Content
		}
	}
	t.Fatal("provider has no EVIE_MEMORY_DATA message")
	return ""
}

func TestMemorySearchCurrentSessionExcludesSiblingSession(t *testing.T) {
	f := newRetrievalFixture(t)
	current, sibling := f.global(), f.global()
	own := f.remember(current, memory.MemorySession, "silver marigold")
	other := f.remember(sibling, memory.MemorySession, "silver forbidden")
	f.refresh()
	client, _ := f.search(current, "silver")
	data := retrievalData(t, client.reqs[1])
	if !strings.Contains(data, string(own.ClaimID)) || strings.Contains(data, string(other.ClaimID)) || strings.Contains(data, "forbidden") {
		t.Fatalf("current session evidence missing or sibling evidence disclosed: %s", data)
	}
}

func (f *retrievalFixture) lifecycle(record memory.Session, toolName string, kind memory.SemanticObjectKind, id memory.SemanticID) {
	f.t.Helper()
	args := map[string]string{"idempotency_key": "idem:v1:" + uuid.NewString(), "object_kind": string(kind), "object_id": string(id)}
	if kind == memory.SemanticObjectSourceLink {
		delete(args, "object_kind")
		delete(args, "object_id")
		args["source_link_id"] = string(id)
	}
	encoded, _ := json.Marshal(args)
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("change-memory", toolName, string(encoded))), assistantStep("Change recorded.", nil)}}
	if err := f.session(record, client).Send(context.Background(), "Apply the requested memory lifecycle change.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		f.t.Fatal(err)
	}
	for _, m := range client.reqs[1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "Error:") {
			f.t.Fatalf("lifecycle failed: %s", m.Content)
		}
	}
}

func TestMemorySearchRetirementAndRestorationRecheckStaleIndex(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	p := f.remember(source, memory.MemoryEverywhere, "saffron teapot")
	f.refresh()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, p.ClaimID)
	client, _ := f.search(reader, "saffron")
	if data := retrievalData(t, client.reqs[len(client.reqs)-1]); strings.Contains(data, "saffron") || strings.Contains(data, string(p.ClaimID)) {
		t.Fatalf("stale index revived retired evidence: %s", data)
	}
	f.lifecycle(source, "memory_restore", memory.SemanticObjectClaim, p.ClaimID)
	f.refresh()
	client, _ = f.search(f.global(), "saffron")
	if data := retrievalData(t, client.reqs[1]); !strings.Contains(data, string(p.ClaimID)) {
		t.Fatalf("restored evidence absent: %s", data)
	}
}

func TestMemorySearchWithholdsSecretBearingEvidence(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	safe := f.remember(source, memory.MemoryEverywhere, "violet garden")
	secret := f.remember(source, memory.MemoryEverywhere, "violet password=verylongsecret")
	f.refresh()
	client, events := f.search(reader, "violet")
	data := retrievalData(t, client.reqs[1])
	if !strings.Contains(data, string(safe.ClaimID)) || strings.Contains(data, string(secret.ClaimID)) || strings.Contains(data, "verylongsecret") {
		t.Fatalf("secret evidence was not excluded: %s", data)
	}
	for _, event := range events {
		if strings.Contains(event.Content, "violet garden") || strings.Contains(string(event.Payload), "violet garden") {
			t.Fatalf("synthetic evidence became an episode: %s", event.ID)
		}
	}
}

func TestMemorySearchOptOutBeforeDispatchWithholdsSupplementalSources(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	p := f.remember(source, memory.MemoryEverywhere, "zinnia keepsake")
	f.refresh()
	client := &fakeClient{steps: []step{
		assistantStep("", nil, toolCall("lookup", "memory_search", `{"query":"zinnia"}`), toolCall("disable", "disable_remote_memory", `{}`)),
		assistantStep("Memory is unavailable.", nil),
	}}
	disable := tools.Tool{Schema: openrouter.Tool{Type: "function", Function: openrouter.Function{Name: "disable_remote_memory", Parameters: openrouter.Parameter{Type: "object"}}}, Execute: func(context.Context, string) (string, error) {
		t.Setenv("EVIE_REMOTE_MEMORY", "off")
		return "disabled", nil
	}}
	if err := f.session(reader, client, disable).Send(context.Background(), "Look up saved evidence.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(client.reqs[1])
	if strings.Contains(string(encoded), string(p.ClaimID)) || strings.Contains(string(encoded), string(p.Source.EventID)) || strings.Contains(string(encoded), "zinnia keepsake") {
		t.Fatal("opt-out before dispatch leaked supplemental evidence or its source references")
	}
}

func TestMemorySearchReceiptSurvivesRestartAndRechecksSourceAccess(t *testing.T) {
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	p := f.remember(source, memory.MemoryEverywhere, "cobalt archive")
	f.refresh()
	_, events := f.search(reader, "cobalt")
	var snapshotID memory.EventID
	for _, event := range events {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Memory != nil && len(snapshot.Memory.Evidence) > 0 {
			snapshotID = event.ID
		}
	}
	if snapshotID == "" {
		t.Fatal("answer has no durable evidence receipt")
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	events, err = f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipt *memory.RetrievalReceipt
	for _, event := range events {
		if event.ID == snapshotID {
			var snapshot memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				t.Fatal(err)
			}
			receipt = snapshot.Memory
		}
	}
	if receipt == nil || len(receipt.Evidence) != 1 || receipt.Evidence[0].ClaimID != p.ClaimID {
		t.Fatalf("original receipt changed after restart: %+v", receipt)
	}
	inspected, err := f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), receipt.Evidence)
	if err != nil || len(inspected) != 1 || !inspected[0].Available || inspected[0].Evidence.Sources[0].Evidence != p.Source.Evidence {
		t.Fatalf("original source unavailable after restart: %+v %v", inspected, err)
	}
	f.lifecycle(source, "memory_retract_source", memory.SemanticObjectSourceLink, p.SourceLinkID)
	inspected, err = f.store.InspectMemoryEvidence(context.Background(), reader.ScopeContext(), receipt.Evidence)
	if err != nil || len(inspected) != 1 || inspected[0].Available || inspected[0].Evidence != nil {
		t.Fatalf("old receipt bypassed source retraction: %+v %v", inspected, err)
	}
}

func TestMemorySearchOutcomesDistinguishIncompleteEmptyAndMalformed(t *testing.T) {
	f := newRetrievalFixture(t)
	reader := f.global()
	client, _ := f.search(reader, "missing")
	if data := retrievalData(t, client.reqs[1]); !strings.Contains(data, `"status":"unavailable"`) {
		t.Fatalf("incomplete index was represented as absence: %s", data)
	}
	f.refresh()
	client, _ = f.search(reader, "missing")
	if data := retrievalData(t, client.reqs[1]); !strings.Contains(data, `"status":"empty"`) {
		t.Fatalf("successful empty search was not distinct: %s", data)
	}
	client, _ = f.search(reader, "***")
	if data := retrievalData(t, client.reqs[1]); !strings.Contains(data, `"status":"failed"`) {
		t.Fatalf("malformed query was represented as absence: %s", data)
	}
}

func TestMemorySearchRepeatedCallsShareTurnBudget(t *testing.T) {
	f := newRetrievalFixture(t)
	f.refresh()
	var calls []openrouter.ToolCall
	for i := 0; i < 9; i++ {
		calls = append(calls, toolCall(uuid.NewString(), "memory_search", `{"query":"missing"}`))
	}
	client := &fakeClient{steps: []step{assistantStep("", nil, calls...), assistantStep("No supported findings; the search budget is exhausted.", nil)}}
	if err := f.session(f.global(), client).Send(context.Background(), "Investigate saved evidence.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if data := retrievalData(t, client.reqs[1]); !strings.Contains(data, `"status":"exhausted"`) {
		t.Fatalf("repeated searches escaped the turn budget: %s", data)
	}
}
