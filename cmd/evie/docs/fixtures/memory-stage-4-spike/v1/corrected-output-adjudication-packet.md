# Corrected Mistral output adjudication

Status: proposed; no human output approval recorded. This packet covers only the new common-field-contract Mistral pass at context 8192. It does not change the earlier packet or the approved input gold.

The 20 requests produced 40 complete raw proposal objects (19 schema, 21 JSON), deduplicated into 23 exact objects below. Schema has one truncated failed response; JSON has one response with a missing context field. Neither arm has an exact approved-gold match. Proposed raw credit for C06 concerns the neighbor meaning only; its asserted resolved identity is wrong, so it is not correctly encoded or production-adequate. All judgments below remain provisional.

| Output / case | Exact new source | Expected memory | Exact proposal meaning (compact) | Proposed label | Qualification |
| --- | --- | --- | --- | --- | --- |
| C01 / N01-a | "I prefer tea." | required: owner prefers tea (standing fact; no date specified) | project scope UUID — preference — tea (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Assigns the owner’s tea preference to the project scope UUID. Correct text/fact fields do not repair the wrong subject. |
| C02 / N01-b | "I ate a pear at lunch." | No memory | project scope UUID — preference — tea (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Repeats the old tea preference with old-only support and assigns it to the project. No new evidence owns this proposal. |
| C03 / N01-b | "I ate a pear at lunch." | No memory | project scope UUID — relationship — pear (text; denied; fact; time=lunch; identity=resolved; effect=correct) | unsupported | Invents a denied relationship with a pear and a correction effect; the source only reports an incidental meal. |
| C04 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — residence — Paris (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Treats unendorsed reported speech as Maya’s residence and marks a bare name resolved. D1 does not admit this assertion. |
| C05 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | Maya — relationship — project owner (text; (empty polarity); consideration; time=(unspecified); identity=unresolved; effect=attach_support) | unsupported | Invents Maya’s project-owner relationship, uses an impossible range, empty polarity and unproven attach-support effect. |
| C06 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — relationship — neighbor (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | optional useful | The neighbor relationship is explicitly supported and optional. Raw semantic credit needs human confirmation; Maya must remain unresolved. The emitted bare name plus resolved identity violates that contract. |
| C07 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | Maya — residence — Paris (date; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Calls Paris a date, marks Maya resolved and nominates 76:102 on a 75-byte source. The exact proposal cannot receive supported-source credit through repair. |
| C08 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | project — decision — write "I live in Paris." (text; affirmed; decision; time=(unspecified); identity=resolved; effect=assert) | unsupported | Turns the earlier story-writing request into a project decision and cites only the old, unendorsed event. |
| C09 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | project scope UUID — preference — tea (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Assigns the preference to the project, drops tea-over-coffee qualification, and omits the exact assistant question needed to interpret Yes. |
| C10 / N04-b | "She has moved to Paris." | No memory | Maya Patel — relationship — colleague (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Attaches the old colleague relation to an impossible 24:51 span on the 23-byte ambiguous pronoun event; Maya Patel is not resolved by that new source. |
| C11 / N04-b | "She has moved to Paris." | No memory | Maya Chen — relationship — cousin (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Attaches the old cousin relation to an impossible 0:51 span on the 23-byte ambiguous pronoun event; Maya Chen is not resolved by that new source. |
| C12 / N04-b | "She has moved to Paris." | No memory | Paris — residence —  (text; affirmed; fact; time=(unspecified); identity=unresolved; effect=assert) | unsupported | Paris as the subject with an empty residence object does not identify the ambiguous person or express a supported move. |
| C13 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — constraint — offline operation (text; affirmed; decision; time=(unspecified); identity=resolved; effect=assert) | unsupported | Reuses only old overlap and classifies the offline constraint as a decision. The nominated 74:83 slice does not state the constraint. |
| C14 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — consideration — PostgreSQL (text; affirmed; consideration; time=(unspecified); identity=unresolved; effect=attach_support) | unsupported | The source states an unadopted PostgreSQL consideration, but this exact proposal requests attachment to accepted support without accepted context and marks the project unresolved. |
| C15 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — decision — SQLite (text; affirmed; decision; time=(unspecified); identity=resolved; effect=assert) | unsupported | Re-proposes the old SQLite decision with old-only evidence. It is not newly supported by the PostgreSQL consideration. |
| C16 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | project scope UUID — residence —  (date; (empty polarity); consideration; time=last month; identity=unresolved; effect=attach_support) | unsupported | Invents an empty residence/date consideration with empty polarity and impossible 45:52 coordinates, instead of the owner’s employment change. |
| C17 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | project scope UUID — residence —  (date; denied; fact; time=last month; identity=resolved; effect=assert) | unsupported | Invents an empty denied residence/date proposition with impossible 45:60 coordinates, instead of the owner’s employment change. |
| C18 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | project scope UUID — employment — Acme (text; denied; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Assigns the denied employment to the project, classifies the departure as a standing fact, and omits last month. |
| C19 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | project scope UUID — decision — Check the local date for me (text; affirmed; decision; time=(unspecified); identity=resolved; effect=attach_support) | unsupported | Reuses an old date-check instruction as a project decision and requests unsupported attachment. It is not new memory evidence. |
| C20 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | project scope UUID — residence — 2026-09-04 09:30:00 (date; affirmed; fact; time=(unspecified); identity=resolved; effect=correct) | unsupported | Turns an old clock observation into project residence with a correction effect, adds full-time precision and omits required context field. It is not the owner’s coffee change. |
| C21 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | project scope UUID — residence — 2026-09-04 09:30:00 (date; affirmed; fact; time=(unspecified); identity=resolved; effect=correct) | unsupported | Turns an old clock observation into project residence with a correction effect and full-time precision. It is not the owner’s coffee change. |
| C22 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | project scope UUID — decision — Use the date you just checked (text; affirmed; decision; time=as of that date; identity=resolved; effect=assert) | unsupported | Turns the date-use instruction into a project decision, retains unresolved relative wording, and omits the owner’s coffee habit change. |
| C23 / N09-a | "I do not prefer café ☕." | required: owner does not prefer café ☕ (denied standing fact; no date specified) | project scope UUID — preference — café ☕ (text; denied; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Assigns the owner’s denied café preference to the project scope UUID. |

## Exact proposal records

### C01 — N01-a

Exact proposal SHA-256: `sha256:12ab6f4ec03703b8d2eb3066443efc77af7b49f8a3ed302d4770081ba70be0b4`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "tea",
  "object_kind": "text",
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

Assigns the owner’s tea preference to the project scope UUID. Correct text/fact fields do not repair the wrong subject.

### C02 — N01-b

Exact proposal SHA-256: `sha256:12ab6f4ec03703b8d2eb3066443efc77af7b49f8a3ed302d4770081ba70be0b4`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "tea",
  "object_kind": "text",
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

Repeats the old tea preference with old-only support and assigns it to the project. No new evidence owns this proposal.

### C03 — N01-b

Exact proposal SHA-256: `sha256:cb10e1690e7ef620bbca1746b9cb92e30f2417c5b686f829f0ad91a1a0d705ac`; occurrences: 2.

```json
{
  "context": [],
  "effect": "correct",
  "identity": "resolved",
  "kind": "fact",
  "object": "pear",
  "object_kind": "text",
  "polarity": "denied",
  "predicate": "relationship",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 22,
      "event_id": "155add8e-9dfd-5e37-a055-65759c3b688d",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": "lunch"
}
```

Invents a denied relationship with a pear and a correction effect; the source only reports an incidental meal.

### C04 — N02-a

Exact proposal SHA-256: `sha256:a48fba38f80bf44c5a60b88a550b02fa184f883d581b24f65977fe49c2a059ab`; occurrences: 1.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "text",
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

Treats unendorsed reported speech as Maya’s residence and marks a bare name resolved. D1 does not admit this assertion.

### C05 — N02-a

Exact proposal SHA-256: `sha256:e2a61678147f9e4b98252989483f87e2c14f702bcb4eb38acdfb4dcb4d93e114`; occurrences: 1.

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "project owner",
  "object_kind": "text",
  "polarity": "",
  "predicate": "relationship",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 104,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 75
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Invents Maya’s project-owner relationship, uses an impossible range, empty polarity and unproven attach-support effect.

### C06 — N02-b

Exact proposal SHA-256: `sha256:1189c4bfa9847c18e1742f07cb34f5359216ff3029b6acdccaa7499635f12c1f`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "neighbor",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "relationship",
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

The neighbor relationship is explicitly supported and optional. Raw semantic credit needs human confirmation; Maya must remain unresolved. The emitted bare name plus resolved identity violates that contract.

### C07 — N02-b

Exact proposal SHA-256: `sha256:8db32d2241c0db1ca845f4424c9661a1b962e312a5bc4d47837c8496ab9b8986`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "date",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 102,
      "event_id": "eaa07e15-ca81-5ffe-b8e3-c7519f474b93",
      "start": 76
    }
  ],
  "subject": "Maya",
  "temporal": ""
}
```

Calls Paris a date, marks Maya resolved and nominates 76:102 on a 75-byte source. The exact proposal cannot receive supported-source credit through repair.

### C08 — N02-b

Exact proposal SHA-256: `sha256:bad878bb101645d417d384d59461ad6781e1b59c41f3b1e52ee74ca93d579444`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "write \"I live in Paris.\"",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 74,
      "event_id": "39878038-6df9-5dac-8e20-f25238f36882",
      "start": 0
    }
  ],
  "subject": "project",
  "temporal": ""
}
```

Turns the earlier story-writing request into a project decision and cites only the old, unendorsed event.

### C09 — N03-b

Exact proposal SHA-256: `sha256:c8f230deca9caa5014f633f5b6f98b1bfb23e4f477c2a3dd2e99708ad9fbca5d`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "tea",
  "object_kind": "text",
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

Assigns the preference to the project, drops tea-over-coffee qualification, and omits the exact assistant question needed to interpret Yes.

### C10 — N04-b

Exact proposal SHA-256: `sha256:07e1f2a4f5c2f99f15354ba80538b74c155d2298f0451e29203476c002598c2a`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "colleague",
  "object_kind": "text",
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

Attaches the old colleague relation to an impossible 24:51 span on the 23-byte ambiguous pronoun event; Maya Patel is not resolved by that new source.

### C11 — N04-b

Exact proposal SHA-256: `sha256:f9e9c28c4409adbe4816383b57439cd5ceab64c1120bf49888fa93ff36c23562`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "cousin",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "relationship",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 51,
      "event_id": "f89cfaed-b920-5115-a0ac-371e724aa98a",
      "start": 0
    }
  ],
  "subject": "Maya Chen",
  "temporal": ""
}
```

Attaches the old cousin relation to an impossible 0:51 span on the 23-byte ambiguous pronoun event; Maya Chen is not resolved by that new source.

### C12 — N04-b

Exact proposal SHA-256: `sha256:facef654fec964e59ad51505b3f6f02936d8eb9e28969137fe8bfc8707d156e9`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 23,
      "event_id": "f89cfaed-b920-5115-a0ac-371e724aa98a",
      "start": 0
    }
  ],
  "subject": "Paris",
  "temporal": ""
}
```

Paris as the subject with an empty residence object does not identify the ambiguous person or express a supported move.

### C13 — N05-b

Exact proposal SHA-256: `sha256:241631dbcd76362d2c277c2c9864aa8ee4dead2b962fef5312fef1ce4405f171`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "offline operation",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "constraint",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 83,
      "event_id": "49290f19-3f35-50aa-b775-cf97d4591adc",
      "start": 74
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Reuses only old overlap and classifies the offline constraint as a decision. The nominated 74:83 slice does not state the constraint.

### C14 — N05-b

Exact proposal SHA-256: `sha256:dd367e7a59b0d58d835d68efbd567f66c729c3df8d95698685d552f007514db8`; occurrences: 2.

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "PostgreSQL",
  "object_kind": "text",
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
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

The source states an unadopted PostgreSQL consideration, but this exact proposal requests attachment to accepted support without accepted context and marks the project unresolved.

### C15 — N05-b

Exact proposal SHA-256: `sha256:fff0f9546c05a4e341a50e5ab16308f96440e96f91b349dca7a7200abdf875f1`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "SQLite",
  "object_kind": "text",
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
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Re-proposes the old SQLite decision with old-only evidence. It is not newly supported by the PostgreSQL consideration.

### C16 — N06-a

Exact proposal SHA-256: `sha256:0f36e8179504f9c13ea09570d97fa19a50a94432bffac89be92fe2cef7deee61`; occurrences: 1.

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "",
  "object_kind": "date",
  "polarity": "",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 52,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "start": 45
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": "last month"
}
```

Invents an empty residence/date consideration with empty polarity and impossible 45:52 coordinates, instead of the owner’s employment change.

### C17 — N06-a

Exact proposal SHA-256: `sha256:3c45144f5bf7d5b93d31b6dad08e394a4b92e696748be6db6f569995695e0b73`; occurrences: 1.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "",
  "object_kind": "date",
  "polarity": "denied",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 60,
      "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
      "start": 45
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": "last month"
}
```

Invents an empty denied residence/date proposition with impossible 45:60 coordinates, instead of the owner’s employment change.

### C18 — N06-a

Exact proposal SHA-256: `sha256:ad2023b85a2bb070ba1da25153198f6b2aa158369de2b98719a376965f146da8`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "Acme",
  "object_kind": "text",
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
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Assigns the denied employment to the project, classifies the departure as a standing fact, and omits last month.

### C19 — N08-b

Exact proposal SHA-256: `sha256:180c0afa5a20a48924722c41db6611d0192902d5dbb0800a1e7ecb081361ea43`; occurrences: 2.

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "resolved",
  "kind": "decision",
  "object": "Check the local date for me",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 28,
      "event_id": "e8cddf05-9db4-550f-aeda-a016becc0cd7",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Reuses an old date-check instruction as a project decision and requests unsupported attachment. It is not new memory evidence.

### C20 — N08-b

Exact proposal SHA-256: `sha256:7e82516e8d1d9f7a91fa5a1ce25a5d86722e73b3ef49dd6edf820f64a01b62d4`; occurrences: 1.

```json
{
  "effect": "correct",
  "identity": "resolved",
  "kind": "fact",
  "object": "2026-09-04 09:30:00",
  "object_kind": "date",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 19,
      "event_id": "edc787df-49e6-5c93-a135-6aa6b7619aa0",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Turns an old clock observation into project residence with a correction effect, adds full-time precision and omits required context field. It is not the owner’s coffee change.

### C21 — N08-b

Exact proposal SHA-256: `sha256:d2fe3b87e455bd94a912ee09110d3fa4d49ad993915d390000e3f4eb56c90a48`; occurrences: 1.

```json
{
  "context": [],
  "effect": "correct",
  "identity": "resolved",
  "kind": "fact",
  "object": "2026-09-04 09:30:00",
  "object_kind": "date",
  "polarity": "affirmed",
  "predicate": "residence",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 19,
      "event_id": "edc787df-49e6-5c93-a135-6aa6b7619aa0",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Turns an old clock observation into project residence with a correction effect and full-time precision. It is not the owner’s coffee change.

### C22 — N08-b

Exact proposal SHA-256: `sha256:dace63c09bbafcd9553773dc9af31076fabd7a59f814557add964b569ca0e9f3`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "Use the date you just checked",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "decision",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 78,
      "event_id": "eb55a65e-dace-573b-8a85-522b88a4f74d",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": "as of that date"
}
```

Turns the date-use instruction into a project decision, retains unresolved relative wording, and omits the owner’s coffee habit change.

### C23 — N09-a

Exact proposal SHA-256: `sha256:29e7e041c5562e12b4ade4ba421dfbceab580c4cb864e6fc4b59d4403b207f1f`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "café ☕",
  "object_kind": "text",
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

Assigns the owner’s denied café preference to the project scope UUID.
