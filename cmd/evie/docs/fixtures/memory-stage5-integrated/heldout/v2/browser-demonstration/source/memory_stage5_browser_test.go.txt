package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
	"github.com/google/uuid"
)

// TestStage5BrowserFixture serves disposable real Kernel history to a browser.
// Answers are scripted demonstration text, never reader-quality measurements.
// Ordinary verification skips it. SIGUSR1 closes a completed browser visit;
// SIGINT/SIGTERM or the one-hour limit stop an unfinished visit without claiming
// its manual checks passed. Artifacts remain in the explicitly new output path.
func TestStage5BrowserFixture(t *testing.T) {
	if os.Getenv("EVIE_STAGE5_BROWSER_FIXTURE") != "1" {
		t.Skip("opt-in disposable browser demonstration")
	}
	output := os.Getenv("EVIE_STAGE5_BROWSER_OUTPUT")
	if !filepath.IsAbs(output) {
		t.Fatal("EVIE_STAGE5_BROWSER_OUTPUT must be an absolute NEW directory")
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	t.Setenv("EVIE_REASONING", "")
	f := &stage5BrowserFixture{t: t, ctx: context.Background(), path: filepath.Join(output, "browser.db"), output: output}
	var err error
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.db.Close() })
	f.store = eviedb.NewStore(f.db)
	composition, err := sessionCompositionManager(t).ResolvePreset("")
	if err != nil {
		t.Fatal(err)
	}
	labels := []string{"01 Fresh-chat preference", "02 Uncompiled original", "03 Cross-topic reference", "04 Bounded investigation", "05 Historical conflict", "06 Retirement suppression", "07 Original sources after restart", "08 Memory unavailable"}
	for _, label := range labels {
		workspace, err := f.store.RegisterWorkspace(f.ctx, label)
		if err != nil {
			t.Fatal(err)
		}
		source, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := f.store.CreateWorkspaceSessionWithComposition(f.ctx, workspace.ID, workspace.CurrentRevisionID, composition.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		f.cases = append(f.cases, stage5BrowserCase{Name: label, Workspace: workspace, Source: source, Reader: reader})
	}

	// One ordinary first request is deliberately left for the browser to send.
	fresh := &f.cases[0]
	diet := f.remember(fresh.Source, "diet", "diet", "vegetarian", memory.ValidTime{}, memory.CardinalityOne)
	fresh.Question = "Suggest dinner matching my saved diet."
	fresh.Description = "Send the exact question in the composer. The first request must contain the accepted dietary Claim."
	fresh.ExpectedClaimIDs = []memory.SemanticID{diet.ClaimID}
	f.freshAnswer = "Scripted demonstration: vegetarian bean chili fits the saved diet. Owner source event " + string(diet.Source.EventID) + "."

	original := &f.cases[1]
	fern := f.send(original.Source, "My fern perked up after I moved it into shade; I have not tested the cause.", false, stage5BrowserText("Scripted source acknowledgement."))
	f.refresh()
	original.Question = "What did I observe after moving the fern into shade?"
	f.send(original.Reader, original.Question, true, stage5BrowserCheckedText("Scripted demonstration: the owner reported the fern perked up after moving into shade; this is an observation, not a diagnosis. Owner source event "+string(fern.Root)+".", string(fern.Root)))
	original.Description = "Inspect the attributed Conversation excerpt, exact original wording and event locator; no accepted Claim was created."

	reference := &f.cases[2]
	runtime := f.runtime(reference.Source, nil, false)
	gift, err := runtime.PrepareRememberEntity(f.ctx, f.store, "Maya prefers jasmine tea as a gift.", memory.RememberEntityRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "gift", PredicateLabel: "gift", Polarity: memory.PolarityAffirmed, Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "Maya"}, Object: memory.EntitySelector{Create: true, CanonicalName: "jasmine tea", EntityType: "food", Alias: "jasmine tea"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResolveRememberEntity(f.ctx, f.store, gift, tools.Approved); err != nil {
		t.Fatal(err)
	}
	f.send(reference.Reader, "Maya, my mother, has a birthday approaching.", false, stage5BrowserText("Scripted discussion: the gift recipient is Maya."))
	f.send(reference.Reader, "Separate topic: the checksum parser still crashes.", false, stage5BrowserText("Scripted discussion: inspect the parser input."))
	f.refresh()
	reference.Question = "Back to the birthday: what present would she like?"
	f.send(reference.Reader, reference.Question, true, stage5BrowserCheckedText("Scripted demonstration: Maya is the birthday recipient; jasmine tea matches her accepted preference. Owner source event "+string(gift.Source.EventID)+".", string(gift.Claim.ID)))
	reference.Description = "The original discussion identifies Maya despite the parser detour; inspect her accepted preference, not the detour."

	expansion := &f.cases[3]
	f.send(expansion.Source, "My mother is Maya; she mentioned a café exhibit.", false, stage5BrowserText("Scripted source acknowledgement."))
	anchor := f.send(expansion.Source, "She might visit the orchidgallery next month. Nothing is booked.", false, stage5BrowserText("Scripted source acknowledgement."))
	f.refresh()
	expansion.Question = "Inspect the tentative orchidgallery discussion and its neighboring context."
	expand := stage5BrowserTool("memory_expand_conversation", map[string]any{"evidence_id": fmt.Sprintf("excerpt:%s:0:%d", anchor.Root, len("She might visit the orchidgallery next month. Nothing is booked.")), "before": 2, "after": 0})
	f.send(expansion.Reader, expansion.Question, false, stage5BrowserTool("memory_search_conversations", map[string]any{"query": "orchidgallery"}), expand, stage5BrowserCheckedText("Scripted demonstration: the preceding original identifies Maya. The possible visit remains tentative and unbooked. Inspect the additional original positions on the third request.", "My mother is Maya", "Nothing is booked"))
	expansion.Description = "Compare the initial excerpt with bounded neighboring originals in successive request tabs."

	history := &f.cases[4]
	f.remember(history.Source, "live_in", "live in", "Boston", memory.ValidTime{}, memory.CardinalityOne)
	f.remember(history.Source, "live_in", "live in", "Portland", memory.ValidTime{}, memory.CardinalityOne)
	f.send(history.Source, "I live in Chicago now.", false, stage5BrowserText("Scripted source acknowledgement; no correction accepted."))
	from, to := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	oldOffice := f.remember(history.Source, "workplace", "workplace", "Willow annex", memory.ValidTime{From: &from, To: &to}, memory.CardinalityOne)
	f.lifecycle(history.Source, "memory_retire", map[string]any{"object_kind": "claim", "object_id": oldOffice.ClaimID})
	f.refresh()
	history.Question = "Inspect the conflicting residence evidence and the historical workplace on June 15, 2022."
	f.send(history.Reader, history.Question, false, stage5BrowserTool("memory_search", map[string]any{"query": "Boston"}), stage5BrowserTool("memory_search", map[string]any{"query": "workplace", "intent": "historical", "valid_at": "2022-06-15T00:00:00Z"}), stage5BrowserCheckedText("Scripted demonstration: Boston and Portland are conflicting accepted entries; Chicago is newer owner wording, not an accepted correction. The historical workplace was Willow annex within [2022-01-01, 2023-01-01), now retired. Inspect exact sources and lifecycle labels.", "Boston", "Portland", "Chicago", "Willow annex", `"current_status":"retired"`))
	history.Description = "Inspect supported conflicting Claims, newer owner discrepancy, and explicitly historical retired evidence with its validity interval."

	retirement := &f.cases[5]
	parcel := f.remember(retirement.Source, "parcel_delivery", "parcel delivery", "leave at the back door", memory.ValidTime{}, memory.CardinalityOne)
	f.send(retirement.Source, "The parcel tracking notebook is blue.", false, stage5BrowserText("Scripted source acknowledgement."))
	f.refresh()
	f.send(retirement.Reader, "Before retirement: inspect my parcel delivery instruction.", false, stage5BrowserTool("memory_search", map[string]any{"query": "parcel delivery"}), stage5BrowserCheckedText("Scripted demonstration before retirement: the accepted instruction says leave at the back door.", string(parcel.ClaimID)))
	beforeReader := retirement.Reader
	retirement.OtherSessions = append(retirement.OtherSessions, beforeReader)
	f.lifecycle(retirement.Source, "memory_retire", map[string]any{"object_kind": "claim", "object_id": parcel.ClaimID})
	retirement.Reader, err = f.store.CreateWorkspaceSessionWithComposition(f.ctx, retirement.Workspace.ID, retirement.Workspace.CurrentRevisionID, composition.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	retirement.Question = "After retirement: search parcel evidence in a fresh conversation."
	step := stage5BrowserCheckedText("Scripted demonstration after retirement: the unrelated blue tracking-notebook statement remains available. The retired delivery instruction is not supplied as ordinary memory.", "tracking notebook is blue")
	step.ForbiddenMemory = []string{"leave at the back door", string(parcel.ClaimID)}
	f.send(retirement.Reader, retirement.Question, true, step)
	retirement.Description = "Compare the before-retirement session with this fresh ordinary request: retired source excluded, unrelated statement retained."

	receipt := &f.cases[6]
	old := f.remember(receipt.Source, "retrieval_marker", "retrieval marker", "azurefolio", memory.ValidTime{}, memory.CardinalityMany)
	restricted := f.remember(receipt.Source, "retrieval_marker", "retrieval marker", "embermanifest", memory.ValidTime{}, memory.CardinalityMany)
	f.refresh()
	receipt.Question = "Original answer: inspect both retrieval markers before later changes."
	f.send(receipt.Reader, receipt.Question, false, stage5BrowserTool("memory_search", map[string]any{"query": "retrieval marker"}), stage5BrowserCheckedText("Scripted original answer: azurefolio and embermanifest were supplied. The original source references stay attached to this answer after later changes.", string(old.ClaimID), string(restricted.ClaimID)))
	owner := f.runtime(receipt.Source, nil, false)
	correction, err := owner.PrepareCorrectClaim(f.ctx, f.store, "Correct the earlier marker to jadefolio; it was an error.", memory.CorrectClaimRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), OldClaimID: old.ClaimID, Mode: memory.CorrectionError, Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID, Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: "jadefolio"}}, Polarity: memory.PolarityAffirmed}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ResolveCorrectClaim(f.ctx, f.store, correction, tools.Approved); err != nil {
		t.Fatal(err)
	}
	f.lifecycle(receipt.Source, "memory_retract_source", map[string]any{"source_link_id": restricted.SourceLinkID})
	receipt.ExpectedClaimIDs = []memory.SemanticID{old.ClaimID, restricted.ClaimID}
	receipt.Description = "SQLite was closed and reopened after public correction and source retraction. Inspect the original azurefolio version with current superseded status; embermanifest source must be unavailable. The old answer text itself remains history."

	unavailable := &f.cases[7]
	t.Setenv("EVIE_REMOTE_MEMORY", "off")
	f.send(unavailable.Reader, "Rewrite this sentence: The meeting is going to begin after lunch.", true, stage5BrowserText("Scripted demonstration: The meeting will begin after lunch."))
	unavailable.Question = "What is my saved dietary preference?"
	f.send(unavailable.Reader, unavailable.Question, true, stage5BrowserText("Scripted demonstration: supplemental memory is unavailable under the configured opt-out, so I cannot supply a stored dietary preference here. This is not proof that no preference exists."))
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	unavailable.Description = "These two persisted turns ran with EVIE_REMOTE_MEMORY=off: self-contained rewriting continued, while the personal-memory answer reported an access limitation, not an empty search. The server now only inspects that recorded state."
	f.refresh()

	// Close/reopen is part of this actual disposable demonstration setup.
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	for i := range f.cases {
		f.collect(&f.cases[i])
	}
	if len(unavailable.MemoryStatuses) != 2 || unavailable.MemoryStatuses[0] != memory.RetrievalUnavailable || unavailable.MemoryStatuses[1] != memory.RetrievalUnavailable || len(unavailable.References) != 0 {
		t.Fatalf("opt-out demonstration receipts: statuses=%v references=%d", unavailable.MemoryStatuses, len(unavailable.References))
	}
	f.verifyOriginalInspection(old.ClaimID, restricted.ClaimID)
	controller := &stage5BrowserController{fixture: f}
	server := web.NewContextMemoryServer(nil, nil, nil, controller, f.store)
	handler := server.Handler()
	selected := httptest.NewRecorder()
	body, _ := json.Marshal(map[string]any{"sessionId": fresh.Reader.ID})
	request := httptest.NewRequest(http.MethodPost, "http://localhost/api/context-sessions/select", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(selected, request)
	if selected.Code != http.StatusOK {
		t.Fatalf("initial browser selection: %d %s", selected.Code, selected.Body.String())
	}
	host := httptest.NewServer(handler)
	defer host.Close()
	metadata := map[string]any{"version": "memory-stage5-browser-v1", "scripted_provider": true, "reader_quality_measurement": false, "database_reopened": true, "url": host.URL, "database": f.path, "output": output, "cases": f.cases}
	f.write("ready.json", metadata)
	raw, _ := json.Marshal(metadata)
	fmt.Println("STAGE5_BROWSER_READY=" + string(raw))
	if os.Getenv("EVIE_STAGE5_BROWSER_VALIDATE_ONLY") == "1" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGUSR1)
	defer signal.Stop(signals)
	end := "time_limit"
	select {
	case sig := <-signals:
		end = sig.String()
	case <-time.After(time.Hour):
	}
	host.Close()
	f.collect(fresh)
	f.write("closed.json", map[string]any{"reason": end, "manual_checks_attested_by_fixture": false, "fresh_scripted_response_returned": f.freshCompleted, "fresh_browser_answer_persisted": len(fresh.AnswerIDs) == 1 && len(fresh.SnapshotIDs) == 1, "cases": f.cases})
}

