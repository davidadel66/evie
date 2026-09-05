# Qwen2.5 novel-output adjudication

Status: proposed; no human output judgments recorded. This packet covers only the new Qwen2.5 schema experiment. The earlier Mistral packets and approved source/gold remain unchanged.

All 20 planned requests completed with whole-response status ok: 30 raw proposals, 16 structurally retained. Four raw/retained occurrences exactly match the already approved tea and denied café preferences (N01-a/N09-a, each twice). The incidental-meal window N01-b emitted no candidates twice. The 26 remaining proposal occurrences are 13 distinct objects below; each occurred twice. Required opportunities are 12, with 4 exact matches and 2 required retained omissions on N08-b. Unlisted meanings remain unadjudicated, not automatically incorrect or useful.

Q10 proposes optional raw semantic credit for PostgreSQL while retaining an encoding error. It grants no required recall or production adequacy. The literal new:Name outputs are shown exactly; the reader must not silently substitute Maya. The experiment demonstrates no usable configuration yet.

| Output / case | Exact new source | Expected memory | Exact proposal meaning (compact) | Proposed label | Qualification |
| --- | --- | --- | --- | --- | --- |
| Q01 / N02-a | "For the story, write \"I live in Paris.\" Maya also told me she moved there." | No memory | project scope UUID — residence — Paris (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Invents project residence from a story/unendorsed report. The project is not a supported resident, and D1 admits neither the story nor report as that assertion. |
| Q02 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | new:Name — residence — Paris (text; affirmed; fact; time=(unspecified); identity=unresolved; effect=assert) | unsupported | The exact subject is the literal placeholder new:Name, not Maya. Inferring the intended person would repair an ungrounded identity; that is not automatic raw meaning credit. |
| Q03 / N02-b | "Maya, my next-door neighbor, now lives in Paris. I can confirm that myself." | required: Maya (neighbor) lives in Paris; optional: Maya is the owner’s neighbor (both affirmed facts; no date specified) | new:Name — relationship — Maya, my next-door neighbor (text; affirmed; fact; time=(unspecified); identity=unresolved; effect=assert) | unsupported | Again uses literal new:Name and puts Maya’s name plus neighbor description inside the relationship object. This exact tuple does not establish the reviewed Maya-neighbor assertion. |
| Q04 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | project scope UUID — preference — tea (text; affirmed; consideration; time=(unspecified); identity=unresolved; effect=assert) | unsupported | Assigns the personal preference to the project, drops tea-over-coffee qualification and calls it an unresolved consideration. It now carries the question context, which does not repair those meaning errors. |
| Q05 / N03-b | "Yes." | required: owner prefers tea over coffee (comparative standing fact; no date specified) | project scope UUID — consideration — drinks (text; affirmed; consideration; time=(unspecified); identity=unresolved; effect=assert) | unsupported | Turns an old discussion of drinks into a project consideration using only old support. No new assent owns this candidate. |
| Q06 / N04-b | "She has moved to Paris." | No memory | new:Paris — residence — Paris (text; affirmed; fact; time=(unspecified); identity=unresolved; effect=assert) | unsupported | Invents an Entity named Paris as the resident instead of resolving the ambiguous She, and places owner text in non-supporting assistant-context references. |
| Q07 / N04-b | "She has moved to Paris." | No memory | new:Maya Chen — relationship — cousin (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Repeats the old cousin relation with no new support, marks a new Entity resolved and misuses owner text as assistant context. |
| Q08 / N04-b | "She has moved to Paris." | No memory | new:Maya Patel — relationship — colleague (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Repeats the old colleague relation with no new support, marks a new Entity resolved and misuses owner text as assistant context. |
| Q09 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — constraint — Offline operation is a lasting requirement. (text; affirmed; fact; time=(unspecified); identity=resolved; effect=assert) | unsupported | Repeats the old offline constraint using only overlap. The new PostgreSQL consideration supplies no new support for this candidate. |
| Q10 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — consideration — PostgreSQL remains a long-term option I am considering. (text; affirmed; consideration; time=(unspecified); identity=resolved; effect=assert) | optional useful | The project’s unadopted PostgreSQL consideration is supported and optional. Proposed raw semantic credit treats the longer text as equivalent; the project scope UUID still needs the requested literal-project representation. No required-memory recall is claimed. |
| Q11 / N05-b | "For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it." | optional: PostgreSQL remains an unadopted long-term project option (consideration; no date specified) | project scope UUID — decision — For this project we have chosen SQLite. (text; affirmed; decision; time=(unspecified); identity=resolved; effect=assert) | unsupported | Repeats the old SQLite decision using only overlap. The new PostgreSQL consideration supplies no new support for it. |
| Q12 / N06-a | "I no longer work at Acme. I left last month." | required: owner no longer works at Acme; left last month (denied employment; world change; relative time retained) | project scope UUID — employment — Acme (entity; affirmed; world_change; time=last month; identity=resolved; effect=assert) | unsupported | Affirms employment after an explicit departure, assigns it to the project and encodes Acme as an Entity reference without the required Entity identity. Keeping last month and world_change does not repair subject/polarity/type. |
| Q13 / N08-b | "Use the date you just checked: as of that date I have stopped drinking coffee." | required: owner stopped drinking coffee as of 2026-09-04 (denied habit; world change; calendar date only) | owner — preference — stopped drinking coffee (text; affirmed; world_change; time=(unspecified); identity=resolved; effect=assert) | unsupported | Asserts a preference for stopped drinking coffee instead of the denied coffee habit, omits the checked calendar date, and places the clock/old owner instruction in forbidden context references. |

## Exact proposal records

### Q01 — N02-a

Exact proposal SHA-256: `sha256:ce9f58fff49ab6235efc54910e57d601409ee30694c62bf1f67b4949effec577`; occurrences: 2.

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
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Invents project residence from a story/unendorsed report. The project is not a supported resident, and D1 admits neither the story nor report as that assertion.

### Q02 — N02-b

Exact proposal SHA-256: `sha256:4e062b6633e51a8a41cac6ffcafba6b44489b26731f6925aae74cd77552d3437`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
  "object_kind": "text",
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
  "subject": "new:Name",
  "temporal": ""
}
```

The exact subject is the literal placeholder new:Name, not Maya. Inferring the intended person would repair an ungrounded identity; that is not automatic raw meaning credit.

### Q03 — N02-b

Exact proposal SHA-256: `sha256:fb28dd7b2351f4e9ac55acf1be0cbd43f2898820ae12fd0d484834ec5823ba36`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Maya, my next-door neighbor",
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
  "subject": "new:Name",
  "temporal": ""
}
```

Again uses literal new:Name and puts Maya’s name plus neighbor description inside the relationship object. This exact tuple does not establish the reviewed Maya-neighbor assertion.

### Q04 — N03-b

Exact proposal SHA-256: `sha256:1be944024c79d91d382c7097ed24390cad925135a3aa8064ffeab0e0ea49cb56`; occurrences: 2.

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
  "kind": "consideration",
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

Assigns the personal preference to the project, drops tea-over-coffee qualification and calls it an unresolved consideration. It now carries the question context, which does not repair those meaning errors.

### Q05 — N03-b

Exact proposal SHA-256: `sha256:8644f01e3785ca89f411a7d86e53e5ffe5e2a311d86883e0d8ec478f1099283f`; occurrences: 2.

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
  "kind": "consideration",
  "object": "drinks",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "consideration",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 22,
      "event_id": "fec582bc-51f0-52a4-a323-93950fd00118",
      "start": 0
    }
  ],
  "subject": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "temporal": ""
}
```

Turns an old discussion of drinks into a project consideration using only old support. No new assent owns this candidate.

### Q06 — N04-b

Exact proposal SHA-256: `sha256:2a56ce8bc801262d4c22d61f7a7d1b16901b8c8a1fbe46baa9f07324ea112763`; occurrences: 2.

```json
{
  "context": [
    {
      "end": 23,
      "event_id": "f89cfaed-b920-5115-a0ac-371e724aa98a",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "Paris",
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
  "subject": "new:Paris",
  "temporal": ""
}
```

Invents an Entity named Paris as the resident instead of resolving the ambiguous She, and places owner text in non-supporting assistant-context references.

### Q07 — N04-b

Exact proposal SHA-256: `sha256:f36beb262809d70ea0ac081a697ddbb02c7f1d281124dff2ceaced7016065591`; occurrences: 2.

```json
{
  "context": [
    {
      "end": 51,
      "event_id": "2b5f248a-7225-5148-8a68-f5911fe3c31a",
      "start": 0
    }
  ],
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
      "event_id": "2b5f248a-7225-5148-8a68-f5911fe3c31a",
      "start": 0
    }
  ],
  "subject": "new:Maya Chen",
  "temporal": ""
}
```

Repeats the old cousin relation with no new support, marks a new Entity resolved and misuses owner text as assistant context.

### Q08 — N04-b

Exact proposal SHA-256: `sha256:f886e6829fbc3cd945868da9a95ba57a18075071c37d02e6cb61656edbda8dd8`; occurrences: 2.

```json
{
  "context": [
    {
      "end": 51,
      "event_id": "2b5f248a-7225-5148-8a68-f5911fe3c31a",
      "start": 0
    }
  ],
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
      "event_id": "2b5f248a-7225-5148-8a68-f5911fe3c31a",
      "start": 0
    }
  ],
  "subject": "new:Maya Patel",
  "temporal": ""
}
```

Repeats the old colleague relation with no new support, marks a new Entity resolved and misuses owner text as assistant context.

### Q09 — N05-b

Exact proposal SHA-256: `sha256:33802d76957c8c985b1a9456487e2e0da6e3de0f6998dcc41b41d8149ecbe247`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "Offline operation is a lasting requirement.",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "constraint",
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

Repeats the old offline constraint using only overlap. The new PostgreSQL consideration supplies no new support for this candidate.

### Q10 — N05-b

Exact proposal SHA-256: `sha256:4bb802cdb79baf5f35d1c0878db14470dc572d3c558435cefeed339aa0f99826`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "consideration",
  "object": "PostgreSQL remains a long-term option I am considering.",
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

The project’s unadopted PostgreSQL consideration is supported and optional. Proposed raw semantic credit treats the longer text as equivalent; the project scope UUID still needs the requested literal-project representation. No required-memory recall is claimed.

### Q11 — N05-b

Exact proposal SHA-256: `sha256:584083dba78503c6f184925697fb8a883f79fe521c837197e223c8b26e09e63f`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "For this project we have chosen SQLite.",
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

Repeats the old SQLite decision using only overlap. The new PostgreSQL consideration supplies no new support for it.

### Q12 — N06-a

Exact proposal SHA-256: `sha256:ff93711a44679e3cfb5d3ab22d224e6a34a8dd253fa91278a94e23904ff84566`; occurrences: 2.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "world_change",
  "object": "Acme",
  "object_kind": "entity",
  "polarity": "affirmed",
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
  "temporal": "last month"
}
```

Affirms employment after an explicit departure, assigns it to the project and encodes Acme as an Entity reference without the required Entity identity. Keeping last month and world_change does not repair subject/polarity/type.

### Q13 — N08-b

Exact proposal SHA-256: `sha256:851f1668b03622b0e132a2d2f1bceac1249f0925344f6908e622828fbce21b2a`; occurrences: 2.

```json
{
  "context": [
    {
      "end": 19,
      "event_id": "edc787df-49e6-5c93-a135-6aa6b7619aa0",
      "start": 0
    },
    {
      "end": 28,
      "event_id": "e8cddf05-9db4-550f-aeda-a016becc0cd7",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "resolved",
  "kind": "world_change",
  "object": "stopped drinking coffee",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "preference",
  "scope": "project:553ecf4c-6a4f-50d4-94e1-8c37985464a7",
  "sources": [
    {
      "end": 78,
      "event_id": "eb55a65e-dace-573b-8a85-522b88a4f74d",
      "start": 0
    }
  ],
  "subject": "owner",
  "temporal": ""
}
```

Asserts a preference for stopped drinking coffee instead of the denied coffee habit, omits the checked calendar date, and places the clock/old owner instruction in forbidden context references.
