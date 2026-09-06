# Readable review: 13 model proposals

This is a reading aid for the existing compact-v3 packet. All judgments remain proposed and unapplied. These are synthetic conversations: “owner” means the speaker in the example, not an assertion about David.

## How to read each card

1. Read the exact evidence. **Earlier** messages were supplied as context from an older interval; **New** messages belong to the interval being compiled. **Assistant context** can explain a reply but cannot independently establish an owner fact.
2. Compare it with the plain-English rendering of the actual model fields. These renderings are explanations, not model-generated sentences. Conflicts and omissions are preserved explicitly.
3. Check the proposed grade and explanation. Tell me the card ID and what you disagree with. You do not need to interpret the JSON.

**Passed validation** means eligible to appear as a candidate, not accepted as memory or established as true. **Blocked** means the validator stopped it; we still grade the raw model output to understand its mistakes.

Grades: **Required and useful** matches a memory the approved labels require; **Optional and useful** is supported and permitted but not required; **Unsupported / ineligible** is wrong or not eligible as a new proposal; **True but unwanted** is accurate information we do not want retained. A repeated older truth can be ineligible without being false.

In this run there were 14 raw proposals and 9 passed validation. One proposal matched approved gold exactly; these 13 are the remaining human judgments. Eight of these 13 passed validation.

[Original packet and exact identifiers](/Users/davidboktor/code/evie/cmd/evie/docs/fixtures/memory-stage-4-spike/v1/qwen-compact-v3/output-adjudication-packet.md)

## C01 · N01-b

**Earlier · owner**

> I prefer tea.

**New · owner**

> I ate a pear at lunch.

**Proposed memory, rendered from the actual fields**

The owner prefers tea. (Fact.)

**Validator result:** Blocked; did not become a reviewable candidate.

**Proposed grade:** Unsupported / ineligible.

Repeats the older tea preference from overlap alone. The newly owned pear meal supplies no support for this preference. Unsupported here means an ineligible repeated truth, not a fabricated historical preference.

<details>
<summary>Approved expectation and exact model fields</summary>

no new memory. The pear is true but unwanted. The old preference alone cannot own a new candidate.

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
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C02 · N02-a

**New · owner**

> For the story, write "I live in Paris." Maya also told me she moved there.

**Proposed memory, rendered from the actual fields**

The owner lives in Paris. (Fact.)

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but treats a fictional first-person line and an unendorsed Maya report as the owner living in Paris. Exact source copying does not supply the missing owner assertion.

<details>
<summary>Approved expectation and exact model fields</summary>

no new memory. Neither owner residence nor Maya residence nor an attributed-report Claim is supported under D1.

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
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C03 · N02-a

**New · owner**

> For the story, write "I live in Paris." Maya also told me she moved there.

**Proposed memory, rendered from the actual fields**

The owner is considering moving to Paris. The model classifies this as a decision, despite using a consideration predicate.

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but the owner never asserts a decision or enduring consideration to move to Paris. The source concerns a story and Maya's unendorsed report; changing it into the owner's choice is unsupported.

<details>
<summary>Approved expectation and exact model fields</summary>

