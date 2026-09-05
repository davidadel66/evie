# Category-schema compact Qwen output review packet

Status: **proposed, not human reviewed; no judgments applied**.

This packet records the 13 unadjudicated raw objects from compact-v3. N01-a is the one automatic exact approved-gold match and is omitted from this judgment list. The run contains 14 raw proposals and 9 retained; 8 retained meanings are still pending. C05 exactly repeats pending compact-v2 V07, so this list is not 13 wholly novel interpretations. All five earlier packets remain unchanged and pending.

Reference-category and dangling-coordinate failures are gone. Structural retention does not establish meaning. Four objects lack newly owned support and one has incompatible identity. Raw semantic credit cannot repair those rejections or create retained output. “Unsupported” also covers true older facts ineligible as new candidates because they use only overlap.

[Exact run and request/schema/source seals](reports/development.json) · [Initial score](reports/development-initial-score.json). Source strings below are exact JSON strings; no source/gold labels changed.

Gold SHA-256: `sha256:78590a0e3a06f8a2510762d96957e441f7ab4b9c9d4bf964bfa0dc5cc9c81b94`. Initial score SHA-256: `sha256:1e26acdbd1fde7881a8f246d96cd985e8bdc10c2569123009bd9b09b64f1d88f`.

| ID | Case | Exact tuple, summarized | Retained | Proposed label |
| --- | --- | --- | --- | --- |
| C01 | N01-b | owner preference=tea (affirmed, fact) | No | unsupported |
| C02 | N02-a | owner residence=Paris (affirmed, fact) | Yes | unsupported |
| C03 | N02-a | owner consideration=move to Paris (affirmed, decision) | Yes | unsupported |
| C04 | N02-b | owner residence=Paris (affirmed, fact) | Yes | unsupported |
| C05 | N03-b | owner preference=tea (affirmed, decision) | Yes | unsupported |
| C06 | N04-b | owner residence=Paris (affirmed, fact) | Yes | unsupported |
| C07 | N05-b | project constraint=offline operation (affirmed, fact) | No | unsupported |
| C08 | N05-b | project decision=PostgreSQL (affirmed, consideration) | No | optional_useful |
| C09 | N05-b | project preference=SQLite (affirmed, decision) | No | unsupported |
| C10 | N06-a | owner employment=work at Acme (affirmed, fact) | Yes | unsupported |
| C11 | N08-b | owner preference=local date (affirmed, fact) | No | unsupported |
| C12 | N08-b | owner decision=stop drinking coffee (affirmed, decision) | Yes | unsupported |
| C13 | N09-a | owner preference=café (denied, fact) | Yes | required_useful |

The judgment choices include C08 optional PostgreSQL raw credit, C10 departure versus historical employment wording, C12 decision/action/date wording versus completed change and missing clock support, and C13 café equivalence. C05 repeats V07 exactly. Existing pending questions are identified for reuse by the review coordinator; no prior packet is reapproved or its labels applied here.

## N01-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:13; `0d8102f9-c956-5110-82d9-4853195285fd`; observed 2026-09-04T13:30:00Z; `sha256:0b848d5bacc4fe482bebcc7651d89f6c1ecf683773e0e528077bad5f36a059e2`.

```json
"I prefer tea."
```

- `s2`: new, owner_statement, original bytes 0:22; `155add8e-9dfd-5e37-a055-65759c3b688d`; observed 2026-09-04T13:30:00Z; `sha256:1eaa90441b1cbb74086377d882716796752383cfea7dca5cc067178d4a0a56d7`.

```json
"I ate a pear at lunch."
```

Approved expectation: no new memory. The pear is true but unwanted. The old preference alone cannot own a new candidate.

### C01 — proposed unsupported

Repeats the older tea preference from overlap alone. The newly owned pear meal supplies no support for this preference. Unsupported here means an ineligible repeated truth, not a fabricated historical preference.

