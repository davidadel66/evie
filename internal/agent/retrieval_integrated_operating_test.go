package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/tools"
)

const (
	integratedOperatingRepetitions = 20
	integratedOperatingTailLimit   = time.Second
	integratedOperatingDeadline    = 2 * time.Second
)

var integratedOperatingModes = []string{"cancellation", "lease_replacement", "caller_deadline"}

type integratedOperatingRequest struct {
	Bytes   int    `json:"bytes"`
	SHA256  string `json:"sha256"`
	Encoded string `json:"encoded_json"`
}

type integratedOperatingSample struct {
	Mode                string                       `json:"mode"`
	Repetition          int                          `json:"repetition"`
	Complete            bool                         `json:"complete"`
	ExpectedClaim       memory.SemanticID            `json:"expected_claim_id"`
	ExpectedSource      memory.SemanticID            `json:"expected_source_link_id"`
	ExpectedEvent       memory.EventID               `json:"expected_source_event_id"`
	BoundaryAt          time.Time                    `json:"boundary_at"`
	ReturnedAt          time.Time                    `json:"returned_at"`
	CallerDeadline      time.Time                    `json:"caller_deadline,omitempty"`
	TailNS              int64                        `json:"tail_ns"`
	SendNS              int64                        `json:"send_ns"`
	SendError           string                       `json:"send_error"`
	ErrorClassification string                       `json:"error_classification"`
	ApprovalCalls       int                          `json:"approval_calls"`
	Evidence            []memory.RetrievalReference  `json:"evidence_before_boundary"`
	Requests            []integratedOperatingRequest `json:"requests"`
	EventsBefore        []memory.Event               `json:"events_before_boundary"`
	EventsAfter         []memory.Event               `json:"events_after_return"`
	RevisionsBefore     []memory.ScopeRevision       `json:"accepted_revisions_before"`
	RevisionsAfter      []memory.ScopeRevision       `json:"accepted_revisions_after"`
	ReplacementLease    *memory.TurnLease            `json:"replacement_lease,omitempty"`
	LeaseAfter          *memory.TurnLease            `json:"lease_after_return,omitempty"`
	Failures            []string                     `json:"failures"`
}

func (s *integratedOperatingSample) fail(format string, args ...any) {
	s.Failures = append(s.Failures, fmt.Sprintf(format, args...))
}

// The ordinary tracer verifies behavior without writing or claiming operating
// measurements. The opt-in runner below owns the fixed twenty-sample gates.
func TestMemoryIntegratedOperatingBoundaries(t *testing.T) {
	for _, mode := range integratedOperatingModes {
		t.Run(mode, func(t *testing.T) {
			sample := integratedOperatingSample{Mode: mode}
			runIntegratedOperatingCase(t, &sample)
			for _, failure := range sample.Failures {
				t.Error(failure)
			}
			if !sample.Complete {
				t.Fatal("operating boundary was not selected and returned")
			}
		})
	}
}

