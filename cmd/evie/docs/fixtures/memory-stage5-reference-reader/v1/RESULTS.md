# Reference reader v1: retained development failures

Frozen before generation; production Default reader resolved to `openai/gpt-6-astra-20260903`. The complete run finished in 32.42 seconds. All nine first-request candidate/source/scope contracts passed. Automated answer markers passed 8/9, but manual review passed only 7/9; the latter is the acceptance result.

| Case | Manual result | Reason |
| --- | --- | --- |
| mother_after_debugging | Pass | Correct Maya/jasmine-tea recommendation; the follow-up asks budget, not recipient identity. |
| mother_or_sister | Pass | One focused mother/Maya versus sister/Nora question and no gift suggestion. |
| misleading_recent_sister | Pass | Explicitly preserves the mother/Maya recipient and jasmine tea. |
| compacted_mother | Pass | Correct Maya/jasmine-tea recommendation attributed to the original owner statement. |
| compacted_ambiguous | **Fail** | Asks the correct recipient question, then prematurely recommends both recipients' gifts before clarification. The frozen automatic marker also fails. |
| explicit_uuid | Pass | Resolves supplied entity UUID to Maya's supported jasmine-tea preference. |
| opaque_alias | **Fail** | Performs two real model-directed targeted reads, then says it cannot confirm what MOM-27 refers to and answers only conditionally. Existing evidence says `exact_or_alias` but does not expose the accepted alias-to-entity mapping. Automatic gift markers missed this failure; manual rubric rejects it. |
| no_evidence | Pass | Asks neutrally who the present is for and the occasion; invents no identity or preference. |
| scope_excluded_sister | Pass | Correct eligible Maya preference; neither restricted sister source nor sapphire-earring preference is exposed. |

There were eleven real model calls: one for each case except opaque_alias, which used three. No initial search was scripted. Original raw wire/composed requests, raw provider responses, normalized results, usage/timing, saved receipts, and all checks remain in this directory. No case is relabeled as successful. This is development evidence, not held-out evaluation or a population accuracy estimate.

The next announced change adds conditional reference-reading guidance to defer recipient-specific recommendations until a materially ambiguous referent is clarified. The opaque-alias failure additionally requires a source-backed, currently eligible alias mapping; a textual lookup reason alone does not establish identity for the reader. Both changes require a separate frozen rerun under the unchanged semantic rubric.
