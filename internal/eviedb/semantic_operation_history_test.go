package eviedb

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

func openOperationHistoryStore(t *testing.T) (context.Context, *Store, memory.Session, memory.Session) {
	t.Helper()
	ctx := context.Background()
	db, err := OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	store := NewStore(db)
	global, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := store.RegisterWorkspace(ctx, "Garden planning")
	if err != nil {
		t.Fatal(err)
	}
	reader, err := store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	return ctx, store, global, reader
}

// ownerTurn appends one owner message under a fresh turn lease and runs fn
// with that lease and message.
func ownerTurn(t *testing.T, ctx context.Context, store *Store, session memory.Session, content, key string, fn func(memory.TurnLease, memory.Event)) {
	t.Helper()
	lease, err := store.AcquireTurnLease(ctx, session.ID, memory.LeaseHolderID("history-"+key), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	fn(lease, event)
	if err := store.ReleaseTurnLease(ctx, lease.SessionID, lease.HolderID, lease.FencingToken); err != nil {
		t.Fatal(err)
	}
}

func rememberLiteralAt(t *testing.T, ctx context.Context, store *Store, session memory.Session, content, predicate string, value memory.TypedLiteral, key string) memory.RememberLiteralProposal {
	t.Helper()
	var proposal memory.RememberLiteralProposal
	ownerTurn(t, ctx, store, session, content, key, func(lease memory.TurnLease, event memory.Event) {
		var err error
		proposal, err = store.PrepareRememberLiteral(ctx, session.ScopeContext(), memory.RememberLiteralRequest{
			IdempotencyKey: "idem:v1:" + key, SourceEventID: event.ID, Predicate: predicate, PredicateLabel: strings.ReplaceAll(predicate, "_", " "),
			Literal: value,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyRememberLiteral(ctx, lease, proposal); err != nil {
			t.Fatal(err)
		}
	})
	return proposal
}

func applyLifecycleAt(t *testing.T, ctx context.Context, store *Store, session memory.Session, content string, action memory.MemoryLifecycleAction, kind memory.SemanticObjectKind, id memory.SemanticID, key string) {
	t.Helper()
	ownerTurn(t, ctx, store, session, content, key, func(lease memory.TurnLease, event memory.Event) {
		proposal, err := store.PrepareMemoryLifecycle(ctx, session.ScopeContext(), memory.MemoryLifecycleRequest{
			IdempotencyKey: "idem:v1:" + key, SourceEventID: event.ID, Action: action, ObjectKind: kind, ObjectID: id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyMemoryLifecycle(ctx, lease, proposal); err != nil {
			t.Fatal(err)
		}
	})
}

// Harness review final pass (M5, finding 3): operation history is a rendering
// of owner text too. A Workspace reader inspecting a Global memory sees only
// bound spans and value sentences of Global messages, never the rest of them.
func TestOperationHistoryOutsideItsScopeShowsOnlyValueSentences(t *testing.T) {
	ctx, store, global, reader := openOperationHistoryStore(t)
	command := "My HIV test was negative. Remember that my favorite color is teal. The kids are sick."
	sentence := "Remember that my favorite color is teal."
	legacy := rememberLegacyWhole(t, ctx, store, global, command, "favorite_color", "teal", "71500000-0000-4000-8000-000000000001")
	applyLifecycleAt(t, ctx, store, global, "My blood test came back fine. Please forget my favorite color.", memory.LifecycleRetire,
		memory.SemanticObjectClaim, legacy.ClaimID, "71500000-0000-4000-8000-000000000002")
	applyLifecycleAt(t, ctx, store, global, "The kids are sick again. Actually my favorite color is teal, restore it.", memory.LifecycleRestore,
		memory.SemanticObjectClaim, legacy.ClaimID, "71500000-0000-4000-8000-000000000003")
	spanned := rememberLiteralAt(t, ctx, store, global, "My therapist moved my appointment. Remember that my locker code word is pelican.",
		"locker_code_word", text("pelican"), "71500000-0000-4000-8000-000000000004")

	private := []string{"HIV", "blood test", "kids are sick", "therapist"}
	for _, target := range []struct {
		kind memory.SemanticObjectKind
		id   memory.SemanticID
	}{
		{memory.SemanticObjectClaim, legacy.ClaimID}, {memory.SemanticObjectSourceLink, legacy.SourceLinkID},
		{memory.SemanticObjectClaim, spanned.ClaimID}, {memory.SemanticObjectSourceLink, spanned.SourceLinkID},
	} {
		inspected, err := store.InspectSemanticObject(ctx, reader.ScopeContext(), target.kind, target.id)
		if err != nil {
			t.Fatal(err)
		}
		if len(inspected.Operations) == 0 {
			t.Fatalf("%s inspection has no operation history", target.kind)
		}
		encoded, _ := json.Marshal(inspected)
		for _, word := range private {
			if strings.Contains(string(encoded), word) {
				t.Fatalf("Workspace reader saw Global message text %q in %s inspection: %s", word, target.kind, encoded)
			}
		}
	}
	inspected, err := store.InspectSemanticObject(ctx, reader.ScopeContext(), memory.SemanticObjectClaim, legacy.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"remember_literal_claim": `"evidence":"` + sentence + `"`,
		"restore_memory":         `"evidence":"Actually my favorite color is teal, restore it."`,
		"retire_memory":          `"evidence":""`,
	}
	for _, operation := range inspected.Operations {
		if want[operation.Kind] != "" && !strings.Contains(operation.PreparedJSON, want[operation.Kind]) {
			t.Fatalf("%s evidence was not narrowed to its value sentence, or omitted without one: %s", operation.Kind, operation.PreparedJSON)
		}
		delete(want, operation.Kind)
	}
	if len(want) != 0 {
		t.Fatalf("operation history lacks %v", want)
	}

	// Entities have no Sources of their own; their creating operation still
	// quotes the message.
	anchor, err := store.InspectSemanticObject(ctx, reader.ScopeContext(), memory.SemanticObjectEntity, legacy.Subject.ID)
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(anchor); strings.Contains(string(encoded), "HIV") || strings.Contains(string(encoded), "kids") {
		t.Fatalf("Workspace reader saw Global message text through the owner anchor's history: %s", encoded)
	}

	// A Global Entity created from one Workspace never shows that
	// Workspace's words to another Workspace.
	first, err := store.RegisterWorkspace(ctx, "Clinic notes")
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.CreateWorkspaceSessionWithComposition(ctx, first.ID, first.CurrentRevisionID, standardReceipt(t))
	if err != nil {
		t.Fatal(err)
	}
	var created memory.RememberEntityProposal
	ownerTurn(t, ctx, store, writer, "My psychiatrist upped the dose. Remember that Sarah plays tennis.", "71500000-0000-4000-8000-000000000005", func(lease memory.TurnLease, event memory.Event) {
		created, err = store.PrepareRememberEntity(ctx, writer.ScopeContext(), memory.RememberEntityRequest{
			Destination: memory.MemoryEverywhere, IdempotencyKey: "idem:v1:71500000-0000-4000-8000-000000000005", SourceEventID: event.ID,
			Predicate: "plays", PredicateLabel: "plays",
			Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
			Object:  memory.EntitySelector{Create: true, CanonicalName: "Tennis", EntityType: "sport", Alias: "tennis"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.ApplyRememberEntity(ctx, lease, created); err != nil {
			t.Fatal(err)
		}
	})
	sarah, err := store.InspectSemanticObject(ctx, reader.ScopeContext(), memory.SemanticObjectEntity, created.Claim.SubjectEntityID)
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(sarah); strings.Contains(string(encoded), "psychiatrist") || strings.Contains(string(encoded), "Sarah plays tennis") {
		t.Fatalf("one Workspace's words reached another Workspace through a Global Entity's history: %s", encoded)
	}

	// The Global reader keeps the whole message: it was said in its scope.
	own, err := store.InspectSemanticObject(ctx, global.ScopeContext(), memory.SemanticObjectClaim, legacy.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(own); !strings.Contains(string(encoded), "HIV") || !strings.Contains(string(encoded), "blood test") {
		t.Fatalf("Global reader lost its own operation history: %s", encoded)
	}
}

// Harness review final pass (M5, finding 4): an Evie-proposed memory's
// evidence is inspectable; it carries its label and quotes nothing.
func TestInspectingEvieProposedEvidenceIsAvailableWithoutText(t *testing.T) {
	ctx, store, global, _ := openOperationHistoryStore(t)
	command := "Read this travel article and remember whatever matters for me."
	proposal := rememberLiteralAt(t, ctx, store, global, command, "home_city", text("Lisbon"), "71500000-0000-4000-8000-000000000011")
	if proposal.Source.Authority != memory.AuthorityEvieProposed {
		t.Fatalf("value absent from the request kept owner authority: %+v", proposal.Source)
	}
	for i := 0; i < 100; i++ {
		coverage, err := store.RefreshMemoryIndex(ctx, 256)
		if err != nil {
			t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
	}
	result, err := store.SearchMemory(ctx, global.ScopeContext(), memory.RetrievalQuery{Text: "Lisbon"})
	if err != nil {
		t.Fatal(err)
	}
	var refs []memory.RetrievalReference
	for _, evidence := range result.Evidence {
		if evidence.ClaimID == proposal.ClaimID {
			refs = append(refs, evidence.Reference())
		}
	}
	if len(refs) != 1 {
		t.Fatalf("Evie-proposed memory is not retrievable: %+v", result.Evidence)
	}
	inspected, err := store.InspectMemoryEvidence(ctx, global.ScopeContext(), refs)
	if err != nil || len(inspected) != 1 {
		t.Fatalf("inspect: %+v: %v", inspected, err)
	}
	item := inspected[0]
	if !item.Available || item.Evidence == nil || len(item.Evidence.Sources) != 1 ||
		item.Evidence.Sources[0].Authority != memory.AuthorityEvieProposed || item.Evidence.Sources[0].Evidence != "" {
		t.Fatalf("Evie-proposed evidence is unavailable or quotes the request: %+v", item)
	}
}

// Confirmation review (defect 6): free text in operation JSON that cites no
// value (a review reason, a note, an approval card's identity details) is
// shown only to a reader in the scope it was written in.
func TestOperationHistoryBlanksFreeTextOutsideItsScope(t *testing.T) {
	operation := `{"kind":"remember_entity_claim","source":{"scope_key":"workspace:a","evidence":"","locator_kind":"utf8_byte_range"},` +
		`"identities":[{"role":"subject","scope_key":"global","selected_by":"alias","aliases":["Sis"],"example_claim":"Sarah — sees: the chemo clinic"}],` +
		`"preview":{"scope_key":"global","candidates":[{"edit":{"reason":"Keep it; my oncologist approved cocoa.","notes":"private"}}]}}`
	for _, tc := range []struct {
		reader          sourceReader
		identity, notes bool
	}{
		{sourceReader{context: "workspace:a", session: "session:x"}, true, false},
		{sourceReader{context: "global", session: "session:y"}, false, true},
		{sourceReader{context: "workspace:b", session: "session:z"}, false, false},
	} {
		n := operationNarrower{ctx: context.Background(), reader: tc.reader}
		rewritten, _, err := n.rewrite(json.RawMessage(operation), "")
		if err != nil {
			t.Fatal(err)
		}
		got := string(rewritten)
		if strings.Contains(got, "chemo clinic") != tc.identity || strings.Contains(got, `"Sis"`) != tc.identity {
			t.Fatalf("%+v: identity details shown=%v, want %v: %s", tc.reader, !tc.identity, tc.identity, got)
		}
		if strings.Contains(got, "oncologist") != tc.notes || strings.Contains(got, `"private"`) != tc.notes {
			t.Fatalf("%+v: review reason shown=%v, want %v: %s", tc.reader, !tc.notes, tc.notes, got)
		}
		var decoded map[string]any
		if err := json.Unmarshal(rewritten, &decoded); err != nil {
			t.Fatalf("rewritten operation is not JSON: %v: %s", err, got)
		}
	}
}

// Confirmation review (defect 1): a Global retire request narrowed for a
// Workspace reader renders the sentence stating the value, never a later
// sentence about someone else that merely names it.
func TestOperationHistoryNarrowsARequestToTheOwnersSentence(t *testing.T) {
	ctx, store, global, reader := openOperationHistoryStore(t)
	saved := rememberLiteralAt(t, ctx, store, global, "Remember that I live in Boston.", "home_city", text("Boston"), "71500000-0000-4000-8000-000000000301")
	applyLifecycleAt(t, ctx, store, global, "Forget that I live in Boston. My therapist in Boston says the panic attacks are getting worse.",
		memory.LifecycleRetire, memory.SemanticObjectClaim, saved.ClaimID, "71500000-0000-4000-8000-000000000302")
	inspected, err := store.InspectSemanticObject(ctx, reader.ScopeContext(), memory.SemanticObjectClaim, saved.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inspected)
	for _, private := range []string{"therapist", "panic attacks"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("Workspace reader saw a Global sentence about someone else (%q): %s", private, encoded)
		}
	}
	retired := false
	for _, operation := range inspected.Operations {
		retired = retired || operation.Kind == "retire_memory" && strings.Contains(operation.PreparedJSON, `"evidence":"Forget that I live in Boston."`)
	}
	if !retired {
		t.Fatalf("retire request was not narrowed to the owner's sentence: %s", encoded)
	}
}
