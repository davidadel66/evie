package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// Harness review Stage 14 (M5, M6): owner authority means the owner actually
// said it, and an Entity's identity is visible when names collide.

func (f *retrievalFixture) prepareLiteral(record memory.Session, command, predicate, value string) memory.RememberLiteralProposal {
	f.t.Helper()
	proposal, err := f.session(record, nil).PrepareRememberLiteral(context.Background(), f.store, command, memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: predicate, PredicateLabel: strings.ReplaceAll(predicate, "_", " "),
		PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: value}, Polarity: memory.PolarityAffirmed,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

func (f *retrievalFixture) approveLiteral(record memory.Session, proposal memory.RememberLiteralProposal) memory.RememberLiteralResult {
	f.t.Helper()
	result, err := f.session(record, nil).ResolveRememberLiteral(context.Background(), f.store, proposal, tools.Approved)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func suppliedEvidence(t *testing.T, client *fakeClient) []memory.RetrievalEvidence {
	t.Helper()
	var supplied struct {
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[len(client.reqs)-1]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
		t.Fatal(err)
	}
	return supplied.Evidence
}

func TestRememberCitesOnlyTheOwnerSpanAndWorkspaceSeesNoOtherGlobalText(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	command := "My blood test came back and the clinic wants a follow-up in March. Remember that my favorite color is Teal! The kids are both sick this week."
	span := "Remember that my favorite color is Teal!"
	proposal := f.prepareLiteral(global, command, "favorite_color", "teal")
	start := strings.Index(command, span)
	if proposal.Source.Authority != memory.AuthorityOwnerStatement || proposal.Source.LocatorKind != memory.LocatorUTF8ByteRange ||
		proposal.Source.LocatorValue != fmt.Sprintf("%d:%d", start, start+len(span)) || proposal.Source.Evidence != span ||
		proposal.Source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(span))) {
		t.Fatalf("proposal source is not the exact owner span: %+v", proposal.Source)
	}
	accepted := f.approveLiteral(global, proposal)

	workspace, err := f.store.RegisterWorkspace(ctx, "Garden planning")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	client, _ := f.search(reader, "teal")
	data := retrievalData(t, client.reqs[1])
	if !strings.Contains(data, string(accepted.ClaimID)) || !strings.Contains(data, span) {
		t.Fatalf("Workspace reader lost the accepted Global memory or its bound span: %s", data)
	}
	for _, private := range []string{"blood test", "clinic", "kids"} {
		if strings.Contains(data, private) {
			t.Fatalf("Global message text beyond the bound span reached a Workspace session (%q): %s", private, data)
		}
	}
	inspected, err := f.store.InspectSemanticObject(ctx, global.ScopeContext(), memory.SemanticObjectSourceLink, accepted.SourceLinkID)
	if err != nil || inspected.Source == nil || inspected.Source.Evidence != span {
		t.Fatalf("source inspection is not the bound span: %+v: %v", inspected.Source, err)
	}
	if verification, err := f.store.VerifySemanticProjection(ctx); err != nil || !verification.Valid {
		t.Fatalf("span-bound memory does not replay: %+v: %v", verification, err)
	}
}

func TestPromotingAnEvieProposedMemoryKeepsItsAuthority(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Kitchen remodel", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	proposal := f.prepareLiteral(source, "Skim the contractor brochure and keep what matters.", "tile_supplier", "Marblecraft")
	if proposal.Source.Authority != memory.AuthorityEvieProposed {
		t.Fatalf("value absent from the request kept owner authority: %+v", proposal.Source)
	}
	private := f.approveLiteral(source, proposal)
	local := f.session(source, nil)
	promotion, err := local.PreparePromotion(ctx, f.store, "Promote the tile supplier globally.", memory.PromotionRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), SourceClaimID: private.ClaimID, DestinationScopeKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := local.ResolvePromotion(ctx, f.store, promotion, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	evidence := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "Marblecraft"); return client }())
	found := false
	for _, item := range evidence {
		if item.ClaimID != promoted.DestinationClaimID {
			continue
		}
		found = true
		if len(item.Sources) != 1 || item.Sources[0].Authority != memory.AuthorityEvieProposed || item.Sources[0].Evidence != "" {
			t.Fatalf("Promotion laundered or quoted an Evie-proposed source: %+v", item.Sources)
		}
	}
	if !found {
		t.Fatalf("promoted Evie-proposed memory missing: %+v", evidence)
	}
	if verification, err := f.store.VerifySemanticProjection(ctx); err != nil || !verification.Valid {
		t.Fatalf("Evie-proposed memory does not replay: %+v: %v", verification, err)
	}
}

