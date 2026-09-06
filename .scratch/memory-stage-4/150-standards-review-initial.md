# #150 Standards review — preliminary

Compared `ebb84af94f37b5bb5278d54208dc1d9f2dba73c7` → `b87d22cdd88034a029b1eba5b0ddf4af8313582f` (nine new pilot tooling files). Applied repository `AGENTS.md` and the complete code-review skill smell baseline; this is independent of the Spec axis.

One material finding:

- **P2 — Human review recorder ignores cancellation.** `scripts/memory-stage4-pilot/main.go:40` installs `signal.NotifyContext` for SIGINT/SIGTERM, while the review-session branch dispatches to `reviewCommand` without the context. `review.go:125` blocks in `scanner.Scan()`. An idle recorder, including an active candidate waiting for the next command, therefore survives Ctrl-C/SIGTERM indefinitely because the default signal action is suppressed and the canceled context is unused. Propagate cancellation to the input boundary or preserve the CLI default signal action for this command; test interruption while awaiting input and preserve completed observations without saving incomplete timing. This is a concrete cancellation defect under `AGENTS.md` Review priorities item 4, not a stylistic heuristic.

No additional documented-standard violations or material baseline smells found. Fixture-only database changes, exclusive output creation, private review observations, bounded workload factors, and separation from learned/human quality conclusions are appropriate to the assigned experiment.

No full checks or performance experiments were run by this reviewer. Root owns final verification. Known Spec refinements and actual workload reports remain pending; this review does not establish pilot readiness.
