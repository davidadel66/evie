# Closed-schema compact Qwen output review packet

Status: **proposed, not human reviewed; no judgments applied**.

These are the 15 novel raw objects from the ten-request compact-v2 run. N01-a already matches the approved tea gold exactly and is omitted from this new judgment list. The run has 16 raw proposals and 3 retained; N03-b and N09-a are the two retained objects still requiring meaning review. All four earlier output packets remain unchanged and pending.

The corrected schema eliminated dangling coordinates. Thirteen objects still fail source-category or identity checks. Raw usefulness judgments cannot repair their structure or turn them into retained candidates. “Unsupported” also covers old facts supported only by overlap; it does not label those historical facts fabricated.

[Exact run and sealed requests](reports/development.json) · [Initial score](reports/development-initial-score.json). Exact source strings below are JSON strings. The source/gold approval already exists; this packet asks only for new output judgments.

Gold SHA-256: `sha256:78590a0e3a06f8a2510762d96957e441f7ab4b9c9d4bf964bfa0dc5cc9c81b94`. Initial score SHA-256: `sha256:1011a9fd3022843d4209ffdceb65955e381b15c903ac23a46921afbba4a07f78`.

| ID | Case | Exact tuple, summarized | Retained | Proposed label |
| --- | --- | --- | --- | --- |
| V01 | N01-b | owner habit=pear (affirmed, fact) | No | unsupported |
| V02 | N01-b | owner preference=tea (affirmed, fact) | No | unsupported |
| V03 | N02-a | owner residence=Paris (affirmed, fact) | No | unsupported |
| V04 | N02-a | owner intention=write 'I live in Paris.' in the story (affirmed, decision) | No | unwanted_but_true |
| V05 | N02-a | owner consideration=Maya's information about her move to Paris (affirmed, consideration) | No | unsupported |
| V06 | N02-b | owner residence=Paris (affirmed, fact) | No | unsupported |
| V07 | N03-b | owner preference=tea (affirmed, decision) | Yes | unsupported |
| V08 | N04-b | owner relationship=Maya Chen (affirmed, fact) | No | unsupported |
| V09 | N04-b | owner residence=Maya Chen (affirmed, fact) | No | unsupported |
| V10 | N04-b | owner relationship=Maya Patel (affirmed, fact) | No | unsupported |
| V11 | N05-b | project decision=PostgreSQL (affirmed, consideration) | No | optional_useful |
| V12 | N05-b | project preference=SQLite (affirmed, fact) | No | unsupported |
| V13 | N06-a | owner employment=Acme (affirmed, fact) | No | unsupported |
| V14 | N08-b | owner constraint=2026-09-04 09:30:00 (affirmed, world_change) | No | unsupported |
| V15 | N09-a | owner preference=café (denied, fact) | Yes | required_useful |

The main judgment choices are V04 (transient writing intent versus unsupported attribution), V07 (whether the shortened tea/decision tuple preserves the asserted meaning), V11 (optional raw PostgreSQL credit despite the predicate error), and V15 (café equivalence). Proposed labels and errors elsewhere are also editable. No label is inferred from source approval.

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

### V01 — proposed unsupported

A single pear at lunch does not support an enduring pear habit. The owner/text tuple is also incorrectly marked unresolved.

Retained: **no**. First structural rejection: `invalid_subject: incompatible identity`. Proposed errors: identity, typed_meaning. Required gold match proposed: [].

