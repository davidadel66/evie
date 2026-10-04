package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func splitBill(t *testing.T) *Validated {
	t.Helper()
	return mustValidate(t, filepath.Join("testdata", "split-bill"), testEnvironment())
}

func rawInputs(t *testing.T, raw string) map[string]json.RawMessage {
	t.Helper()
	out, err := ParseInputs([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// split_evenly@1 splits whole cents exactly: the shares add up to the total
// and the first shares carry the remainder.
func TestWorkflowSplitEvenlyIsExactAndBounded(t *testing.T) {
	out := call(t, "split_evenly@1", map[string]any{"total": int64(1000), "parts": int64(3)})
	want := map[string]any{"shares": []any{int64(334), int64(333), int64(333)}, "share": int64(333), "remainder": int64(1)}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("1000/3 = %#v", out)
	}
	out = call(t, "split_evenly@1", map[string]any{"total": int64(900), "parts": int64(3)})
	if out["remainder"] != int64(0) || !reflect.DeepEqual(out["shares"], []any{int64(300), int64(300), int64(300)}) {
		t.Fatalf("900/3 = %#v", out)
	}
	f := reviewedFunctions()["split_evenly@1"]
	for name, args := range map[string]map[string]any{
		"too many parts": {"total": int64(10), "parts": int64(1001)},
		"no parts":       {"total": int64(10), "parts": int64(0)},
		"negative total": {"total": int64(-1), "parts": int64(2)},
		"not an integer": {"total": 1.5, "parts": int64(2)},
	} {
		if _, err := f.Call(args); err == nil {
			t.Fatalf("%s: accepted %v", name, args)
		}
	}
	refined := f.Refine(map[string]StaticArg{"total": {Type: IntegerType()}, "parts": {Type: IntegerType(), Literal: int64(4), IsConst: true}})
	shares := refined.Properties["shares"]
	if shares.MinItems == nil || *shares.MinItems != 4 || shares.MaxItems == nil || *shares.MaxItems != 4 {
		t.Fatalf("refined shares = %+v", shares)
	}
	if open := f.Refine(map[string]StaticArg{"total": {Type: IntegerType()}, "parts": {Type: IntegerType()}}); *open.Properties["shares"].MaxItems != 1000 {
		t.Fatalf("shares are not bounded by the parts bound: %+v", open.Properties["shares"])
	}
}

// The split-bill example validates, needs no capability, model or worker,
// and simulates with only its inputs.
func TestWorkflowSplitBillExampleSimulates(t *testing.T) {
	v := splitBill(t)
	if got := v.StepKinds(); !reflect.DeepEqual(got, []string{KindBranch, KindDeterministic}) {
		t.Fatalf("kinds = %v", got)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "split-bill.fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := simulate(t, v, fixtures(t, string(data)))
	if s.Status != SimulationSucceeded || !reflect.DeepEqual(s.Outputs["leftover_cents"], int64(1)) {
		t.Fatalf("simulation = %s %s %v", s.Status, s.Error, s.Outputs)
	}
	if _, ok := s.Outputs["per_person"]; ok {
		t.Fatalf("per_person produced on the leftover path: %v", s.Outputs)
	}
}

// The execution primitives a run uses are the simulation's own semantics:
// same accepted inputs, Run Key, step inputs, branch choice and outputs.
func TestWorkflowExecutionPrimitivesMatchSimulation(t *testing.T) {
	v := splitBill(t)
	if v.Start() != "split" {
		t.Fatalf("start = %q", v.Start())
	}
	split, ok := v.Step("split")
	if !ok || split.Kind != KindDeterministic || split.Attempts != 3 || split.Next != "check" || split.Function != "split_evenly@1" {
		t.Fatalf("split = %+v", split)
	}
	if end, _ := v.Step("leftover"); end.Next != "" || end.Attempts != 1 {
		t.Fatalf("leftover = %+v", end)
	}
	if _, ok := v.Step("missing"); ok {
		t.Fatal("unknown step found")
	}
	if l := v.Limits(); l.Attempts != 10 || l.Duration.Minutes() != 5 || l.Parallel != 1 || l.Tokens != 1 {
		t.Fatalf("limits = %+v", l)
	}
	inputs, err := v.AcceptInputs(rawInputs(t, `{"bill_id": "dinner-1", "total_cents": 1000}`), map[string]string{"people": "3"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inputs, map[string]any{"bill_id": "dinner-1", "total_cents": int64(1000), "people": int64(3)}) {
		t.Fatalf("inputs = %#v", inputs)
	}
	sim := simulate(t, v, fixtures(t, `{"inputs": {"bill_id": "dinner-1", "total_cents": 1000, "people": 3}}`))
	if key := v.RunKey(inputs); key != sim.RunKey || !strings.HasPrefix(key, "split-bill:sha256:") {
		t.Fatalf("run key %q, simulation %q", key, sim.RunKey)
	}
	if other, _ := v.AcceptInputs(nil, map[string]string{"bill_id": "dinner-1", "total_cents": "5", "people": "2"}); v.RunKey(other) != v.RunKey(inputs) {
		t.Fatal("the run key depends on inputs outside the run key rule")
	}

	state := map[string]any{}
	in, err := v.StepInput("split", inputs, state)
	if err != nil || !reflect.DeepEqual(in, map[string]any{"total": int64(1000), "parts": int64(3)}) {
		t.Fatalf("split input = %#v, %v", in, err)
	}
	out, err := v.CallFunction("split", in)
	if err != nil {
		t.Fatal(err)
	}
	state["split"] = out
	d, err := v.Branch("check", inputs, state)
	if err != nil || d.Next != "leftover" || d.Case != 0 || !strings.Contains(d.Note, "default") {
		t.Fatalf("branch = %+v, %v", d, err)
	}
	if !reflect.DeepEqual(d.Read, map[string]any{"$steps.split.output.remainder": int64(1)}) {
		t.Fatalf("branch read = %#v", d.Read)
	}
	in, _ = v.StepInput("leftover", inputs, state)
	state["leftover"], err = v.CallFunction("leftover", in)
	if err != nil {
		t.Fatal(err)
	}
	if outputs := v.Outputs(inputs, state); !reflect.DeepEqual(outputs, sim.Outputs) {
		t.Fatalf("outputs = %#v, simulation %#v", outputs, sim.Outputs)
	}

	even := map[string]any{"split": map[string]any{"shares": []any{int64(300), int64(300), int64(300)}, "share": int64(300), "remainder": int64(0)}}
	if d, _ := v.Branch("check", inputs, even); d.Next != "per_person" || d.Case != 1 {
		t.Fatalf("even branch = %+v", d)
	}
	if _, err := v.CallFunction("check", nil); err == nil {
		t.Fatal("a branch was called as a function")
	}
	if _, err := v.StepInput("split", inputs, map[string]any{}); err != nil {
		t.Fatalf("split reads only inputs: %v", err)
	}
	if _, err := v.StepInput("leftover", inputs, map[string]any{}); err == nil {
		t.Fatal("leftover read the output of a step that has not run")
	}
}

// A deterministic function that fails is an attempt failure, never a crash:
// errors and panics are returned, and a result outside the function's
// declared type is refused.
func TestWorkflowCallFunctionReturnsFailures(t *testing.T) {
	v := splitBill(t)
	if _, err := v.CallFunction("split", map[string]any{"total": int64(10), "parts": int64(5000)}); err == nil || !strings.Contains(err.Error(), "parts must be between 1 and 1000") {
		t.Fatalf("out of bounds: %v", err)
	}
	if _, err := v.CallFunction("missing", nil); err == nil {
		t.Fatal("unknown step called")
	}
}

// Run inputs are accepted exactly as declared: text values from the command
// line convert only to the declared type, and nothing is given twice.
func TestWorkflowAcceptInputsChecksTypesAndSources(t *testing.T) {
	v := splitBill(t)
	good := map[string]string{"bill_id": "b", "total_cents": "10", "people": "2"}
	for name, c := range map[string]struct {
		json string
		text map[string]string
		want string
	}{
		"missing required":  {text: map[string]string{"bill_id": "b", "total_cents": "10"}, want: "inputs.people: required"},
		"unknown":           {text: map[string]string{"bill_id": "b", "total_cents": "10", "people": "2", "tip": "1"}, want: "inputs.tip"},
		"decimal integer":   {text: map[string]string{"bill_id": "b", "total_cents": "10.5", "people": "2"}, want: "inputs.total_cents"},
		"signed integer":    {text: map[string]string{"bill_id": "b", "total_cents": "+10", "people": "2"}, want: "inputs.total_cents"},
		"below minimum":     {text: map[string]string{"bill_id": "b", "total_cents": "-1", "people": "2"}, want: "inputs.total_cents"},
		"string as integer": {json: `{"total_cents": "10"}`, text: map[string]string{"bill_id": "b", "people": "2"}, want: "inputs.total_cents"},
		"given twice":       {json: `{"people": 2}`, text: good, want: "inputs.people: given twice"},
		"too long":          {text: map[string]string{"bill_id": strings.Repeat("b", 65), "total_cents": "10", "people": "2"}, want: "inputs.bill_id"},
	} {
		var raw map[string]json.RawMessage
		if c.json != "" {
			raw = rawInputs(t, c.json)
		}
		_, err := v.AcceptInputs(raw, c.text)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("%s: %v, want %q", name, err, c.want)
		}
	}
	if _, err := v.AcceptInputs(nil, good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`[]`, `{"a": 1, "a": 2}`, `{"a": 1} {}`, `nope`} {
		if _, err := ParseInputs([]byte(bad)); err == nil {
			t.Fatalf("ParseInputs accepted %s", bad)
		}
	}
}

// Values decoded from the database are canonical, so a step reading a
// checkpoint after a restart sees integers as integers.
func TestWorkflowDecodeValueIsCanonical(t *testing.T) {
	v, err := DecodeValue([]byte(`{"a": 1, "b": 1.5, "c": [2, "x", true], "d": {"e": 3}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"a": int64(1), "b": 1.5, "c": []any{int64(2), "x", true}, "d": map[string]any{"e": int64(3)}}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("decoded %#v", v)
	}
	data, err := EncodeValue(want)
	if err != nil || string(data) != `{"a":1,"b":1.5,"c":[2,"x",true],"d":{"e":3}}` {
		t.Fatalf("encoded %s, %v", data, err)
	}
	if _, err := DecodeValue([]byte(`{} {}`)); err == nil {
		t.Fatal("two documents decoded")
	}
	if _, err := DecodeValue([]byte(`{`)); err == nil {
		t.Fatal("broken JSON decoded")
	}
}

// Step kinds include map items, so a runtime can refuse what it cannot run.
func TestWorkflowStepKindsIncludeItems(t *testing.T) {
	research := mustValidate(t, filepath.Join("testdata", "deep-research"), testEnvironment())
	if got := research.StepKinds(); !reflect.DeepEqual(got, []string{KindAgent, KindAI, KindDeterministic, KindMap}) {
		t.Fatalf("deep-research kinds = %v", got)
	}
	notes := mustValidate(t, filepath.Join("testdata", "notes-digest"), testEnvironment())
	if got := notes.StepKinds(); !reflect.DeepEqual(got, []string{KindAI, KindBranch, KindCapability, KindInterrupt}) {
		t.Fatalf("notes-digest kinds = %v", got)
	}
}
