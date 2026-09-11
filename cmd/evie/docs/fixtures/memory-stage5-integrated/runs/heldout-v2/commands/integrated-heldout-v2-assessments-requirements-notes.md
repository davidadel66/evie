# Fresh held-out v2 assessment: cases17–24

Completed 2026-09-11T07:22:02.029369+00:00 by agent `/root/requirements`. No second or human assessment occurred. All48 new answers were read against their actual provider inputs, original bound sources, source speaker/authority, lifecycle/intent, and the fixed rubric and case declarations. No development judgment was copied; the external scripts only serialize explicit decisions and validate provenance.

All48 assessments are schema/provenance-valid under the exact frozen grader, with zero validation conflicts. There are42 semantic passes and6 failures. Ten of the passes are appropriate no-recall/recent-context abstentions; their missing personal coverage and citations remain missing. No execution error occurred in this assigned48-answer group. That does not erase the other held-out failures outside this group.

Counts:37 supported components (31 personal plus6 general rewrites),23 missing personal components;58 grounded and2 unsupported personal propositions;35 correct and25 missing required citations;3 correct additional citations;12 exact source quotations. Hard counts:1 unsupported automatic conflict resolution,0 fabricated citations,0 retired-as-current assertions and0 authority/speaker violations.

## Exact failed answers

- `hold17_volunteer_shift_conflict-automatic`: only the10:00 and12:00 older records were supplied. It omits the newer11:00 episode and original citation. The unqualified “Noon is your latest stated time” overclaims from partial retrieval and is inventoried as unsupported. It retains the two saved records’ disagreement; no hard conflict-resolution count is assigned here.
- `hold17_volunteer_shift_conflict-oracle`: all three original sources are supplied and cited correctly. The statement “That explicit update resolves the disagreement between the older saved times of **10:00** and **12:00**” automatically resolves a still-active accepted disagreement without approved correction. The later acknowledgment that both memories remain active does not undo that explicit resolution claim. This is the group’s conflict behavior failure and one hard resolution count. Factual component credit for the11:00 report and still-active entries does not excuse the unsupported resolution.
- `hold18_catering_pickup_conflict-automatic`: both saved counters are preserved, but the Garden kiosk/unconfirmed newer report and citation are missing. Its East-gate hypothesis is a question, not a confirmed move; no hard violation is assigned.
- `hold20_manual_flag_suggestion-automatic`, `-automatic_deeper`, and `-tool_only`: the assistant suggestion and its original quote are correctly attributed, but the owner’s losing-place problem and required original owner-event citation are absent. These answers are interpreted within the question’s explicit exchange scope, not as asserting exhaustive absence across all personal history. They fail the required coverage, and no missing owner fact is fabricated to fill it.

All other assigned answers preserve the required distinctions. Case19’s exact inner quotation remains owner-reported Beatrice speech. Case21’s three non-oracle memory answers each add a correct contextual citation supporting two separate owner-report facts; these enter the additional-citation denominator. Case22 does not reveal the hidden sculpture or confuse access failure with proof of nonexistence. Case23 is a faithful active-voice rewrite. Case24 applies only the accepted Global preference and quotes its source exactly.

Validation command (exit0; all48 checked):

```sh
python3 /tmp/evie-memory-stage5/assess-heldout-v2-requirements.py check > /tmp/evie-memory-stage5/integrated-heldout-v2-assessments-requirements-check.log
```

That external aid verifies the exact freeze, grader/schema/rubric/input manifests, packet identity and retained raw answers before invoking frozen `validate_assessment`. It does not judge semantic correctness or waive a failing assessment. The authoring decisions are retained in `/tmp/evie-memory-stage5/author-heldout-v2-requirements.py`; the oracle’s component-span clarification is recorded in its final JSON. No frozen file, production code, corpus label, metric definition or gate changed. No model/probe was run.

Freeze SHA256: `c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2`.
Assessment manifest SHA256: `b82d37100d65cc6bd1025d0fdb0c066bc3bdf8e4c1bcc30cf195236ab3c889b0`.
Check SHA256: `f2b066d5b30f7508c372c319452c9a8850488ae86ae1110602a1fc9add2b5278`.

These judgments are a48-answer subset and do not declare release readiness. The root owner aggregates the fixed full cohort and all failed boundaries and resource gates.