Retained: **no**. First structural rejection: `candidate has no newly owned support`. Proposed errors: source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:19ead54a895cf265144b88e2b074ebd8d9abe1a941612587fa9861db1d5b7286`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"tea","object_kind":"text","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N02-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:74; `39878038-6df9-5dac-8e20-f25238f36882`; observed 2026-09-04T13:30:00Z; `sha256:f0928ace258d2d73efb3084da334d9607959e28affd2f9cec59d6a62d891c338`.

```json
"For the story, write \"I live in Paris.\" Maya also told me she moved there."
```

Approved expectation: no new memory. Neither owner residence nor Maya residence nor an attributed-report Claim is supported under D1.

### C02 — proposed unsupported

Retained, but treats a fictional first-person line and an unendorsed Maya report as the owner living in Paris. Exact source copying does not supply the missing owner assertion.

Retained: **yes**. First structural rejection: `none`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:3233b60e719dc2a7b1dd11d393d938bdcbd4d995fff4456871dc42e364d82954`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"Paris","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### C03 — proposed unsupported

Retained, but the owner never asserts a decision or enduring consideration to move to Paris. The source concerns a story and Maya's unendorsed report; changing it into the owner's choice is unsupported.

Retained: **yes**. First structural rejection: `none`. Proposed errors: identity, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:52b8207c79112efcd255561fb4e3841979a6b382d4fe78cd3557829697328799`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"decision","object":"move to Paris","object_kind":"text","polarity":"affirmed","predicate":"consideration","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N02-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:74; `39878038-6df9-5dac-8e20-f25238f36882`; observed 2026-09-04T13:30:00Z; `sha256:f0928ace258d2d73efb3084da334d9607959e28affd2f9cec59d6a62d891c338`.

```json
"For the story, write \"I live in Paris.\" Maya also told me she moved there."
```

- `s2`: new, owner_statement, original bytes 0:75; `eaa07e15-ca81-5ffe-b8e3-c7519f474b93`; observed 2026-09-04T13:30:00Z; `sha256:4e701d36372a437c4ee48f46ff53c0d3490267c5e4756679bd4eb4f717ac0ad8`.

```json
"Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."
```

Approved case expectations: required_useful: new:Maya (neighbor) residence=Paris (affirmed, fact; temporal ''); optional_useful: new:Maya (neighbor) relationship=neighbor (affirmed, fact; temporal '').

### C04 — proposed unsupported

Retained, but the new endorsement concerns Maya the neighbor, not the owner. The subject is explicitly owner; no new-Entity identity is proposed.

Retained: **yes**. First structural rejection: `none`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:b8fa2a905a74d1a412924dd92a612138bde4dacf9bb61232e932004f682d830f`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"Paris","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s2","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N03-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:22; `fec582bc-51f0-52a4-a323-93950fd00118`; observed 2026-09-04T13:30:00Z; `sha256:c52d1a2b546bb6a49005c279babcf0f4cbf3a8841b0bdfd244acb4f2a953afd9`.

```json
"Let us discuss drinks."
```

- `s2`: context, none, original bytes 0:28; `067d03f1-f474-52a7-ba4b-7f678dc5fead`; observed 2026-09-04T13:30:00Z; `sha256:7554d91ba8ce18ef922fbc9f4c587d8e0369e9ecd98a8046e6cea466d97d4ec3`.

```json
"Do you prefer tea to coffee?"
```

- `s3`: new, owner_statement, original bytes 0:4; `0935c3b6-ecbb-571d-88e9-54711ce43e30`; observed 2026-09-04T13:30:00Z; `sha256:5f9a2b795615ba6a3d5455fd5624d773fbca5bcd16249c421fd37411dc9837da`.

```json
"Yes."
```

Approved case expectations: required_useful: owner preference=tea over coffee (affirmed, fact; temporal '').

### C05 — proposed unsupported

Retained with correct assistant question context, but preference=tea and kind=decision lose the reviewed standing tea-over-coffee comparison/fact encoding. This raw object is byte-for-byte identical after canonical JSON encoding to pending compact-v2 V07; no second independent judgment or automatic approval is inferred.

Retained: **yes**. First structural rejection: `none`. Proposed errors: typed_meaning. Required gold match proposed: [].

Raw object SHA-256: `sha256:e2332ccab62606d71bff818644dca17bfc2d6162cb370e6d1a97ab6b32055506`. Occurrences: 1.

