# Memory Stage 4 ticket breakdown

Status: breakdown approved and published as issues #132–#151. See [implementation progress](IMPLEMENTATION.md). Numbers below retain their draft identifiers.

Parent: [Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131). Published parent body matches the local Stage 4 specification; no parent comments or existing child implementation tickets were found when preparing these drafts.

The first three tickets freeze the parent specification's explicitly unresolved contracts. Ticket 04 is the standalone local-model experiment. These are verifiable prerequisite deliverables; later tickets implement narrow complete user paths. The initial production path is deliberately bounded to selected owner evidence. Recovery and continuous processing extend it separately, while owner review can proceed independently after candidate production.

Each implementation ticket owns its affected scope, provenance, authorization, atomicity, isolation and replay checks from the first change. Ticket 18 assembles the required end-to-end conformance demonstration; it is not a later hardening phase. The integrated pilot follows conformance, and untouched final-holdout evaluation follows frozen pilot gates.

These drafts neither choose the remaining open contracts nor invent numerical acceptance thresholds. A prerequisite is complete only when its deliverable is usable and binding; an inconclusive model spike or failing pilot keeps dependent work blocked.

## Proposed tickets

1. **[Freeze evidence, source-window, and closure rules](issues/01-evidence-and-closure-contract.md)**
   **Blocked by:** None (can start immediately)
   **Delivers:** A reviewed contract and worked examples define exactly which committed evidence the compiler may process and cite.

2. **[Freeze durable work, coverage, and generation transitions](issues/02-durable-work-contract.md)**
   **Blocked by:** 01
   **Delivers:** A state and transition contract makes scheduling, recovery, coverage gaps, and generation changes unambiguous.

3. **[Freeze owner review after source sessions close](issues/03-owner-review-contract.md)**
   **Blocked by:** 01
   **Delivers:** Exact approval, edit, rejection, and batch scenarios define how durable candidates become accepted knowledge.

4. **[Measure a local extractor on reviewed evaluation data](issues/04-local-extractor-spike.md)**
   **Blocked by:** 01
   **Delivers:** A repeatable local experiment selects a model/runtime/schema using reviewed data and a frozen evaluation protocol.

5. **[Compile one selected source unit into durable candidates](issues/05-bounded-candidate-compilation.md)**
   **Blocked by:** 02, 04
   **Delivers:** An explicitly selected owner assertion produces an inspectable, persisted candidate or an explicit successful empty result.

6. **[Recover unfinished compilation safely across processes](issues/06-worker-recovery-and-capacity.md)**
   **Blocked by:** 05
   **Delivers:** Restart, retry, cancellation, and competing processes preserve durable work without duplicate or stale outcomes.

7. **[Activate background compilation for new evidence](issues/07-new-evidence-activation.md)**
   **Blocked by:** 06
   **Delivers:** The owner activates a generation at an explicit frontier while conversations finish independently of extraction.

8. **[Select historical backfill and inspect honest coverage](issues/08-bounded-historical-backfill.md)**
   **Blocked by:** 07
   **Delivers:** The owner selects bounded historical ranges, sees gaps and outside-selection history, and keeps recent work progressing.

9. **[Accept or reject a candidate after its conversation closes](issues/09-closed-session-owner-review.md)**
   **Blocked by:** 03, 05
   **Delivers:** The owner reviews one exact simple candidate through the CLI and commits replayable knowledge or a durable rejection.

10. **[Review people, relationships, and new Predicate definitions](issues/10-identity-and-predicate-review.md)**
   **Blocked by:** 09
   **Delivers:** Candidates can propose relationships while exposing identity ambiguity and every necessary graph-definition effect.

11. **[Review temporal changes, corrections, and additional support](issues/11-temporal-change-and-support.md)**
   **Blocked by:** 09
   **Delivers:** The owner can review meaningful changes without losing uncertainty, prior history, or the distinction between errors and changed circumstances.

12. **[Compile and review the initial contracted tool observation](issues/12-contracted-tool-observations.md)**
   **Blocked by:** 09
   **Delivers:** A specifically allowed completed tool observation becomes a sourced candidate while retaining its original authority.

13. **[Edit candidates and approve bounded dependent batches](issues/13-edits-and-dependent-batches.md)**
   **Blocked by:** 10, 11, 12
   **Delivers:** The owner can correct a proposal or review an efficient batch while seeing exact dependencies and preserving its origin.

14. **[Review simple candidates in the web inbox](issues/14-web-inbox-basic-review.md)**
   **Blocked by:** 09
   **Delivers:** The web owner can inspect, accept or reject a simple candidate with the same scope and approval rules as the CLI.

