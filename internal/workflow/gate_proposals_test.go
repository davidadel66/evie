package workflow

import (
	"reflect"
	"testing"
)

func gateCandidate(id, merchant string, net int64) map[string]any {
	return map[string]any{"id": id, "date": "2026-09-30", "name": "BANK " + id, "merchant_name": merchant, "net_cents": net, "suggested_category": ""}
}

func gateProposal(id, category string, confidence float64, rule bool) map[string]any {
	return map[string]any{"id": id, "category": category, "confidence": confidence, "reason": "  because   evidence  ", "rule": rule}
}

// gate_proposals@1 keeps candidate order, drops every proposal that breaks
// a rule, and reports how many it kept and dropped.
func TestWorkflowGateProposalsKeepsOnlyValidLinesInCandidateOrder(t *testing.T) {
	candidates := []any{gateCandidate("a", "Notion Labs Inc.", -2400), gateCandidate("b", "", -38700), gateCandidate("c", "Roca", -10800), gateCandidate("d", "Watch It", -249)}
	proposals := []any{
		gateProposal("c", "Dining", 0.9, true),         // kept, out of order
		gateProposal("a", "Subscriptions", 0.95, true), // kept
		gateProposal("a", "Software", 0.99, true),      // duplicate id, dropped
		gateProposal("b", "excluded", 0.8, true),       // rule on a bank string, dropped
		gateProposal("d", "Entertainment", 0.4, true),  // below threshold, dropped
		gateProposal("zz", "Dining", 0.9, false),       // not a candidate, dropped
		gateProposal("d", "entertainment", 0.9, false), // wrong case, dropped
	}
	out := call(t, "gate_proposals@1", map[string]any{"candidates": candidates, "proposals": proposals,
		"categories": []any{"Dining", "Subscriptions", "Entertainment", "excluded"}, "min_confidence": 0.6})
	if out["kept"] != int64(2) || out["dropped"] != int64(5) {
		t.Fatalf("kept, dropped = %v, %v", out["kept"], out["dropped"])
	}
	lines := out["lines"].([]any)
	want := []any{
		map[string]any{"id": "a", "date": "2026-09-30", "name": "BANK a", "merchant_name": "Notion Labs Inc.", "net_cents": int64(-2400),
			"category": "Subscriptions", "confidence": 0.95, "reason": "because evidence", "rule": true},
		map[string]any{"id": "c", "date": "2026-09-30", "name": "BANK c", "merchant_name": "Roca", "net_cents": int64(-10800),
			"category": "Dining", "confidence": 0.9, "reason": "because evidence", "rule": true},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines = %#v", lines)
	}
	// A bank-string candidate may still be classified without a rule.
	out = call(t, "gate_proposals@1", map[string]any{"candidates": candidates, "proposals": []any{gateProposal("b", "excluded", 0.8, false)},
		"categories": []any{"excluded"}, "min_confidence": 0.6})
	if out["kept"] != int64(1) {
		t.Fatalf("bank string without rule = %#v", out)
	}
	f := reviewedFunctions()["gate_proposals@1"]
	for name, args := range map[string]map[string]any{
		"threshold above one": {"candidates": candidates, "proposals": []any{}, "categories": []any{}, "min_confidence": 1.5},
		"duplicate candidate": {"candidates": []any{gateCandidate("a", "", 1), gateCandidate("a", "", 1)}, "proposals": []any{}, "categories": []any{}, "min_confidence": 0.5},
	} {
		if _, err := f.Call(args); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
