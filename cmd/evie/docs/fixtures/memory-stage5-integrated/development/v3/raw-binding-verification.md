# Bind assessments to retained reader results

Before development v3, the resource validator checked hashes of the retained
reader files but trusted the derived evidence report and assessment packets. Its
fresh grader recomputation checked those mutable inputs against each other. It
did not establish that a packet's answer or working context came from the actual
reader, or that the reported source union came from the actual provider payloads.

A retained-v2 counterexample changed a copied `dev23_no_memory_needed-automatic`
packet by appending a sentence to its final answer. It refreshed that copy's
assessment answer/packet hashes and recomputed the frozen quality report. The
original reader answer stayed unchanged. The old frozen resource validator still
passed `evidence:all_boundaries_and_denominators` and
`quality:exact_recomputation`; it had no raw-packet rejection. Existing independent
v2 quality failures remained failures. This is evidence of an undetected packet
edit, not a claim that the failed v2 pilot passed all release gates.

The analyzer now exposes a read-only function:

```python
build_report(freeze, results, freeze_sha256=None)
# Returns {"report": ..., "results_manifest": ..., "packets": {filename: packet}}
```

It executes the existing source and provider-payload audit and reconstructs every
assessment input from retained result files. The existing CLI delegates to this
function and preserves the report/packet schemas, canonical manifest bytes and
refusal to overwrite previous output. The resource validator requires exact
reconstructed report equality, the complete expected packet filename set, and
exact equality of every packet body before semantic grading. It also captures
the packet hashes in its own input inventory. Rewriting an answer, working
context or source union cannot be legitimized by updating intermediate hashes.

The analyzer loads its source auditor directly from its adjacent frozen file.
A regression first demonstrated that an unrelated preloaded `audit_sources`
module could otherwise replace the supposedly frozen dependency. The corrected
loader ignores that cache entry and leaves it unchanged. No source-sufficiency
rule, semantic label, metric definition, threshold, Go behavior or reader input
changes in this fix.

## Verification

```sh
python3 -B cmd/evie/docs/fixtures/memory-stage5-integrated/development/v3/raw_binding_regressions.py \
  --freeze /private/tmp/evie-memory-stage5/integrated-development-v2/freeze.json \
  --evidence-report /tmp/evie-memory-stage5/integrated-development-v2-evidence/evidence-report.json
```

Seven controls passed in 4.596 s. They reconstruct all 144 unchanged retained
packets, reject edited answers despite refreshed superficial hashes, reject an
invented source union with matching packet and summary arithmetic, reject edited
provider context, reject missing/extra packets, prove dependency isolation from a
poisoned module cache, and preserve CLI output shape/overwrite refusal. Scratch
copies are removed; original retained result hashes are checked unchanged.

The controls aggregate only the raw-binding gates. Their pass is not semantic
assessment, a new reader measurement, a full resource result, or release
readiness. The source auditor used here is the separately documented current v3
correction; the frozen v2 audit and reader artifacts are not rewritten.

The prior resource, nullable-work and execution-order controls also passed, and
six citation/pure-resource-API controls passed in 0.039 s. Python syntax and
scoped whitespace checks passed. The exact failing/passing transcripts and
source hashes are in `raw-binding-red-green-record.json`. A fresh frozen v3
reader comparison and the complete gate report remain required.
