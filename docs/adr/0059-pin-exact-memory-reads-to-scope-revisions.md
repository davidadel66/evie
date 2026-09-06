# Pin exact memory reads to Scope Revisions

Stage 3 exposes exact paginated listing, inspection, Claim queries, provenance,
history, and deterministic traversal rather than relevance search. Every result
echoes its effective Valid and Transaction Times, and pagination remains pinned
to the initial Scope Revision instead of mixing concurrent states. One- and
two-hop traversal reapplies scope, lifecycle, temporal, and source-eligibility
rules at every hop. Local and model-facing callers share these semantics, while
the model-facing adapter additionally enforces remote-memory opt-in, bounds,
secret scanning, source-scope redaction, and untrusted-data rendering.
