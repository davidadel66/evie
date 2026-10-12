package openrouter

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestChatModelCatalogFiltersDeduplicatesAndGroupsProviders(t *testing.T) {
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" || r.Header.Get("Authorization") != "Bearer profile-key" {
			t.Errorf("unexpected metadata request: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[
		{"id":"openai/gpt-test","name":"GPT Test","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":32000}},
		{"id":"anthropic/claude-test","name":"Claude Test","architecture":{"input_modalities":["text","image"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":32000}},
		{"id":"openai/gpt-test","name":"Duplicate","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":32000}},
		{"id":"image/model","architecture":{"input_modalities":["text"],"output_modalities":["image"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":32000}},
		{"id":"text/model","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens"],"top_provider":{"max_completion_tokens":32000}},
		{"id":"invalid","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":32000}}
		]}`))
	}))
	models, err := client.ListChatModels(context.Background())
	if err != nil || len(models) != 2 || models[0].Provider != "anthropic" || models[1].ID != "openai/gpt-test" || models[1].Name != "GPT Test" {
		t.Fatalf("models=%+v err=%v", models, err)
	}
}

func TestChatModelCatalogListsOnlySelectableModels(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
		{"id":"vendor/kept","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":16384}},
		{"id":"vendor/kept:free","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":16384}},
		{"id":"vendor/kept:batch","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":16384}},
		{"id":"~vendor/latest","architecture":{"tokenizer":"Router","input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":128000}},
		{"id":"openrouter/auto","architecture":{"tokenizer":"Router","input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"]},
		{"id":"vendor/completion-only","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_completion_tokens","tools"],"top_provider":{"max_completion_tokens":128000}},
		{"id":"openai/gpt-6-astra","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_completion_tokens","reasoning","tool_choice","tools"],"top_provider":{"max_completion_tokens":128000}},
		{"id":"vendor/short-output","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"],"top_provider":{"max_completion_tokens":16383}},
		{"id":"vendor/unknown-output","architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["max_tokens","tools"]}
		]}`))
	}))
	ids := func() []string {
		t.Helper()
		models, err := client.ListChatModels(context.Background())
		if err != nil {
			t.Fatalf("list chat models: %v", err)
		}
		listed := make([]string, 0, len(models))
		for _, model := range models {
			listed = append(listed, model.ID)
		}
		return listed
	}

	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")
	if got := strings.Join(ids(), ","); got != "openai/gpt-6-astra,vendor/kept" {
		t.Fatalf("default reserve listed %s", got)
	}
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "20000")
	if got := strings.Join(ids(), ","); got != "openai/gpt-6-astra" {
		t.Fatalf("explicit reserve listed %s", got)
	}
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "not-a-number")
	if _, err := client.ListChatModels(context.Background()); err == nil {
		t.Fatal("invalid output reserve accepted")
	}
}

func TestChatModelCatalogRejectsProviderErrorsAndOversizedBodies(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider", true: "oversized"}[oversized], func(t *testing.T) {
			client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if oversized {
					_, _ = w.Write([]byte(strings.Repeat(" ", (8<<20)+1)))
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("secret-provider-body"))
			}))
			_, err := client.ListChatModels(context.Background())
			if err == nil || strings.Contains(err.Error(), "secret-provider-body") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestSelectedChatModelFitsDefaultWorkingCeilingButHonorsExplicitLimits(t *testing.T) {
	t.Setenv("EVIE_CONTEXT_WINDOW_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "")
	t.Setenv("EVIE_CONTEXT_OUTPUT_RESERVE_TOKENS", "")
	client, _ := contextProfileClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/endpoints") {
			_, _ = w.Write([]byte(`{"data":{"id":"vendor/small","endpoints":[{"context_length":128000,"max_completion_tokens":32000,"status":0,"supported_parameters":["max_tokens","tools"]}]}}`))
		} else {
			_, _ = w.Write([]byte(`{"data":{"id":"vendor/small","canonical_slug":"vendor/small","context_length":128000}}`))
		}
	}))
	profile, err := client.ResolveChatContextProfile(context.Background(), "vendor/small")
	if err != nil || profile.Diagnostics().WorkingTokens != 128000 {
		t.Fatalf("profile=%+v err=%v", profile.Diagnostics(), err)
	}
	if _, err := client.ResolveContextProfile(context.Background(), "vendor/small"); err == nil {
		t.Fatal("startup profile unexpectedly relaxed")
	}
	t.Setenv("EVIE_CONTEXT_WORKING_TOKENS", "200000")
	if _, err := client.ResolveChatContextProfile(context.Background(), "vendor/small"); err == nil {
		t.Fatal("explicit working ceiling ignored")
	}
}
