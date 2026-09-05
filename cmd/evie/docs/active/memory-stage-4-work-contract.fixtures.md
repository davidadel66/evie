# Memory Stage 4 durable work: worked scenarios

These are deterministic acceptance oracles for the
[work contract](memory-stage-4-work-contract.decisions.md), not reports of an
implemented worker. `G1/G2` are pinned generations, `sA/sB` are independent
sessions, and intervals below are inclusive source sequences. Excluded control
events inside a unit remain accounted for without becoming factual support.

| ID | Initial state and interleaving | Required durable/observable result |
| --- | --- | --- |
| W01: failure gap and later success | `G1/sA` selects `[1,9]`; independent windows own `[1,3]`, `[4,6]`, `[7,9]`. First completes with candidates; second fails; third completes with candidates. | Successful ranges are `[1,3]` and `[7,9]`; gap `[4,6]` exposes its failure; frontier is `3`. Both successful groups are reviewable. Neither extraction reads the other's unaccepted candidates. Completing `[4,6]` later advances the frontier to `9` without republishing other groups. |
| W02: independent sessions | W01 exists in `sA`; `sB/[1,4]` completes and its event timestamps precede and follow `sA` unpredictably. | `sB` frontier is `4`, `sA` remains `3`. No scalar “scope sequence 9” is shown. Scheduler order is not conversational context or accepted-effect order. |
| W03: duplicate scheduling and delivery | Two reconcilers find the same selected sealed prefix, two wakeups arrive, and the client retries a completion after its acknowledgement is lost. | One owned interval, job and completed candidate group exist. The stored completion receipt is returned. No extra inference after known completion, duplicate candidates, coverage increment, or accepted operation. Different staging content for the same job/fence conflicts. |
| W04: racing activation and append | Global append position is `100`. One transaction activates `G1`; another appends a terminal event. | If append commits first as `101`, activation captures `F=101` and that event is outside live selection. If activation commits first with `F=100`, the append at `101` and scheduling indication commit together. Reconciliation cannot change which side won. A duplicate activation request returns the same F; a competing replacement with stale revision conflicts. |
| W05: disabled and historical selection | Events exist in the legacy cohort and positions `1..100`; activate at `100`. Disable at `110`; events `111..120` commit; reactivate at `120`. Select historical `sA/[5,12]` separately. | Only positions `(100,110]` and `>120` are live selected. The disabled interval and all other history remain outside selection. Previously selected incomplete jobs pause. Backfill owns only captured `[5,12]`; sequence `13` appended later is excluded from that request. No configured extractor means no activation or new pending job rows. |
| W06: queue saturation and lost notification | All 1,024 unfinished job slots are occupied. A selected final event commits and the process dies before notifying a worker. | Event/position/scheduling indication survive atomically. No 1,025th materialized unfinished job is admitted. Selection remains unmaterialized and visible; it consumes no attempt. A later reconciler creates exactly one job when a slot becomes free. No committed event is lost or mislabeled empty. |
| W07: incomplete prefix and late suffix | #132 H06 is captured at `sA/3` after a serialized no-live-lease check. A legitimate later transaction extends that root through `5`; prefix `[1,3]` is still retrying. | The old manifest/cutoff stays at `3`. Suffix `[4,5]` receives disjoint ownership under the applicable selection; it may use bounded earlier overlap. Only new support can own its candidate. Successful suffix coverage leaves frontier before `[1,3]` until that earlier work resolves. An actually live lease would instead have left the prefix deferred. |
| W08: every durable extraction boundary | Kill the worker (a) before claim commit; (b) after attempt reservation, before dispatch; (c) after response, before staging; (d) after stage commit; (e) after publication, before receipt. | (a) No attempt consumed. (b) Attempt consumed; uncertain capacity reconciles before replacement. (c) No durable result, so a counted retry is needed after release. (d) A new fence adopts/revalidates and publishes the stored envelope with no inference. (e) Return the existing publication receipt. At no point is a partial group reviewable. |
| W09: stale completion and release | Attempt holder/fence `A/7` times out; cancellation or replacement records fence `8`. A eventually returns a valid-looking response and tries to stage, complete, and release capacity. | All old-fence durable writes are rejected. Client returns within the transport cancellation bound, but capacity stays `release_pending` until the actual server request finishes or contracted release is proved. Fence `8` cannot start another inference while release is uncertain. A healthy socket alone proves neither release nor permission to publish. |
| W10: retry and repair budget | Starting at time `0`, each attempt fails immediately. Attempt 2 is a schema-repair request; processes restart between attempts. | Attempt starts are `0,5,15,35,75` seconds. Failures preserve counts and due times across restart. Attempt 5 failure records exhaustion; there is no automatic attempt 6 or new repair budget. Repeated selection of the same range/generation returns the exhausted job. Waiting for capacity or publishing a saved stage adds no inference attempt. |
| W11: empty, excluded, invalid, oversized | Four independent selected units produce: a valid explicit zero-candidate result; only policy-excluded source fields; an invalid source hash; and 32 KiB plus one byte of newly admitted support. | Respectively `completed_empty` with successful attempt; `excluded` with content-free reasons and no model call; `failed:invalid_source`; `failed:oversized_input` with zero calls. Only the first two permit their segment frontier to pass. Original episodes remain retained in all four. |
| W12: cancellation and shutdown | Cancel a running job before stage; separately cancel a staged job before publication; separately stop the hosting process during work. | Explicit cancellation durably fences both jobs and leaves gaps; saved staged data remains audit, unavailable for publication without explicit resume/revalidation. Publishing that intact stage consumes no new inference attempt, even if it came from attempt five; any new inference needs remaining budget. Shutdown stops new claims and performs bounded cleanup; absence of a supervisor never implies completion. Restart reconciles leases and server capacity before resuming selected work. |
| W13: fairness and bounded admission | Both lanes remain ready for 18 slot claims; three processes contend and restart. Inbox has only 15 free unreserved candidate positions. | With counter initially zero, backfill wins claims `9` and `18`; no two requests run concurrently. Persisted counters survive restart. Before any new extraction, admission waits for the required 16-position reservation; it neither evicts history nor consumes an attempt. A valid stage publishes its entire group atomically against its reservation. |
| W14: generation replacement and review | `G1` has accepted candidate A, rejected B, owner-edited unresolved C, and unedited unresolved D. Activate `G2` at `F2`, then explicitly select their historical ranges. | Old live selection ends at F2; old selected jobs retain G1. G2 produces distinct groups and independent coverage. Exact repeats link/suppress A–D without copying accepted/rejected states, replacing C's edit, changing D's preview, or creating semantic operations. A materially different support manifest appears freshly for explicit review with the old review link. A concurrent edit/review serializes with equivalence classification so it cannot be erased by stale suppression. |