func TestRememberValueAbsentFromOwnerWordsIsEvieProposed(t *testing.T) {
	f := newRetrievalFixture(t)
	global := f.global()
	command := "Read this travel article and remember whatever matters for me."
	proposal := f.prepareLiteral(global, command, "home_city", "Lisbon")
	if proposal.Source.Authority != memory.AuthorityEvieProposed || proposal.Source.Actor != memory.SemanticActorOwner ||
		proposal.Source.LocatorKind != memory.LocatorWhole || proposal.Source.Evidence != "" ||
		proposal.Source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(command))) {
		t.Fatalf("value absent from the owner's words kept owner authority or quoted the message: %+v", proposal.Source)
	}
	accepted := f.approveLiteral(global, proposal)
	f.refresh()
	evidence := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "Lisbon"); return client }())
	found := false
	for _, item := range evidence {
		if item.ClaimID != accepted.ClaimID {
			continue
		}
		found = true
		if len(item.Sources) != 1 || item.Sources[0].Authority != memory.AuthorityEvieProposed || item.Sources[0].Evidence != "" {
			t.Fatalf("Evie-proposed memory lost its label or quoted the owner message: %+v", item.Sources)
		}
	}
	if !found {
		t.Fatalf("approved Evie-proposed memory is not retrievable: %+v", evidence)
	}
	inspected, err := f.store.InspectSemanticObject(context.Background(), global.ScopeContext(), memory.SemanticObjectSourceLink, accepted.SourceLinkID)
	if err != nil || inspected.Source == nil || inspected.Source.Authority != memory.AuthorityEvieProposed || inspected.Source.Evidence != "" {
		t.Fatalf("source inspection quoted the request as evidence for Evie's proposal: %+v: %v", inspected.Source, err)
	}
	claim, err := f.store.InspectSemanticObject(context.Background(), global.ScopeContext(), memory.SemanticObjectClaim, accepted.ClaimID)
	if err != nil {
		t.Fatal(err)
	}
	if encoded, _ := json.Marshal(claim); strings.Contains(string(encoded), "travel article") {
		t.Fatalf("Claim inspection quoted the request as evidence for Evie's proposal: %s", encoded)
	}
}

func TestModelProposedMemoryFromFetchedTextIsLabelledOnTheApprovalCard(t *testing.T) {
	f := newRetrievalFixture(t)
	global := f.global()
	args, _ := json.Marshal(map[string]string{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "predicate": "preferred_payee", "predicate_label": "preferred payee",
		"cardinality": "one", "literal_kind": "text", "literal_value": "Acme Offshore Holdings", "polarity": "affirmed",
	})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("remember", "memory_remember_literal", string(args))), assistantStep("Saved.", nil)}}
	var shown string
	approve := func(_ context.Context, name, arguments string, _ *tools.FileChangePreview) tools.Decision {
		if name == "memory_remember_literal" {
			shown = arguments
		}
		return tools.Declined
	}
	if err := f.session(global, client).Send(context.Background(), "Read this article and remember what matters.", &recorder{}, approve); err != nil {
		t.Fatal(err)
	}
	var proposal memory.RememberLiteralProposal
	if err := json.Unmarshal([]byte(shown), &proposal); err != nil {
		t.Fatalf("approval arguments are not the prepared proposal: %q: %v", shown, err)
	}
	if proposal.Source.Authority != memory.AuthorityEvieProposed || proposal.Source.Evidence != "" ||
		strings.Contains(shown, "Read this article") {
		t.Fatalf("approval card presented an injected value as the owner's statement: %s", shown)
	}
}

