# finance-review — spec

Decided 2026-10-07 with the owner; binding record in
[finance-review.decisions.md](finance-review.decisions.md).

## Purpose

Shrink the awaiting-review pile without a model ever classifying money on
its own. A workflow gathers the unclassified posted transactions with the
evidence a reviewer would want (matching rules, similar past transactions
and how they were categorized), has a model propose a category per
transaction as typed data, has deterministic code throw out anything that
breaks the rules, pauses for the owner's per-line answer, and applies the
accepted lines in one approved effect, minting an exact merchant rule with
each one so the same merchant is never decided twice.

Measured 2026-10-07: 100 posted transactions awaiting review, 90 with a
merchant name over 77 merchants, 10 without. Twelve merchants recur in the
pile; several are near-misses of known merchants.

## Boundaries (ADR 0010, categorize.decisions.md, categorize-patterns)

- The model proposes typed data and nothing else: no tools, no writes.
- Deterministic code gates every proposal before the owner sees it.
- Nothing is written until the owner answers, and only the effect writes.
- Matching stays exact: a minted rule is the merchant's Plaid spelling.
  No prefix, fuzzy or pattern rule is ever minted by this workflow.
- `Categorize` is unchanged. The workflow writes entries and rules through
  the same rechecks `Categorize` uses (posted, same amount, no entry yet).

## Read `finance.review_candidates` (contract 1.0.0)

Input `{limit?: integer 1..40}` (default 25). Output:

```
candidates[] (≤40): {
  id, date, name, merchant_name, net_cents, plaid_category,
  similar[] (≤8): {label, category, source, count}   -- evidence, see below
  suggested_category: string ("" when the evidence does not agree)
  suggestion_reason: string
}
remaining: integer      -- candidates not included this run
categories[]: string    -- the configured vocabulary
```

Candidates are posted transactions with no budget entry and no
`review_decisions` row, merchants that recur in the pile first, then newest
first. Evidence is deterministic, over merchants normalized (lowercase,
punctuation and Inc/LLC/Co/The stripped): (1) rules and (2) categorized
transactions whose merchant shares this merchant's first significant token
(≥4 characters), the near-miss shape Plaid produces ("Notion" for "Notion
Labs Inc."); (3) for rows
with no merchant, categorized or excluded transactions whose bank string
shares the first two tokens. `suggested_category` is set only when all
evidence agrees on one category and comes from a rule or at least two
rows. No bank credential, account id or tag is included.

## AI step `propose`

Model policy `openrouter` / `deepseek/deepseek-v4.1-flash`, temperature 0,
`max_output_tokens` 8000. Prompt (`prompts/propose.md`) instructs: keep a
`suggested_category` when present; otherwise choose from `categories` or
leave `category` empty when unsure; `rule` true only when the merchant name
is a real merchant likely to recur, never for a bank string; give a reason
in one sentence. Output schema `schemas/proposals.schema.json`:
`proposals[] {id, category, confidence 0..1, reason ≤200, rule, recurring}`.

## Deterministic gate `gate_proposals@1`

Pure reviewed function. Input: `candidates`, `proposals`, `categories`,
`min_confidence` (0..1). Output `{lines[] (≤40), dropped}` where a line is
`{id, date, name, merchant_name, net_cents, category, confidence, reason,
rule}`. A proposal is dropped when its id is not a candidate, its category
is not in the vocabulary (exact), its confidence is below the threshold,
it is a duplicate id (first kept), or it asks for a rule on a transaction
with no merchant name. Lines keep candidate order.

## Interrupt `review`

Message (fixed, ≤2,000 bytes): what the input shows and how to answer.
Input: `lines` from the gate, so the Workspace page and the CLI show every
proposal with the question. Response schema:

```
{ reject?: [id] (≤40), change?: [{id, category}] (≤40), no_rule?: [id] (≤40) }
```

An empty object accepts every line as proposed, with its rule. `reject`
drops a line and records the rejection so it is not proposed again;
`change` accepts a line with a different category (rule minted in that
category); `no_rule` accepts the entry but mints no rule.

## Effect `finance.apply_review` (contract 1.0.0, Finance 1.3.0)

Input `{lines, reject, change, no_rule}`; idempotent by key with an
`effect_receipts` row in the same transaction as the writes. For each line
not rejected: insert one full-amount `budget_entries` row with
`source='agent'` using the `Categorize` rechecks (posted, same amount, no
entry yet; a row that no longer qualifies is counted as `skipped`); when
the line has `rule` and is not in `no_rule`, insert the exact merchant rule
with `ON CONFLICT DO NOTHING` (an existing rule is never overwritten) and
the category into `categories` if new. Every line, accepted or rejected,
gets a `review_decisions` row `{transaction_id, decision, category, reason,
run_key, decided_at}`. Output `{applied, rules_minted, rejected, skipped}`.

## Definition `cmd/evie/workflows/finance-review/`

`gather` (read) → `propose` (ai) → `gate` (deterministic) → `review`
(interrupt) → `apply` (effect). Inputs `limit` (default 25) and
`min_confidence` (default 0.6). Run key `[day, at]` from the schedule or
`[started]` when run by hand; schedule Mondays 09:30 America/Detroit,
catch-up `skip` (a review can wait for the next Monday). Authority: the
read and the effect on `finance-local: ledger`, one call each. Retention
`sensitive`. Limits: 7 days (the interrupt may wait), `max_tokens` 60,000.
Fixtures simulate one batch of three proposals.

## Surfaces

- A run starts from the schedule or from chat with `workflow_start`.
- The pause shows on the Finance Workspace page ("Waiting for you") with
  the lines and an answer form, and in `evie workflows interruptions`. Chat
  reports it through `workflow_status` but cannot answer (runtime rule).
- `query_db`/`edit_db` descriptions name `review_decisions` and the
  `agent` source. The Spending UI labels an `agent` source "Agent
  (approved)" instead of "(legacy)".

## Non-goals

- No auto-apply at any confidence (owner decision Q1).
- No pattern or name-rule minting; no change to `Categorize`.
- No chat answer path; that is a runtime boundary, not this feature's.
- No learning from rejections beyond not re-proposing the rejected row.

## Verification

- Finance: candidate evidence and ordering, decisions exclude rows,
  apply with the `Categorize` rechecks, replay by key, no credential in
  outputs.
- Workflow: `gate_proposals@1` cases for each drop reason and ordering.
- Plugins: manifest 1.3.0 with the new read and effect, contracts, and an
  end-to-end perform/reconcile of `apply_review` on a seeded ledger.
- `evie workflows validate` and `simulate --fixtures` on the definition.
- `./scripts/verify-change.sh`, then `scripts/release.sh`.
- Demonstration: one real run, the proposal list, an answer rejecting one
  line and changing one, and the resulting entries, rules and decisions.
