package plugins

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/workflow"
)

// Finance workflow effects (finance-daily, 2026-10-06): the first
// production WorkflowEffectProvider. sync_ledger pulls every linked bank's
// new transactions and categorize_ledger mints budget entries for posted
// transactions without one, both on the finance-local connection's ledger
// resource, so a reviewed, scheduled workflow can do unattended what the
// finance_sync and finance_categorize chat tools do on request. They are
// separate Capability IDs because one Capability is contributed through
// exactly one role, and the chat tools stay.
//
// Both are idempotent per key: the finance package records one receipt
// per idempotency key (effect_receipts) and a replay returns the recorded
// output without acting, so a retry or a reconciliation replay is safe.
// Reconcile answers from the same table: a receipt proves the effect
// happened with that output; no receipt proves nothing was recorded, and
// for these two effects that is "did not happen" (categorize records its
// receipt with its entries in one transaction; sync's committed pages are
// ones a re-sent intent continues past, see finance/effects.go). The
// default Resource Conflict Keys serialize every effect on the ledger, so
// two runs never sync concurrently and categorize never races a sync.

const (
	FinanceSyncLedgerCapabilityID       CapabilityID = "finance.sync_ledger"
	FinanceCategorizeLedgerCapabilityID CapabilityID = "finance.categorize_ledger"

	financeEffectContractVersion = "1.0.0"
	financeMaxBankWarnings       = 16
	financeMaxBanks              = 64
)

// WorkflowEffects are the ledger effects workflows may perform.
func (f Finance) WorkflowEffects() []WorkflowEffectCapability {
	served := func(e WorkflowEffectCapability) WorkflowEffectCapability {
		e.ContractVersion = financeEffectContractVersion
		e.Connections, e.Resources = []string{FinanceConnectionID}, []string{FinanceLedgerResource}
		e.Idempotent = true
		return e
	}
	warnings := workflow.ArrayType(workflow.StringType())
	maxWarnings := int64(financeMaxBankWarnings)
	warnings.MaxItems = &maxWarnings
	bank := workflow.ObjectType(
		workflow.Field{Name: "label", Type: workflow.StringType()},
		workflow.Field{Name: "added", Type: workflow.IntegerType()},
		workflow.Field{Name: "modified", Type: workflow.IntegerType()},
		workflow.Field{Name: "removed", Type: workflow.IntegerType()},
		workflow.Field{Name: "failed", Type: workflow.BooleanType()},
		workflow.Field{Name: "error", Type: workflow.StringType()},
		workflow.Field{Name: "warnings", Type: warnings},
	)
	banks := workflow.ArrayType(bank)
	maxBanks := int64(financeMaxBanks)
	banks.MaxItems = &maxBanks
	// day is the scheduled day the effect belongs to. It carries no
	// behavior: the runtime derives the idempotency key from the node's
	// input, so naming the day makes each day's intent its own key.
	day := workflow.ObjectType(workflow.Field{Name: "day", Type: boundedString(1, 10)})
	return []WorkflowEffectCapability{
		served(WorkflowEffectCapability{
			ID:    FinanceSyncLedgerCapabilityID,
			Input: day,
			Output: workflow.ObjectType(
				workflow.Field{Name: "banks", Type: banks},
				workflow.Field{Name: "added", Type: workflow.IntegerType()},
				workflow.Field{Name: "modified", Type: workflow.IntegerType()},
				workflow.Field{Name: "removed", Type: workflow.IntegerType()},
				workflow.Field{Name: "failed_banks", Type: workflow.IntegerType()},
				workflow.Field{Name: "replayed", Type: workflow.BooleanType()},
			),
			Perform:   f.performSyncLedger,
			Reconcile: reconcileFinanceEffect(func() any { return &finance.SyncLedgerOutput{} }),
		}),
		served(WorkflowEffectCapability{
			ID:    FinanceCategorizeLedgerCapabilityID,
			Input: day,
			Output: workflow.ObjectType(
				workflow.Field{Name: "categorized", Type: workflow.IntegerType()},
				workflow.Field{Name: "uncategorized", Type: workflow.IntegerType()},
				workflow.Field{Name: "replayed", Type: workflow.BooleanType()},
			),
			Perform:   f.performCategorizeLedger,
			Reconcile: reconcileFinanceEffect(func() any { return &finance.CategorizeLedgerOutput{} }),
		}),
	}
}

// financeEffectRequest refuses a request outside the ledger or without a
// key before anything is opened; both are proofs nothing happened.
func financeEffectRequest(req WorkflowEffectRequest) error {
	if req.Connection != FinanceConnectionID || req.Resource != FinanceLedgerResource {
		return EffectNotPerformed(fmt.Errorf("finance effects serve connection %s resource %s, not %s %s", FinanceConnectionID, FinanceLedgerResource, req.Connection, req.Resource))
	}
	if req.IdempotencyKey == "" {
		return EffectNotPerformed(finance.ErrEffectKeyRequired)
	}
	var in struct {
		Day string `json:"day"`
	}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return EffectNotPerformed(err)
	}
	return nil
}

// financeEffectValue renders an output as the JSON-shaped values the
// runtime validates against the contract.
func financeEffectValue(output any) (any, error) {
	data, err := json.Marshal(output)
	if err != nil {
		return nil, err
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func (f Finance) performSyncLedger(ctx context.Context, req WorkflowEffectRequest) (any, error) {
	if err := financeEffectRequest(req); err != nil {
		return nil, err
	}
	db, err := finance.OpenDBContext(ctx)
	if err != nil {
		return nil, EffectNotPerformed(err)
	}
	defer db.Close()
	out, err := finance.SyncLedger(ctx, db, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	return financeEffectValue(out)
}

func (f Finance) performCategorizeLedger(ctx context.Context, req WorkflowEffectRequest) (any, error) {
	if err := financeEffectRequest(req); err != nil {
		return nil, err
	}
	db, err := finance.OpenDBContext(ctx)
	if err != nil {
		return nil, EffectNotPerformed(err)
	}
	defer db.Close()
	out, err := finance.CategorizeLedger(ctx, db, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	return financeEffectValue(out)
}

// reconcileFinanceEffect answers from the effect receipt under the
// request's key. A lookup that cannot be made now is an error, and the
// runtime asks again later.
func reconcileFinanceEffect(newOutput func() any) WorkflowEffectReconcile {
	return func(ctx context.Context, req WorkflowEffectRequest) (WorkflowEffectLookup, error) {
		if err := financeEffectRequest(req); err != nil {
			return WorkflowEffectLookup{}, err
		}
		db, err := finance.OpenDBContext(ctx)
		if err != nil {
			return WorkflowEffectLookup{}, err
		}
		defer db.Close()
		output := newOutput()
		found, err := finance.LookupEffect(ctx, db, req.IdempotencyKey, output)
		if err != nil {
			return WorkflowEffectLookup{}, err
		}
		if !found {
			return WorkflowEffectLookup{Outcome: WorkflowEffectDidNotHappen, Evidence: "no effect receipt is recorded under the intent's idempotency key"}, nil
		}
		value, err := financeEffectValue(output)
		if err != nil {
			return WorkflowEffectLookup{}, err
		}
		return WorkflowEffectLookup{Outcome: WorkflowEffectHappened, Output: value, Evidence: "the effect receipt recorded under the intent's idempotency key"}, nil
	}
}
