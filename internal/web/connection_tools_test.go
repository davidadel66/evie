package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/mcpchat"
	"github.com/davidadel66/evie/internal/mcpconn"
	"github.com/davidadel66/evie/internal/memory"
)

type fakeToolReview struct {
	set []string
	err error
}

func (f *fakeToolReview) Review(_ context.Context, ws memory.WorkspaceID, name string) (mcpchat.Review, error) {
	if f.err != nil {
		return mcpchat.Review{}, f.err
	}
	return mcpchat.Review{Connection: mcpconn.Connection{Name: name}, AccessRevision: 3, Tools: []mcpchat.ReviewedTool{
		{Tool: mcpconn.Tool{Name: "make_api_request_read", Hash: "r1"}, State: mcpchat.ToolEnabled}}}, nil
}

func (f *fakeToolReview) SetToolEnabled(_ context.Context, ws memory.WorkspaceID, name, tool string, enabled bool) (mcpchat.Review, error) {
	if f.err != nil {
		return mcpchat.Review{}, f.err
	}
	state := "disable"
	if enabled {
		state = "enable"
	}
	f.set = append(f.set, string(ws)+" "+name+" "+tool+" "+state)
	return f.Review(context.Background(), ws, name)
}

func TestConnectionToolReviewRoutes(t *testing.T) {
	review := &fakeToolReview{}
	handler := WithMCPToolReview(NewServer(nil), review).Handler()
	call := func(route, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, managementRequest(route, body))
		return rec
	}
	if rec := call("/api/connections/tools", `{"workspace_id":"ws-1","name":"square"}`); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"state":"enabled"`) || !strings.Contains(rec.Body.String(), `"access_revision":3`) {
		t.Fatalf("tools = %d %s", rec.Code, rec.Body)
	}
	if rec := call("/api/connections/tools/set", `{"workspace_id":"ws-1","name":"square","tool":"make_api_request_read","enabled":true}`); rec.Code != http.StatusOK ||
		len(review.set) != 1 || review.set[0] != "ws-1 square make_api_request_read enable" {
		t.Fatalf("set = %d %s, %v", rec.Code, rec.Body, review.set)
	}
	if rec := call("/api/connections/tools/set", `{"workspace_id":"ws-1","name":"square","tool":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("set without enabled = %d", rec.Code)
	}
	for err, code := range map[error]int{
		eviedb.ErrWorkspaceAccessChanged: http.StatusConflict,
		eviedb.ErrWorkspaceInactive:      http.StatusConflict,
		eviedb.ErrWorkspaceNotFound:      http.StatusNotFound,
		mcpchat.ErrToolNotOffered:        http.StatusBadRequest,
		mcpchat.ErrToolUnselectable:      http.StatusBadRequest,
		mcpconn.ErrNotConnected:          http.StatusConflict,
		mcpconn.ErrConnectionNotFound:    http.StatusNotFound,
	} {
		review.err = err
		if rec := call("/api/connections/tools/set", `{"workspace_id":"ws-1","name":"square","tool":"x","enabled":false}`); rec.Code != code {
			t.Fatalf("%v = %d %s, want %d", err, rec.Code, rec.Body, code)
		}
	}
}