func TestRetiringASpanBoundMemoryKeepsTheRestOfTheMessageRecallable(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	command := "The plumber replaced the boiler valve this morning. Remember that my locker code word is pelican."
	proposal := f.prepareLiteral(source, command, "locker_code_word", "pelican")
	accepted := f.approveLiteral(source, proposal)
	f.refresh()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, accepted.ClaimID)
	f.refresh()

	evidence := suppliedEvidence(t, f.searchConversations(f.global(), "boiler"))
	found := false
	for _, item := range evidence {
		if strings.Contains(item.Text, "pelican") {
			t.Fatalf("retired memory's bound span came back through conversation search: %+v", item)
		}
		found = found || strings.Contains(item.Text, "boiler valve")
	}
	if !found {
		t.Fatalf("retirement hid the unrelated sentence of the same message: %+v", evidence)
	}
	for _, item := range suppliedEvidence(t, f.searchConversations(f.global(), "pelican")) {
		if strings.Contains(item.Text, "pelican") {
			t.Fatalf("retired span is recallable: %+v", item)
		}
	}
}

// Harness review final pass (M5, finding 1): the source is the sentence that
// states the value, so another scope never receives an unrelated sentence of
// the message and retirement suppresses the right one.
func TestRememberCitesTheSentenceThatStatesTheValueNotAnEarlierMention(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	command := "My therapist in Boston says the panic attacks are getting worse. Remember that I live in Boston."
	span := "Remember that I live in Boston."
	proposal := f.prepareLiteral(global, command, "home_city", "Boston")
	if proposal.Source.Authority != memory.AuthorityOwnerStatement || proposal.Source.Evidence != span {
		t.Fatalf("proposal cites the wrong sentence: %+v", proposal.Source)
	}
	accepted := f.approveLiteral(global, proposal)
	workspace, err := f.store.RegisterWorkspace(ctx, "Garden planning")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	client, _ := f.search(reader, "home city")
	data := retrievalData(t, client.reqs[1])
	if !strings.Contains(data, string(accepted.ClaimID)) || !strings.Contains(data, span) {
		t.Fatalf("Workspace reader lost the memory or its stating sentence: %s", data)
	}
	for _, private := range []string{"therapist", "panic attacks"} {
		if strings.Contains(data, private) {
			t.Fatalf("Workspace reader received an unrelated Global sentence as the source (%q): %s", private, data)
		}
	}
	f.lifecycle(global, "memory_retire", memory.SemanticObjectClaim, accepted.ClaimID)
	f.refresh()
	for _, item := range suppliedEvidence(t, f.searchConversations(f.global(), "live in Boston")) {
		if !strings.Contains(item.Text, "I live in Boston") {
			continue
		}
		linked := false
		for _, link := range item.HistoricalClaims {
			linked = linked || link.ClaimID == accepted.ClaimID
		}
		if !linked {
			t.Fatalf("retired memory's own sentence is recalled as current: %+v", item)
		}
	}
}

// Harness review final pass (M5, finding 3): a Workspace session inspecting a
// Global memory through memory_inspect_object receives no other Global text,
// including the owner's lifecycle request in the operation history.
func TestWorkspaceInspectionOfAGlobalMemoryCarriesNoOtherGlobalText(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	accepted := f.approveLiteral(global, f.prepareLiteral(global, "My blood test came back. Remember that my favorite color is teal.", "favorite_color", "teal"))
	retire, _ := json.Marshal(map[string]string{"idempotency_key": "idem:v1:" + uuid.NewString(), "object_kind": "claim", "object_id": string(accepted.ClaimID)})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("retire", "memory_retire", string(retire))), assistantStep("Done.", nil)}}
	approve := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }
	if err := f.session(global, client).Send(ctx, "My HIV test was negative. Please forget my favorite color.", &recorder{}, approve); err != nil {
		t.Fatal(err)
	}
	workspace, err := f.store.RegisterWorkspace(ctx, "Garden planning")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	inspect, _ := json.Marshal(map[string]string{"object_kind": "claim", "object_id": string(accepted.ClaimID)})
	client = &fakeClient{steps: []step{assistantStep("", nil, toolCall("inspect", "memory_inspect_object", string(inspect))), assistantStep("Seen.", nil)}}
	if err := f.session(reader, client).Send(ctx, "Inspect that memory.", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	sent, _ := json.Marshal(client.reqs[len(client.reqs)-1])
	if !strings.Contains(string(sent), "retire_memory") || !strings.Contains(string(sent), "Remember that my favorite color is teal.") {
		t.Fatalf("inspection lost the memory's history or bound span: %s", sent)
	}
	for _, private := range []string{"HIV", "blood test"} {
		if strings.Contains(string(sent), private) {
			t.Fatalf("Workspace inspection carried Global message text %q: %s", private, sent)
		}
	}
}

