package memoryeval_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/memoryeval"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/google/uuid"
)

// The scale replay drives real agent turns against real SQLite, the Memory
// Plugin and the production retrieval path. Only the conversational provider
// (scripted) and, in the large tier, the local embedding endpoint (a loopback
// deterministic fake) are replaced. Nothing here reads a real Evie database.
const (
	scaleUpdateEnv = "EVIE_MEMORY_SCALE_UPDATE"
	scaleTierEnv   = "EVIE_MEMORY_SCALE_EVAL"
	// Mirrors the production dense scan budget (eviedb denseVectorScanBudget)
	// so the report can say which targets a budgeted scan could reach.
	scaleDenseScanLimit = 65536
	// The single-page bound before Stage 12; the large tier is sized past it.
	scaleLegacyDenseScanBound = 4096
)

func TestMemoryScaleReplayDefaultTier(t *testing.T) {
	if testing.Short() {
		t.Skip("scale replay skipped in -short mode")
	}
	runScaleTier(t, memoryeval.DefaultScaleCorpusOptions(), filepath.Join("testdata", "scale-baseline-default.json"), false)
}

func TestMemoryScaleReplayLargeTier(t *testing.T) {
	if os.Getenv(scaleTierEnv) != memoryeval.ScaleTierLarge {
		t.Skipf("set %s=%s to run the dense tier beyond %d vectors", scaleTierEnv, memoryeval.ScaleTierLarge, scaleLegacyDenseScanBound)
	}
	runScaleTier(t, memoryeval.LargeScaleCorpusOptions(), filepath.Join("testdata", "scale-baseline-large.json"), true)
}

func runScaleTier(t *testing.T, options memoryeval.ScaleCorpusOptions, baselinePath string, dense bool) {
	if memoryeval.ScaleGapDenseScan != memory.RetrievalGapDenseScan {
		t.Fatal("the scorer's dense scan gap no longer mirrors the production signal")
	}
	corpus := memoryeval.GenerateScaleCorpus(options)
	if err := corpus.Validate(); err != nil {
		t.Fatalf("invalid generated corpus: %v", err)
	}
	// Production IDs are random UUIDs, and some orderings (the dense scan in
	// particular) follow them. A seeded source makes every run reproduce the
	// same IDs for the same code; it changes no production behavior.
	var seed [32]byte
	binary.LittleEndian.PutUint64(seed[:], options.Seed)
	uuid.SetRand(rand.NewChaCha8(seed))
	t.Cleanup(func() { uuid.SetRand(nil) })
	t.Setenv("EVIE_REMOTE_MEMORY", "on")
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	if dense {
		t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", newScaleEmbedder(t, corpus.ConceptOf()).URL)
	}
	replay := newScaleReplay(t)
	started := time.Now()
	replay.build(corpus)
	built := time.Since(started)
	started = time.Now()
	replay.refresh()
	indexed := time.Since(started)
	if dense {
		replay.measureDenseIndex()
	}
	replay.freeze()
	started = time.Now()
	var observations []memoryeval.ScaleObservation
	for _, probe := range corpus.Probes {
		observations = append(observations, replay.probe(probe))
	}
	probed := time.Since(started)
	report := memoryeval.ScoreScale(corpus, observations)
	if dense {
		replay.denseMetrics(corpus, observations, &report)
	}
	t.Logf("timing (not part of the baseline): build=%s index=%s probes=%s", built.Round(time.Millisecond), indexed.Round(time.Millisecond), probed.Round(time.Millisecond))
	t.Logf("\n%s", report.Markdown())
	if report.ScopeLeaks != 0 || report.Unmapped != 0 {
		t.Errorf("hard boundary violated: scope leaks=%d unmapped items=%d", report.ScopeLeaks, report.Unmapped)
	}
	// Controls use wording the current implementation already handles. On the
	// model-directed tool paths they must pass, or the stale-fact measurement
	// itself is wrong. Automatic outcomes are recorded, not asserted.
	for _, outcome := range report.Stale.Outcomes {
		if outcome.Control && outcome.Path != memoryeval.ScalePathAutomatic && outcome.Miss {
			t.Errorf("instrument control %s failed (%s); the stale-fact measurement is not trustworthy", outcome.ProbeID, outcome.Outcome)
		}
	}
	var unmet []string
	for _, target := range report.Targets {
		if !target.Met {
			unmet = append(unmet, target.ID+" ("+target.Observed+")")
		}
	}
	if len(unmet) > 0 {
		t.Logf("KNOWN UNMET TARGETS recorded in the baseline, not passing behavior:\n  %s", strings.Join(unmet, "\n  "))
	}
	compareScaleBaseline(t, baselinePath, report)
}

