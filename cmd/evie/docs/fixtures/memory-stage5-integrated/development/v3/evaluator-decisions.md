# Source sufficiency correction before development v3

This is a correction to the #167 evaluator, defined before any v3 reader
outputs. The retained v2 outputs and assessments exposed cases where valid
evidence was rejected solely because it lacked annotations attached to the
oracle's query. The v2 freezes, source maps, raw requests, answers, assessments,
and failure reports remain unchanged. Replaying those requests below tests the
auditor; it is not a fresh reader measurement or a release result.

The questions, sufficient support sets, accepted contextual records, semantic
rubric, numerical gates, and authority/scope/secret requirements do not change.
The source-audit API and output fields do not change. Meaning-based manual
assessment must still decide whether the answer used the supplied facts
correctly; proving source availability does not grade an answer as correct.

## Evidence establishes facts; query annotations select a view

- A currently active original conversation episode establishes its recorded
  wording when supplied through either current or historical retrieval. Its
  original speaker, authority, bytes, observation instant and lifecycle remain
  exact. Historical retrieval does not convert an episode into an accepted
  Claim or a current fact.
- An explicit Entity ID can be established by the accepted Claim's delivered
  structured subject when it exactly matches the canonical bound subject. This
  fallback is subject-only. A separately supplied valid identity marker may
  retain its existing subject/object behavior.
- An accepted alias can be established by a valid supplied identity marker, or
  by its fully delivered accepted origin. The latter must have the exact
  canonical Alias ID/entity association, source event, accepted operation and
  scope, and the complete original Source Link locator must contain the literal
  alias as a token. A raw episode, partial origin, near match, unrelated alias
  origin, or merely knowing an alias's string is insufficient. This fallback
  also targets the canonical Claim subject only.
- Every identity marker actually supplied is validated before considering
  alternate proof. A forged marker cannot be rescued by otherwise valid source
  text. Revoked alias-origin access also fails validation.
- A historical question's world-time target is proved by the accepted Claim's
  canonical effective validity interval. Both bounds must be known and exactly
  match the bound fact; the requested point must lie in the half-open interval
  `[from, to)`. The retrieval query's clock time need not equal the oracle's
  clock time. An unknown bound, incorrect interval, or target at the exclusive
  end cannot establish the requested point. Observation and read-pin timestamps
  cannot substitute for fact validity.
- Only explicitly declared acceptable context is exempt from the question's
  target-date or query-intent sufficiency requirement. It still needs exact
  canonical Claim, validity, source and lifecycle validation to support its own
  wording. It does not count toward the unchanged gold support numerator. A
  record appearing in both required support and acceptable context keeps the
  stricter required-target rule. Standalone seeds without gold/context metadata
  retain the strict default. This prevents a current contextual record with
  unknown validity from being misrepresented as proof of the historical target.

Retired evidence remains available only through explicit historical retrieval,
with exact status at the knowledge pin and current retired status. All source
access rules still apply. The accepted-source audit additionally checks the
canonical Source Link's eligibility: retaining an immutable public event cannot
restore a retracted link's accepted authority.

The existing full-source coverage requirement remains. Graph sufficiency still
requires both independently valid accepted records; supplying a raw episode in
place of the accepted bridge does not satisfy that support set. Invalid items
earn no support, even when their IDs identify known records. Source metadata,
UTF-8 locators, original bytes, SHA256 and RFC3339 instants (including
nanoseconds) remain exact.

These rules follow the Stage 5 specification's evidence identity, source
filtering, and time distinctions, the glossary's separation of Alias, Claim,
Conversation Excerpt and Valid Time, and ADRs 0052 and 0059. Query provenance is
retained; an oracle query annotation is not itself a necessary fact premise.

## Deterministic replay and adversarial controls

Run from the implementation worktree, supplying the preserved v2 locations:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/development/v3/audit_source_regressions.py \
  --freeze /private/tmp/evie-memory-stage5/integrated-development-v2/freeze.json \
  --evidence-report /tmp/evie-memory-stage5/integrated-development-v2-evidence/evidence-report.json
```

The runner verifies the freeze identity, canonical input manifest, loaded source
map, result manifest, and each actual wire-request hash before auditing the
delivered `EVIE_MEMORY_DATA`. Negative variants modify copies in memory only.
No models, embeddings, new reader answers, or held-out records are involved.

`audit-red-green-record.json` retains the observed failure and passing
transcripts. Four separate failing public-API replays preceded their fixes:
active episodes read historically; the accepted Lucia bridge without a query
marker; an exact structured Entity subject without a query marker; and a
historical fact interval with a different query clock time. The adversarial
suite additionally exposed and then closed the retracted canonical Source Link
false pass. Existing full v2 payloads and raw failures were not rewritten.

Controls cover missing or misbound accepted aliases, a near-match alias token,
forged supplied identity markers, wrong Claim operations/subjects, wrong source
links/authority/speaker/scope/observation instants, recomputed hashes over forged
text, clipped alias origins, absent structured subject proof, revoked canonical
Source Links, current-intent retired evidence, forged lifecycle labels, wrong
or unknown fact intervals, exclusive interval endpoints, and incomplete graph
support. Required-target/context label overlap and standalone strictness are
also covered. Test fixture IDs select retained records; the auditor has no
case-ID branches.

Final verification: the command above passed **17 tests in 0.403 seconds**,
including the entire retained 144-case/condition payload cohort. Every retained
payload remains valid, with zero source-audit violations and no previously
supported record lost. This does not change or rescore the frozen v2 report.
The context regression was first observed failing after the target-interval
correction, then fixed before the v3 freeze. Its RED and GREEN transcripts are
included alongside the final passing transcript.
