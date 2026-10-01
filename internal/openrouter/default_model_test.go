package openrouter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/openrouter"
)

// C7: whatever the application default is, startup must survive a metadata
// outage for it with an explicit, checked-in fallback window.
func TestApplicationDefaultModelHasBuiltinContextFallback(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "metadata outage", http.StatusBadGateway)
	}))
	t.Cleanup(server.Close)
	client, err := openrouter.NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	openrouter.SetMetadataEndpointForTest(client, server.URL, time.Second)
	profile, err := client.ResolveContextProfile(context.Background(), agent.DefaultModel)
	if err != nil {
		t.Fatalf("startup profile for default model %q failed: %v", agent.DefaultModel, err)
	}
	if d := profile.Diagnostics(); d.Source != openrouter.ContextProfileBuiltinFallback || d.HardWindowTokens <= 0 {
		t.Fatalf("diagnostics=%+v", d)
	}
}
