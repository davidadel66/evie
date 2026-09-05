package eviedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// This scripted runtime has authenticated request-specific status semantics.
// It deliberately continues computing after its HTTP client disconnects.
// It is not a claim about Ollama's supported API or its release behaviour.
type workerHTTPRuntime struct {
	server              *httptest.Server
	token               string
	started             chan string
	release             chan struct{}
	mu                  sync.Mutex
	active, peak, calls int
	finished            map[string]bool
}

func newWorkerHTTPRuntime(t *testing.T) *workerHTTPRuntime {
	t.Helper()
	r := &workerHTTPRuntime{token: "fixture-request-status-secret", started: make(chan string, 8), release: make(chan struct{}), finished: map[string]bool{}}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		id := req.Header.Get("X-Attempt-ID")
		if req.URL.Path == "/status" {
			if req.Header.Get("Authorization") != "Bearer "+r.token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.mu.Lock()
			done := r.finished[id]
			r.mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"attempt_id": id, "completed": done})
			return
		}
		var request memory.CompilerRequest
		if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		r.mu.Lock()
		r.active++
		r.calls++
		if r.active > r.peak {
			r.peak = r.active
		}
		r.mu.Unlock()
		r.started <- id
		<-r.release
		r.mu.Lock()
		r.active--
		r.finished[id] = true
		r.mu.Unlock()
		json.NewEncoder(w).Encode(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{}})
	}))
	t.Cleanup(r.server.Close)
	return r
}
func (r *workerHTTPRuntime) ServerIdentity() string { return r.server.URL }
func (r *workerHTTPRuntime) Extract(ctx context.Context, _ memory.CompilerGeneration, request memory.CompilerRequest) (CompilerExtraction, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.server.URL+"/extract", strings.NewReader(string(compilerJSON(request))))
	if err != nil {
		return CompilerExtraction{}, err
	}
	req.Header.Set("X-Attempt-ID", request.AttemptID)
	response, err := r.server.Client().Do(req)
	if err != nil {
		return CompilerExtraction{}, err
	}
	defer response.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return CompilerExtraction{}, err
	}
	return CompilerExtraction{Raw: raw, ReleaseEvidence: "completed"}, nil
}
func (r *workerHTTPRuntime) VerifyCompilerRelease(ctx context.Context, reservation CompilerCapacityReservation) (CompilerReleaseAcknowledgement, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.server.URL+"/status", nil)
	if err != nil {
		return CompilerReleaseAcknowledgement{}, err
	}
	req.Header.Set("X-Attempt-ID", reservation.RequestID)
	req.Header.Set("Authorization", "Bearer "+r.token)
	response, err := r.server.Client().Do(req)
	if err != nil {
		return CompilerReleaseAcknowledgement{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return CompilerReleaseAcknowledgement{}, fmt.Errorf("status denied")
	}
	var status struct {
		AttemptID string `json:"attempt_id"`
		Completed bool   `json:"completed"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		return CompilerReleaseAcknowledgement{}, err
	}
	if !status.Completed || status.AttemptID != reservation.RequestID || reservation.ServerIdentity != r.ServerIdentity() {
		return CompilerReleaseAcknowledgement{}, errors.New("request still active")
	}
	return CompilerReleaseAcknowledgement{Reservation: reservation, Kind: "request_completed"}, nil
}

func TestCompilerWorkerHTTPUnknownReleaseBlocksOtherStore(t *testing.T) {
	f := newWorkerFixture(t)
	first := f.queue(t, "I prefer tea.")
	second := f.queue(t, "I prefer coffee.")
	runtime := newWorkerHTTPRuntime(t)
	other := f.second(t)
	ctx := context.Background()
	finished := make(chan error, 1)
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(runtime.release) }) })
	go func() { _, err := f.store.RunCompilerStep(ctx, f.config(runtime)); finished <- err }()
	var attemptID string
	select {
	case attemptID = <-runtime.started:
	case <-time.After(time.Second):
		t.Fatal("no server request")
	}
	start := time.Now()
	if _, err := other.CancelCompilation(ctx, f.owner, first.JobID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("cancelled worker succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("client did not cancel promptly")
	}
	if time.Since(start) >= time.Second {
		t.Fatal("client cancellation missed deadline")
	}
	config := f.config(runtime)
	config.ReleaseVerifier = runtime
	if worked, err := other.RunCompilerStep(ctx, config); worked || !errors.Is(err, ErrCompilerCapacityBlocked) {
		t.Fatalf("second request while server still active %v %v", worked, err)
	}
	if err := other.ReconcileCompilerCapacity(ctx, nil); !errors.Is(err, ErrCompilerCapacityBlocked) {
		t.Fatalf("missing verifier released capacity %v", err)
	}
	status, err := other.InspectCompilerStatus(ctx, f.owner, "", 64)
	if err != nil || status.CapacityState != "capacity_blocked" {
		t.Fatalf("blocked status %+v %v", status, err)
	}
	runtime.mu.Lock()
	active, calls, peak := runtime.active, runtime.calls, runtime.peak
	runtime.mu.Unlock()
	if active != 1 || calls != 1 || peak != 1 || attemptID == "" {
		t.Fatalf("request overlap active=%d calls=%d peak=%d", active, calls, peak)
	}
	releaseOnce.Do(func() { close(runtime.release) })
	deadline := time.Now().Add(time.Second)
	for {
		runtime.mu.Lock()
		done := runtime.finished[attemptID]
		runtime.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server failed to finish")
		}
		time.Sleep(time.Millisecond)
	}
	if worked, err := other.RunCompilerStep(ctx, config); !worked || err != nil {
		t.Fatalf("verified release did not advance later work %v %v", worked, err)
	}
	got, err := other.InspectCompilation(ctx, f.owner, second.JobID)
	if err != nil || got.State != "completed_empty" {
		t.Fatalf("later %+v %v", got, err)
	}
	firstGot, err := other.InspectCompilation(ctx, f.owner, first.JobID)
	if err != nil || firstGot.State != "cancelled" || len(firstGot.Candidates) != 0 {
		t.Fatalf("stale bytes changed cancelled job %+v %v", firstGot, err)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.peak != 1 || runtime.calls != 2 {
		t.Fatalf("observed request counts peak=%d calls=%d", runtime.peak, runtime.calls)
	}
}