```json
{"context":[{"ref":"s2","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"decision","object":"tea","object_kind":"text","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s3"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N04-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:51; `2b5f248a-7225-5148-8a68-f5911fe3c31a`; observed 2026-09-04T13:30:00Z; `sha256:3b9c154037632ff6c62694d266d50c1b2f73e9860a5d8943dd75b730daba70c3`.

```json
"Maya Chen is my cousin. Maya Patel is my colleague."
```

- `s2`: new, owner_statement, original bytes 0:23; `f89cfaed-b920-5115-a0ac-371e724aa98a`; observed 2026-09-04T13:30:00Z; `sha256:cdc2c8ccccb3af63b222bb46880aedaf16baf69d2ae98ab6a016c06ae69eb57b`.

```json
"She has moved to Paris."
```

Approved expectation: no new memory. The pronoun has two plausible antecedents. Neither Maya receives a residence/move assertion.

### C06 — proposed unsupported

Retained, but She is ambiguous between Maya Chen and Maya Patel. The output instead assigns Paris residence to the owner, which the source does not assert.

Retained: **yes**. First structural rejection: `none`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:2594c3028394905519542cad347403a3c9b0b8285e723a6adafe1b384f9251af`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"Paris","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s2"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N05-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:83; `49290f19-3f35-50aa-b775-cf97d4591adc`; observed 2026-09-04T13:30:00Z; `sha256:b1c638915131496bd0157607e3f01edd6dddf8963fdac03cd31a033acfd97b7a`.

```json
"For this project we have chosen SQLite. Offline operation is a lasting requirement."
```

- `s2`: new, owner_statement, original bytes 0:98; `0b9cd9f7-c1e4-595e-98f5-8044d8340602`; observed 2026-09-04T13:30:00Z; `sha256:2dc63a1b1fe684023f89662c475518ef26282b3802d1dcaacdb4b207507b7cf1`.

```json
"For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."
```

Approved case expectations: optional_useful: project consideration=PostgreSQL (affirmed, consideration; temporal '').

### C07 — proposed unsupported

The offline requirement is an older true project constraint, supported only by overlap. The new PostgreSQL consideration does not own this separate constraint; no new candidate is eligible.

Retained: **no**. First structural rejection: `candidate has no newly owned support`. Proposed errors: source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:1b08c16b0e2793488cecb554e721af616d4dff9a309cc56d8ad59f01bcce225f`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"offline operation","object_kind":"text","polarity":"affirmed","predicate":"constraint","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"project","temporal":""}
```

### C08 — proposed optional_useful

Proposed optional raw credit recognizes the explicitly unadopted PostgreSQL consideration. The exact predicate=decision conflicts with kind=consideration, and project/text is incorrectly unresolved. Human judgment is needed for raw credit; approval would not repair identity, create retention or supply required recall.

Retained: **no**. First structural rejection: `invalid_subject: incompatible identity`. Proposed errors: identity, predicate. Required gold match proposed: [].