func (f *retrievalFixture) prepareEntity(record memory.Session, command string, request memory.RememberEntityRequest) memory.RememberEntityProposal {
	f.t.Helper()
	request.IdempotencyKey = "idem:v1:" + uuid.NewString()
	request.PredicateCardinality, request.Polarity = memory.CardinalityMany, memory.PolarityAffirmed
	proposal, err := f.session(record, nil).PrepareRememberEntity(context.Background(), f.store, command, request)
	if err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

func proposalIdentity(t *testing.T, proposal memory.RememberEntityProposal, role string) memory.ProposalEntityIdentity {
	t.Helper()
	for _, identity := range proposal.Identities {
		if identity.Role == role {
			return identity
		}
	}
	t.Fatalf("proposal has no %s identity: %+v", role, proposal.Identities)
	return memory.ProposalEntityIdentity{}
}

func TestRememberEntityShowsReusedAndSameNamedEntitiesForApproval(t *testing.T) {
	f := newRetrievalFixture(t)
	global := f.global()
	first := f.rememberGraphEntity(global, "My sister Sarah plays tennis.", memory.RememberEntityRequest{
		Predicate: "plays", PredicateLabel: "plays",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Tennis", EntityType: "sport", Alias: "tennis"},
	})

	byAlias := f.prepareEntity(global, "Sarah also loves chess.", memory.RememberEntityRequest{
		Predicate: "loves", PredicateLabel: "loves",
		Subject: memory.EntitySelector{Alias: "Sarah"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Chess", EntityType: "game", Alias: "chess"},
	})
	subject := proposalIdentity(t, byAlias, "subject")
	if !subject.Reused || subject.SelectedBy != memory.EntitySelectedByAlias || subject.EntityID != first.Claim.SubjectEntityID ||
		subject.CanonicalName != "Sarah" || subject.EntityType != "person" || len(subject.Aliases) == 0 || subject.Aliases[0] != "Sarah" ||
		subject.ExampleClaim != "Sarah — plays: Tennis" {
		t.Fatalf("alias reuse is not shown with distinguishing detail: %+v", subject)
	}
	if object := proposalIdentity(t, byAlias, "object"); object.Reused || object.SelectedBy != memory.EntitySelectedByCreate || object.CanonicalName != "Chess" {
		t.Fatalf("created object identity = %+v", object)
	}

	byID := f.prepareEntity(global, "Sarah plays tennis on Sundays.", memory.RememberEntityRequest{
		Predicate: "plays_on_sundays", PredicateLabel: "plays on Sundays",
		Subject: memory.EntitySelector{EntityID: first.Claim.SubjectEntityID}, Object: memory.EntitySelector{EntityID: first.Claim.ObjectEntityID},
	})
	if identity := proposalIdentity(t, byID, "subject"); !identity.Reused || identity.SelectedBy != memory.EntitySelectedByID || identity.EntityID != first.Claim.SubjectEntityID {
		t.Fatalf("stable-ID reuse is not shown: %+v", identity)
	}

	second := f.prepareEntity(global, "My coworker Sarah plays squash.", memory.RememberEntityRequest{
		Predicate: "plays", PredicateLabel: "plays",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Squash", EntityType: "sport", Alias: "squash"},
	})
	if identity := proposalIdentity(t, second, "subject"); identity.Reused || identity.SameName != 1 {
		t.Fatalf("a new same-named Entity does not say another Sarah exists: %+v", identity)
	}
}

func TestRecallMarksClaimsWhoseEntityNameIsAmbiguous(t *testing.T) {
	f := newRetrievalFixture(t)
	global := f.global()
	sister := f.rememberGraphEntity(global, "My sister Sarah plays tennis.", memory.RememberEntityRequest{
		Predicate: "plays", PredicateLabel: "plays",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Tennis", EntityType: "sport", Alias: "tennis"},
	})
	coworker := f.rememberGraphEntity(global, "My coworker Sarah plays squash.", memory.RememberEntityRequest{
		Predicate: "plays", PredicateLabel: "plays",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Sarah", EntityType: "person", Alias: "Sarah"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Squash", EntityType: "sport", Alias: "squash"},
	})
	solo := f.rememberGraphEntity(global, "My uncle Omar plays chess.", memory.RememberEntityRequest{
		Predicate: "plays", PredicateLabel: "plays",
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Omar", EntityType: "person", Alias: "Omar"},
		Object:  memory.EntitySelector{Create: true, CanonicalName: "Chess", EntityType: "game", Alias: "chess"},
	})
	f.refresh()
	evidence := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "Sarah"); return client }())
	texts := map[memory.SemanticID]memory.RetrievalEvidence{}
	for _, item := range evidence {
		texts[item.ClaimID] = item
	}
	for _, proposal := range []memory.RememberEntityProposal{sister, coworker} {
		item, ok := texts[proposal.Claim.ID]
		if !ok {
			t.Fatalf("Sarah Claim %s missing from recall: %+v", proposal.Claim.ID, evidence)
		}
		if len(item.AmbiguousNames) != 1 || item.AmbiguousNames[0].EntityID != proposal.Claim.SubjectEntityID ||
			item.AmbiguousNames[0].Entities != 2 || !strings.EqualFold(item.AmbiguousNames[0].Name, "sarah") ||
			!strings.Contains(item.Text, string(proposal.Claim.SubjectEntityID)[:8]) || !strings.Contains(item.Text, "2 entities") {
			t.Fatalf("ambiguous Sarah is not marked with its identity: %+v", item)
		}
	}
	if texts[sister.Claim.ID].Text == texts[coworker.Claim.ID].Text {
		t.Fatalf("two different Sarahs render identically: %q", texts[sister.Claim.ID].Text)
	}
	omar := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "Omar"); return client }())
	for _, item := range omar {
		if item.ClaimID == solo.Claim.ID && (len(item.AmbiguousNames) != 0 || item.Text != "Omar — plays: Chess") {
			t.Fatalf("an unambiguous name was marked: %+v", item)
		}
	}
}

