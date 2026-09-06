# Ticket 144 CLI conformance contribution

Only new `cmd/evie/candidate_batch_test.go` is owned. No production files, staging, commits, branches, pushes, or user bytes were changed. The source path was originally absent (recorded in `144-cli-originals/absence.json`). The final source is frozen in `144-cli-frozen-files/`; import `144-cli-frozen-manifest.json` into the ticket144 manifest.

The test uses real SQLite and the public `memory-review` dispatcher. It seeds accepted Claims, compiles scripted identity/temporal candidates, runs the actual agent/runtime get_time path, compiles its contracted clock candidate, and closes the source session. CLI edits change Maya to Maya Chen, coffee to espresso, and the clock-supported literal while keeping unknown Valid Time. Identity and correction choices are explicit after edits. Exact edit revision before/after, original extraction/source order, generation/job/destination, authority, and competing stale/terminal edits are checked.

One complete batch accepts the edited relationship with new Entity/Predicate, an error correction of the original tea Claim, and the edited local-date clock observation; it rejects a separate one-time suggestion without a Semantic Operation. Preparation and inspection return identical complete bytes; group previews bind the batch ID, exact actions/refs/effects/digests, and one starting scope vector. Cross-group proposed-identity dependency, changed digest/action, duplicate or unknown JSON keys, trailing JSON, missing scope/IDs, and invalid command flags fail without CLI output. Accepted receipts match exact generated operation/Claim IDs and enumerated own revision advances. Public operation inspection retains the exact preview and batch audit. Owner and tool source authorities remain separate; the clock date retains 0:10 provenance without inventing UTC.

After DB reopen, identical delivery returns identical receipt bytes; changed approval conflicts. Canonical projection replay passes with extraction/client/tool counters unchanged from fixture creation. This is deterministic conformance, not semantic quality adjudication or a David pilot.

Final checks:
- `go test ./cmd/evie -run '^TestOwnerReview(BatchCLI|ClockCLI|TemporalCLI|IdentityCLI)' -count=1`: PASS, 1.119s.
- `go test -race ./cmd/evie -run '^TestOwnerReview(BatchCLI|ClockCLI|TemporalCLI|IdentityCLI)' -count=1`: PASS, 20.199s.
- Ordering regression `go test ./cmd/evie -run '^TestOwnerReviewBatchCLI' -count=10`: PASS, 1.876s.
- `gofmt -w cmd/evie/candidate_batch_test.go` and `git diff --check -- cmd/evie/candidate_batch_test.go`: PASS.

The first race run caught a test-only source-order assumption; its failing log and the follow-up probe are retained. Original is compared with the actual compiled extraction, while Edit.Before is compared with canonical CLI disclosure. No production fix was necessary.

Root will run required full repository verification and independent Standards/Spec review on the integrated frozen ticket144 tree; they were deliberately not duplicated here. Exact check metadata is in `144-cli-verification.json`.
