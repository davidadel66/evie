package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/workflow"
)

// finance-review capabilities (finance-review.spec.md, 2026-10-07). The
// review_candidates read returns the unclassified posted transactions
// with the deterministic evidence a reviewer would gather, computed in
// the finance package; the workflow's model proposes from it and never
// writes.

const (
	FinanceReviewCandidatesCapabilityID CapabilityID = "finance.review_candidates"

	financeReviewContractVersion = "1.0.0"
)

func financeReviewReads() []WorkflowCapability {
	evidence := workflow.ObjectType(
		workflow.Field{Name: "label", Type: workflow.StringType()},
		workflow.Field{Name: "category", Type: workflow.StringType()},
		workflow.Field{Name: "source", Type: workflow.StringType()},
		workflow.Field{Name: "count", Type: workflow.IntegerType()},
	)
	similar := workflow.ArrayType(evidence)
	maxEvidence := int64(finance.ReviewMaxEvidence)
	similar.MaxItems = &maxEvidence
	candidate := workflow.ObjectType(
		workflow.Field{Name: "id", Type: workflow.StringType()},
		workflow.Field{Name: "date", Type: workflow.StringType()},
		workflow.Field{Name: "name", Type: workflow.StringType()},
		workflow.Field{Name: "merchant_name", Type: workflow.StringType()},
		workflow.Field{Name: "net_cents", Type: workflow.IntegerType()},
		workflow.Field{Name: "plaid_category", Type: workflow.StringType()},
		workflow.Field{Name: "similar", Type: similar},
		workflow.Field{Name: "suggested_category", Type: workflow.StringType()},
		workflow.Field{Name: "suggestion_reason", Type: workflow.StringType()},
	)
	candidates := workflow.ArrayType(candidate)
	maxCandidates := int64(finance.ReviewMaxCandidates)
	candidates.MaxItems = &maxCandidates
	return []WorkflowCapability{{
		ID: FinanceReviewCandidatesCapabilityID, ContractVersion: financeReviewContractVersion,
		Connections: []string{FinanceConnectionID}, Resources: []string{FinanceLedgerResource},
		Input: workflow.ObjectType(workflow.Field{Name: "limit", Type: boundedInteger(1, finance.ReviewMaxCandidates), Optional: true}),
		Output: workflow.ObjectType(
			workflow.Field{Name: "candidates", Type: candidates},
			workflow.Field{Name: "remaining", Type: workflow.IntegerType()},
			workflow.Field{Name: "categories", Type: workflow.ArrayType(workflow.StringType())},
		),
		Read: readReviewCandidates,
	}}
}

func readReviewCandidates(ctx context.Context, req WorkflowReadRequest) (any, error) {
	if err := financeLedger(req); err != nil {
		return nil, err
	}
	in := struct {
		Limit int `json:"limit"`
	}{Limit: finance.ReviewDefaultLimit}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return nil, err
	}
	db, err := finance.OpenDBContext(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	report, err := finance.ReviewCandidates(ctx, db, in.Limit)
	if err != nil {
		return nil, err
	}
	candidates := make([]any, 0, len(report.Candidates))
	for _, c := range report.Candidates {
		net, err := financeCents(c.NetCents)
		if err != nil {
			return nil, err
		}
		similar := make([]any, 0, len(c.Similar))
		for _, e := range c.Similar {
			similar = append(similar, map[string]any{"label": e.Label, "category": e.Category, "source": e.Source, "count": int64(e.Count)})
		}
		candidates = append(candidates, map[string]any{"id": c.ID, "date": c.Date, "name": c.Name, "merchant_name": c.MerchantName,
			"net_cents": net, "plaid_category": c.PlaidCategory, "similar": similar,
			"suggested_category": c.SuggestedCategory, "suggestion_reason": c.SuggestionReason})
	}
	categories := make([]any, 0, len(report.Categories))
	for _, c := range report.Categories {
		categories = append(categories, c)
	}
	return map[string]any{"candidates": candidates, "remaining": int64(report.Remaining), "categories": categories}, nil
}

