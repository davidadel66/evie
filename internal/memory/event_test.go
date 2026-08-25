package memory

import (
	"encoding/json"
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