Raw object SHA-256: `sha256:21591dc80d8debdbcd2b0aa2eb18da5afcb0e19c703698378d47e9fc49479eda`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"unresolved","kind":"consideration","object":"PostgreSQL","object_kind":"text","polarity":"affirmed","predicate":"decision","sources":[{"ref":"s2","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"project","temporal":""}
```

### C09 — proposed unsupported

Repeats the old SQLite decision from overlap as a preference. No newly owned source supports this separate old decision; the newly owned PostgreSQL option does not change that ownership.

Retained: **no**. First structural rejection: `candidate has no newly owned support`. Proposed errors: predicate, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:a574750e382c8ce772b58f91e2ec0ea0835a124456772de750f136332c6c179d`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"decision","object":"SQLite","object_kind":"text","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"project","temporal":""}
```

## N06-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:44; `71442dec-2d3c-50d4-a649-498937e18b6a`; observed 2026-09-04T13:30:00Z; `sha256:cc126513ae425f5819871d418eb7c0d9311b08187b949b6f2e70b64736acfe27`.

```json
"I no longer work at Acme. I left last month."
```

Approved case expectations: required_useful: owner employment=Acme (denied, world_change; temporal 'last month').

### C10 — proposed unsupported

Retained, but emits affirmative employment/work at Acme as fact with last month, instead of the explicit departure/denied world-change meaning. If last month is read purely as past employment, that is a different historical proposition and still omits the required departure. This distinction is for actual human judgment; the proposed errors are not scored totals.

Retained: **yes**. First structural rejection: `none`. Proposed errors: polarity, typed_meaning. Required gold match proposed: [].

Raw object SHA-256: `sha256:e97c6ae7cde2e33c8c79b212c72be3b90886cf54af028d81d611d70b654a2b0f`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"work at Acme","object_kind":"text","polarity":"affirmed","predicate":"employment","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":"last month"}
```

## N08-b: exact offered evidence

- `s1`: overlap, owner_statement, original bytes 0:28; `e8cddf05-9db4-550f-aeda-a016becc0cd7`; observed 2026-09-04T13:30:00Z; `sha256:ecd98d1de10bf574e828b3dd6fcad7d847a0d23d1852a3db2f5a67d81e22fb86`.

```json
"Check the local date for me."
```

- `s2`: overlap, tool_observation, original bytes 0:19; `edc787df-49e6-5c93-a135-6aa6b7619aa0`; observed 2026-09-04T13:30:00Z; `sha256:9e1d9445a52d8465e4b69a3c137d2130e5cc06ca37c39229a1deff54609381e1`.

```json
"2026-09-04 09:30:00"
```

- `s3`: new, owner_statement, original bytes 0:78; `eb55a65e-dace-573b-8a85-522b88a4f74d`; observed 2026-09-04T13:30:00Z; `sha256:2a5a4fcfe445308a4a97b52746d173eea1eff78e58b3d5c552d739a7a75e6989`.

```json
"Use the date you just checked: as of that date I have stopped drinking coffee."
```

Approved case expectations: required_useful: owner habit=drinking coffee (denied, world_change; temporal '2026-09-04').

### C11 — proposed unsupported

A clock-only overlap reference cannot establish an owner preference. local date is not a typed calendar date value, and attach_support has no offered equal accepted Claim. It has no newly owned owner support and does not represent the coffee change.

Retained: **no**. First structural rejection: `candidate has no newly owned support`. Proposed errors: predicate, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:2c800225b98e766319c747295e3848aaf34b0a367fd60d6c2ce3a5d74048eb0c`. Occurrences: 1.

```json
{"context":[],"effect":"attach_support","identity":"resolved","kind":"fact","object":"local date","object_kind":"date","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s2","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### C12 — proposed unsupported

Retained, but represents a decision to stop coffee rather than the asserted completed habit change. Its exact calendar date is taken from the clock without citing that required clock support; only the owner's relative reference is cited. Whether this action/date wording merits partial raw useful credit despite those defects is explicitly left for human review; no required recall match is proposed.

Retained: **yes**. First structural rejection: `none`. Proposed errors: predicate, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:478f2c0e80289a52cee5e8bc4a187c1ff4f42f27128d6b57e917594595654794`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"decision","object":"stop drinking coffee","object_kind":"text","polarity":"affirmed","predicate":"decision","sources":[{"ref":"s3","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":"as of 2026-09-04"}
```

## N09-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:26; `6b4b9ad5-f436-5e1f-b92a-6eab9543c666`; observed 2026-09-04T13:30:00Z; `sha256:00479e7e524eb59e2c69c612d9c9baac37cd023a7da917279c74206d5822660e`.

```json
"I do not prefer café ☕."
```

Approved case expectations: required_useful: owner preference=café ☕ (denied, fact; temporal '').

### C13 — proposed required_useful

Proposed equivalence treats café as the approved café ☕ preference target with a decorative emoji. Denied polarity, whole exact Unicode source, owner and fact encoding are preserved. This is the same unresolved equivalence question as compact-v2 V15, not a new automatic match. Actual human approval is still required.

Retained: **yes**. First structural rejection: `none`. Proposed errors: none; equivalence pending. Required gold match proposed: [0].

Raw object SHA-256: `sha256:27bef93893e3a889b007b378c0d808d21f4bbf73137c3700402f9f657065028a`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"café","object_kind":"text","polarity":"denied","predicate":"preference","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```
