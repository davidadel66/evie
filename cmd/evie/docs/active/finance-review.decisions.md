# finance-review — decisions

Spec: [finance-review.spec.md](finance-review.spec.md). Decided 2026-10-07
with the owner, in a four-question round.

- **Nothing writes without the owner's answer** (Q1, option a). The
  August decisions say a model never categorizes money on its own; a
  weekly batch answer costs a minute. Rejected: auto-applying the
  deterministic matches, and auto-applying above a confidence threshold.
- **Every accepted line mints its exact merchant rule** (Q2), unless the
  owner lists it under `no_rule`; the model marks lines unlikely to recur
  so he can decline those. Rules are Plaid spellings, never patterns.
- **Entries the owner approves from a batch carry `source='agent'`**
  (Q3), so `human` keeps meaning he typed it in the day dialog. The
  approval itself is on the run record. The reserved value from
  categorize.spec.md is used for the first time; `budget_entries.source`
  has no CHECK constraint, so no migration.
- **A Workspace custom workflow in Finance** (Q4), not a chat skill over
  `edit_db`: durable batches, a run record per proposal, one approval of
  the effect instead of a prompt per row.
- **Evidence is computed in the read, not in a reviewed function.** The
  similarity rules are deterministic and need the database; putting them
  in `finance.review_candidates` keeps the definition to five steps and
  avoids a research-style function that would need the whole ledger as
  input.
- **The answer is "accept all unless listed"**: an empty object accepts
  the batch, and `reject`, `change` and `no_rule` are exceptions. The web
  answer form is a JSON textarea, so the common case must be the shortest
  possible answer.
- **Rejections are remembered** in `review_decisions` so a rejected row
  is not proposed every week; the owner handles it in chat. Rejected:
  re-proposing with the rejection as context (a feedback loop the
  August decisions did not ask for).
- **Model**: `deepseek/deepseek-v4.1-flash` at temperature 0, the model
  deep-research already pins; categorization with evidence in hand is an
  easy task and the owner can change the policy in the definition.
- **Weekly, Mondays 09:30 America/Detroit, catch-up skip**, after the daily
  run has posted the week's transactions; also startable from chat.