type stage5BrowserCase struct {
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Question         string                      `json:"question"`
	Workspace        memory.Workspace            `json:"workspace"`
	Source           memory.Session              `json:"source_session"`
	Reader           memory.Session              `json:"reader_session"`
	OtherSessions    []memory.Session            `json:"other_sessions,omitempty"`
	AnswerIDs        []memory.EventID            `json:"answer_ids"`
	SnapshotIDs      []memory.EventID            `json:"snapshot_ids"`
	MemoryStatuses   []string                    `json:"memory_statuses"`
	ExpectedClaimIDs []memory.SemanticID         `json:"expected_claim_ids,omitempty"`
	References       []memory.RetrievalReference `json:"original_references,omitempty"`
}

type stage5BrowserFixture struct {
	t              *testing.T
	ctx            context.Context
	db             *sql.DB
	store          *eviedb.Store
	path, output   string
	cases          []stage5BrowserCase
	freshAnswer    string
	freshCompleted bool
	mu             sync.Mutex
	requestNumber  int
}

type stage5BrowserStep struct {
	Content         string
	Tools           []openrouter.ToolCall
	RequiredMemory  []string
	ForbiddenMemory []string
}

func stage5BrowserText(text string) stage5BrowserStep { return stage5BrowserStep{Content: text} }
func stage5BrowserCheckedText(text string, required ...string) stage5BrowserStep {
	return stage5BrowserStep{Content: text, RequiredMemory: required}
}
func stage5BrowserTool(name string, args map[string]any) stage5BrowserStep {
	raw, _ := json.Marshal(args)
	return stage5BrowserStep{Tools: []openrouter.ToolCall{{ID: "browser-" + uuid.NewString(), Type: "function", Function: openrouter.FunctionCall{Name: name, Arguments: string(raw)}}}}
}

