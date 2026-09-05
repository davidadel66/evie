# Stage 4 worker recovery engineering checkpoint

Ticket: #137, recover unfinished compilation safely across processes. Binding
behavior is in `../active/memory-stage-4-work-contract.decisions.md`, especially
W08–W13 of the adjacent worked fixtures. This record covers deterministic
engineering. No adequate selected model/runtime has been configured or evaluated
by this change; scripted HTTP release semantics are not Ollama acceptance.

The SQLite-backed worker exposes queue-only selected compilation, a one-step
worker, and a bounded long-lived supervisor. Queueing requires an explicitly
configured extractor but does not invoke it. Source windows are sealed on
selection; accepted-state context is sealed with the first atomic claim. Retries
reuse that complete request. Each claim records a separate durable attempt ID,
increments the five-attempt counter and fence, and reserves the database-wide
request slot plus staging/inbox capacity. Leases use SQLite time with a 30-second
expiry and 10-second renewal. A 100ms authority check interrupts a locally
running client after another process cancels its job.

The automatic retry delays are 5, 10, 20 and 40 seconds. Configuration, source
binding and forbidden effects fail closed; malformed/schema-invalid output is
retryable. Missing output never becomes successful empty extraction. Expired
workers and supervisor shutdown retain interrupted attempts and coverage gaps.
Complete durable stages can be adopted with a new fence and revalidated without
inference. Owner cancellation requires explicit resume, including an intact
stage saved by attempt five. Neither reselection nor resume grants request six.

Unknown server release retains `release_pending`. Lease expiry, process death,
connection close, and client cancellation never free that reservation. A trusted
runtime adapter can provide an authenticated acknowledgement for the exact
request/holder/fence/server reservation, or prove a controlled restart covering
that reservation. The production Ollama adapter has no such recovery mechanism;
endpoint/version/model identity is not a server boot identity. There is no CLI
force-release or unverified restart bypass. Completed durable stages can publish
while another job's capacity remains blocked.

The resource reservations enforce 128 groups/16MiB staging and 2,048 unresolved
candidate positions, with 16 positions reserved per extraction. The 1,024-job
queue preserves selection records when full. Recovery changes at most 16 jobs
per transaction (at most 64 ledger/resource mutations). A worker step performs
at most one inference/publication. The scheduler preserves job lane/order and a
shared fairness counter; activation and backfill install those fields in the
same transaction that creates a job. Their selection/host integration is owned
by #138/#139.

Owner commands:

```sh
evie memory-candidates status --session SESSION_ID --limit 32
evie memory-candidates cancel --session SESSION_ID --id JOB_ID
evie memory-candidates resume --session SESSION_ID --id JOB_ID
```

Status pages expose only bounded work metadata, retry due time, capacity state,
reservations and recovery advice. Existing exact candidate inspection remains
available separately. Cancellation and failed earlier intervals remain gaps in
the exact coverage ledger while independent later candidates are reviewable.
Activation/backfill surfaces own the corresponding selection segment vectors.

Deterministic tests cover real SQLite reopen, independent Stores, abrupt
subprocess exits before/after claim, after response, after staging and after
publication, attempt ceilings and due times, fifth-stage resume, stale outputs
and release acknowledgements, full capacity, safe status, accepted snapshot
pinning, later visible candidates, scheduler fairness, and prompt HTTP client
cancellation while the scripted server remains active. The implementation owner
runs focused/race checks; the root integrator runs `./scripts/verify-change.sh`
on the isolated ticket tree and shared integration tree, and coordinates the
independent Standards/Spec reviews before committing this ticket.
