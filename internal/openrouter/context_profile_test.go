package openrouter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testBuiltinModel = "moonshotai/kimi-k3"

func contextProfileClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewClient("profile-key")
	if err != nil {
		t.Fatal(err)
	}
	client.apiBaseURL = server.URL + "/api/v1"
	return client, server
}

func TestResolveContextProfileUsesCanonicalRouteSafeWindow(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")

	var requests []string
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer profile-key" {
			t.Errorf("Authorization=%q", got)
		}
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/model/vendor/alias":
			_, _ = w.Write([]byte(`{"data":{"id":"vendor/advertised","canonical_slug":"vendor/canonical","context_length":1000000}}`))
		case "/api/v1/models/vendor/canonical/endpoints":
			_, _ = w.Write([]byte(`{"data":{"id":"vendor/canonical","endpoints":[` +
				`{"context_length":500000,"max_completion_tokens":50000,"status":0,"supported_parameters":["max_tokens"]},` +
				`{"context_length":400000,"max_completion_tokens":20000,"status":0,"supported_parameters":["max_tokens"]},` +
				`{"context_length":100000,"max_completion_tokens":50000,"status":1,"supported_parameters":["max_tokens"]},` +
				`{"context_length":90000,"max_completion_tokens":10000,"status":0,"supported_parameters":["max_tokens"]},` +
				`{"context_length":80000,"max_completion_tokens":50000,"status":0,"supported_parameters":["temperature"]}` +
				`]}}`))
		default:
			http.NotFound(w, r)
		}
	}))

	profile, err := client.ResolveContextProfile(context.Background(), "vendor/alias")
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := profile.Diagnostics()
	if diagnostics.ConfiguredModel != "vendor/alias" || diagnostics.AdvertisedModel != "vendor/advertised" ||
		diagnostics.CanonicalModel != "vendor/canonical" {
		t.Fatalf("identity diagnostics=%+v", diagnostics)
	}
	if diagnostics.AdvertisedWindowTokens != 1000000 || diagnostics.HardWindowTokens != 400000 ||
		diagnostics.WorkingTokens != 262144 || diagnostics.OutputReserveTokens != 16384 ||
		diagnostics.EstimationMarginTokens != 4096 || diagnostics.Source != ContextProfileRemoteMetadata {
		t.Fatalf("context diagnostics=%+v", diagnostics)
	}
	if got, want := strings.Join(requests, ","), "/api/v1/model/vendor/alias,/api/v1/models/vendor/canonical/endpoints"; got != want {
		t.Fatalf("requests=%q, want %q", got, want)
	}
}

func TestResolveContextProfileExplicitOverrideSkipsDiscovery(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "300000")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "200000")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "12000")

	var calls atomic.Int32
	client, _ := contextProfileClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	profile, err := client.ResolveContextProfile(context.Background(), "custom/model")
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := profile.Diagnostics()
	if calls.Load() != 0 {
		t.Fatalf("metadata calls=%d, want 0", calls.Load())
	}
	if diagnostics.Source != ContextProfileExplicitOverride || diagnostics.HardWindowTokens != 300000 ||
		diagnostics.WorkingTokens != 200000 || diagnostics.OutputReserveTokens != 12000 ||
		diagnostics.CanonicalModel != "" || diagnostics.AdvertisedWindowTokens != 0 {
		t.Fatalf("override diagnostics=%+v", diagnostics)
	}
}

