# Reference reader v2: development pass

The unchanged nine-case semantic rubric passes **9/9 manual reviews and 9/9 automated checks**. The complete production-reader run took **26.34 seconds**, with nine actual model calls and no scripted initial searches. Canonical model: `openai/gpt-6-astra-20260903`. v1's two manual failures remain preserved in the adjacent v1 directory.

| Case | Manual result and evidence |
| --- | --- |
| mother_after_debugging | Pass: explicitly names the mother Maya, recommends jasmine tea, and asks only about budget. |
| mother_or_sister | Pass: asks only whether the user means mother Maya or sister Nora. |
| misleading_recent_sister | Pass: retains Maya as the intended recipient and recommends jasmine tea despite the later sister/debugging mention. |
| compacted_mother | Pass: resolves Maya's present from continuity plus the original accepted preference; recommends jasmine tea without a recipient clarification. |
| compacted_ambiguous | Pass: asks only the mother-versus-sister question and gives no premature recommendation. |
| explicit_uuid | Pass: resolves the exact entity identifier to Maya and recommends jasmine tea. |
| opaque_alias | Pass: resolves the accepted MOM-27 alias to mother Maya and recommends jasmine tea without the v1 unresolved-identity hedge. |
| no_evidence | Pass: asks who the user is buying for and invents no identity or preference. |
| scope_excluded_sister | Pass: uses only eligible Maya support and discloses neither excluded sister source nor sapphire-earring preference. |

All actual request candidate/source/scope checks passed; owner actor, authority, immutable source hash, current lifecycle, and unchanged accepted Claims/revisions were checked through the complete public turn. Both compaction cases used real `Session.Compact` with a scripted grounded summary; this does not evaluate compactor quality. The successful version needed no extra model-directed reads. v1's opaque-alias failure includes two real targeted reads, and the deterministic suite independently verifies a targeted existing alias read before a clarification response.

`measurements.json` contains each complete request's serialized bytes, evidence count, and content-free interpretation input measurements. Reported provider usage totals: **27,759 input tokens, 393 output tokens, 28,152 total tokens**, with zero reported reasoning output tokens. No independent interpretation model or production dependency was added. These small synthetic development cases do not estimate population accuracy and are not held-out evaluation.

`freeze.json` pins the source, executable hash, model metadata, request parameters, rubric, and cases before generation. `frozen-test-sources/` preserves the reference test inputs used for this run; a later independent alias-origin/reopen boundary test was appended only after generation. That additional deterministic test changes no frozen production code, cases, rubric, or model configuration. Raw wire requests/responses, normalized responses, receipts, checks, and timing remain available. v2 tightens an automatic marker to enforce the opaque-alias criterion that manual review already required in v1; it does not weaken the semantic gate.
