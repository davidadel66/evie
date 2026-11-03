package eviedb_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/delegation"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

// The owner's report is the stored final report the parent model pages
// through read_subagent_report, byte for byte.
func TestSubagentReportIsTheReportTheParentPages(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	report := "## Summary\nHorizons double. <script>alert(1)</script>\n\n## Details\n" + strings.Repeat("détail ✓ ", 6000)
	a, _ := finishedChild(t, store, parent, receipt, report)

	var paged strings.Builder
	for offset := 0; ; {
		page, err := store.ReadSubagentReport(ctx, parent, a.ID, offset, 32*1024)
		if err != nil {
			t.Fatal(err)
		}
		paged.WriteString(page.Text)
		if page.NextOffset == nil {
			break
		}
		offset = *page.NextOffset
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text == nil || *got.Text != paged.String() || *got.Text != report {
		t.Fatalf("owner report differs from the paged report")
	}
	if got.ExecutionID != a.ID || got.Worker != delegation.WorkerResearch || got.Status != "succeeded" || got.Reason != "" ||
		got.TotalBytes != len(report) || got.OmittedBytes != 0 {
		t.Fatalf("report document = %+v", got)
	}
}

// Only the chat that delegated a child opens its report.
func TestSubagentReportStaysWithinTheParentSession(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	a, _ := finishedChild(t, store, parent, receipt, "## Summary\nsecret-sentinel")
	other, err := store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		session memory.SessionID
		id      string
	}{
		{"another chat's worker", other.ID, a.ID},
		{"the worker's own session", a.Child.ID, a.ID},
		{"an unknown execution", parent.Scope.SessionID, "missing"},
	} {
		got, err := store.SubagentReport(ctx, tc.session, tc.id)
		if !errors.Is(err, sql.ErrNoRows) || got.Text != nil {
			t.Fatalf("%s: %+v %v", tc.name, got, err)
		}
	}
}

