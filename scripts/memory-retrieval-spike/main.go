// memory-retrieval-spike exposes the shipped Kernel as a disposable JSON-line
// experiment process. It never opens an owner database or makes model calls.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

type fixture struct{ ID, Scope, Kind, Text string }
type request struct {
	Action, Scope, Kind, Query string
	IDs                        []string
	Limit, MaxBytes            int
}
type item struct {
	ID       string                   `json:"id"`
	Evidence memory.RetrievalEvidence `json:"evidence"`
}
type response struct {
	Items     []item `json:"items"`
	Bytes     int    `json:"bytes"`
	Error     string `json:"error,omitempty"`
	ElapsedNS int64  `json:"elapsed_ns"`
}
type broker struct {
	store    *eviedb.Store
	readers  map[string]memory.Session
	evidence map[string]memory.RetrievalEvidence
	byClaim  map[memory.SemanticID]string
	byEvent  map[memory.EventID]string
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func key(s memory.Session) string {
	if s.WorkspaceID != "" {
		return "workspace:" + string(s.WorkspaceID)
	}
	if s.ProjectID != "" {
		return "project:" + string(s.ProjectID)
	}
	return "global"
}
func sessionAgent(store *eviedb.Store, s memory.Session) *agent.Session {
	p, err := openrouter.NewExplicitContextProfile("scripted", 131072, 98304, 4096)
	must(err)
	holder := memory.LeaseHolderID("spike-" + string(s.ID))
	return agent.NewWithToolset(nil, p, store.BindHistory(s.ID, holder), s.ScopeContext(), store.BindTurnOwner(s.ID, holder), tools.NewToolset(nil))
}
func appendText(store *eviedb.Store, s memory.Session, text string) memory.Event {
	ctx := context.Background()
	lease, err := store.AcquireTurnLease(ctx, s.ID, "spike-source", time.Minute)
	must(err)
	e, err := store.AppendEventWithLease(ctx, s.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	must(err)
	_, err = store.AppendEventWithLease(ctx, s.ID, lease.HolderID, lease.FencingToken, memory.EventInput{ParentID: e.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Recorded in the synthetic fixture."})
	must(err)
	must(store.ReleaseTurnLease(ctx, s.ID, lease.HolderID, lease.FencingToken))
	return e
}

func (b *broker) addConversation(id string, event memory.Event, s memory.Session) {
	b.byEvent[event.ID] = id
	locator := fmt.Sprintf("0:%d", len(event.Content))
	now := time.Now().UTC()
	actor, authority := memory.SemanticActorOwner, memory.AuthorityOwnerStatement
	if event.Type == memory.EventAssistantMessage {
		actor, authority = "assistant", "none"
	}
	b.evidence[id] = memory.RetrievalEvidence{ID: "excerpt:" + string(event.ID) + ":" + locator, Kind: memory.RetrievalConversationExcerpt, ScopeKey: key(s), Status: memory.SemanticStatusActive, Text: event.Content, AsKnownAt: now, ValidAt: now, Paths: []string{"spike_candidate"}, Sources: []memory.SemanticSource{{EventID: event.ID, SessionID: s.ID, ScopeKey: key(s), EventPart: memory.EvidenceContent, LocatorKind: memory.LocatorUTF8ByteRange, LocatorValue: locator, EvidenceSHA256: memory.CompilerHash([]byte(event.Content)), Actor: actor, SourceType: memory.SemanticSourceType(event.Type), Authority: authority, ObservedAt: event.RecordedAt.UTC().Format(time.RFC3339Nano), Evidence: event.Content, Eligibility: memory.EligibilityEligible}}}
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: broker CORPUS.json FRESH_DIRECTORY")
	}
	var corpus []fixture
	data, err := os.ReadFile(os.Args[1])
	must(err)
	must(json.Unmarshal(data, &corpus))
	if len(corpus) > 2000 {
		panic("fixture exceeds 2000 records")
	}
	path := filepath.Join(os.Args[2], "evie.db")
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		panic("fresh disposable directory required")
	}
	must(os.MkdirAll(os.Args[2], 0700))
	db, err := eviedb.OpenDBAt(path)
	must(err)
	defer db.Close()
	store := eviedb.NewStore(db)
	ctx := context.Background()
	b := broker{store: store, readers: map[string]memory.Session{}, evidence: map[string]memory.RetrievalEvidence{}, byClaim: map[memory.SemanticID]string{}, byEvent: map[memory.EventID]string{}}
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(store))
	must(err)
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID} {
		must(manager.SetEnabled(id, true))
	}
	resolved, err := manager.ResolvePreset(plugins.StandardPresetID)
	must(err)
	sources := map[string]memory.Session{}
	for _, scope := range []string{"global", "general", "workspace", "project_a", "project_b"} {
		var source, reader memory.Session
		switch scope {
		case "global":
			source, err = store.CreateGlobalSession(ctx)
			must(err)
			reader, err = store.CreateGlobalSession(ctx)
		case "general", "workspace":
			w, e := store.RegisterWorkspace(ctx, scope)
			must(e)
			source, err = store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, resolved.Receipt)
			must(err)
			reader, err = store.CreateWorkspaceSessionWithComposition(ctx, w.ID, w.CurrentRevisionID, resolved.Receipt)
		default:
			must(os.MkdirAll(filepath.Join(os.Args[2], scope), 0700))
			p, e := store.RegisterProject(ctx, scope, filepath.Join(os.Args[2], scope))
			must(e)
			source, err = store.CreateProjectSession(ctx, p.ID)
			must(err)
			reader, err = store.CreateProjectSession(ctx, p.ID)
		}
		must(err)
		sources[scope] = source
		b.readers[scope] = reader
	}
	claimScopes := map[memory.SemanticID]string{}
	for _, record := range corpus {
		s, ok := sources[record.Scope]
		if !ok || record.ID == "" {
			panic("invalid fixture scope/id")
		}
		var event memory.Event
		if record.Kind == "claim" {
			a := sessionAgent(store, s)
			p, e := a.PrepareRememberLiteral(ctx, store, record.Text, memory.RememberLiteralRequest{IdempotencyKey: "idem:v1:" + uuid.NewString(), Predicate: "spike_preference", PredicateLabel: "preference", PredicateCardinality: memory.CardinalityMany, Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: record.Text}, Polarity: memory.PolarityAffirmed})
			must(e)
			r, e := a.ResolveRememberLiteral(ctx, store, p, tools.Approved)
			must(e)
			b.byClaim[r.ClaimID] = record.ID + "/claim"
			claimScopes[r.ClaimID] = record.Scope
			events, e := store.LoadEvents(ctx, s.ID)
			must(e)
			for _, candidate := range events {
				if candidate.ID == p.Source.EventID {
					event = candidate
					break
				}
			}
		} else if record.Kind == "conversation" {
			event = appendText(store, s, record.Text)
		} else {
			panic("invalid fixture kind")
		}
		b.addConversation(record.ID+"/excerpt", event, s)
	}
	for scope, s := range sources {
		events, e := store.LoadEvents(ctx, s.ID)
		must(e)
		for _, event := range events {
			if (event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage) && b.byEvent[event.ID] == "" {
				b.addConversation(fmt.Sprintf("aux/%s/%d", scope, event.Sequence), event, s)
			}
		}
	}
	for i := 0; i < 100; i++ {
		coverage, e := store.RefreshMemoryIndex(ctx, 256)
		must(e)
		if coverage.State == "active" && coverage.Pending == 0 {
			break
		}
		if i == 99 {
			panic("incomplete fixture backfill")
		}
	}
	for claim, id := range b.byClaim {
		result, e := store.SearchMemory(ctx, b.readers[claimScopes[claim]].ScopeContext(), memory.RetrievalQuery{Text: string(claim)})
		must(e)
		for _, evidence := range result.Evidence {
			if evidence.ClaimID == claim {
				b.evidence[id] = evidence
			}
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	for scanner.Scan() {
		var r request
		if err = json.Unmarshal(scanner.Bytes(), &r); err != nil {
			must(encoder.Encode(response{Error: err.Error()}))
			continue
		}
		started := time.Now()
		out, e := b.run(r)
		out.ElapsedNS = time.Since(started).Nanoseconds()
		if e != nil {
			out.Error = e.Error()
		}
		must(encoder.Encode(out))
	}
	must(scanner.Err())
}

func (b *broker) run(r request) (response, error) {
	out := response{Items: []item{}}
	s, ok := b.readers[r.Scope]
	if !ok {
		return out, fmt.Errorf("unknown actor")
	}
	ctx := context.Background()
	if r.Limit == 0 {
		r.Limit = 4
	}
	if r.MaxBytes == 0 {
		r.MaxBytes = 8192
	}
	if r.Limit < 1 || r.Limit > 8 || r.MaxBytes < 512 || r.MaxBytes > 24576 {
		return out, fmt.Errorf("invalid budget")
	}
	var evidence []memory.RetrievalEvidence
	if r.Action == "search" {
		found, err := b.store.SearchMemory(ctx, s.ScopeContext(), memory.RetrievalQuery{Kind: r.Kind, Text: r.Query, Limit: r.Limit, MaxBytes: r.MaxBytes})
		if err != nil {
			return out, err
		}
		evidence = found.Evidence
	} else if r.Action == "corpus" || r.Action == "validate" {
		var candidates []memory.RetrievalEvidence
		if r.Action == "corpus" {
			for _, e := range b.evidence {
				if r.Kind == "" || e.Kind == r.Kind {
					candidates = append(candidates, e)
				}
			}
		} else {
			if len(r.IDs) > 64 {
				return out, fmt.Errorf("candidate bound")
			}
			for _, id := range r.IDs {
				e, ok := b.evidence[id]
				if !ok {
					return out, fmt.Errorf("unknown candidate")
				}
				candidates = append(candidates, e)
			}
		}
		for start := 0; start < len(candidates); start += 8 {
			valid, err := b.store.RevalidateMemoryEvidence(ctx, s.ScopeContext(), candidates[start:min(start+8, len(candidates))])
			if err != nil {
				return out, err
			}
			evidence = append(evidence, valid...)
		}
	} else {
		return out, fmt.Errorf("unknown action")
	}
	for _, e := range evidence {
		id := b.byClaim[e.ClaimID]
		if e.Kind == memory.RetrievalConversationExcerpt && len(e.Sources) == 1 {
			id = b.byEvent[e.Sources[0].EventID]
		}
		if id == "" {
			return out, fmt.Errorf("unmapped Kernel evidence")
		}
		if memory.HasRetrievalSecret([]byte(e.Text)) {
			return out, fmt.Errorf("Kernel exposed excluded evidence")
		}
		out.Items = append(out.Items, item{ID: id, Evidence: e})
		if r.Action != "corpus" {
			payload, _ := json.Marshal(out.Items)
			if len("EVIE_MEMORY_DATA\n")+len(payload) > r.MaxBytes {
				out.Items = out.Items[:len(out.Items)-1]
				break
			}
			if len(out.Items) == r.Limit {
				break
			}
		}
	}
	payload, err := json.Marshal(out.Items)
	out.Bytes = len("EVIE_MEMORY_DATA\n") + len(payload)
	return out, err
}
