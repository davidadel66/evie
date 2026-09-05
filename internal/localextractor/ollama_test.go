package localextractor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

func generation(tokenizer map[string]any) memory.CompilerGeneration {
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "synthetic:fixture", ModelSHA256: strings.Repeat("1", 64), Quantization: "Q4_K_M", RuntimeVersion: "0.6.3", ProtocolVersion: "ollama-generate-v1", Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract only supported owner assertions.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
	g.ModelManifest = json.RawMessage(`{"schemaVersion":2,"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	encoded, _ := json.Marshal(tokenizer)
	g.TokenizerSHA256 = memory.CompilerHash(encoded)
	g.EvidencePolicy = memory.CompilerPolicyVersion
	g.SecretPolicy = memory.CompilerPolicyVersion
	g.ClosurePolicy = memory.CompilerPolicyVersion
	g.WindowPolicy = memory.CompilerPolicyVersion
	g.PredicatePolicy = memory.CompilerPolicyVersion
	g.EntityPolicy = memory.CompilerPolicyVersion
	g.ValidationPolicy = memory.CompilerPolicyVersion
	g.EquivalencePolicy = memory.CompilerPolicyVersion
	g.EffectPolicy = memory.CompilerPolicyVersion
	return g
}
func request(g memory.CompilerGeneration) memory.CompilerRequest {
	id, _, _ := memory.CompilerGenerationIdentity(g)
	return memory.CompilerRequest{ID: "sealed-request", GenerationID: id, Window: memory.CompilerWindow{Sources: []memory.CompilerSource{{Evidence: "café"}}}}
}

func TestPinnedLocalTransportHandlesManifestAndBoundedVerboseMetadata(t *testing.T) {
	tokenizer := map[string]any{"tokenizer.ggml.tokens": []string{strings.Repeat("fixture", 30000)}, "tokenizer.ggml.model": "gpt2"}
	g := generation(tokenizer)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]any{"version": g.RuntimeVersion})
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": g.ModelArtifact, "digest": g.ModelManifestSHA256}}})
		case "/api/show":
			json.NewEncoder(w).Encode(map[string]any{"template": g.Template, "system": "", "details": map[string]any{"quantization_level": g.Quantization}, "model_info": tokenizer})
		case "/api/generate":
			calls.Add(1)
			var body struct {
				Model, Prompt string
				Raw, Stream   bool
				Options       map[string]any
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if !body.Raw || body.Stream || !strings.Contains(body.Prompt, "café") || body.Options["seed"] != float64(17) || body.Options["num_predict"] != float64(768) {
				t.Errorf("request contract %+v", body)
			}
			json.NewEncoder(w).Encode(map[string]any{"model": g.ModelArtifact, "done": true, "done_reason": "stop", "response": `{"request_id":"sealed-request","candidates":[]}`})
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
	result, err := adapter.Extract(context.Background(), g, request(g))
	if err != nil || result.ReleaseEvidence != "completed" || calls.Load() != 1 {
		t.Fatalf("result %+v err %v calls %d", result, err, calls.Load())
	}
}

func TestLocalTransportFailsClosedWithoutFallback(t *testing.T) {
	for _, mode := range []string{"artifact_not_manifest", "redirect", "tokenizer_mismatch", "render_bound", "truncated", "timeout", "oversized_response"} {
		t.Run(mode, func(t *testing.T) {
			tokenizer := map[string]any{"tokenizer.ggml.model": "gpt2"}
			g := generation(tokenizer)
			if mode == "render_bound" {
				g.Template = strings.Repeat("{{.Prompt}}", 2000)
				g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
			}
			var generated atomic.Int32
			releaseServer := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/version":
					if mode == "redirect" {
						http.Redirect(w, r, "http://127.0.0.1:1/forbidden", http.StatusTemporaryRedirect)
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"version": g.RuntimeVersion})
				case "/api/tags":
					digest := g.ModelManifestSHA256
					if mode == "artifact_not_manifest" {
						digest = g.ModelSHA256
					}
					json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": g.ModelArtifact, "digest": digest}}})
				case "/api/show":
					metadata := tokenizer
					if mode == "tokenizer_mismatch" {
						metadata = map[string]any{"tokenizer.ggml.model": "other"}
					}
					json.NewEncoder(w).Encode(map[string]any{"template": g.Template, "system": "", "details": map[string]any{"quantization_level": g.Quantization}, "model_info": metadata})
				case "/api/generate":
					generated.Add(1)
					if mode == "timeout" {
						select {
						case <-r.Context().Done():
						case <-releaseServer:
						}
						return
					}
					if mode == "oversized_response" {
						w.Write([]byte(strings.Repeat("x", memory.CompilerMaxBytes+1)))
						return
					}
					json.NewEncoder(w).Encode(map[string]any{"model": g.ModelArtifact, "done": true, "done_reason": "length", "response": "partial"})
				}
			}))
			defer server.Close()
			defer close(releaseServer)
			adapter, err := localextractor.New(localextractor.Config{Endpoint: server.URL, Generation: g})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			started := time.Now()
			result, err := adapter.Extract(ctx, g, request(g))
			if err == nil {
				t.Fatal("unsafe configuration/output accepted")
			}
			wantCalls := int32(0)
			wantRelease := "not_dispatched"
			if mode == "truncated" {
				wantCalls = 1
				wantRelease = "completed"
			}
			if mode == "timeout" || mode == "oversized_response" {
				wantCalls = 1
				wantRelease = ""
			}
			if generated.Load() != wantCalls || result.ReleaseEvidence != wantRelease {
				t.Fatalf("calls=%d release=%s err=%v", generated.Load(), result.ReleaseEvidence, err)
			}
			if mode == "timeout" && time.Since(started) > time.Second {
				t.Fatal("cancellation too slow")
			}
		})
	}
	for _, endpoint := range []string{"https://127.0.0.1:11434", "http://localhost:11434", "http://192.0.2.1:11434", "http://127.0.0.1:11434/path", "http://user@127.0.0.1:11434", "http://127.0.0.1:11434?x=1"} {
		if _, err := localextractor.New(localextractor.Config{Endpoint: endpoint, Generation: generation(map[string]any{"tokenizer.ggml.model": "gpt2"})}); err == nil {
			t.Fatalf("unsafe endpoint %s", endpoint)
		}
	}
}