// reviewJSON is a test-friendly rendering of a read result.
func reviewJSON(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

// FinanceApplyReviewCapabilityID is the review workflow's one effect.
const FinanceApplyReviewCapabilityID CapabilityID = "finance.apply_review"

func financeReviewEffects(f Finance) []WorkflowEffectCapability {
	ids := workflow.ArrayType(workflow.StringType())
	maxIDs := int64(finance.ReviewMaxCandidates)
	ids.MaxItems = &maxIDs
	reason := boundedString(0, 200)
	line := workflow.ObjectType(
		workflow.Field{Name: "id", Type: workflow.StringType()},
		workflow.Field{Name: "date", Type: workflow.StringType()},
		workflow.Field{Name: "name", Type: workflow.StringType()},
		workflow.Field{Name: "merchant_name", Type: workflow.StringType()},
		workflow.Field{Name: "net_cents", Type: workflow.IntegerType()},
		workflow.Field{Name: "category", Type: workflow.StringType()},
		workflow.Field{Name: "confidence", Type: workflow.NumberType()},
		workflow.Field{Name: "reason", Type: reason},
		workflow.Field{Name: "rule", Type: workflow.BooleanType()},
	)
	lines := workflow.ArrayType(line)
	lines.MaxItems = &maxIDs
	change := workflow.ObjectType(
		workflow.Field{Name: "id", Type: workflow.StringType()},
		workflow.Field{Name: "category", Type: workflow.StringType()},
	)
	changes := workflow.ArrayType(change)
	changes.MaxItems = &maxIDs
	return []WorkflowEffectCapability{{
		ID: FinanceApplyReviewCapabilityID, ContractVersion: financeReviewContractVersion,
		Connections: []string{FinanceConnectionID}, Resources: []string{FinanceLedgerResource},
		Idempotent: true,
		Input: workflow.ObjectType(
			workflow.Field{Name: "lines", Type: lines},
			workflow.Field{Name: "answer", Type: workflow.ObjectType(
				workflow.Field{Name: "reject", Type: ids, Optional: true},
				workflow.Field{Name: "change", Type: changes, Optional: true},
				workflow.Field{Name: "no_rule", Type: ids, Optional: true},
			)},
			workflow.Field{Name: "run_key", Type: boundedString(1, 200)},
		),
		Output: workflow.ObjectType(
			workflow.Field{Name: "applied", Type: workflow.IntegerType()},
			workflow.Field{Name: "rules_minted", Type: workflow.IntegerType()},
			workflow.Field{Name: "rejected", Type: workflow.IntegerType()},
			workflow.Field{Name: "skipped", Type: workflow.IntegerType()},
			workflow.Field{Name: "replayed", Type: workflow.BooleanType()},
		),
		Perform:   f.performApplyReview,
		Reconcile: reconcileFinanceEffect(func() any { return &finance.ApplyReviewOutput{} }, func(req WorkflowEffectRequest) error { _, err := applyReviewRequest(req); return err }),
	}}
}

// applyReviewRequest refuses a request outside the ledger, without a key
// or outside the contract; each refusal proves nothing happened.
func applyReviewRequest(req WorkflowEffectRequest) (finance.ApplyReviewInput, error) {
	var in finance.ApplyReviewInput
	if req.Connection != FinanceConnectionID || req.Resource != FinanceLedgerResource {
		return in, EffectNotPerformed(fmt.Errorf("finance effects serve connection %s resource %s, not %s %s", FinanceConnectionID, FinanceLedgerResource, req.Connection, req.Resource))
	}
	if req.IdempotencyKey == "" {
		return in, EffectNotPerformed(finance.ErrEffectKeyRequired)
	}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return in, EffectNotPerformed(err)
	}
	return in, nil
}

func (f Finance) performApplyReview(ctx context.Context, req WorkflowEffectRequest) (any, error) {
	in, err := applyReviewRequest(req)
	if err != nil {
		return nil, err
	}
	db, err := finance.OpenDBContext(ctx)
	if err != nil {
		return nil, EffectNotPerformed(err)
	}
	defer db.Close()
	out, err := finance.ApplyReview(ctx, db, req.IdempotencyKey, in)
	if err != nil {
		return nil, err
	}
	return financeEffectValue(out)
}
