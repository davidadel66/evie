# Finance review

A Workspace custom Workflow Definition (2026-10-07) that shrinks the
awaiting-review pile without a model ever classifying money on its own.

## What it does

1. **Gather** (read `finance.review_candidates`): up to `limit`
   unclassified posted transactions, recurring merchants first, each with
   deterministic evidence: the owner's rules and past transactions that
   share the merchant's first significant token, and for bank strings the
   first two tokens, with the category he gave them.
2. **Propose** (AI step, no tools): one category per candidate from the
   evidence and the vocabulary, with a confidence, a reason, and whether a
   rule should be minted. Typed data only.
3. **Gate** (`gate_proposals@1`): drops any proposal that names no
   candidate, uses a category not in the vocabulary, falls below
   `min_confidence`, repeats an id, or asks for a rule without a merchant
   name.
4. **Review** (interrupt): the run waits for the owner. The lines are in
   the question's input. `{}` accepts every line with its rule; `reject`,
   `change` and `no_rule` list the exceptions. If nothing survived the
   gate, the run ends without asking.
5. **Apply** (effect `finance.apply_review`): inserts one full-amount entry
   with source `agent` for each accepted line through the same rechecks
   categorize uses, mints the exact merchant rule unless declined (never
   overwriting an existing rule), and records every decision so a
   rejected row is not proposed again.

## Schedule

Mondays 09:30 America/Detroit, after the daily sync. Missed Mondays are
skipped, not caught up. It can also be started from chat.

## Answering

On the Finance Workspace page ("Waiting for you") or:

    evie workflows answer <run-id> review --answer '{}'
    evie workflows answer <run-id> review --answer '{"reject":["<id>"],"change":[{"id":"<id>","category":"Dining"}]}'
