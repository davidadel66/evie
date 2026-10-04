package web_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// The owner cancels a run from the web (Stage 14 sub-stage 8) and reruns a
// succeeded logical execution with an audited override; the surface is
// always the web. Malformed requests, unknown runs, finished runs and
// refused overrides answer with their reason.
func TestWorkflowWebCancelsRunsAndRerunsWithAnOverride(t *testing.T) {
	_, rt, w := workflowRuntimeForWeb(t)
	splitFixtures, err := os.ReadFile(filepath.Join(webTestdata, "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	approveForWeb(t, rt, w.ID, "split-bill", splitFixtures)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	start := map[string]any{"workspace_id": w.ID, "name": "split-bill", "inputs": map[string]any{"bill_id": "dinner-1", "total_cents": 1000, "people": 3}, "detach": true}
	resp := workflowPost(t, h, "start", start)
	var queued workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &queued) != nil || queued.Run.Run.State != eviedb.WorkflowRunQueued {
		t.Fatalf("start = %d %s", resp.Code, resp.Body)
	}
	id := queued.Run.Run.ID
	for name, body := range map[string]map[string]any{
		"no run":         {"reason": "x"},
		"long reason":    {"run_id": id, "reason": strings.Repeat("x", 1001)},
		"surface chosen": {"run_id": id, "surface": "chat"},
	} {
		if resp := workflowPost(t, h, "cancel", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
	if resp := workflowPost(t, h, "cancel", map[string]any{"run_id": "missing"}); resp.Code != http.StatusNotFound {
		t.Fatalf("missing run = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "cancel", map[string]any{"run_id": id, "reason": "entered twice"})
	var cancelled eviedb.WorkflowRunDetail
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &cancelled) != nil || cancelled.Run.State != eviedb.WorkflowRunCancelled ||
		cancelled.Cancellation == nil || cancelled.Cancellation.Surface != "web" || cancelled.Cancellation.Reason != "entered twice" {
		t.Fatalf("cancel = %d %s", resp.Code, resp.Body)
	}
	delete(start, "detach")
	resp = workflowPost(t, h, "start", start)
	var first workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &first) != nil || first.Existing || first.Run.Run.State != eviedb.WorkflowRunSucceeded {
		t.Fatalf("start of the freed key = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "cancel", map[string]any{"run_id": first.Run.Run.ID}); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "run_finished") {
		t.Fatalf("cancelling a succeeded run = %d %s", resp.Code, resp.Body)
	}

	rerun := map[string]any{"workspace_id": w.ID, "name": "split-bill", "rerun_of": first.Run.Run.ID}
	if resp := workflowPost(t, h, "start", rerun); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "rerun_refused") {
		t.Fatalf("a rerun without a reason = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "start", map[string]any{"workspace_id": w.ID, "name": "split-bill", "rerun_of": first.Run.Run.ID, "reason": strings.Repeat("x", 1001)}); resp.Code != http.StatusBadRequest {
		t.Fatalf("a long reason = %d %s", resp.Code, resp.Body)
	}
	rerun["reason"] = "the bill was corrected"
	resp = workflowPost(t, h, "start", rerun)
	var second workflowruntime.RunResult
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &second) != nil || second.Existing || second.Run.Run.State != eviedb.WorkflowRunSucceeded ||
		second.Run.Override == nil || second.Run.Override.RerunOf != first.Run.Run.ID || second.Run.Override.Surface != "web" || second.Run.Run.RunKey == first.Run.Run.RunKey {
		t.Fatalf("rerun = %d %s", resp.Code, resp.Body)
	}
}
