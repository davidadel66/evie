# Ticket 144 correction migration and record-bound contribution

Final files are frozen and require no further edits from this agent. Fold both contributions into ticket 144's single commit; do not create a migration ticket or independent commit.

## Correction migration

- Frozen directory: `.scratch/memory-stage-4/144-migration-frozen-files`.
- Exact manifest: `.scratch/memory-stage-4/144-migration-frozen-manifest.json`.
- Original tracked bytes: `.scratch/memory-stage-4/144-migration-originals/internal/eviedb/semantic.go`; new files had no prior bytes.
- Review entry: `ensureSemanticBaseSchema` and `migrateSemanticCorrectionSchema` in `internal/eviedb/candidate_review_correction_schema.go`, with the narrow hook and DDL concatenation in `internal/eviedb/semantic.go`.
- Changes the correction primary key to `(operation_id, old_claim_id)` while retaining unique old/replacement Claims, all four FKs, mode/time/positive-revision checks, scope index, and append-only triggers.
- Holds `BEGIN IMMEDIATE` while inspecting the schema, migrating, and bootstrapping base tables. It copies all cells and rowids without decoding historical bytes, retains foreign-key enforcement, and recreates auxiliaries transactionally.
- Recognizes shipped old/current DDL only; incompatible columns, checks, FKs, indexes, triggers, or inbound references fail closed. A temporary-name collision does not overwrite the pre-existing table.
- Tests populate the frozen Stage 3 correction table (from commit 9ef0771) through Stage 3 prepare/apply APIs before Stage 4 startup. They compare SQLite storage classes and every cell's bytes for operations, events, semantic projections and lifecycle/source records before/after upgrade and shadow rebuild; exercise canonical old replay before/after; absent/durable commit-response failures, cancellation, rollback, FK-copy failure, reopening, and concurrent startup; and prove multiple same-operation corrections plus every relational boundary.
- The fresh schema initializer test preopens/pings independent WAL connections before the simultaneous schema calls. An earlier version raced SQLite journal-mode initialization and failed with `open immediate transaction connection: database is locked (5) (SQLITE_BUSY)` before reaching the migration. That separate connection-initialization race is not changed by this ticket. Populated concurrent `OpenDBAt` integration coverage remains.
- The v5 runtime acceptance/replay case with two correction members is owned by implement_144_edits_batches; this agent's multi-correction inserts prove relational constraints only.

## Additional record-bound test

- Reserved by implement_144_edits_batches after migration implementation.
- Frozen directory: `.scratch/memory-stage-4/144-record-bound-frozen-files`.
- Exact manifest: `.scratch/memory-stage-4/144-record-bound-frozen-manifest.json`.
- File: `internal/eviedb/candidate_review_batch_record_bound_test.go` (new).
- Uses real compiler source windows, explicit owner identity choices, and actual enumerated effect records. Twenty independent groups totaling exactly 256 records prepare, persist, all accept, and replay. A separate 257-record selection fails `review_too_large` without a new batch preview or accepted operation.
- Both variants use actual supported relationships: new subject/object identities, new distinct Predicates and Aliases, seeded owner reuse, and a second owner support source. No production helper or synthetic integer-only bound substitutes for the Kernel seam.

## Final verification

Run from `/Users/davidboktor/code/evie` against the working contribution (shared ticket 144 code compiled successfully):

- `gofmt -w internal/eviedb/semantic.go internal/eviedb/candidate_review_correction_schema.go internal/eviedb/candidate_review_correction_schema_test.go internal/eviedb/candidate_review_batch_record_bound_test.go` — formatted.
- `go test ./internal/eviedb -run 'Test(CorrectionSchema|CorrectClaim|SemanticProjectionReplay|SemanticProjectionRebuild)' -count=1` — PASS, 1.639s; `144-migration-normal.log`.
- `go test -race ./internal/eviedb -run 'Test(CorrectionSchema|CorrectClaim|SemanticProjectionReplay|SemanticProjectionRebuild)' -count=1` — PASS, 33.755s; `144-migration-race.log`.
- `go test ./internal/eviedb -run '^TestOwnerReviewBatchRealSQLiteInclusiveSemanticRecordBound$' -count=1` — PASS, 0.606s; `144-record-bound-normal.log`.
- `go test -race ./internal/eviedb -run '^TestOwnerReviewBatchRealSQLiteInclusiveSemanticRecordBound$' -count=1` — PASS, 8.755s; `144-record-bound-race.log`.
- `git diff --check -- internal/eviedb/semantic.go internal/eviedb/candidate_review_correction_schema.go internal/eviedb/candidate_review_correction_schema_test.go internal/eviedb/candidate_review_batch_record_bound_test.go` — PASS.
- `git diff --no-index --check /dev/null <new-file>` separately for each of the three new files — PASS (checks untracked files as well).

No live model, production dependency, staging, commit, branch/worktree changes, or semantic_graph.go edits. Root owns independent Standards/Spec review and `./scripts/verify-change.sh` against the combined exact ticket 144 snapshot; those full checks were not repeated by this bounded subagent. No remaining implementation work in these reserved files.

Manual demonstration: open a populated Stage 3 database through ordinary startup and inspect correction history; it remains unchanged. Owner 144's CLI batch demonstration exercises multiple corrections sharing one accepted operation on the upgraded database.
