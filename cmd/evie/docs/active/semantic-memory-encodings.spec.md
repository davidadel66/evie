# Semantic Memory canonical encodings

## Purpose and authority

This specification freezes the Stage 3 wire and equality encodings that must be
stable before semantic DDL. Storage schemas and Go types may have different
internal names, but they must preserve these values exactly and produce the same
fixture results.

The contract concretizes, without weakening, the active Stage 3 requirements and
the accepted memory decisions: semantic truth stays Kernel-owned; accepted
operations replay without model calls; world Claims remain separate from
structural Graph Links; lifecycle is append-only; polarity is separate from
Predicate cardinality; and every accepted mutation is atomic and source-linked.
The [Graphiti inspection](../research/graphiti-episode-persistence.md) is
evidence only.

## Scalar encodings

All persisted and fixture text is valid UTF-8. Tokens, IDs, hashes, timestamps,
and numeric encodings below are ASCII. A value that is semantically invalid or
not in canonical form is rejected rather than silently repaired.

### Stable IDs

Every semantic object, accepted operation, and Source Link uses a random UUIDv4.
The canonical form is 36 lowercase characters with hyphens,
`xxxxxxxx-xxxx-4xxx-[89ab]xxx-xxxxxxxxxxxx`. Production generates an ID exactly
once. The approved effect and accepted operation record every generated ID;
projection writes and replay consume those IDs and never generate replacements.
Fixtures use fixed UUIDv4-shaped values supplied by their `generated_ids` list.

Canonical owner, Evie, and Context Entity IDs obey the same rule. They are
created or resolved by a sourced operation and then remain stable; they are not
path hashes, names, or deterministic UUIDv5 values.

### Time

Transaction Time and datetime values use fixed-width UTC RFC 3339 text with
exactly nine fractional digits:

```text
2006-01-02T15:04:05.000000000Z
```

Inputs with an offset are converted to UTC and nanoseconds are retained. Leap
seconds and timezone-less inputs are rejected. One accepted operation assigns
one Transaction Time to all its effects. It is the later of the normalized
commit clock and the latest Transaction Time in every affected scope, so a clock
rollback cannot move accepted state backward. Scope Revision, not a wall-clock
tie, orders operations that share a Transaction Time.

A Valid Time is `{from,to}`, where either bound may be JSON `null`. Non-null
bounds use the datetime encoding above and the interval is half-open `[from,to)`.
When both bounds exist, `from < to` is required. `{null,null}` means the
proposition has no known world-time bound; it does not mean an empty interval.
The exact pair participates in Claim equality.

### Hashes and canonical operation JSON

A SHA-256 is encoded as `sha256:` followed by 64 lowercase hexadecimal digits.
Proposal and operation-effect hashes cover UTF-8 JSON with no insignificant
whitespace. The hashed schema is composed of ordered structs: fields appear in
schema order, arrays preserve contract-defined order, map-shaped values are not
allowed, exact numbers are strings, and absent optional values are emitted as
JSON `null`. This avoids dependence on generic map iteration or floating-point
serialization.

## Scope registry

A scope registry row contains one random `scope_id` and one unique canonical
`scope_key`. The backing registry identity is encoded exactly once in that key;
the global key has no backing identity. Semantic rows contain one non-null
`scope_id` foreign key and no nullable Workspace/project/session column
combination.

`scope_key` has exactly one of these forms:

```text
global
workspace:<workspace-uuid>
project:<project-uuid>
session:<session-uuid>
```

The referenced UUID is the stable identity in the corresponding canonical
registry. ASCII kind prefixes are lowercase and are never aliases or display
names. There is exactly one `global` row, one row for each registered Workspace
or project, and one row for each session. Workspace, project, and session scope
rows retain their registry reference even after archival. The registry resolves
the UUID embedded in `scope_key` through its kind-specific registry and rejects
an unknown kind, a missing backing identity, or a second scope row for the same
key.

Each scope owns a non-negative integer Scope Revision. The initial revision is
`0`; one accepted operation increments every written scope exactly once. Reads
do not increment it. A proposal carries the sorted vector of every scope it
reads or writes as `(scope_key, revision)`, ordered bytewise by `scope_key`.
Apply compares the complete vector atomically. Any mismatch rejects the whole
operation and changes no row or revision.

## Predicates and Typed Literals

### Predicate definitions

A Predicate token is 1 to 64 ASCII characters matching
`[a-z][a-z0-9]*(?:_[a-z0-9]+)*`. Whitespace, uppercase, punctuation, empty
segments, and Unicode lookalikes are rejected rather than normalized. The human
label is separate and does not participate in identity.

