# Human adjudication of the first two-format comparison

Status: proposed output judgments; David has approved the input gold, **not these new output interpretations**. These results are exploratory and the comparison has a known prompt confound: JSON mode was not given the complete schema text. A separate corrected-prompt trial may follow; this packet must not be read as a model-family ranking.

**None of the three repair-needed examples is correctly encoded under the reviewed output contract.** Proposed raw semantic credit, if David agrees, does not grant typed/Predicate agreement, source acceptance, or production adequacy. They are explicit human equivalence judgments, not automatic matches.

The 40 requests produced 66 complete JSON proposal objects (34 schema-format, 32 JSON-format). Four requests had truncated output and remain failures. The 33 distinct outputs below each occurred twice. Raw useful meaning and conformance to the experimental field/type/source contract are separate. Three proposed judgments credit a useful underlying interpretation despite encoding defects; all judgments require human confirmation.

| Output / case | Exact new source | Expected memory | Raw proposal | Proposed label | Reason / qualification |
| --- | --- | --- | --- | --- | --- |
| O01 / N01-a | "I prefer tea." | required: owner prefers tea (standing fact; no date specified) | the project — preference — tea (affirmed; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. |
| O02 / N01-a | "I prefer tea." | required: owner prefers tea (standing fact; no date specified) | the project — preference — tea (affirmed; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. |
| O03 / N01-b | "I ate a pear at lunch." | No memory | the project — preference — tea (affirmed; decision, assert) | unsupported | Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal. |
| O04 / N01-b | "I ate a pear at lunch." | No memory | the project — meal — pear (denied; world_change, correct) | unsupported | Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal. |
| O05 / N01-b | "I ate a pear at lunch." | No memory | the project — preference — tea (affirmed; decision, assert) | unsupported | Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal. |
| O06 / N01-b | "I ate a pear at lunch." | No memory | the project — decision — not eating pear at lunch (denied; decision, correct) | unsupported | Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal. |
| O07 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — residence — Paris (affirmed; fact, correct) | unsupported | Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven. |
| O08 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — lives in — Paris (affirmed; fact, assert) | unsupported | Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven. |
| O09 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — moved to — Paris (affirmed; fact, assert) | unsupported | Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven. |
| O10 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — residence — Paris (affirmed; fact, assert) | unsupported | Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven. |
| O11 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — residence — Paris (affirmed; fact, correct) | unsupported | Cites the earlier unendorsed report as a correction/move rather than the new explicit residence endorsement. Neither that source nor that effect is sufficient. |
| O12 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — lives — Paris (affirmed; fact, assert) | required useful | Maya’s supported Paris residence is useful and equivalent to required gold 0. Preserve unresolved identity. The experimental object/subject encoding needs repair; the JSON-format lives/location variant also violates the requested Predicate/type enums. |
| O13 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — moved — Paris (affirmed; fact, assert) | unsupported | Cites the earlier unendorsed report as a correction/move rather than the new explicit residence endorsement. Neither that source nor that effect is sufficient. |
| O14 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — residence — Paris (affirmed; fact, assert) | required useful | Maya’s supported Paris residence is useful and equivalent to required gold 0. Preserve unresolved identity. The experimental object/subject encoding needs repair; the JSON-format lives/location variant also violates the requested Predicate/type enums. |
| O15 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | the project — preference — coffee (denied; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee. |
| O16 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | the project — preference — tea (affirmed; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee. |
| O17 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | the project — preference — tea (affirmed; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee. |
| O18 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | the project — preference — coffee (denied; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee. |
| O19 / N04-b | "She has moved to Paris." | No memory | Maya Chen — relationship — cousin (affirmed; fact, assert) | unsupported | Cites the new ambiguous “She has moved” event as support for an old cousin/colleague relationship; one range also exceeds that event. The new pronoun does not resolve a person. |
| O20 / N04-b | "She has moved to Paris." | No memory | Maya Patel — relationship — colleague (affirmed; fact, assert) | unsupported | Cites the new ambiguous “She has moved” event as support for an old cousin/colleague relationship; one range also exceeds that event. The new pronoun does not resolve a person. |
| O21 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | SQLite — decision — the project (affirmed; decision, assert) | unsupported | Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration. |
| O22 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | PostgreSQL — is a long-term option being considered —  (affirmed; consideration, assert) | optional useful | The source supports PostgreSQL as an unadopted consideration. Its raw meaning is optionally useful; the empty object/type/identity and free-form Predicate fail the requested output contract. |
| O23 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | SQLite — is used for project — 553ecf4c-6a4f-50d4-94e1-8c37985464a7 (affirmed; fact, assert) | unsupported | Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration. |
| O24 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | Offline operation — is a lasting requirement for project — 553ecf4c-6a4f-50d4-94e1-8c37985464a7 (affirmed; fact, assert) | unsupported | Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration. |
| O25 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | offline operation — preference — the project (affirmed; fact, assert) | unsupported | Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration. |
| O26 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | PostgreSQL — consideration — the project (affirmed; consideration, attach_support) | unsupported | Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration. |
| O27 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | Acme — employment_departure — last month (affirmed; fact, assert) | unsupported | Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date. |
| O28 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | Acme — employment — Acme (denied; fact, assert) | unsupported | Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date. |
| O29 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | Acme — employment_status — former (affirmed; fact, assert) | unsupported | Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date. |
| O30 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | Acme — employment — last month (denied; fact, assert) | unsupported | Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date. |
| O31 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | the project — has_decision — Use the date you just checked (affirmed; decision, assert) | unsupported | Turns an instruction to use the checked date into a project decision, loses the coffee change, and adds clock-time precision beyond the required calendar date. |
| O32 / N09-a | "I do not prefer café ☕." | required: owner does not prefer café ☕ (denied standing fact; no date specified) | the project — preference — café ☕ (denied; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. |
| O33 / N09-a | "I do not prefer café ☕." | required: owner does not prefer café ☕ (denied standing fact; no date specified) | the project — preference — cafe ☕ (denied; decision, assert) | unsupported | Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. |

## Exact review records

### O01 — N01-a

Exact proposal identity: `sha256:4d4b73c74c1a8d23df961364ede8e6eccc1968eebd1a26ba0684a1e13d76a053`.

New source evidence:
- "I prefer tea."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 13,
      "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented.

### O02 — N01-a

Exact proposal identity: `sha256:6924283537eda8fe71fc115644ff7706d401945515300e011c419d7af4a2cdfb`.

New source evidence:
- "I prefer tea."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "owner_statement",
  "kind": "decision",
  "object": "tea",
  "object_kind": "EntityID",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 13,
      "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
      "session_id": "4ce94707-d456-561c-9100-150a6a3d3c6a",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented.

### O03 — N01-b

Exact proposal identity: `sha256:293bd8045f9e63d0d01bc826559fec19cfa4b88619bcf11e6a9b00cbcc7caf9d`.

New source evidence:
- "I ate a pear at lunch."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "owner_statement",
  "kind": "decision",
  "object": "tea",
  "object_kind": "EntityID",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 13,
      "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal.

### O04 — N01-b

Exact proposal identity: `sha256:4632948b3f28a7a56121e6fcdd8e6ccf3753b8bcd69046510697b219a1ee206c`.

New source evidence:
- "I ate a pear at lunch."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "correct",
  "identity": "owner_statement",
  "kind": "world_change",
  "object": "pear",
  "object_kind": "EntityID",
  "polarity": "denied",
  "predicate": "meal",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 22,
      "event_id": "155add8e-9dfd-5e37-a055-65759c3b688d",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal.

### O05 — N01-b

Exact proposal identity: `sha256:4d4b73c74c1a8d23df961364ede8e6eccc1968eebd1a26ba0684a1e13d76a053`.

New source evidence:
- "I ate a pear at lunch."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 13,
      "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal.

### O06 — N01-b

Exact proposal identity: `sha256:6c7ec35cca700cde01a0732c4a37fb0ac32f33f292d478570c82d1d1d6907175`.

New source evidence:
- "I ate a pear at lunch."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "correct",
  "identity": "unresolved",
  "kind": "decision",
  "object": "not eating pear at lunch",
  "object_kind": "entity",
  "polarity": "denied",
  "predicate": "decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 22,
      "event_id": "155add8e-9dfd-5e37-a055-65759c3b688d",
      "start": 0
    },
    {
      "end": 13,
      "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Either re-proposes an old preference without new supporting evidence, or invents a denial/correction of the reported pear meal.

### O07 — N02-a

Exact proposal identity: `sha256:28291d5897156b94647ec386b6fb34736b53be85faf4f06698e5a135b95ccfb1`.

New source evidence:
- "For the story, write \"I live in Paris.\" Maya also told me she moved there."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "correct",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "21c06de7-390c-5551-a302-0e469a63c811",
      "start": 0
    },
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 61
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven.

### O08 — N02-a

Exact proposal identity: `sha256:359287afa13733d5a777b574727161b64bdd8611536e6be78eb97001360607c1`.

New source evidence:
- "For the story, write \"I live in Paris.\" Maya also told me she moved there."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "location",
  "polarity": "affirmed",
  "predicate": "lives in",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "session_id": "21c06de7-390c-5551-a302-0e469a63c811",
      "start": 0,
      "text": "For the story, write \"I live in Paris.\" Maya also told me she moved there."
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven.

### O09 — N02-a

Exact proposal identity: `sha256:829d39a009154b84b8a454ddb7c41ab1a2c72ca9442ba2468a6438d65e2778e7`.

New source evidence:
- "For the story, write \"I live in Paris.\" Maya also told me she moved there."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "location",
  "polarity": "affirmed",
  "predicate": "moved to",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "session_id": "21c06de7-390c-5551-a302-0e469a63c811",
      "start": 62,
      "text": "Maya also told me she moved there."
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven.

### O10 — N02-a

Exact proposal identity: `sha256:a3f43751748eda3a2b84054bf63308ce645db85b67571792e557e572fe271225`.

New source evidence:
- "For the story, write \"I live in Paris.\" Maya also told me she moved there."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 0
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Treats the unendorsed report/story as residence or move evidence; D1 does not admit that assertion. A correction effect is also unproven.

### O11 — N02-b

Exact proposal identity: `sha256:284068bb994b1c3fe2acffa122bae7b84965a498e652cb96ae87a8db79ff9788`.

New source evidence:
- "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "correct",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 0
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Cites the earlier unendorsed report as a correction/move rather than the new explicit residence endorsement. Neither that source nor that effect is sufficient.

### O12 — N02-b

Exact proposal identity: `sha256:5333db534629f8429228236d301a19a9ccdbfe72c29fadbb9e143ce04a514f14`.

New source evidence:
- "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "location",
  "polarity": "affirmed",
  "predicate": "lives",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 75,
      "event_id": "eaa07e15-ca81-5ffe-b8e3-c7519f474b93",
      "start": 0
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Maya’s supported Paris residence is useful and equivalent to required gold 0. Preserve unresolved identity. The experimental object/subject encoding needs repair; the JSON-format lives/location variant also violates the requested Predicate/type enums.

### O13 — N02-b

Exact proposal identity: `sha256:84921ce629598dd008b356f4584efa4c51a712a8636d2d8cec3f8c71a3af380d`.

New source evidence:
- "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "location",
  "polarity": "affirmed",
  "predicate": "moved",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 0
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Cites the earlier unendorsed report as a correction/move rather than the new explicit residence endorsement. Neither that source nor that effect is sufficient.

### O14 — N02-b

Exact proposal identity: `sha256:af9dd3a252da47266dd0df9165b147be5cde964fa879f0001c3abd88a3af997d`.

New source evidence:
- "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 75,
      "event_id": "eaa07e15-ca81-5ffe-b8e3-c7519f474b93",
      "start": 0
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Maya’s supported Paris residence is useful and equivalent to required gold 0. Preserve unresolved identity. The experimental object/subject encoding needs repair; the JSON-format lives/location variant also violates the requested Predicate/type enums.

### O15 — N03-b

Exact proposal identity: `sha256:1b0d5dcfe94d6131bc7ab69a2cf9db932d9b110688623768b981acea0b7f1ada`.

New source evidence:
- "Yes."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [
    {
      "end": 28,
      "event_id": "067d03f1-f474-52a7-ba4b-7f678dc5fead",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "coffee",
  "object_kind": "entity",
  "polarity": "denied",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 4,
      "event_id": "0935c3b6-ecbb-571d-88e9-54711ce43e30",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee.

### O16 — N03-b

Exact proposal identity: `sha256:402aedd84a16f421bb223b9e7ee1d65b70fa514467940b03cd436f078b5061e8`.

New source evidence:
- "Yes."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [
    {
      "end": 28,
      "event_id": "067d03f1-f474-52a7-ba4b-7f678dc5fead",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "EntityID",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 4,
      "event_id": "0935c3b6-ecbb-571d-88e9-54711ce43e30",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee.

### O17 — N03-b

Exact proposal identity: `sha256:48c0a58d1c55f1c6988b00c4d6cec6e989c9af2d45e5e99a6546692a5a50d562`.

New source evidence:
- "Yes."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [
    {
      "end": 28,
      "event_id": "067d03f1-f474-52a7-ba4b-7f678dc5fead",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 4,
      "event_id": "0935c3b6-ecbb-571d-88e9-54711ce43e30",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee.

### O18 — N03-b

Exact proposal identity: `sha256:96727311db20d5424b6b557efb559b1edf43a46f8f2f049953288b9cffeb46d3`.

New source evidence:
- "Yes."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [
    {
      "end": 28,
      "event_id": "067d03f1-f474-52a7-ba4b-7f678dc5fead",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "coffee",
  "object_kind": "EntityID",
  "polarity": "denied",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 4,
      "event_id": "0935c3b6-ecbb-571d-88e9-54711ce43e30",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented. Assent also requires the exact non-supporting question; it does not independently establish absolute dislike of coffee.

### O19 — N04-b

Exact proposal identity: `sha256:164e9daba9cf50d1b1af8f2abafc2cf6dbc0565be425ff7f5e9b697a27201545`.

New source evidence:
- "She has moved to Paris."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "cousin",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "relationship",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 23,
      "event_id": "f89cfaed-b920-5115-a0ac-371e724aa98a",
      "start": 0
    }
  ],
  "subject": "Maya Chen",
  "temporal": ""
}
```

Cites the new ambiguous “She has moved” event as support for an old cousin/colleague relationship; one range also exceeds that event. The new pronoun does not resolve a person.

### O20 — N04-b

Exact proposal identity: `sha256:58278ca542438dd0ba8d3607467a5293fbf3417cb0cff497a1b177464fb47dd3`.

New source evidence:
- "She has moved to Paris."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "colleague",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "relationship",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 51,
      "event_id": "f89cfaed-b920-5115-a0ac-371e724aa98a",
      "start": 24
    }
  ],
  "subject": "Maya Patel",
  "temporal": ""
}
```

Cites the new ambiguous “She has moved” event as support for an old cousin/colleague relationship; one range also exceeds that event. The new pronoun does not resolve a person.

### O21 — N05-b

Exact proposal identity: `sha256:0584cecc3963054f8aaddc8df860c270b5fc6f5d5157f572f669eabfde3c4c5d`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 83,
      "event_id": "49290f19-3f35-50aa-b775-cf97d4591adc",
      "start": 0
    }
  ],
  "subject": "SQLite",
  "temporal": ""
}
```

Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration.

### O22 — N05-b

Exact proposal identity: `sha256:15af807072e7348d20e8d22a6d21eda27a2637bae6673db7ed75f2e976908ebe`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "",
  "kind": "consideration",
  "object": "",
  "object_kind": "",
  "polarity": "affirmed",
  "predicate": "is a long-term option being considered",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 98,
      "event_id": "0b9cd9f7-c1e4-595e-98f5-8044d8340602",
      "start": 0
    }
  ],
  "subject": "PostgreSQL",
  "temporal": ""
}
```

The source supports PostgreSQL as an unadopted consideration. Its raw meaning is optionally useful; the empty object/type/identity and free-form Predicate fail the requested output contract.

### O23 — N05-b

Exact proposal identity: `sha256:420132adeda57c95b4bfd09a3b668cef1d7c676667a16586ce537a718d169680`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "",
  "kind": "fact",
  "object": "553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "object_kind": "project",
  "polarity": "affirmed",
  "predicate": "is used for project",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 83,
      "event_id": "49290f19-3f35-50aa-b775-cf97d4591adc",
      "start": 0
    }
  ],
  "subject": "SQLite",
  "temporal": ""
}
```

Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration.

### O24 — N05-b

Exact proposal identity: `sha256:7eb3dc96e35fd31504c4d62f6d24daa98802b38ef58ccab286c0ec0e67a2e459`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "",
  "kind": "fact",
  "object": "553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "object_kind": "project",
  "polarity": "affirmed",
  "predicate": "is a lasting requirement for project",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 83,
      "event_id": "49290f19-3f35-50aa-b775-cf97d4591adc",
      "start": 68
    }
  ],
  "subject": "Offline operation",
  "temporal": ""
}
```

Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration.

### O25 — N05-b

Exact proposal identity: `sha256:b14b1ae289a0ee1b83afb181378df446d9ca588c2cd5dd6dbffa239246d20684`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 83,
      "event_id": "49290f19-3f35-50aa-b775-cf97d4591adc",
      "start": 78
    }
  ],
  "subject": "offline operation",
  "temporal": ""
}
```

Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration.

### O26 — N05-b

Exact proposal identity: `sha256:de3abc3a5dc0832805250f06db5a2c766670f13956b6885aad596e7229950448`.

New source evidence:
- "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "object_kind": "entity",
  "polarity": "affirmed",
  "predicate": "consideration",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 98,
      "event_id": "0b9cd9f7-c1e4-595e-98f5-8044d8340602",
      "start": 0
    }
  ],
  "subject": "PostgreSQL",
  "temporal": ""
}
```

Reverses or changes the relation, invents an additional-support effect without accepted context, or reuses only old/insufficient overlap. The raw effect is not the supported new optional consideration.

### O27 — N06-a

Exact proposal identity: `sha256:0cfa7906e2d202bc03079b9b19a4a0fad81ac85e88db49fdc35bf281ad2efdef`.

New source evidence:
- "I no longer work at Acme. I left last month."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "last month",
  "object_kind": "time",
  "polarity": "affirmed",
  "predicate": "employment_departure",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 52,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "session_id": "c41e0614-f391-5a70-be92-08b95cbc847c",
      "start": 45
    }
  ],
  "subject": "Acme",
  "temporal": "past"
}
```

Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date.

### O28 — N06-a

Exact proposal identity: `sha256:14757daac9c572e56de740d2f8e71f1265d417e7964aad9c9b958ec45025b797`.

New source evidence:
- "I no longer work at Acme. I left last month."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Acme",
  "object_kind": "entity",
  "polarity": "denied",
  "predicate": "employment",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 44,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "start": 0
    }
  ],
  "subject": "Acme",
  "temporal": "past"
}
```

Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date.

