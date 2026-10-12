package finance

import (
	"context"
	"database/sql"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

// finance-review candidates (finance-review.spec.md, 2026-10-07): the
// unclassified posted transactions a reviewer should look at, each with
// the deterministic evidence a reviewer would gather by hand. Evidence is
// computed here, in the read, so the workflow stays five steps and no
// reviewed function needs the whole ledger as input. The model that reads
// this proposes; it never writes. No bank credential, account id or tag
// leaves this read.

// ReviewEvidence is one piece of evidence: a rule or a group of
// categorized transactions that resemble the candidate.
type ReviewEvidence struct {
	Label    string `json:"label"`
	Category string `json:"category"`
	Source   string `json:"source"`
	Count    int    `json:"count"`
}

// ReviewCandidate is one transaction awaiting review with its evidence.
type ReviewCandidate struct {
	ID                string           `json:"id"`
	Date              string           `json:"date"`
	Name              string           `json:"name"`
	MerchantName      string           `json:"merchant_name"`
	NetCents          string           `json:"net_cents"`
	PlaidCategory     string           `json:"plaid_category"`
	Similar           []ReviewEvidence `json:"similar"`
	SuggestedCategory string           `json:"suggested_category"`
	SuggestionReason  string           `json:"suggestion_reason"`
}

// ReviewCandidatesReport is one batch of candidates.
type ReviewCandidatesReport struct {
	Candidates []ReviewCandidate `json:"candidates"`
	Remaining  int               `json:"remaining"`
	Categories []string          `json:"categories"`
}

// Bounds of a review batch.
const (
	ReviewMaxCandidates = 40
	ReviewMaxEvidence   = 8
	ReviewDefaultLimit  = 25
)

// Decisions the owner can record for a reviewed transaction.
const (
	ReviewAccepted = "accepted"
	ReviewRejected = "rejected"
	ReviewChanged  = "changed"
)

var merchantNoise = map[string]bool{"inc": true, "llc": true, "co": true, "corp": true, "ltd": true, "the": true, "of": true, "and": true}

// normalizeMerchant lowercases, strips punctuation and drops corporate
// noise words, so "Notion Labs Inc." and "notion labs" compare equal.
func normalizeMerchant(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	var kept []string
	for _, tok := range strings.Fields(b.String()) {
		if !merchantNoise[tok] {
			kept = append(kept, tok)
		}
	}
	return strings.Join(kept, " ")
}

// significantToken is the first token of at least four characters of a
// normalized merchant, the handle that "Kazoo Parking" and "Kazoo Parking
// Kiosks" share.
func significantToken(normalized string) string {
	for _, tok := range strings.Fields(normalized) {
		if len(tok) >= 4 {
			return tok
		}
	}
	return ""
}

// namePrefix is the first two tokens of a normalized bank string, the
// handle machine-generated descriptors repeat.
func namePrefix(normalized string) string {
	tokens := strings.Fields(normalized)
	if len(tokens) < 2 {
		return ""
	}
	return tokens[0] + " " + tokens[1]
}

type reviewKnown struct {
	label, category, source string
	count                   int
}

// ReviewCandidates returns up to limit candidates with evidence, recurring
// merchants first, then newest first, skipping transactions the owner has
// already decided in a review.
func ReviewCandidates(ctx context.Context, db *sql.DB, limit int) (ReviewCandidatesReport, error) {
	if limit <= 0 {
		limit = ReviewDefaultLimit
	}
	if limit > ReviewMaxCandidates {
		limit = ReviewMaxCandidates
	}
	report := ReviewCandidatesReport{Candidates: []ReviewCandidate{}, Categories: []string{}}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report, fmt.Errorf("begin review read: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT name FROM categories WHERE TRIM(name) != '' ORDER BY name COLLATE NOCASE, name`)
	if err != nil {
		return report, fmt.Errorf("load categories: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return report, err
		}
		report.Categories = append(report.Categories, name)
	}
	rows.Close()

	// Rules by the first significant token of their normalized merchant:
	// "Notion" is evidence for "Notion Labs Inc." and "Kazoo Parking
	// Kiosks" for "Kazoo Parking", the near-miss shape Plaid produces.
	rules := map[string][]reviewKnown{}
	rows, err = tx.QueryContext(ctx, `SELECT merchant, category FROM rules`)
	if err != nil {
		return report, fmt.Errorf("load rules: %w", err)
	}
	for rows.Next() {
		var merchant, category string
		if err := rows.Scan(&merchant, &category); err != nil {
			rows.Close()
			return report, err
		}
		if token := significantToken(normalizeMerchant(merchant)); token != "" {
			rules[token] = append(rules[token], reviewKnown{label: merchant, category: category, source: "rule", count: 1})
		}
	}
	rows.Close()

	// Categorized transactions grouped by handle: merchant token for rows
	// with a merchant, bank-string prefix for rows without.
	byToken := map[string]map[string]*reviewKnown{}
	byPrefix := map[string]map[string]*reviewKnown{}
	rows, err = tx.QueryContext(ctx, `
		SELECT COALESCE(t.merchant_name, ''), COALESCE(t.name, ''), e.category, e.source
		FROM budget_entries e JOIN transactions t ON t.transaction_id = e.transaction_id
		WHERE t.pending = 0`)
	if err != nil {
		return report, fmt.Errorf("load categorized: %w", err)
	}
	for rows.Next() {
		var merchant, name, category, source string
		if err := rows.Scan(&merchant, &name, &category, &source); err != nil {
			rows.Close()
			return report, err
		}
		add := func(index map[string]map[string]*reviewKnown, handle, label string) {
			if handle == "" {
				return
			}
			if index[handle] == nil {
				index[handle] = map[string]*reviewKnown{}
			}
			key := label + "\x00" + category + "\x00" + source
			if index[handle][key] == nil {
				index[handle][key] = &reviewKnown{label: label, category: category, source: source}
			}
			index[handle][key].count++
		}
		if merchant != "" {
			add(byToken, significantToken(normalizeMerchant(merchant)), merchant)
		} else {
			add(byPrefix, namePrefix(normalizeMerchant(name)), name)
		}
	}
	rows.Close()

	// Candidates: posted, no entry, no decision.
	type raw struct {
		id, date, name, merchant, plaid string
		cents                           int64
	}
	var all []raw
	rows, err = tx.QueryContext(ctx, `
		SELECT t.transaction_id, COALESCE(t.date, ''), COALESCE(t.name, ''), COALESCE(t.merchant_name, ''), COALESCE(t.plaid_category, ''), t.amount_cents
		FROM transactions t
		WHERE t.pending = 0
		  AND NOT EXISTS (SELECT 1 FROM budget_entries e WHERE e.transaction_id = t.transaction_id)
		  AND NOT EXISTS (SELECT 1 FROM review_decisions d WHERE d.transaction_id = t.transaction_id)
		ORDER BY t.date DESC, t.transaction_id`)
	if err != nil {
		return report, fmt.Errorf("load candidates: %w", err)
	}
	for rows.Next() {
		var r raw
		if err := rows.Scan(&r.id, &r.date, &r.name, &r.merchant, &r.plaid, &r.cents); err != nil {
			rows.Close()
			return report, err
		}
		all = append(all, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return report, err
	}
	rows.Close()

	recurrence := map[string]int{}
	for _, r := range all {
		if r.merchant != "" {
			recurrence[normalizeMerchant(r.merchant)]++
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		ri, rj := recurrence[normalizeMerchant(all[i].merchant)], recurrence[normalizeMerchant(all[j].merchant)]
		if ri != rj {
			return ri > rj
		}
		return false // keep the date-descending order from the query
	})
	if len(all) > limit {
		report.Remaining = len(all) - limit
		all = all[:limit]
	}
	for _, r := range all {
		c := ReviewCandidate{ID: r.id, Date: r.date, Name: r.name, MerchantName: r.merchant, PlaidCategory: r.plaid,
			NetCents: new(big.Int).Neg(big.NewInt(r.cents)).String(), Similar: []ReviewEvidence{}}
		var evidence []reviewKnown
		if r.merchant != "" {
			token := significantToken(normalizeMerchant(r.merchant))
			evidence = append(evidence, rules[token]...)
			for _, known := range byToken[token] {
				evidence = append(evidence, *known)
			}
		} else {
			for _, known := range byPrefix[namePrefix(normalizeMerchant(r.name))] {
				evidence = append(evidence, *known)
			}
		}
		sort.SliceStable(evidence, func(i, j int) bool {
			if evidence[i].count != evidence[j].count {
				return evidence[i].count > evidence[j].count
			}
			if evidence[i].label != evidence[j].label {
				return evidence[i].label < evidence[j].label
			}
			return evidence[i].category < evidence[j].category
		})
		if len(evidence) > ReviewMaxEvidence {
			evidence = evidence[:ReviewMaxEvidence]
		}
		categories := map[string]bool{}
		weight, ruled := 0, false
		for _, e := range evidence {
			c.Similar = append(c.Similar, ReviewEvidence{Label: e.label, Category: e.category, Source: e.source, Count: e.count})
			categories[e.category] = true
			weight += e.count
			ruled = ruled || e.source == "rule"
		}
		if len(categories) == 1 && (ruled || weight >= 2) {
			for category := range categories {
				c.SuggestedCategory = category
			}
			switch {
			case ruled:
				c.SuggestionReason = "an existing rule names this merchant"
			default:
				c.SuggestionReason = fmt.Sprintf("%d similar transactions were categorized the same way", weight)
			}
		}
		report.Candidates = append(report.Candidates, c)
	}
	return report, nil
}