type stage5BrowserClient struct {
	fixture *stage5BrowserFixture
	steps   []stage5BrowserStep
	fresh   bool
}

func (c *stage5BrowserClient) ChatStream(ctx context.Context, req openrouter.ChatRequest, h openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return openrouter.ChatResponse{}, err
	}
	c.fixture.mu.Lock()
	defer c.fixture.mu.Unlock()
	if len(c.steps) == 0 {
		return openrouter.ChatResponse{}, errors.New("scripted browser fixture has no response for this request; no model is configured")
	}
	if c.fresh {
		if c.fixture.freshCompleted || len(req.Messages) == 0 || req.Messages[len(req.Messages)-1].Content != c.fixture.cases[0].Question {
			return openrouter.ChatResponse{}, errors.New("send only the fixture's exact fresh-chat demonstration question once")
		}
	}
	step := c.steps[0]
	c.steps = c.steps[1:]
	var projection string
	for _, message := range req.Messages {
		if strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
			projection = message.Content
		}
	}
	for _, required := range step.RequiredMemory {
		if !strings.Contains(projection, required) {
			return openrouter.ChatResponse{}, fmt.Errorf("scripted fixture required evidence missing: %s", required)
		}
	}
	for _, forbidden := range step.ForbiddenMemory {
		if strings.Contains(projection, forbidden) {
			return openrouter.ChatResponse{}, errors.New("scripted fixture received forbidden retired evidence")
		}
	}
	wire, err := openrouter.RequestBytes(req)
	if err != nil {
		return openrouter.ChatResponse{}, err
	}
	c.fixture.requestNumber++
	c.fixture.write(fmt.Sprintf("request-%03d.json", c.fixture.requestNumber), map[string]any{"request": req, "sha256": fmt.Sprintf("%x", sha256.Sum256(wire)), "serialized_bytes": len(wire), "scripted_provider": true})
	if h.OnContent != nil && step.Content != "" {
		h.OnContent(step.Content)
	}
	if c.fresh {
		c.fixture.freshCompleted = true
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: step.Content, ToolCalls: step.Tools}}}}, nil
}

