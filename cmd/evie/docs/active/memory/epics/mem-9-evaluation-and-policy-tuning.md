# MEM-9 - Evaluation and policy tuning

Status: approved; not started

## Outcome

Measure extraction, entity resolution, lifecycle behavior, retrieval, grounding,
abstention, isolation, poisoning resistance, latency, and storage so policy
changes are decided from comparable versioned evidence.

## Specification references

- [Research Basis](../../memory.spec.md#research-basis)
- [Stage 9 - Memory evaluation and policy tuning](../../memory.spec.md#stage-9---memory-evaluation-and-policy-tuning)
- [Definition Of Done](../../memory.spec.md#definition-of-done)
- [Open Questions](../../memory.spec.md#open-questions)
- [Binding memory decisions](../../memory.decisions.md)

## In scope

- Redacted replayable Evie fixtures and a versioned evaluation manifest.
- Compiler/entity/lifecycle and retrieval/grounding/safety metrics.
- Latency, token, storage, and rebuild reporting.
- Comparable configuration runs and evidence-backed policy decisions.

## Out of scope

- Committing raw private event payloads.
- Tuning against unversioned anecdotes or changing policy without a recorded
  comparison.
- Assuming automatic admission is desirable.
- Hard-erasure evaluation or optional research-topic workspaces.

## Stories

### MEM-9.1 - Redacted corpus and evaluation contract

- Outcome: Create 10-20 replayable Evie task fixtures with fixed expectations,
  configurations, metric formulas, and pass thresholds.
- Depends on: MEM-8 and representative completed memory workflows.
- Acceptance summary: Fixtures contain no raw private event payloads; the
  manifest versions model/prompt/index configuration and expected
  entities/claims/transitions/retrievals; formulas and thresholds are explicit
  enough for independent reproduction.
- Verification summary: Redaction review, deterministic fixture validation,
  schema checks, expectation round trips, and documented commands.
- Proposed PR boundary: Corpus, manifest, metric definitions, and fixture loader;
  no policy tuning.

### MEM-9.2 - Compiler, entity, and lifecycle evaluation

- Outcome: Measure candidate write quality, entity-resolution errors, evidence
  validity, and temporal/lifecycle updates across fixed configurations.
- Depends on: MEM-9.1 and MEM-4.
- Acceptance summary: Reports include precision/recall, unsupported candidates,
  unsafe merge/split errors, correction/change/supersession/retraction/retirement
  accuracy, abstention/failure counts, and configuration identity.
- Verification summary: `go test -tags eval ./...` compiler/lifecycle cases,
  deterministic metric recomputation, malformed/timeout cases, and report golden
  checks.
- Proposed PR boundary: Compiler/entity/lifecycle evaluation runner and report;
  no extraction-policy change.

### MEM-9.3 - Retrieval, grounding, isolation, and poisoning evaluation

- Outcome: Measure whether retrieved evidence and final answers are relevant,
  sourced, scope-safe, temporally correct, and resistant to untrusted
  instructions.
- Depends on: MEM-9.1 and MEM-5.
- Acceptance summary: Cases cover temporal, multi-session, multi-hop, alias,
  exact-ID, promotion, correction, retirement, contradiction, abstention,
  cross-project isolation, remote-egress exclusions, and poisoning; reports
  retain configuration identity and expected source IDs.
- Verification summary: `go test -tags eval ./...` retrieval/answer cases,
  captured provider requests, deterministic recall/grounding calculations, and
  redaction checks.
- Proposed PR boundary: Retrieval/answer/safety evaluation runner and report; no
  ranking or graph-depth change.

### MEM-9.4 - Operational performance and comparison reporting

- Outcome: Produce comparable latency, token, storage-growth, memory, and
  index-rebuild measurements for fixed memory configurations.
- Depends on: MEM-9.1, MEM-9.2, and MEM-9.3.
- Acceptance summary: Reports include environment/configuration identity,
  ingestion and retrieval latency distributions, provider token use, database
  and index growth, cache memory, and full projection rebuild time; before/after
  runs use the same fixture manifest.
- Verification summary: Reproducible benchmark commands, report schema/golden
  checks, deterministic comparison calculations, and variance notes.
- Proposed PR boundary: Operational metrics and comparison reporter; no policy
  selection.

### MEM-9.R1 - Evidence-backed memory-policy decisions

- Outcome: Approve, reject, or defer changes to extraction, predicate
  normalization, graph depth, ranking, candidate admission, and optional
  Personalized PageRank using versioned evaluation evidence.
- Depends on: MEM-9.2, MEM-9.3, and MEM-9.4.
- Acceptance summary: Every decision cites a fixed before/after report and states
  safety/quality/performance tradeoffs; trusted-tool or automatic admission gets
  an explicit predicate/authority/confidence allowlist or remains disabled;
  project/session compilation defaults are recorded.
- Verification summary: Independent report recomputation, decision-to-manifest
  links, threshold checks, and `git diff --check` for the decision update.
- Proposed PR boundary: Decision records and, only when already approved as part
  of the selected story, the smallest policy/configuration change; larger
  behavior changes receive new stories.

### MEM-9.5 - Core memory end-to-end acceptance scenario

- Outcome: Own the specification's required scripted proof that the complete
  core memory system works across admission, scope, recovery, egress, replay,
  indexes, and concurrent processes.
- Depends on: MEM-9.R1 and completion of MEM-1 through MEM-8.
- Acceptance summary: One reproducible scenario creates and approves a candidate,
  proves global/project/session fences, races two processes, restarts, resolves
  an interrupted execution, captures the opted-in remote payload, drops and
  rebuilds graph/index projections, and compares the accepted snapshot without
  exposing private raw events.
- Verification summary: Run the scripted scenario from a clean temporary Evie
  home, `go test -tags eval ./...`, `go test ./...`, and `go vet ./...`; preserve
  redacted logs and deterministic snapshot comparisons as review evidence.
- Proposed PR boundary: End-to-end harness, redacted fixtures, deterministic
  assertions, and run documentation only; behavioral defects discovered by the
  scenario receive separate focused stories unless a minimal correction is
  explicitly approved.

## Epic completion evidence

- Changing a memory configuration produces a reproducible before/after report
  across write quality, lifecycle, retrieval, grounding, safety, latency, token,
  storage, and rebuild metrics.
- Model-backed evaluations run under `go test -tags eval ./...` without committed
  raw private payloads.
- Any policy change is traceable to versioned evidence; unproven automatic
  admission remains disabled.
- The core memory Definition of Done and scripted end-to-end scenario pass.

## Risks and open decisions

- Redaction must preserve evaluation meaning without leaking private event data.
- Model-backed metrics can vary; manifests and reports must pin configurations
  and disclose variance rather than hide it.
- Personalized PageRank and automatic admission are optional hypotheses, not
  promised outcomes.
- MEM-9.5 owns the required cross-epic scripted acceptance proof; it must not
  become a catch-all implementation PR for defects found during the run.

## Approval record

- 2026-08-21: David approved this epic and its story boundaries as part of the
  memory delivery initiative.