Raw object SHA-256: `sha256:27521a82d6c99939e998331e9301a18e791c5b562cc49c04c1a1fea41a570d39`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"unresolved","kind":"fact","object":"pear","object_kind":"text","polarity":"affirmed","predicate":"habit","sources":[{"ref":"s2"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### V02 — proposed unsupported

Repeats the older tea preference from overlap alone, with no newly owned support. The explicit owner/text tuple is incorrectly unresolved. Unsupported here means an ineligible repeated truth, not a fabricated historical preference.

Retained: **no**. First structural rejection: `invalid_subject: incompatible identity`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:6cbd38bd697ad3b37c08f118ba8073e50093fa7e5cc8f4fb2ab5559f0a43e3b2`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"unresolved","kind":"fact","object":"tea","object_kind":"text","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s1"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N02-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:74; `39878038-6df9-5dac-8e20-f25238f36882`; observed 2026-09-04T13:30:00Z; `sha256:f0928ace258d2d73efb3084da334d9607959e28affd2f9cec59d6a62d891c338`.

```json
"For the story, write \"I live in Paris.\" Maya also told me she moved there."
```

Approved expectation: no new memory. Neither owner residence nor Maya residence nor an attributed-report Claim is supported under D1.

### V03 — proposed unsupported

Treats fiction and an unendorsed Maya report as the owner living in Paris. The owner field is also wrongly duplicated into assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:6a7a46be0dd599a7d2bd63d5a1552a71da2bc70d9578543f7883e83714c3087f`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Paris","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### V04 — proposed unwanted_but_true

Proposed raw judgment treats the writing request as a transient intent, which is not useful enduring memory. It is not an adopted lasting decision; the owner field is wrongly duplicated into assistant context. This request-as-owner-intention interpretation is explicitly for human review and may instead be judged unsupported.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:d4740635d1d4ae63715abf46bc1f56b6498dada483393e502ff8ab11410f56e2`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"decision","object":"write 'I live in Paris.' in the story","object_kind":"text","polarity":"affirmed","predicate":"intention","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### V05 — proposed unsupported

Mentioning Maya's report does not assert an enduring owner consideration of that information. It also places owner evidence in assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:efd335224141c0ca81aaf238c722e343a80027e0005454f31c3ff62365867b1d`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"consideration","object":"Maya's information about her move to Paris","object_kind":"text","polarity":"affirmed","predicate":"consideration","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
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

### V06 — proposed unsupported

The new endorsement concerns Maya the neighbor, not the owner. The exact output changes the subject to owner and duplicates owner evidence into assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: identity, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:9dd7ffbf53f944955cc7103719a534546a57cc27a6be6fd0ae520109ff28506f`. Occurrences: 1.

```json
{"context":[{"ref":"s2","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Paris","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s2"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
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

### V07 — proposed unsupported

The retained tuple says preference=tea and kind=decision. The reviewed statement is a standing tea-over-coffee comparison, not an adopted decision. The assistant question context is now cited correctly, but structural retention does not restore the lost comparison or kind.

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

### V08 — proposed unsupported

The overlap says Maya Chen is a cousin. The proposed owner relationship object omits cousin and has no newly owned support; owner overlap is also duplicated into assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:2145c6e7ff690d314ac7fcaaff0cc4e8f0ede93dad04f56565bbe20d0843f11c`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Maya Chen","object_kind":"text","polarity":"affirmed","predicate":"relationship","sources":[{"ref":"s1"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

### V09 — proposed unsupported

The exact tuple gives owner residence=Maya Chen, although Maya Chen is not a place and She is unresolved between two Mayas. It invents present temporal wording and duplicates owner evidence into assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: identity, typed_meaning, temporal, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:45b8818b11228b0b1d1771b29cb98639ef61b25474dd57c3ba380bcfd731c45c`. Occurrences: 1.

```json
{"context":[{"ref":"s2","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Maya Chen","object_kind":"text","polarity":"affirmed","predicate":"residence","sources":[{"ref":"s2"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":"present"}
```

### V10 — proposed unsupported

The overlap says Maya Patel is a colleague. The proposed owner relationship object omits colleague and has no newly owned support; owner overlap is also duplicated into assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:bdea167bfcb9c634a8dfedec8087f9a7d2a3c2495a943fc5799e2f34f3eed73a`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Maya Patel","object_kind":"text","polarity":"affirmed","predicate":"relationship","sources":[{"ref":"s1"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
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

### V11 — proposed optional_useful

The raw kind=consideration and future PostgreSQL wording preserve the explicitly unadopted long-term option. Proposed optional raw credit requires human judgment because predicate=decision conflicts with that representation. The owner field is wrongly placed in assistant context; approval would not create retention or required recall.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: predicate, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:67b0c92e2f28ff2c1deb1b2c8f5918bd15648d43dca72cc35b37f811a66473f5`. Occurrences: 1.

```json
{"context":[{"ref":"s2","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"consideration","object":"PostgreSQL","object_kind":"text","polarity":"affirmed","predicate":"decision","sources":[{"ref":"s2"}],"subject_entity_ref":"","subject_name":"","subject_type":"project","temporal":"future"}
```

### V12 — proposed unsupported

Repeats an overlap-only adopted SQLite decision as a preference/fact. There is no new support for that separate proposition, and the owner overlap is wrongly placed in assistant context.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: predicate, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:94f4702658b5536302ecfe455dda5afc21f1e57c84d3558358c5347daeecd706`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"SQLite","object_kind":"text","polarity":"affirmed","predicate":"preference","sources":[{"ref":"s1"}],"subject_entity_ref":"","subject_name":"","subject_type":"project","temporal":""}
```

## N06-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:44; `71442dec-2d3c-50d4-a649-498937e18b6a`; observed 2026-09-04T13:30:00Z; `sha256:cc126513ae425f5819871d418eb7c0d9311b08187b949b6f2e70b64736acfe27`.

```json
"I no longer work at Acme. I left last month."
```

Approved case expectations: required_useful: owner employment=Acme (denied, world_change; temporal 'last month').

### V13 — proposed unsupported

Affirms employment at Acme after the explicit departure. Acme is emitted as an unoffered accepted entity alias, kind=fact loses world_change, and owner evidence is duplicated into assistant context. Keeping last month does not repair the opposed meaning.

Retained: **no**. First structural rejection: `invalid_subject: unknown accepted object alias`. Proposed errors: identity, polarity, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:1403d9aedd2b74e89e75f98039a46ecb303c14eda4baa92e1fd7719c0865d1d1`. Occurrences: 1.

```json
{"context":[{"ref":"s1","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"fact","object":"Acme","object_kind":"entity","polarity":"affirmed","predicate":"employment","sources":[{"ref":"s1","selector":"whole"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":"last month"}
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

### V14 — proposed unsupported

The tuple is a generic constraint whose date object is the unzoned full clock display. It loses stopped drinking coffee and the denied habit meaning, and wrongly treats the tool observation as assistant context. This is not the explicit coffee-change wording emitted by compact-v1.

Retained: **no**. First structural rejection: `reference_category: support/context alias mismatch`. Proposed errors: predicate, typed_meaning, source_attribution. Required gold match proposed: [].

Raw object SHA-256: `sha256:a008e2f4c44184f386f367d4b342a6b1f7be8afe2e930d393deb3fb7979be21b`. Occurrences: 1.

```json
{"context":[{"ref":"s2","selector":"whole"}],"effect":"assert","identity":"resolved","kind":"world_change","object":"2026-09-04 09:30:00","object_kind":"date","polarity":"affirmed","predicate":"constraint","sources":[{"ref":"s3"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```

## N09-a: exact offered evidence

- `s1`: new, owner_statement, original bytes 0:26; `6b4b9ad5-f436-5e1f-b92a-6eab9543c666`; observed 2026-09-04T13:30:00Z; `sha256:00479e7e524eb59e2c69c612d9c9baac37cd023a7da917279c74206d5822660e`.

```json
"I do not prefer café ☕."
```

Approved case expectations: required_useful: owner preference=café ☕ (denied, fact; temporal '').

### V15 — proposed required_useful

Proposed equivalence treats café as the same preference target as the approved café ☕ wording, with the emoji decorative. The denied polarity, whole exact Unicode source, owner and fact encoding are preserved. This retained proposal requires actual human equivalence judgment; it is not an automatic exact match.

Retained: **yes**. First structural rejection: `none`. Proposed errors: none; equivalence pending. Required gold match proposed: [0].

Raw object SHA-256: `sha256:e5c4b37092427407316f3e9dfbe923851a03f468bcc13acc4aa66a0ec31a3805`. Occurrences: 1.

```json
{"context":[],"effect":"assert","identity":"resolved","kind":"fact","object":"café","object_kind":"text","polarity":"denied","predicate":"preference","sources":[{"ref":"s1"}],"subject_entity_ref":"","subject_name":"","subject_type":"owner","temporal":""}
```
