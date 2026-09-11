# Held-out V2 preparation correction

Agent `/root/heldout_curator` created V2 at 2026-09-11T06:40:07.171412+00:00 after root reported that V1
canonical preparation failed for the two compaction cases, before any held-out
reader or retrieval-performance evaluation. The frozen compaction contract
requires at least three completed turns and retains the newest two; V1 had two.

V2 appends this neutral third exchange to `hold07_handover_continuity` and
`hold08_volunteer_pickup`, with fresh IDs `neutral_exchange_hold07` and
`neutral_exchange_hold08`:

- Owner: “One moment.”
- Assistant: “Okay.”

No people, preferences, answer clues, recipient selection or new gold support
are introduced. The original first exchange becomes eligible for compaction.
All existing records, discussions, continuity strings, questions, gold labels,
roles, metadata and the other 22 cases are unchanged. The workload schema remains
version 1; the directory records revision V2.

```text
V1 SHA256 f487649e46249ab1f461f2c4b8551c12a5b6ff319461856d2d733ef9420a3cdd
V2 SHA256 22cd641595f6aa9791f212db4f9ce95a745edbd68444a073d106b5bc3b52582b
```

Static validation passed: the exact JSON diff contains only additions at
`/cases/6/recent_discussion/2` and `/cases/7/recent_discussion/2`. Removing them
restores the original serialized V1 bytes. Every V1 file hash is unchanged.
`preparation-correction.json` retains the exact diff, source-contract hashes,
timestamp and static-check record. Direct whitespace checks and
`git diff --check` passed.

No outputs were inspected beyond root's bounded preparation-failure report;
no models, embeddings or retrieval queries ran. Root owns canonical preparation
and sealing in a new destination and preserves the failed V1 attempt. No code,
configuration, rubric, gates, V1 files, staging or commits were changed.