Definitions are global and append-only. Each definition has a random stable
`predicate_id`; `(token,version)` is its unique canonical key, where `version`
is a positive integer beginning at `1` and increasing by one for that token. A
definition fixes:

- `object_constraint`: `entity` or one literal kind;
- `cardinality`: `one` or `many`.

A Claim references the stable Predicate ID and records its exact token and
version for canonical inspection. A newer definition never reinterprets an
older Claim. Cardinality produces diagnostics; it is not a uniqueness rule and
does not encode negation.

### Typed Literals

A Typed Literal is encoded as `{"kind":"...","value":"..."}`. Kind and
canonical string value jointly define equality. The closed v1 kinds are:

| Kind | Canonical value |
| --- | --- |
| `text` | Valid UTF-8 preserved byte-for-byte. No trimming, case folding, or Unicode normalization occurs. Empty text is valid. |
| `integer` | Signed base-10 integer with no `+`, leading zero, or `-0`; `0` is canonical. Values are mathematically exact rather than JSON or SQLite floating-point numbers. |
| `decimal` | Signed base-10 decimal without exponent, leading zero, trailing fractional zero, or negative zero. `0` is canonical; a nonzero fractional form has at least one digit on both sides of `.`. |
| `boolean` | Exactly `true` or `false`. |
| `date` | A real proleptic-Gregorian date in `YYYY-MM-DD` form. Date precision is retained and is not converted to midnight. |
| `datetime` | The fixed-width UTC nanosecond datetime encoding defined above. |

Floats, exponent notation, arbitrary JSON, money, duration, quantity, implicit
coercion, and locale-dependent formatting are rejected in v1.

## Claim and Source Link equality

A Claim object is either `{"entity_id":"<uuid>"}` or
`{"literal":{"kind":"...","value":"..."}}`; exactly one form is present.
Polarity is exactly `affirmed` or `denied`.

Claim proposition equality is the tuple:

```text
(scope_id, subject_entity_id, predicate_id,
 object_kind, canonical_object_value, polarity, valid_from, valid_to)
```

For an Entity object, `canonical_object_value` is its stable Entity ID. For a
literal, it is `(kind,value)`. Source evidence, lifecycle state, operation ID,
Transaction Time, and Scope Revision are excluded. An independently proposed
equivalent proposition reuses the Claim and may attach new eligible evidence.
It is not an idempotent retry.

A Source Link has its own random stable ID. Its natural equality tuple is:

```text
(claim_id, event_id, event_part, locator_kind, locator_value, evidence_sha256)
```

`event_part` is `content` or `payload`. A locator is exactly one of:

- `whole`, whose `locator_value` is the empty string;
- `utf8_byte_range`, whose value is `<start>:<end>` for a zero-based half-open
  byte range aligned to UTF-8 code-point boundaries;
- `json_pointer`, whose value is an RFC 6901 JSON Pointer into the selected
  event part.

`evidence_sha256` hashes the exact selected UTF-8 bytes. Source actor/type,
authority class, observation time, and source session/scope are copied from or
deterministically derived from the cited event and remain inspectable Source
Link attributes; changing those values cannot manufacture a different citation
to the same immutable event. Eligibility changes append Source Link state and do
not change identity.

## Accepted operation and idempotency encoding

Operation schema versions `1` and `2` are defined here. They are positive JSON
integers stored beside every accepted operation. Version `1` remains the frozen
contract for `remember_literal_claim` and `remember_entity_claim`. Version `2`
adds only the `correct_claim` effect below. Replay dispatches by the recorded
version and fails closed before changing a shadow projection when the version
is unknown, malformed, or paired with an operation kind that version does not
define.

An idempotency key is `idem:v1:<uuidv4>`, generated by the caller once per
logical submission and unique across Semantic Memory. The accepted operation
stores it with the proposal SHA-256 and normalized effect SHA-256. Reuse with
the same proposal hash returns the original operation ID, result, Transaction
Time, and revision vector without a write. Reuse with a different proposal hash
is an idempotency conflict and changes nothing. Failed stale or invalid attempts
do not reserve a key because no operation was accepted.

The accepted v1 operation envelope records, in schema order:

1. operation schema version, operation ID, operation kind, and idempotency key;
2. actor and immutable session Scope Context;
3. sorted prior Scope Revision vector;
4. exact source event IDs and proposal SHA-256;
5. the complete normalized effect, including reused and generated IDs;
6. one Transaction Time and sorted resulting Scope Revision vector;
7. normalized effect SHA-256.