// compareScaleBaseline is a ratchet: any change, better or worse, must update
// the committed baseline in the same change so the diff shows before/after.
func compareScaleBaseline(t *testing.T, path string, report memoryeval.ScaleReport) {
	t.Helper()
	got, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if os.Getenv(scaleUpdateEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read baseline %s: %v (run with %s=1 to record it)", path, err, scaleUpdateEnv)
	}
	if bytes.Equal(want, got) {
		return
	}
	wantLines, gotLines := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	var removed, added []string
	counts := map[string]int{}
	for _, line := range gotLines {
		counts[line]++
	}
	for _, line := range wantLines {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		removed = append(removed, strings.TrimSpace(line))
	}
	counts = map[string]int{}
	for _, line := range wantLines {
		counts[line]++
	}
	for _, line := range gotLines {
		if counts[line] > 0 {
			counts[line]--
			continue
		}
		added = append(added, strings.TrimSpace(line))
	}
	limit := func(lines []string) string {
		if len(lines) > 40 {
			lines = append(lines[:40:40], fmt.Sprintf("... %d more", len(lines)-40))
		}
		return strings.Join(lines, "\n  ")
	}
	t.Errorf("scale evaluation differs from %s.\nOnly in baseline:\n  %s\nOnly in this run:\n  %s\nIf the change is intended, rerun with %s=1 and commit the new baseline with the behavior change.",
		path, limit(removed), limit(added), scaleUpdateEnv)
}

type scaleReplay struct {
	t        *testing.T
	ctx      context.Context
	path     string // built history; frozen before probing
	probes   int
	db       *sql.DB
	store    *eviedb.Store
	profile  openrouter.ContextProfile
	projects map[string]memory.ProjectID // area key -> project
	areas    map[string]string           // scope key -> area key
	sessions map[memory.SessionID]string // session -> area key
	events   map[memory.EventID]string   // event -> corpus key
	claims   map[memory.SemanticID]string
	claimIDs map[string]memory.RememberLiteralProposal
	// Dense index layout of the built history, in scan order.
	densePosition map[memory.EventID]int
	denseVectors  int
}

func newScaleReplay(t *testing.T) *scaleReplay {
	t.Helper()
	profile, err := openrouter.NewExplicitContextProfile("test-model", 300000, 262144, 16384)
	if err != nil {
		t.Fatal(err)
	}
	r := &scaleReplay{t: t, ctx: context.Background(), path: filepath.Join(t.TempDir(), "history.db"), profile: profile,
		projects: map[string]memory.ProjectID{}, areas: map[string]string{memoryeval.ScaleAreaGlobal: memoryeval.ScaleAreaGlobal},
		sessions: map[memory.SessionID]string{}, events: map[memory.EventID]string{}, claims: map[memory.SemanticID]string{},
		claimIDs: map[string]memory.RememberLiteralProposal{}}
	r.open(r.path)
	return r
}

func (r *scaleReplay) open(path string) {
	r.t.Helper()
	db, err := eviedb.OpenDBAt(path)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(func() { db.Close() })
	r.db, r.store = db, eviedb.NewStore(db)
}

// freeze checkpoints and closes the built history. Every probe then runs on
// its own copy: new events are indexed as they are appended, so probing the
// shared database would let one probe's messages become another's evidence.
func (r *scaleReplay) freeze() {
	r.t.Helper()
	if _, err := r.db.ExecContext(r.ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		r.t.Fatal(err)
	}
	if err := r.db.Close(); err != nil {
		r.t.Fatal(err)
	}
	r.db, r.store = nil, nil
}