func (f *stage5BrowserFixture) runtime(record memory.Session, client agent.Client, automatic bool) *agent.Session {
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(f.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("stage5-browser-" + string(record.ID))
	return agent.NewWithToolset(client, evieTestContextProfile("stage5-scripted-browser"), f.store.BindHistory(record.ID, holder), record.ScopeContext(), f.store.BindTurnOwner(record.ID, holder), tools.NewToolset(definitions), agent.WithAutomaticMemoryRecall(automatic))
}

func (f *stage5BrowserFixture) send(record memory.Session, question string, automatic bool, steps ...stage5BrowserStep) struct{ Root memory.EventID } {
	f.t.Helper()
	client := &stage5BrowserClient{fixture: f, steps: steps}
	if err := f.runtime(record, client, automatic).Send(f.ctx, question, clockCLIEvents{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }); err != nil {
		f.t.Fatalf("scripted browser turn %q: %v", question, err)
	}
	if len(client.steps) != 0 {
		f.t.Fatal("scripted browser turn omitted planned provider steps")
	}
	events, err := f.store.LoadEvents(f.ctx, record.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	var root memory.EventID
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			root = event.ID
		}
	}
	return struct{ Root memory.EventID }{root}
}

func (f *stage5BrowserFixture) remember(record memory.Session, predicate, label, value string, valid memory.ValidTime, cardinality memory.PredicateCardinality) memory.RememberLiteralProposal {
	f.t.Helper()
	session := f.runtime(record, nil, false)
	proposal, err := session.PrepareRememberLiteral(f.ctx, f.store, "Remember my "+label+": "+value+".", memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: predicate, PredicateLabel: label, PredicateCardinality: cardinality, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed, ValidTime: valid})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := session.ResolveRememberLiteral(f.ctx, f.store, proposal, tools.Approved); err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

