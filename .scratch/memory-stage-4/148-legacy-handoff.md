# #148 bounded legacy status contribution

Four files are frozen in `148-legacy-frozen-files` with hashes and existing-file predecessors in `148-legacy-frozen-manifest.json`. No staging, commits, branch operations, dependencies, or root integration hooks were changed by this contributor. Both existing files matched HEAD before editing; their original bytes are in `148-legacy-originals`.

## Behavior

`InspectCompilerActivations` and `InspectCompilerHistory` retain their exact small-session totals. Large sessions use a persisted exact-scope count cursor: one request indexes at most 128 event coordinates and, for activation status, 32 root records. It commits this progress and returns `ErrCompilerStatusIndexing` with an instruction to repeat the authorized request. No partial, capped, or estimated count is returned as exact. Source text is never loaded by total indexing.

Normal event appends extend the immutable session sequence cursor. Live activation only selects future append positions. History-reference insertions atomically increment the exact generation/destination/session union revision; a later status request rebuilds only that affected count cursor in bounded pages. Reference updates for overlaps, cancellation, and resume do not change the ever-selected union. Legacy position deletion invalidates the affected session. No accepted operations, compiler requests, candidates, or source events are rewritten.

Activation root totals use the immutable `(activation_id, root_id)` prefix rather than implicit rowids. Insert/update/delete triggers adjust counts for roots in an already indexed prefix; later roots are read by bounded continuation. Counts survive rollback, concurrent connections, reopening, VACUUM, and discovery of a root whose ID sorts before the cursor.

Activation metadata listing uses two indexed top-129 buckets (scope-wide and exact-session), then sorts at most 258 metadata rows. Root listing and root/bootstrap high-water lookups use matching indexes. Per-event activation membership uses a constant two or four predecessor seeks. Equal activation frontiers use the largest endpoint, with an open interval first, so resuming an old empty segment cannot hide a newer open segment.

Detailed history range inspection retains the existing explicit <=10,000 selected-event receipt bound and <=64 returned intervals. That computation is distinct from the 128-coordinate session-total continuation. Its selection join now seeks one immutable interval predecessor, then verifies the endpoint; `nextCompilerInterval` guarantees disjoint ownership per generation/destination/session/root. A gap does not borrow a later or earlier owner, and a busy root does not cause each event to scan all retained selection segments.

## Integration

`ensureCompilerStatusProjectionSchema(ctx, db)` is called sequentially by the diagnostics owner's `ensureCompilerDiagnosticsSchema`, after its diagnostics transaction. It creates side tables, triggers, and indexes; it does not reconstruct retained count totals. Standard SQLite first-time index creation still builds indexes over retained metadata once. Normal subsequent startup does not perform count sweeps. Do not describe first-time DDL index construction itself as constant work.

## Verification

- `go test ./internal/eviedb -run '^TestCompiler(Activation|History)' -count=1`: PASS 5.315s on initial replacement.
- `go test ./internal/eviedb -count=1`: PASS 40.711s before the final predecessor/stable-key refinements.
- `go test ./internal/eviedb -run '^TestCompiler(StatusProjection|Activation|History|Intervals)' -count=1`: PASS 6.163s on final production bytes.
- `go test ./internal/eviedb -run '^TestCompilerStatusProjection' -count=1`: PASS 1.016s on final frozen files, including the last before-cursor insertion assertion.
- `go test -race ./internal/eviedb -run '^TestCompiler(StatusProjection|Activation|History|Intervals)' -count=1`: PASS on final frozen files: `ok  	github.com/davidadel66/evie/internal/eviedb	115.592s` (log `148-legacy-stable-key-race.log`). The earlier 115.949s broad race log is also preserved.
- `gofmt` on all four files and `git diff --check` on existing edited files: PASS.
- Root owns required isolated `./scripts/verify-change.sh`, independent two-axis reviews, aggregate one-ticket commit, and UI/CLI integration checks; those are not replaced by this contribution's focused checks.

New tests exercise public exact-scope APIs with real SQLite: bounded legacy bootstrap, empty startup projections, exact rollback and error identity, concurrent continuation, current authorization after caching, cancellation identity, overlap/cancel/generation/destination union isolation, VACUUM/reopen, root deltas before/after the cursor, equal-frontier activation replacement plus old-segment resume, 3,000 retained event/activation/root query plans and page counts, and 3,000 same-root selection intervals with an explicit unowned gap. They dispatch no model inference.

Review entry points: `compiler_status_projection.go` (schema, revision maintenance, bounded caches), the two legacy inspection methods, and `compiler_status_projection_test.go`.
