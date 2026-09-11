# Retained integrated runs

Each run preserves its original observed paths, timestamps, input identities and
failed attempts. It must not be relabeled as a later result.

`development-v1/` is a failed development pilot. It contains the immutable
configuration, scorers, source and canonical-input archives, full lossless local,
index and operating traces, partial resource report, verification logs and both
cold/warm and corrected-context diagnostics. Its 466 original files were copied
and independently hash-verified. The 42 preserved payload files occupy 12,236,338
bytes including the preservation plan; a small final manifest is additional.

No integrated reader generation ran in v1: 320 of 2,880 local turns and eight
of 24 index cases failed. All 60 operating samples passed. The corrected-context
single-case replay and the warm index diagnostic remain supplemental observations,
not replacements for those failed cohorts. Failed-turn final runtime work is
unknown; a v1 zero/earlier-receipt value is not a measured complete-turn total.

`development-v2/` is also a failed development pilot, retained unchanged. Its
[freeze](development-v2/frozen/freeze.json) was written at
2026-09-11 04:26:38.186911 UTC. The source HEAD **at freeze** was
`fbabb1fec70fdc8f1b338a5aba05d5343d774f9b`; this historical identifier may change
when issue-owned fixes are folded. The archived compilation inputs identify the
measured source independently of the final branch history.

| Frozen artifact | SHA-256 |
| --- | --- |
| `freeze.json` | `d1ee307d4afd81270a8cbceaeaf0a61269b9a77a18a240beb6478e5fa90c1080` |
| Compiled test executable, excluded from payload | `f80a23aeb1c060a851f19962b2373abec547322247f764f2d423f50e22a41797` |
| `compiled-source.tar.gz` | `51c0b7e6ba7e4075db8ab041413ff2e3cc24c7675db79d2fc97436f7d6950446` |
| `prepared-inputs.tar.gz` | `49780997a68713f22ccae7ff18ddef9b8da1c12ad1e0c734ed403077e24d8dc4` |
| Workload | `2e1aff23f8426b45b6a1cc3301c769026ecb84cc55a9d4f03ba922eff333d562` |
| Adopted gates | `06aaf6f4bff1486a78cef71e88dffeec54ae529f23c322fd8ec5d1bc04c928fd` |
| Reader rubric | `a64da1e7cf6219862c478c24dd8dd1172a4cec7073e55a2508cbf6e7e4bcc813` |

V2 completed 144 reader turns across 24 cases and six conditions, with 181 actual
reader requests and zero execution errors. The retained 183 raw HTTP responses
include those 181 reader responses and two metadata responses. Reader execution
ran from 04:29:16.569264 to 04:37:31.684761 UTC on 11 September 2026. All 2,880
local turns (20 per case and condition), 24 index cases and 60 operating samples
passed their execution checks. The [deterministic record](development-v2/reports/deterministic.json)
retains the successful full verification and focused checks. The frozen
[source audit](development-v2/reports/evidence-report.json) reports zero source
boundary violations.

Explicit agent assessments, without implied human review, marked 136 of 144
answers semantically passing. Three failures were incomplete answers in
comparison conditions: automatic-only missed Omar's snack in `dev11` and the
historical venue in `dev15`; tool-only missed the original travel-support source
in `dev12`. These gaps remain measured comparison results. Five other answers
had real quotation errors under the unchanged exact-byte rubric: `dev13` oracle
and tool-only capitalized the original lowercase instruction; `dev14` automatic
and oracle changed punctuation inside a quotation; `dev17` automatic inserted
Markdown emphasis inside quoted source text. Correct overall meaning does not
remove those quote failures.

Separately, the frozen evaluator rejects 13 assessments with 137 validation
messages because it requires query-specific identity markers or exact oracle
read intent/time pins even where the actual supplied sources support the manual
judgment. The original packets, explicit judgments and disagreements are all
retained; no judgment was falsified to satisfy the evaluator. The
[quality report](development-v2/reports/quality-report.json) and
[resource report](development-v2/reports/resource-report.json) both remain
non-passing. The latter records 1,065 of 1,074 gates passing, eight failing and
one incomplete. All nine non-passing entries concern quality; `release_ready`
remains false. This development pilot provides no held-out release evidence.