### O29 — N06-a

Exact proposal identity: `sha256:8fd590f731e73b90d53b40b9644a00788758bb606eccbd4478e682fecf05b242`.

New source evidence:
- "I no longer work at Acme. I left last month."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "former",
  "object_kind": "state",
  "polarity": "affirmed",
  "predicate": "employment_status",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 44,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "session_id": "c41e0614-f391-5a70-be92-08b95cbc847c",
      "start": 0
    }
  ],
  "subject": "Acme",
  "temporal": "past"
}
```

Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date.

### O30 — N06-a

Exact proposal identity: `sha256:b1f2ba27dea3f8e1481717a63c9954877cfb1ece0417f6c06b6ea0d2558b9e6d`.

New source evidence:
- "I no longer work at Acme. I left last month."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "last month",
  "object_kind": "date",
  "polarity": "denied",
  "predicate": "employment",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 52,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "start": 45
    }
  ],
  "subject": "Acme",
  "temporal": "past"
}
```

Makes Acme the subject, loses the owner’s actual employment change, and replaces the explicit relative date with generic past or an incorrectly typed/denied date.

### O31 — N08-b

Exact proposal identity: `sha256:21ab450bafbbe7519d643eccd91b2056064ec6fcc5612e1c33e8509b47ee23e3`.

