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
