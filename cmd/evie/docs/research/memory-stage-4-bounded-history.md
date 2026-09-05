# Stage 4 bounded historical selection (#139)

The shared Kernel selects retained history through `SelectCompilerHistory`,
separately from live activation. Every range names its exact source lineage,
destination, session, inclusive first/last sequence, and boundary event IDs.
The trusted local adapter resolves and authorizes each source independently.

One transaction admits 1–100 ranges and at most 10,000 distinct selected events.
Its immutable receipt pins original ranges, generation, count, and selection
order. Later appends do not extend it. Identical request deliveries return that
original receipt after restart or cancellation; changed content conflicts.
Legacy events keep their original coordinates without fabricated commit positions.

A reconciliation step discovers one event's ancestry and seals one root in
separate transactions. Live and historical discovery share the same resolver.
Each transaction inspects at most 128 events and mutates fewer than 64 ledger
rows. Indexed pending-request counters exclude retained completed/cancelled
histories from discovery. A separate bounded explicit-resume queue avoids
polling all retained cancelled jobs. Indexed coordinates and aggregate counts are separate from source-field
inspection. Root obligations remain revisitable through live leases, missing
configuration, full queues, and restarts. An archived Project, inactive
Workspace, missing registry identity, or typed semantic quarantine pauses its
source obligation with `source_scope_unavailable`. Only safe scheduling metadata
is committed, allowing other selections and explicit resumes to progress.
Restoration reconsiders the original cutoff; a closed source session remains
eligible. Unexpected SQL, cancellation, or transaction failures are errors, not
authorization pauses. A later root outside a selected cutoff can establish
closure without supplying outside evidence or coverage.

Selection reuses owned intervals and job budgets, including failed work. The
persistent scheduler gives new work its existing priority and gives the oldest
ready historical selection the ninth saturated dispatch after eight new attempts.
Later independent candidates remain reviewable across an earlier failure gap.

Only jobs newly created by historical scheduling acquire history cancellation
authority. Reused explicit/live jobs retain their original authority and lane.
Cancelling a request leaves its jobs untouched when another active historical
request selects overlapping evidence. The explicit owner cancellation
transaction fences at most the existing 1,024 unfinished jobs. Explicit owner
select/cancel/resume also adjusts at most 20,000 exact source/destination
reference rows: 10,000 selected events with at most two permitted destinations
per source. Self-overlapping ranges count once per destination, and repeated
operation deliveries never adjust twice. Background gates probe the indexed
references for only the job's at-most-128 sealed event IDs, avoiding scans of
retained overlapping receipts. Background transactions retain their separate
64-mutation limit. Cancellation changes fences
before interrupting clients, preserves stages and audit, and retains unknown
server capacity. A proven late completion releases only its exact original
reservation; it cannot publish a cancelled result or release a replacement.

Explicit resume verifies the pinned configuration and records a new resume
order. Background work readmits one cancelled job through the existing resource
and attempt checks. Merely submitting another overlapping selection does not
resume cancelled work. No skip action, cleanup, implicit historical generation,
accepted-memory change, or default model configuration is introduced.

## CLI demonstration

Prepare `selection.json`, replacing example identifiers with retained event IDs:

```json
{
  "request_id": "history-example-1",
  "ranges": [{
    "source_scope": "global",
    "destination": "global",
    "session_id": "SESSION_ID",
    "first_sequence": 1,
    "last_sequence": 12,
    "first_event_id": "FIRST_EVENT_ID",
    "last_event_id": "LAST_EVENT_ID"
  }]
}
```

```sh
evie memory-backfill select --selection selection.json --config compiler.json
evie memory-backfill status --selection selection.json --range 0 --limit 64
evie memory-backfill status --selection selection.json --range 0 --after 6 --limit 64
evie memory-backfill cancel --selection selection.json --operation cancel-1 --revision 1
evie memory-backfill resume --selection selection.json --operation resume-1 --revision 2 --config compiler.json
```

Select/resume verify metadata without inference. A configured long-lived REPL or
web host performs reconciliation and extraction. Status returns a bounded page
of exact intervals for one receipt range, its contiguous frontier, and counts of
selected/outside events for the same session, generation, and destination.
`next_sequence` is a page cursor, not a frontier. Each range/session has its own
frontier; disjoint selections cannot bridge unselected history. Successful
candidate groups, successful empty results, deterministic exclusions, failures,
retries, cancellation, and pending work remain distinct. Review decisions do not
advance coverage.

## Deterministic verification

`TestCompilerHistoryCLIMultiScopeSelectionProgressCancellationAndReopen` executes
the actual CLI adapter against real SQLite, two source sessions, and global and
Project destinations. A local HTTP fixture verifies configuration metadata and
asserts management commands make zero inference requests. Scripted extraction
through the real worker produces independently inspectable successful-empty
frontiers; cancel/resume and original receipts survive database reopen.

Kernel fixtures cover range/event limits and exact IDs, missing scope authority,
concurrent overlapping requests/reconcilers, persistent 8:1 fairness using actual
live/history selections, failed early work with later reviewable candidates,
source exclusion/oversize distinction, queue-full restart, ancestry 128/129
boundaries, cancellation rollback, cancel/resume ABA fences, explicit-resume-only
semantics, and prompt client cancellation with known/unknown release. Rejection
of a later candidate leaves exact coverage and the old failure gap unchanged.

The parent implementation checkpoint records repository-wide isolated
verification and independent Standards/Spec review before the ticket commit.
These deterministic checks establish durable work behavior, not model adequacy
or human evaluation quality.
