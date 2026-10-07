package workflow

import (
	"fmt"
	"strings"
)

// Function is one reviewed deterministic function a deterministic step may
// call by `name@version`. The registry is compiled into Evie: a definition
// cannot add one, and a new version is a new identity in the executable
// hash. Every function is pure: the same arguments give the same result.
type Function struct {
	Name    string
	Version int
	Summary string
	Params  []Param
	Result  *Type
	// Refine narrows the result type from the step's static arguments, such
	// as a list bound given by a literal. It may be nil.
	Refine func(args map[string]StaticArg) *Type
	Call   func(args map[string]any) (any, error)
}

// Param is one typed parameter. An integer parameter with Max > 0 accepts
// only Min..Max.
type Param struct {
	Name string
	Type *Type
	Min  int64
	Max  int64
}

// StaticArg is what validation knows about one argument before a run.
type StaticArg struct {
	Type    *Type
	Literal any
	IsConst bool
}

// ID is the function's registry identity.
func (f Function) ID() string { return fmt.Sprintf("%s@%d", f.Name, f.Version) }

func (p Param) checkBounds(v any) error {
	n, ok := v.(int64)
	if !ok || p.Max == 0 {
		return nil
	}
	if n < p.Min || n > p.Max {
		return fmt.Errorf("%s must be between %d and %d", p.Name, p.Min, p.Max)
	}
	return nil
}

// reviewedFunctions is the reviewed registry, keyed by ID.
func reviewedFunctions() map[string]Function {
	out := map[string]Function{}
	for _, f := range []Function{collectClaims(), expandBallots(), majorityFilter(), splitEvenly(), tallyVerdicts(), checkCitations(),
		// The built-in deep-research v2's steps (2026-10-04, research.go).
		dedupSources(), rankClaims(), expandBallotsV2(), tallyAdversarial(),
		// finance-daily's final step (2026-10-06).
		requireZero()} {
		out[f.ID()] = f
	}
	return out
}

// claimType is one collected claim.
func claimType() *Type {
	return ObjectType(
		Field{Name: "id", Type: StringType()},
		Field{Name: "angle_id", Type: StringType()},
		Field{Name: "text", Type: StringType()},
		Field{Name: "sources", Type: ArrayType(StringType())},
		Field{Name: "unverified_sources", Type: ArrayType(StringType())},
	)
}

func collectClaims() Function {
	finding := openObject(
		Field{Name: "item", Type: openObject(Field{Name: "id", Type: StringType()})},
		Field{Name: "output", Type: openObject(
			Field{Name: "sources", Type: ArrayType(openObject(Field{Name: "url", Type: StringType()}))},
			Field{Name: "result", Type: openObject(Field{Name: "claims", Type: ArrayType(openObject(
				Field{Name: "text", Type: StringType()},
				Field{Name: "sources", Type: ArrayType(StringType())},
			))})},
		)},
	)
	result := func() *Type {
		return ObjectType(
			Field{Name: "claims", Type: ArrayType(claimType())},
			Field{Name: "count", Type: IntegerType()},
			Field{Name: "dropped", Type: IntegerType()},
		)
	}
	params := []Param{
		{Name: "findings", Type: ArrayType(finding)},
		{Name: "max_claims", Type: IntegerType(), Min: 1, Max: 100},
	}
	return Function{
		Name: "collect_claims", Version: 1, Params: params, Result: result(),
		Summary: "merge claims from research results in order, keep only sources the worker fetched, cap the count",
		Refine: func(args map[string]StaticArg) *Type {
			t := result()
			if a := args["max_claims"]; a.IsConst {
				if n, ok := a.Literal.(int64); ok {
					t.Properties["claims"].MaxItems = &n
				}
			}
			return t
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			limit := args["max_claims"].(int64)
			claims := []any{}
			byKey := map[string]map[string]any{}
			dropped := int64(0)
			for _, f := range args["findings"].([]any) {
				angle, _ := walkValue(f, []string{"item", "id"}).(string)
				fetched := map[string]bool{}
				sources, _ := walkValue(f, []string{"output", "sources"}).([]any)
				for _, s := range sources {
					if u, ok := walkValue(s, []string{"url"}).(string); ok {
						fetched[u] = true
					}
				}
				found, _ := walkValue(f, []string{"output", "result", "claims"}).([]any)
				for _, c := range found {
					text := strings.Join(strings.Fields(fmt.Sprint(walkValue(c, []string{"text"}))), " ")
					if text == "" {
						continue
					}
					key := strings.ToLower(text)
					claim, seen := byKey[key]
					if !seen {
						if int64(len(claims)) >= limit {
							dropped++
							continue
						}
						claim = map[string]any{
							"id": fmt.Sprintf("c%d", len(claims)+1), "angle_id": angle, "text": text,
							"sources": []any{}, "unverified_sources": []any{},
						}
						byKey[key] = claim
						claims = append(claims, claim)
					}
					cited, _ := walkValue(c, []string{"sources"}).([]any)
					for _, s := range cited {
						u, ok := s.(string)
						if !ok {
							continue
						}
						field := "unverified_sources"
						if fetched[u] {
							field = "sources"
						}
						list := claim[field].([]any)
						if !containsValue(list, u) {
							claim[field] = append(list, u)
						}
					}
				}
			}
			return map[string]any{"claims": claims, "count": int64(len(claims)), "dropped": dropped}, nil
		},
	}
}

