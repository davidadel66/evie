# Reproduce the integrated evaluation

The exact observed commands, toolchain, machine, model identities and hashes
belong to each retained run. This file describes the command interface; it does
not establish a passing run. Use the immutable frozen programs for an existing
assessment. A fresh execution is a new measurement with its own artifacts.

## Prerequisites

Use the recorded Go toolchain and repository source archive, the selected local
MiniLM manifest/blobs and Ollama executable, and an authorized production reader
credential supplied through the existing environment. Credentials must never be
written to an artifact. The literal loopback embedding endpoint must already be
running with the selected model; the runner performs no model download or
fallback. Changed reader metadata, local model identity or runtime hash fails
preflight. Do not silently replace an unavailable frozen model.

From the repository root, run `python3 -B
cmd/evie/docs/fixtures/memory-stage5-integrated/run.py freeze --help` for the
complete freeze interface. Supply the workload, adopted gates, rubric, operating
matrix, evaluation/index/operating procedures, evidence scorer/source auditor,
semantic grader/assessment schema and resource auditor. Choose a new absolute
output directory. `freeze` copies the  compiled Go inputs, builds the executable,
checks production metadata, prepares canonical SQLite sources without reader
generation, and writes `freeze.json` only after recording all input hashes.
Preserve a failed destination rather than reusing it.

## Measured entry points

Use the same frozen input file and a distinct new result directory for each mode:

```sh
python3 -B /absolute/frozen/run.py \
  run --freeze /absolute/frozen/freeze.json --mode index --output /absolute/run/index
python3 -B /absolute/frozen/run.py \
  run --freeze /absolute/frozen/freeze.json --mode local --output /absolute/run/local
python3 -B /absolute/frozen/run.py \
  run --freeze /absolute/frozen/freeze.json --mode operating --output /absolute/run/operating
python3 -B /absolute/frozen/run.py \
  run --freeze /absolute/frozen/freeze.json --mode reader --output /absolute/run/reader
```

Run index immediately after the mandatory canonical source preparation, then
local, operating and reader, serially without concurrent build or performance
workloads. The runner records the actual model state; it does not add warmup
queries or select a replacement for a failed first attempt. The runner
retains start/finish records and exit status, rejects skipped/missing test entry
points, and verifies canonical inputs after each attempt. Reader failures remain
in the cohort; they do not authorize a best-of-N retry. The local and operating
providers are scripted and establish no answer-quality measurement.

## Audit and semantic assessment

```sh
python3 -B /absolute/frozen/scorer.py \
  --freeze /absolute/frozen/freeze.json --results /absolute/run/reader \
  --output /absolute/run/evidence
```

Read every generated assessment packet under the exact frozen rubric/schema.
Write explicit semantic assessments to a new directory. Record the actual
assessor and any second review; agent assessment is not human review. Then run:

```sh
python3 -B /absolute/frozen/grader.py \
  --freeze /absolute/frozen/freeze.json \
  --evidence-report /absolute/run/evidence/evidence-report.json \
  --assessments /absolute/run/assessments --output /absolute/run/quality
```

Run the exact mapped Go and UI tests and `./scripts/verify-change.sh`. Retain the
commands, structured Go/Vitest outputs, logs, exit codes and hashes in the
resource auditor's deterministic attestation. Its `--help` describes required
paths. Feed the evidence/quality reports, local/index/operating result directories
and attestation to the frozen `resources.py`, using a new output directory. It
independently rebuilds the evidence report and every assessment packet from the
retained raw reader results before recomputing semantic arithmetic and integrity
checks; missing or edited intermediate evidence cannot
produce a passing gate. A development gate pass still has `release_ready: false`.

## Release inputs

Only after #167 is committed and its exact development report passes all required
gates, use `seal-release --help` to prepare the separately authored held-out
workload under the unchanged executable, model, scripts, thresholds and budgets.
Supply the exact development freeze and report. Sealing verifies every retained
report input hash and recomputes the report through the frozen resource auditor;
a summary pass flag alone is insufficient. Every compiled source path must also
match the committed #167 bytes before release inputs are created. The recorded
source commit is resolved once and verified independently of later HEAD movement.
The evaluation procedure defines
curator separation, source-label sealing and the prohibition on tuning against
held-out outputs. Repeat the same measured and deterministic commands, preserving
all failures. Final review and the user-facing readiness declaration remain
required after these numerical checks.

Large raw outputs may be archived losslessly with an exact manifest. Extract them
before rerunning an auditor; do not modify file content, original hashes or
measured timestamps to match a new location. Original absolute paths document
the observed run. For reproduction on another machine, prepare a new freeze with
its own paths and observed hashes rather than claiming it is the measured binary.