func (r *scaleReplay) thaw() func() {
	r.t.Helper()
	r.probes++
	data, err := os.ReadFile(r.path)
	if err != nil {
		r.t.Fatal(err)
	}
	copyPath := filepath.Join(filepath.Dir(r.path), fmt.Sprintf("probe-%03d.db", r.probes))
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		r.t.Fatal(err)
	}
	r.open(copyPath)
	return func() {
		if err := r.db.Close(); err != nil {
			r.t.Fatal(err)
		}
		r.db, r.store = nil, nil
		for _, suffix := range []string{"", "-wal", "-shm"} {
			os.Remove(copyPath + suffix)
		}
	}
}

func (r *scaleReplay) newSession(area string) memory.Session {
	r.t.Helper()
	var record memory.Session
	var err error
	if area == memoryeval.ScaleAreaGlobal {
		record, err = r.store.CreateGlobalSession(r.ctx)
	} else {
		project, ok := r.projects[area]
		if !ok {
			r.t.Fatalf("unknown project area %s", area)
		}
		record, err = r.store.CreateProjectSession(r.ctx, project)
	}
	if err != nil {
		r.t.Fatal(err)
	}
	r.sessions[record.ID] = area
	return record
}

func (r *scaleReplay) agent(record memory.Session, client *scaleClient, automatic bool) *agent.Session {
	var definitions []tools.Tool
	for _, capability := range plugins.NewMemory(r.store).ToolCapabilities() {
		definitions = append(definitions, capability.Tool)
	}
	holder := memory.LeaseHolderID("scale-" + string(record.ID))
	return agent.NewWithToolset(client, r.profile, r.store.BindHistory(record.ID, holder), record.ScopeContext(), r.store.BindTurnOwner(record.ID, holder), tools.NewToolset(definitions), agent.WithAutomaticMemoryRecall(automatic))
}

