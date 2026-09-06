package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

type measuredHistory struct {
	*fakeHistory
	records   []memory.CompilerForegroundMeasurement
	recordErr error
}

func (h *measuredHistory) RecordCompilerForeground(ctx context.Context, m memory.CompilerForegroundMeasurement) error {
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("missing bound")
	}
	h.records = append(h.records, m)
	return h.recordErr
}

func TestForegroundMeasurementWaitsForActualHostBoundary(t *testing.T) {
	history := &measuredHistory{fakeHistory: &fakeHistory{}}
	client := &fakeClient{steps: []step{assistantStep("done", nil)}}
	session := newTestSession(client, DefaultModel)
	session.history = history
	ctx, finish := BeginResponseMeasurement(context.Background())
	if err := session.Send(ctx, "start", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(history.records) != 0 {
		t.Fatal("Send return claimed host finalization")
	}
	before := time.Now().UnixMilli()
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if len(history.records) != 1 {
		t.Fatalf("records %d", len(history.records))
	}
	m := history.records[0]
	if m.RootID == "" || m.Outcome != "success" || m.TerminalCommittedAtUnixMS == nil || m.TerminalCommitNanos == nil || m.ResponseFinalizedAtUnixMS == nil || *m.ResponseFinalizedAtUnixMS < before || m.ResponseFinalizationNanos == nil || *m.ResponseFinalizationNanos < *m.TerminalCommitNanos {
		t.Fatalf("measurement %+v", m)
	}
	if err := finish(nil); err != nil || len(history.records) != 1 {
		t.Fatalf("repeat finalizer %v %d", err, len(history.records))
	}
}
func TestForegroundMeasurementAbsentHostAndStorageFailure(t *testing.T) {
	for _, host := range []bool{false, true} {
		t.Run(map[bool]string{false: "no host", true: "host"}[host], func(t *testing.T) {
			h := &measuredHistory{fakeHistory: &fakeHistory{}, recordErr: errors.New("measurement unavailable")}
			session := newTestSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, DefaultModel)
			session.history = h
			ctx := context.Background()
			var finish func(error) error
			if host {
				ctx, finish = BeginResponseMeasurement(ctx)
			}
			if err := session.Send(ctx, "start", &recorder{}, nil); err != nil {
				t.Fatalf("diagnostic changed turn: %v", err)
			}
			if host {
				if err := finish(nil); err == nil {
					t.Fatal("collector failure hidden from host")
				}
			}
			if len(h.records) != 1 {
				t.Fatalf("records %d", len(h.records))
			}
			if !host && h.records[0].ResponseFinalizedAtUnixMS != nil {
				t.Fatal("invented host finalization")
			}
		})
	}
}
func TestForegroundMeasurementFailedTerminalIsSeparate(t *testing.T) {
	h := &measuredHistory{fakeHistory: &fakeHistory{}}
	session := newTestSession(&fakeClient{steps: []step{{err: errors.New("provider failed")}}}, DefaultModel)
	session.history = h
	ctx, finish := BeginResponseMeasurement(context.Background())
	if err := session.Send(ctx, "start", &recorder{}, nil); err == nil {
		t.Fatal("expected provider failure")
	}
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if len(h.records) != 1 || h.records[0].Outcome != "failed" || h.records[0].TerminalCommittedAtUnixMS == nil {
		t.Fatalf("failure %+v", h.records)
	}
}

func TestForegroundMeasurementFailedOutputKeepsTerminalAndCannotBeRetriedAsSuccess(t *testing.T) {
	h := &measuredHistory{fakeHistory: &fakeHistory{}}
	session := newTestSession(&fakeClient{steps: []step{assistantStep("done", nil)}}, DefaultModel)
	session.history = h
	ctx, finish := BeginResponseMeasurement(context.Background())
	if err := session.Send(ctx, "start", &recorder{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := finish(errors.New("host output unavailable")); err != nil {
		t.Fatalf("output failure is not a measurement storage failure: %v", err)
	}
	if err := finish(nil); err != nil || len(h.records) != 1 {
		t.Fatalf("repeated finalizer changed observation: %v %+v", err, h.records)
	}
	m := h.records[0]
	if m.Outcome != "success" || m.TerminalCommittedAtUnixMS == nil || m.TerminalCommitNanos == nil || m.ResponseFinalizedAtUnixMS != nil || m.ResponseFinalizationNanos != nil {
		t.Fatalf("failed output measurement %+v", m)
	}
}
