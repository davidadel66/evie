# Ticket #138 activation and host handoff

Root integration map, not a replacement for the approved issue or binding contracts. Implement with the implement skill, on codex/memory-stage-4, after #137 worker APIs freeze. Read published-bodies/07-new-evidence-activation.md and binding evidence/work contracts plus fixtures. No selected adequate extractor exists; do deterministic engineering with explicit configurations and never install a default or claim actual-model acceptance.

## Ownership and preserved work

#137 owns worker/recovery additions and necessary compiler_schema/work/inspection changes. Coordinate its final checkpoint before writing those files. #140 owns candidate_review files and accepted semantic hooks. Prefer new compiler_activation/reconciliation and cmd host/management files. Root integrates main.go and any clean shared schema hooks. Root has original bytes for all218 preexisting user files in preexisting-user-bytes/ (two original deletions absent); never stage whole main.go or user-owned internal/web/serve.go.

## Activation decision

One owner-authorized, revision-CAS transaction captures current atomic commit position F and complete verified generation identity for an exact source-lineage selector/destination. Request idempotence returns original F. Overlapping selectors for a destination conflict; session destination remains distinct. New activation selects only positioned events >F; legacy and <=F are outside selection. Replacement closes previous interval at F2 and opens new afterF2, keeping previous jobs pinned. Disabling closes at captured frontier and pauses selected incomplete work; resumed old work retains its manifest. No configuration means no activation/materialized jobs. Unreachable configured endpoints use #137 bounded failure behavior.

The current #136 append-position trigger is inside the event INSERT transaction. Extend the same atomic trigger/side-record arrangement so selected appends record a coalesced high-water/dirty obligation, independent of job queue availability or channel notification. Do not depend on unordered execution between separate AFTER INSERT triggers. Event append/activation serialize through SQLite, and rolled-back append must leave no indication.

## Reconciliation and worker seam

Queue creation, source cutoff/closure/lease decision, ownership and scheduling metadata must be one serialized transaction. #137 owner has been told not to set lane/position through a second public mutation after enqueue; a worker could otherwise claim wrong-lane work. Use a trusted internal helper to create selected jobs with new-evidence scheduling metadata. Queue-only public explicit compilation keeps its documented lane.

captureCompilerWindow(ctx,*sql.Conn,owner,selection,first) already accepts the first newly-owned sequence; it accounts for earlier root contents as overlap when first > root sequence. Current selection only exposes cutoff. Activation must supply min selected sequence from F and existing ownership without inventing preceding completion, changing arbitrary source snippets into selection, or extending across unrelated roots. Account for intervening control/excluded root members exactly. Bound each reconciliation page/transaction; cursors cannot hide deferred roots or queue-limited obligations.

Do not replace D3 with a blanket live-lease exclusion: final no-tool assistant, recorded terminal or later root can independently close a captured prefix. Lease expiry alone can make a committed failed/crashed/command prefix eligible; never fabricate terminal events. Keep existing unknown/oversized/missing data classifications.

## Actual host integration

Root main.go currently dispatches explicit compiler and owner review commands before provider construction. Short commands should exit without draining inference. Only configured long-lived REPL/web hosts start supervisor. Foreground model/tool/episode commits never wait on Extract, including when a scripted extractor blocks. Stop claims and cancel clients on shutdown, then use #137 bounded five-second cleanup. Existing web serveServer uses http.ListenAndServe without context shutdown; REPL blocks on scanner. Design a small testable lifecycle adapter for the actual entries; do not add an unrelated daemon or refactor unrelated setup. Preserve current web user-owned extra database-inspection argument.

CLI should demonstrate exact activate/status/disable/resume semantics without printing protected source. Explicit config bytes use existing bounded localextractor Config decoder. Changing host endpoint is not generation change if the pinned contract is verified; endpoint/version/model is not server boot identity and cannot release uncertain capacity.

## Verification

Real SQLite two-store activation/append and replacement races; lost notification/reopen; rollback; all closure classes; current-root postF suffix; disabled interval stays outside selection; queue full preserves dirty obligation. Exercise normal foreground finalization while inference stalls, both configured unavailable and unconfigured cases. Observe finalization/scheduling durations needed by later pilot without inventing performance budget. No live model calls or fabricated measurements. Run focused checks, freeze exact per-ticket before/after hashes, independent Standards/Spec reviews and root isolated full verify.
