package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/davidadel66/evie/internal/finance"
	"github.com/davidadel66/evie/internal/workflow"
)

// Finance workflow reads (Stage 14 sub-stage 5, 2026-10-04): two fixed,
// parameterized reads of the local finance database through the same
// read-only Spending queries the Spending view uses. No query text, table
// or column is ever taken from a definition, and no bank credential is
// read. No Account Connection registry exists yet, so the finance plugin
// names the one connection it serves: finance-local, the owner's local
// finance database (~/.finance/finance.db and the banks linked through it),
// whose one resource is its ledger. A Workspace grants it with
// `evie workspaces access <id> --set finance-local=ledger`, and a workflow
// requests it as Standing Authority like any connection.

const (
	FinanceMonthCashFlowCapabilityID     CapabilityID = "finance.month_cash_flow"
	FinanceMonthTransactionsCapabilityID CapabilityID = "finance.month_transactions"

	// FinanceConnectionID and FinanceLedgerResource are the connection and
	// resource the finance reads serve.
	FinanceConnectionID   = "finance-local"
	FinanceLedgerResource = "ledger"

	financeReadContractVersion = "1.0.0"
	financeMaxPageSize         = 100
	financeMaxOffset           = 1_000_000
)

func boundedString(minLength, maxLength int64) *workflow.Type {
	t := workflow.StringType()
	t.MinLength, t.MaxLength = &minLength, &maxLength
	return t
}

func boundedInteger(minimum, maximum float64) *workflow.Type {
	t := workflow.IntegerType()
	t.Minimum, t.Maximum = &minimum, &maximum
	return t
}

func enumString(values ...string) *workflow.Type {
	t := workflow.StringType()
	for _, v := range values {
		t.Enum = append(t.Enum, v)
	}
	return t
}

func financeMonth() *workflow.Type { return boundedString(7, 7) }

func cashFlowTotals(extra ...workflow.Field) *workflow.Type {
	return workflow.ObjectType(append([]workflow.Field{
		{Name: "inflow_cents", Type: workflow.IntegerType()},
		{Name: "outflow_cents", Type: workflow.IntegerType()},
		{Name: "net_cents", Type: workflow.IntegerType()},
		{Name: "transactions", Type: workflow.IntegerType()},
	}, extra...)...)
}