func (r *scaleReplay) build(corpus memoryeval.ScaleCorpus) {
	r.t.Helper()
	for _, area := range corpus.Areas {
		if area.Key == memoryeval.ScaleAreaGlobal {
			continue
		}
		project, err := r.store.RegisterProject(r.ctx, area.Name, r.t.TempDir())
		if err != nil {
			r.t.Fatal(err)
		}
		r.projects[area.Key] = project.ID
		r.areas["project:"+string(project.ID)] = area.Key
	}
	for _, spec := range corpus.Sessions {
		record := r.newSession(spec.Area)
		client := &scaleClient{}
		// History is built without automatic recall: its injections are not
		// measured here and would only slow the build.
		session := r.agent(record, client, false)
		mapped := 0
		for _, step := range spec.Steps {
			switch step.Kind {
			case memoryeval.ScaleStepExchange:
				client.script(scaleAnswer(step.Assistant.Text))
				if err := session.Send(r.ctx, step.Owner.Text, scaleEvents{}, nil); err != nil {
					r.t.Fatalf("replay %s: %v", step.Owner.Key, err)
				}
			case memoryeval.ScaleStepRemember:
				claim, _ := corpus.Claim(step.Claim)
				proposal, err := session.PrepareRememberLiteral(r.ctx, r.store, step.Owner.Text, memory.RememberLiteralRequest{
					IdempotencyKey: step.IdempotencyKey, Predicate: claim.Predicate, PredicateLabel: claim.Label, PredicateCardinality: memory.PredicateCardinality(claim.Cardinality),
					Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: claim.Value}, Polarity: memory.PolarityAffirmed})
				if err != nil {
					r.t.Fatalf("prepare %s: %v", step.Claim, err)
				}
				if _, err := session.ResolveRememberLiteral(r.ctx, r.store, proposal, tools.Approved); err != nil {
					r.t.Fatalf("accept %s: %v", step.Claim, err)
				}
				r.claimIDs[step.Claim] = proposal
				r.claims[proposal.ClaimID] = step.Claim
			case memoryeval.ScaleStepCorrect:
				claim, _ := corpus.Claim(step.Claim)
				old := r.claimIDs[step.Target]
				// A real-world change closes the old interval now; an error
				// inherits the old validity interval and takes no effective time.
				var effective *time.Time
				if step.Mode == string(memory.CorrectionChanged) {
					now := time.Now().UTC()
					effective = &now
				}
				proposal, err := session.PrepareCorrectClaim(r.ctx, r.store, step.Owner.Text, memory.CorrectClaimRequest{
					IdempotencyKey: step.IdempotencyKey, OldClaimID: old.ClaimID, Mode: memory.CorrectionMode(step.Mode), EffectiveTime: effective,
					Replacement: memory.ClaimProposition{SubjectEntityID: old.Subject.ID, PredicateID: old.Predicate.ID, Polarity: memory.PolarityAffirmed,
						Object: memory.ClaimObject{Literal: &memory.TypedLiteral{Kind: memory.LiteralText, Value: claim.Value}}}})
				if err != nil {
					r.t.Fatalf("prepare correction %s: %v", step.Claim, err)
				}
				result, err := session.ResolveCorrectClaim(r.ctx, r.store, proposal, tools.Approved)
				if err != nil {
					r.t.Fatalf("apply correction %s: %v", step.Claim, err)
				}
				r.claims[result.ReplacementClaimID] = step.Claim
			case memoryeval.ScaleStepRetire:
				target := r.claimIDs[step.Target]
				args, _ := json.Marshal(map[string]string{"idempotency_key": step.IdempotencyKey, "object_kind": string(memory.SemanticObjectClaim), "object_id": string(target.ClaimID)})
				client.script(scaleToolCalls(openrouter.ToolCall{ID: "retire", Type: "function", Function: openrouter.FunctionCall{Name: "memory_retire", Arguments: string(args)}}), scaleAnswer(step.Assistant.Text))
				approve := func(context.Context, string, string, *tools.FileChangePreview) tools.Decision { return tools.Approved }
				if err := session.Send(r.ctx, step.Owner.Text, scaleEvents{}, approve); err != nil {
					r.t.Fatalf("retire %s: %v", step.Target, err)
				}
				inspected, err := r.store.InspectSemanticObject(r.ctx, record.ScopeContext(), memory.SemanticObjectClaim, target.ClaimID)
				if err != nil || inspected.Status != memory.SemanticStatusRetired {
					r.t.Fatalf("retire %s did not apply: %+v %v", step.Target, inspected.Status, err)
				}
			}
			mapped = r.mapEvents(record, step, mapped)
		}
	}
}

