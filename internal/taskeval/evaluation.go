// Package taskeval derives a stable, content-free evidence baseline for
// durable Task evaluation. It does not score model quality or persist reports.
package taskeval

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/task"
)

const ReportSchemaVersion = 1

type Input struct {
	Tasks          []task.Task
	TaskEvents     []task.Event
	EpisodicEvents []memory.Event
	SessionScopes  map[memory.SessionID]task.Scope
}

type Report struct {
	SchemaVersion         int            `json:"schema_version"`
	TaskCount             int            `json:"task_count"`
	CompletionCount       int            `json:"completion_count"`
	AbandonmentCount      int            `json:"abandonment_count"`
	DuplicateAttemptCount int            `json:"duplicate_attempt_count"`
	ConflictCount         int            `json:"conflict_count"`
	RetryCount            int            `json:"retry_count"`
	DecompositionCount    int            `json:"decomposition_count"`
	RecoveryCount         int            `json:"recovery_count"`
	EvidenceLinkCount     int            `json:"evidence_link_count"`
	EvidenceLinks         []EvidenceLink `json:"evidence_links"`
}

// EvidenceLink contains correlation identities only. It deliberately excludes
// prompts, reasoning, tool arguments and results, Task content, and credentials.
type EvidenceLink struct {
	TaskEventID     string           `json:"task_event_id"`
	SessionID       string           `json:"session_id"`
	ExecutionID     string           `json:"execution_id"`
	ToolName        string           `json:"tool_name"`
	IntentEventID   memory.EventID   `json:"intent_event_id"`
	ApprovalEventID memory.EventID   `json:"approval_event_id,omitempty"`
	OutcomeEventID  memory.EventID   `json:"outcome_event_id"`
	OutcomeType     memory.EventType `json:"outcome_type"`
}

type executionEvidence struct {
	toolName        string
	intentEventID   memory.EventID
	approvalEventID memory.EventID
	outcomeEventID  memory.EventID
	outcomeType     memory.EventType
}

type executionKey struct {
	sessionID   string
	executionID string
}

// Derive mechanically counts persisted state and joins Task events to
// episodic tool evidence by their trusted session and execution identities.
func Derive(input Input) (Report, error) {
	report := Report{SchemaVersion: ReportSchemaVersion, TaskCount: len(input.Tasks), EvidenceLinks: []EvidenceLink{}}
	for _, value := range input.Tasks {
		switch value.Status {
		case task.StatusCompleted:
			report.CompletionCount++
		case task.StatusCancelled:
			report.AbandonmentCount++
		}
	}
	for _, event := range input.TaskEvents {
		if event.Operation == task.OperationDecompose && event.Outcome == task.MutationAccepted {
			report.DecompositionCount++
		}
		if event.Outcome == task.MutationRejected && event.DiagnosticCode == task.DiagnosticRevisionConflict {
			report.ConflictCount++
		}
		if event.Operation == task.OperationRelease && event.Outcome == task.MutationAccepted &&
			(event.ClaimReason == "recovery" || event.ClaimReason == "execution_ended") {
			report.RecoveryCount++
		}
	}

	evidence, attemptGroups, err := indexEpisodicEvidence(input.EpisodicEvents, input.SessionScopes)
	if err != nil {
		return Report{}, err
	}
	for _, requests := range attemptGroups {
		total := 0
		for _, count := range requests {
			total += count
			report.RetryCount += count - 1
		}
		report.DuplicateAttemptCount += total - 1
		report.ConflictCount += len(requests) - 1
	}
	for _, event := range input.TaskEvents {
		key := executionKey{sessionID: event.SessionID, executionID: event.RunID}
		linked := evidence[key]
		if linked.intentEventID == "" || linked.outcomeEventID == "" {
			continue
		}
		report.EvidenceLinks = append(report.EvidenceLinks, EvidenceLink{
			TaskEventID: event.ID, SessionID: event.SessionID, ExecutionID: event.RunID,
			ToolName: linked.toolName, IntentEventID: linked.intentEventID, ApprovalEventID: linked.approvalEventID,
			OutcomeEventID: linked.outcomeEventID, OutcomeType: linked.outcomeType,
		})
	}
	sort.Slice(report.EvidenceLinks, func(i, j int) bool {
		left, right := report.EvidenceLinks[i], report.EvidenceLinks[j]
		if left.SessionID != right.SessionID {
			return left.SessionID < right.SessionID
		}
		if left.ExecutionID != right.ExecutionID {
			return left.ExecutionID < right.ExecutionID
		}
		return left.TaskEventID < right.TaskEventID
	})
	report.EvidenceLinkCount = len(report.EvidenceLinks)
	return report, nil
}

func indexEpisodicEvidence(
	events []memory.Event,
	sessionScopes map[memory.SessionID]task.Scope,
) (map[executionKey]executionEvidence, map[string]map[string]int, error) {
	evidence := make(map[executionKey]executionEvidence)
	attemptGroups := make(map[string]map[string]int)
	for _, event := range events {
		key := executionKey{sessionID: string(event.SessionID), executionID: string(event.ExecutionID)}
		linked := evidence[key]
		switch event.Type {
		case memory.EventToolIntent:
			var payload memory.ToolIntentPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return nil, nil, fmt.Errorf("decode tool intent %q: %w", event.ID, err)
			}
			linked.toolName = payload.Call.Name
			linked.intentEventID = event.ID
			identity, request, found, err := canonicalAttempt(event.SessionID, payload.Call, sessionScopes)
			if err != nil {
				return nil, nil, fmt.Errorf("derive tool intent %q: %w", event.ID, err)
			}
			if found {
				if attemptGroups[identity] == nil {
					attemptGroups[identity] = make(map[string]int)
				}
				attemptGroups[identity][request]++
			}
		case memory.EventApproval:
			linked.approvalEventID = event.ID
		case memory.EventToolSucceeded, memory.EventToolFailed, memory.EventToolCancelled:
			linked.outcomeEventID = event.ID
			linked.outcomeType = event.Type
		}
		evidence[key] = linked
	}
	return evidence, attemptGroups, nil
}

func canonicalAttempt(
	sessionID memory.SessionID,
	call memory.ToolCall,
	sessionScopes map[memory.SessionID]task.Scope,
) (string, string, bool, error) {
	attempt, found, err := plugins.CanonicalTodoMutationAttempt(call.Name, call.Arguments, sessionScopes[sessionID])
	if err != nil {
		return "", "", false, fmt.Errorf("canonicalize Todo mutation: %w", err)
	}
	if !found {
		return "", "", false, nil
	}
	return string(sessionID) + ":" + attempt.IdempotencySHA256, attempt.RequestSHA256, true, nil
}
