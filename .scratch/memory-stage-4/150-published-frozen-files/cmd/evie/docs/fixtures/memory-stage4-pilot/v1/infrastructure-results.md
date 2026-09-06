# Stage 4 infrastructure pilot observations

The final 99-trial infrastructure matrix passes exact deterministic outcome checks after two reconciliation defects were corrected. **The actual model/owner pilot and final release evaluation remain incomplete.** No adequate model, model-output adjudication, David review observations or numerical release gates were inferred from these measurements.

All work used the real Kernel, SQLite, Agent.Send, compiler host processes and public candidate preview/resolve APIs with explicitly scripted providers and artificial data. There was no learned-model inference or human semantic judgment. Final-holdout narratives were not created, exposed or run.

The measured code tree is `4b29071dc69018a9fedb2464315453baab0c9025`. The final matrix binary SHA256 is `bd190f98c3648c9f0a2e7faa1098389428fc4258344a6a1b74e4532eeed55e43`. [The machine-readable report](infrastructure-observations.json) pins its source inventory, generation, workload, corpus/scoring-contract hashes and conformance. Publishing these report artifacts changes the delivery tree: the eventual delivery commit is not claimed to have been measured. The thirteen frozen ticket production/tooling files are preserved unchanged.

Each of eleven workload variants has three paired repetitions of compilation disabled, explicit new-evidence processing, and new evidence competing with explicit historical catch-up. Mode order rotates. The baseline uses 10,000 archived events, 256-byte inputs, one accepted Claim, one session destination, 25ms scripted extraction delay, one worker, sixteen foreground turns and sixteen available historical roots. Variants independently change retained events to 100,000/1,000,000; source bytes to 4,096/12,000; Claims to 100/1,000; destinations to sixteen; service delay to 0/250ms; or workers to two. These factors were not combined into a capacity promise.

The final run contains **1,584 foreground turns**, **4,752 actual persisted foreground events**, **1,584 extraction attempts** and **219 scripted preview/resolve operations**. Job outcomes are 1,518 completed-candidate, 33 completed-empty and 33 failed. The failed jobs are exactly one deliberately injected invalid-output historical root per history trial; **unexpected job outcomes are zero**. Each real dispatch is preserved; cooperating extraction intervals did not overlap. Every disposable database was removed.

The event denominator includes context snapshots committed by Agent.Send, even though those snapshots cannot support a memory. Completed-event counts are checked against the actual selected membership, and history counts come from the explicit history receipt. A failed historical root remains a coverage gap while later work progresses. This finite workload did not induce retries, cancellation or stale-attempt waste; those remain separately tested conformance behaviors.

Paired foreground overhead is the median of three within-repetition p95 differences in milliseconds. Each p95 has sixteen observations and therefore equals that trial's maximum under nearest-rank calculation. These are small-sample observations, not stable tail-latency estimates. Negative deltas are noise, not evidence that compilation accelerates foreground work.

| Variant | Terminal, new | Terminal, history | Finalization, new | Finalization, history |
|---|---:|---:|---:|---:|
| baseline | 0.579 | 0.182 | 1.770 | 3.322 |
| retained-events-100000 | -0.182 | -0.355 | 4.316 | -1.077 |
| retained-events-1000000 | 0.094 | 0.808 | 1.298 | 2.573 |
| source-bytes-4096 | 0.228 | -0.024 | 3.438 | 1.712 |
| source-bytes-12000 | 0.025 | 0.546 | 7.369 | 8.849 |
| graph-claims-100 | -0.195 | 0.037 | 2.585 | 3.907 |
| graph-claims-1000 | -0.139 | -0.188 | 2.347 | 2.098 |
| scopes-16 | -0.035 | -0.078 | 2.143 | 2.666 |
| delay-ms-0 | 0.733 | 0.080 | 1.412 | 1.238 |
| delay-ms-250 | 0.418 | 0.013 | 2.068 | 2.668 |
| processes-2 | 0.152 | -0.006 | 0.889 | 0.096 |

Across all final foreground samples, the largest terminal-event commit was 2.326ms and the largest host response finalization was 36.457ms. The host finalizer follows an actual write to os.DevNull; it excludes browser, SSE, network and conversational-provider latency. Scripted public preview/resolve median was 2.813ms, maximum 12.962ms. This is database resolution cost, not active human review time.

| Variant | Maximum candidate freshness, seconds | Sampled peak Go process-tree RSS, MiB | Maximum DB after, MiB | Maximum WAL after, MiB |
|---|---:|---:|---:|---:|
| baseline | 1.034 | 55.562 | 6.055 | 4.086 |
| retained-events-100000 | 1.049 | 57.688 | 42.781 | 4.035 |
| retained-events-1000000 | 1.050 | 59.094 | 412.191 | 4.016 |
| source-bytes-4096 | 1.148 | 63.312 | 7.000 | 4.043 |
| source-bytes-12000 | 1.163 | 67.734 | 7.727 | 4.035 |
| graph-claims-100 | 1.059 | 58.547 | 6.855 | 4.059 |
| graph-claims-1000 | 1.048 | 56.375 | 13.094 | 4.071 |
| scopes-16 | 1.037 | 58.469 | 6.094 | 4.047 |
| delay-ms-0 | 1.018 | 54.547 | 6.082 | 4.071 |
| delay-ms-250 | 4.872 | 55.562 | 6.086 | 4.173 |
| processes-2 | 1.033 | 82.062 | 6.047 | 4.035 |

