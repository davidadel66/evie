package memory

import (
	"strings"
	"testing"
)

func calibratedSnapshotPayload() ContextSnapshotPayload {
	return ContextSnapshotPayload{
		SchemaVersion: 1, ComposerVersion: "context-composer-v2", EstimatorVersion: "calibrated-provider-json-bytes-v3",
		Iteration: 1, ConfiguredModel: "vendor/model", CanonicalModel: "vendor/model",
		ProfileSource: "explicit_override", HardWindowTokens: 262144, WorkingCeilingTokens: 262144,
		OutputReserveTokens: 16384, EstimationMarginTokens: 4096,
		BytesPerTokenMilli: 4237, CalibrationSamples: 9,
		// floor(241664 * 4237 / 1000) and ceil(1234 * 1000 / 4237).
		UsableInputBytes: 1023930, SerializedBytes: 1234, RoughTokenEstimate: 292,
		RequestSHA256:        strings.Repeat("a", 64),
		RetainedFirstEventID: "event-1", RetainedFirstSequence: 1,
		RetainedLastEventID: "event-2", RetainedLastSequence: 2,
		MessageCount: 3, ToolSchemaCount: 2, SystemMessageBytes: 100, HistoryMessageBytes: 300,
		ToolSchemaBytes: 200, RequestSettingsBytes: 80,
	}
}

// C3: a calibrated snapshot records the bytes-per-token ratio it used, and
// its byte budget and token estimate are exact functions of that ratio.
func TestContextSnapshotPayloadValidatesCalibratedRatio(t *testing.T) {
	valid := calibratedSnapshotPayload()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid calibrated payload rejected: %v", err)
	}
	if got := ContextBudgetBytes(241664, 4237); got != valid.UsableInputBytes {
		t.Fatalf("ContextBudgetBytes=%d, want %d", got, valid.UsableInputBytes)
	}
	if got := ContextBudgetBytes(241664, 0); got != 241664 {
		t.Fatalf("legacy ContextBudgetBytes=%d, want one byte per token", got)
	}
	for _, tt := range []struct {
		name   string
		mutate func(*ContextSnapshotPayload)
	}{
		{"usable not scaled by ratio", func(p *ContextSnapshotPayload) { p.UsableInputBytes = 241664 }},
		{"token estimate not from ratio", func(p *ContextSnapshotPayload) { p.RoughTokenEstimate = (p.SerializedBytes + 3) / 4 }},
		{"negative ratio", func(p *ContextSnapshotPayload) { p.BytesPerTokenMilli = -1 }},
		{"negative samples", func(p *ContextSnapshotPayload) { p.CalibrationSamples = -1 }},
		{"samples without ratio", func(p *ContextSnapshotPayload) {
			p.BytesPerTokenMilli, p.UsableInputBytes, p.RoughTokenEstimate = 0, 241664, (p.SerializedBytes+3)/4
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := calibratedSnapshotPayload()
			tt.mutate(&payload)
			if err := payload.Validate(); err == nil {
				t.Fatalf("invalid payload validated: %+v", payload)
			}
		})
	}
}

// C8: a provider context-length rejection that survives one compact-and-retry
// is recorded as context_overflow at the provider stage.
func TestTurnTerminalPayloadAllowsProviderStageContextOverflow(t *testing.T) {
	for _, stage := range []TurnStage{StageContextCompose, StageProvider} {
		payload := TurnTerminalPayload{TurnID: "turn", Classification: ClassificationContextOverflow, Stage: stage}
		if err := payload.Validate(EventTurnFailed); err != nil {
			t.Fatalf("stage=%q: %v", stage, err)
		}
	}
	for _, stage := range []TurnStage{StageTurnStart, StageAssistantCommit, StageToolPrepare, StageContextCompaction} {
		payload := TurnTerminalPayload{TurnID: "turn", Classification: ClassificationContextOverflow, Stage: stage}
		if err := payload.Validate(EventTurnFailed); err == nil {
			t.Fatalf("context overflow accepted at stage %q", stage)
		}
	}
}