// mapEvents assigns corpus keys to the conversation events a step produced.
// Only non-empty user and assistant messages are conversation evidence.
func (r *scaleReplay) mapEvents(record memory.Session, step memoryeval.ScaleStep, from int) int {
	r.t.Helper()
	events, err := r.store.LoadEvents(r.ctx, record.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	users, assistants := 0, 0
	for _, event := range events[from:] {
		switch {
		case event.Type == memory.EventUserMessage && event.Role == memory.RoleUser:
			users++
			r.events[event.ID] = step.Owner.Key
		case event.Type == memory.EventAssistantMessage && event.Content != "":
			assistants++
			if step.Assistant == nil {
				r.t.Fatalf("step %s produced unexpected assistant text %q", step.Owner.Key, event.Content)
			}
			r.events[event.ID] = step.Assistant.Key
		}
	}
	if users != 1 || assistants > 1 {
		r.t.Fatalf("step %s produced %d owner and %d assistant messages", step.Owner.Key, users, assistants)
	}
	return len(events)
}

func (r *scaleReplay) refresh() {
	r.t.Helper()
	for i := 0; i < 100000; i++ {
		coverage, err := r.store.RefreshMemoryIndex(r.ctx, 256)
		if err != nil {
			r.t.Fatal(err)
		}
		if coverage.State == "active" && coverage.Pending == 0 {
			return
		}
	}
	r.t.Fatal("index backfill did not complete")
}

// probe runs one fresh-session turn on its own copy of the built history, so
// one probe's messages can never become evidence for another.
func (r *scaleReplay) probe(probe memoryeval.ScaleProbe) memoryeval.ScaleObservation {
	r.t.Helper()
	defer r.thaw()()
	record := r.newSession(probe.Area)
	client := &scaleClient{}
	session := r.agent(record, client, probe.Path == memoryeval.ScalePathAutomatic)
	for _, prelude := range probe.Prelude {
		client.script(scaleAnswer("Here is a short answer to that."))
		if err := session.Send(r.ctx, prelude, scaleEvents{}, nil); err != nil {
			r.t.Fatalf("probe %s prelude: %v", probe.ID, err)
		}
	}
	// Earlier messages of the probe's own session are already in the provider
	// request; label them so re-injection is visible rather than unmapped.
	events, err := r.store.LoadEvents(r.ctx, record.ID)
	if err != nil {
		r.t.Fatal(err)
	}
	for i, event := range events {
		if event.Type == memory.EventUserMessage || event.Type == memory.EventAssistantMessage {
			r.events[event.ID] = fmt.Sprintf("%s%s.%d", memoryeval.ScaleSessionKeyPrefix, event.Type, i)
		}
	}
	observation := memoryeval.ScaleObservation{ProbeID: probe.ID, Items: []memoryeval.ScaleItem{}}
	var request openrouter.ChatRequest
	if probe.Path == memoryeval.ScalePathAutomatic {
		client.script(scaleAnswer("Answered with the supplied context."))
		if err := session.Send(r.ctx, probe.Message, scaleEvents{}, nil); err != nil {
			r.t.Fatalf("probe %s: %v", probe.ID, err)
		}
		request = client.requests[0]
	} else {
		args, _ := json.Marshal(map[string]string{"query": probe.Query})
		client.script(scaleToolCalls(openrouter.ToolCall{ID: "lookup", Type: "function", Function: openrouter.FunctionCall{Name: probe.Path, Arguments: string(args)}}), scaleAnswer("Answered with the retrieved evidence."))
		if err := session.Send(r.ctx, probe.Message, scaleEvents{}, nil); err != nil {
			r.t.Fatalf("probe %s: %v", probe.ID, err)
		}
		if len(client.requests) != 2 {
			r.t.Fatalf("probe %s made %d provider requests", probe.ID, len(client.requests))
		}
		request = client.requests[1]
		for _, message := range request.Messages {
			if message.Role != "tool" || message.ToolCallID != "lookup" {
				continue
			}
			// The model-visible outcome is framed metadata: status, match count,
			// lexical coverage, a truncation flag and any coverage gaps. Dense
			// coverage itself is not exposed; "partial" alone cannot tell a scan
			// cut from pending index work, so the distinct gap is read too.
			lines := strings.Split(message.Content, "\n")
			var result struct {
				Status string   `json:"status"`
				Gaps   []string `json:"gaps"`
			}
			if len(lines) != 3 || json.Unmarshal([]byte(lines[1]), &result) != nil {
				r.t.Fatalf("probe %s tool outcome has an unexpected frame: %q", probe.ID, message.Content)
			}
			observation.ToolStatus = append(observation.ToolStatus, result.Status)
			observation.ToolPartial = result.Status == memory.RetrievalPartial
			observation.Gaps = scaleAddGaps(observation.Gaps, result.Gaps)
		}
	}
	data, ok := scaleMemoryData(request)
	if !ok {
		observation.Status = "none"
		return observation
	}
	var payload struct {
		Status   string                     `json:"status"`
		Gaps     []string                   `json:"gaps"`
		Evidence []memory.RetrievalEvidence `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, "EVIE_MEMORY_DATA\n")), &payload); err != nil {
		r.t.Fatalf("probe %s memory data: %v", probe.ID, err)
	}
	observation.Status = payload.Status
	observation.Gaps = scaleAddGaps(observation.Gaps, payload.Gaps)
	for _, evidence := range payload.Evidence {
		observation.Items = append(observation.Items, r.item(evidence))
	}
	return observation
}

func (r *scaleReplay) item(e memory.RetrievalEvidence) memoryeval.ScaleItem {
	item := memoryeval.ScaleItem{Kind: e.Kind, Intent: e.Intent, Status: string(e.Status), CurrentStatus: string(e.CurrentStatus), Paths: e.Paths}
	if e.Kind == memory.RetrievalAcceptedMemory {
		item.Key = r.claims[e.ClaimID]
	} else if len(e.Sources) > 0 {
		item.Key = r.events[e.Sources[0].EventID]
	}
	item.Area = r.areas[e.ScopeKey]
	if item.Area == "" && len(e.Sources) > 0 {
		item.Area = r.sessions[e.Sources[0].SessionID]
	}
	if item.Area == "" {
		item.Area = "unknown:" + e.ScopeKey
	}
	for _, id := range e.RelatedClaimIDs {
		item.RelatedKeys = append(item.RelatedKeys, r.claims[id])
	}
	for _, conflict := range e.Conflicts {
		for _, id := range conflict.ClaimIDs {
			if id != e.ClaimID && !slices.Contains(item.ConflictKeys, r.claims[id]) {
				item.ConflictKeys = append(item.ConflictKeys, r.claims[id])
			}
		}
	}
	for _, link := range e.HistoricalClaims {
		for _, id := range []memory.SemanticID{link.ClaimID, link.ReplacementClaimID} {
			if id != "" && !slices.Contains(item.HistoricalKeys, r.claims[id]) {
				item.HistoricalKeys = append(item.HistoricalKeys, r.claims[id])
			}
		}
	}
	return item
}

// measureDenseIndex records the built history's Global vector layout in the
// order the bounded dense scan reads it. It reads the derived index table
// directly; this is measurement, not a production read path.
func (r *scaleReplay) measureDenseIndex() {
	r.t.Helper()
	rows, err := r.db.QueryContext(r.ctx, `SELECT event_id FROM memory_dense_event_vectors WHERE scope_key='global' ORDER BY event_id,byte_start`)
	if err != nil {
		r.t.Fatal(err)
	}
	defer rows.Close()
	r.densePosition = map[memory.EventID]int{}
	for rows.Next() {
		var id memory.EventID
		if err := rows.Scan(&id); err != nil {
			r.t.Fatal(err)
		}
		if _, seen := r.densePosition[id]; !seen {
			r.densePosition[id] = r.denseVectors
		}
		r.denseVectors++
	}
	if err := rows.Err(); err != nil {
		r.t.Fatal(err)
	}
}

// denseMetrics adds what only the built database can say: how many vectors
// share the Global scope and which targets fall inside the bounded scan.
func (r *scaleReplay) denseMetrics(corpus memoryeval.ScaleCorpus, observations []memoryeval.ScaleObservation, report *memoryeval.ScaleReport) {
	r.t.Helper()
	if report.Dense == nil {
		r.t.Fatal("large tier produced no dense coverage probes")
	}
	report.Dense.VectorsInScope = r.denseVectors
	report.Dense.ScanLimit = scaleDenseScanLimit
	fraction := 1.0
	if r.denseVectors > scaleDenseScanLimit {
		fraction = float64(int(float64(scaleDenseScanLimit)/float64(r.denseVectors)*10000)) / 10000
	}
	report.Dense.ReachableFraction = &fraction
	keyEvent := map[string]memory.EventID{}
	for id, key := range r.events {
		keyEvent[key] = id
	}
	found := map[string]bool{}
	for _, observation := range observations {
		for _, item := range observation.Items {
			found[observation.ProbeID+"|"+item.Key] = true
		}
	}
	for _, probe := range corpus.Probes {
		if probe.Family != memoryeval.ScaleFamilyDenseCoverage {
			continue
		}
		target := probe.Required[0]
		at, ok := r.densePosition[keyEvent[target]]
		reachable := ok && at < scaleDenseScanLimit
		if reachable {
			report.Dense.TargetsReachable++
		}
		if found[probe.ID+"|"+target] {
			if reachable {
				report.Dense.ReachableFound++
			} else {
				report.Dense.UnreachableFound++
			}
		}
	}
	report.Targets = memoryeval.ScaleTargetsFor(*report)
}

func scaleAddGaps(gaps, more []string) []string {
	for _, gap := range more {
		if !slices.Contains(gaps, gap) {
			gaps = append(gaps, gap)
		}
	}
	slices.Sort(gaps)
	return gaps
}

func scaleMemoryData(request openrouter.ChatRequest) (string, bool) {
	for _, message := range request.Messages {
		if message.Role == "user" && strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
			return message.Content, true
		}
	}
	return "", false
}

type scaleClient struct {
	responses []openrouter.ChatResponse
	requests  []openrouter.ChatRequest
}

func (c *scaleClient) script(responses ...openrouter.ChatResponse) {
	c.responses, c.requests = responses, nil
}

func (c *scaleClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	c.requests = append(c.requests, request)
	if len(c.responses) == 0 {
		return openrouter.ChatResponse{}, errors.New("scale replay: no scripted provider response left")
	}
	response := c.responses[0]
	c.responses = c.responses[1:]
	return response, nil
}

func scaleAnswer(text string) openrouter.ChatResponse {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: text}}}}
}

func scaleToolCalls(calls ...openrouter.ToolCall) openrouter.ChatResponse {
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", ToolCalls: calls}}}}
}

type scaleEvents struct{}

func (scaleEvents) Delta(string)                                  {}
func (scaleEvents) Reasoning(string)                              {}
func (scaleEvents) ReasoningDone()                                {}
func (scaleEvents) AssistantDone(string)                          {}
func (scaleEvents) ToolCall(string, string, string)               {}
func (scaleEvents) ToolResult(string, string, bool)               {}
func (scaleEvents) ResponseDiscarded(agent.DiscardReason, string) {}

// newScaleEmbedder serves the local embedding protocol on loopback, as the
// existing dense acceptance tests do. Each concept maps to a fixed
// pseudo-random ±1 direction; a text is the normalized sum of its concepts, so
// unrelated texts are nearly orthogonal and shared concepts raise similarity.
func newScaleEmbedder(t *testing.T, concepts map[string]string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	cache := map[string][]float32{}
	direction := func(concept string) []float32 {
		mu.Lock()
		defer mu.Unlock()
		if vector, ok := cache[concept]; ok {
			return vector
		}
		h := fnv.New64a()
		h.Write([]byte(concept))
		source := rand.New(rand.NewPCG(h.Sum64(), 0x5ca1ab1e))
		vector := make([]float32, 384)
		for i := range vector {
			vector[i] = float32(1 - 2*int(source.Uint64()&1))
		}
		cache[concept] = vector
		return vector
	}
	embed := func(text string) []float32 {
		vector := make([]float32, 384)
		used := false
		for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len(word) < 3 || scaleStopWords[word] {
				continue
			}
			concept := word
			if mapped, ok := concepts[word]; ok {
				concept = mapped
			}
			for i, value := range direction(concept) {
				vector[i] += value
			}
			used = true
		}
		if !used {
			vector[0] = 1
		}
		return vector
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{{"name": "all-minilm:22m", "model": "all-minilm:22m", "digest": "1b226e2802dbb772b5fc32a58f103ca1804ef7501331012de126ab22f67475ef"}}})
		case "/api/embed":
			var request struct {
				Model string   `json:"model"`
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&request); err != nil {
				http.Error(w, "invalid bounded request", http.StatusBadRequest)
				return
			}
			embeddings := make([][]float32, 0, len(request.Input))
			for _, text := range request.Input {
				embeddings = append(embeddings, embed(text))
			}
			json.NewEncoder(w).Encode(map[string]any{"model": request.Model, "embeddings": embeddings})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

var scaleStopWords = map[string]bool{}

func init() {
	for _, word := range strings.Fields(`the and for with that this you your are was were have has had but not what when where which who how did does can should would could about from into there their they them then than its our out all any get got let just still every each some more most much very too also been being will may might must his her she him one two three four six eight over under again only own same such few both other here why now new old make made like want need know think good great thanks okay sure please stored`) {
		scaleStopWords[word] = true
	}
}