func TestRetiringAnEvieProposedMemoryLeavesTheRequestMessageUnlabelled(t *testing.T) {
	f := newRetrievalFixture(t)
	source := f.global()
	command := "Read the brochure about the ferry timetable and remember what matters."
	proposal := f.prepareLiteral(source, command, "departure_pier", "Pier 39")
	if proposal.Source.Authority != memory.AuthorityEvieProposed {
		t.Fatalf("value absent from the request kept owner authority: %+v", proposal.Source)
	}
	accepted := f.approveLiteral(source, proposal)
	f.refresh()
	f.lifecycle(source, "memory_retire", memory.SemanticObjectClaim, accepted.ClaimID)
	f.refresh()
	found := false
	for _, item := range suppliedEvidence(t, f.searchConversations(f.global(), "ferry timetable")) {
		if !strings.Contains(item.Text, "ferry timetable") {
			continue
		}
		found = true
		for _, link := range item.HistoricalClaims {
			if link.ClaimID == accepted.ClaimID {
				t.Fatalf("the owner's request was labelled a source of Evie's retired proposal: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("retiring an Evie-proposed memory hid the owner's unrelated request message")
	}
}

func (f *retrievalFixture) prepareCorrection(record memory.Session, saved memory.RememberLiteralProposal, command, value string) memory.CorrectClaimProposal {
	f.t.Helper()
	proposal, err := f.session(record, nil).PrepareCorrectClaim(context.Background(), f.store, command, memory.CorrectClaimRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), OldClaimID: saved.ClaimID, Mode: memory.CorrectionError,
		Replacement: memory.ClaimProposition{SubjectEntityID: saved.Subject.ID, PredicateID: saved.Predicate.ID, Polarity: memory.PolarityAffirmed,
			Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: value}}}})
	if err != nil {
		f.t.Fatal(err)
	}
	return proposal
}

