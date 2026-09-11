# #166 first frozen integration run: recall gate not met

The development comparison passes the unchanged gates. Replaying the #165
held-out partition fails the 85% paraphrase minimum at **81.25%**. The required
improvement, latency, scope, secret and delivered-budget checks pass. No gate,
question, expected evidence ID or configuration was changed between partitions.

| Partition / condition | Evidence recovered | Paraphrase recovered | Whole-turn p50 / p95 |
|---|---:|---:|---:|
| Development lexical | 75/96 | 27/48 = 56.25% | 17.424 / 33.472 ms |
| Development hybrid | 90/96 | 42/48 = 87.5% | 29.859 / 46.888 ms |
| Reused held-out lexical | 75/96 | 27/48 = 56.25% | 18.750 / 35.465 ms |
| Reused held-out hybrid | 87/96 | 39/48 = 81.25% | 33.021 / 47.426 ms |

Each partition contains 32 cases repeated three times under each condition:
192 complete turns and 384 captured provider requests. All lexical controls
were recovered. Hybrid misses repeat identically across repetitions:
`development-03-paraphrase`, `development-11-paraphrase`, and held-out cases
`heldout-03-paraphrase`, `heldout-09-paraphrase`, `heldout-13-paraphrase`.
The #165 comparison had already missed `heldout-13-paraphrase`; the other
integration losses must not be hidden by the passing improvement gate.

No forbidden/scope result, secret-bearing provider evidence, unmapped source,
delivered-result-count violation or memory-projection turn-cap violation was
observed. Every turn completed two scripted provider calls and preserved
accepted scope revisions.

| Observation | Development | Reused held-out |
|---|---:|---:|
| Corpus construction | 1.231 s | 1.204 s |
| Retained maintenance | 3.589 s | 3.516 s |
| Maintenance batches, at most 256 scanned rows | 73 | 73 |
| Actual embedding requests, setup plus queries | 169 | 169 |
| Actual embedding inputs / distinct input hashes | 869 / 561 | 869 / 561 |
| Observed HTTP requests | 507 | 507 |
| Largest embedding input | 105 bytes | 105 bytes |
| Largest complete encoded provider request | 34415 bytes | 34562 bytes |
| Largest serialized synthetic memory message | 13319 bytes | 13458 bytes |

All observed HTTP responses were 200, with no transport failure and zero input
secret-scanner positives. The scanner covers all actual input-bearing requests,
including indexing. The maximum input is below the 240-byte chunk bound; this
fixture does not measure long-source chunk recall. RSS, native reader token
usage, answer quality and independent crash/rebuild performance are not measured
by this harness. The other assigned deterministic lifecycle tests and #165
operational measurements remain separate evidence.

## Development-only discrepancy diagnosis

Both failed development hybrid traces contain only `conversation_lexical`
paths. Their returned items do not show any dense contribution. The frozen
production conversation path retrieves up to 64 lexical candidate IDs and gives
the dense generator `64 - len(ids)` remaining candidate slots. A broad lexical
query can therefore consume all slots before semantic ranking, although only
eight lexical evidence items are delivered. This is a plausible generic cause
of the observed loss, pending a deterministic regression; it is not a model
quality conclusion based on held-out wording.

#165 instead obtained eight dense suggestions independently, revalidated them,
and then fused them with its lexical list. Other explicit differences include
the approved source prefix/predicate, additional genuine source acknowledgements,
the eight-result/full-turn budgets, source chunk support, manifest verification
before and after inference, and complete-turn persistence/context composition.
The two failing development cases are raw conversation targets, whose original
source text is unchanged by `f.converse`; their sources and all observed model
inputs are short enough that the 240-byte chunk limit is inactive.

A neutral regression can test candidate starvation without using a held-out
question: at least 70 unrelated original messages matching the common word
`at`, one original `runs before sunrise` target, and the deterministic semantic
query `jogging at dawn`. It should preserve the shared candidate bound while
allowing the complementary dense generator to contribute. Any fix and rerun
must receive a new freeze and retain these failing artifacts.

## Separate compatibility probe

After both timed comparisons, the same frozen executable ran
`TestDenseSelectedModelHandlesBoundedTokenDenseSources` against the actual
selected endpoint. It passed in 0.46 s. This development compatibility probe
checks bounded punctuation-heavy sources and pre/post-inference model identity;
it does not repair or override the failed recall gate.
