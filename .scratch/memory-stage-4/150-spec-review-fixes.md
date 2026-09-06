# Ticket 150 narrow Spec recheck

**Pass: all three preliminary findings are resolved.** Reviewed final tooling tree `d12470f707ba5cc4ae3f4f45df4b3f2fc4361871` over conformance base `ebb84af94f37b5bb5278d54208dc1d9f2dba73c7`, with the corrective delta from preliminary tooling tree `b87d22cdd88034a029b1eba5b0ddf4af8313582f`. Exact snapshot: `/var/folders/nv/59bkjyks06gdx0l5k1v5pqk40000gn/T/evie-150-measurement-checkpoint-brqc4qx4`.

- **Event denominator:** the runner queries each foreground session's actual committed sequence interval, records per-type counts, and computes arrival rate from persisted events. The real Kernel smoke checks user, snapshot, and assistant counts and equality with new-mode completed coverage.
- **Conformance binding:** validation now compares complete production dependency path sets and hashes. Added or missing internal/cmd Go files are rejected; the deliberately separate pilot tooling is allowed.
- **Failed receipts:** absent and malformed trial reports yield explicit failed observations with null or preserved raw-byte hashes. The wrapper retains failed entries, excludes them from paired deltas, and writes the versioned aggregate failure report.

Independent checks from the exact snapshot:

- `go test -count=1 ./scripts/memory-stage4-pilot -run '^TestPilotRealKernelSmoke$'`: PASS, 2.969s.
- `python3 scripts/memory-stage4-pilot/measure_test.py`: PASS, six tests, 0.027s. Printed failed trial statuses are deliberate failure-reporting fixtures, not failed tests.

No outstanding finding within this narrow recheck. The declared infrastructure matrix may start. This verdict does not approve unproduced measurement results, model quality, actual owner sessions, numerical release gates, or release readiness. Final result/report additions still require independent delta review; ticket 150 remains incomplete for its acknowledged human/model requirements.
