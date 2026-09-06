# Ticket #135 review and verification

Status: all90 comparison requests and final reviews complete; actual human output judgments pending. No commit window granted. Fixed baseline: `a41aa6189191ed1ca3cdaa53e496269db489085d`. Owned paths are the standalone spike command, versioned spike fixtures/reports, and spike research report. Unrelated working-tree changes are excluded. New files are reviewed with `git diff --no-index /dev/null <file>`; exit 1 is expected for additions.

## Standards

Initial review identified exact enum-membership and non-null source-coordinate gaps. Owner added public-boundary regressions and corrected them. Fresh Standards recheck found one ineffective regression: clock ancestry cases used an unbudgeted input and accepted any command failure, so the token-budget rejection could mask a removed validator. Recheck confirms the owner fixed this with budgeted N08-b, an ancestry-specific rejection assertion, and a valid dispatch control. New prompt/batch tests also assert specific failures and valid controls; no findings remain in that focused Standards recheck. No other substantive Standards/smell findings were reported for the inspected scoring and immutable-report code; the new general template bound and final report still need review.

## Spec

Initial review identified schema membership/null coordinates, incomplete local-clock ancestry, and a missing token/context fit boundary. Recheck confirmed these resolved for the exact frozen-request experiment.

Follow-up findings: preflight all selected requests before any inference so a later unknown budget cannot discard prior results; pin applied output-adjudication bytes and bind positional gold matches to the approved gold. Independent Spec recheck confirms both fixes. It also verified the new pinned-SPM B+2 full-template bound, identities, and arithmetic: current fixtures require at most 7,702 against 8,192 context tokens. No actionable code findings remain in this recheck. Reviewers ran no model inference or tests during the measurement phase.

Read-only recheck confirmed source/gold approval hashes, unchanged raw/request/latency evidence across original and corrected offline reports, and 20 attempts / 12 required-memory opportunities per first-comparison arm.

## Evaluation and human review

David approved the original 24-case source/gold packet. Human review of actual novel model outputs was requested asynchronously against frozen output packet SHA-256 `aedc9737dd0ee9c3cc081a9a009d8e11a2073da119bb609ca8486379f3d45b89`. No output approval has yet been received. Three proposed raw-meaning credits retain their explicit typed/encoding errors and cannot establish production adequacy.

The initial comparison has a prompt/schema-visibility confound. A second bounded comparison gives both arms identical full field definitions and schema text; its context-fit proof must be validated before dispatch. Originals remain immutable. Final holdout is uncreated and unexposed.

## Verification

Final original-run verification: `go test ./scripts/memory-extractor-spike -count=1` passed (4.624s); Python compile checks passed; `./scripts/verify-change.sh` passed (exit 0). Existing Icon Fast Refresh and large Vite chunk warnings remain. No required repository checks skipped. Independent Standards and Spec reviews of Qwen budget/adapter and stop-on-failure changes found no actionable findings.

Final report Spec review identified omitted completed corrected-Mistral resource results and an unsupported manual-closure coverage claim. Both were corrected. Counts, denominators, exact-match rates and latency quantiles agree with frozen artifacts. Human-output judgments remain incomplete, so no usable configuration or production selection is asserted.

The two pending packets' extra EOF blank lines were removed mechanically. `output-packet-formatting-revision.json` archives original packet and proposed-record bytes, old/new hashes, and assertions. Root independently proved each packet lost exactly one LF and each proposed record changed only its packet hash; judgments and pending status are unchanged. No new approval was inferred.

All 76 owned files passed separate new-file whitespace checks, all 33 local Markdown targets resolve, and packet/source/gold/manifest bindings passed. All 220 pre-existing paths outside task scratch match baseline hashes; the index is empty. The final original-code identity matches the full verification record. All 64 prior fixture artifacts are frozen in `pre-compact-artifact-baseline.json` before the new experiment.

## Next experiment review boundary

Fresh owner `/root/implement_135_compact_wire` is implementing the bounded proposal in `next-extractor-experiment.md`. This is independent tuning on already approved development sources/gold; it does not apply pending output labels or select a production extractor. Focused public-boundary tests and independent Standards/Spec review of the new transport, deterministic expansion, raw-denominator scoring and complete request budget are required before ten real inference requests. Preserve old artifacts and report the new configuration separately. Full verification must cover the final changed executable.

Compact experiment predispatch review: Standards identified explicit-empty-selector acceptance and a preflight regression that used preflight-only mode. Both are fixed with public regressions; Standards recheck passes. Spec identified expansion before response-window binding; wrong/missing/duplicate response identity now prevents any expansion in runner and scorer, and Spec recheck passes. Root also flagged unbounded sequence-gap iteration and failed-attempt scoring; both were corrected and covered.

Final compact-focused package tests passed (7.608s). Root ran `./scripts/verify-change.sh`: exit 0, package 8.187s, same existing Icon export/Fast Refresh and Vite chunk-size warnings, no required checks skipped. Code identities recorded in `compact-root-verification.json`. Root independently matched all ten prepared requests to exact approved source fields, chronological sequence, ownership/authority and rendered budgets; maximum 7171/8192. All 64 original fixture artifacts remain unchanged and all 220 pre-existing paths are preserved.

Root authorized exactly the frozen ten-request compact batch after verifying 24 manifest file bindings, current tested code, binary/request hashes, options and zero prior dispatch. Manifest SHA-256 `a7ff9a886c9976568bb0e409f986ebe10ab018e09670b0fb9821d1476cc82fd1`; executable `53c71887d0829bbff7869d10ca8d49491c24c4d528b44be3c10419dbd283e958`. No additional model/repair requests, code changes or production selection are authorized by this dispatch. Actual results and final novel-output review remain pending.

Final compact measurements:10/10 requests completed with request-specific finished responses,12raw/0retained,0automatic canonical matches across6required opportunities. The schema permits all observed dangling-start combinations even though the adapter rejects them; report clearly attributes this limitation to the tested schema and does not equate zero retention with universal meaning failure. Independent paired metrics and final Spec report review pass. All90 comparison attempts are recorded; no further run or production selection.

Root final audit checked112owned files for untracked whitespace,49local links,all64prior fixture hashes,all220pre-existing paths,actual10request/raw/seal hashes and unchanged tested code. All pass; index empty; all owned servers stopped. Compact output packet/proposed hashes are53b6e3609f03731f1f9f8c369fba62aa534a03f9fdddf95ceaddabd602576354 /7590dcfe454b4e64ba4236adad72440f9c2c6620cd24174ad1ebce89aec27bee, frozen before the third async output-review question. None of the four packets has human output approval yet. #135 remains open/uncommitted and #136blocked.