// WorkflowCapabilities are the finance reads workflows may call.
func (f Finance) WorkflowCapabilities() []WorkflowCapability {
	served := func(c WorkflowCapability) WorkflowCapability {
		c.ContractVersion = financeReadContractVersion
		c.Connections, c.Resources = []string{FinanceConnectionID}, []string{FinanceLedgerResource}
		return c
	}
	category := cashFlowTotals(
		workflow.Field{Name: "kind", Type: enumString("category", "unclassified", "unreconciled")},
		workflow.Field{Name: "category", Type: workflow.StringType()},
	)
	monthFlow := cashFlowTotals(
		workflow.Field{Name: "month", Type: financeMonth()},
		workflow.Field{Name: "pending_transactions", Type: workflow.IntegerType()},
		workflow.Field{Name: "categories", Type: workflow.ArrayType(category)},
	)
	entry := workflow.ObjectType(
		workflow.Field{Name: "category", Type: workflow.StringType()},
		workflow.Field{Name: "amount_cents", Type: workflow.IntegerType()},
		workflow.Field{Name: "source", Type: workflow.StringType()},
	)
	transaction := workflow.ObjectType(
		workflow.Field{Name: "id", Type: workflow.StringType()},
		workflow.Field{Name: "date", Type: workflow.StringType()},
		workflow.Field{Name: "name", Type: workflow.StringType()},
		workflow.Field{Name: "merchant_name", Type: workflow.StringType()},
		workflow.Field{Name: "net_cents", Type: workflow.IntegerType()},
		workflow.Field{Name: "classification", Type: enumString("classified", "pending", "unclassified")},
		workflow.Field{Name: "entries", Type: workflow.ArrayType(entry)},
	)
	page := workflow.ArrayType(transaction)
	maxItems := int64(financeMaxPageSize)
	page.MaxItems = &maxItems
	statuses := enumString("all", "classified", "pending", "unclassified")
	return []WorkflowCapability{
		served(WorkflowCapability{
			ID:     FinanceMonthCashFlowCapabilityID,
			Input:  workflow.ObjectType(workflow.Field{Name: "month", Type: financeMonth()}),
			Output: monthFlow,
			Read:   f.readMonthCashFlow,
		}),
		served(WorkflowCapability{
			ID: FinanceMonthTransactionsCapabilityID,
			Input: workflow.ObjectType(
				workflow.Field{Name: "month", Type: financeMonth()},
				workflow.Field{Name: "status", Type: statuses, Optional: true},
				workflow.Field{Name: "offset", Type: boundedInteger(0, financeMaxOffset), Optional: true},
				workflow.Field{Name: "page_size", Type: boundedInteger(1, financeMaxPageSize), Optional: true},
			),
			Output: workflow.ObjectType(
				workflow.Field{Name: "month", Type: financeMonth()},
				workflow.Field{Name: "status", Type: statuses},
				workflow.Field{Name: "offset", Type: workflow.IntegerType()},
				workflow.Field{Name: "page_size", Type: workflow.IntegerType()},
				workflow.Field{Name: "total", Type: workflow.IntegerType()},
				workflow.Field{Name: "has_more", Type: workflow.BooleanType()},
				workflow.Field{Name: "counts", Type: workflow.ObjectType(
					workflow.Field{Name: "all", Type: workflow.IntegerType()},
					workflow.Field{Name: "unclassified", Type: workflow.IntegerType()},
					workflow.Field{Name: "classified", Type: workflow.IntegerType()},
					workflow.Field{Name: "pending", Type: workflow.IntegerType()},
				)},
				workflow.Field{Name: "transactions", Type: page},
			),
			Read: f.readMonthTransactions,
		}),
	}
}

func (f Finance) spendingService() *finance.SpendingService {
	if f.spending == nil {
		return finance.NewSpendingService()
	}
	return f.spending
}

// financeLedger refuses a read of any connection or resource but the one
// the finance reads serve; the runtime has already admitted it.
func financeLedger(req WorkflowReadRequest) error {
	if req.Connection != FinanceConnectionID || req.Resource != FinanceLedgerResource {
		return fmt.Errorf("finance reads serve connection %s resource %s, not %s %s", FinanceConnectionID, FinanceLedgerResource, req.Connection, req.Resource)
	}
	return nil
}

// decodeFinanceInput decodes a read's input into its fixed parameters,
// refusing any field the contract does not declare.
func decodeFinanceInput(input map[string]any, into any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("finance read input: %w", err)
	}
	return nil
}

// financeMonthRange checks a YYYY-MM month and returns its last day.
func financeMonthRange(month string) (string, error) {
	start, err := time.Parse("2006-01", month)
	if err != nil || start.Format("2006-01") != month {
		return "", fmt.Errorf("month %q is not a YYYY-MM month", month)
	}
	return start.AddDate(0, 1, -1).Format("2006-01-02"), nil
}

// financeCents converts the Spending queries' exact decimal cents to an
// integer, refusing a sum outside int64 rather than rounding it.
func financeCents(text string) (int64, error) {
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("amount %s cents does not fit the read's integer contract", text)
	}
	return n, nil
}

func financeTotals(t finance.SpendingCashFlowTotals, into map[string]any) error {
	for _, field := range []struct {
		name  string
		value string
	}{{"inflow_cents", t.InflowCents}, {"outflow_cents", t.OutflowCents}, {"net_cents", t.NetCents}} {
		n, err := financeCents(field.value)
		if err != nil {
			return err
		}
		into[field.name] = n
	}
	into["transactions"] = int64(t.Transactions)
	return nil
}

