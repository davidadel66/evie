package openrouter

import "testing"

func TestWorkerLimitsUseIndependentBudgetWithinModelWindow(t *testing.T) {
	for _, test := range []struct {
		name, model               string
		hard, request, wantUsable int64
	}{
		{"one MiB", "test/model", 2_000_000, 1_048_576, 1_048_576},
		{"smaller model", "test/model", 131_072, 1_048_576, 125_952},
		{"operator override", "test/model", 2_000_000, 32_768, 32_768},
		{"Responses prompt cap", AstraModel, 131_072, 1_048_576, 110_592},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent, err := NewExplicitContextProfile(test.model, test.hard, 65_536, 16_384)
			if err != nil {
				t.Fatal(err)
			}
			worker, err := parent.WithWorkerLimits(test.request, 1024)
			if err != nil {
				t.Fatal(err)
			}
			d := worker.Diagnostics()
			if got := d.WorkingTokens - d.OutputReserveTokens - d.EstimationMarginTokens; got != test.wantUsable {
				t.Fatalf("usable=%d want=%d", got, test.wantUsable)
			}
			if d.ConfiguredModel != test.model || d.OutputReserveTokens != 1024 || d.WorkingTokens > d.HardWindowTokens || parent.Diagnostics().WorkingTokens != 65_536 {
				t.Fatalf("worker=%+v parent=%+v", d, parent.Diagnostics())
			}
		})
	}
}
