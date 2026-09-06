# Memory Stage 4 release assessment

This command implements the independently useful evaluation tooling for
[ticket #151](https://github.com/davidadel66/evie/issues/151). The final evaluation
is **not run**. There is no selected adequate model, frozen numerical release
plan, completed actual owner pilot, or created final holdout. The 24 approved
source/gold labels do not approve model outputs. See the committed
[pending assessment](../../cmd/evie/docs/fixtures/memory-stage4-release/v1/pending-report.json)
and [pilot preparation](../../cmd/evie/docs/fixtures/memory-stage4-pilot/v1/preparation.json).

The command assesses metadata and independently adjudicated observations. It
cannot invoke a model, open a corpus or gold file by identity, author a holdout,
or enable compilation. A passed report is evidence for a later owner decision;
`holdout_run_authorized` is always false. This engineering outcome does not close
#151 or replace its missing experimental acceptance.

## Reproduce the current pending assessment

From the repository root, choose a new output path:

```sh
go run ./scripts/memory-stage4-release \
  -input cmd/evie/docs/fixtures/memory-stage4-release/v1/pending-submission.json \
  -output /tmp/evie-memory-release-pending.json
```

The compiled command exits 2 for a valid pending or failed assessment, 0 for a
passed assessment, and 1 for invalid input or publication failure. `go run`
wraps a nonzero program exit and prints `exit status 2`; use a built binary when
an automation must distinguish 1 from 2. Reports are written and synced before
being linked to a new destination. An existing report is never overwritten.
No current receipt asserts a final run or numerical gate result.

## Frozen metadata contract

`internal/memoryeval/stage4_release.go` defines the closed v1 submission and
plan contracts. Unknown JSON fields, duplicate keys, invalid UTF-8, trailing
values and excessive nesting are rejected. The command accepts at most 16 MiB
of submission metadata. No source or gold meaning appears in that contract.

A separately authorized custodian must first supply and preserve:

- The exact candidate and baseline generation, model artifact, runtime, prompt,
  schema, decoding and evidence-policy hashes; source tree and environment;
  rubric, workload, corpus and gold identities; explicit workload limits.
- Complete-history case metadata with narrative-family assignments and required
  memory counts, at least two repetitions, and a fixed workload list. The
  Cartesian product of case, repetition and candidate/baseline role declares
  every run in advance. The source/gold data stays with the custodian.
- Actual human model-selection, gold, integrated-pilot and release-gate approvals
  bound to their exact subjects and recorded no later than freeze. Gate approval
  binds the canonical plan digest. A proposed approval is not accepted.
- Independent custody reviewed before freeze, excluding the original N01–N12
  development/pilot families and all other exposed families. Relabeling or
  paraphrasing an existing family does not establish independence; this remains
  an auditable curator obligation, not something a hash can prove.
- Actual pilot-derived numerical gates. There are no supplied thresholds. Both
  raw and retained populations require positive supported-useful precision and
  required-memory recall floors, plus identity, temporal, source-attribution,
  unwanted-proposal and failed-run ceilings. A zero recall gate is invalid.
  Infrastructure gates fix statistic, sample minimum, candidate or paired-delta
  comparison and ceiling. Their metric names include units explicitly.

Use `memoryeval.Stage4Digest(plan)` to obtain the canonical Go JSON SHA-256;
the report also carries the exact original submission-byte hash. Freeze the
plan, approval and custody records before a separate human-authorized final
run. This tool cannot turn a passing prerequisite check into that authorization.

## Recording the final campaign later

The custodian logs one frozen campaign exposure before any baseline or candidate
attempt begins. Every planned attempt, including timeout, malformed output,
interruption and zero-candidate success, remains recorded. Each has configuration,
start/end times and immutable raw/retained output hashes. Missing planned runs
stay pending; duplicates and unplanned retries fail. A prior or second exposure
cannot be relabeled as an untouched holdout.

Adjudication stays outside extractor input. The execution attests that its input
manifest contains source evidence only and binds the approved corpus, scoring
artifact, actual adjudicator and timestamp. Proposal labels use the frozen spike
scorer's `required_useful`, `optional_useful`, `unsupported` and
`unwanted_but_true` vocabulary. Required matches reference opaque zero-based gold
identities within a case. A supported-useful label cannot conceal identity,
temporal or source-attribution errors. Raw proposals are counted by occurrence;
required matches are deduplicated within each run. Validation losses stay visible
in the separate retained panel. Failed runs retain their required-memory
opportunities in the recall denominator and their failure slice in the report.

The report preserves counts and denominators, candidate/baseline deltas, and a
paired narrative-family cluster bootstrap interval (1,000 resamples, fixed seed).
Variants and repeated runs stay in the same cluster. The interval is descriptive;
a tiny curated sample is not a population guarantee. Zero denominators remain
null. Incomplete paired runs do not produce a purported paired delta.

Scope, authority, source-binding, persistence and replay each require a positive
checked test count, command and immutable receipt from the frozen configuration.
Any failure, error or skip blocks readiness independently of quality averages.
The actual required repository and integrated checks must be retained in those
receipts; the evaluator cannot determine whether an external report lied about
what its command ran. Stage 3 conformance machinery and its gates are unchanged.
Retrieval and production-answer panels remain explicitly unpopulated for Stage 5.

Every declared infrastructure workload/repetition requires paired observations.
Each workload must satisfy its ceiling independently; small workloads cannot
average away a failing large workload. Final observations must identify an actual
integrated model. Active review requires separately verified useful accepted-change
receipts. Self-attested timer output, scripted decisions and approval rate cannot
stand in for this evidence. The #150 `memory-stage4-pilot-matrix-v1` and
`memory-stage4-pilot-infrastructure-v1` outputs currently identify
`scripted_infrastructure_only` and `release_eligible:false`; their engineering
measurements must never be normalized into `actual_integrated_model` observations.

## Checking artifact bytes

A submission with complete-looking metadata still cannot pass until every
referenced receipt and raw/retained output artifact is hashed. Provide
`-artifacts path/to/index.json`, containing an array such as:

```json
[{"sha256":"sha256:<64 lowercase hex characters>","path":"receipts/pilot.json"}]
```

The inventory must contain exactly `memoryeval.Stage4RequiredEvidence(submission)`:
approval evidence, custody record, input manifest, adjudication, raw and retained
outputs, conformance receipts and measurements. Each file is relative to the
index directory; traversal and symlinks outside it are rejected. Corpus, gold and
model artifact identities are prohibited from this receipt reader. Files are
limited to 16 MiB each and 64 MiB total. The exact files and their hashes remain
part of the auditable final evidence. An integrity proof is bound to the whole
submission and cannot be reused after changing a threshold or observation.

This checks bytes and consistency. It does not establish human identity,
semantic truth, independence of a history, or authenticity of a model run.
Those remain explicit human/custodian trust boundaries. Invented attestation
metadata is not evidence of a completed experiment.

Preserve the original failed report and all exposed attempts. If their results
inform tuning, move those narrative families into development/regression and
require freshly curated final material. Neither this tool nor a failed gate
permits changing the baseline, denominator or threshold in the evaluated result.