func (f *stage5BrowserFixture) lifecycle(record memory.Session, name string, args map[string]any) {
	f.t.Helper()
	args["idempotency_key"] = "idem:v1:" + uuid.NewString()
	f.send(record, "Apply this explicitly approved demonstration lifecycle change.", false, stage5BrowserTool(name, args), stage5BrowserText("Scripted demonstration lifecycle operation returned; authoritative state is checked separately."))
	var id memory.SemanticID
	kind := memory.SemanticObjectClaim
	expected := memory.SemanticStatusRetired
	if name == "memory_retract_source" {
		id = args["source_link_id"].(memory.SemanticID)
		kind, expected = memory.SemanticObjectSourceLink, memory.SemanticStatusSourceRetracted
	} else {
		id = args["object_id"].(memory.SemanticID)
	}
	object, err := f.store.InspectSemanticObject(f.ctx, record.ScopeContext(), kind, id)
	if err != nil || object.Status != expected {
		f.t.Fatalf("browser lifecycle state=%s expected=%s: %v", object.Status, expected, err)
	}
}

func (f *stage5BrowserFixture) refresh() {
	f.t.Helper()
	for i := 0; i < 100; i++ {
		coverage, err := f.store.RefreshMemoryIndex(f.ctx, 256)
		if err != nil {
			f.t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			return
		}
	}
	f.t.Fatal("disposable browser index did not complete within bounded batches")
}