15. **[Review identities, edits, and compound effects on the web](issues/15-web-advanced-review.md)**
   **Blocked by:** 13, 14
   **Delivers:** The web inbox supports the full reviewed candidate vocabulary, including ambiguity, edits and bounded batches.

16. **[Change generations without losing review decisions](issues/16-generation-upgrades-and-review-history.md)**
   **Blocked by:** 08, 13
   **Delivers:** A new extractor configuration can process explicitly selected evidence while accepted memory and prior review decisions remain intact.

17. **[Inspect compiler health, coverage, and review backlog](issues/17-compiler-health-and-review-diagnostics.md)**
   **Blocked by:** 15, 16
   **Delivers:** Safe CLI and web diagnostics show why work is waiting and provide the measurements needed for the real pilot.

18. **[Prove the complete Stage 4 path with deterministic acceptance](issues/18-deterministic-stage4-acceptance.md)**
   **Blocked by:** 15, 16
   **Delivers:** A reproducible acceptance run proves foreground independence, restart recovery, closed-session review and replay across surfaces.

19. **[Run the integrated pilot and freeze release gates](issues/19-integrated-pilot-and-gates.md)**
   **Blocked by:** 17, 18
   **Delivers:** Measured foreground cost, extraction quality, review effort and scaling establish numerical gates before final evaluation.

20. **[Evaluate the frozen configuration and declare Stage 4 readiness](issues/20-final-holdout-and-release-evaluation.md)**
   **Blocked by:** 19
   **Delivers:** A final versioned report states whether the frozen Stage 4 configuration passes every agreed gate for ongoing use.

## Parent story coverage

Story numbers refer to the 80 user stories in the parent specification. Mapping denotes explicit implementation or evaluation responsibility; shared invariants still apply to every affected slice.

| Draft | Primary and explicit coverage |
| --- | --- |
| 01 | 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 80 |
| 02 | 39, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 69, 80 |
| 03 | 22, 23, 26, 28, 32, 33, 35, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 80 |
| 04 | 6, 10, 24, 29, 30, 31, 36, 38, 71, 72, 75, 77, 78, 79, 80 |
| 05 | 1, 2, 6, 7, 9, 10, 11, 13, 15, 16, 19, 20, 21, 23, 36, 37, 38, 42, 47, 50, 55, 60, 69, 70, 76 |
| 06 | 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 69, 76 |
| 07 | 1, 17, 18, 37, 39, 43, 46, 47, 49, 52, 54, 55, 76 |
| 08 | 42, 43, 48, 49, 50, 51, 53, 54, 69, 76 |
| 09 | 9, 20, 21, 22, 23, 35, 58, 59, 60, 61, 63, 65, 66, 67, 68, 70, 76 |
| 10 | 3, 4, 24, 25, 26, 27, 28, 33, 34, 35, 60, 67, 76 |
| 11 | 5, 29, 30, 31, 32, 33, 34, 35, 60, 67, 76 |
| 12 | 8, 9, 12, 15, 17, 18, 19, 20, 21, 22, 60, 66, 67, 76 |
| 13 | 26, 28, 32, 33, 58, 60, 62, 63, 64, 65, 66, 67, 76 |
| 14 | 58, 59, 60, 61, 63, 65, 66, 68, 70, 76 |
| 15 | 24, 26, 28, 29, 30, 32, 33, 60, 62, 64, 65, 66, 68, 76 |
| 16 | 47, 52, 53, 54, 55, 56, 57, 63, 67, 69, 76 |
| 17 | 40, 41, 42, 43, 49, 51, 55, 58, 69, 70, 73, 74, 78, 79 |
| 18 | 17, 20, 21, 22, 23, 39, 41, 44, 45, 46, 47, 48, 49, 50, 59, 60, 61, 62, 63, 64, 65, 66, 67, 68, 70, 76, 79 |
| 19 | 40, 41, 42, 43, 71, 72, 73, 74, 75, 77, 78, 79, 80 |
| 20 | 40, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80 |

## Publication procedure

Publication completed in dependency order with the ready-for-agent label, real blocker links, and verified native GitHub blocking dependencies. Each child references the parent; the parent remains unchanged. Original draft bodies are retained for comparison, with actual published bodies stored separately.

The current startable frontier is draft 01. After it is done, drafts 02, 03 and 04 can proceed independently. After draft 05, the recovery/activation/backfill path and the owner-review path can proceed in parallel. The full direct-dependency graph is recorded in the machine-readable manifest.

All draft issue bodies follow the requested tracker template and avoid implementation file paths. All 20 approved issues are published; no parent modification or closure occurred.

