package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
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

type compilerRuntimeFixture struct {
	db         *sql.DB
	store      *eviedb.Store
	session    memory.Session
	generation memory.CompilerGeneration
	configPath string
	server     *httptest.Server
	inferences atomic.Int32
}

func newCompilerRuntimeFixture(t *testing.T, generate http.HandlerFunc) *compilerRuntimeFixture {
	t.Helper()
	db, err := eviedb.OpenDBAt(filepath.Join(t.TempDir(), "evie.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	f := &compilerRuntimeFixture{db: db, store: eviedb.NewStore(db)}
	f.session, err = f.store.CreateGlobalSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tokenizer := map[string]any{"tokenizer.ggml.model": "fixture"}
	tokenizerJSON, err := json.Marshal(tokenizer)
	if err != nil {
		t.Fatal(err)
	}
	g := memory.CompilerGeneration{
		Version: "compiler-generation-v1", ModelArtifact: "fixture:model", ModelSHA256: strings.Repeat("1", 64),
		Quantization: "fixture", RuntimeVersion: "fixture", ProtocolVersion: "ollama-generate-v1",
		TokenizerSHA256: memory.CompilerHash(tokenizerJSON), Template: "{{.System}}\n{{.Prompt}}",
		Prompt: "Extract fixture candidates.", Schema: json.RawMessage(`{"type":"object"}`),
		TokenBoundProofSHA256: strings.Repeat("3", 64), TokensPerByte: 1, TemplateTokenOverhead: 8,
		Decoding:       memory.CompilerDecoding{ContextTokens: 131072, OutputTokens: 768, Seed: 17},
		EvidencePolicy: memory.CompilerPolicyVersion, SecretPolicy: memory.CompilerPolicyVersion,
		ClosurePolicy: memory.CompilerPolicyVersion, WindowPolicy: memory.CompilerPolicyVersion,
		PredicatePolicy: memory.CompilerPolicyVersion, EntityPolicy: memory.CompilerPolicyVersion,
		ValidationPolicy: memory.CompilerPolicyVersion, EquivalencePolicy: memory.CompilerPolicyVersion,
		EffectPolicy: memory.CompilerPolicyVersion,
	}
	g.ModelManifest = json.RawMessage(`{"layers":[{"mediaType":"application/vnd.ollama.image.model","digest":"sha256:` + g.ModelSHA256 + `"}]}`)
	g.ModelManifestSHA256 = memory.CompilerHash(g.ModelManifest)
	g.TemplateSHA256 = memory.CompilerHash([]byte(g.Template))
	f.generation = g
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"version": g.RuntimeVersion})
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]any{"name": g.ModelArtifact, "digest": g.ModelManifestSHA256}}})
		case "/api/show":
			_ = json.NewEncoder(w).Encode(map[string]any{"template": g.Template, "details": map[string]any{"quantization_level": g.Quantization}, "model_info": tokenizer})
		case "/api/generate":
			f.inferences.Add(1)
			if generate != nil {
				generate(w, r)
			} else {
				http.Error(w, "inference was not authorized by this fixture", http.StatusServiceUnavailable)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	f.configPath = filepath.Join(t.TempDir(), "compiler.json")
	config, err := json.Marshal(localextractor.Config{Endpoint: f.server.URL, Generation: g})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.configPath, config, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *compilerRuntimeFixture) command(t *testing.T, action string, args ...string) []byte {
	t.Helper()
	command := append([]string{"memory-compiler", action, "--session", string(f.session.ID)}, args...)
	var out bytes.Buffer
	handled, err := runCompilerActivationManagement(context.Background(), command, &out, f.store)
	if !handled || err != nil {
		t.Fatalf("%s handled=%v error=%v", action, handled, err)
	}
	for _, protected := range []string{"private source", f.generation.Prompt, f.generation.ModelArtifact, f.server.URL} {
		if strings.Contains(out.String(), protected) {
			t.Fatalf("%s output includes protected source or configuration", action)
		}
	}
	return out.Bytes()
}

