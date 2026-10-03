package localextractor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/localextractor"
)

// The memory background pass (Stage 4) can run on the configured local
// model: one JSON chat request, after metadata-only verification of the
// pinned configuration, bounded by the generation's output allowance.
func TestBackgroundChatUsesThePinnedLocalModelWithJSONOutput(t *testing.T) {
	tokenizer := map[string]any{"tokenizer.ggml.model": "gpt2"}
	g := generation(tokenizer)
	var chats int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]any{"version": g.RuntimeVersion})
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": g.ModelArtifact, "digest": g.ModelManifestSHA256}}})
		case "/api/show":
			json.NewEncoder(w).Encode(map[string]any{"template": g.Template, "system": "", "details": map[string]any{"quantization_level": g.Quantization}, "model_info": tokenizer})
		case "/api/chat":
			chats++
			var body struct {
				Model    string
				Stream   bool
				Format   string
				Messages []map[string]string
				Options  map[string]any
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != g.ModelArtifact || body.Stream || body.Format != "json" || len(body.Messages) != 2 ||
				body.Messages[0]["role"] != "system" || body.Messages[1]["content"] != "owner messages" || body.Options["num_predict"] != float64(768) {
				t.Errorf("chat request = %+v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"model": g.ModelArtifact, "done": true, "done_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": `{"memories":[]}`}, "prompt_eval_count": 120, "eval_count": 9})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	adapter, err := localextractor.New(localextractor.Config{Endpoint: server.URL, Generation: g})
	if err != nil {
		t.Fatal(err)
	}
	text, input, output, err := adapter.Chat(context.Background(), g, "system", "owner messages", 4000)
	if err != nil || text != `{"memories":[]}` || input != 120 || output != 9 || chats != 1 {
		t.Fatalf("chat = %q %d %d %v (calls %d)", text, input, output, err, chats)
	}
	other := g
	other.Prompt = "a different generation"
	if _, _, _, err := adapter.Chat(context.Background(), other, "s", "u", 10); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("a different generation was accepted: %v", err)
	}
}