func ballotType() *Type {
	return ObjectType(
		Field{Name: "id", Type: StringType()},
		Field{Name: "claim_id", Type: StringType()},
		Field{Name: "text", Type: StringType()},
		Field{Name: "sources", Type: ArrayType(StringType())},
		Field{Name: "voter", Type: IntegerType()},
	)
}

func expandBallots() Function {
	params := []Param{
		{Name: "claims", Type: ArrayType(openObject(
			Field{Name: "id", Type: StringType()},
			Field{Name: "text", Type: StringType()},
			Field{Name: "sources", Type: ArrayType(StringType())},
		))},
		{Name: "votes", Type: IntegerType(), Min: 1, Max: 7},
	}
	result := func() *Type {
		return ObjectType(Field{Name: "ballots", Type: ArrayType(ballotType())}, Field{Name: "count", Type: IntegerType()})
	}
	return Function{
		Name: "expand_ballots", Version: 1, Params: params, Result: result(),
		Summary: "one ballot for each claim and voter, in claim order",
		Refine: func(args map[string]StaticArg) *Type {
			t := result()
			claims, votes := args["claims"], args["votes"]
			n, ok := votes.Literal.(int64)
			if votes.IsConst && ok && claims.Type != nil && claims.Type.MaxItems != nil {
				bound := *claims.Type.MaxItems * n
				t.Properties["ballots"].MaxItems = &bound
			}
			return t
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			votes := args["votes"].(int64)
			ballots := []any{}
			for _, c := range args["claims"].([]any) {
				id, _ := walkValue(c, []string{"id"}).(string)
				text, _ := walkValue(c, []string{"text"}).(string)
				sources, _ := walkValue(c, []string{"sources"}).([]any)
				for voter := int64(1); voter <= votes; voter++ {
					ballots = append(ballots, map[string]any{
						"id": fmt.Sprintf("%s-v%d", id, voter), "claim_id": id, "text": text,
						"sources": append([]any{}, sources...), "voter": voter,
					})
				}
			}
			return map[string]any{"ballots": ballots, "count": int64(len(ballots))}, nil
		},
	}
}

var verdicts = []string{"supported", "refuted", "unclear"}

func majorityFilter() Function {
	params := []Param{
		{Name: "claims", Type: ArrayType(openObject(Field{Name: "id", Type: StringType()}))},
		{Name: "votes", Type: ArrayType(openObject(
			Field{Name: "item", Type: openObject(Field{Name: "claim_id", Type: StringType()})},
			Field{Name: "output", Type: openObject(Field{Name: "result", Type: openObject(Field{Name: "verdict", Type: StringType()})})},
		))},
		{Name: "quorum", Type: IntegerType(), Min: 1, Max: 7},
	}
	tallied := func(claim *Type) *Type {
		t := claim.clone()
		if t.isAny() || t.Kind != KindObject {
			t = openObject(Field{Name: "id", Type: StringType()})
		}
		for _, v := range verdicts {
			t.Properties[v] = IntegerType()
			if !t.requires(v) {
				t.Required = sortedStrings(append(t.Required, v))
			}
		}
		return t
	}
	result := func(claim *Type, bound *int64) *Type {
		accepted, rejected := ArrayType(tallied(claim)), ArrayType(tallied(claim))
		accepted.MaxItems, rejected.MaxItems = bound, bound
		return ObjectType(
			Field{Name: "accepted", Type: accepted},
			Field{Name: "rejected", Type: rejected},
			Field{Name: "accepted_count", Type: IntegerType()},
			Field{Name: "rejected_count", Type: IntegerType()},
		)
	}
	return Function{
		Name: "majority_filter", Version: 1, Params: params, Result: result(AnyType(), nil),
		Summary: "accept a claim with at least quorum supported votes; a missing vote counts against",
		Refine: func(args map[string]StaticArg) *Type {
			claims := args["claims"].Type
			if claims == nil || claims.Kind != KindArray {
				return result(AnyType(), nil)
			}
			return result(claims.Items, claims.MaxItems)
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			quorum := args["quorum"].(int64)
			claims := args["claims"].([]any)
			tally := map[string]map[string]int64{}
			for _, c := range claims {
				id, _ := walkValue(c, []string{"id"}).(string)
				tally[id] = map[string]int64{"supported": 0, "refuted": 0, "unclear": 0}
			}
			for _, v := range args["votes"].([]any) {
				id, _ := walkValue(v, []string{"item", "claim_id"}).(string)
				counts, ok := tally[id]
				if !ok {
					return nil, fmt.Errorf("a vote names claim %q, which is not in claims", id)
				}
				verdict, _ := walkValue(v, []string{"output", "result", "verdict"}).(string)
				if _, ok := counts[verdict]; !ok {
					return nil, fmt.Errorf("vote for claim %s has verdict %q; expected %s", id, verdict, strings.Join(verdicts, ", "))
				}
				counts[verdict]++
			}
			accepted, rejected := []any{}, []any{}
			for _, c := range claims {
				m, _ := c.(map[string]any)
				id, _ := m["id"].(string)
				out := map[string]any{}
				for k, v := range m {
					out[k] = v
				}
				for k, n := range tally[id] {
					out[k] = n
				}
				if tally[id]["supported"] >= quorum {
					accepted = append(accepted, out)
				} else {
					rejected = append(rejected, out)
				}
			}
			return map[string]any{
				"accepted": accepted, "rejected": rejected,
				"accepted_count": int64(len(accepted)), "rejected_count": int64(len(rejected)),
			}, nil
		},
	}
}

