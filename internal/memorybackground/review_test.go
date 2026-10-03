package memorybackground

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

// At the defaults the largest possible batch and decision both fit the pass
// budget, so no conversation can fail every pass on size alone.
func TestWorstCaseRequestsFitThePassBudget(t *testing.T) {
	config := Defaults()
	var batch memory.BackgroundBatch
	for i := range config.BatchMessages {
		// Angle brackets would cost six bytes each with HTML escaping.
		content := strings.Repeat("<a>&\"", config.BatchBytes/config.BatchMessages/5)
		batch.Messages = append(batch.Messages, memory.BackgroundOwnerMessage{EventID: memory.EventID(fmt.Sprint(i)), Content: content})
	}
	batch.Messages[0].Content = strings.Repeat("x", 64<<10) // one oversized message, cut visibly
	pool := memory.BackgroundPool{ContextScopeKey: "workspace:w"}
	for i := range 60 {
		pool.Predicates = append(pool.Predicates, memory.BackgroundPredicate{Token: fmt.Sprintf("%s_%02d", strings.Repeat("p", 60), i), Label: strings.Repeat("l", 80)})
	}
	extract, err := extractRequest(batch, pool, config)
	if err != nil {
		t.Fatal(err)
	}
	var comparisons []comparison
	for i := range config.CandidateLimit {
		c := candidate{Message: memory.BackgroundOwnerMessage{Content: strings.Repeat("o", 10_000)}, Label: strings.Repeat("l", 80), Literal: memory.TypedLiteral{Kind: memory.LiteralText, Value: strings.Repeat("v", 300)}}
		var similar []memory.BackgroundExistingMemory
		for j := range config.SimilarLimit {
			similar = append(similar, memory.BackgroundExistingMemory{ClaimID: memory.SemanticID(fmt.Sprintf("c%d-%d", i, j)), Text: strings.Repeat("t", 5_000)})
		}
		comparisons = append(comparisons, comparison{candidate: c, similar: similar})
	}
	decide, err := decideRequest(comparisons, config)
	if err != nil {
		t.Fatal(err)
	}
	extractIn, extractOut := extract.EstimatedTokens()
	decideIn, decideOut := decide.EstimatedTokens()
	if total := extractIn + extractOut + decideIn + decideOut; total > config.PassTokens {
		t.Fatalf("worst case needs %d tokens (extract %d+%d, decide %d+%d), beyond the %d pass budget", total, extractIn, extractOut, decideIn, decideOut, config.PassTokens)
	}
	if !strings.Contains(extract.User, "<a>&") {
		t.Fatal("owner text is HTML-escaped, inflating the request")
	}
}

// A memory similar to several candidates is listed once.
func TestDecisionRequestListsEachExistingMemoryOnce(t *testing.T) {
	shared := memory.BackgroundExistingMemory{ClaimID: "c1", Text: "owner — home city: Boston"}
	comparisons := []comparison{
		{candidate: candidate{Label: "home city", Literal: memory.TypedLiteral{Value: "Chicago"}}, similar: []memory.BackgroundExistingMemory{shared}},
		{candidate: candidate{Label: "city", Literal: memory.TypedLiteral{Value: "Chicago"}}, similar: []memory.BackgroundExistingMemory{shared}},
	}
	request, err := decideRequest(comparisons, Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(request.User, "home city: Boston") != 1 || strings.Count(request.User, `"compare_with":["m1"]`) != 2 {
		t.Fatalf("decision request = %s", request.User)
	}
}

// A process that lost the runner lease to another makes no calls.
func TestARunnerWithoutTheRunnerLeaseMakesNoCalls(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	f.turn(session, "I prefer tea.")
	f.offset = f.config.Idle + time.Second
	if ok, err := f.store.AcquireMemoryBackgroundRunner(f.ctx, "other-process", time.Hour); err != nil || !ok {
		t.Fatalf("other process lease = %v, %v", ok, err)
	}
	result, err := f.runner.Tick(f.ctx)
	if err != nil || result.Leased || len(f.model.calls("")) != 0 {
		t.Fatalf("tick without the runner lease = %+v, %v (calls %d)", result, err, len(f.model.calls("")))
	}
	if _, err := f.runner.PassThrough(f.ctx, session.ID, 10); err == nil || len(f.model.calls("")) != 0 {
		t.Fatalf("a compaction flush ran without the runner lease: %v", err)
	}
}

// Two passes that read the same batch: the second to commit records its
// spend as superseded and queues nothing; the cursor stays the first's.
func TestASupersededPassStillCountsItsSpend(t *testing.T) {
	f := newFixture(t)
	session := f.globalSession()
	f.turn(session, "I'm a vegetarian.")
	release := make(chan struct{})
	reply := `{"memories":[{"message":0,"kind":"profile_fact","applies":"profile","predicate":"diet","label":"diet","cardinality":"one","value_type":"text","value":"vegetarian","polarity":"affirmed"}]}`
	f.model.script("extract", scriptedReply{text: reply, usage: [2]int64{700, 70}, wait: release})
	done := make(chan memory.BackgroundRun, 1)
	go func() {
		run, _ := f.runner.runSession(f.ctx, session.ID, memory.BackgroundTriggerIdle, 0)
		done <- run
	}()
	for len(f.model.calls("extract")) == 0 {
		time.Sleep(time.Millisecond)
	}
	f.model.script("extract", scriptedReply{text: reply, usage: [2]int64{900, 90}})
	first := f.mustRun(session)
	close(release)
	late := <-done
	if late.Status != memory.BackgroundRunSuperseded || late.InputTokens != 700 || late.Queued != 0 {
		t.Fatalf("late pass = %+v", late)
	}
	if f.cursor(session) != first.ThroughSequence || f.inbox().Pending != 1 {
		t.Fatalf("cursor = %d inbox = %+v", f.cursor(session), f.inbox())
	}
	progress, err := f.store.MemoryBackgroundProgress(f.ctx, f.runner.now(), 10)
	if err != nil || progress.Today.InputTokens != 1600 {
		t.Fatalf("spend = %+v, %v", progress.Today, err)
	}
}

type truncatingLocal struct{}

func (truncatingLocal) Chat(context.Context, memory.CompilerGeneration, string, string, int64) (string, int64, int64, error) {
	return "", 10, 768, localextractor.ErrRuntimeOutputInvalid
}

// A truncated local reply repeats under deterministic decoding, so it
// counts toward skipping the batch rather than retrying it forever.
func TestTruncatedLocalOutputCountsAsMalformed(t *testing.T) {
	model := NewLocalModel(truncatingLocal{}, memory.CompilerGeneration{ModelArtifact: "local:fixture"})
	_, err := model.Complete(context.Background(), ModelRequest{})
	var provider *ProviderError
	if !errors.Is(err, ErrMalformed) || !errors.As(err, &provider) || provider.Retryable {
		t.Fatalf("truncated local output = %v", err)
	}
}