func (f *stage5BrowserFixture) collect(item *stage5BrowserCase) {
	f.t.Helper()
	events, err := f.store.LoadEvents(f.ctx, item.Reader.ID)
	if err != nil {
		f.t.Fatal(err)
	}
	item.AnswerIDs, item.SnapshotIDs, item.References, item.MemoryStatuses = nil, nil, nil, nil
	for _, event := range events {
		if event.Type == memory.EventAssistantMessage && event.Content != "" {
			item.AnswerIDs = append(item.AnswerIDs, event.ID)
		}
		if event.Type == memory.EventContextSnapshot {
			var payload memory.ContextSnapshotPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				f.t.Fatal(err)
			}
			item.SnapshotIDs = append(item.SnapshotIDs, event.ID)
			if payload.Memory != nil {
				item.References = payload.Memory.Evidence
				item.MemoryStatuses = append(item.MemoryStatuses, payload.Memory.Status)
			}
		}
	}
}

func (f *stage5BrowserFixture) verifyOriginalInspection(corrected, restricted memory.SemanticID) {
	f.t.Helper()
	item := f.cases[6]
	evidence, err := f.store.InspectMemoryEvidence(f.ctx, item.Reader.ScopeContext(), item.References)
	if err != nil {
		f.t.Fatal(err)
	}
	var oldOK, restrictionOK bool
	for _, inspected := range evidence {
		if inspected.Reference.ClaimID == corrected {
			oldOK = inspected.Available && inspected.Evidence != nil && inspected.CurrentStatus == memory.SemanticStatusSuperseded && strings.Contains(inspected.Evidence.Text, "azurefolio") && !strings.Contains(inspected.Evidence.Text, "jadefolio")
		}
		if inspected.Reference.ClaimID == restricted {
			restrictionOK = !inspected.Available && inspected.Evidence == nil
		}
	}
	if !oldOK || !restrictionOK {
		f.t.Fatalf("reopened original inspection: corrected=%v restricted=%v", oldOK, restrictionOK)
	}
}

func (f *stage5BrowserFixture) write(name string, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(f.output, name), append(raw, '\n'), 0o600)
	}
	if err != nil {
		f.t.Fatal(err)
	}
}

type stage5BrowserController struct{ fixture *stage5BrowserFixture }

func (c *stage5BrowserController) Snapshot(ctx context.Context) (web.ContextSessionSnapshot, error) {
	workspaces, err := c.fixture.store.ListWorkspaces(ctx, false)
	if err != nil {
		return web.ContextSessionSnapshot{}, err
	}
	sessions, err := c.fixture.store.ListActiveSessions(ctx)
	return web.ContextSessionSnapshot{Workspaces: workspaces, Projects: []memory.Project{}, Sessions: sessions}, err
}
func (*stage5BrowserController) RegisterWorkspace(context.Context, string) (memory.Workspace, error) {
	return memory.Workspace{}, errors.New("the scripted fixture contains only its eight prepared Workspaces")
}
func (c *stage5BrowserController) SelectSession(ctx context.Context, selection web.ContextSessionSelection) (web.OpenedContextSession, error) {
	if selection.SessionID == "" {
		return web.OpenedContextSession{}, errors.New("select an existing labeled demonstration session")
	}
	record, err := c.fixture.store.GetActiveSession(ctx, selection.SessionID)
	if err != nil {
		return web.OpenedContextSession{}, err
	}
	client := &stage5BrowserClient{fixture: c.fixture}
	if record.ID == c.fixture.cases[0].Reader.ID {
		client.fresh = true
		client.steps = []stage5BrowserStep{stage5BrowserCheckedText(c.fixture.freshAnswer, string(c.fixture.cases[0].ExpectedClaimIDs[0]))}
	}
	return web.OpenedContextSession{Session: record, Agent: c.fixture.runtime(record, client, true)}, nil
}
