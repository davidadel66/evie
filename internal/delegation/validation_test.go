package delegation

import (
	"strings"
	"testing"
)

// G7: each validation failure names the assignment, the field and the limit,
// so the parent model can correct the exact call instead of guessing.
func TestValidateBatchNamesAssignmentFieldAndLimit(t *testing.T) {
	p := DefaultPolicy()
	long := func(n int) string { return strings.Repeat("x", n) }
	cases := []struct {
		name    string
		batch   []Assignment
		want    []string
		wantNot []string
	}{
		{"empty", nil, []string{"at least one assignment"}, nil},
		{"too_many", make([]Assignment, 9), []string{"9 assignments", "limit 8"}, nil},
		{"blank_key", []Assignment{{Key: "ok", Objective: "a"}, {Key: " ", Objective: "b"}}, []string{"assignment 2", "idempotency_key", "blank"}, []string{`"ok"`}},
		{"long_key", []Assignment{{Key: long(129), Objective: "a"}}, []string{"assignment 1", "idempotency_key is 129 bytes", "limit 128"}, nil},
		{"duplicate_key", []Assignment{{Key: "r1", Objective: "a"}, {Key: "r1", Objective: "b"}}, []string{"assignments 1 and 2", `"r1"`}, nil},
		{"blank_objective", []Assignment{{Key: "r2", Objective: "\t"}}, []string{`assignment "r2"`, "objective", "blank"}, nil},
		{"oversized", []Assignment{{Key: "r3", Objective: long(1022), Context: long(8192)}}, []string{`assignment "r3"`, "objective plus context is 9,214 bytes", "limit 8,192"}, nil},
		{"long_task", []Assignment{{Key: "r4", Objective: "a", TaskID: long(200)}}, []string{`assignment "r4"`, "task_id is 200 bytes", "limit 128"}, nil},
		{"several", []Assignment{{Key: "a", Objective: ""}, {Key: "b", Objective: "ok", TaskID: long(129)}}, []string{`assignment "a" objective is blank`, `assignment "b" task_id is 129 bytes`}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := p.ValidateBatch(c.batch)
			if err == nil {
				t.Fatal("invalid batch accepted")
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %q", err, want)
				}
			}
			for _, unwanted := range c.wantNot {
				if strings.Contains(err.Error(), unwanted) {
					t.Errorf("error %q blames %q", err, unwanted)
				}
			}
		})
	}
	if err := p.ValidateBatch([]Assignment{{Key: "a", Objective: "a", Context: long(8000), TaskID: long(128)}, {Key: long(128), Objective: long(8192)}}); err != nil {
		t.Fatalf("batch at the limits rejected: %v", err)
	}
}

func TestValidateContinuationNamesFieldAndLimit(t *testing.T) {
	p := DefaultPolicy()
	cases := []struct {
		id, message string
		want        []string
	}{
		{"", "more", []string{"execution_id", "blank"}},
		{strings.Repeat("e", 129), "more", []string{"execution_id is 129 bytes", "limit 128"}},
		{"e1", " ", []string{"message", "blank"}},
		{"e1", strings.Repeat("m", 9000), []string{"message is 9,000 bytes", "limit 8,192"}},
	}
	for _, c := range cases {
		err := p.ValidateContinuation(c.id, c.message)
		if err == nil {
			t.Fatalf("continuation %q accepted", c.want)
		}
		for _, want := range c.want {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not name %q", err, want)
			}
		}
	}
}

func TestPolicyValidationNamesTheInvalidSetting(t *testing.T) {
	p := DefaultPolicy()
	p.PerTurn, p.TokenBudget = 0, -1
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "per_turn") || !strings.Contains(err.Error(), "token_budget") || strings.Contains(err.Error(), "per_parent") {
		t.Fatalf("policy error %v does not name exactly the invalid settings", err)
	}
	p = DefaultPolicy()
	p.ResultBytes = 100
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "result_bytes is 100") || !strings.Contains(err.Error(), "512") {
		t.Fatalf("result_bytes error: %v", err)
	}
	p = DefaultPolicy()
	p.ResultBytes = 20000
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "max_batch 8") || !strings.Contains(err.Error(), "result_bytes 20,000") {
		t.Fatalf("envelope error: %v", err)
	}
}

