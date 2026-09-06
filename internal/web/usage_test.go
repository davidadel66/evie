package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/usage"
)

type usageReaderStub struct {
	calls  int
	period usage.Period
}

func (stub *usageReaderStub) InspectUsage(_ context.Context, p usage.Period) (usage.Report, error) {
	stub.calls++
	stub.period = p
	return usage.Report{Period: p, Sources: []usage.Source{{ID: "evie", Label: "Evie", State: "partial"}}}, nil
}
func TestUsageHTTPGuardsValidateBeforeReading(t *testing.T) {
	reader := &usageReaderStub{}
	handler := WithUsage(NewServer(nil), reader).Handler()
	body := `{"from":"2026-09-01","to":"2026-09-07","timezone":"America/Detroit"}`
	for _, test := range []struct {
		name, body, method, origin, content string
		status                              int
	}{
		{"valid", body, "POST", "", "application/json", 200},
		{"method", body, "GET", "", "application/json", 405},
		{"origin", body, "POST", "https://example.com", "application/json", 403},
		{"content", body, "POST", "", "text/plain", 403},
		{"unknown", `{"from":"2026-09-01","to":"2026-09-07","timezone":"UTC","path":"secret"}`, "POST", "", "application/json", 400},
		{"wide", `{"from":"2026-01-01","to":"2026-09-07","timezone":"UTC"}`, "POST", "", "application/json", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := reader.calls
			request := httptest.NewRequest(test.method, "http://localhost/api/data/usage/summary", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.content)
			request.Header.Set("Origin", test.origin)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, request)
			if rec.Code != test.status {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			if test.status == 200 {
				if reader.calls != before+1 || rec.Header().Get("Cache-Control") != "no-store" || reader.period.Timezone != "America/Detroit" {
					t.Fatal("valid query not forwarded")
				}
			} else if reader.calls != before {
				t.Fatal("invalid query reached reader")
			}
		})
	}
}
