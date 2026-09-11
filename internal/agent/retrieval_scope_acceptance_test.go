package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

func TestMemorySearchTurnKeepsPromotedSourceBoundaries(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	project, err := f.store.RegisterProject(ctx, "Private project", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.CreateProjectSession(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	local := f.session(source, nil)
	proposal, err := local.PrepareRememberLiteral(ctx, f.store, "Unshared project blueprints accompany the hyacinth preference.", memory.RememberLiteralRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "favorite_flower", PredicateLabel: "favorite flower", PredicateCardinality: memory.CardinalityMany,
		Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: "hyacinth"}, Polarity: memory.PolarityAffirmed})
	if err != nil {
		t.Fatal(err)
	}
	private, err := local.ResolveRememberLiteral(ctx, f.store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	global := f.global()
	f.refresh()
	before, _ := f.search(global, "hyacinth")
	data := retrievalData(t, before.reqs[1])
	if strings.Contains(data, string(private.ClaimID)) || strings.Contains(data, "Unshared project blueprints") {
		t.Fatal("Global reader received private project evidence")
	}
	promotion, err := local.PreparePromotion(ctx, f.store, "Promote the accepted flower preference globally.", memory.PromotionRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), SourceClaimID: private.ClaimID, DestinationScopeKey: "global"})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := local.ResolvePromotion(ctx, f.store, promotion, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	after, _ := f.search(global, "hyacinth")
	data = retrievalData(t, after.reqs[1])
	if !strings.Contains(data, string(promoted.DestinationClaimID)) {
		t.Fatal("eligible promoted Global Claim was missing")
	}
	if strings.Contains(data, "Unshared project blueprints") {
		t.Fatal("promoted Claim disclosed narrower source text")
	}
}

func TestMemorySearchTurnFindsExplicitCurrentSessionEntityClaim(t *testing.T) {
	f := newRetrievalFixture(t)
	current := f.global()
	local := f.session(current, nil)
	proposal, err := local.PrepareRememberEntityForCurrentSession(context.Background(), f.store, "For this session Maya recommends camellia.", memory.RememberEntityRequest{
		IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "recommends", PredicateLabel: "recommends", PredicateCardinality: memory.CardinalityMany, Polarity: memory.PolarityAffirmed,
		Subject: memory.EntitySelector{Create: true, CanonicalName: "Maya", EntityType: "person", Alias: "MZ-731"}, Object: memory.EntitySelector{Create: true, CanonicalName: "camellia", EntityType: "plant", Alias: "camellia"}})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := local.ResolveRememberEntity(context.Background(), f.store, proposal, tools.Approved)
	if err != nil {
		t.Fatal(err)
	}
	f.refresh()
	client, _ := f.search(current, "camellia")
	if !strings.Contains(retrievalData(t, client.reqs[1]), string(accepted.ClaimID)) {
		t.Fatal("current-session Entity Claim was not supplied")
	}
	for _, test := range []struct {
		name, query string
		exactOnly   bool
	}{
		{name: "subject identifier", query: string(proposal.Claim.SubjectEntityID), exactOnly: true},
		{name: "accepted alias", query: "MZ-731"},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := f.search(current, test.query)
			var supplied struct {
				Evidence []memory.RetrievalEvidence `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[1]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
				t.Fatal(err)
			}
			if len(supplied.Evidence) != 1 || supplied.Evidence[0].ClaimID != accepted.ClaimID {
				t.Fatalf("exact identity evidence=%+v", supplied.Evidence)
			}
			paths := supplied.Evidence[0].Paths
			if !strings.Contains(strings.Join(paths, ","), "exact_or_alias") {
				t.Fatalf("exact path absent: %v", paths)
			}
			if test.exactOnly && !reflect.DeepEqual(paths, []string{"exact_or_alias"}) {
				t.Fatalf("identifier unexpectedly relied on lexical search: %v", paths)
			}
		})
	}
}

func TestMemorySearchTurnRechecksArchivedSourceArea(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	workspace, err := f.store.RegisterWorkspace(ctx, "Evidence area")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	claim := f.remember(source, memory.MemoryEverywhere, "archived amaranth")
	global := f.global()
	f.refresh()
	before, _ := f.search(global, "amaranth")
	if !strings.Contains(retrievalData(t, before.reqs[1]), string(claim.ClaimID)) {
		t.Fatal("initial eligible Global Claim missing")
	}
	if _, err = f.store.ArchiveWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := f.search(global, "amaranth")
	if strings.Contains(retrievalData(t, after.reqs[1]), string(claim.ClaimID)) {
		t.Fatal("retrieval reused evidence from an unavailable source area")
	}
}

func TestMemorySearchTurnScopeMatrix(t *testing.T) {
	f := newRetrievalFixture(t)
	ctx := context.Background()
	resolved, err := standardManager(t, f.store).ResolvePreset(plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	type actor struct {
		name                                     string
		session, sibling                         memory.Session
		contextClaim, sessionClaim, siblingClaim memory.SemanticID
	}
	actors := []actor{{name: "Global", session: f.global(), sibling: f.global()}}
	for _, name := range []string{"General", "Second Workspace"} {
		workspace, err := f.store.RegisterWorkspace(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		sibling, err := f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, resolved.Receipt)
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, actor{name: name, session: current, sibling: sibling})
	}
	for _, name := range []string{"Project A", "Project B"} {
		project, err := f.store.RegisterProject(ctx, name, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		current, err := f.store.CreateProjectSession(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		sibling, err := f.store.CreateProjectSession(ctx, project.ID)
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, actor{name: name, session: current, sibling: sibling})
	}
	for i := range actors {
		a := &actors[i]
		a.contextClaim = f.remember(a.session, "", "scopeviolet "+a.name+" context").ClaimID
		a.sessionClaim = f.remember(a.session, memory.MemorySession, "scopeviolet "+a.name+" current session").ClaimID
		a.siblingClaim = f.remember(a.sibling, memory.MemorySession, "scopeviolet "+a.name+" sibling session").ClaimID
	}
	f.refresh()
	for _, a := range actors {
		t.Run(a.name, func(t *testing.T) {
			client, _ := f.search(a.session, "scopeviolet")
			var supplied struct {
				Evidence []memory.RetrievalEvidence `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(retrievalData(t, client.reqs[1]), "EVIE_MEMORY_DATA\n")), &supplied); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range supplied.Evidence {
				got = append(got, string(e.ClaimID))
			}
			want := []string{string(actors[0].contextClaim), string(a.sessionClaim)}
			if a.name != "Global" {
				want = append(want, string(a.contextClaim))
			}
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("provider evidence Claims=%v want exactly %v", got, want)
			}
		})
	}
}
