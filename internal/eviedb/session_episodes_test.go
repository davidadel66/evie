package eviedb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
)

type episodeFixture struct {
	t      *testing.T
	path   string
	db     *sql.DB
	store  *eviedb.Store
	holder memory.LeaseHolderID
}

func newEpisodeFixture(t *testing.T) *episodeFixture {
	t.Helper()
	f := &episodeFixture{t: t, path: filepath.Join(t.TempDir(), "evie.db"), holder: "episode-fixture"}
	f.open()
	t.Cleanup(func() { f.db.Close() })
	return f
}

func (f *episodeFixture) open() {
	f.t.Helper()
	db, err := eviedb.OpenDBAt(f.path)
	if err != nil {
		f.t.Fatal(err)
	}
	f.db, f.store = db, eviedb.NewStore(db)
}

func (f *episodeFixture) reopen() {
	f.t.Helper()
	if err := f.db.Close(); err != nil {
		f.t.Fatal(err)
	}
	f.open()
}

func (f *episodeFixture) turn(session memory.Session, text string) {
	f.t.Helper()
	ctx := context.Background()
	history := f.store.BindHistory(session.ID, f.holder)
	lease, err := f.store.AcquireTurnLease(ctx, session.ID, f.holder, time.Minute)
	if err != nil {
		f.t.Fatal(err)
	}
	root, err := history.Append(ctx, lease, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: text})
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err = history.Append(ctx, lease, memory.EventInput{ParentID: root.ID, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: "Noted.", Payload: json.RawMessage(`{}`)}); err != nil {
		f.t.Fatal(err)
	}
	if err = f.store.ReleaseTurnLease(ctx, session.ID, f.holder, lease.FencingToken); err != nil {
		f.t.Fatal(err)
	}
}

// compact runs a real manual compaction with a scripted compactor.
func (f *episodeFixture) compact(session memory.Session, state string) memory.EventID {
	f.t.Helper()
	var summary strings.Builder
	for _, heading := range memory.ContextCompactionSectionHeadings() {
		body := "kept"
		if heading == "Current state / completed actions" {
			body = state
		}
		fmt.Fprintf(&summary, "## %s\n%s\n\n", heading, body)
	}
	compactor := &compactionAcceptanceClient{responses: []openrouter.ChatResponse{compactionAcceptanceResponse(summary.String())}}
	runtime := agent.NewWithCompactor(compactor, compactor, compactionAcceptanceProfile(f.t, "test/model", 262144),
		f.store.BindHistory(session.ID, f.holder), session.ScopeContext(), f.store.BindTurnOwner(session.ID, f.holder))
	result, err := runtime.Compact(context.Background())
	if err != nil {
		f.t.Fatalf("compact %s: %v", session.ID, err)
	}
	return result.CompactionEventID
}

