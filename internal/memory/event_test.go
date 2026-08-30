package memory

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestAssistantMessagePayloadUsageJSONPreservesPartialZeroAndAbsence(t *testing.T) {
	zero, total := int64(0), int64(12)
	payload := AssistantMessagePayload{Usage: &TokenUsage{
		InputTokens: &zero,
		TotalTokens: &total,
	}}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"usage":{"input_tokens":0,"total_tokens":12}}`
	if string(encoded) != want {
		t.Fatalf("payload JSON=%s, want %s", encoded, want)
	}

	var decoded AssistantMessagePayload
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Usage == nil || decoded.Usage.InputTokens == nil || *decoded.Usage.InputTokens != 0 ||
		decoded.Usage.TotalTokens == nil || *decoded.Usage.TotalTokens != 12 ||
		decoded.Usage.OutputTokens != nil {
		t.Fatalf("round-tripped payload=%+v usage=%+v", decoded, decoded.Usage)
	}

	absent, err := json.Marshal(AssistantMessagePayload{})
	if err != nil {
		t.Fatal(err)
	}
	if string(absent) != `{}` {
		t.Fatalf("absent usage JSON=%s, want {}", absent)
	}
}

func TestContextSnapshotPayloadValidation(t *testing.T) {
	valid := ContextSnapshotPayload{
		SchemaVersion: 1, ComposerVersion: "context-composer-v1", EstimatorVersion: "canonical-json-bytes-v1",
		Iteration: 1, ConfiguredModel: "vendor/model", CanonicalModel: "vendor/model",
		ProfileSource: "explicit_override", HardWindowTokens: 262144, WorkingCeilingTokens: 262144,
		OutputReserveTokens: 16384, EstimationMarginTokens: 4096, UsableInputBytes: 241664,
		SerializedBytes: 1234, RoughTokenEstimate: 309, RequestSHA256: strings.Repeat("a", 64),
		RetainedFirstEventID: "event-1", RetainedFirstSequence: 1,
		RetainedLastEventID: "event-2", RetainedLastSequence: 2,
		MessageCount: 3, ToolSchemaCount: 2, SystemMessageBytes: 100, HistoryMessageBytes: 300,
		ToolSchemaBytes: 200, RequestSettingsBytes: 80,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ContextSnapshotPayload)
	}{
		{"unsupported version", func(p *ContextSnapshotPayload) { p.SchemaVersion = 2 }},
		{"bad arithmetic", func(p *ContextSnapshotPayload) { p.UsableInputBytes++ }},
		{"oversized request", func(p *ContextSnapshotPayload) { p.SerializedBytes = p.UsableInputBytes + 1 }},
		{"bad hash", func(p *ContextSnapshotPayload) { p.RequestSHA256 = "secret" }},
		{"backward frontier", func(p *ContextSnapshotPayload) { p.RetainedLastSequence = 0 }},
		{"unknown failure", func(p *ContextSnapshotPayload) { p.CompactionFailureCategory = "raw provider error" }},
		{"unexpected advertised metadata", func(p *ContextSnapshotPayload) { p.AdvertisedModel = "secret" }},
		{"compaction without summary", func(p *ContextSnapshotPayload) { p.ActiveCompactionEventID = "compaction-1" }},
		{"summary without compaction", func(p *ContextSnapshotPayload) { p.SummaryMessageBytes = 10 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := valid
			tt.mutate(&payload)
			if err := payload.Validate(); err == nil {
				t.Fatalf("invalid payload validated: %+v", payload)
			}
		})
	}
}

func TestContextCompactedPayloadValidation(t *testing.T) {
	var summaryBuilder strings.Builder
	for _, heading := range ContextCompactionSectionHeadings() {
		fmt.Fprintf(&summaryBuilder, "## %s\nkept\n\n", heading)
	}
	summary := summaryBuilder.String()
	digest := sha256.Sum256([]byte(summary))
	valid := ContextCompactedPayload{
		SchemaVersion: ContextCompactedSchemaVersion,
		Generation:    1, Trigger: ContextCompactionManual,
		CoveredFirstEventID: "event-1", CoveredFirstSequence: 1,
		CoveredLastEventID: "event-2", CoveredLastSequence: 2,
		FirstRetainedEventID: "event-3", CanonicalModel: "vendor/model",
		PromptVersion: "compaction-v1", SummaryBytes: int64(len(summary)),
		SummarySHA256: fmt.Sprintf("%x", digest),
	}
	if err := valid.Validate(summary); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*ContextCompactedPayload)
	}{
		{"unsupported version", func(p *ContextCompactedPayload) { p.SchemaVersion++ }},
		{"missing generation", func(p *ContextCompactedPayload) { p.Generation = 0 }},
		{"manual generation has prior", func(p *ContextCompactedPayload) { p.PriorCompactionEventID = "prior" }},
		{"unknown trigger", func(p *ContextCompactedPayload) { p.Trigger = "automatic-ish" }},
		{"backward coverage", func(p *ContextCompactedPayload) { p.CoveredLastSequence = 0 }},
		{"missing retained identity", func(p *ContextCompactedPayload) { p.FirstRetainedEventID = "" }},
		{"wrong byte count", func(p *ContextCompactedPayload) { p.SummaryBytes++ }},
		{"wrong digest", func(p *ContextCompactedPayload) { p.SummarySHA256 = strings.Repeat("0", 64) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.mutate(&payload)
			if err := payload.Validate(summary); err == nil {
				t.Fatalf("invalid payload validated: %+v", payload)
			}
		})
	}
}

func TestTurnTerminalPayloadClosedStageAndClassificationMatrix(t *testing.T) {
	stages := []TurnStage{
		StageTurnStart, StageProvider, StageAssistantCommit, StageToolPrepare,
		StageToolApproval, StageToolExecute, StageToolCommit,
	}
	for _, stage := range stages {
		for _, tt := range []struct {
			eventType      EventType
			classification TurnClassification
		}{
			{EventTurnFailed, ClassificationProviderError},
			{EventTurnFailed, ClassificationProviderResponseInvalid},
			{EventTurnInterrupted, ClassificationCallerCancelled},
			{EventTurnInterrupted, ClassificationCallerDeadlineExceeded},
		} {
			payload := TurnTerminalPayload{TurnID: "turn", Classification: tt.classification, Stage: stage}
			if err := payload.Validate(tt.eventType); err != nil {
				t.Errorf("stage=%q classification=%q: %v", stage, tt.classification, err)
			}
		}
	}

	invalid := []TurnTerminalPayload{
		{TurnID: "turn", Classification: ClassificationProviderError, Stage: "unknown"},
		{TurnID: "", Classification: ClassificationProviderError, Stage: StageProvider},
		{TurnID: "turn", Classification: "unknown", Stage: StageProvider},
	}
	for _, payload := range invalid {
		if err := payload.Validate(EventTurnFailed); err == nil {
			t.Errorf("invalid payload validated: %+v", payload)
		}
	}
}

func TestTurnTerminalPayloadHTTPStatusOnlyAcceptsNon2xxProviderErrors(t *testing.T) {
	for _, status := range []int{100, 199, 300, 404, 503, 999} {
		payload := TurnTerminalPayload{TurnID: "turn", Classification: ClassificationProviderError, Stage: StageProvider, HTTPStatus: &status}
		if err := payload.Validate(EventTurnFailed); err != nil {
			t.Errorf("status %d rejected: %v", status, err)
		}
	}
	for _, status := range []int{0, 99, 200, 204, 299, 1000} {
		payload := TurnTerminalPayload{TurnID: "turn", Classification: ClassificationProviderError, Stage: StageProvider, HTTPStatus: &status}
		if err := payload.Validate(EventTurnFailed); err == nil {
			t.Errorf("status %d accepted", status)
		}
	}
}
