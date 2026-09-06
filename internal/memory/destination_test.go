package memory

import "testing"

func TestMemoryDestinationCannotInventContextOrOverrideExplicitChoice(t *testing.T) {
	for _, tc := range []struct {
		scope       ScopeContext
		destination MemoryDestination
		legacy      bool
	}{
		{ScopeContext{SessionID: "s"}, MemoryWorkspace, false},
		{ScopeContext{}, MemorySession, false},
		{ScopeContext{SessionID: "s", WorkspaceID: "w"}, "workspace:other", false},
		{ScopeContext{SessionID: "s", WorkspaceID: "w"}, MemoryEverywhere, true},
	} {
		if _, err := ResolveMemoryDestination(tc.scope, tc.destination, tc.legacy); err == nil {
			t.Fatalf("invalid destination accepted %+v", tc)
		}
	}
}

func TestScopePolicyPromptIsFrozen(t *testing.T) {
	if CompilerHash([]byte(MemoryScopeRecommendationInstructions)) != "8b5cda619a3b9cd56dfb6e3b17adb6d692cbec841f7ecaaca785a6d1b4ba04f8" {
		t.Fatal("scope prompt changed: version the policy before editing frozen compiler instructions")
	}
}