V2 preserves 2,102 original files in 66 repository files totaling 16,327,656 bytes.
The [preservation manifest](development-v2/preservation-manifest.json) accounts
for 16,317,363 payload bytes; the manifest itself is additional. The source and
input archives contain 393 compilation inputs and 48 canonical seed files.
Separate lossless archives retain all reader requests/responses/events,
144 assessment packets, 144 final assessments, local/index/operating traces and
verification logs. Copied bytes, every archive member and original modification
times were checked against the originals. The frozen programs and failed reports
are preserved alongside their hashes and actual execution commands.

From the repository root, verify the retained v2 payload without models or
source changes:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v2/preserve-integrated-v2.py verify \
  --directory cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v2
```

The verifier must report 2,102 verified originals and 66 output files. Keep new
notes outside each preserved directory: the exact manifest intentionally rejects
unexpected files. V1 and v2 are distinct immutable observations; later corrections
require a new freeze and new outputs.

`development-v3/` is a passing development pilot, preserved after its final
quality and resource audits closed. Its [freeze](development-v3/frozen/freeze.json)
was written at 2026-09-11 05:10:54.897313 UTC. The source HEAD **at freeze** was
`7381eead7f24962cea3a3ee7433acdd41e91f871`; this historical identifier is separate
from the final issue-folded branch history. The exact compiled-source archive
remains the reproducible source identity.

| Frozen artifact | SHA-256 |
| --- | --- |
| `freeze.json` | `932e95f04123c5935b432a47f8980db8629e62aa0a1cb056f82be35949bfdba0` |
| Compiled test executable, excluded from payload | `15e4d28e71df4cc3638890a3509675be0e192d850b69fa41f836f2f83569e6cb` |
| `compiled-source.tar.gz` | `3330f5b51729194a9fe4e68d4e3b6786507248d34129dd7342f7e8d886ef7fb1` |
| `prepared-inputs.tar.gz` | `f66f0597d78ef06da148d1766eaa45881c3c30cdc484d3f7e0176170f9a85911` |
| Workload | `2e1aff23f8426b45b6a1cc3301c769026ecb84cc55a9d4f03ba922eff333d562` |
| Adopted gates | `bf730615b47e3acb2f9fc12fe57a1dad88d19169ce46967716ec6fde144b1ea9` |
| Reader rubric | `a64da1e7cf6219862c478c24dd8dd1172a4cec7073e55a2508cbf6e7e4bcc813` |

V3 completed all 144 reader turns with 178 actual reader calls and dispatches,
zero execution errors, and 180 retained raw HTTP responses including two metadata
responses. Reader execution ran from 05:13:34.046059 to 05:21:50.398019 UTC on
11 September 2026. All 2,880 local turns, 24 index cases and 60 operating samples
completed successfully. The [deterministic record](development-v3/reports/deterministic.json)
retains the full verification and mapped checks, including equality between the
verified checkout and archived compilation inputs. The
[source audit](development-v3/reports/evidence-report.json) reports zero boundary
violations across all 144 complete assessment packets.

Fresh explicit agent assessments marked 142 of 144 answers semantically passing;
all 144 assessments passed the frozen validator without evaluator conflicts.
The two remaining incomplete answers are automatic-only comparisons: `dev11`
truthfully lacks Omar's snack preference, and `dev15` truthfully lacks the
historical venue. The production `automatic_deeper` condition and oracle each
pass all 24 cases, with 37/37 supported answer components, 47/47 grounded personal
propositions and 28/28 correct citations, including one additional citation.
All hard violation counts are zero. Baseline abstention retains its missing
components and source coverage; no human review is implied.

The [quality report](development-v3/reports/quality-report.json) passes all adopted
quality gates. The [resource report](development-v3/reports/resource-report.json)
is complete with 1,077/1,077 gates passing and no failures. Both final audit
commands exited zero. `release_ready` remains false because this is development
evidence; it does not replace the separate held-out release evaluation. V1 and
v2 failures and all original v3 judgments remain unchanged.

V3 preserves 2,114 original entries in 72 repository files totaling 16,330,459
bytes. The [preservation manifest](development-v3/preservation-manifest.json)
accounts for 16,319,051 payload bytes; its own 11,408 bytes are additional.
The source and input archives retain 393 compilation inputs and 48 canonical
seed files. Lossless archives contain the full reader wire and encoded requests,
responses, events, 144 audited packets, 144 merged assessments, all local/index/
operating traces, verification logs, audit drivers, and assessor checks and notes.
Every copied file and archive member was verified by hash, original modification
time and mode. Eleven existing v3 regression programs, records and decisions are
referenced by repository-relative path and exact hash, avoiding duplicate copies.

From the repository root, verify the complete v3 preservation and its referenced
regression records without model calls:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v3/preserve-integrated-v3.py verify \
  --directory cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v3
```