// readMonthCashFlow is one month's posted cash flow as of its last day:
// totals, pending transactions left out, and the month by category (saved
// classifications that reconcile exactly, otherwise unclassified or
// unreconciled), as the Spending view reports it.
func (f Finance) readMonthCashFlow(ctx context.Context, req WorkflowReadRequest) (any, error) {
	if err := financeLedger(req); err != nil {
		return nil, err
	}
	var in struct {
		Month string `json:"month"`
	}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return nil, err
	}
	last, err := financeMonthRange(in.Month)
	if err != nil {
		return nil, err
	}
	report, err := f.spendingService().InspectSpendingCashFlow(ctx, finance.SpendingCashFlowQuery{Month: in.Month, HistoryEnd: in.Month, AsOfDate: last})
	if err != nil {
		return nil, err
	}
	out := map[string]any{"month": report.Month, "pending_transactions": int64(report.PendingTransactions)}
	if err := financeTotals(report.Selected, out); err != nil {
		return nil, err
	}
	categories := make([]any, 0, len(report.Categories))
	for _, c := range report.Categories {
		item := map[string]any{"kind": c.Kind, "category": ""}
		if c.Category != nil {
			item["category"] = *c.Category
		}
		if err := financeTotals(c.SpendingCashFlowTotals, item); err != nil {
			return nil, err
		}
		categories = append(categories, item)
	}
	out["categories"] = categories
	return out, nil
}

// readMonthTransactions is one bounded page of a month's transactions,
// newest first, with each one's saved classification entries and the
// month's classification counts.
func (f Finance) readMonthTransactions(ctx context.Context, req WorkflowReadRequest) (any, error) {
	if err := financeLedger(req); err != nil {
		return nil, err
	}
	in := struct {
		Month    string `json:"month"`
		Status   string `json:"status"`
		Offset   int    `json:"offset"`
		PageSize int    `json:"page_size"`
	}{Status: "all", PageSize: 50}
	if err := decodeFinanceInput(req.Input, &in); err != nil {
		return nil, err
	}
	if _, err := financeMonthRange(in.Month); err != nil {
		return nil, err
	}
	if in.PageSize < 1 || in.PageSize > financeMaxPageSize || in.Offset < 0 || in.Offset > financeMaxOffset {
		return nil, fmt.Errorf("page_size must be 1 to %d and offset 0 to %d", financeMaxPageSize, financeMaxOffset)
	}
	report, err := f.spendingService().InspectSpendingTransactions(ctx, finance.SpendingTransactionsQuery{Month: in.Month, Status: in.Status, Offset: in.Offset, PageSize: in.PageSize})
	if errors.Is(err, finance.ErrSpendingTransactionsQuery) {
		return nil, fmt.Errorf("status must be all, classified, pending or unclassified: %w", err)
	}
	if err != nil {
		return nil, err
	}
	transactions := make([]any, 0, len(report.Transactions))
	for _, t := range report.Transactions {
		net, err := financeCents(t.NetCents)
		if err != nil {
			return nil, err
		}
		entries := make([]any, 0, len(t.Entries))
		for _, e := range t.Entries {
			amount, err := financeCents(e.AmountCents)
			if err != nil {
				return nil, err
			}
			entries = append(entries, map[string]any{"category": e.Category, "amount_cents": amount, "source": e.Source})
		}
		transactions = append(transactions, map[string]any{"id": t.ID, "date": t.Date, "name": t.Name, "merchant_name": t.MerchantName,
			"net_cents": net, "classification": t.Classification, "entries": entries})
	}
	return map[string]any{
		"month": report.Month, "status": report.Status, "offset": int64(report.Offset), "page_size": int64(report.PageSize),
		"total": int64(report.Total), "has_more": report.HasMore,
		"counts": map[string]any{"all": int64(report.Counts.All), "unclassified": int64(report.Counts.Unclassified),
			"classified": int64(report.Counts.Classified), "pending": int64(report.Counts.Pending)},
		"transactions": transactions,
	}, nil
}
