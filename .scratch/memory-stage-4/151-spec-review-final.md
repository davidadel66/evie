# Spec review — ticket 151 frozen engineering slice

**No unresolved Spec findings in the authorized engineering slice.** This is not acceptance of ticket 151's unperformed final evaluation.

Reviewed the exact 14 new files in `151-frozen-files/` against base149 tree `ebb84af94f37b5bb5278d54208dc1d9f2dba73c7`. Independently verified every file hash, all 118,852 bytes, absence of each path from the base, and manifest SHA-256 `6c2d2687a28377c2da5ed9aa143b208878de14d7233dea391657fab2ced17fcc`. Sources: active Stage 4 specification, first-round memory decisions, published ticket 20/#151, and the authorized preflight limitation. No Standards findings were imported.

Both earlier Spec findings are resolved:

- The requirement to “Keep narrative variants together” (`semantic-memory-stage-4.spec.md:387–389`) is enforced by a consistent history-to-family mapping (`stage4_release.go:434–442`). Regression coverage rejects conflicting assignments and preserves one cluster for multiple windows from one family.
- The required “queue and inference latency, throughput” and “active review time per useful accepted change, accept/edit/reject outcomes, and inbox age” (`spec:317–322`) now have retained observations. The 27-metric reporting contract is distinct from pilot-defined numerical thresholds. Every workload/repetition needs complete observations; missing or zero-denominator pairs prevent readiness. Review ratios are checked against actual action/usefulness/time counts. Configuration, environment, workload, source identity, distributions, counts, denominators, and sampling limits remain visible.

The release tooling preserves separate raw/retained quality and conformance panels, failure accounting, paired uncertainty, immutable report publication, and deferred Stage 5 panels. No unauthorized model execution, holdout construction, automatic activation, or other scope creep was found. Artifact authenticity and semantic adjudication remain explicit external trust boundaries.

Experimental acceptance remains pending: the spec requires “Freeze numerical release gates from that pilot before final-holdout evaluation” (`spec:323–326`). Adequate model selection, model-output adjudication, actual David pilot observations, frozen numerical gates, and untouched final-holdout execution are absent. Scripted #150 measurements do not satisfy these requirements.

Static review only; no tests were duplicated. Inspected receipts record successful focused normal/race tests and vet, plus deterministic pending-report publication and overwrite refusal. Required root `./scripts/verify-change.sh` remains pending under `spec:407–410`; this review does not replace it. No live source was edited.