The verifier must report 2,114 verified originals, 72 output files, 11 verified
repository records and `all_member_hashes_times_modes_verified: true`. Keep new
notes outside the preserved directory and preserve the referenced v3 records.

The original executables and duplicate unpacked source/input trees are excluded.
Executable hashes, exact compilation commands, compiled-source archives and input
archives remain available. Extract raw archives before auditing. Original
absolute paths document the observed run; use a new explicit freeze to reproduce
on a different machine or path. `preserve-integrated-v1.py verify --directory`
checks the exact preservation manifest and every archive member without model
calls or source mutations.

## Development v4: corrected cross-scope refresh

`development-v4/` preserves the verified correction of the #162 cross-scope
refresh finding discovered after v3. All 1,077 required gates pass. The new
[freeze](development-v4/frozen/freeze.json) was written at
2026-09-11 05:59:09.519646 UTC from source HEAD
`52d00bf5aa40c6323321253b54e66bf153de7ccd`. Its complete archive retains 394
compilation inputs, including the new complete-turn regression file, and 48
canonical seed files. Questions, numerical gates, rubric and scoring programs
remain unchanged from v3. The matrix now requires 91 Go test functions and the
same 14 UI cases; all 23 tests in the seven selected UI files passed.

The 144 reader turns made 179 actual calls with matching encoded and wire
requests and no turn errors. All 144 source packets passed the audit. All
assessments validated; 143 answers passed semantic assessment. The remaining
automatic-only historical comparison truthfully lacks the required historical
source. Production automatic-plus-deeper and oracle each pass 24/24 cases,
37/37 personal components, 48/48 grounded propositions and 28/28 citations.
The 55 source quotations are exact and all hard violation counts are zero.
All 2,880 local turns, 24 index cases, 60 operating samples and required
repository checks passed. These remain development results; release readiness
requires the separate held-out assessment.

| Artifact | SHA-256 |
| --- | --- |
| Freeze | `57a810541c66dbdc3ef09749b6ac786e360e0ab4c02473d529a9cf8f7c011a3f` |
| Measured executable, retained locally | `06ba5e56aecf5684f2209bd801074ea65ed032dc88181d07a4d016260532f072` |
| Quality report | `7c4893588b2ad4d7ace5925796fa9416be0ffbccc2649bccb4098e7d3db9f3fb` |
| Resource report | `30a2e068ded9637dc38a3a9ec015d756c7c5338f8fd0267a85ba378fdcfd8e43` |
| Assessment manifest | `e390b48ef58b33237ff66cbd083a08e175696a5a8c88c073eabe30b551a1d2f6` |

The [preservation manifest](development-v4/preservation-manifest.json) accounts
for 2,105 original entries in 73 files totaling 16,290,515 bytes, including its
own 11,571 bytes. Every copy and archive member was verified by hash, original
modification time and mode. Fifteen existing regression and decision records
are referenced by repository-relative path and hash. Raw artifacts include all
reader/source/assessment/local/index/operating results, closed execution logs,
cross-scope RED/GREEN traces and the owning-commit fold proof. Keep new notes
outside this immutable directory and retain the referenced records.

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v4/preserve-integrated-v4.py verify \
  --directory cmd/evie/docs/fixtures/memory-stage5-integrated/runs/development-v4
