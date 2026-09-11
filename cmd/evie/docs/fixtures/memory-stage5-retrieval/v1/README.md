# Accepted-memory slice measurements (#156)

Run from the repository root:

```sh
go test ./internal/agent -run '^TestMemoryRetrievalSliceMeasurements$' -count=1 -v
```

The `MEMORY_RETRIEVAL_SLICE_REPORT` line contains the versioned synthetic
fixture, actual generated evidence IDs, exact configuration, and raw samples.
`accepted-slice-report.json` records the implementation run on Apple M3 Pro,
18 GiB RAM, macOS 15.7.8 arm64. All provider responses are scripted; no model,
embedding, remote call, or held-out evaluation is represented by this report.

The fixture creates 45 accepted Claims through approved public operations,
backfills the real SQLite FTS generation, and performs 30 fresh complete turns.
Every turn must receive exactly five expected Claim IDs, preserving semantic
revisions. Timings bracket `Session.Send`; they include durable history, tools,
revalidation, and complete context serialization. Source creation and session
creation are outside the turn timing. Percentiles use nearest rank.

This record supports initial conservative resource caps. It does not establish
production throughput, peak memory use, answer grounding, or Stage 5 readiness.
