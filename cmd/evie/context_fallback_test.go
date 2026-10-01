package main

import (
	"strings"
	"testing"

	"github.com/davidadel66/evie/internal/openrouter"
)

func TestContextFallbackWarningMakesDegradedStartupVisible(t *testing.T) {
	warning := contextFallbackWarning(openrouter.ContextProfileDiagnostics{
		ConfiguredModel: "deepseek/deepseek-v4.1-flash", HardWindowTokens: 262144,
		Source: openrouter.ContextProfileBuiltinFallback,
	})
	if !strings.Contains(warning, "deepseek/deepseek-v4.1-flash") || !strings.Contains(warning, "262144") {
		t.Fatalf("warning=%q", warning)
	}
	for _, source := range []openrouter.ContextProfileSource{openrouter.ContextProfileRemoteMetadata, openrouter.ContextProfileExplicitOverride} {
		if warning := contextFallbackWarning(openrouter.ContextProfileDiagnostics{Source: source}); warning != "" {
			t.Fatalf("source %q warned %q", source, warning)
		}
	}
}