New source evidence:
- "Use the date you just checked: as of that date I have stopped drinking coffee."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "new",
  "kind": "decision",
  "object": "Use the date you just checked",
  "object_kind": "fact",
  "polarity": "affirmed",
  "predicate": "has_decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 78,
      "event_id": "eb55a65e-dace-573b-8a85-522b88a4f74d",
      "start": 0
    },
    {
      "end": 28,
      "event_id": "e8cddf05-9db4-550f-aeda-a016becc0cd7",
      "start": 0
    },
    {
      "end": 19,
      "event_id": "edc787df-49e6-5c93-a135-6aa6b7619aa0",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": "as of 2026-09-04 09:30:00"
}
```

Turns an instruction to use the checked date into a project decision, loses the coffee change, and adds clock-time precision beyond the required calendar date.

### O32 — N09-a

Exact proposal identity: `sha256:aedb1ffad11363e1a9e99d933985e1318f6612cd03852db5ea141840a7122bfc`.

New source evidence:
- "I do not prefer café ☕."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "café ☕",
  "object_kind": "place",
  "polarity": "denied",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 26,
      "event_id": "6b4b9ad5-f436-5e1f-b92a-6eab9543c666",
      "session_id": "a45fa288-6b2f-5367-82f4-e91aa687e11b",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented.

### O33 — N09-a

Exact proposal identity: `sha256:fa29bc47fdf9b3bf3789361496eb3dbad725feaa927583ec659be1dbd0ad4c61`.

New source evidence:
- "I do not prefer café ☕."

Exact raw object (including schema violations and source/context coordinates):

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "decision",
  "object": "cafe ☕",
  "object_kind": "entity",
  "polarity": "denied",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 26,
      "event_id": "6b4b9ad5-f436-5e1f-b92a-6eab9543c666",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the owner’s preference to the project and relabels it a decision/entity. The intended owner memory is not represented.
