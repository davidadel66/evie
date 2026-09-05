# Compact Qwen output review packet

Status: proposed, not human reviewed. No judgments in this packet have been applied.

This packet contains all 12 novel raw objects from the ten-request compact-v1 run. The source corpus/gold were previously human approved; this is a separate review of actual model outputs. All objects were structurally rejected. The score therefore has zero exact canonical matches, while all raw semantic judgments remain pending.

The frozen schema mistakenly permits independently optional selector/start/end fields. Every object emitted an incomplete or incompatible reference under the adapter contract. This schema design flaw does not prove every underlying meaning wrong. The existing unsupported taxonomy also covers overlap-only repetitions that are ineligible as new proposals; it does not call those historical truths fabricated. Proposed raw semantic credit below never repairs a locator, creates retained output, or changes accepted memory.

Raw wire values below are formatted for reading; the exact model text and full request/alias seals remain in [the run report](reports/development.json). Source strings are shown as JSON values, preserving their exact decoded bytes. The [initial score](reports/development-initial-score.json) supplies case plus raw-wire SHA-256 identities.

Gold SHA-256: `sha256:78590a0e3a06f8a2510762d96957e441f7ab4b9c9d4bf964bfa0dc5cc9c81b94`. Initial score SHA-256: `sha256:503cad16e1baa77dad310216c1307b963bd2540f34bdd1a758f1271b70596efd`.

Please review each proposed label, errors and any required gold match. Café equivalence, the coffee-change raw wording despite its typed errors, and optional PostgreSQL raw credit are explicit judgment questions; none inherits the source-label approval.

| ID | Case | Proposed raw interpretation | Proposed label | Main reason / error |
| --- | --- | --- | --- | --- |
| W01 | N01-a | Owner prefers tea | required_useful | Explicit meaning; invalid locator only. |
| W02 | N01-b | Repeats the older tea preference | unsupported | No newly owned support; identity flag also wrong. |
| W03 | N02-a | Owner lives in Paris | unsupported | Source is fiction/unendorsed Maya report, not owner residence. |
| W04 | N02-b | Owner lives in Paris | unsupported | Endorsement concerns Maya; subject is wrong. |
| W05 | N03-b | Owner prefers tea as a decision | unsupported | Loses tea-over-coffee comparison and required question context. |
| W06 | N04-b | Owner lives in Paris | unsupported | She is ambiguous between two Mayas, not the owner. |
| W07 | N05-b | Project requires offline operation | unsupported | Old overlap-only truth; no newly owned support. |
| W08 | N05-b | Project adopts SQLite | unsupported | Old overlap-only decision; no newly owned support. |
| W09 | N05-b | Project considers PostgreSQL | optional_useful | Optional unadopted meaning; identity flag and locator need correction. |
| W10 | N06-a | Owner employment / employment, affirmed | unsupported | Loses Acme, departure polarity and world-change encoding. |
| W11 | N08-b | Owner stopped coffee as of checked 2026-09-04 | required_useful | Raw dated change is explicit; predicate/kind/category/locator remain wrong. |
| W12 | N09-a | Owner does not prefer café | required_useful | Human equivalence question: café versus café ☕; invalid locator. |

## W01 — N01-a

Proposed label: **required_useful**. Proposed errors: source_attribution.

The explicit owner/tea meaning is supported by s1. Proposed raw semantic credit only; its dangling start coordinate is not a valid citation and remains rejected. Human approval would not repair the reference or create retained credit.

