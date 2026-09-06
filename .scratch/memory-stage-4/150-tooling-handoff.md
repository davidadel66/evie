# Ticket 150 tooling checkpoint

Nine new files are frozen in `150-tooling-frozen-manifest.json` and
`150-tooling-frozen-files`. No pre-existing or production files were changed.
The full pilot remains incomplete. No large workload experiment has run yet.

The implementation is under `scripts/memory-stage4-pilot/`: Go command with
actual public Kernel, Agent.Send, production compiler host and review paths;
separate explicit active-time recorder; Python matrix/resource wrapper; focused
Go and Python tests; README. The preparation contract is
`cmd/evie/docs/fixtures/memory-stage4-pilot/v1/preparation.json`.

## Resume measurements

After ticket 149 has a frozen snapshot and a complete passing conformance
receipt, copy only these nine frozen files into that isolated snapshot. Run:

```sh
python3 scripts/memory-stage4-pilot/measure.py --conformance /absolute/149/report.json --output /absolute/new-results --repetitions 3
```

The wrapper requires status `passed`, zero failures/skips, and matching existing
Go dependency hashes. It builds a hashed executable, freezes a plan, and runs
sequential disposable databases. It offers eleven independent-factor variants
and rotates disabled/new/history mode order over three repetitions: 99 trials.
Each has 16 fixed foreground turns. Event sizes include 10k/100k/1m, source
length 256/4096/12000 bytes, accepted Claim count 1/100/1000, destinations 1/16,
scripted service delay 25/0/250ms, and worker processes 1/2. Baseline backfill has
16 potential roots; history mode explicitly selects them, preserving one failed
gap and one completed empty result. Normal modes preserve identical source data.

The runner uses only scripted infrastructure extraction and a constant scripted
foreground provider. It records actual terminal commits and response finalization
after a real host write to os.DevNull. Model-server resources and semantic quality
remain null. The sampled process-tree RSS/CPU includes setup; database/WAL sizes,
job attempt latency/freshness/counts, finite catch-up and actual scripted
preview/resolve costs remain distinct. Dispatch intervals must not overlap across
worker processes. Every temporary database and owned worker is cleaned up. A
3 GiB free-space guard prevents starting another fixture; current free space was
17 GiB. Final report metadata must label actual tested limits, incomplete metrics,
and failed experiments honestly.

Once results exist, publish raw reports/resource receipts and a readable versioned
pilot report under `cmd/evie/docs/fixtures/memory-stage4-pilot/v1/`, then freeze the
final full ticket manifest (tooling plus results/report). Root owns full isolated
verification, both independent reviews, and one ticket commit. Ticket 150 must
remain open for missing actual model/human observations and adopted numerical
gates; no final holdout was created, exposed, or run.

## Check results

- `go test -count=1 ./scripts/memory-stage4-pilot`: PASS, 2.938s.
- `go test -race -count=1 ./scripts/memory-stage4-pilot`: PASS, 13.148s.
- `go vet ./scripts/memory-stage4-pilot`: PASS.
- `python3 scripts/memory-stage4-pilot/measure_test.py`: PASS, three tests.
- Python bytecode compilation: PASS.
- Root full verification and independent reviews remain pending by assignment.

The agent for ticket 151 has the report/preparation schema and knows that
`infrastructure_status: passed` is not a completed pilot or release approval.
