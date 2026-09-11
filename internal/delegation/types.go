// Package delegation defines durable Kernel execution records, separate from Tasks.
package delegation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/memory"
)

const CapabilityID = "subagents.research"
const ToolName = "delegate_research"

var ErrConflict = errors.New("subagent idempotency key conflicts with an existing assignment")
var ErrCapacity = errors.New("subagent concurrency capacity is occupied")
var ErrAuthority = errors.New("subagent authority is no longer available")
var ErrPolicy = errors.New("subagent execution policy limit")

type Assignment struct {
	Key       string `json:"idempotency_key"`
	Objective string `json:"objective"`
	Context   string `json:"context,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
}

type Policy struct {
	PerParent       int           `json:"per_parent"`
	Runtime         int           `json:"runtime"`
	MaxBatch        int           `json:"max_batch"`
	Deadline        time.Duration `json:"deadline"`
	ModelCalls      int           `json:"model_calls"`
	AssignmentBytes int           `json:"assignment_bytes"`
	RequestBytes    int           `json:"request_bytes"`
	ResultBytes     int           `json:"result_bytes"`
	OutputTokens    int           `json:"output_tokens"`
}

func DefaultPolicy() Policy {
	return Policy{PerParent: 2, Runtime: 4, MaxBatch: 8, Deadline: 2 * time.Minute, ModelCalls: 8, AssignmentBytes: 8192, RequestBytes: 65536, ResultBytes: 2048, OutputTokens: 1024}
}
func (p Policy) Validate() error {
	if p.PerParent <= 0 || p.Runtime <= 0 || p.MaxBatch <= 0 || p.Deadline <= 0 || p.ModelCalls <= 0 || p.AssignmentBytes <= 0 || p.RequestBytes <= 0 || p.ResultBytes < 512 || p.OutputTokens <= 0 {
		return errors.New("subagent policy requires finite positive limits (result_bytes >= 512)")
	}
	if p.ResultBytes > 96*1024 || p.MaxBatch > (96*1024-2)/(p.ResultBytes+1) {
		return errors.New("subagent batch result envelope exceeds 96 KiB; lower max_batch or result_bytes")
	}
	return nil
}
func (p Policy) ValidateBatch(a []Assignment) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if len(a) == 0 || len(a) > p.MaxBatch {
		return errors.New("subagent batch size exceeds policy")
	}
	keys := map[string]bool{}
	for _, v := range a {
		if strings.TrimSpace(v.Key) == "" || len(v.Key) > 128 || keys[v.Key] || strings.TrimSpace(v.Objective) == "" || len(v.Objective)+len(v.Context) > p.AssignmentBytes || len(v.TaskID) > 128 {
			return errors.New("invalid or oversized subagent assignment")
		}
		keys[v.Key] = true
	}
	return nil
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	d := sha256.Sum256(b)
	return hex.EncodeToString(d[:])
}

type Parent struct {
	Scope         memory.ScopeContext
	Lease         memory.TurnLease
	SourceEventID memory.EventID
	IntentEventID memory.EventID
}

type Result struct {
	ExecutionID    string             `json:"execution_id"`
	ChildSessionID memory.SessionID   `json:"child_session_id"`
	Status         string             `json:"status"`
	Findings       string             `json:"findings,omitempty"`
	Sources        []string           `json:"sources,omitempty"`
	Limitations    []string           `json:"limitations,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Usage          *memory.TokenUsage `json:"usage"`
}

type Attempt struct {
	DispatchID   string              `json:"dispatch_id"`
	ID           string              `json:"id"`
	Parent       Parent              `json:"parent"`
	Assignment   Assignment          `json:"assignment"`
	Digest       string              `json:"digest"`
	Child        memory.Session      `json:"child"`
	Receipt      composition.Receipt `json:"receipt"`
	Policy       Policy              `json:"policy"`
	PolicyID     string              `json:"policy_id"`
	State        string              `json:"state"`
	CreatedAt    time.Time           `json:"created_at"`
	StartedAt    *time.Time          `json:"started_at,omitempty"`
	EndedAt      *time.Time          `json:"ended_at,omitempty"`
	FinalEventID memory.EventID      `json:"final_event_id,omitempty"`
	Result       *Result             `json:"result,omitempty"`
}

func (a Attempt) Terminal() bool { return a.State != "admitted" && a.State != "running" }
