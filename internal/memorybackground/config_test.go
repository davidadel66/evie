package memorybackground

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// The pass is always on when a model is available, remote only with
// EVIE_REMOTE_MEMORY=on (owner messages leave the machine), local through
// the existing extractor configuration, and otherwise disabled with a
// reason the owner can act on. No setting can stop Evie from starting.
func TestConfigurationSelectsAModelOrDisablesWithAReason(t *testing.T) {
	remote := map[string]string{EnvRemote: "on", EnvAPIKey: "key"}
	cases := []struct {
		name     string
		values   map[string]string
		enabled  bool
		provider string
		model    string
		reason   string
	}{
		{"default remote", remote, true, ProviderOpenRouter, DefaultModel, ""},
		{"nothing configured", map[string]string{}, false, "", "", "EVIE_REMOTE_MEMORY=on"},
		{"remote memory off keeps messages local", map[string]string{EnvAPIKey: "key"}, false, "", "", "EVIE_REMOTE_MEMORY=on"},
		{"remote without key", map[string]string{EnvRemote: "on"}, false, "", "", EnvAPIKey},
		{"local fallback", map[string]string{EnvLocal: "/x/compiler.json"}, true, ProviderLocal, DefaultModel, ""},
		{"forced local", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvMode: "local", EnvLocal: "/x/c.json"}, true, ProviderLocal, DefaultModel, ""},
		{"local without config", map[string]string{EnvMode: "local"}, false, "", "", EnvLocal},
		{"forced remote without opt-in", map[string]string{EnvMode: "remote", EnvLocal: "/x/c.json"}, false, "", "", "EVIE_REMOTE_MEMORY=on"},
		{"off", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvMode: "off"}, false, "", "", "turned off"},
		{"unknown mode", map[string]string{EnvMode: "sometimes"}, false, "", "", "must be on, off"},
		{"pinned override", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "deepseek/deepseek-v4-flash-0731"}, true, ProviderOpenRouter, "deepseek/deepseek-v4-flash-0731", ""},
		{"latest alias", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "~deepseek/deepseek-flash-latest"}, false, "", "", "pinned"},
		{"latest name", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "deepseek/deepseek-v4-flash-latest"}, false, "", "", "pinned"},
		{"auto router", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "openrouter/auto"}, false, "", "", "pinned"},
		{"batch variant", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "deepseek/deepseek-v4.1-flash:batch"}, false, "", "", "Batch API"},
		{"not a model ID", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvModel: "flash"}, false, "", "", "model ID"},
		{"bad idle", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvIdle: "0"}, false, "", "", EnvIdle},
		{"bad turns", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvTurns: "many"}, false, "", "", EnvTurns},
		{"bad usd", map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvDailyUSD: "-1"}, false, "", "", EnvDailyUSD},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := FromEnv(env(tc.values))
			if config.Enabled != tc.enabled || config.Provider != tc.provider || tc.enabled && config.Model != tc.model ||
				!strings.Contains(config.Reason, tc.reason) {
				t.Fatalf("config = %+v", config)
			}
		})
	}
}

func TestConfigurationDefaultsAreConservativeAndOverridable(t *testing.T) {
	config := FromEnv(env(map[string]string{EnvRemote: "on", EnvAPIKey: "key"}))
	if config.Idle != 10*time.Minute || config.Turns != 20 || config.PassTokens != 32_000 || config.DailyTokens != 300_000 ||
		config.DailyMicroUSD != 250_000 || config.BatchMessages != 30 || config.BatchBytes != 24<<10 {
		t.Fatalf("defaults = %+v", config)
	}
	config = FromEnv(env(map[string]string{EnvRemote: "on", EnvAPIKey: "key", EnvIdle: "5", EnvTurns: "8", EnvDailyTokens: "50000", EnvDailyUSD: "0.05", EnvPassTokens: "48000"}))
	if !config.Enabled || config.Idle != 5*time.Minute || config.Turns != 8 || config.DailyTokens != 50_000 || config.DailyMicroUSD != 50_000 || config.PassTokens != 48_000 {
		t.Fatalf("overrides = %+v", config)
	}
}

func TestPricingRoundsCostUp(t *testing.T) {
	flash := Pricing{InputPerMillion: 0.3, OutputPerMillion: 1.2}
	if got := flash.MicroUSD(1_000, 100); got != 420 {
		t.Fatalf("cost = %d micro-USD", got)
	}
	if got := flash.MicroUSD(1, 0); got != 1 {
		t.Fatalf("a fraction of a micro-dollar rounds up, got %d", got)
	}
	if FallbackPricing.InputPerMillion < flash.InputPerMillion || FallbackPricing.OutputPerMillion < flash.OutputPerMillion {
		t.Fatal("fallback pricing must not be cheaper than the default model's catalog price")
	}
}