// A worker a Workflow Run owns is not the chat's to open, even if its record
// named the chat as its scope: run pages have their own views.
func TestSubagentReportOfARunOwnedWorkerIsNotFound(t *testing.T) {
	ctx := context.Background()
	store, db, _, parent, receipt := newSubagentAdmission(t)
	a, _ := finishedChild(t, store, parent, receipt, "## Summary\nrun-sentinel")
	if _, err := db.Exec(`UPDATE subagent_executions SET record_json=json_set(record_json,'$.run',json(?)) WHERE id=?`,
		`{"run_id":"run-1","workspace_id":"ws","workflow":"fix-bugs","step":"research","attempt_seq":1,"holder":"runtime","token":1}`, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if !errors.Is(err, sql.ErrNoRows) || got.Text != nil {
		t.Fatalf("run-owned worker read: %+v %v", got, err)
	}
}

// A child without a final report is not an error: the owner sees how it
// ended, or that it is still running, and no report text.
func TestSubagentReportWithoutAFinalReportStatesHowTheChildEnded(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	running, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || running.Status != "running" || running.Reason != "" || running.Text != nil || running.TotalBytes != 0 {
		t.Fatalf("running child: %+v %v", running, err)
	}
	// Until it settles, Evie has recorded nothing about it.
	if running.Sources != nil || running.Notes != nil || running.Limitations != nil || running.Usage != nil || running.ModelResponses != nil {
		t.Fatalf("running child has a record: %+v", running)
	}
	if _, err = store.FinishSubagent(ctx, a.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}
	ended, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || ended.ExecutionID != a.ID || ended.Status != "interrupted" || ended.Reason != "no_accepted_final_answer" || ended.Text != nil || ended.TotalBytes != 0 {
		t.Fatalf("child without a report: %+v %v", ended, err)
	}
}

// A record that names a final report its child does not have is broken, not
// missing: the owner's read fails as a store failure, never as "no such
// child", and the parent model's read fails as it always has.
func TestSubagentReportWhoseFinalEventIsMissingIsAFailureNotAMissingChild(t *testing.T) {
	ctx := context.Background()
	store, db, _, parent, receipt := newSubagentAdmission(t)
	a, _ := finishedChild(t, store, parent, receipt, "## Summary\norphaned")
	if _, err := db.Exec(`UPDATE subagent_executions SET record_json=json_set(record_json,'$.final_event_id','no-such-event') WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err == nil || errors.Is(err, sql.ErrNoRows) || got.Text != nil {
		t.Fatalf("owner read of a broken record: %+v %v", got, err)
	}
	if _, err = store.ReadSubagentReport(ctx, parent, a.ID, 0, 0); err == nil || err.Error() != "load stored report: sql: no rows in result set" {
		t.Fatalf("parent read of a broken record: %v", err)
	}
}

// A partial report opens with the reason its child stopped early.
func TestSubagentReportOfAPartialReportCarriesItsReason(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	lease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "## Summary\nhalf done"}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.FinishSubagent(ctx, a.ID, delegation.StatePartial, delegation.WrapUpTime); err != nil {
		t.Fatal(err)
	}
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil || got.Status != delegation.StatePartial || got.Reason != delegation.WrapUpTime || got.Text == nil || *got.Text != "## Summary\nhalf done" {
		t.Fatalf("partial report: %+v %v", got, err)
	}
}

// childRound appends one model response of a child after prior, with usage
// (nil when the provider reported none), then each of its tool calls and the
// call's successful outcome. It returns the last event it appended.
func childRound(t *testing.T, store *eviedb.Store, session memory.SessionID, lease memory.TurnLease, prior memory.EventID, content string, usage *memory.TokenUsage, calls map[memory.ToolCall]string) memory.EventID {
	t.Helper()
	ctx := context.Background()
	appendEvent := func(input memory.EventInput) memory.EventID {
		t.Helper()
		event, err := store.AppendEventWithLease(ctx, session, lease.HolderID, lease.FencingToken, input)
		if err != nil {
			t.Fatal(err)
		}
		return event.ID
	}
	payload := memory.AssistantMessagePayload{Usage: usage}
	for call := range calls {
		payload.ToolCalls = append(payload.ToolCalls, call)
	}
	slices.SortFunc(payload.ToolCalls, func(a, b memory.ToolCall) int { return strings.Compare(a.ID, b.ID) })
	encoded, _ := json.Marshal(payload)
	last := appendEvent(memory.EventInput{ParentID: prior, Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, Content: content, Payload: encoded})
	for _, call := range payload.ToolCalls {
		execution := memory.ExecutionID("execution-" + call.ID)
		intent, _ := json.Marshal(memory.ToolIntentPayload{Call: call})
		intentID := appendEvent(memory.EventInput{ParentID: last, Type: memory.EventToolIntent, ExecutionID: execution, Payload: intent})
		result, _ := json.Marshal(memory.ToolResultPayload{ToolCallID: call.ID})
		last = appendEvent(memory.EventInput{ParentID: intentID, Type: memory.EventToolSucceeded, Role: memory.RoleTool, ExecutionID: execution, Content: calls[call], Payload: result})
	}
	return last
}

func tokens(input, output int64) *memory.TokenUsage {
	total := input + output
	return &memory.TokenUsage{InputTokens: &input, OutputTokens: &output, TotalTokens: &total}
}

// Evie's own record of a report opens with it: the sources the worker's Web
// tools actually returned and whether the report cites each, the cited URLs
// no tool returned, Evie's notes, the worker's limitations, the stored usage
// and how many model responses the attempt took.
func TestSubagentReportCarriesWhatEvieRecordedAboutIt(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	attempts, err := store.AdmitSubagents(ctx, parent, []delegation.Assignment{{Key: "one", Objective: "find evidence"}}, receipt, delegation.DefaultPolicy(), "dispatch")
	if err != nil {
		t.Fatal(err)
	}
	a, started, err := store.StartSubagent(ctx, attempts[0].ID)
	if err != nil || !started {
		t.Fatalf("start: %t %v", started, err)
	}
	lease, err := store.AcquireTurnLease(ctx, a.Child.ID, "child-holder", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, a.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "assignment"})
	if err != nil {
		t.Fatal(err)
	}
	// The first response reports no usage, so the stored total is a lower bound.
	last := childRound(t, store, a.Child.ID, lease, root.ID, "", nil, map[memory.ToolCall]string{
		{ID: "search", Name: "web_search", Arguments: `{"query":"agent horizons"}`}: "1. METR\nhttps://metr.org/blog\nSnippet\n2. Listed\nhttps://listed.example/only",
	})
	last = childRound(t, store, a.Child.ID, lease, last, "Reading both.", tokens(200, 20), map[memory.ToolCall]string{
		{ID: "fetch-1", Name: "web_fetch", Arguments: `{"url":"https://epoch.ai/data"}`}:           `{"url":"https://epoch.ai/data","excerpt":"data"}`,
		{ID: "fetch-2", Name: "web_fetch", Arguments: `{"url":"https://fetched.example/uncited"}`}: `{"url":"https://fetched.example/uncited","excerpt":"other"}`,
	})
	report := "## Summary\nHorizons double (https://epoch.ai/data, https://metr.org/blog); see https://unverified.example/claim.\n\n## Limitations\n- Only two sources were read.\n- No primary data.\n"
	childRound(t, store, a.Child.ID, lease, last, report, tokens(300, 30), nil)
	if _, err = store.FinishSubagent(ctx, a.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}

	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text == nil || *got.Text != report || got.Status != "succeeded" {
		t.Fatalf("report document = %+v", got)
	}
	wantSources := []eviedb.SubagentReportSource{
		{URL: "https://metr.org/blog", Fetched: false, Cited: true},
		{URL: "https://epoch.ai/data", Fetched: true, Cited: true},
		{URL: "https://fetched.example/uncited", Fetched: true, Cited: false},
	}
	if !slices.Equal(got.Sources, wantSources) {
		t.Fatalf("sources = %+v", got.Sources)
	}
	if !slices.Equal(got.UnverifiedURLs, []string{"https://unverified.example/claim"}) {
		t.Fatalf("unverified URLs = %q", got.UnverifiedURLs)
	}
	if !slices.Equal(got.Limitations, []string{"Only two sources were read.", "No primary data."}) {
		t.Fatalf("limitations = %q", got.Limitations)
	}
	if !slices.Equal(got.Notes, []string{"URLs in unverified_urls are cited in the report but were never fetched or returned by a search."}) {
		t.Fatalf("notes = %q", got.Notes)
	}
	if u := got.Usage; u == nil || !u.Incomplete || u.InputTokens == nil || *u.InputTokens != 500 || u.OutputTokens == nil || *u.OutputTokens != 50 || u.TotalTokens == nil || *u.TotalTokens != 550 {
		t.Fatalf("usage = %+v", got.Usage)
	}
	// Each committed response counts, with or without tool calls.
	if responses(got) != 3 {
		t.Fatalf("model responses = %v", responses(got))
	}
}

// Results stored before Stage 8 (inline findings, plain-string sources) and
// before Stage 10 (no harness notes) still open. A legacy source says it was
// recorded before Evie checked sources, and the report text comes from the
// final report event, never from the inline findings.
func TestSubagentReportOpensResultsStoredBeforeSourcesAndNotes(t *testing.T) {
	for _, tc := range []struct {
		name, result string
		final        bool
		sources      []eviedb.SubagentReportSource
		limitations  []string
	}{
		{
			name:        "pre-Stage 8",
			result:      `{"status":"succeeded","findings":"old findings https://example.com/a","sources":["https://example.com/a"],"usage":{"total_tokens":40}}`,
			final:       true,
			sources:     []eviedb.SubagentReportSource{{URL: "https://example.com/a", Legacy: true}},
			limitations: nil,
		},
		{
			name:    "pre-Stage 8 without a final event",
			result:  `{"status":"succeeded","findings":"old findings https://example.com/a","sources":["https://example.com/a"],"usage":{"total_tokens":40}}`,
			sources: []eviedb.SubagentReportSource{{URL: "https://example.com/a", Legacy: true}},
		},
		{
			name:        "pre-Stage 10",
			result:      `{"status":"succeeded","summary":"stored summary","sources":[{"url":"https://example.com/b","fetched":true,"cited":true}],"limitations":["Only one source.","The inline summary was cut to fit the result limit."],"usage":{"total_tokens":40}}`,
			final:       true,
			sources:     []eviedb.SubagentReportSource{{URL: "https://example.com/b", Fetched: true, Cited: true}},
			limitations: []string{"Only one source.", "The inline summary was cut to fit the result limit."},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store, db, _, parent, receipt := newSubagentAdmission(t)
			a, _ := finishedChild(t, store, parent, receipt, "## Summary\nthe stored report")
			var raw string
			if err := db.QueryRow(`SELECT record_json FROM subagent_executions WHERE id=?`, a.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var record map[string]json.RawMessage
			if err := json.Unmarshal([]byte(raw), &record); err != nil {
				t.Fatal(err)
			}
			var result map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.result), &result); err != nil {
				t.Fatal(err)
			}
			result["execution_id"], _ = json.Marshal(a.ID)
			result["child_session_id"], _ = json.Marshal(a.Child.ID)
			record["result"], _ = json.Marshal(result)
			if !tc.final {
				delete(record, "final_event_id")
			}
			encoded, _ := json.Marshal(record)
			if _, err := db.Exec(`UPDATE subagent_executions SET record_json=? WHERE id=?`, string(encoded), a.ID); err != nil {
				t.Fatal(err)
			}

			got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if tc.final != (got.Text != nil) || (got.Text != nil && *got.Text != "## Summary\nthe stored report") {
				t.Fatalf("report text = %v", got.Text)
			}
			if !slices.Equal(got.Sources, tc.sources) || !slices.Equal(got.Limitations, tc.limitations) {
				t.Fatalf("sources %+v, limitations %q", got.Sources, got.Limitations)
			}
			if got.Notes != nil || got.UnverifiedURLs != nil {
				t.Fatalf("an old result gained notes %q or unverified URLs %q", got.Notes, got.UnverifiedURLs)
			}
			if got.Usage == nil || got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != 40 || responses(got) != 1 {
				t.Fatalf("usage %+v, model responses %v", got.Usage, responses(got))
			}
		})
	}
}

// Each attempt of a continued child counts only its own model responses:
// the original's stop where the continuation's turn began.
func TestSubagentReportCountsOnlyItsOwnAttemptsModelResponses(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	original, _ := finishedChild(t, store, parent, receipt, "## Summary\nfirst report")
	p := continueTurn(t, store, parent, original.ID, "go on")
	c, err := store.AdmitSubagentContinuation(ctx, p, original.ID, "go on", delegation.DefaultPolicy(), "continuation")
	if err != nil {
		t.Fatal(err)
	}
	if c, _, err = store.StartSubagent(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, c.Child.ID, "child-holder-2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, c.Child.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "go on"})
	if err != nil {
		t.Fatal(err)
	}
	last := childRound(t, store, c.Child.ID, lease, root.ID, "", tokens(10, 1), map[memory.ToolCall]string{
		{ID: "search", Name: "web_search", Arguments: `{"query":"more"}`}: "1. More\nhttps://more.example/a",
	})
	childRound(t, store, c.Child.ID, lease, last, "## Summary\nsecond report", tokens(10, 1), nil)
	if _, err = store.FinishSubagent(ctx, c.ID, "succeeded", ""); err != nil {
		t.Fatal(err)
	}

	for id, want := range map[string]int{original.ID: 1, c.ID: 2} {
		got, err := store.SubagentReport(ctx, parent.Scope.SessionID, id)
		if err != nil {
			t.Fatal(err)
		}
		if responses(got) != want {
			t.Fatalf("attempt %s: model responses = %v, want %d", id, responses(got), want)
		}
	}
}

// A report over 1 MiB is cut at a UTF-8 boundary and says how much is left
// out; the parent's pages are unaffected.
func TestSubagentReportOverOneMebibyteStatesTheBytesLeftOut(t *testing.T) {
	ctx := context.Background()
	store, _, _, parent, receipt := newSubagentAdmission(t)
	// Two-byte runes start at odd offsets after the 11-byte heading, so the
	// 1 MiB mark falls inside one and the cut steps back a byte.
	report := "## Summary\n" + strings.Repeat("é", 600_000)
	a, _ := finishedChild(t, store, parent, receipt, report)
	got, err := store.SubagentReport(ctx, parent.Scope.SessionID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	const kept = 1<<20 - 1
	if got.Text == nil || len(*got.Text) != kept || *got.Text != report[:kept] || !utf8.ValidString(*got.Text) ||
		got.TotalBytes != 1_200_011 || got.OmittedBytes != 1_200_011-kept {
		t.Fatalf("cut report: text %d bytes, total %d, left out %d", len(*got.Text), got.TotalBytes, got.OmittedBytes)
	}
}

// responses is a report's model-response count, or -1 when it has none.
func responses(doc eviedb.SubagentReport) int {
	if doc.ModelResponses == nil {
		return -1
	}
	return *doc.ModelResponses
}