For W01, the selected contiguous frontier progression is initially `0`, then
`3`, still `3` after later `[7,9]` succeeds, then `9` only after `[4,6]` succeeds.
If `[4,6]` is explicitly cancelled or exhausted, the frontier remains `3`; there
is no v1 skip operation. A disjoint selection `[12,14]` has its own frontier and
does not count unselected `[10,11]` as compiled.

For W08/W09, separately simulate lease expiry, explicit cancellation, abrupt
process death, normal transport completion, delayed server cancellation, and an
endpoint that cannot prove request release. The last case must remain visibly
capacity-blocked, even after the 120-second client deadline and 30-second job
lease have expired. The deterministic suite controls release acknowledgements;
the local-model spike separately demonstrates what the selected server does.

For W14, additionally vary one field at a time: generation/display confidence
only (equivalent); exact source event/hash/context (different); scope, polarity,
Valid Time, correction mode, Predicate definition, or identity alternatives
(different). Same-named Entities in different lineages must not collapse. A
repeated original extraction after an edited interpretation was accepted or
rejected links to that full review lineage and does not reopen the decision. A new source
supporting an accepted equal Claim can propose a source attachment, but cannot
attach it without a fresh exact owner preview and approval.

Verification for this document is whitespace/local-link checking and direct
calculation of these interval, retry, and fairness examples. The dependent
Kernel/SQLite and local-HTTP tests must establish the actual transactional and
cancellation behavior; reading this table is not evidence those tests passed.
