package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

const integratedWorkloadDefault = "../../cmd/evie/docs/fixtures/memory-stage5-integrated/development/v1/workload.json"

type integratedEntity struct {
	Key        string `json:"key"`
	Name       string `json:"name"`
	EntityType string `json:"entity_type"`
	Alias      string `json:"alias,omitempty"`
}

type integratedRecord struct {
	ID        string           `json:"id"`
	Kind      string           `json:"kind"`
	SessionID string           `json:"session_id"`
	Scope     string           `json:"scope"`
	Text      string           `json:"text"`
	Subject   integratedEntity `json:"subject"`
	Predicate struct {
		Token       string                      `json:"token"`
		Label       string                      `json:"label"`
		Cardinality memory.PredicateCardinality `json:"cardinality"`
	} `json:"predicate"`
	Object struct {
		integratedEntity
		Kind        string             `json:"kind"`
		LiteralKind memory.LiteralKind `json:"literal_kind"`
		Value       string             `json:"value"`
	} `json:"object"`
	ValidTime memory.ValidTime `json:"valid_time"`
	Retired   bool             `json:"retired"`
}

type integratedDiscussion struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Assistant string `json:"assistant"`
}

type integratedGold struct {
	OracleIntent      string     `json:"oracle_intent,omitempty"`
	OracleValidAt     *time.Time `json:"oracle_valid_at,omitempty"`
	SupportSets       [][]string `json:"support_sets"`
	AcceptableContext []string   `json:"acceptable_context_record_ids"`
	ForbiddenCurrent  []string   `json:"forbidden_current_record_ids"`
	ForbiddenText     []string   `json:"forbidden_current_text"`
	Components        []string   `json:"expected_answer_components"`
	ResponseKind      string     `json:"response_kind"`
	RequireCitations  bool       `json:"require_source_event_citations"`
	Attribution       []string   `json:"attribution"`
	ReaderRules       []string   `json:"reader_rules"`
}

type integratedCase struct {
	GateRoles   []string               `json:"gate_roles"`
	ID          string                 `json:"id"`
	Family      string                 `json:"family"`
	ReaderScope string                 `json:"reader_scope"`
	MemoryMode  string                 `json:"memory_mode"`
	Records     []integratedRecord     `json:"records"`
	Recent      []integratedDiscussion `json:"recent_discussion"`
	Continuity  string                 `json:"compaction_continuity,omitempty"`
	Question    string                 `json:"question"`
	Gold        integratedGold         `json:"gold"`
}

type integratedWorkload struct {
	Version   int              `json:"version"`
	Partition string           `json:"partition"`
	Cases     []integratedCase `json:"cases"`
}

type integratedBinding struct {
	RecordID        string                    `json:"record_id"`
	RecordKind      string                    `json:"record_kind"`
	Session         memory.Session            `json:"session"`
	Source          memory.SemanticSource     `json:"source"`
	ClaimID         memory.SemanticID         `json:"claim_id,omitempty"`
	OperationID     memory.SemanticID         `json:"claim_operation_id,omitempty"`
	SubjectEntityID memory.SemanticID         `json:"subject_entity_id,omitempty"`
	Aliases         []memory.SemanticAlias    `json:"accepted_aliases,omitempty"`
	AcceptedAt      time.Time                 `json:"accepted_at,omitempty"`
	ValidTime       memory.ValidTime          `json:"valid_time"`
	Retired         bool                      `json:"retired"`
	Lifecycle       []memory.SemanticState    `json:"lifecycle,omitempty"`
	Reference       memory.RetrievalReference `json:"exact_reference"`
}

type integratedSeed struct {
	Version       int                                `json:"version"`
	Case          integratedCase                     `json:"case"`
	Question      string                             `json:"rendered_question"`
	Bindings      map[string]integratedBinding       `json:"bindings"`
	Readers       map[string]memory.Session          `json:"readers"`
	Auxiliary     []memory.SemanticSource            `json:"auxiliary_sources"`
	RecentSources map[string][]memory.SemanticSource `json:"recent_sources"`
	// These originals support source auditing only; their presence is neither
	// gold support nor authorization to disclose them in a reader's scope.
	AllPublicSources   []memory.SemanticSource  `json:"all_public_sources"`
	Compaction         map[string]any           `json:"compaction,omitempty"`
	IndexCoverage      memory.RetrievalCoverage `json:"index_coverage"`
	IndexBatches       int                      `json:"index_batches"`
	IndexElapsedNS     int64                    `json:"index_elapsed_ns"`
	SeedDatabaseSHA256 string                   `json:"seed_database_sha256"`
}

