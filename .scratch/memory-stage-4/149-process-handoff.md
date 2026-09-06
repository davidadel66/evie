# Ticket 149 process conformance contribution

Only new `internal/eviedb/stage4_process_conformance_test.go` is owned. Frozen bytes are in `149-process-files/`; `149-process-manifest.json` contains the exact new-file hash, byte count, commands, environment, and five safe assertion receipts. No production/shared files, existing tests, Git index, branch, or commits were changed.

## Coverage

- Three actual-process worker scenarios: no-defer process death after dispatch; expired lease with delayed successful transport delivery; explicit cancellation/resume with delayed delivery. Each uses public `RunCompilerStep` and ordinary `OpenDBAt`, preserves attempts and exact dispatch reservation, blocks unknown and wrong-fence release, then verifies a request-specific scripted completion acknowledgement. Replacement stays gated while stale delivery is fenced; its capacity survives. Exactly one candidate group, consumed stage, coverage record, and trusted release receipt remain. Duplicate execution/reselection do not dispatch again. Candidate source authority and unchanged accepted graph are checked.
- Two acceptance crash boundaries: `os.Exit(86)` immediately before or after COMMIT via the existing transaction resolver. Two independent processes obtain current owner authority and simultaneously retry the same delivery. Both return the exact canonical result. A further distinct delivery returns the actual already-resolved receipt. Counts prove atomic claims, source links, audit, delivery, candidate resolution, and scope revision. Canonical replay succeeds.
- Child processes use the current Go test binary, bounded file handshakes, temporary SQLite files, content-free coordination records, and exact scripted extraction. There are no model/network/tool calls. Five `STAGE4_EVIDENCE` JSON receipts are emitted for the owner runner.

## Verification

`go test ./internal/eviedb -run '^TestStage4ProcessConformance' -count=1 -v` PASS (0.989s), log `149-process-normal.log`.

`go test -race ./internal/eviedb -run '^TestStage4ProcessConformance' -count=1 -v` PASS (33.133s), log `149-process-race.log`.

`git diff --no-index --check /dev/null internal/eviedb/stage4_process_conformance_test.go` had no whitespace output (exit 1 means new-file differences); `gofmt -l internal/eviedb/stage4_process_conformance_test.go` PASS with no output.

An initial normal run failed because the test expected `completed` instead of public `completed_candidates`; corrected fixture, no production defect. Initial evidence is retained separately. Root owns pending required combined-tree `./scripts/verify-change.sh`, independent Standards/Spec review, and the single ticket commit. This contribution does not claim the aggregate ticket is complete.

Best review entry points: `TestStage4ProcessConformanceWorkerRecovery`, `TestStage4ProcessConformanceReviewCommitRecovery`, and the guarded child/transaction crash seam. No manual setup is required: rerun focused commands or the aggregate owner runner.