```

The verifier reports 2,105 verified originals, 73 output files, 15 repository
records and `all_member_hashes_times_modes_verified: true`, without model calls.

## Held-out v1 setup and v2 release assessment

`heldout-v1-failed-setup/` preserves the first held-out preparation failure:
22 of 24 cases prepared; two compacted-reference fixtures lacked a third
completed root required by the public compaction contract. There was no sealed
held-out freeze, reader call, index measurement or local/operating cohort in v1.
The independent curator appended one neutral completed exchange to each affected
fixture before evaluation. Removing those two additions reconstructs the entire
v1 workload byte for byte. Original wording, questions, gold and the other
22 cases stayed unchanged. Both versions and qualification records remain in
[`heldout/`](../heldout/).

`heldout-v2/` preserves the completed, **failed** release assessment. Its
[freeze](heldout-v2/frozen/freeze.json) was sealed at
2026-09-11 06:41:45.948626 UTC, SHA-256
`c6d6a2c8627580963fb7f2edb341173bd34487e09c80a3e9a9813f0fef169ed2`.
It reuses the exact development-v4 executable and all 394 compilation inputs.
No corpus, configuration, gate or scoring program changed after held-out output.
The original wrapper withheld reader execution after index/local failures;
the explicit first reader continuation is retained as a protocol deviation,
with no replacement attempt or waived gate.

The [resource report](heldout-v2/reports/resource-report.json) contains
**1,051 passing, 23 failing and three incomplete gates** out of 1,077.
All readiness flags are false. All 144 manual agent assessments are retained;
142 validate, with 14 errors across two truthful raw-source judgments rejected
by the frozen accepted-record binding. Validated semantic passes are 126/144;
raw manual passes are 128/144. These denominators are not interchangeable.
The [release assessment](../../../active/memory-stage-5-release-assessment.md)
separates retrieval coverage, answer quality, oracle behavior, resource costs,
annotation limitations and the actual silent-conflict-resolution error.

All 144 reader cases remain: 187 prepared requests, 184 actual wire/HTTP calls,
and three failed turns with requests blocked before dispatch. All 2,880 local
samples remain, including 20 failed historical-oracle samples with unknown final
runtime totals. Index checks pass in 23/24 cases; operating checks pass in all
60 samples. The historical gold also forbids its own required location text;
that diagnosed contradiction stays a failed gate. No failed sample was removed,
retried as a replacement, or converted to a successful measurement.

The exact [gate ledger](../heldout/v2/post-run-diagnostics/heldout-v2-final-gate-ledger.json)
contains every failure, threshold and validation message. Lossless archives
retain requests, responses, source packets, judgments, local/index/operating
traces and closed executions. `curation-provenance.tar.gz` also captures the
final repository checks and the separate disposable browser demonstration;
the readable originals are under [`heldout/v2/`](../heldout/v2/).
Those eight scripted UI/state demonstrations do not replace real-reader quality.

From the original preserved working tree, check all exact copied bytes,
archive members, original timestamps and modes without model calls:

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/runs/heldout-v2/preserve-integrated-heldout-v2.py verify \
  --directory cmd/evie/docs/fixtures/memory-stage5-integrated/runs/heldout-v2
```

The preservation manifest identifies exact inventories and hashes. Keep new
notes outside each immutable run directory. Git does not preserve direct-file
modification times; use the separately documented transport verification for a
fresh checkout, without presenting checkout times as observed run timestamps.
Future evaluator or runtime corrections need a separate fresh assessment;
these held-out cases are now known regression material.

The held-out v2 preservation verifier reports 2,438 verified original entries,
81 output files, 23 unchanged repository references and
`all_member_hashes_times_modes_verified: true`. Its payload is 19,012,435 bytes
before the final preservation manifest. The source and canonical input archives
are complete; compiled executables and model weights are explicitly excluded.

## Verifying a fresh Git checkout

The supplementary [transport verifier](transport-verification/verify-preserved-transport.py)
checks the exact file set, content hashes, archive-member metadata, frozen source
and seed manifests, and repository references. It reports direct-file checkout
mtime/mode differences without claiming those original values survived Git.
The immutable strict verifiers above remain useful against the original local
copies. Neither verification method reruns or reclassifies the experiment.

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/runs/transport-verification/verify-preserved-transport.py \
  --repository . \
  --directory cmd/evie/docs/fixtures/memory-stage5-integrated/runs/heldout-v2 \
  --output /tmp/evie-heldout-v2-transport-verification.json
```

Use a new output filename. The [observed held-out check](transport-verification/heldout-v2-transport-result.json)
passed all 81 retained files, 2,369 preservation-archive members, 394 source
inputs, 48 canonical seed/map inputs and 23 repository references. Controls
changed all 64 direct-file timestamps in a disposable development-v4 copy:
transport checking passed while the strict verifier correctly rejected the
metadata differences. A changed freeze byte was rejected. The
[reconstruction note](transport-verification/heldout-v2-transport-verification-note.md)
and exact control records explain the distinction and original build/run paths.
This does not claim bit-identical executable rebuilds or deterministic live
provider answers.