Retained archived data is inserted in bounded transactions before the measured foreground interval. The million-event variant grows the real indexed database; foreground conversations remain separate small sessions. One busy destination and sixteen destinations test different workload distributions, while Workspace/project authorization is separately covered by deterministic conformance. RSS/CPU are process-tree samples at 100ms, including setup, and can miss peaks. There is no model server, so model-server resource observations are null.

Observed finite-workload rates, minimum–maximum across the thirty-three trials per mode:

| Mode | Foreground persisted events/s | Completed selected events/s | Candidate arrivals/s |
|---|---:|---:|---:|
| disabled | 51.79–55.51 | 0.00–0.00 | 0.00–0.00 |
| new | 50.88–55.41 | 8.69–36.01 | 2.90–12.00 |
| history | 49.54–55.35 | 7.79–47.34 | 3.00–18.21 |

The history completion rate includes selected backfill, while its offered foreground rate counts only newly persisted foreground events. These finite intervals include worker startup and catch-up, so they cannot determine sustained service capacity.

The resource receipts contain 7,158 nonempty process-tree CPU samples. The largest sum of ps-reported CPU percentages was 121.8%, including fixture setup. These percentages are interval/lifetime estimates and may span multiple cores; this is not instantaneous CPU utilization or model-server cost.

Source-arrival, observed compilation and candidate-arrival rates are retained per trial in the raw matrix report. Finite catch-up completion is not a sustained capacity measurement. Owner review capacity, active seconds, candidates per useful accepted change, supported useful precision and required-memory recall remain unavailable. Scripted approval rate or an empty inbox is not a quality score.

The host is an Apple M3 Pro with 18 GiB RAM and eleven CPUs, macOS 15.7.8. Exact power observations for the final matrix:

```text
Now drawing from 'AC Power'
 -InternalBattery-0 (id=22085731)	95%; charging; 0:21 remaining present: true
Now drawing from 'AC Power'
 -InternalBattery-0 (id=22085731)	99%; finishing charge; 0:04 remaining present: true
```

OS cache, thermal state and unrelated operating-system activity were uncontrolled. The original matrix ran on battery; the final matrix ran under the power conditions above. Original and final total intervals also use different catch-up validation. These experiments are not a paired before/after benchmark of the correction.

The first complete matrix produced 111 unexpected zero-attempt empty-selection failures across 54 trials, although its original command-level checks returned success. That matrix is disqualified. Source capture could seal a root before discovery reached its last member; later-root coordinates then manufactured an empty suffix during reconsideration. The correction preserves exact root boundaries and reuses already captured members, including sparse coordinates, pre-activation roots and interleaved late members.

The second matrix was interrupted after twenty complete trial receipts when independent review reproduced another ownership gap: A1..2 and an explicitly owned historical A5..6 surround B3..4. Both automatic reconciliation orders now record a proven zero-member gap as excluded/no_root_members bookkeeping with no job or coverage. Tests verify both orders, three database reopen cycles, unchanged historical ownership/lane, no false B coverage, genuine later A progress and unchanged direct explicit-selection errors. The partial run retains nineteen complete resource receipts; sampling for the twentieth trial was interrupted. It is not used for paired conclusions.

The runner now waits for discovery/materialization to settle and validates exact jobs, attempts, dispatches, candidates, states, selected events and completed coverage. It retains expected counts and failures alongside raw evidence. Fresh browser/full conformance and both independent review axes passed on the measured tree before the final matrix. One prior conformance run failed only because the browser receipt path was mistyped; that failed report is preserved beside the subsequent valid runs.

Artifacts preserve the original evidence and denominators without database files or executables:

- [original-matrix-v1.tar.gz](original-matrix-v1.tar.gz): 822,209 bytes; SHA256 `46c1c7ef8e0e9eeda3d6dfcfa02ba2616feda4fdff2eef1ad0c7924d0131fd40`.
- [corrected-matrix-v3.tar.gz](corrected-matrix-v3.tar.gz): 815,678 bytes; SHA256 `91642e94a35c8b5718490b1912619b9806592702e004fdee2a3282662476260c`.
- [interrupted-matrix-v2.tar.gz](interrupted-matrix-v2.tar.gz): 191,926 bytes; SHA256 `dddddaaef984180af98be2cb1f9456745db4cc1db4f465951b1743ecc8181aeb`.
- [corrected-conformance-evidence.tar.gz](corrected-conformance-evidence.tar.gz): 216,756 bytes; SHA256 `b7b360dc0ffe675ffb07152ff719fbde014373293f490b031f09352b352d8ac9`.

To inspect raw receipts, extract an archive with `tar -xzf ARCHIVE -C NEW_DIRECTORY`. Its report.json and plan.json identify every workload command and receipt hash. The interrupted archive instead contains interruption.json and the retained partial receipts. [The runner instructions](../../../../../../scripts/memory-stage4-pilot/README.md) reproduce the experiment from a frozen code checkout after complete matching conformance; no threshold is supplied by this report.

[An independent root audit](infrastructure-audit.json) verified every final trial hash, unique job/dispatch identity, exact outcome count and cleanup result.

[Owner-session preparation](preparation.json) and the explicit active-time recorder are ready for actual sessions once a development configuration and output adjudication are available. Numerical quality, recall, foreground, freshness, resource and review gates must be frozen from that actual pilot before final-holdout exposure or ongoing enablement. Tickets #150 and #151 experimental acceptance stays pending.
