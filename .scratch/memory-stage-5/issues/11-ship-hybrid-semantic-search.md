## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie finds relevant evidence even when the wording differs, alongside lexical and relationship matches.

## Acceptance criteria

- [ ] Integrate the configuration selected by the completed local experiment into the existing retrieval contract, tools, receipts, and source UI; do not add a separate search authority path.
- [ ] Create immutable embedding/index generations with source/revision/content hashes, durable coverage checkpoints, bounded backfill, and idempotent continuous refresh for eligible Claims and conversation evidence.
- [ ] Do not serve a generation before its required retained coverage is reconciled. Verify concurrent appends/accepted revisions, disabled-generation behavior, replacement, process restart, and rebuild.
- [ ] Filter every vector hit against authoritative SQLite scope/source/state and selected temporal/lifecycle intent, including retirement-corresponding excerpts. Stale vector matches cannot bypass deterministic eligibility.
- [ ] Fuse dense evidence with lexical, exact, temporal, and graph results under shared deadlines and focused context budgets, preserving distinct source authority and conflicting evidence.
- [ ] Demonstrate paraphrase recall through an actual turn or model-directed search, including failure/cancellation of the local embedding endpoint. Automatic Recall uses the same implementation when integrated; this ticket does not require Automatic Recall to have landed.
- [ ] Verify deterministic eligible result contracts with fixture embedding I/O, run the selected local model/index experiment against its declared expectations, and run required repository verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/164
- https://github.com/davidadel66/evie/issues/165
