package web_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/web"
	"github.com/davidadel66/evie/internal/workflowruntime"
)

// Built-in workflows (2026-10-04) on the web management API: the built-ins
// are listed with what a run of each would pin in a Workspace, and the
// owner turns one off and on for every Workspace from the web.
func TestWorkflowWebBuiltinsListAndToggle(t *testing.T) {
	_, rt, w := workflowRuntimeForWeb(t)
	h := web.WithWorkflows(web.NewServer(nil), rt).Handler()
	resp := workflowPost(t, h, "builtins", map[string]any{"workspace_id": w.ID})
	var listed struct {
		Builtins []workflowruntime.BuiltinView `json:"builtins"`
	}
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || len(listed.Builtins) != 1 {
		t.Fatalf("builtins = %d %s", resp.Code, resp.Body)
	}
	b := listed.Builtins[0]
	if b.Name != "deep-research" || !b.Enabled || b.Workspace == nil || b.Workspace.Source != "builtin" || !strings.Contains(strings.Join(b.Workspace.Blocked, " "), "research delegation") {
		t.Fatalf("deep-research = %+v", b)
	}
	for _, bad := range []map[string]any{{"name": "deep-research"}, {"enabled": false}, {"name": "deep-research", "enabled": false, "extra": 1}} {
		if resp := workflowPost(t, h, "builtins/control", bad); resp.Code != http.StatusBadRequest {
			t.Fatalf("control %v = %d %s", bad, resp.Code, resp.Body)
		}
	}
	resp = workflowPost(t, h, "builtins/control", map[string]any{"name": "deep-research", "enabled": false, "reason": "not now"})
	var control eviedb.WorkflowBuiltinControl
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &control) != nil || control.Enabled || control.Surface != "web" || control.Reason != "not now" {
		t.Fatalf("disable = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "builtins/control", map[string]any{"name": "deep-research", "enabled": false}); resp.Code != http.StatusConflict || !strings.Contains(resp.Body.String(), "builtin_unchanged") {
		t.Fatalf("disabling twice = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "builtins/control", map[string]any{"name": "no-such", "enabled": false}); resp.Code != http.StatusNotFound {
		t.Fatalf("unknown built-in = %d %s", resp.Code, resp.Body)
	}
	resp = workflowPost(t, h, "builtins", map[string]any{})
	listed.Builtins = nil
	if resp.Code != http.StatusOK || json.Unmarshal(resp.Body.Bytes(), &listed) != nil || listed.Builtins[0].Enabled || listed.Builtins[0].Workspace != nil {
		t.Fatalf("builtins while disabled = %d %s", resp.Code, resp.Body)
	}
	if resp := workflowPost(t, h, "builtins/control", map[string]any{"name": "deep-research", "enabled": true}); resp.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", resp.Code, resp.Body)
	}
	// Management guard: a foreign origin is refused.
	if resp := workflowPost(t, h, "builtins/control", map[string]any{"name": "deep-research", "enabled": false}, func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }); resp.Code == http.StatusOK {
		t.Fatalf("foreign origin = %d %s", resp.Code, resp.Body)
	}
}
