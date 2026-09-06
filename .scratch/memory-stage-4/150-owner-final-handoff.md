# Ticket 150 engineering and infrastructure measurements

Engineering is complete and frozen for root's artifact review and one ticket commit. Actual model/owner pilot acceptance is incomplete: no adequate selected model, model-output adjudication, David active-review observations or numerical release gates have been invented. The final holdout remains uncreated, unexposed and unrun. No production database, ongoing activation, commit, push, PR or issue closure was performed by this owner.

Use `150-published-frozen-manifest.json` and `150-published-frozen-files/`: **20 files, 2,246,003 bytes**. Base is final #149 tree `ebb84af94f37b5bb5278d54208dc1d9f2dba73c7`. Delivery snapshot/tree are in `150-published-checkpoint.json`: `c8789a2dc1675be950abe106898fd27af6480481`. The measured code tree is `4b29071dc69018a9fedb2464315453baab0c9025`. Its thirteen files remain byte-identical. The delivery tree adds exactly seven report/artifact files; its final whole-tree fingerprint is not claimed as measured.

## What changed

The versioned pilot CLI/wrapper runs actual Agent.Send, terminal commitment, host finalization, Kernel compilation, public owner preview/resolution and one/two real compiler-host processes against disposable artificial workloads. It compares disabled/new/history modes with three rotating paired repetitions across eleven independently varied workload factors, retains raw resource/dispatch/job/foreground observations, enforces exact fixture outcomes and preserves missing/failed receipts. The active owner-review recorder uses explicit timing intervals and retained Kernel receipt references; it records no observations without actual operator input and preserves completed observations on signals.

The actual experiment exposed reconciliation defects. `compiler_reconciliation.go` now excludes later-root endpoints, handles sparse/activation-frontier boundaries, and reuses an immutable selection when discovery reaches an already captured member. Actual later members remain revisitable. A bounded automatic capture of a zero-root-member coordinate gap between existing live/historical ownership now records only an excluded selection with `no_root_members`. `compiler_work.go` returns before creating any job or coverage. Only the two automatic AwaitClosure reconcilers receive this treatment; direct explicit QueueCandidateUnit and other validation errors retain their outcomes. The reason is visible safely through `compiler_history_inspection.go`.

Public regressions cover sparse coordinates, pre-frontier roots, interleaved late events, immutable interval reuse, both live-first/history-first gap orderings, three real database reopen cycles, no job/coverage fabrication, historical lane/ownership preservation, no false B-event coverage, later genuine A-member progress, and unchanged direct explicit empty-selection failure.

## Results and evidence

Read `cmd/evie/docs/fixtures/memory-stage4-pilot/v1/infrastructure-results.md` and `infrastructure-observations.json`. Compressed archives retain original, interrupted and final raw/resource receipts and conformance/review evidence; no database or executable is included. `infrastructure-audit.json` is root's unchanged independent final matrix audit.

Final v3 has 99 passed trials; 1,584 actual foreground turns; 4,752 persisted foreground events; 1,584 unique jobs, attempts and dispatches; 1,518 completed-candidate jobs; 33 completed-empty jobs; exactly 33 injected failed historical jobs; zero unexpected outcomes. Selected events are 4,224; completed events 4,158; the difference is exactly the 66 deliberately failed historical members. There are 219 scripted preview/resolve operations. Every raw/resource hash matches and every disposable DB cleanup is true. No extraction intervals overlap and no owned pilot/browser process remains.

The original complete v1 is disqualified by 111 unexpected zero-attempt failed jobs across 54 trials. Partial v2 was stopped after a confirmed historical ownership defect: 20 raw receipts complete, 19 resource receipts complete, final resource sampling explicitly interrupted. Neither is relabeled as clean evidence. The earlier conformance failure caused solely by a mistyped browser receipt path is also preserved.