func (f *episodeFixture) search(reader memory.Session, text string) memory.SessionEpisodeResult {
	f.t.Helper()
	result, err := f.store.SearchSessionEpisodes(context.Background(), reader.ScopeContext(), memory.SessionEpisodeQuery{Text: text, Limit: 5})
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func episodeSessions(result memory.SessionEpisodeResult) map[memory.SessionID]memory.SessionEpisode {
	found := make(map[memory.SessionID]memory.SessionEpisode, len(result.Episodes))
	for _, episode := range result.Episodes {
		found[episode.SessionID] = episode
	}
	return found
}

func TestSessionEpisodesBackfillOnStartupAndReconcileStaleRows(t *testing.T) {
	f := newEpisodeFixture(t)
	ctx := context.Background()
	workspace, err := f.store.RegisterWorkspace(ctx, "Birds")
	if err != nil {
		t.Fatal(err)
	}
	sessions := make([]memory.Session, 3)
	for i := range sessions {
		if sessions[i], err = f.store.CreateWorkspaceSessionWithComposition(ctx, workspace.ID, workspace.CurrentRevisionID, reviewTestReceipt()); err != nil {
			t.Fatal(err)
		}
	}
	single, double, reader := sessions[0], sessions[1], sessions[2]
	for i := 1; i <= 4; i++ {
		f.turn(single, fmt.Sprintf("osprey nesting survey %d", i))
		f.turn(double, fmt.Sprintf("osprey banding round %d", i))
	}
	singleCompaction := f.compact(single, "osprey nesting survey finished")
	firstOfDouble := f.compact(double, "osprey banding started")
	f.turn(double, "osprey banding round 5")
	f.turn(double, "osprey banding round 6")
	latestOfDouble := f.compact(double, "osprey banding results recorded")

	// A database written before session episodes existed has none of their
	// storage; opening it creates the storage and derives every episode from
	// the retained context_compacted events, without a model call.
	if _, err = f.db.Exec(`DROP TABLE memory_session_episode_fts; DROP TABLE memory_session_episodes; DROP INDEX events_context_compacted_idx`); err != nil {
		t.Fatal(err)
	}
	f.reopen()
	found := episodeSessions(f.search(reader, "osprey"))
	if len(found) != 2 || found[single.ID].Source.SummaryEventID != singleCompaction || found[single.ID].Source.Generation != 1 ||
		found[double.ID].Source.SummaryEventID != latestOfDouble || found[double.ID].Source.Generation != 2 ||
		!strings.Contains(found[double.ID].Summary, "osprey banding results recorded") {
		t.Fatalf("backfilled episodes = %+v", found)
	}
	rowids := map[memory.SessionID]int64{}
	rows, err := f.db.Query(`SELECT session_id,episode_rowid FROM memory_session_episodes`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var session memory.SessionID
		var rowid int64
		if err = rows.Scan(&session, &rowid); err != nil {
			t.Fatal(err)
		}
		rowids[session] = rowid
	}
	rows.Close()
	if len(rowids) != 2 {
		t.Fatalf("stored episodes = %v, want one per compacted session", rowids)
	}

	// An older process appended a later generation without maintaining the
	// episode. The stale summary is withheld, never served as current.
	if _, err = f.db.Exec(`UPDATE memory_session_episodes SET summary_event_id=?,generation=1 WHERE session_id=?`, firstOfDouble, double.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`INSERT INTO memory_session_episode_fts(rowid,body) SELECT ep.episode_rowid,e.content FROM memory_session_episodes ep
 JOIN events e ON e.id=ep.summary_event_id WHERE ep.session_id=?`, double.ID); err != nil {
		t.Fatal(err)
	}
	stale := f.search(reader, "osprey")
	if found = episodeSessions(stale); stale.Status != memory.RetrievalPartial || stale.Withheld != 1 || len(found) != 1 || found[single.ID].SessionID != single.ID {
		t.Fatalf("stale episode search = %+v", stale)
	}
	f.reopen()
	reconciled := f.search(reader, "osprey")
	if found = episodeSessions(reconciled); reconciled.Status != memory.RetrievalSuccess || len(found) != 2 ||
		found[double.ID].Source.SummaryEventID != latestOfDouble || found[double.ID].Source.Generation != 2 {
		t.Fatalf("startup did not reconcile the stale episode: %+v", reconciled)
	}
	f.reopen()
	var unchanged int
	for session, rowid := range rowids {
		if err = f.db.QueryRow(`SELECT count(*) FROM memory_session_episodes WHERE session_id=? AND episode_rowid=?`, session, rowid).Scan(&unchanged); err != nil || unchanged != 1 {
			t.Fatalf("reopening rewrote episode %s: %v", session, err)
		}
	}
}

func TestSessionEpisodeWithDetectedSecretIsNeverIndexedOrReturned(t *testing.T) {
	f := newEpisodeFixture(t)
	ctx := context.Background()
	source, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := f.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 4; i++ {
		f.turn(source, fmt.Sprintf("pelican deploy step %d", i))
	}
	f.compact(source, "pelican deploy used key AKIAABCDEFGHIJKLMNOP")
	result := f.search(reader, "pelican")
	if len(result.Episodes) != 0 || strings.Contains(fmt.Sprint(result), "AKIA") {
		t.Fatalf("secret-bearing summary was returned: %+v", result)
	}
	var stored, indexed int
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_session_episodes WHERE session_id=?`, source.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err = f.db.QueryRow(`SELECT count(*) FROM memory_session_episode_fts`).Scan(&indexed); err != nil {
		t.Fatal(err)
	}
	if stored != 1 || indexed != 0 {
		t.Fatalf("secret-bearing summary: stored=%d indexed=%d, want provenance only", stored, indexed)
	}
}

func TestSessionEpisodeSearchRejectsUnboundedQueries(t *testing.T) {
	f := newEpisodeFixture(t)
	reader, err := f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, query := range []memory.SessionEpisodeQuery{
		{Text: ""}, {Text: "osprey", Limit: 6}, {Text: "osprey", Limit: -1}, {Text: "osprey", Intent: "future"},
		{Text: strings.Repeat("osprey ", 200)},
	} {
		if _, err := f.store.SearchSessionEpisodes(context.Background(), reader.ScopeContext(), query); err == nil {
			t.Fatalf("unbounded episode query %+v was accepted", query)
		}
	}
	other := reader.ScopeContext()
	other.WorkspaceID = "88888888-8888-4888-8888-888888888888"
	if _, err := f.store.SearchSessionEpisodes(context.Background(), other, memory.SessionEpisodeQuery{Text: "osprey"}); err == nil {
		t.Fatal("a scope that does not match its durable session was accepted")
	}
}