func TestMemoryStage5IntegratedOperatingMeasurements(t *testing.T) {
	output := os.Getenv("EVIE_MEMORY_INTEGRATED_OPERATING_OUTPUT")
	if output == "" {
		t.Skip("requires a frozen executable and EVIE_MEMORY_INTEGRATED_OPERATING_OUTPUT")
	}
	if !filepath.IsAbs(output) {
		t.Fatal("operating output must be an absolute new directory")
	}
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatalf("create new operating output (existing attempts are never overwritten): %v", err)
	}
	configuration := map[string]any{
		"version": "integrated-operating-v1", "repetitions_per_condition": integratedOperatingRepetitions,
		"conditions": integratedOperatingModes, "max_tail_ns": integratedOperatingTailLimit.Nanoseconds(),
		"caller_deadline_after_send_entry_ns": integratedOperatingDeadline.Nanoseconds(),
		"go_version":                          runtime.Version(), "goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"logical_cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0),
		"reader": "scripted provider; no reader quality or embedding inference",
	}
	if err := writeIntegratedOperatingJSON(filepath.Join(output, "configuration.json"), configuration); err != nil {
		t.Fatal(err)
	}
	journal, err := os.OpenFile(filepath.Join(output, "samples.ndjson"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	var samples []integratedOperatingSample
	defer func() {
		report := integratedOperatingReport(samples)
		if err := writeIntegratedOperatingJSON(filepath.Join(output, "report.json"), report); err != nil {
			t.Errorf("retain operating report: %v", err)
		}
	}()
	for repetition := 0; repetition < integratedOperatingRepetitions; repetition++ {
		for offset := range integratedOperatingModes {
			mode := integratedOperatingModes[(repetition+offset)%len(integratedOperatingModes)]
			t.Run(fmt.Sprintf("%02d/%s", repetition, mode), func(t *testing.T) {
				sample := integratedOperatingSample{Mode: mode, Repetition: repetition}
				defer func() {
					if !sample.Complete {
						sample.fail("sample aborted before boundary/return; retain the test execution log for the original setup error")
					}
					if sample.Complete && (sample.TailNS < 0 || sample.TailNS > integratedOperatingTailLimit.Nanoseconds()) {
						sample.fail("tail %d ns violates the fixed [0,%d] ns gate", sample.TailNS, integratedOperatingTailLimit.Nanoseconds())
					}
					if err := json.NewEncoder(journal).Encode(sample); err != nil {
						sample.fail("retain raw operating sample: %v", err)
					}
					if err := journal.Sync(); err != nil {
						sample.fail("flush raw operating sample: %v", err)
					}
					samples = append(samples, sample)
					for _, failure := range sample.Failures {
						t.Error(failure)
					}
				}()
				runIntegratedOperatingCase(t, &sample)
			})
		}
	}
	if len(samples) != len(integratedOperatingModes)*integratedOperatingRepetitions {
		t.Errorf("incomplete operating run: retained %d samples, expected 60", len(samples))
	}
}

func runIntegratedOperatingCase(t *testing.T, sample *integratedOperatingSample) {
	t.Helper()
	t.Setenv("EVIE_MEMORY_EMBEDDING_ENDPOINT", "")
	f := newRetrievalFixture(t)
	source, reader := f.global(), f.global()
	saved := f.remember(source, memory.MemoryEverywhere, "spinel operating evidence")
	sample.ExpectedClaim, sample.ExpectedSource, sample.ExpectedEvent = saved.ClaimID, saved.SourceLinkID, saved.Source.EventID
	f.refresh()
	before, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		t.Fatal(err)
	}
	sample.RevisionsBefore = before.ScopeRevisions
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &expansionBoundaryClient{reply: func(request openrouter.ChatRequest, call int) step {
		if call == 0 {
			return assistantStep("", nil, toolCall("accepted", "memory_search", `{"query":"spinel"}`))
		}
		if call > 1 {
			sample.fail("provider called after the selected boundary: request %d", call+1)
			return assistantStep("Unexpected late request.", nil)
		}
		var evidence []memory.RetrievalEvidence
		for _, message := range request.Messages {
			if !strings.HasPrefix(message.Content, "EVIE_MEMORY_DATA\n") {
				continue
			}
			var data struct {
				Evidence []memory.RetrievalEvidence `json:"evidence"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(message.Content, "EVIE_MEMORY_DATA\n")), &data); err != nil {
				sample.fail("decode supplied memory: %v", err)
			}
			evidence = data.Evidence
		}
		for _, item := range evidence {
			sample.Evidence = append(sample.Evidence, item.Reference())
		}
		if len(evidence) != 1 || evidence[0].ClaimID != saved.ClaimID || len(evidence[0].Sources) != 1 || evidence[0].Sources[0].ID != saved.SourceLinkID || evidence[0].Sources[0].EventID != saved.Source.EventID {
			sample.fail("the exact accepted Claim/source was not supplied before the boundary")
		}
		var err error
		sample.EventsBefore, err = f.store.LoadEvents(context.Background(), reader.ID)
		if err != nil {
			sample.fail("read events before boundary: %v", err)
		}
		switch sample.Mode {
		case "cancellation":
			sample.BoundaryAt = time.Now()
			cancel()
		case "lease_replacement":
			current, err := f.store.GetTurnLease(context.Background(), reader.ID)
			if err != nil {
				sample.fail("read original live lease: %v", err)
				break
			}
			sample.BoundaryAt = time.Now()
			if err := f.store.ReleaseTurnLease(context.Background(), reader.ID, current.HolderID, current.FencingToken); err != nil {
				sample.fail("release original live lease: %v", err)
				break
			}
			replacement, err := f.store.AcquireTurnLease(context.Background(), reader.ID, "operating-replacement", time.Minute)
			if err != nil {
				sample.fail("acquire replacement live lease: %v", err)
				break
			}
			sample.ReplacementLease = &replacement
		case "caller_deadline":
			sample.BoundaryAt = sample.CallerDeadline
			<-ctx.Done()
		default:
			sample.fail("unknown operating condition %q", sample.Mode)
		}
		return assistantStep("", nil,
			toolCall("late-search", "memory_search_conversations", `{"query":"spinel"}`),
			toolCall("late-retire", "memory_retire", `{"object_kind":"claim","object_id":"`+string(saved.ClaimID)+`","idempotency_key":"idem:v1:2c2ffaad-1ce1-4dd6-911d-f0c47c7479aa"}`))
	}}
	session := f.session(reader, client)
	started := time.Now()
	if sample.Mode == "caller_deadline" {
		sample.CallerDeadline = started.Add(integratedOperatingDeadline)
		var stop context.CancelFunc
		ctx, stop = context.WithDeadline(ctx, sample.CallerDeadline)
		defer stop()
	}
	err = session.Send(ctx, "Investigate the spinel evidence.", &recorder{}, func(context.Context, string, string, *tools.FileChangePreview) tools.Decision {
		sample.ApprovalCalls++
		return tools.Approved
	})
	sample.ReturnedAt = time.Now()
	sample.SendNS = sample.ReturnedAt.Sub(started).Nanoseconds()
	sample.Complete = !sample.BoundaryAt.IsZero()
	if sample.Complete {
		sample.TailNS = sample.ReturnedAt.Sub(sample.BoundaryAt).Nanoseconds()
	}
	if err != nil {
		sample.SendError = err.Error()
	}
	switch {
	case errors.Is(err, context.Canceled):
		sample.ErrorClassification = "cancellation"
	case errors.Is(err, context.DeadlineExceeded):
		sample.ErrorClassification = "caller_deadline"
	case errors.Is(err, ErrLeaseLost):
		sample.ErrorClassification = "lease_replacement"
	default:
		sample.ErrorClassification = "unexpected"
	}
	if sample.ErrorClassification != sample.Mode {
		sample.fail("wrong Send terminal classification: %s (%s)", sample.ErrorClassification, sample.SendError)
	}
	if len(client.reqs) != 2 || sample.ApprovalCalls != 0 {
		sample.fail("late effects: provider requests=%d approvals=%d", len(client.reqs), sample.ApprovalCalls)
	}
	for _, request := range client.reqs {
		wire, err := openrouter.RequestBytes(request)
		if err != nil {
			sample.fail("encode actual provider request: %v", err)
			continue
		}
		sample.Requests = append(sample.Requests, integratedOperatingRequest{len(wire), fmt.Sprintf("%x", sha256.Sum256(wire)), string(wire)})
	}
	sample.EventsAfter, err = f.store.LoadEvents(context.Background(), reader.ID)
	if err != nil {
		sample.fail("read events after return: %v", err)
	}
	if !reflect.DeepEqual(integratedOperatingProtectedEvents(sample.EventsBefore), integratedOperatingProtectedEvents(sample.EventsAfter)) {
		sample.fail("late tool activity, new receipt or changed original receipt after the boundary")
	}
	count := 0
	for _, event := range sample.EventsAfter {
		if event.Type != memory.EventContextSnapshot {
			continue
		}
		var snapshot memory.ContextSnapshotPayload
		if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
			sample.fail("decode context receipt: %v", err)
			continue
		}
		if snapshot.Memory != nil {
			count++
			if !reflect.DeepEqual(snapshot.Memory.Evidence, sample.Evidence) || snapshot.Memory.Investigation == nil || snapshot.Memory.Investigation.SearchAttempts != 1 || len(sample.Requests) != 2 || snapshot.RequestSHA256 != sample.Requests[1].SHA256 || snapshot.SerializedBytes != int64(sample.Requests[1].Bytes) {
				sample.fail("original memory receipt differs from the supplied evidence/request")
			}
		}
	}
	if count != 1 {
		sample.fail("memory receipt count=%d, expected one before the boundary", count)
	}
	after, err := f.store.InspectClaims(context.Background(), source.ScopeContext(), memory.ClaimQuery{})
	if err != nil {
		sample.fail("inspect accepted memory after return: %v", err)
	} else {
		sample.RevisionsAfter = after.ScopeRevisions
		if !reflect.DeepEqual(before.Claims, after.Claims) || !reflect.DeepEqual(before.ScopeRevisions, after.ScopeRevisions) {
			sample.fail("accepted Claims, source support or scope revisions changed after the boundary")
		}
	}
	if sample.ReplacementLease != nil {
		current, err := f.store.GetTurnLease(context.Background(), reader.ID)
		if err != nil {
			sample.fail("read surviving replacement lease: %v", err)
		} else {
			sample.LeaseAfter = &current
			if current != *sample.ReplacementLease {
				sample.fail("stale turn cleanup changed the replacement lease")
			}
		}
		lease := sample.ReplacementLease
		if err := f.store.ReleaseTurnLease(context.Background(), reader.ID, lease.HolderID, lease.FencingToken); err != nil {
			sample.fail("release test-owned replacement after measurement: %v", err)
		}
	}
}

func integratedOperatingProtectedEvents(events []memory.Event) []memory.Event {
	var selected []memory.Event
	for _, event := range events {
		if event.Type == memory.EventContextSnapshot || strings.HasPrefix(string(event.Type), "tool_") {
			selected = append(selected, event)
		}
	}
	return selected
}

func integratedOperatingReport(samples []integratedOperatingSample) map[string]any {
	conditions := make(map[string]any)
	var failureRecords []map[string]any
	passed := len(samples) == len(integratedOperatingModes)*integratedOperatingRepetitions
	for _, sample := range samples {
		if !sample.Complete || len(sample.Failures) != 0 {
			failureRecords = append(failureRecords, map[string]any{"mode": sample.Mode, "repetition": sample.Repetition, "complete": sample.Complete, "failures": sample.Failures})
		}
	}
	for _, mode := range integratedOperatingModes {
		var tails []int64
		count, failures := 0, 0
		for _, sample := range samples {
			if sample.Mode != mode {
				continue
			}
			count++
			if sample.Complete {
				tails = append(tails, sample.TailNS)
			}
			if !sample.Complete || len(sample.Failures) != 0 {
				failures++
			}
		}
		sort.Slice(tails, func(i, j int) bool { return tails[i] < tails[j] })
		condition := map[string]any{"samples": count, "completed_samples": len(tails), "failed_samples": failures, "max_tail_ns_gate": integratedOperatingTailLimit.Nanoseconds()}
		pass := count == integratedOperatingRepetitions && len(tails) == count && failures == 0
		if len(tails) > 0 {
			condition["p50_tail_ns"] = tails[(len(tails)*50+99)/100-1]
			condition["p95_tail_ns"] = tails[(len(tails)*95+99)/100-1]
			condition["max_tail_ns"] = tails[len(tails)-1]
			pass = pass && tails[0] >= 0 && tails[len(tails)-1] <= integratedOperatingTailLimit.Nanoseconds()
		}
		condition["passed"] = pass
		conditions[mode], passed = condition, passed && pass
	}
	return map[string]any{"version": "integrated-operating-v1", "passed": passed, "samples": len(samples), "expected_samples": 60, "conditions": conditions, "failure_records": failureRecords, "quantiles": "nearest rank over every completed sample, including failures; incomplete samples fail readiness and remain in the journal", "quality_claim": "none; actual scripted-turn operating boundaries only"}
}

func writeIntegratedOperatingJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(value); err != nil {
		return err
	}
	return f.Sync()
}
