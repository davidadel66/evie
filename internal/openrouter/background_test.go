package openrouter

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// The memory background pass (Stage 4) asks for JSON output and reads the
// model's catalog price for its daily USD cap.

func TestResponseFormatIsOmittedUnlessRequested(t *testing.T) {
	plain := ChatRequest{Model: "deepseek/deepseek-v4.1-flash", Messages: []Message{{Role: "user", Content: "hi"}}}
	encoded, err := RequestBytes(plain)
	if err != nil || strings.Contains(string(encoded), "response_format") {
		t.Fatalf("ordinary request = %s, %v", encoded, err)
	}
	plain.ResponseFormat = &ResponseFormat{Type: "json_object"}
	encoded, err = RequestBytes(plain)
	if err != nil || !strings.Contains(string(encoded), `"response_format":{"type":"json_object"}`) {
		t.Fatalf("structured request = %s, %v", encoded, err)
	}
}

func TestModelPricingReadsTheCatalogPriceOfExactlyThatModel(t *testing.T) {
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Errorf("unexpected metadata request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[
		{"id":"deepseek/deepseek-v4.1-flash:batch","pricing":{"prompt":"0.000000112","completion":"0.000000336"}},
		{"id":"deepseek/deepseek-v4.1-flash","pricing":{"prompt":"0.0000003","completion":"0.0000012"}},
		{"id":"vendor/broken","pricing":{"prompt":"-1","completion":"x"}}
		]}`))
	}))
	pricing, err := client.ModelPricing(context.Background(), "deepseek/deepseek-v4.1-flash")
	if err != nil || pricing.Prompt != 0.0000003 || pricing.Completion != 0.0000012 {
		t.Fatalf("pricing = %+v, %v", pricing, err)
	}
	if _, err := client.ModelPricing(context.Background(), "vendor/broken"); err == nil {
		t.Fatal("an invalid catalog price was accepted")
	}
	if _, err := client.ModelPricing(context.Background(), "vendor/missing"); err == nil {
		t.Fatal("a model missing from the catalog was priced")
	}
}