// Bounds of split_evenly@1.
const (
	maxSplitTotal = 1_000_000_000_000
	maxSplitParts = 1_000
)

// splitEvenly splits a whole-number total (such as cents) into parts that
// differ by at most one and add up to the total exactly; the first
// `remainder` shares carry the extra unit (Stage 14 sub-stage 3, the
// deterministic example for manual runs).
func splitEvenly() Function {
	params := []Param{
		{Name: "total", Type: IntegerType(), Min: 0, Max: maxSplitTotal},
		{Name: "parts", Type: IntegerType(), Min: 1, Max: maxSplitParts},
	}
	result := func(minItems, maxItems int64) *Type {
		shares := ArrayType(IntegerType())
		shares.MinItems, shares.MaxItems = &minItems, &maxItems
		return ObjectType(
			Field{Name: "shares", Type: shares},
			Field{Name: "share", Type: IntegerType()},
			Field{Name: "remainder", Type: IntegerType()},
		)
	}
	return Function{
		Name: "split_evenly", Version: 1, Params: params, Result: result(1, maxSplitParts),
		Summary: "split a whole-number total into parts that differ by at most one and add up to it exactly",
		Refine: func(args map[string]StaticArg) *Type {
			if a := args["parts"]; a.IsConst {
				if n, ok := a.Literal.(int64); ok {
					return result(n, n)
				}
			}
			return result(1, maxSplitParts)
		},
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			total, parts := args["total"].(int64), args["parts"].(int64)
			share, remainder := total/parts, total%parts
			shares := make([]any, parts)
			for i := range shares {
				shares[i] = share
				if int64(i) < remainder {
					shares[i] = share + 1
				}
			}
			return map[string]any{"shares": shares, "share": share, "remainder": remainder}, nil
		},
	}
}

// checkArgs validates arguments at call time: shapes against the declared
// types and integer bounds.
func checkArgs(params []Param, args map[string]any) error {
	for _, p := range params {
		v, ok := args[p.Name]
		if !ok {
			return fmt.Errorf("missing argument %s", p.Name)
		}
		if err := checkAt(p.Type, v, p.Name); err != nil {
			return err
		}
		if err := p.checkBounds(v); err != nil {
			return err
		}
	}
	return nil
}

func containsValue(list []any, v any) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Bounds of require_zero@1.
const (
	maxRequireCount   = 1_000_000_000
	maxRequireSubject = 200
)

// requireZero fails its step, and so the run once its Retry Policy is
// spent, when a count a prior step produced is not zero: the reviewed way
// for a workflow to turn "some of what the effect reported went wrong"
// into a failed run with a durable notification, after the steps that
// should still happen have happened (finance-daily, 2026-10-06). It is
// pure: the same count and subject always give the same answer.
func requireZero() Function {
	subject := StringType()
	maxSubject := int64(maxRequireSubject)
	subject.MaxLength = &maxSubject
	params := []Param{
		{Name: "count", Type: IntegerType(), Min: 0, Max: maxRequireCount},
		{Name: "subject", Type: subject},
	}
	return Function{
		Name: "require_zero", Version: 1, Params: params,
		Result:  ObjectType(Field{Name: "count", Type: IntegerType()}),
		Summary: "fail the step unless a count is zero, naming what was counted",
		Call: func(args map[string]any) (any, error) {
			if err := checkArgs(params, args); err != nil {
				return nil, err
			}
			count, subject := args["count"].(int64), args["subject"].(string)
			if strings.TrimSpace(subject) == "" {
				return nil, fmt.Errorf("subject must name what was counted")
			}
			if count != 0 {
				return nil, fmt.Errorf("%d %s", count, strings.TrimSpace(subject))
			}
			return map[string]any{"count": int64(0)}, nil
		},
	}
}
