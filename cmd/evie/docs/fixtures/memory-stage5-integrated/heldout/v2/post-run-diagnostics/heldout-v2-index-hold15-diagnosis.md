# Held-out v2 historical index failure diagnosis

The locked `hold15_archive_historical` case both requires the historical Marlow
boathouse source and puts that phrase in `forbidden_current_text`. The frozen Go
client and scorer prohibit every listed text occurrence anywhere in the payload;
only the separate source-ID rule has a historical exception.

Baseline, reopened and rebuilt runs each have a clean first call and fail on
**call 2 (array index 1)**. The matching phrase appears only in the memory
projection, as historical accepted evidence and its attributed original excerpt.
Both carry `intent: historical`, `status: retired` and `current_status: retired`.
The accepted interval is `[2019-04-01T00:00:00Z, 2020-08-15T00:00:00Z)` and contains
the requested 2020-01-20 point. The original owner source’s hash, current
eligibility and actual 2026-09-11 observation timestamp are intact.

No retired-as-current production leak is shown by these retained payloads.
Canonical references match exactly across the three variants, and retained
inspection/revision checks pass. Nevertheless, the actual turn errors keep the
frozen restart/rebuild gates **failed**. This diagnosis does not waive them or
claim a successful full run.

A future correction requires a separately authorized fresh assessment with
predeclared distinction between unconditional and current-only prohibitions and
pre-seal consistency checks. This locked assessment’s annotations, programs,
outputs and failures must remain unchanged.

Detailed report: `/tmp/evie-memory-stage5/heldout-v2-index-hold15-diagnosis.json`.
Report SHA256: `b5332d2e09077e7f6e7e2213f3703037f8e77d574cd18d7c1f6e272492952ca9`.
Original index artifact SHA256: `1312e21726080e543dcff5275adc8c77ebadee05cab73228881e82b100f149c6`.
Freeze SHA256: `c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2`.

Read-only artifact/source inspection only; no model calls, new queries, probes,
production edits, corpus edits, scorer changes or gate changes were made.
