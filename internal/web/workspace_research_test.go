package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
)

type researchWorkspaceController struct {
	fakeContextSessionController
	id       memory.WorkspaceID
	revision memory.WorkspaceRevisionID
	enabled  bool
	err      error
}

func (c *researchWorkspaceController) SetWorkspaceResearch(_ context.Context, id memory.WorkspaceID, revision memory.WorkspaceRevisionID, enabled bool) (memory.Workspace, error) {
	c.id, c.revision, c.enabled = id, revision, enabled
	return memory.Workspace{ID: id, CurrentRevisionID: "next"}, c.err
}
func TestWorkspaceResearchHTTPAllowsRevocationDuringChatAndRejectsStaleEdits(t *testing.T) {
	c := &researchWorkspaceController{}
	s := NewContextServer(nil, nil, nil, c)
	s.activeTurns = 1
	handler := s.Handler()
	request := `{"workspaceId":"workspace","revision":"revision","enabled":false}`
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/research", request))
	if w.Code != http.StatusOK || c.id != "workspace" || c.revision != "revision" || c.enabled {
		t.Fatalf("revocation=%d %s", w.Code, w.Body)
	}
	for _, tc := range []struct {
		err    error
		status int
	}{{eviedb.ErrChooserStateChanged, 409}, {errors.New("secret private path"), 422}} {
		c.err = tc.err
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/research", request))
		if w.Code != tc.status || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("error=%d %s", w.Code, w.Body)
		}
	}
	for _, request := range []string{`{"workspaceId":"workspace","revision":"revision"}`, `{"workspaceId":"workspace","enabled":true}`} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/research", request))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("missing explicit fields=%d", w.Code)
		}
	}
}
