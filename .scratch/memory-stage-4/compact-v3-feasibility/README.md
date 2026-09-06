# Request-specific source/context schema feasibility

This is an offline design result only. No compact-v2 artifact, raw output,
review label, source/gold, production code or configured experiment was changed.
No server, generation, model load, download or upgrade ran.

A separate compact-v3 is technically feasible for source-category membership:

- Derive permitted supporting aliases from the sealed selected support fields,
  including new and overlap. Put that list into the `ref` enum of each existing
  closed supporting-reference alternative.
- Derive permitted context aliases exclusively from sealed assistant context.
  Put that list in each context-reference alternative when nonempty.
- When no assistant field is offered, constrain context with
  `{"const":[],"type":"array"}`. Keep the field required. Do not use an empty
  enum and do not rely on `maxItems:0` without `items`.
- Keep the source input, source ordering, roles, scope, authority, window seal,
  raw denominator, selector/range protocol and strict adapter unchanged. Build
  the emitted schema and its embedded prompt text from the same derived value.
  No gold or meaning judgment participates in this derivation.

The unchanged pinned Ollama0.6.3 converter/parser proof binary compiled all ten
request-specific schemas. All **177 offline ASCII grammar checks passed**.
Every offered support alias and offered assistant alias still has the original
whole/date/range shape alternatives; cross-category aliases, invented aliases,
nonempty context without assistant fields and dangling references are rejected.

The largest exact rendered-byte upper bound is **7,750/8,192**, including the
unchanged 768 output cap and 64 reserve, at N03-b with its one assistant field.
Headroom is 442. Across the same ten windows, generated grammar sizes are
4,813–6,750 bytes, below Ollama's 32,768-byte buffer. Most prompts shrink because
an empty context constant replaces the full unused reference subtree. These
are exact source-derived bounds for this ten-case plan; larger or different
inputs still need normal full-batch proof and rejection before dispatch.

The pinned converter handles `enum` and `const` before object/array branches.
An actual regression probe confirms that `{ "type":"array", "maxItems":0 }`
without `items` accepts `[1]` in this runtime, while `const:[]` rejects it and
accepts `[]`. This is a runtime grammar limitation, not JSON Schema semantics.
The resulting grammar still constrains property order to schema order, as in v2.
Source coordinates and Unicode semantics remain adapter responsibilities; these
ASCII grammar tests are not new Unicode or model-quality measurements.

The three v2 first `invalid_subject` rejections are also protocol combinations:

| Observed output | Why invalid | What shape enforcement would require |
| --- | --- | --- |
| N01-b owner/text preference and habit, identity unresolved (two objects) | Owner/text has no unresolved Entity; identity must be resolved. | Correlate subject type, object kind and identity. A global resolved enum would incorrectly forbid source-supported new Entities. |
| N06-a object_kind entity, object Acme, no accepted identity aliases offered | Entity objects must be an offered accepted alias or `new:actual-name` with unresolved identity. | Correlate object kind/value and identity; preserve both new and accepted Entity alternatives. |

Those first errors are structurally expressible, but the raw outputs also have
meaning errors: one meal becomes a habit, tea repeats overlap only, and Acme
employment is affirmed after departure. Fixing cross-field identity shape does
not fix those meanings, the wrong owner subject for Maya, lost comparison or
clock/coffee meaning. The source/context-only proposal does not eliminate the
three identity failures or prove improved retained/semantic quality. No old
output is modified to estimate a new retained count.

A faithful identity schema needs a separate complete cross-field union or a
more compact subject/object protocol. The pinned converter's `allOf` branch
flattens properties and does not implement general conditional intersection;
adding `if/then`, `dependentSchemas` or an `allOf` condition without another
actual-runtime proof would be unsafe. Full candidate duplication increases the
embedded-schema byte cost. No combined identity schema is claimed to fit here.

If root chooses the category-only experiment, implementation must add a new
pinned schema-derivation version, base-template identity, per-request schema and
system hashes, reconstruction during offline scoring, public CLI tampering and
category regression tests, current artifact/API preflight and final reviewed
predispatch plan. Existing v1/v2 behavior and artifacts stay unchanged. The
same ten cases at one pass with seed17/temperature0/context8192/output768/60s
would remain a small descriptive development experiment, with no adequacy or
selection claim and no implicit authorization for broader runs.

Reproduce with the existing pinned source-proof binary:

```sh
python3 .scratch/memory-stage-4/compact-v3-feasibility/analyze.py
```

`summary.json` records per-case schema/system/input/full-rendering/grammar sizes,
source/context aliases, passed-case counts and the exact proof binary/schema
hashes. Per-case schema, vectors, generated grammar and acceptance output are
saved alongside it. The proof binary's source provenance and reproduction are
already frozen in compact-v2's `grammar-proof` package.
