# #159 reader development v3: retirement fixed in this case, two failures

This attempt adds the production `memory-retrieval-v2` reading guide and
negative-only `historical_only` evidence IDs. The compact v2 system instruction,
model, settings, three development cases, harness, text checks and original
manual rubric remain unchanged. **Historical retirement passes; Boston/Chicago
and Kyoto attribution fail.** This result does not satisfy the failed reader
quality criteria, and the failed checks have not been weakened.

| Case | Model input / output tokens | Model-call time | Marker checks | Manual rubric |
|---|---:|---:|---|---|
| historical_retired | 3248 / 116 | 29.097 s | Pass | Pass |
| saved_boston_newer_chicago | 3120 / 125 | 12.250 s | **Fail: missing source ID** | **Fail** |
| tentative_quote_and_inference | 2671 / 184 | 13.011 s | **Fail: missing source ID** | **Fail** |

The compiled test returned FAIL in 54.65 seconds. Each case made one actual
model request. The evidence contract assertions passed before every model
request, accepted-memory revisions stayed unchanged, and no model response was
truncated. This attempt contains no held-out #167/#168 cases.

## Frozen change and execution

`rendering-delta.json` records the production change. The reading guide makes
retirement and exact source ownership explicit beside the evidence. The
negative-only list is derived from currently retired evidence after eligibility
revalidation; it does not assert that every other item is an accepted current
fact. Both additions use the existing serialized projection budget and are
cleared when the evidence projection is withheld. The Evidence array shape is
unchanged.

`freeze.json` pins the harness, unchanged original rubric, model, settings,
system prompt and retrieval rendering. `run-metadata.json` pins the compiled
binary and production Go file hashes, which were checked before and after the
compilation. All pins were written before the first model request. Prior
attempts remain unchanged. The root task's Ollama server was not stopped or
modified.

## Manual assessment against the frozen rubric

**Historical retirement — pass.** The reader names the old chickpea-sandwich
preference and correctly cites event `cb898cd9-caae-4dfd-9ea0-002f89bec527`. It
explicitly says the record is historical and retired and therefore does not
establish the current preference. It invents no replacement preference or
unsupported date. This is a pass on one development case, not a general
retirement-quality guarantee.

**Boston/Chicago — fail.** The reader cites the newer Chicago event
`166fe869-cb85-4024-bc58-4cf3ac8a1362`, but omits the Boston source event
`ff4c05fa-cd8b-446f-82b2-493d96538d6a`. It says the earlier Boston record “has
been superseded” and that the search found no other conflicting information.
The actual supplied Boston Claim remains accepted and active; the Chicago
statement has not changed it. Consequently the answer fails the required
accepted-record-versus-later-statement distinction and discrepancy explanation,
as well as the source-ID check. The broad discrepancy marker also produces a
false positive because it matches the word inside a denial of conflict.

The captured final projection contains both required records but no
`newer_owner_statement` path or `related_claim_ids` on the Chicago excerpt.
The two scripted reads can replace an existing companion by its ordinary
conversation-search result. This observation was reported for a separate
deterministic regression; it does not excuse the reader failure or establish
the cause of the answer.

**Tentative quote and inference — fail.** The reader preserves the owner's
uncertainty and correctly attributes the colleague's definite plan to owner
source event `f103c2ae-46dd-47c9-bd66-3d1711a79ace`. However, it cites invented
event `b22b31b6-60b1-489c-b03f-c5126c1fddce` for the owner's uncertain-plan
quotation, which belongs to that same original owner event. It omits actual
assistant event `93eea48f-370f-45bd-b64d-d371c6c685ca`, and says the inference
was based on the colleague's statement without that causal explanation being
present in the assistant source. These violate faithful source attribution.
No booking, itinerary or precise date was invented.

The raw requests, answers, source identities, exact timing samples and failed
marker checks remain immutable evidence of this attempt. Future production
changes require a new freeze and attempt; no result here is relabeled as a
pass to obtain an acceptable overall score.
