package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/tools"
)

// The approval card's "Always applies" switch answers with the prepared
// alternative; a plain answer keeps its meaning.
func TestApprovalAnswerCarriesTheCardAlternative(t *testing.T) {
	srv, h := newTestServerFull(&fakeClient{})
	for _, tc := range []struct {
		body map[string]any
		want tools.Decision
	}{
		{map[string]any{"approve": true, "alternative": true}, tools.ApprovedAlternative},
		{map[string]any{"approve": true}, tools.Approved},
		{map[string]any{"approve": false, "alternative": true}, tools.Declined},
	} {
		id, ch := srv.newPending()
		tc.body["id"] = id
		raw, _ := json.Marshal(tc.body)
		req := httptest.NewRequest("POST", "http://127.0.0.1:6687/api/approve", strings.NewReader(string(raw)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("approve %v status = %d", tc.body, rec.Code)
		}
		if got := <-ch; got != tc.want {
			t.Fatalf("answer %v reached the approver as %v, want %v", tc.body, got, tc.want)
		}
	}
}
