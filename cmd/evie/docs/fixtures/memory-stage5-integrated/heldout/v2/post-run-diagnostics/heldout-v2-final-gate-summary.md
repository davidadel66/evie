# Held-out V2 final gate summary

Frozen report: **not release ready**. The closed resource execution exited 1: **1,051 pass, 23 fail, 3 incomplete** across 1,077 gates. `complete`, `all_required_gates_pass`, and `release_ready` are all false. No input, answer, scorer, gate or failed attempt was changed.

Freeze SHA256: `c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2`. [Exact ledger](/tmp/evie-memory-stage5/heldout-v2-final-gate-ledger.json) retains every nonpassing gate, its actual value, threshold, unit and full details, plus source hashes and closed execution records. [Frozen resource report](/tmp/evie-memory-stage5/integrated-heldout-v2-resource/report.json) · [Quality report](/tmp/evie-memory-stage5/integrated-heldout-v2-quality/quality-report.json) · [Evidence report](/tmp/evie-memory-stage5/integrated-heldout-v2-evidence/evidence-report.json).

## Validated quality

All conditions retain 24 planned assessments, 36 personal answer components and 28 required original-source citations. Additional citations add to the citation denominator. Baseline semantic passes can represent honest abstention or clarification; they do not imply retrieved-answer coverage.

| Condition | Valid /24 | Semantic pass /24 | Components | Required citations | Additional citations | All-citation accuracy | Grounding |
|---|---:|---:|---:|---:|---:|---:|---:|
| no_recall | 24 | 24 | 0/36 (0.00%) | 0/28 | 0/0 | 0/28 (0.00%) | null |
| recent_context | 24 | 24 | 2/36 (5.56%) | 0/28 | 0/0 | 0/28 (0.00%) | 2/2 (100.00%) |
| tool_only | 24 | 20 | 29/36 (80.56%) | 24/28 | 1/1 | 25/29 (86.21%) | 43/43 (100.00%) |
| automatic | 23 | 18 | 27/36 (75.00%) | 22/28 | 2/2 | 24/30 (80.00%) | null |
| automatic_deeper | 23 | 20 | 30/36 (83.33%) | 24/28 | 1/1 | 25/29 (86.21%) | null |
| oracle | 24 | 20 | 32/36 (88.89%) | 25/28 | 0/0 | 25/28 (89.29%) | 43/44 (97.73%) |

**Manual completeness and validation differ.** All 144 agent-authored assessments are present and marked review complete; 142 validate. Raw judgments contain 128 semantic passes and 16 failures. Frozen validation records 126 passes and 18 nonpasses, including two invalid hold12 rows. No human review is claimed.

The two `hold12_screen_free_errands` assessments (`automatic`, `automatic_deeper`) each have seven errors: unsupported record/event proof for component 0, unsupported record/event proof for propositions 0 and 1, and citation lacking independently supported original-event proof. The owner’s paper-checklist wording reached the reader as an eligible ConversationExcerpt; the frozen accepted-record binding did not gain accepted-Claim support. Those truthful judgments remain intact, the 14 errors remain failures, and no accepted-memory retrieval credit is invented. Grounding is null for both affected conditions despite observed valid-subset counts 47/48 and 45/45. `no_recall` grounding is null because it asserts no personal propositions.

Raw judgments, separately: 122/216 personal components; 184 grounded and 2 unsupported propositions; 97/168 required citations plus 4 correct additional citations; 13 purported source quotations inventoried. These counts are not substituted into validated metrics.

## Every failed or incomplete gate

The table includes aggregate checks and repeated checks on the same incident. Exact strings and details are in the ledger. There are 26 nonpassing gate records, not 26 independent defects.

