package openrouter

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

// A chat open resolves its model's profile; a recently resolved model is
// reused instead of asking OpenRouter again (multi-chat.spec.md Stage 1).
func TestResolveChatContextProfileReusesARecentLookup(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")
	var requests atomic.Int64
	var failing atomic.Bool
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		time.Sleep(85 * time.Millisecond) // about one OpenRouter metadata request
		if failing.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.URL.Path {
		case "/api/v1/model/deepseek/deepseek-v4.1-flash", "/api/v1/model/moonshotai/kimi-k3":
			model := r.URL.Path[len("/api/v1/model/"):]
			_, _ = w.Write([]byte(`{"data":{"id":"` + model + `","canonical_slug":"` + model + `","context_length":1048576}}`))
		case "/api/v1/models/deepseek/deepseek-v4.1-flash/endpoints", "/api/v1/models/moonshotai/kimi-k3/endpoints":
			model := r.URL.Path[len("/api/v1/models/") : len(r.URL.Path)-len("/endpoints")]
			_, _ = w.Write([]byte(`{"data":{"id":"` + model + `","endpoints":[{"context_length":1000000,"max_completion_tokens":393216,"status":0,"supported_parameters":["max_tokens","tools","tool_choice","reasoning"]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	client.profiles = newProfileCache(func() time.Time { return now })
	ctx := context.Background()
	resolve := func(model string) (ContextProfile, int64, time.Duration) {
		t.Helper()
		before, start := requests.Load(), time.Now()
		profile, err := client.ResolveChatContextProfile(ctx, model)
		if err != nil {
			t.Fatal(err)
		}
		return profile, requests.Load() - before, time.Since(start)
	}

	first, made, firstTook := resolve("deepseek/deepseek-v4.1-flash")
	if made != 2 || first.Diagnostics().Source != ContextProfileRemoteMetadata {
		t.Fatalf("first open made %d requests, source %s", made, first.Diagnostics().Source)
	}
	again, made, againTook := resolve("deepseek/deepseek-v4.1-flash")
	if made != 0 || again != first {
		t.Fatalf("second open made %d requests or changed profile", made)
	}
	t.Logf("open chat profile: first %v, reused %v", firstTook.Round(time.Millisecond), againTook.Round(time.Microsecond))
	if againTook >= firstTook/4 {
		t.Fatalf("reused profile took %v, first lookup %v", againTook, firstTook)
	}
	if _, made, _ = resolve("moonshotai/kimi-k3"); made != 2 {
		t.Fatalf("another model made %d requests", made)
	}
	// The non-chat profile has different limits, so it is looked up on its own.
	before := requests.Load()
	if _, err := client.ResolveContextProfile(ctx, "deepseek/deepseek-v4.1-flash"); err != nil || requests.Load()-before != 2 {
		t.Fatalf("non-chat profile: %d requests, %v", requests.Load()-before, err)
	}

	now = now.Add(profileReuse)
	if _, made, _ = resolve("deepseek/deepseek-v4.1-flash"); made != 2 {
		t.Fatalf("expired profile made %d requests", made)
	}

	// A failed lookup falls back to the built-in window and is not kept, so
	// the next open asks again and gets the real metadata.
	now = now.Add(profileReuse)
	failing.Store(true)
	fallback, _, _ := resolve("deepseek/deepseek-v4.1-flash")
	if fallback.Diagnostics().Source != ContextProfileBuiltinFallback {
		t.Fatalf("failed lookup source %s", fallback.Diagnostics().Source)
	}
	failing.Store(false)
	recovered, made, _ := resolve("deepseek/deepseek-v4.1-flash")
	if made != 2 || recovered.Diagnostics().Source != ContextProfileRemoteMetadata {
		t.Fatalf("after a failure: %d requests, source %s", made, recovered.Diagnostics().Source)
	}
}