no new memory. Neither owner residence nor Maya residence nor an attributed-report Claim is supported under D1.

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "move to Paris",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "consideration",
  "sources": [
    {
      "ref": "s1",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C04 · N02-b

**Earlier · owner**

> For the story, write "I live in Paris." Maya also told me she moved there.

**New · owner**

> Maya, my next-door neighbor, now lives in Paris. I can confirm that myself.

**Proposed memory, rendered from the actual fields**

The owner lives in Paris. (Fact.)

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but the new endorsement concerns Maya the neighbor, not the owner. The subject is explicitly owner; no new-Entity identity is proposed.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: new:Maya (neighbor) residence=Paris (affirmed, fact; temporal ''); optional_useful: new:Maya (neighbor) relationship=neighbor (affirmed, fact; temporal '').

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
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C05 · N03-b

**Earlier · owner**

> Let us discuss drinks.

**Assistant context**

> Do you prefer tea to coffee?

**New · owner**

> Yes.

**Proposed memory, rendered from the actual fields**

The owner prefers tea. The model classifies this as a decision; it does not preserve “over coffee.”

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained with correct assistant question context, but preference=tea and kind=decision lose the reviewed standing tea-over-coffee comparison/fact encoding. This raw object is byte-for-byte identical after canonical JSON encoding to pending compact-v2 V07; no second independent judgment or automatic approval is inferred.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: owner preference=tea over coffee (affirmed, fact; temporal '').

```json
{
  "context": [
    {
      "ref": "s2",
      "selector": "whole"
    }
  ],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "tea",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s3"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C06 · N04-b

**Earlier · owner**

> Maya Chen is my cousin. Maya Patel is my colleague.

**New · owner**

> She has moved to Paris.

**Proposed memory, rendered from the actual fields**

The owner lives in Paris. (Fact.)

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but She is ambiguous between Maya Chen and Maya Patel. The output instead assigns Paris residence to the owner, which the source does not assert.

<details>
<summary>Approved expectation and exact model fields</summary>

no new memory. The pronoun has two plausible antecedents. Neither Maya receives a residence/move assertion.

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
      "ref": "s2"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C07 · N05-b

**Earlier · owner**

> For this project we have chosen SQLite. Offline operation is a lasting requirement.

**New · owner**

> For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it.

**Proposed memory, rendered from the actual fields**

Offline operation is a requirement for this project. (Fact.)

**Validator result:** Blocked; did not become a reviewable candidate.

**Proposed grade:** Unsupported / ineligible.

The offline requirement is an older true project constraint, supported only by overlap. The new PostgreSQL consideration does not own this separate constraint; no new candidate is eligible.

<details>
<summary>Approved expectation and exact model fields</summary>

optional_useful: project consideration=PostgreSQL (affirmed, consideration; temporal '').

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "offline operation",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "constraint",
  "sources": [
    {
      "ref": "s1",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

</details>

---

## C08 · N05-b

**Earlier · owner**

> For this project we have chosen SQLite. Offline operation is a lasting requirement.

**New · owner**

> For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it.

**Proposed memory, rendered from the actual fields**

PostgreSQL is a consideration for this project. Conflicting field: the predicate says decision. The project identity is also marked unresolved.

**Validator result:** Blocked; did not become a reviewable candidate.

**Proposed grade:** Optional and useful (raw meaning only).

Proposed optional raw credit recognizes the explicitly unadopted PostgreSQL consideration. The exact predicate=decision conflicts with kind=consideration, and project/text is incorrectly unresolved. Human judgment is needed for raw credit; approval would not repair identity, create retention or supply required recall.

<details>
<summary>Approved expectation and exact model fields</summary>

optional_useful: project consideration=PostgreSQL (affirmed, consideration; temporal '').

```json
{
  "context": [],
  "effect": "assert",
  "identity": "unresolved",
  "kind": "consideration",
  "object": "PostgreSQL",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "decision",
  "sources": [
    {
      "ref": "s2",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

</details>

---

## C09 · N05-b

**Earlier · owner**

> For this project we have chosen SQLite. Offline operation is a lasting requirement.

**New · owner**

> For future storage, PostgreSQL remains a long-term option I am considering. I have not adopted it.

**Proposed memory, rendered from the actual fields**

The project prefers SQLite. The model classifies this as a decision.

**Validator result:** Blocked; did not become a reviewable candidate.

**Proposed grade:** Unsupported / ineligible.

Repeats the old SQLite decision from overlap as a preference. No newly owned source supports this separate old decision; the newly owned PostgreSQL option does not change that ownership.

<details>
<summary>Approved expectation and exact model fields</summary>

optional_useful: project consideration=PostgreSQL (affirmed, consideration; temporal '').

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "SQLite",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s1",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "project",
  "temporal": ""
}
```

</details>

---

## C10 · N06-a

**New · owner**

> I no longer work at Acme. I left last month.

**Proposed memory, rendered from the actual fields**

The owner works at Acme — affirmative fact, with the time label “last month.” It does not encode leaving Acme.

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but emits affirmative employment/work at Acme as fact with last month, instead of the explicit departure/denied world-change meaning. If last month is read purely as past employment, that is a different historical proposition and still omits the required departure. This distinction is for actual human judgment; the proposed errors are not scored totals.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: owner employment=Acme (denied, world_change; temporal 'last month').

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "fact",
  "object": "work at Acme",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "employment",
  "sources": [
    {
      "ref": "s1",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": "last month"
}
```

</details>

---

## C11 · N08-b

**Earlier · owner**

> Check the local date for me.

**Earlier · tool**

> 2026-09-04 09:30:00

**New · owner**

> Use the date you just checked: as of that date I have stopped drinking coffee.

**Proposed memory, rendered from the actual fields**

The owner prefers “local date.” The model types that value as a date and asks to attach support to an existing claim.

**Validator result:** Blocked; did not become a reviewable candidate.

**Proposed grade:** Unsupported / ineligible.

A clock-only overlap reference cannot establish an owner preference. local date is not a typed calendar date value, and attach_support has no offered equal accepted Claim. It has no newly owned owner support and does not represent the coffee change.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: owner habit=drinking coffee (denied, world_change; temporal '2026-09-04').

```json
{
  "context": [],
  "effect": "attach_support",
  "identity": "resolved",
  "kind": "fact",
  "object": "local date",
  "object_kind": "date",
  "polarity": "affirmed",
  "predicate": "preference",
  "sources": [
    {
      "ref": "s2",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---

## C12 · N08-b

**Earlier · owner**

> Check the local date for me.

**Earlier · tool**

> 2026-09-04 09:30:00

**New · owner**

> Use the date you just checked: as of that date I have stopped drinking coffee.

**Proposed memory, rendered from the actual fields**

The owner decided to stop drinking coffee, as of 2026-09-04. It encodes a decision, not a completed habit change.

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Unsupported / ineligible.

Retained, but represents a decision to stop coffee rather than the asserted completed habit change. Its exact calendar date is taken from the clock without citing that required clock support; only the owner's relative reference is cited. Whether this action/date wording merits partial raw useful credit despite those defects is explicitly left for human review; no required recall match is proposed.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: owner habit=drinking coffee (denied, world_change; temporal '2026-09-04').

```json
{
  "context": [],
  "effect": "assert",
  "identity": "resolved",
  "kind": "decision",
  "object": "stop drinking coffee",
  "object_kind": "text",
  "polarity": "affirmed",
  "predicate": "decision",
  "sources": [
    {
      "ref": "s3",
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": "as of 2026-09-04"
}
```

</details>

---

## C13 · N09-a

**New · owner**

> I do not prefer café ☕.

**Proposed memory, rendered from the actual fields**

The owner does not prefer café. (Fact.) The emoji from the source is omitted.

**Validator result:** Passed validation; still only a candidate.

**Proposed grade:** Required and useful (equivalence needs your judgment).

Proposed equivalence treats café as the approved café ☕ preference target with a decorative emoji. Denied polarity, whole exact Unicode source, owner and fact encoding are preserved. This is the same unresolved equivalence question as compact-v2 V15, not a new automatic match. Actual human approval is still required.

<details>
<summary>Approved expectation and exact model fields</summary>

required_useful: owner preference=café ☕ (denied, fact; temporal '').

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
      "selector": "whole"
    }
  ],
  "subject_entity_ref": "",
  "subject_name": "",
  "subject_type": "owner",
  "temporal": ""
}
```

</details>

---