| Gate ID | Status | Actual | Required |
|---|---|---:|---:|
| `reader:selected_test` | fail | false | == true |
| `reader:exit_code` | fail | 1 | == 0 |
| `evidence:all_boundaries_and_denominators` | fail | 20 | == 0 |
| `quality:all_assessments_valid` | fail | 14 | == 0 |
| `quality:all_quality_gates_pass` | fail | false | == true |
| `quality:required_denominators` | fail | 2 | == 0 |
| `quality:None:all_planned_assessments_valid` | fail | 142 | == 144 |
| `quality:None:evidence_audit_violations` | fail | 10 | == 0 |
| `quality:automatic_deeper:supported_answer_component_recall` | fail | 0.8333333333333334 | >= 0.85 |
| `quality:automatic_deeper:personal_proposition_grounding` | incomplete | null | >= 0.95 |
| `quality:automatic_deeper:original_event_citation_accuracy` | fail | 0.8620689655172413 | >= 0.95 |
| `quality:automatic_deeper:role_passes:critical_semantic` | fail | 13 | == 15 |
| `quality:automatic_deeper:family_semantic_pass:graph_and_dense_paraphrase` | fail | 0.0 | >= 0.5 |
| `quality:oracle:supported_answer_component_recall` | fail | 0.8888888888888888 | >= 0.95 |
| `quality:oracle:original_event_citation_accuracy` | fail | 0.8928571428571429 | >= 0.95 |
| `quality:oracle:role_passes:critical_semantic` | fail | 12 | == 15 |
| `quality:None:silent_conflict_resolutions` | fail | 1 | <= 0 |
| `quality:hard_zero:silent_conflict_resolutions` | fail | 1 | <= 0 |
| `local:selected_test` | fail | false | == true |
| `local:exit_code` | fail | 1 | == 0 |
| `local:hold15_archive_historical-oracle:boundaries` | fail | 60 | == 0 |
| `local:hold15_archive_historical-oracle:runtime_accounting_samples` | incomplete | null | == 20 |
| `local:hold15_archive_historical-oracle:work_p95` | incomplete | null | <= 3000000000 |
| `index:selected_test` | fail | false | == true |
| `index:exit_code` | fail | 1 | == 0 |
| `index:hold15_archive_historical:boundaries` | fail | 14 | == 0 |

The quality report has 45 passing and 11 nonpassing gates. Production (`automatic_deeper`) misses component recall 85%, citation accuracy 95%, required critical cases 13/15 and the graph/dense family floor 0.5; grounding cannot be established. Oracle misses component/citation 95% and critical cases 12/15. The single hard violation is hold17/oracle: a newer owner update is claimed to resolve an accepted conflict whose two entries remain active. Fabricated citations, retired-as-current assertions and authority/speaker violations are each 0; the conflict count 1 is checked twice but remains one violation.

Concrete semantic failures retain their original notes in the ledger: missing required citations in hold03/oracle and hold11/deeper+oracle; missed preference/context in hold07/tool_only and hold12/tool_only; missing later choice in hold13/automatic; absent historical answer in hold15’s four memory-enabled conditions; missing newer reports in hold17/automatic and hold18/automatic; the hold17/oracle resolution error; and missing original owner context/citation in hold20/tool_only, automatic and automatic_deeper. Two additional hold12 nonpasses arise from validation, not rewritten manual judgments.

## Closed runtime, provenance and recovery results

- Reader: 144 planned/recorded turns, 187 prepared requests, 184 actual model calls and 184 successful reader HTTP responses. Three hold15 requests were blocked before HTTP (tool_only dispatch 2, automatic_deeper dispatch 2, oracle dispatch 1); all three turns failed. Two extra HTTP metadata files are excluded from reader-call counts. All 10 source-audit violations belong to those hold15 turns; the resource check surfaces these twice as 20 entries.
- Local: 2,880/2,880 planned samples retained; 20 failed, all hold15/oracle. Of 144 cohorts, 143 have complete final runtime accounting; 20 sample totals in the remaining cohort are unknown, so its runtime quantile stays null. Its 60 boundary entries are three checks per failed turn. First-dispatch/whole-turn timing includes failed attempts; no successful-only replacement is used.
- Index: 24 cases retained; 23 satisfy boundary/recovery checks. Hold15 has 14 reported checks from its failed baseline, restarted and rebuilt public turns. Its locked historical gold requires a retired location whose exact text is also unconditionally forbidden; the frozen check rejects marked historical evidence. This diagnosed annotation/audit conflict does not waive any gate.
- Operating: 60/60 samples retained and pass (20 each cancellation, caller deadline and lease replacement). Maximum tails are 0.816583 ms, 12.749584 ms and 0.482583 ms respectively, below the unchanged 1,000 ms gate. Deterministic attestation gates pass; they do not substitute for failed held-out gates.

Index timing, derived-storage and Go-worker RSS gates pass. Out-of-process endpoint RSS remains a separate observed process-family sample; endpoint inference call/input-byte counters are unavailable. Resource repetitions are not additional answer-quality trials. Any future evaluator or production correction requires a separate fresh assessment; this held-out result stays failed.