Maximum observed final terminal commit: 2.326ms. Maximum host finalization: 36.457ms. Maximum candidate freshness: 4.872s under the 250ms scripted service. Largest sampled Go process-tree RSS: 82.062MiB. Largest DB after a trial: 412.191MiB. Source arrival and finite completion/candidate rates are reported separately, not as sustained capacity promises. Resource receipts contain 7,158 nonempty CPU samples; ps percentages are described as estimates. Original runs were on battery; final run was AC-powered, 95% charging to 99% finishing charge. Caches, thermal state and OS work were uncontrolled.

Final matrix report SHA256: `5a7513e7129cf72a5a33d933ce47f9bd781aa842c0367c03d63a94bee6ff6a7b`.
Measured source inventory SHA256: `9287c9175d38658d959d72439e24897c0f67c90a73c8967c7079bdf61e9a5a38`.
Binary SHA256: `bd190f98c3648c9f0a2e7faa1098389428fc4258344a6a1b74e4532eeed55e43`.
Matching conformance SHA256: `4d1698e9af5c73cbfb518055156629c4b74429cd2b67f142b91021db1f78d8de`.
Conformance's full-source fingerprint: `7de332d0adc6796185009301581fb7f7949b12309de73eb0d25b228d6fa23610` (different inventory algorithm from the measurement subset).

## Verification

Final measured snapshot focused checks are in `150-auto-gap-focused-verification.json` and matching logs:

- `go test -count=1 ./scripts/memory-stage4-pilot`: PASS, 4.222s command wall time.
- `go test -race -count=1 ./scripts/memory-stage4-pilot`: PASS, 26.239s.
- `go vet ./scripts/memory-stage4-pilot`: PASS, 0.383s.
- `python3 scripts/memory-stage4-pilot/measure_test.py`: all six tests PASS, 0.090s. Its printed failed trial names are expected missing/malformed-report test cases.

`150-auto-gap-browser/browser.json`: actual Chrome/React/HTTP seven-scope browser conformance PASS. `150-auto-gap-conformance/report.json`: full source-bound PASS, no failures/skips. Integrated tests 4.303s, integrated race 53.314s, `./scripts/verify-change.sh` 51.935s, UI tests 1.264s. Existing UI tooling warnings remain in the logs. Root also passed final combined #150/#151 code verification on tree `b542031b0754b2c3ef19314f703c66d6e554bf6f`.

Both independent final review axes pass on measured tree4b290; see `150-spec-review-auto-gap-final.json/.md` and `150-standards-review.md`. Original red regressions and intermediate failure reviews are retained. A separate static publication review found no arithmetic/units/interpretation errors and requested stronger provenance guards; those guards were applied before publication. Publication asserts exact matrix-to-conformance hash/source binding, checkpoint tree file hashes, all thirteen unchanged current files, exactly three pair deltas, and null for wholly missing freshness.

`150-publication-verification.json`: four archive hashes/membership sets checked; all 198 final raw/resource hashes verified from inside the archive; all readable report links valid; thirteen measured files unchanged. Root's independent `150-root-matrix-audit.json` independently confirms all counts, IDs, membership, hashes and cleanup. `git diff --check ebb84af... c8789a2...`: PASS. The difference between measured4b290 and deliveryc8789 is exactly seven report-only paths. No additional Go/UI rerun is needed for those data/document additions.

## Remaining acceptance and demonstration

Scripted inference is infrastructure-only and cannot establish local model fitness, useful precision, required recall, owner review capacity, numerical gates or release readiness. Source/gold label approval remains separate from pending output adjudication. #150 and #151 experimental acceptance stays open pending actual resources/model/human prerequisites, as required by the tickets and preflight.

The README shows the frozen matrix command and interactive review-session commands. After an actual development configuration and candidates are available, use the explicit recorder alongside the real review surface and retain verified Kernel accept/edit/reject/defer receipts. Do not invent timing, silently promote these infrastructure receipts into final-model evidence, create a holdout, or enable ongoing compilation.

Root owns final report-delta reviews, one #150 commit and subsequent #151 engineering commit. Best review entry points are the readable report, machine-readable observations/audit, exact 20-file manifest, source boundary changes, then `run.go` and the public regression file.
