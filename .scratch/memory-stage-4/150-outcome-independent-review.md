# Independent review: pilot outcome hardening

PASS: no unresolved actionable finding in the three-file delta against
`d12470f707ba5cc4ae3f4f45df4b3f2fc4361871`. Exact reviewed hashes are recorded in
`150-outcome-independent-review.json`. This was a static review; no live source
edits or duplicate test runs were performed.

The added checks now distinguish the expected failed/empty/successful obligations
from extra terminal failures. Success requires exact job, attempt, candidate and
dispatch counts; positive selected members; one attempt per unique job; exact
completed-member coverage; and the expected deliberately injected failure reason.
Actual selected history comes from the public selection receipt instead of a
manufactured event count. Discovery/materialization must settle before collection.

Outcome mismatches are checked after retaining job/foreground/candidate diagnostics,
storage and dispatch evidence. The command records the error with that report,
syncs it and exits unsuccessfully. The added regression retains real Kernel smoke
observations, injects the discovered zero-attempt failed-job shape and verifies
that it is rejected without dropping those observations.

These additions remain deterministic fixture checks. They do not invent numerical
release gates or change `scripted_infrastructure_only` / `release_eligible:false`.
They are compatible with #151's artifact and provenance requirements, but none of
this scripted evidence can populate an actual-model or owner-quality observation.

The production reconciliation change was outside this review and has its own
independent reviewer. The original matrix remains preserved with 111 unexpected
failures; fresh source-bound conformance and a measured rerun are still required.

The three reviewed blobs also match corrected snapshot tree
`413601a7c6088b59fb5dcd7ced53f46193590ffe` exactly (all hashes and byte sizes checked).