func integratedLoadWorkload(t *testing.T, path string) integratedWorkload {
	t.Helper()
	if path == "" {
		path = integratedWorkloadDefault
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workload integratedWorkload
	if err := json.Unmarshal(raw, &workload); err != nil {
		t.Fatal(err)
	}
	if workload.Version != 1 || (workload.Partition != "development" && workload.Partition != "heldout") || len(workload.Cases) != 24 {
		t.Fatal("integrated harness permits only a declared 24-case version-1 development or heldout workload")
	}
	seen := map[string]bool{}
	for _, c := range workload.Cases {
		if c.ID == "" || seen[c.ID] {
			t.Fatal("invalid or duplicate development case ID")
		}
		seen[c.ID] = true
		byID := map[string]integratedRecord{}
		for _, r := range c.Records {
			if r.ID == "" || byID[r.ID].ID != "" {
				t.Fatal("duplicate source label")
			}
			byID[r.ID] = r
		}
		if len(c.Gold.SupportSets) != 1 {
			t.Fatal("version-1 oracle requires the one predeclared sufficient alternative")
		}
		for _, support := range c.Gold.SupportSets {
			counts := map[string]int{}
			for _, id := range support {
				r, ok := byID[id]
				if !ok {
					t.Fatalf("gold source %s has no independent record", id)
				}
				kind := memory.RetrievalConversationExcerpt
				if strings.HasPrefix(r.Kind, "accepted_") {
					kind = memory.RetrievalAcceptedMemory
				}
				counts[kind]++
				if counts[kind] > 2 {
					t.Fatalf("%s oracle exceeds two evidence items per kind", c.ID)
				}
			}
		}
	}
	return workload
}

func integratedScopeKey(s memory.Session) string {
	if s.ProjectID != "" {
		return "project:" + string(s.ProjectID)
	}
	if s.WorkspaceID != "" {
		return "workspace:" + string(s.WorkspaceID)
	}
	return "global"
}

func integratedEventSource(event memory.Event, session memory.Session) memory.SemanticSource {
	actor, authority := memory.SemanticActorOwner, memory.AuthorityOwnerStatement
	if event.Type == memory.EventAssistantMessage {
		actor, authority = "assistant", "none"
	}
	return memory.SemanticSource{EventID: event.ID, SessionID: session.ID, ScopeKey: integratedScopeKey(session), EventPart: memory.EvidenceContent,
		LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: fmt.Sprintf("0:%d", len(event.Content)), EvidenceSHA256: memory.CompilerHash([]byte(event.Content)),
		Actor: actor, SourceType: memory.SemanticSourceType(event.Type), Authority: authority, ObservedAt: event.RecordedAt.UTC().Format(time.RFC3339Nano), Evidence: event.Content, Eligibility: memory.EligibilityEligible}
}

func integratedConversation(t *testing.T, f *retrievalFixture, session memory.Session, owner, assistant string) []memory.SemanticSource {
	t.Helper()
	before, err := f.store.LoadEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	client := &fakeClient{steps: []step{assistantStep(assistant, nil)}}
	if err := f.session(session, client).Send(context.Background(), owner, &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	events, err := f.store.LoadEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	var result []memory.SemanticSource
	for _, event := range events[len(before):] {
		if event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage {
			result = append(result, integratedEventSource(event, session))
		}
	}
	if len(result) != 2 || result[0].Evidence != owner || result[1].Evidence != assistant {
		t.Fatal("public conversation did not preserve the declared two source events")
	}
	return result
}

// integratedBuildSeed writes approved facts and ordinary turns once. Every
// condition receives a private byte-copy after SQLite closes and checkpoints.
// No search result is used to choose gold or manufacture its provenance.
func integratedBuildSeed(t *testing.T, c integratedCase, destination string) integratedSeed {
	t.Helper()
	f := newRetrievalFixture(t)
	seed := integratedSeed{Version: 1, Case: c, Question: c.Question, Bindings: map[string]integratedBinding{}, Readers: map[string]memory.Session{}, RecentSources: map[string][]memory.SemanticSource{}}
	projects := map[string]memory.ProjectID{}
	newSession := func(scope string) memory.Session {
		if scope == "global" {
			return f.global()
		}
		if !strings.HasPrefix(scope, "project:") {
			t.Fatalf("unsupported declared fixture scope %s", scope)
		}
		project := projects[scope]
		if project == "" {
			p, err := f.store.RegisterProject(context.Background(), scope, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			project = p.ID
			projects[scope] = project
		}
		s, err := f.store.CreateProjectSession(context.Background(), project)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	sessions := map[string]memory.Session{}
	entities := map[string]memory.SemanticID{}
	selector := func(scope string, entity integratedEntity) memory.EntitySelector {
		if id := entities[scope+":"+entity.Key]; id != "" {
			return memory.EntitySelector{EntityID: id}
		}
		if entity.Alias == "" {
			// Public creation requires an explicit alias. Reuse the exact
			// declared name; never invent an additional identity association.
			entity.Alias = entity.Name
		}
		return memory.EntitySelector{Create: true, CanonicalName: entity.Name, EntityType: entity.EntityType, Alias: entity.Alias}
	}
	for i := 0; i < len(c.Records); i++ {
		r := c.Records[i]
		source, exists := sessions[r.SessionID]
		if !exists {
			source = newSession(r.Scope)
			sessions[r.SessionID] = source
		}
		if integratedScopeKey(source) != "global" && r.Scope == "global" {
			t.Fatal("source session label reused across scopes")
		}
		binding := integratedBinding{RecordID: r.ID, RecordKind: r.Kind, Session: source, ValidTime: r.ValidTime, Retired: r.Retired}
		destinationScope := memory.MemoryDestination("")
		if r.Scope == "global" {
			destinationScope = memory.MemoryEverywhere
		}
		session := f.session(source, nil)
		switch r.Kind {
		case "accepted_literal":
			if r.Subject.Key != "owner" {
				t.Fatal("literal approval can only bind the actual local Owner")
			}
			proposal, err := session.PrepareRememberLiteral(context.Background(), f.store, r.Text, memory.RememberLiteralRequest{Destination: destinationScope, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: r.Predicate.Token, PredicateLabel: r.Predicate.Label, PredicateCardinality: r.Predicate.Cardinality, Literal: memory.TypedLiteral{Kind: r.Object.LiteralKind, Value: r.Object.Value}, Polarity: memory.PolarityAffirmed, ValidTime: r.ValidTime})
			if err != nil {
				t.Fatalf("%s prepare: %v", r.ID, err)
			}
			result, err := session.ResolveRememberLiteral(context.Background(), f.store, proposal, tools.Approved)
			if err != nil {
				t.Fatal(err)
			}
			binding.ClaimID, binding.OperationID, binding.AcceptedAt, binding.SubjectEntityID = result.ClaimID, result.OperationID, result.TransactionTime, proposal.Subject.ID
		case "accepted_entity":
			proposal, err := session.PrepareRememberEntity(context.Background(), f.store, r.Text, memory.RememberEntityRequest{Destination: destinationScope, IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: r.Predicate.Token, PredicateLabel: r.Predicate.Label, PredicateCardinality: r.Predicate.Cardinality, Subject: selector(r.Scope, r.Subject), Object: selector(r.Scope, r.Object.integratedEntity), Polarity: memory.PolarityAffirmed, ValidTime: r.ValidTime})
			if err != nil {
				t.Fatalf("%s prepare: %v", r.ID, err)
			}
			result, err := session.ResolveRememberEntity(context.Background(), f.store, proposal, tools.Approved)
			if err != nil {
				t.Fatal(err)
			}
			binding.ClaimID, binding.OperationID, binding.AcceptedAt, binding.SubjectEntityID = result.ClaimID, result.OperationID, result.TransactionTime, proposal.Claim.SubjectEntityID
			for _, proposedAlias := range proposal.Aliases {
				actual, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectAlias, proposedAlias.ID)
				if err != nil || actual.Alias == nil || actual.Alias.Value != proposedAlias.Value {
					t.Fatalf("approved alias changed from its explicit proposal: %s", r.ID)
				}
				binding.Aliases = append(binding.Aliases, *actual.Alias)
			}
			entities[r.Scope+":"+r.Subject.Key], entities[r.Scope+":"+r.Object.Key] = proposal.Claim.SubjectEntityID, proposal.Claim.ObjectEntityID
		case "owner_conversation":
			assistant := "Understood."
			paired := i+1 < len(c.Records) && c.Records[i+1].Kind == "assistant_conversation" && c.Records[i+1].SessionID == r.SessionID
			if paired {
				assistant = c.Records[i+1].Text
			}
			events := integratedConversation(t, f, source, r.Text, assistant)
			binding.Source = events[0]
			if paired {
				i++
				other := c.Records[i]
				seed.Bindings[other.ID] = integratedBinding{RecordID: other.ID, RecordKind: other.Kind, Session: source, Source: events[1]}
			} else {
				seed.Auxiliary = append(seed.Auxiliary, events[1])
			}
		default:
			t.Fatalf("record %s cannot be constructed as %s", r.ID, r.Kind)
		}
		if binding.ClaimID != "" {
			inspected, err := f.store.InspectSemanticObject(context.Background(), source.ScopeContext(), memory.SemanticObjectClaim, binding.ClaimID)
			if err != nil || inspected.Claim == nil || len(inspected.Sources) != 1 {
				t.Fatalf("exact approved source missing: %s: %v", r.ID, err)
			}
			binding.Source = inspected.Sources[0].Source
			if binding.Source.ID == "" || binding.Source.Evidence != r.Text {
				t.Fatalf("exact approved source differs from declared record %s", r.ID)
			}
		}
		seed.Bindings[r.ID] = binding
	}
	for _, r := range c.Records {
		if !r.Retired {
			continue
		}
		b := seed.Bindings[r.ID]
		f.lifecycle(b.Session, "memory_retire", memory.SemanticObjectClaim, b.ClaimID)
		inspected, err := f.store.InspectSemanticObject(context.Background(), b.Session.ScopeContext(), memory.SemanticObjectClaim, b.ClaimID)
		if err != nil || inspected.Status != memory.SemanticStatusRetired {
			t.Fatal("approved retirement did not apply")
		}
		b.Lifecycle = inspected.Lifecycle
		seed.Bindings[r.ID] = b
	}
	seed.Readers["recent"] = newSession(c.ReaderScope)
	seed.Readers["empty"] = newSession(c.ReaderScope)
	reader := seed.Readers["recent"]
	for _, discussion := range c.Recent {
		seed.RecentSources[discussion.ID] = integratedConversation(t, f, reader, discussion.Owner, discussion.Assistant)
	}
	if c.Continuity != "" {
		summary := strings.Replace(validCompactionSummary(), "kept", c.Continuity, 1)
		compactor := &fakeClient{steps: []step{assistantStep(summary, nil)}}
		holder := memory.LeaseHolderID("integrated-compactor-" + string(reader.ID))
		s := NewWithCompactorAndToolset(nil, compactor, testContextProfile("test-model"), f.store.BindHistory(reader.ID, holder), reader.ScopeContext(), f.store.BindTurnOwner(reader.ID, holder), tools.NewToolset(nil), WithAutomaticMemoryRecall(false))
		result, err := s.Compact(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		seed.Compaction = map[string]any{"result": result, "summary": summary, "scripted_compactor_requests": compactor.reqs, "compactor_quality_evaluated": false}
	}
	allSessions := map[memory.SessionID]memory.Session{}
	for _, session := range sessions {
		allSessions[session.ID] = session
	}
	for _, session := range seed.Readers {
		allSessions[session.ID] = session
	}
	var sessionIDs []memory.SessionID
	for id := range allSessions {
		sessionIDs = append(sessionIDs, id)
	}
	slices.Sort(sessionIDs)
	for _, id := range sessionIDs {
		events, err := f.store.LoadEvents(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			public := event.Type == memory.EventUserMessage && event.Role == memory.RoleUser || event.Type == memory.EventAssistantMessage && event.Role == memory.RoleAssistant
			if !public || event.FormatVersion != 1 || event.Content == "" || len(event.Content) > 32768 || !utf8.ValidString(event.Content) || memory.HasRetrievalSecret([]byte(event.Content)) {
				continue
			}
			seed.AllPublicSources = append(seed.AllPublicSources, integratedEventSource(event, allSessions[id]))
		}
	}
	known := time.Now().UTC()
	for id, b := range seed.Bindings {
		ref := memory.RetrievalReference{Kind: memory.RetrievalConversationExcerpt, Intent: memory.RetrievalCurrent, AsKnownAt: known, ValidAt: known, ScopeKey: b.Source.ScopeKey, Status: memory.SemanticStatusActive, CurrentStatus: memory.SemanticStatusActive, Paths: []string{"oracle_exact"}}
		if b.ClaimID != "" {
			ref.Kind = memory.RetrievalAcceptedMemory
			ref.ID = "claim:" + string(b.ClaimID)
			ref.ClaimID = b.ClaimID
			ref.ClaimOperationID = b.OperationID
		} else {
			ref.ID = "excerpt:" + string(b.Source.EventID) + ":" + b.Source.LocatorValue
		}
		ref.Sources = []memory.RetrievalSourceReference{{SourceLinkID: b.Source.ID, SessionID: b.Source.SessionID, ScopeKey: b.Source.ScopeKey, Authority: b.Source.Authority, ObservedAt: b.Source.ObservedAt, EvidenceLocator: memory.EvidenceLocator{EventID: b.Source.EventID, EventPart: b.Source.EventPart, LocatorKind: b.Source.LocatorKind, LocatorValue: b.Source.LocatorValue, EvidenceSHA256: b.Source.EvidenceSHA256}}}
		for _, alias := range b.Aliases {
			if alias.EntityID == b.SubjectEntityID && strings.Contains(c.Question, alias.Value) {
				ref.IdentityMatches = append(ref.IdentityMatches, memory.RetrievalIdentityReference{Kind: "alias", EntityID: alias.EntityID, AliasID: alias.ID})
			}
		}
		if strings.Contains(c.Question, "{{subject_entity_id:"+id+"}}") {
			ref.IdentityMatches = append(ref.IdentityMatches, memory.RetrievalIdentityReference{Kind: "entity", EntityID: b.SubjectEntityID})
		}
		if b.Retired {
			ref.CurrentStatus = memory.SemanticStatusRetired
			ref.Status = memory.SemanticStatusRetired
		}
		if c.Gold.OracleIntent != "" {
			ref.Intent = c.Gold.OracleIntent
		}
		if c.Gold.OracleValidAt != nil {
			ref.ValidAtConstrained = true
			ref.ValidAt = *c.Gold.OracleValidAt
		}
		b.Reference = ref
		seed.Bindings[id] = b
		seed.Question = strings.ReplaceAll(seed.Question, "{{subject_entity_id:"+id+"}}", string(b.SubjectEntityID))
	}
	if strings.Contains(seed.Question, "{{") {
		t.Fatal("unbound question identifier")
	}
	started := time.Now()
	for seed.IndexBatches < 10000 {
		var err error
		seed.IndexCoverage, err = f.store.RefreshMemoryIndex(context.Background(), 256)
		if err != nil {
			t.Fatal(err)
		}
		seed.IndexBatches++
		if seed.IndexCoverage.State == "active" && seed.IndexCoverage.Pending == 0 {
			break
		}
	}
	seed.IndexElapsedNS = time.Since(started).Nanoseconds()
	if seed.IndexCoverage.State != "active" || seed.IndexCoverage.Pending != 0 {
		t.Fatal("canonical fixture index did not reach active coverage")
	}
	if err := f.db.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(f.path + "-wal"); err == nil && info.Size() != 0 {
		t.Fatal("canonical SQLite close left uncheckpointed WAL")
	}
	raw, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	seed.SeedDatabaseSHA256 = memory.CompilerHash(raw)
	if err := os.MkdirAll(destination, 0700); err != nil {
		t.Fatal(err)
	}
	productionReaderRaw(t, destination, "seed.db", raw)
	productionReaderWrite(t, destination, "seed.json", seed)
	return seed
}

func integratedCloneSeed(t *testing.T, directory string, seed integratedSeed) *retrievalFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(directory, "seed.db"))
	if err != nil {
		t.Fatal(err)
	}
	if memory.CompilerHash(raw) != seed.SeedDatabaseSHA256 {
		t.Fatal("immutable canonical database hash changed")
	}
	f := &retrievalFixture{t: t, path: filepath.Join(t.TempDir(), "clone.db")}
	if err := os.WriteFile(f.path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	f.db, err = eviedb.OpenDBAt(f.path)
	if err != nil {
		t.Fatal(err)
	}
	f.store = eviedb.NewStore(f.db)
	t.Cleanup(func() { f.db.Close() })
	return f
}

func TestMemoryStage5IntegratedPrepare(t *testing.T) {
	directory := os.Getenv("EVIE_MEMORY_INTEGRATED_INPUTS")
	if directory == "" {
		t.Skip("canonical integrated fixture preparation is opt-in")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("integrated input directory must be absolute")
	}
	workload := integratedLoadWorkload(t, os.Getenv("EVIE_MEMORY_INTEGRATED_WORKLOAD"))
	for _, c := range workload.Cases {
		t.Run(c.ID, func(t *testing.T) { integratedBuildSeed(t, c, filepath.Join(directory, c.ID)) })
	}
}