// A correction's replacement value is proposed the same way as a remembered
// value, so it binds to the owner's words the same way (harness review M5).
func TestCorrectionCitesOnlyTheOwnerSpanOfItsReplacement(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	global := f.global()
	saved := f.prepareLiteral(global, "Remember that my mom lives in Boston.", "mom_city", "Boston")
	f.approveLiteral(global, saved)
	command := "We talked about the clinic and the kids yesterday. Correction: my mom lives in Chicago, not Boston."
	span := "Correction: my mom lives in Chicago, not Boston."
	proposal := f.prepareCorrection(global, saved, command, "Chicago")
	start := strings.Index(command, span)
	if proposal.Source.Authority != memory.AuthorityOwnerStatement || proposal.Source.LocatorKind != memory.LocatorUTF8ByteRange ||
		proposal.Source.LocatorValue != fmt.Sprintf("%d:%d", start, start+len(span)) || proposal.Source.Evidence != span ||
		proposal.Source.EvidenceSHA256 != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(span))) {
		t.Fatalf("correction source is not the exact owner span: %+v", proposal.Source)
	}
	if _, err := f.session(global, nil).ResolveCorrectClaim(ctx, f.store, proposal, tools.Approved); err != nil {
		t.Fatal(err)
	}

	other := f.prepareLiteral(global, "Remember that my dad lives in Denver.", "dad_city", "Denver")
	f.approveLiteral(global, other)
	absent := f.prepareCorrection(global, other, "Fix my dad's city using the article I pasted.", "Lisbon")
	if absent.Source.Authority != memory.AuthorityEvieProposed || absent.Source.LocatorKind != memory.LocatorWhole || absent.Source.Evidence != "" {
		t.Fatalf("correction value absent from the owner's words kept owner authority or quoted the request: %+v", absent.Source)
	}
	corrected, err := f.session(global, nil).ResolveCorrectClaim(ctx, f.store, absent, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	evidence := suppliedEvidence(t, func() *fakeClient { client, _ := f.search(f.global(), "Lisbon"); return client }())
	found := false
	for _, item := range evidence {
		if item.ClaimID == corrected.ReplacementClaimID {
			found = len(item.Sources) == 1 && item.Sources[0].Authority == memory.AuthorityEvieProposed && item.Sources[0].Evidence == ""
		}
	}
	if !found {
		t.Fatalf("Evie-proposed correction is missing or lost its label: %+v", evidence)
	}
	if verification, err := f.store.VerifySemanticProjection(ctx); err != nil || !verification.Valid {
		t.Fatalf("bound corrections do not replay: %+v: %v", verification, err)
	}
}

func TestModelProposedCorrectionFromFetchedTextIsLabelledOnTheApprovalCard(t *testing.T) {
	f := newRetrievalFixture(t)
	global := f.global()
	saved := f.prepareLiteral(global, "Remember that my preferred payee is Northwind Credit Union.", "preferred_payee", "Northwind Credit Union")
	f.approveLiteral(global, saved)
	args, _ := json.Marshal(map[string]string{
		"idempotency_key": "idem:v1:" + uuid.NewString(), "claim_id": string(saved.ClaimID), "subject_entity_id": string(saved.Subject.ID),
		"predicate_id": string(saved.Predicate.ID), "literal_kind": "text", "literal_value": "Acme Offshore Holdings", "polarity": "affirmed", "mode": "error",
	})
	client := &fakeClient{steps: []step{assistantStep("", nil, toolCall("correct", "memory_correct_claim", string(args))), assistantStep("Updated.", nil)}}
	var shown string
	approve := func(_ context.Context, name, arguments string, _ *tools.FileChangePreview) tools.Decision {
		if name == "memory_correct_claim" {
			shown = arguments
		}
		return tools.Declined
	}
	if err := f.session(global, client).Send(context.Background(), "Read this article and update my payee if needed.", &recorder{}, approve); err != nil {
		t.Fatal(err)
	}
	var proposal memory.CorrectClaimProposal
	if err := json.Unmarshal([]byte(shown), &proposal); err != nil {
		t.Fatalf("approval arguments are not the prepared correction: %q: %v", shown, err)
	}
	if proposal.Source.Authority != memory.AuthorityEvieProposed || proposal.Source.Evidence != "" || strings.Contains(shown, "Read this article") {
		t.Fatalf("approval card presented an injected correction as the owner's statement: %s", shown)
	}
}