func (f *compilerRuntimeFixture) activate(t *testing.T) memory.CompilerActivation {
	t.Helper()
	var result memory.CompilerActivation
	if err := json.Unmarshal(f.command(t, "activate", "--request", "activate", "--config", f.configPath), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (f *compilerRuntimeFixture) appendOwnerEvent(t *testing.T, content string) memory.Event {
	t.Helper()
	ctx := context.Background()
	lease, err := f.store.AcquireTurnLease(ctx, f.session.ID, "cli-fixture", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	event, appendErr := f.store.AppendEventWithLease(ctx, f.session.ID, lease.HolderID, lease.FencingToken, memory.EventInput{
		Type: memory.EventUserMessage, Role: memory.RoleUser, Content: content,
	})
	if err := errors.Join(appendErr, f.store.ReleaseTurnLease(ctx, f.session.ID, lease.HolderID, lease.FencingToken)); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestCompilerActivationCLIFrontierDisableResumeWithoutInference(t *testing.T) {
	f := newCompilerRuntimeFixture(t, nil)
	f.appendOwnerEvent(t, "private source before activation")
	active := f.activate(t)
	if active.AfterPosition != 1 || active.Revision != 1 || active.Selector.SessionID != f.session.ID || active.Selector.Destination != "global" {
		t.Fatalf("activation did not pin exact new selection: %+v", active)
	}
	f.appendOwnerEvent(t, "private source inside selection")
	if replay := f.activate(t); replay.ID != active.ID || replay.AfterPosition != active.AfterPosition {
		t.Fatalf("request replay moved frontier: %+v", replay)
	}
	var disabled memory.CompilerActivation
	if err := json.Unmarshal(f.command(t, "disable", "--request", "disable", "--id", active.ID, "--revision", "1"), &disabled); err != nil {
		t.Fatal(err)
	}
	if disabled.ThroughPosition == nil || *disabled.ThroughPosition != 2 || !disabled.WorkPaused || disabled.Revision != 2 {
		t.Fatalf("disable did not close captured selection: %+v", disabled)
	}
	f.appendOwnerEvent(t, "private source while disabled")
	var resumed memory.CompilerActivation
	if err := json.Unmarshal(f.command(t, "resume", "--request", "resume", "--id", active.ID, "--revision", strconv.FormatInt(disabled.Revision, 10), "--config", f.configPath), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.WorkPaused || resumed.ThroughPosition == nil || *resumed.ThroughPosition != 2 || resumed.ID != active.ID {
		t.Fatalf("resume reopened live selection: %+v", resumed)
	}
	var status memory.CompilerActivationStatus
	if err := json.Unmarshal(f.command(t, "status"), &status); err != nil {
		t.Fatal(err)
	}
	if status.SelectedEvents != 1 || status.OutsideSelectionEvents != 2 || len(status.Activations) != 1 {
		t.Fatalf("CLI lost selection boundaries: %+v", status)
	}
	var jobs int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM memory_compiler_jobs`).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 0 || f.inferences.Load() != 0 {
		t.Fatalf("short CLI drained compilation: jobs=%d inference=%d", jobs, f.inferences.Load())
	}
}

func TestCompilerActivationCLIRequiresExplicitConfiguration(t *testing.T) {
	f := newCompilerRuntimeFixture(t, nil)
	handled, err := runCompilerActivationManagement(context.Background(), []string{
		"memory-compiler", "activate", "--session", string(f.session.ID), "--request", "unconfigured",
	}, io.Discard, f.store)
	if !handled || !errors.Is(err, eviedb.ErrCompilerNotConfigured) {
		t.Fatalf("unconfigured activation handled=%v error=%v", handled, err)
	}
	var status memory.CompilerActivationStatus
	if err := json.Unmarshal(f.command(t, "status"), &status); err != nil {
		t.Fatal(err)
	}
	if len(status.Activations) != 0 || f.inferences.Load() != 0 {
		t.Fatalf("unconfigured activation created work: %+v", status)
	}
}
