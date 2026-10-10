package web_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflow"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// workspace-activity.spec.md story 2: the run list carries each run's step
// states for its step bar, and a run's own pinned workflow.yaml is outlined
// for its panel, so neither needs a full inspection per row.
func TestWorkflowRunsCarryStepStatesAndARunsDefinitionOutline(t *testing.T) {
	ctx := context.Background()
	_, rt, w := workflowRuntimeForWeb(t)
	f, err := workflow.ParseFixtures([]byte(`{"inputs": {"day": "2026-10-05", "at": "07:00", "total_cents": 1000, "people": 3}}`))
	if err != nil {
		t.Fatal(err)
	}
	card, err := rt.Propose(ctx, workflowruntime.Proposal{WorkspaceID: w.ID, Dir: filepath.Join("..", "workflow", "testdata", "scheduled-split"), Fixtures: &f, Surface: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Approve(ctx, workflowruntime.Approval{ProposalID: card.Proposal.ID, ExecutableHash: card.Proposal.ExecutableHash, AuthorityHash: card.Proposal.AuthorityHash, Surface: "cli"}); err != nil {
		t.Fatal(err)
	}
	started, err := rt.StartRun(ctx, workflowruntime.RunRequest{WorkspaceID: w.ID, Workflow: "scheduled-split", Surface: "cli",
		TextInputs: map[string]string{"day": "2026-10-05", "at": "07:00", "total_cents": "1000", "people": "3"}})
	if err != nil {
		t.Fatal(err)
	}
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()

	resp := workflowPost(t, h, "runs", map[string]any{"workspace_id": w.ID})
	var listed struct {
		Runs []struct {
			ID         string            `json:"id"`
			Workflow   string            `json:"workflow"`
			State      string            `json:"state"`
			StepStates map[string]string `json:"step_states"`
		} `json:"runs"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Runs) != 1 {
		t.Fatalf("runs = %d %s", resp.Code, resp.Body)
	}
	if run := listed.Runs[0]; run.ID != started.Run.Run.ID || run.State != "succeeded" || len(run.StepStates) != 1 || run.StepStates["split"] != "succeeded" {
		t.Fatalf("run row = %+v", run)
	}

	resp = workflowPost(t, h, "definition", map[string]any{"run_id": started.Run.Run.ID})
	var def struct {
		WorkspaceID string                 `json:"workspace_id"`
		Name        string                 `json:"name"`
		Version     int64                  `json:"version"`
		Source      string                 `json:"source"`
		Steps       []workflow.StepOutline `json:"steps"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &def) != nil {
		t.Fatalf("definition = %d %s", resp.Code, resp.Body)
	}
	if def.WorkspaceID != string(w.ID) || def.Name != "scheduled-split" || def.Version != started.Run.Run.Version ||
		!strings.HasPrefix(def.Source, "# A scheduled deterministic example") || len(def.Steps) != 1 {
		t.Fatalf("definition = %+v", def)
	}
	if s := def.Steps[0]; s.ID != "split" || s.Kind != "deterministic" || s.FirstLine != 54 || s.LastLine != 59 {
		t.Fatalf("step = %+v", s)
	}

	if resp := workflowPost(t, h, "definition", map[string]any{"run_id": "no-such-run"}); resp.Code != http.StatusNotFound {
		t.Fatalf("unknown run = %d %s", resp.Code, resp.Body)
	}
	for name, body := range map[string]any{
		"no run":        map[string]any{},
		"a long run id": map[string]any{"run_id": strings.Repeat("r", 129)},
		"another field": map[string]any{"run_id": "r", "path": "prompts/propose.md"},
	} {
		if resp := workflowPost(t, h, "definition", body); resp.Code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", name, resp.Code, resp.Body)
		}
	}
}
