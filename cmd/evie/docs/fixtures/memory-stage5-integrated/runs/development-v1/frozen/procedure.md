# Integrated development and held-out evaluation procedure

This procedure is frozen with the executable, source archive, numerical gates,
workload, reader rubric, semantic assessment schema, evidence auditor, quality
grader and resource auditor before the first integrated reader generation.
All commands write new destinations. Preserve every failed attempt, including
setup failures. No successful-only filtering or best-of-N replacement is allowed.

## Development

Prepare one canonical SQLite database and public source map per declared case.
All six conditions clone those exact inputs; the question, source identities and
gold labels stay identical. The selected local embedding model may index the
source corpus during preparation. Preparation never generates a reader answer.
Seal and hash the complete inputs and compiled executable before evaluation.

Run the frozen local, index, operating and real-reader entry points serially on
the declared machine without concurrent build or performance workloads. Record
actual commands, start/end times, exit status and all artifacts. Local measurement
uses twenty fresh-clone repetitions per case and condition; the reader uses one.
The first observed sample is not guaranteed cold. Conditions rotate by case.
The index and operating procedures define their independent timing boundaries.

The evidence auditor checks exact retained HTTP bodies against persisted request
hashes, source maps, locators and authorization metadata. It measures first,
final and union support independently from answers. A successful HTTP response
shows that a request was submitted; it does not establish which source caused an
answer. Preserve every planned denominator when an attempt or assessment fails.

Assess all final answers under the frozen semantic rubric and schema. An agent
assessor must be explicitly identified as an agent. Split cases among assessors;
retain exact answer spans, source/event IDs, actual-context spans, notes and any
second review. Automation validates these assessments and computes arithmetic;
it cannot replace semantic judgment with keyword matches. Publish ablations and
oracle results alongside production quality. Missing assessments block quality.

Compare every mandatory boundary and resource gate separately. A quality average
cannot excuse a source, scope, authority, lifecycle or receipt violation. Report
failures and limitations even if another condition passes. A failing development
pilot keeps readiness blocked. Corrections require a new version and freeze;
preserve earlier failures and rerun affected checks under the unchanged gates.
Commit the verified #167 implementation and pilot before the release assessment.

## Fresh held-out corpus

After the development configuration is frozen and #167 is verified and committed,
a separate curator receives the frozen procedure, fixture schema, semantic rubric
and behavioral obligations. The curator must not inspect development reader
answers, development retrieval traces, prior component held-out answers or any
release outputs. Record the actual curator identity and context restrictions.

Author twenty-four new cases, two per frozen behavior family, with independently
written original statements, people, topics and sessions. Do not rename or
paraphrase development cases, copy their source/answer strings, transform prior
component held-out examples, or select cases based on measured retrieval success.
Preserve the same observable obligations: fresh preference, uncompiled original,
clear and ambiguous references with/without compaction, accepted identity mapping,
relationship support, bounded neighbors, historical/current lifecycle, explicit
conflict and tentative updates, speaker attribution, unanswerable questions,
Memory unavailable, a general task and project/global boundaries.

Each case declares one minimal sufficient support set (possibly empty), acceptable
context, forbidden current sources/text, expected semantic components, intent and
validity where needed. Keep the same oracle bounds of at most two accepted items
and two conversation items. Declare at least two material-clarification cases,
two clear-reference cases and fifteen critical-semantic cases using gate_roles.
The required demonstrations retain critical roles; original-source inspection
following correction/restart also remains a deterministic operating obligation.
Declare all labels before any release output. Check only schema completeness,
source consistency and fixture preparation at this stage, not model performance.
Seal the corpus with a timestamp and SHA256 before evaluation; record any setup
failure. A content error discovered after release outputs cannot be silently
repaired and rescored as the same held-out assessment.

## Release assessment

Use seal-release to prepare the new corpus with the exact development executable,
source snapshot, reader/model identities, gates, ranking, budgets, rubric and
scoring programs. Only independently authored source and question inputs change.
The binary hash must remain identical. Run the same four measured entry points,
full deterministic boundary matrix and required repository verification. Assess
all 144 final answers using the same schema and rubric. Publish evidence quality
and reader quality separately, including every ablation and oracle comparison.

Never change the corpus, configuration, metric definitions or thresholds in
response to held-out results. Declare readiness only if every required quality,
boundary, resource, recovery and repository check passes. Otherwise report the
exact failed gates and follow-up work with readiness false. A future correction
requires a separately authorized fresh assessment; these outputs are then known
regression material. #168 records the honest assessment, not a deployment.

## Retention and reproduction

Retain input/source archives and manifests, immutable configuration, exact model
and runtime identities, request/response bodies, original sources, receipts,
measurements, assessments, command logs and reports. Never include credentials.
Archive large JSON bodies losslessly and keep their SHA256 manifest. The compiled
binary may remain local when source and the exact build command/toolchain are
retained; a reproduced binary has its own observed hash and is never represented
as the original measured executable. Hardware-sensitive measurements must be
rerun and reported as new measurements. Document all warnings, skipped optional
checks and unavailable instrumentation without inventing values.
