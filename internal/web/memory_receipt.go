package web

import (
	"context"
	"encoding/json"

	"github.com/davidadel66/evie/internal/memory"
)

// MemoryEvidenceHistory reads already committed events while the selected
// conversation may have a provider request in progress.
type MemoryEvidenceHistory interface {
	LoadEvents(context.Context, memory.SessionID) ([]memory.Event, error)
}

type memoryRequestRecord struct {
	event      memory.Event
	snapshot   memory.ContextSnapshotPayload
	rootID     memory.EventID
	responseID memory.EventID
	status     string
	final      bool
}

// Request snapshots and responses share the same causal parent. Pair only the
// nearest earlier snapshot for that parent, never a neighboring turn or a
// similarity search. A saved request alone does not prove provider delivery.
func memoryRequestRecords(events []memory.Event) ([]memoryRequestRecord, error) {
	var records []memoryRequestRecord
	roots := make(map[memory.EventID]memory.EventID)
	latest := make(map[memory.EventID]int)
	for _, event := range events {
		root := roots[event.ParentID]
		if event.Type == memory.EventUserMessage {
			root = event.ID
		}
		roots[event.ID] = root
		switch event.Type {
		case memory.EventContextSnapshot:
			var snapshot memory.ContextSnapshotPayload
			if len(event.Payload) == 0 {
				continue
			}
			if err := json.Unmarshal(event.Payload, &snapshot); err != nil {
				return nil, err
			}
			records = append(records, memoryRequestRecord{event: event, snapshot: snapshot, rootID: root, status: "prepared"})
			latest[event.ParentID] = len(records) - 1
		case memory.EventAssistantMessage:
			index, ok := latest[event.ParentID]
			if !ok || root == "" || records[index].rootID != root || records[index].responseID != "" || records[index].status != "prepared" {
				continue
			}
			var assistant memory.AssistantMessagePayload
			if len(event.Payload) > 0 {
				if err := json.Unmarshal(event.Payload, &assistant); err != nil {
					return nil, err
				}
			}
			records[index].responseID = event.ID
			records[index].status = "completed"
			records[index].final = len(assistant.ToolCalls) == 0
		case memory.EventTurnFailed, memory.EventTurnInterrupted:
			var terminal memory.TurnTerminalPayload
			if len(event.Payload) == 0 {
				continue
			}
			if err := json.Unmarshal(event.Payload, &terminal); err != nil {
				return nil, err
			}
			if terminal.TurnID == "" || terminal.TurnID != root {
				continue
			}
			for i := range records {
				if records[i].rootID == root && records[i].status == "prepared" {
					records[i].status = "interrupted"
				}
			}
		}
	}
	return records, nil
}

type memoryEvidenceRequest struct {
	SnapshotID      memory.EventID               `json:"snapshotId"`
	ResponseID      memory.EventID               `json:"responseId,omitempty"`
	RequestStatus   string                       `json:"requestStatus"`
	Iteration       int                          `json:"iteration"`
	RequestSHA256   string                       `json:"requestSHA256"`
	SerializedBytes int64                        `json:"serializedBytes"`
	Version         string                       `json:"version"`
	Status          string                       `json:"status"`
	Evidence        []memory.RetrievalInspection `json:"evidence"`
}

type memoryEvidenceResponse struct {
	SessionID  memory.SessionID             `json:"sessionId"`
	SnapshotID memory.EventID               `json:"snapshotId"`
	AnswerID   memory.EventID               `json:"answerId,omitempty"`
	Version    string                       `json:"version"`
	Status     string                       `json:"status"`
	Evidence   []memory.RetrievalInspection `json:"evidence"`
	Requests   []memoryEvidenceRequest      `json:"requests"`
}

func inspectMemoryRequests(ctx context.Context, inspector MemoryEvidenceInspector, scope memory.ScopeContext, records []memoryRequestRecord, snapshotID, answerID memory.EventID) (*memoryEvidenceResponse, error) {
	selected := -1
	answerIndex := -1
	for i, record := range records {
		if record.event.SessionID != scope.SessionID {
			continue
		}
		if snapshotID != "" && record.event.ID == snapshotID {
			selected = i
		}
		if answerID != "" && record.responseID == answerID && record.final {
			answerIndex = i
		}
	}
	if answerID != "" {
		if answerIndex < 0 {
			return nil, nil
		}
		if snapshotID == "" {
			selected = answerIndex
		}
		if selected < 0 || records[selected].rootID != records[answerIndex].rootID || selected > answerIndex {
			return nil, nil
		}
	}
	if selected < 0 {
		return nil, nil
	}
	result := &memoryEvidenceResponse{SessionID: scope.SessionID, SnapshotID: records[selected].event.ID, AnswerID: answerID, Evidence: []memory.RetrievalInspection{}, Requests: []memoryEvidenceRequest{}}
	for i, record := range records {
		if record.event.SessionID != scope.SessionID {
			continue
		}
		if i != selected && (record.rootID == "" || record.rootID != records[selected].rootID) {
			continue
		}
		if answerIndex >= 0 && i > answerIndex {
			continue
		}
		request := memoryEvidenceRequest{SnapshotID: record.event.ID, ResponseID: record.responseID, RequestStatus: record.status, Iteration: record.snapshot.Iteration, RequestSHA256: record.snapshot.RequestSHA256, SerializedBytes: record.snapshot.SerializedBytes, Evidence: []memory.RetrievalInspection{}}
		if record.snapshot.Memory != nil {
			request.Version = record.snapshot.Memory.Version
			request.Status = record.snapshot.Memory.Status
			inspected, err := inspector.InspectMemoryEvidence(ctx, scope, record.snapshot.Memory.Evidence)
			if err != nil {
				return nil, err
			}
			if inspected != nil {
				request.Evidence = inspected
			}
		}
		if record.final {
			result.AnswerID = record.responseID
		}
		if i == selected {
			result.Version = request.Version
			result.Status = request.Status
			result.Evidence = request.Evidence
		}
		result.Requests = append(result.Requests, request)
	}
	return result, nil
}
