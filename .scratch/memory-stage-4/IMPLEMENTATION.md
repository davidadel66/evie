# Memory Stage 4 implementation

Branch: `codex/memory-stage-4` · HEAD: `c8872ed37c9222e40170113561dc0b406361cb7c`

David authorized the schema fix and continued implementation of the remaining stories. Root orchestrates implementation owners, independent reviews, required checks, and one commit per ticket. No push, pull request, merge, or parent closure is included.

| Ticket | Status | Commit |
| --- | --- | --- |
| [#132: Freeze evidence, source-window, and closure rules](https://github.com/davidadel66/evie/issues/132) | completed | — |
| [#133: Freeze durable work, coverage, and generation transitions](https://github.com/davidadel66/evie/issues/133) | completed | — |
| [#134: Freeze owner review after source sessions close](https://github.com/davidadel66/evie/issues/134) | completed | — |
| [#135: Measure a local extractor on reviewed evaluation data](https://github.com/davidadel66/evie/issues/135) | engineering committed human quality pending | cadbe75 |
| [#136: Compile one selected source unit into durable candidates](https://github.com/davidadel66/evie/issues/136) | engineering committed model criterion pending | 24c1f90 |
| [#137: Recover unfinished compilation safely across processes](https://github.com/davidadel66/evie/issues/137) | completed | c8872ed |
| [#138: Activate background compilation for new evidence](https://github.com/davidadel66/evie/issues/138) | engineering review fixes in progress | — |
| [#139: Select historical backfill and inspect honest coverage](https://github.com/davidadel66/evie/issues/139) | blocked | — |
| [#140: Accept or reject a candidate after its conversation closes](https://github.com/davidadel66/evie/issues/140) | completed | 0138e39 |
| [#141: Review people, relationships, and new Predicate definitions](https://github.com/davidadel66/evie/issues/141) | engineering in progress | — |
| [#142: Review temporal changes, corrections, and additional support](https://github.com/davidadel66/evie/issues/142) | queued | — |
| [#143: Compile and review the initial contracted tool observation](https://github.com/davidadel66/evie/issues/143) | queued | — |
| [#144: Edit candidates and approve bounded dependent batches](https://github.com/davidadel66/evie/issues/144) | blocked | — |
| [#145: Review simple candidates in the web inbox](https://github.com/davidadel66/evie/issues/145) | engineering in progress | — |
| [#146: Review identities, edits, and compound effects on the web](https://github.com/davidadel66/evie/issues/146) | blocked | — |
| [#147: Change generations without losing review decisions](https://github.com/davidadel66/evie/issues/147) | blocked | — |
| [#148: Inspect compiler health, coverage, and review backlog](https://github.com/davidadel66/evie/issues/148) | blocked | — |
| [#149: Prove the complete Stage 4 path with deterministic acceptance](https://github.com/davidadel66/evie/issues/149) | blocked | — |
| [#150: Run the integrated pilot and freeze release gates](https://github.com/davidadel66/evie/issues/150) | blocked | — |
| [#151: Evaluate the frozen configuration and declare Stage 4 readiness](https://github.com/davidadel66/evie/issues/151) | blocked | — |

## Evaluation gates

The source/gold labels were explicitly approved. Actual model-output adjudications remain pending across six packets; continuing engineering does not approve those judgments. The schema fix completed 10/10 compact-v3 requests with no category/dangling-reference failures, but interpretation errors remain. All 110 comparisons are frozen. No adequate extractor or production configuration has been selected. Integrated human review, numerical release gates, and final untouched-holdout evaluation remain required.

The final holdout is uncreated and unexposed. No live model server or inference runner remains active. Original and corrected raw artifacts are retained verbatim; four exact raw-log paths have narrow Git attributes preserving their recorded whitespace.

## Current engineering evidence

#135 and #136 engineering are committed with evaluation criteria still open. #137 and #140 are complete: final independent Standards/Spec reviews, focused regression/race checks, and isolated full verification passed. #138 preliminary full verification passed, then independent review found an interval-ownership gap after pause/reactivation/resume and a masked shutdown cleanup error; the owner is fixing both. #141 identity/Predicate core and focused tests are implemented. #145 basic web review is active.

Each ticket checkpoint records exact files, hashes, frozen Git tree, verification logs, and review results. Isolated checks exclude unrelated user changes while using the normal repository verification script. The existing Vite chunk-size warning remains; no required check was skipped for committed engineering.

## Preserved work

All 220 pre-existing paths have an original hash record. The latest audit matches 218 exactly; only main.go and internal/web/serve.go contain authorized integration hooks. Exact original bytes are retained for the 218 originally existing files, and the two pre-existing deletions remain absent. Commits use an isolated index with exact ticket blobs, followed by an index-only fast-forward that does not overwrite the working tree.

Review entry points: `manifest.json`, `implementation-review-findings.json`, each numbered `engineering-checkpoint.json` and `checkpoint.json`, and the adjacent full verification logs.