The effect is atomic. Replay validates the envelope, hash, version, IDs, prior
revisions, and effect before applying it. Replay performs no ID generation,
clock read, model call, Capability call, network request, or external effect.

### Correction operation encoding v2

The v2 accepted-operation envelope is identical to v1 except that
`schema_version` is `2`, `proposal.kind` is exactly `correct_claim`, and the
normalized effect has one additional final field named `corrections`. All
pre-existing effect arrays retain their v1 encodings and equality rules. The
`corrections` array contains exactly one record in this order:

1. `old_claim_id` and `replacement_claim_id`, both canonical UUIDv4 values;
2. `mode`, exactly `error` or `changed`;
3. nullable `effective_time`, which is null for `error` and a canonical
   timestamp for `changed`;
4. `valid_time_effect`, containing `old_before`, `old_after`, and
   `replacement` Valid Time intervals in that order.

The v2 effect contains exactly one immutable replacement Claim, exactly one new
eligible Source Link for that Claim, the ordered old-Claim `superseded`, new-
Claim `active`, and Source-Link `eligible` transitions, and the correction
record. Its proposal and effect hashes cover all of those records. It does not
rewrite the v1 Claim or Source Link encodings. The `idem:v1:` prefix remains the
version of the idempotency-key syntax, not the accepted-operation schema
version.

An existing database is upgraded by widening only the accepted-operation
version constraint from `1` to `1` or `2`. The migration copies every v1 row
byte-for-byte, preserves IDs, proposal/effect hashes, results, Transaction
Times, revision vectors, and foreign-key references, and performs a foreign-key
check before commit. It does not reinterpret or rehash accepted v1 history.
Consequently, a legacy v1 idempotent retry returns its original result after
upgrade. Replay validates and applies v1 operations with the frozen v1
dispatcher, and validates and applies v2 corrections with the v2 dispatcher;
both fail closed on cross-version shapes.

## Versioned fixture manifest

The v1 machine-readable contract is
[`manifest.schema.json`](../fixtures/semantic-memory/v1/manifest.schema.json).
The companion
[`literal-claim.json`](../fixtures/semantic-memory/v1/literal-claim.json) proves
that one manifest can represent all four scope identities, fixed source
evidence, operations, generated IDs, expected projection state, exact queries,
paths, and expected failures.

A fixture runner must reject unknown manifest versions and validate the entire
manifest before applying an operation. Schema validation covers every semantic
record in an effect and projection, every evidence locator, and every query,
path, and failure envelope; operation-specific semantic validation then enforces
cross-record references and equality rules. The runner resets to the declared
registry and source evidence, substitutes only the declared fixed generated IDs
and clock values, applies operations in array order, and compares canonical
results in the declared order. `expected_failures` asserts the error code and
unchanged scope revision vector; it cannot be averaged into a score.

The narrow v2 correction contract is
[`manifest.schema.json`](../fixtures/semantic-memory/v2/manifest.schema.json).
It composes the frozen v1 semantic record definitions and v1 operation
definition without modifying them, and adds the v2 correction operation only.
Its companion [`claim-correction.json`](../fixtures/semantic-memory/v2/claim-correction.json)
fixes the complete replacement Claim, evidence, transitions, correction effect,
proposal hash, effect hash, Transaction Time, and resulting revision. A v2
runner must continue accepting a v1 operation through the referenced frozen v1
definition, dispatch a v2 operation only when its kind is `correct_claim`, and
reject a v2-shaped effect labeled as v1.

## Reconciliation with existing decisions

No binding conflict was found. This contract resolves details that the Stage 3
specification intentionally required before DDL:

- `group_id` is not adopted from Graphiti; canonical non-null `scope_id` rows
  preserve Evie's four explicit scope kinds.
- learned Graphiti relationship text, embeddings, and summaries remain derived
  or later-stage data, not accepted Claim identity.
- `expired_at` mutation is not adopted; append-only lifecycle and separate Valid
  and Transaction Time preserve accepted history.
- Predicate cardinality stays diagnostic, polarity stays explicit, and ordinary
  Entity relationships remain Claims rather than structural Graph Links.
- operation idempotency, proposition equality, and Source Link equality are
  three distinct contracts.

Later implementation may add bounds such as maximum text or decimal length, but
must do so as explicit validation limits without changing canonical equality.
Changing any v1 or v2 encoding or equality rule above requires a new operation
and fixture schema version plus a migration/replay decision; a DDL-only change
may not reinterpret accepted v1 or v2 history.
