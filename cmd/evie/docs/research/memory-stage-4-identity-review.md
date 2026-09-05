# Stage 4 identity and Predicate review (#141)

This implements the approved identity/Predicate ticket through the existing
compiler and owner-review boundary. It does not select or configure a model.
Fixtures use scripted extraction and real SQLite persistence.

## Contract and compatibility

`identity-review-v2` must be selected together for the generation's Entity,
Predicate, validation, equivalence, and effect policies. Evidence, secret,
closure, and window policies remain `owner-assertions-v1`. Accepted Alias context
is bounded to 32 records and 8 KiB, scoped to already offered Entities. The
sealed request identifies the v2 interpretation policy. Old requests and
proposals omit new fields; their canonical JSON and the v1 review golden
fixture remain unchanged.

A candidate may carry a sourced subject/object mention and/or a proposed
Predicate definition. A mention's exact supporting projection must appear in
candidate support and contain the name verbatim. Unresolved references cannot
also carry a model-selected stable ID. This checks structural support, not
entailment. Confidence and uncertainty remain visible interpretation metadata.

Owner alternatives use exact IDs, case-insensitive canonical names, and existing
normalized Alias equality. Up to 32 distinct Entities remain visible, including
same-name people, with matching Alias provenance and up to four contextual
accepted Claims in authorized scopes. Oversized alternatives fail visibly.
There is no similarity score, automatic merge, or inference from name to person.

The owner chooses an offered existing Entity or creates a distinct Entity. A
Predicate choice reuses an exactly equal definition or approves a new token and
complete definition. Existing tokens cannot be redefined through this path.
New definitions are disclosed global structural effects; Claims, new ordinary
Entities, Aliases, and original source remain in the candidate destination.
Broader reuse still requires explicit Promotion.

## Review and persistence

Alternatives carry a digest bound to the exact candidate and scope revisions.
Choosing appends an immutable owner interpretation and audit, advances
interpretation/review revisions with compare-and-swap, and retains original
extractor bytes. Earlier interpretations remain available by exact revision.
Owner choice is never added as a factual source.

Preparation discloses `owner-review-preview-v2` / `owner-review-effect-v2` with
generated IDs, chosen alternatives, complete Predicate definition, Entity/Alias
creates and reuses, canonical equal-Claim reuse, Source Links, uncertainty, and
original support/context. Global Predicate creation and destination changes use
one atomic v6 accepted Semantic Operation and the existing scope revision
vector. V2 preview/effect/operation encodings have new canonical domain strings;
old v1 v6 operations retain their original bytes and replay behavior.

Changed alternatives, scope revisions, owner choices, source policy, or source
eligibility prevent using an old preview. A Source Link write failure rolls back
the whole dependent operation, including new identities and Predicate, Claim,
audit, and resolution. Historical replay verifies recorded effects and original
sources without invoking the extractor or owner. Explicit Promotion retains
source-policy redaction for new relationships.

## CLI demonstration

Use a v2 generation or the scripted CLI fixture to produce a candidate. Review
stays available after the source session closes.

1. `evie memory-review inspect --scope SCOPE --id CANDIDATE`
2. `evie memory-review alternatives --scope SCOPE --id CANDIDATE --revision R --interpretation I`
3. `evie memory-review choose --scope SCOPE --id CANDIDATE --revision R --interpretation I --options OPTIONS_SHA256 --choices JSON`
4. `evie memory-review prepare --scope SCOPE --id CANDIDATE --revision NEW_R --interpretation NEW_I --action accept`
5. `evie memory-review resolve --scope SCOPE --preview PREVIEW --digest PREVIEW_SHA256 --delivery idem:v1:UUID --action accept`
6. `evie memory-review operation --scope SCOPE --id OPERATION`
7. `evie memory-review identity-revision --scope SCOPE --id CANDIDATE --interpretation I`

For a proposed new object Entity and new Predicate, `JSON` is:

```json
{"subject":null,"object":{"entity_id":"","create":true},"predicate":{"predicate_id":"","create":true}}
```

For reuse, supply the exact offered ID and `create:false`. Choices for absent
proposal components must be null. General interpretation editing and batches
belong to later tickets.

## Verification

Public tests cover closed-session CLI acceptance, dependent creation and exact
Claim reuse, Alias/same-name alternatives, unsupported references, confidence
bounds, policy/encoding compatibility, choice history and two-Store CAS, changed
graph/choice staleness, atomic rollback, forbidden Predicate redefinition,
private session isolation, explicit Promotion and source redaction, project
context decisions, and accepted projection replay. Both v1 and v2 golden
encodings are checked. Root orchestration runs required full verification
against the isolated ticket tree before committing.