// Stage 7: only explore assignments declare a path, bounded like a key; an
// assignment without one keeps its pre-Stage 7 digest.
func TestExploreBatchValidationAndUnchangedResearchDigests(t *testing.T) {
	p := DefaultPolicy()
	if err := p.ValidateExploreBatch([]Assignment{{Key: "e", Objective: "look", Path: "docs"}}); err != nil {
		t.Fatalf("explore path refused: %v", err)
	}
	err := p.ValidateExploreBatch([]Assignment{{Key: "e", Objective: "look", Path: strings.Repeat("p", MaxPathBytes+1)}})
	if err == nil || !strings.Contains(err.Error(), `invalid delegate_explore request: assignment "e" path is 1,025 bytes; limit 1,024`) {
		t.Fatalf("long path: %v", err)
	}
	if err := p.ValidateBatch([]Assignment{{Key: "r", Objective: "look", Path: "docs"}}); err == nil || !strings.Contains(err.Error(), `assignment "r" path is not supported by delegate_research`) {
		t.Fatalf("research path: %v", err)
	}
	if err := p.ValidateExploreBatch(nil); err == nil || !strings.Contains(err.Error(), "delegate_explore requires at least one assignment") {
		t.Fatalf("empty explore batch: %v", err)
	}
	if got := Digest(Assignment{Key: "k", Objective: "o"}); got != Digest(struct {
		Key       string `json:"idempotency_key"`
		Objective string `json:"objective"`
	}{"k", "o"}) {
		t.Fatal("an assignment without a path no longer digests as before")
	}
}

// Stage 11: a build assignment names its Task and may name a base; base is
// refused by the other workers, and an assignment without one keeps its
// earlier digest.
func TestBuildBatchValidationRequiresTaskAndBoundsBase(t *testing.T) {
	p := DefaultPolicy()
	if err := p.ValidateBuildBatch([]Assignment{{Key: "b", Objective: "fix", TaskID: "t1", Base: "main"}}); err != nil {
		t.Fatalf("build assignment refused: %v", err)
	}
	if err := p.ValidateBuildBatch([]Assignment{{Key: "b", Objective: "fix"}}); err == nil || !strings.Contains(err.Error(), `invalid delegate_build request: assignment "b" task_id is blank`) {
		t.Fatalf("missing task: %v", err)
	}
	if err := p.ValidateBuildBatch([]Assignment{{Key: "b", Objective: "fix", TaskID: "t", Base: strings.Repeat("x", MaxBaseBytes+1)}}); err == nil || !strings.Contains(err.Error(), `assignment "b" base is 201 bytes; limit 200`) {
		t.Fatalf("long base: %v", err)
	}
	if err := p.ValidateBuildBatch([]Assignment{{Key: "b", Objective: "fix", TaskID: "t", Path: "docs"}}); err == nil || !strings.Contains(err.Error(), `assignment "b" path is not supported by delegate_build`) {
		t.Fatalf("build path: %v", err)
	}
	for _, validate := range []func([]Assignment) error{p.ValidateBatch, p.ValidateExploreBatch} {
		if err := validate([]Assignment{{Key: "r", Objective: "look", Base: "main"}}); err == nil || !strings.Contains(err.Error(), `assignment "r" base is not supported by delegate_`) {
			t.Fatalf("base outside build: %v", err)
		}
	}
	if got := Digest(Assignment{Key: "k", Objective: "o", TaskID: "t"}); got != Digest(struct {
		Key       string `json:"idempotency_key"`
		Objective string `json:"objective"`
		TaskID    string `json:"task_id"`
	}{"k", "o", "t"}) {
		t.Fatal("an assignment without a base no longer digests as before")
	}
	if DelegationToolName(WorkerBuild) != BuildToolName || !AdmitsChildren(BuildToolName) {
		t.Fatal("delegate_build does not admit build children")
	}
}
