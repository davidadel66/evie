# Ticket 143 frozen engineering handoff

Outcome: exact durable get_time empty-object observations now compile under the
explicit owner-clock-observations-v2 evidence policy, reach closed-session CLI
owner review, retain original tool authority through v4 acceptance, and remain
inspectable/replayable through explicit Promotion. No production dependency,
model configuration, tool implementation, staging, commit, branch, or push work
was performed.

The 24-file checkpoint is `143-engineering-checkpoint.json`; each entry carries
path, before_sha256, after_sha256, and bytes. Final file copies are under
`143-frozen-files`; all pre-existing owned originals are under
`143-owned/originals`. The compiler_work.go original incorporates #139 owner's
requested final NOT EXISTS gate refinement, keeping that delta out of #143.

Review entry points: compiler_tool_observation.go (strict source contract,
ancestry binding, per-transaction128-row cache); compiler_source.go (explicit
policy admission); compiler_validation.go (exact locator and co-citation);
candidate_review_prepare/encoding/history/provenance (v4 authority and historical
replay); candidate_clock_test.go (actual runtime tool-commit -> closed CLI journey).
The implementation explanation lives in
cmd/evie/docs/research/memory-stage-4-clock-observation.md.

Verification and exact timings are recorded in the checkpoint. Final frozen-byte
focused race: DB56.323s, CLI3.518s PASS. Normal clock boundary tests after final
refinements: DB4.203s PASS. Prior broad DB/CLI/memory regression: all PASS; unchanged
v1/v2/v3 golden encodings and new v4 golden PASS. Whitespace check PASS.
Root owns independent two-axis review and `./scripts/verify-change.sh` against
its isolated frozen tree, followed by the explicitly requested ticket commit.

Demonstration: run `go test ./cmd/evie -run
'^TestOwnerReviewClockCLIActualCommittedToolPath$' -count=1`. Its scripted provider
and clock use the real durable tool path, then exercise memory-review inbox,
prepare, resolve, operation, accepted authority checks and replay. Further SQLite
tests cover malformed/missing/duplicate outputs, exact approval/argument linkage,
incomplete prefixes, failure/interruption after success, forbidden projections,
source/control mutation, foreign session corruption, narrower scope visibility,
Promotion, current-policy redaction, and independent original replay.

Limits: the structural validator proves durable correlation, exact bytes and
owner co-citation. Whether the owner explicitly referred to the checked date and
the proposition follows remains extraction/owner-review quality judgment. Clock
text never establishes timezone, location or trusted UTC time. Those quality
panels remain pending separately from these deterministic engineering checks.

Standards P2 repair: every clock ancestry read preserves coded SQLite, context,
connection and driver error identity. Only a proven missing row becomes invalid
source. Historical QueryContext row-iteration and close errors are also preserved.
Injected failures cover all four ancestry lookup sites against real SQLite;
queued work retains zero attempts and no failure reason, and completes after the
fault is removed. Missing-row and mixed-error boundary checks also pass.

The repair changed only compiler_tool_observation.go and added
compiler_tool_observation_error_test.go. All other original frozen file bytes are
unchanged, and no #144 additions were copied into the snapshot. Final focused
normal tests: DB3.574s, CLI0.403s PASS. Final focused race: DB72.142s, CLI3.464s
PASS. All 24 snapshot hashes and byte lengths match the checkpoint. Root retains
full isolated verification and independent rereview ownership.
