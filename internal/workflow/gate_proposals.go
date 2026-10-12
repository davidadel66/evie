package workflow

import (
	"fmt"
	"math"
	"strings"
)

// gate_proposals@1 (finance-review.spec.md, 2026-10-07): the deterministic
// gate between a model's proposed classifications and the owner's review.
// It keeps a proposal only when its id names a candidate, its category is
// in the vocabulary exactly, its confidence reaches the threshold, it is
// the first proposal for that id, and it asks for a rule only when the
// candidate has a merchant name (a rule is an exact Plaid merchant
// spelling, never a bank string). Lines keep candidate order and carry
// what the owner needs to decide without opening anything else. Pure.

// Bounds of gate_proposals@1.
const (
	maxGateCandidates = 40
	maxGateReason     = 200
)

func gateLineType() *Type {
	reason := StringType()
	maxReason := int64(maxGateReason)
	reason.MaxLength = &maxReason
	return ObjectType(
		Field{Name: "id", Type: StringType()},
		Field{Name: "date", Type: StringType()},
		Field{Name: "name", Type: StringType()},
		Field{Name: "merchant_name", Type: StringType()},
		Field{Name: "net_cents", Type: IntegerType()},
		Field{Name: "category", Type: StringType()},
		Field{Name: "confidence", Type: NumberType()},
		Field{Name: "reason", Type: reason},
		Field{Name: "rule", Type: BooleanType()},
	)
}

func gateProposals() Function {
	candidate := openObject(
		Field{Name: "id", Type: StringType()},
		Field{Name: "date", Type: StringType()},
		Field{Name: "name", Type: StringType()},
		Field{Name: "merchant_name", Type: StringType()},
		Field{Name: "net_cents", Type: IntegerType()},
	)
	proposal := openObject(
		Field{Name: "id", Type: StringType()},
		Field{Name: "category", Type: StringType()},
		Field{Name: "confidence", Type: NumberType()},
		Field{Name: "reason", Type: StringType()},
		Field{Name: "rule", Type: BooleanType()},
	)
	zero, one := 0.0, 1.0
	threshold := NumberType()
	threshold.Minimum, threshold.Maximum = &zero, &one
	params := []Param{
		{Name: "candidates", Type: ArrayType(candidate)},
		{Name: "proposals", Type: ArrayType(proposal)},
		{Name: "categories", Type: ArrayType(StringType())},
		{Name: "min_confidence", Type: threshold},
	}
	result := func() *Type {
		lines := ArrayType(gateLineType())
		maxLines := int64(maxGateCandidates)
		lines.MaxItems = &maxLines
		return ObjectType(
			Field{Name: "lines", Type: lines},
			Field{Name: "kept", Type: IntegerType()},
			Field{Name: "dropped", Type: IntegerType()},
		)
	}
	return Function{
		Name: "gate_proposals", Version: 1, Params: params, Result: result(),
		Summary: "keep only proposed classifications that name a candidate, use a known category exactly, reach the confidence threshold and ask for a rule only with a merchant name",
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			minConfidence, err := gateNumber(args["min_confidence"])
			if err != nil || minConfidence < 0 || minConfidence > 1 {
				return nil, fmt.Errorf("min_confidence must be between 0 and 1")
			}
			known := map[string]bool{}
			for _, c := range args["categories"].([]any) {
				if s, ok := c.(string); ok && strings.TrimSpace(s) != "" {
					known[s] = true
				}
			}
			candidates := args["candidates"].([]any)
			if len(candidates) > maxGateCandidates {
				return nil, fmt.Errorf("at most %d candidates can be gated at once", maxGateCandidates)
			}
			order := map[string]int{}
			byID := map[string]map[string]any{}
			for i, c := range candidates {
				m, _ := c.(map[string]any)
				id, _ := m["id"].(string)
				if id == "" {
					return nil, fmt.Errorf("candidates[%d] has no id", i)
				}
				if _, dup := byID[id]; dup {
					return nil, fmt.Errorf("candidate %q appears twice", id)
				}
				order[id], byID[id] = i, m
			}
			lines := make([]any, len(candidates))
			kept, dropped := int64(0), int64(0)
			seen := map[string]bool{}
			for _, p := range args["proposals"].([]any) {
				m, _ := p.(map[string]any)
				id, _ := m["id"].(string)
				category, _ := m["category"].(string)
				reason, _ := m["reason"].(string)
				rule, _ := m["rule"].(bool)
				confidence, err := gateNumber(m["confidence"])
				c, isCandidate := byID[id]
				merchant, _ := c["merchant_name"].(string)
				switch {
				case !isCandidate, seen[id], !known[category], err != nil, confidence < minConfidence, rule && strings.TrimSpace(merchant) == "":
					dropped++
					continue
				}
				seen[id] = true
				reason = strings.Join(strings.Fields(reason), " ")
				if len(reason) > maxGateReason {
					reason = reason[:maxGateReason]
				}
				net, _ := c["net_cents"].(int64)
				lines[order[id]] = map[string]any{
					"id": id, "date": fmt.Sprint(c["date"]), "name": fmt.Sprint(c["name"]), "merchant_name": merchant,
					"net_cents": net, "category": category, "confidence": confidence, "reason": reason, "rule": rule,
				}
				kept++
			}
			out := make([]any, 0, kept)
			for _, line := range lines {
				if line != nil {
					out = append(out, line)
				}
			}
			return map[string]any{"lines": out, "kept": kept, "dropped": dropped}, nil
		},
	}
}

// gateNumber accepts the integer or float a decoded JSON number may be.
func gateNumber(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, fmt.Errorf("not a finite number")
		}
		return n, nil
	case int64:
		return float64(n), nil
	}
	return 0, fmt.Errorf("not a number")
}
