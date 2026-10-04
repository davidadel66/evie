package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// An AI step and an interruption, as the runtime runs them (Stage 14
// sub-stage 7): the exact reviewed prompt and its hash, the pinned Model
// Policy, the canonical output schema, and deterministic validation of a
// proposal or an answer against it.
func TestWorkflowAIStepAndInterruptionAccessors(t *testing.T) {
	dir := filepath.Join("testdata", "receipt-split")
	v := mustValidate(t, dir, testEnvironment())
	prompt, err := os.ReadFile(filepath.Join(dir, "prompts", "read.md"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(prompt)
	ai, ok := v.AIStep("read")
	if !ok {
		t.Fatal("read is an AI step")
	}
	temperature := 0.0
	want := PolicyReport{Name: "reader", Version: 1, Provider: "openrouter", Model: "deepseek/deepseek-v4.1-flash", Temperature: &temperature, MaxOutputTokens: 500}
	if !reflect.DeepEqual(ai.Policy, want) || string(ai.Prompt) != string(prompt) || ai.PromptSHA256 != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("AI step = %+v", ai)
	}
	if string(ai.Schema) != `{"additionalProperties":false,"properties":{"confidence":{"enum":["high","low"],"type":"string"},"total_cents":{"maximum":10000000,"minimum":0,"type":"integer"}},"required":["total_cents","confidence"],"type":"object"}` ||
		!strings.HasPrefix(ai.SchemaSHA256, "sha256:") {
		t.Fatalf("schema = %s (%s)", ai.Schema, ai.SchemaSHA256)
	}
	if _, ok := v.AIStep("confirm"); ok {
		t.Fatal("confirm is not an AI step")
	}
	q, ok := v.InterruptStep("confirm")
	if !ok || q.Message != "Evie read this receipt's total with low confidence. Reply with the total in cents." ||
		!strings.Contains(string(q.Schema), `"total_cents"`) || !strings.HasPrefix(q.SchemaSHA256, "sha256:") {
		t.Fatalf("interruption = %+v", q)
	}
	if policies := v.ModelPolicies(); !reflect.DeepEqual(policies, []PolicyReport{want}) {
		t.Fatalf("pinned policies = %+v", policies)
	}

	for _, tc := range []struct {
		step  string
		value any
		bad   string
	}{
		{"read", map[string]any{"total_cents": int64(1200), "confidence": "high"}, ""},
		{"read", map[string]any{"total_cents": int64(1200)}, "missing field confidence"},
		{"read", map[string]any{"total_cents": int64(-5), "confidence": "high"}, "below the minimum 0"},
		{"read", map[string]any{"total_cents": int64(20_000_000), "confidence": "high"}, "above the maximum"},
		{"read", map[string]any{"total_cents": 12.5, "confidence": "high"}, "expected integer"},
		{"read", map[string]any{"total_cents": int64(1), "confidence": "maybe"}, "is not one of"},
		{"read", map[string]any{"total_cents": int64(1), "confidence": "low", "why": "x"}, "unexpected field why"},
		{"confirm", map[string]any{"total_cents": int64(1300), "note": "checked"}, ""},
		{"confirm", map[string]any{"note": "checked"}, "missing field total_cents"},
		{"tip", map[string]any{}, "not an AI or interruption step"},
	} {
		err := v.CheckStepOutput(tc.step, tc.value)
		if tc.bad == "" && err != nil || tc.bad != "" && (err == nil || !strings.Contains(err.Error(), tc.bad)) {
			t.Fatalf("%s %v: %v, want %q", tc.step, tc.value, err, tc.bad)
		}
	}
}

// A Model Policy names one exact model: OpenRouter's routers pick a model
// per request, and the :batch variant needs the asynchronous Batch API.
func TestWorkflowModelPolicyRefusesRouters(t *testing.T) {
	for _, model := range []string{"openrouter/auto", "deepseek/deepseek-v4.1-flash:batch"} {
		t.Run(model, func(t *testing.T) {
			dir := copyDefinition(t, "receipt-split")
			edit(t, dir, "workflow.yaml", "model: deepseek/deepseek-v4.1-flash", "model: "+model)
			_, err := validateDir(dir, testEnvironment())
			wantProblem(t, err, "model_policies.reader.model", "exact model")
		})
	}
}