func TestResolveContextProfileBuiltinFallback(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")

	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"canonical_slug":"moonshotai/kimi-k3","context_length":"broken"}}`))
	}))
	profile, err := client.ResolveContextProfile(context.Background(), testBuiltinModel)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := profile.Diagnostics()
	if diagnostics.Source != ContextProfileBuiltinFallback || diagnostics.HardWindowTokens != 262144 ||
		diagnostics.CanonicalModel != testBuiltinModel || diagnostics.AdvertisedWindowTokens != 0 {
		t.Fatalf("fallback diagnostics=%+v", diagnostics)
	}
}

func TestResolveContextProfileUnknownModelFailsWithoutOverride(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing", http.StatusNotFound)
	}))
	_, err := client.ResolveContextProfile(context.Background(), "custom/model")
	if err == nil || !strings.Contains(err.Error(), "custom/model") {
		t.Fatalf("error=%v", err)
	}
}

func TestResolveContextProfileHonorsCallerCancellation(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	started := make(chan struct{})
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.ResolveContextProfile(ctx, testBuiltinModel)
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func TestResolveContextProfileTimesOutDiscovery(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	client.contextDiscoveryTimeout = 20 * time.Millisecond
	profile, err := client.ResolveContextProfile(context.Background(), testBuiltinModel)
	if err != nil {
		t.Fatal(err)
	}
	if got := profile.Diagnostics().Source; got != ContextProfileBuiltinFallback {
		t.Fatalf("source=%q, want %q", got, ContextProfileBuiltinFallback)
	}
}

func TestNewClientUsesThreeSecondContextDiscoveryTimeout(t *testing.T) {
	client, err := NewClient("key")
	if err != nil {
		t.Fatal(err)
	}
	if client.contextDiscoveryTimeout != 3*time.Second {
		t.Fatalf("context discovery timeout=%s, want 3s", client.contextDiscoveryTimeout)
	}
}

func TestResolveContextProfileRejectsInvalidConfiguration(t *testing.T) {
	maxInt64 := strconv.FormatInt(int64(^uint64(0)>>1), 10)
	tests := []struct {
		name, hard, working, reserve string
	}{
		{name: "zero hard", hard: "0"},
		{name: "negative hard", hard: "-1"},
		{name: "hard overflow", hard: "9223372036854775808"},
		{name: "zero working", hard: "300000", working: "0"},
		{name: "working above hard", hard: "200000", working: "200001"},
		{name: "zero reserve", hard: "300000", working: "200000", reserve: "0"},
		{name: "reserve plus margin equals working", hard: "300000", working: "20000", reserve: "15904"},
		{name: "reserve plus margin overflows", hard: maxInt64, working: maxInt64, reserve: maxInt64},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", tt.hard)
			t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", tt.working)
			t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", tt.reserve)
			client, _ := contextProfileClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("invalid explicit configuration performed discovery")
			}))
			if _, err := client.ResolveContextProfile(context.Background(), "custom/model"); err == nil {
				t.Fatal("ResolveContextProfile succeeded")
			}
		})
	}
}

func TestResolveContextProfileRejectsMalformedEndpointMetadata(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/model/custom/model":
			_, _ = w.Write([]byte(`{"data":{"id":"custom/model","canonical_slug":"custom/model","context_length":300000}}`))
		case "/api/v1/models/custom/model/endpoints":
			_, _ = w.Write([]byte(`{"data":{"id":"custom/model","endpoints":[{"context_length":0,"max_completion_tokens":20000,"status":0,"supported_parameters":["max_tokens"]}]}}`))
		}
	}))
	if _, err := client.ResolveContextProfile(context.Background(), "custom/model"); err == nil {
		t.Fatal("malformed endpoint metadata succeeded")
	}
}

func TestResolveContextProfileRejectsMalformedCompletionLimits(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	tests := []struct {
		name  string
		limit string
	}{
		{name: "missing", limit: "null"},
		{name: "zero", limit: "0"},
		{name: "negative", limit: "-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/model/custom/model":
					_, _ = w.Write([]byte(`{"data":{"id":"custom/model","canonical_slug":"custom/model","context_length":300000}}`))
				case "/api/v1/models/custom/model/endpoints":
					_, _ = w.Write([]byte(`{"data":{"id":"custom/model","endpoints":[` +
						`{"context_length":280000,"max_completion_tokens":` + tt.limit + `,"status":0,"supported_parameters":["max_tokens"]},` +
						`{"context_length":300000,"max_completion_tokens":20000,"status":0,"supported_parameters":["max_tokens"]}` +
						`]}}`))
				}
			}))
			if _, err := client.ResolveContextProfile(context.Background(), "custom/model"); err == nil {
				t.Fatal("malformed completion limit succeeded")
			}
		})
	}
}

func TestResolveContextProfileRejectsMalformedAdvertisedIdentity(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/model/custom/model":
			_, _ = w.Write([]byte(`{"data":{"id":"not-a-model-identity","canonical_slug":"custom/model","context_length":300000}}`))
		case "/api/v1/models/custom/model/endpoints":
			_, _ = w.Write([]byte(`{"data":{"id":"custom/model","endpoints":[{"context_length":300000,"max_completion_tokens":20000,"status":0,"supported_parameters":["max_tokens"]}]}}`))
		}
	}))
	if _, err := client.ResolveContextProfile(context.Background(), "custom/model"); err == nil {
		t.Fatal("malformed advertised identity succeeded")
	}
}

func TestResolveContextProfileRejectsWorkingCeilingAboveDiscoveredHardWindow(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "262144")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/model/moonshotai/kimi-k3":
			_, _ = w.Write([]byte(`{"data":{"id":"moonshotai/kimi-k3","canonical_slug":"moonshotai/kimi-k3","context_length":300000}}`))
		case "/api/v1/models/moonshotai/kimi-k3/endpoints":
			_, _ = w.Write([]byte(`{"data":{"id":"moonshotai/kimi-k3","endpoints":[{"context_length":200000,"max_completion_tokens":20000,"status":0,"supported_parameters":["max_tokens"]}]}}`))
		}
	}))
	profile, err := client.ResolveContextProfile(context.Background(), testBuiltinModel)
	if err == nil {
		t.Fatalf("profile=%+v, want inconsistent working ceiling error", profile.Diagnostics())
	}
}