Raw identity: `sha256:d1c203b4e80d79be453924ba329902dab606af0e2001e9e4f4bff79e2b1844f9`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

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
  "sources": [
    {
      "ref": "s1",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: new, owner_statement, 0d8102f9-c956-5110-82d9-4853195285fd content 0:13; observed 2026-09-04T13:30:00Z; `sha256:0b848d5bacc4fe482bebcc7651d89f6c1ecf683773e0e528077bad5f36a059e2`.

```json
"I prefer tea."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "owner",
      "predicate": "preference",
      "object_kind": "text",
      "object": "tea",
      "polarity": "affirmed",
      "kind": "fact",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "0d8102f9-c956-5110-82d9-4853195285fd",
        "start": 0,
        "end": 13
      }
    ],
    "context": []
  }
]
```

## W02 — N01-b

Proposed label: **unsupported**. Proposed errors: identity, source_attribution.

Repeats tea from s1 overlap when the newly owned source is an incidental pear lunch. No new support owns this preference. It also marks the explicit owner/text tuple unresolved and emits an invalid reference. Here unsupported means an ineligible repeated truth under newly owned evidence rules, not a fabricated historical fact.

Raw identity: `sha256:2d3b37e4c5a831da7b7d0dd3c5027cb399c11d11f67c7e42f06041e4d405c557`. Occurrences: 1. First structural rejection: `invalid_subject: incompatible identity`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "fact",
  "object": "tea",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s1",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 0d8102f9-c956-5110-82d9-4853195285fd content 0:13; observed 2026-09-04T13:30:00Z; `sha256:0b848d5bacc4fe482bebcc7651d89f6c1ecf683773e0e528077bad5f36a059e2`.

```json
"I prefer tea."
```

- `s2`: new, owner_statement, 155add8e-9dfd-5e37-a055-65759c3b688d content 0:22; observed 2026-09-04T13:30:00Z; `sha256:1eaa90441b1cbb74086377d882716796752383cfea7dca5cc067178d4a0a56d7`.

```json
"I ate a pear at lunch."
```

Previously approved case expectations:

```json
[]
```

## W03 — N02-a

Proposed label: **unsupported**. Proposed errors: identity, source_attribution.

Treats the story/unendorsed report as the owner living in Paris. Neither passage asserts the owner residence. The source locator is also incomplete.

Raw identity: `sha256:fb7cf0022334c3433d23a853b4e8b83901f422c110f275630ba433ceb118e4bc`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

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
  "sources": [
    {
      "ref": "s1",
      "start": 35
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: new, owner_statement, 39878038-6df9-5dac-8e20-f25238f36882 content 0:74; observed 2026-09-04T13:30:00Z; `sha256:f0928ace258d2d73efb3084da334d9607959e28affd2f9cec59d6a62d891c338`.

```json
"For the story, write \"I live in Paris.\" Maya also told me she moved there."
```

Previously approved case expectations:

```json
[]
```

Excluded interpretation: Neither owner residence nor Maya residence nor an attributed-report Claim is supported under D1.

## W04 — N02-b

Proposed label: **unsupported**. Proposed errors: identity, source_attribution.

The exact subject is owner, while the new assertion concerns Maya the neighbor. Changing owner to Maya would repair a wrong identity, not merely transport. The reference is incomplete.

Raw identity: `sha256:809a00fd8560562ec35e9b81ffc79f4af4197e3d1c743623a767cb3ed3982f2a`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

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
  "sources": [
    {
      "ref": "s2",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 39878038-6df9-5dac-8e20-f25238f36882 content 0:74; observed 2026-09-04T13:30:00Z; `sha256:f0928ace258d2d73efb3084da334d9607959e28affd2f9cec59d6a62d891c338`.

```json
"For the story, write \"I live in Paris.\" Maya also told me she moved there."
```

- `s2`: new, owner_statement, eaa07e15-ca81-5ffe-b8e3-c7519f474b93 content 0:75; observed 2026-09-04T13:30:00Z; `sha256:4e701d36372a437c4ee48f46ff53c0d3490267c5e4756679bd4eb4f717ac0ad8`.

```json
"Maya, my next-door neighbor, now lives in Paris. I can confirm that myself."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "new:Maya (neighbor)",
      "predicate": "residence",
      "object_kind": "text",
      "object": "Paris",
      "polarity": "affirmed",
      "kind": "fact",
      "temporal": "",
      "identity": "unresolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "eaa07e15-ca81-5ffe-b8e3-c7519f474b93",
        "start": 0,
        "end": 75
      }
    ],
    "context": []
  },
  {
    "label": "optional_useful",
    "meaning": {
      "subject": "new:Maya (neighbor)",
      "predicate": "relationship",
      "object_kind": "text",
      "object": "neighbor",
      "polarity": "affirmed",
      "kind": "fact",
      "temporal": "",
      "identity": "unresolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "eaa07e15-ca81-5ffe-b8e3-c7519f474b93",
        "start": 0,
        "end": 75
      }
    ],
    "context": []
  }
]
```

## W05 — N03-b

Proposed label: **unsupported**. Proposed errors: typed_meaning, source_attribution.

Drops the tea-over-coffee comparison, represents a decision instead of a standing fact, omits the required assistant question context, and supplies an incomplete citation for Yes.

Raw identity: `sha256:49ba4078625819bc9370aed2fa9747d5416508e7d054ae65d003243ebfd2c0fa`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s3",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, fec582bc-51f0-52a4-a323-93950fd00118 content 0:22; observed 2026-09-04T13:30:00Z; `sha256:c52d1a2b546bb6a49005c279babcf0f4cbf3a8841b0bdfd244acb4f2a953afd9`.

```json
"Let us discuss drinks."
```

- `s2`: context, none, 067d03f1-f474-52a7-ba4b-7f678dc5fead content 0:28; observed 2026-09-04T13:30:00Z; `sha256:7554d91ba8ce18ef922fbc9f4c587d8e0369e9ecd98a8046e6cea466d97d4ec3`.

```json
"Do you prefer tea to coffee?"
```

- `s3`: new, owner_statement, 0935c3b6-ecbb-571d-88e9-54711ce43e30 content 0:4; observed 2026-09-04T13:30:00Z; `sha256:5f9a2b795615ba6a3d5455fd5624d773fbca5bcd16249c421fd37411dc9837da`.

```json
"Yes."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "owner",
      "predicate": "preference",
      "object_kind": "text",
      "object": "tea over coffee",
      "polarity": "affirmed",
      "kind": "fact",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "0935c3b6-ecbb-571d-88e9-54711ce43e30",
        "start": 0,
        "end": 4
      }
    ],
    "context": [
      {
        "event_id": "067d03f1-f474-52a7-ba4b-7f678dc5fead",
        "start": 0,
        "end": 28
      }
    ]
  }
]
```

## W06 — N04-b

Proposed label: **unsupported**. Proposed errors: identity, source_attribution.

Assigns residence to owner even though She is ambiguous between Maya Chen and Maya Patel. The adapter must not resolve it to the owner; its nominated citation is also incomplete.

Raw identity: `sha256:809a00fd8560562ec35e9b81ffc79f4af4197e3d1c743623a767cb3ed3982f2a`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

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
  "sources": [
    {
      "ref": "s2",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 2b5f248a-7225-5148-8a68-f5911fe3c31a content 0:51; observed 2026-09-04T13:30:00Z; `sha256:3b9c154037632ff6c62694d266d50c1b2f73e9860a5d8943dd75b730daba70c3`.

```json
"Maya Chen is my cousin. Maya Patel is my colleague."
```

- `s2`: new, owner_statement, f89cfaed-b920-5115-a0ac-371e724aa98a content 0:23; observed 2026-09-04T13:30:00Z; `sha256:cdc2c8ccccb3af63b222bb46880aedaf16baf69d2ae98ab6a016c06ae69eb57b`.

```json
"She has moved to Paris."
```

Previously approved case expectations:

```json
[]
```

Excluded interpretation: The pronoun has two plausible antecedents. Neither Maya receives a residence/move assertion.

## W07 — N05-b

Proposed label: **unsupported**. Proposed errors: source_attribution.

Repeats the old offline constraint from overlap s1. The new PostgreSQL consideration supplies no newly owned support for this separate claim; the reference is also incomplete. Here unsupported means an ineligible repeated truth under newly owned evidence rules, not a fabricated historical fact.

Raw identity: `sha256:6961d8a8b3e156be3d18c46c7a3d3e47e4e3b31677a4339f7b69fad78d4c4ebc`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "offline operation is a lasting requirement",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "constraint",
  "sources": [
    {
      "ref": "s1",
      "start": 58
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 49290f19-3f35-50aa-b775-cf97d4591adc content 0:83; observed 2026-09-04T13:30:00Z; `sha256:b1c638915131496bd0157607e3f01edd6dddf8963fdac03cd31a033acfd97b7a`.

```json
"For this project we have chosen SQLite. Offline operation is a lasting requirement."
```

- `s2`: new, owner_statement, 0b9cd9f7-c1e4-595e-98f5-8044d8340602 content 0:98; observed 2026-09-04T13:30:00Z; `sha256:2dc63a1b1fe684023f89662c475518ef26282b3802d1dcaacdb4b207507b7cf1`.

```json
"For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."
```

Previously approved case expectations:

```json
[
  {
    "label": "optional_useful",
    "meaning": {
      "subject": "project",
      "predicate": "consideration",
      "object_kind": "text",
      "object": "PostgreSQL",
      "polarity": "affirmed",
      "kind": "consideration",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "0b9cd9f7-c1e4-595e-98f5-8044d8340602",
        "start": 0,
        "end": 98
      }
    ],
    "context": []
  }
]
```

Excluded interpretation: An adopted PostgreSQL decision is unsupported. Abstention on the optional consideration passes.

## W08 — N05-b

Proposed label: **unsupported**. Proposed errors: source_attribution.

Repeats the old SQLite decision from overlap s1. The new PostgreSQL consideration supplies no newly owned support for this separate claim; the reference is also incomplete. Here unsupported means an ineligible repeated truth under newly owned evidence rules, not a fabricated historical fact.

Raw identity: `sha256:8321b876a4f9bfe45153d37c440f114391c63d052a44024cd8dc3f91376389ce`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

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
  "sources": [
    {
      "ref": "s1",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 49290f19-3f35-50aa-b775-cf97d4591adc content 0:83; observed 2026-09-04T13:30:00Z; `sha256:b1c638915131496bd0157607e3f01edd6dddf8963fdac03cd31a033acfd97b7a`.

```json
"For this project we have chosen SQLite. Offline operation is a lasting requirement."
```

- `s2`: new, owner_statement, 0b9cd9f7-c1e4-595e-98f5-8044d8340602 content 0:98; observed 2026-09-04T13:30:00Z; `sha256:2dc63a1b1fe684023f89662c475518ef26282b3802d1dcaacdb4b207507b7cf1`.

```json
"For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."
```

Previously approved case expectations:

```json
[
  {
    "label": "optional_useful",
    "meaning": {
      "subject": "project",
      "predicate": "consideration",
      "object_kind": "text",
      "object": "PostgreSQL",
      "polarity": "affirmed",
      "kind": "consideration",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "0b9cd9f7-c1e4-595e-98f5-8044d8340602",
        "start": 0,
        "end": 98
      }
    ],
    "context": []
  }
]
```

Excluded interpretation: An adopted PostgreSQL decision is unsupported. Abstention on the optional consideration passes.

## W09 — N05-b

Proposed label: **optional_useful**. Proposed errors: identity, source_attribution.

The explicit project PostgreSQL consideration is supported and optional. Proposed raw semantic credit keeps it unadopted; the unresolved identity flag conflicts with the explicit project/text tuple, and the citation is incomplete. No required recall credit is proposed.

Raw identity: `sha256:99130053022f8a39cff5eaed983c7ba16f8213eddfd0579116bf6e086ca4f53d`. Occurrences: 1. First structural rejection: `invalid_subject: incompatible identity`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "PostgreSQL",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "consideration",
  "sources": [
    {
      "ref": "s2",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, 49290f19-3f35-50aa-b775-cf97d4591adc content 0:83; observed 2026-09-04T13:30:00Z; `sha256:b1c638915131496bd0157607e3f01edd6dddf8963fdac03cd31a033acfd97b7a`.

```json
"For this project we have chosen SQLite. Offline operation is a lasting requirement."
```

- `s2`: new, owner_statement, 0b9cd9f7-c1e4-595e-98f5-8044d8340602 content 0:98; observed 2026-09-04T13:30:00Z; `sha256:2dc63a1b1fe684023f89662c475518ef26282b3802d1dcaacdb4b207507b7cf1`.

```json
"For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it."
```

Previously approved case expectations:

```json
[
  {
    "label": "optional_useful",
    "meaning": {
      "subject": "project",
      "predicate": "consideration",
      "object_kind": "text",
      "object": "PostgreSQL",
      "polarity": "affirmed",
      "kind": "consideration",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "0b9cd9f7-c1e4-595e-98f5-8044d8340602",
        "start": 0,
        "end": 98
      }
    ],
    "context": []
  }
]
```

Excluded interpretation: An adopted PostgreSQL decision is unsupported. Abstention on the optional consideration passes.

## W10 — N06-a

Proposed label: **unsupported**. Proposed errors: polarity, typed_meaning, source_attribution.

Affirms a generic employment object after an explicit departure from Acme. The employer is lost and kind=fact replaces the reviewed world_change. Keeping last month does not repair meaning or the incomplete citation.

Raw identity: `sha256:0e6525b5b1c1d2448149efbcbec5345d8f8ee203683092593413c160c74437f0`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "employment",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "employment",
  "sources": [
    {
      "ref": "s1",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": "last month"
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: new, owner_statement, 71442dec-2d3c-50d4-a649-498937e18b6a content 0:44; observed 2026-09-04T13:30:00Z; `sha256:cc126513ae425f5819871d418eb7c0d9311b08187b949b6f2e70b64736acfe27`.

```json
"I no longer work at Acme. I left last month."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "owner",
      "predicate": "employment",
      "object_kind": "text",
      "object": "Acme",
      "polarity": "denied",
      "kind": "world_change",
      "temporal": "last month",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "71442dec-2d3c-50d4-a649-498937e18b6a",
        "start": 0,
        "end": 44
      }
    ],
    "context": []
  }
]
```

## W11 — N08-b

Proposed label: **required_useful**. Proposed errors: predicate, typed_meaning, source_attribution.

The raw owner wording explicitly says stopped drinking coffee as of the checked 2026-09-04 date. Proposed raw semantic credit recognizes that stated change, subject to human judgment. The encoded constraint/fact tuple is not the reviewed habit/world_change representation; the clock is wrongly placed in assistant context and whole/date references carry forbidden start coordinates. Raw approval repairs none of these structural or typed errors and creates no retained credit.

Raw identity: `sha256:eb537c5762213f11d845a43bdc87c839c8da1f30ac53e790178c463a8ba9a4a3`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

```json
{
  "context": [
    {
      "ref": "s2",
      "selector": "date",
      "start": 0
    }
  ],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "stopped drinking coffee",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "constraint",
  "sources": [
    {
      "ref": "s3",
      "selector": "whole",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": "as of 2026-09-04"
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: overlap, owner_statement, e8cddf05-9db4-550f-aeda-a016becc0cd7 content 0:28; observed 2026-09-04T13:30:00Z; `sha256:ecd98d1de10bf574e828b3dd6fcad7d847a0d23d1852a3db2f5a67d81e22fb86`.

```json
"Check the local date for me."
```

- `s2`: overlap, tool_observation, edc787df-49e6-5c93-a135-6aa6b7619aa0 content 0:19; observed 2026-09-04T13:30:00Z; `sha256:9e1d9445a52d8465e4b69a3c137d2130e5cc06ca37c39229a1deff54609381e1`.

```json
"2026-09-04 09:30:00"
```

- `s3`: new, owner_statement, eb55a65e-dace-573b-8a85-522b88a4f74d content 0:78; observed 2026-09-04T13:30:00Z; `sha256:2a5a4fcfe445308a4a97b52746d173eea1eff78e58b3d5c552d739a7a75e6989`.

```json
"Use the date you just checked: as of that date I have stopped drinking coffee."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "owner",
      "predicate": "habit",
      "object_kind": "text",
      "object": "drinking coffee",
      "polarity": "denied",
      "kind": "world_change",
      "temporal": "2026-09-04",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "eb55a65e-dace-573b-8a85-522b88a4f74d",
        "start": 0,
        "end": 78
      },
      {
        "event_id": "edc787df-49e6-5c93-a135-6aa6b7619aa0",
        "start": 0,
        "end": 10
      }
    ],
    "context": []
  }
]
```

## W12 — N09-a

Proposed label: **required_useful**. Proposed errors: source_attribution.

Proposed raw equivalence treats café as the same preference target as the approved café ☕ wording, with the emoji decorative. This equivalence specifically needs human judgment. Its dangling start coordinate remains invalid and cannot be repaired by approval.

Raw identity: `sha256:4ec578a61caeaeaff3209212ba9aad1d16df88153c2d566f37ec3eca735e81b8`. Occurrences: 1. First structural rejection: `invalid_selector: whole selector has coordinates`.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "café",
  "object_kind": "text",
  "polarity": "denied",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s1",
      "start": 0
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

Offered source aliases (the full original source identity and hash remain sealed):

- `s1`: new, owner_statement, 6b4b9ad5-f436-5e1f-b92a-6eab9543c666 content 0:26; observed 2026-09-04T13:30:00Z; `sha256:00479e7e524eb59e2c69c612d9c9baac37cd023a7da917279c74206d5822660e`.

```json
"I do not prefer café ☕."
```

Previously approved case expectations:

```json
[
  {
    "label": "required_useful",
    "meaning": {
      "subject": "owner",
      "predicate": "preference",
      "object_kind": "text",
      "object": "café ☕",
      "polarity": "denied",
      "kind": "fact",
      "temporal": "",
      "identity": "resolved",
      "effect": "assert"
    },
    "sources": [
      {
        "event_id": "6b4b9ad5-f436-5e1f-b92a-6eab9543c666",
        "start": 0,
        "end": 26
      }
    ],
    "context": []
  }
]
```
