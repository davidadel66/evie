# Ticket 151 independent release-evaluation engineering

Status: engineering frozen for root full verification and independent two-axis
review. The final evaluation is NOT run and ticket 151 must remain open. There
is no adequate selected model, complete output adjudication, actual David pilot,
frozen numerical release plan or created/exposed final holdout.

The 12 owned paths are all new; no user file, root hook, other story file,
production dependency, branch, index, commit or issue was changed. Use
`151-frozen-manifest.json` plus `151-frozen-files/` for the isolated checkpoint.
No branch switch, commit, push, PR or issue closure was performed.

## Behavior

- New pure memoryeval Stage 4 submission/plan/report types retain existing
  Stage 3 panel identities and leave its conformance implementation unchanged.
- Predeclared exact config/generation/runtime/model/prompt/schema/decoding/policy,
  environment/source tree/rubric/corpus/gold/workload/baseline/repetition identity;
  actual human prerequisite approvals; independent complete-history custody;
  explicit N01–N12 exclusions plus later exposed families; exactly one logged
  frozen campaign exposure before every attempt.
- All planned case/role/repetition observations are required. Failures retain
  required-memory opportunities; raw/retained populations are separate; raw
  occurrence counts and deduplicated required matches use spike label vocabulary.
  Required/optional useful labels cannot conceal semantic error categories.
- Explicit precision/recall/error/unwanted/failure rates and denominators, paired
  baseline deltas and 1,000-resample narrative-family cluster uncertainty.
  No paired deltas on incomplete paired runs; no invented zero-denominator score.
- Five mandatory zero-tolerance conformance boundaries. Numerical gates have no
  supplied thresholds; positive recall and precision floors are mandatory.
  Every infrastructure workload/repetition requires paired actual-model
  observations. Each workload must satisfy its ceiling independently. Verified
  actual human review receipts are required for active review cost. Scripted
  #150 infrastructure observations are explicitly ineligible.
- Report includes per-gate pass/fail/pending, frozen plan, observed failed attempts,
  evidence digests, limitations, readiness and fresh-holdout follow-up. It never
  authorizes a holdout run or enables production compilation.
- Verified artifact proof cannot be supplied through JSON. It requires actual
  bytes for every referenced approval/custody/input-manifest/adjudication,
  raw/retained output, conformance and measurement artifact and binds the exact
  submission. CLI `-artifacts` resolves a bounded, contained receipt inventory;
  corpus/gold/model identities are excluded from those reads.
- CLI rejects duplicate keys, unknown fields, malformed UTF-8, trailing values,
  excessive depth/size, traversal and outside symlinks. Fully written synced
  output is hard-linked to a NEW path; existing reports cannot be overwritten.
- Current pending artifact contains only absent prerequisites. No fake plan,
  thresholds, histories, gold meanings, model output or human observations were
  produced. Unit examples are explicitly fabricated metadata/counts only.

## Explicit limits

This is independently useful evaluation infrastructure, not the completed final
experiment. Artifact hashing establishes byte identity. Truthfulness of a
custodian/human attestation, correct semantic adjudication, narrative independence,
and accuracy of external normalized measurement/test records remain trust
boundaries; the tool cannot prove those from a hash. It does not automatically
normalize or promote the current #149/#150 engineering receipts into final-model
observations. A later authorized custodian must supply the frozen final evidence.

The command has no model/network/activation/corpus-loader path. Stage 5 retrieval
and production-answer panels stay not_populated. No readiness, owner-review
quality, model selection, final holdout or ticket acceptance is claimed.

## Verification

`151-owner-verification.json` / `.log`:

- `go test ./internal/memoryeval ./scripts/memory-stage4-release -count=1`: PASS.
- `go test -race ./internal/memoryeval ./scripts/memory-stage4-release -count=1`: PASS.
- `go vet ./internal/memoryeval ./scripts/memory-stage4-release`: PASS.
- Both README local links checked: PASS.

`151-command-smoke.json`: built command emits exact committed pending-report
bytes, exit 2; reusing its destination exits 1 with original bytes unchanged.
The `go run` demonstration correctly wraps the command's exit 2 as shell exit 1;
this is an expected pending result, not a failed implementation check.

Root must run the required isolated `./scripts/verify-change.sh` and two independent
review axes before the one engineering commit. No UI code changed, so no new
manual UI behavior or actual model experiment is required for this tooling slice.
Review entry points: stage4_release.go, stage4_release_evidence.go,
stage4_release_quality.go, stage4_release_gates.go, then command artifacts/main
and the public-seam tests. Full experimental acceptance remains pending openly.

## Final review fixes, 2026-09-05

The final owner freeze now contains 14 new files, including the new
stage4_release_observations.go and pending-report-v2.json. The original manifest,
frozen bytes, verification and pending report remain preserved. Use the current
151-frozen-manifest.json/151-frozen-files for root's final isolated checkpoint.

Both initial Spec findings and both initial Standards findings were corrected:

- Every window in one history must retain one narrative family. A regression
  rebinding all plan/approval/custody hashes rejects one history under two families;
  a positive test permits several windows in one family without adding clusters.
- Required reported coverage now includes 27 timing/resource/backlog/throughput/
  review metrics, separately from the nine mandatory infrastructure numeric gates.
  New metrics may receive explicit pilot-chosen gates; none are automatically
  assigned thresholds. Reports retain raw sample arrays, per-workload distributions,
  observation counts, ratio numerators/denominators, unavailable pairs, review
  outcomes, exact config/environment/workload/source bindings and limitations.
- Actual accept/edit/reject/defer dispositions and active/elapsed/usefulness counts
  are required. Their counts must agree with the review cost/throughput denominators.
- Each required workload/repetition must have available observations. The narrow
  Standards follow-up found that one zero denominator could hide behind a second
  repetition; the correction retains unavailable-pair counts and keeps both
  reported coverage and any numerical gate pending. A regression reproduces the
  exact one-unavailable report-only throughput case.
- The artifact index and every receipt are read through one os.OpenRoot descriptor.
  A test renames/replaces the directory after opening; reads stay in the original
  descriptor root and a new outside symlink is rejected.
- Immutable report publication fsyncs the directory after linking and propagates
  sync/close errors while preserving a report that has already been published.

All verification began only after owner #150 confirmed that all 99 measured
trials, final source/digest checks and disposable-database cleanup had finished.
151-review-fix-verification.json/log records:

- `go test ./internal/memoryeval ./scripts/memory-stage4-release -count=1`: PASS
  (memoryeval 0.591s; command 0.180s).
- `go test -race ./internal/memoryeval ./scripts/memory-stage4-release -count=1`:
  PASS (memoryeval 3.171s; command 1.322s).
- `go vet ./internal/memoryeval ./scripts/memory-stage4-release`: PASS.

151-command-smoke-v2.json records a built-command pending exit 2, immutable
retry rejection exit 1, identical repeat output, valid local links and preservation
of the initial pending report. Final pending report SHA-256:
8a91b00cf9473c4c94c7c23eabba169237e319a47bca09df02a8c8beb3e73801.

Root's initial full verification passed before these fixes. A final isolated full
verification and final independent review checks remain required. No currently
running owner build/test process remains. Experimental ticket acceptance is still
pending; neither report claims a final holdout run or readiness.

Final independent Standards and fresh Spec-only reviews both PASS on all 14 frozen
files. Reports: 151-standards-review.md and151-spec-review-final.md. Verified manifest
SHA-256:6c2d2687a28377c2da5ed9aa143b208878de14d7233dea391657fab2ced17fcc.
Root final isolated full verification remains the outstanding engineering check.
No experimental gate has been cleared by these code reviews.
