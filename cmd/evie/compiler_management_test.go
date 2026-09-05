package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/localextractor"
	"github.com/davidadel66/evie/internal/memory"
)

func TestCompilerCLIUsesSQLiteAndLocalTransportWithoutConversationalClient(t *testing.T) {
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := eviedb.NewStore(db)
	ctx := context.Background()
	session, err := store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.AcquireTurnLease(ctx, session.ID, "cli-test", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	root, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventUserMessage, Role: memory.RoleUser, Content: "I prefer tea."})
	if err != nil {
		t.Fatal(err)
	}
	last, err := store.AppendEventWithLease(ctx, session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{Type: memory.EventAssistantMessage, Role: memory.RoleAssistant, ParentID: root.ID, Content: "Recorded."})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	handled, err := runCompilerManagement(ctx, []string{"memory-compile", "--session", string(session.ID)}, &out, store)
	if !handled || !errors.Is(err, eviedb.ErrCompilerNotConfigured) || out.Len() != 0 {
		t.Fatalf("unconfigured command %v %v %s", handled, err, out.String())
	}
	tokenizer := map[string]any{"tokenizer.ggml.model": "fixture"}
	tokenizerJSON, _ := json.Marshal(tokenizer)
	g := memory.CompilerGeneration{Version: "compiler-generation-v1", ModelArtifact: "fixture:model", ModelSHA256: strings.Repeat("1", 64), Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1", TokenizerSHA256: memory.CompilerHash(tokenizerJSON), Template: "{{.System}}\n{{.Prompt}}", Prompt: "Extract.", Schema: json.RawMessage(`{"type":"object"}`), TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8, Decoding: memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17}}
	g.ModelManifest = json.RawMessage(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	g.EvidencePolicy = memory.CompilerPolicyVersion
	g.SecretPolicy = memory.CompilerPolicyVersion
	g.ClosurePolicy = memory.CompilerPolicyVersion
	g.WindowPolicy = memory.CompilerPolicyVersion
	g.PredicatePolicy = memory.CompilerPolicyVersion
	g.EntityPolicy = memory.CompilerPolicyVersion
	g.ValidationPolicy = memory.CompilerPolicyVersion
	g.EquivalencePolicy = memory.CompilerPolicyVersion
	g.EffectPolicy = memory.CompilerPolicyVersion
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			json.NewEncoder(w).Encode(map[string]any{"version": g.RuntimeVersion})
		case "/api/tags":
			json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": g.ModelArtifact, "digest": g.ModelManifestSHA256}}})
		case "/api/show":
			json.NewEncoder(w).Encode(map[string]any{"template": g.Template, "details": map[string]any{"quantization_level": g.Quantization}, "model_info": tokenizer})
		case "/api/generate":
			calls.Add(1)
			var body struct{ Prompt string }
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			payload := strings.SplitN(strings.TrimPrefix(body.Prompt, g.Prompt+"\n"), "\nExtraction schema:", 2)[0]
			var request memory.CompilerRequest
			if err := json.Unmarshal([]byte(payload), &request); err != nil {
				t.Error(err)
			}
			response, _ := json.Marshal(memory.CompilerResponse{RequestID: request.ID, Candidates: []memory.ExtractorCandidate{}})
			json.NewEncoder(w).Encode(map[string]any{"model": g.ModelArtifact, "done": true, "done_reason": "stop", "response": string(response)})
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "compiler.json")
	config, _ := json.Marshal(localextractor.Config{Endpoint: server.URL, Generation: g})
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"memory-compile", "--session", string(session.ID), "--root", string(root.ID), "--cutoff", strconv.FormatInt(last.Sequence, 10), "--config", configPath}
	handled, err = runCompilerManagement(ctx, args, &out, store)
	if !handled || err != nil {
		t.Fatalf("compile command %v %v", handled, err)
	}
	var result memory.Compilation
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "completed_empty" || calls.Load() != 1 {
		t.Fatalf("result %+v calls%d", result, calls.Load())
	}
	out.Reset()
	handled, err = runCompilerManagement(ctx, []string{"memory-candidates", "inspect", "--session", string(session.ID), "--id", result.JobID}, &out, store)
	if !handled || err != nil || !strings.Contains(out.String(), "owner_statement") || !strings.Contains(out.String(), "I prefer tea.") || calls.Load() != 1 {
		t.Fatalf("inspection %v %v %s", handled, err, out.String())
	}
}
