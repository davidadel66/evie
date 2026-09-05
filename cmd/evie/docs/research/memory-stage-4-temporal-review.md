# Memory Stage 4: temporal review and additional support (#142)

This implements the approved temporal/review contract for existing unambiguous
Entities. `temporal-review-v3` is an explicit Compiler Generation interpretation
policy; all five interpretation-policy fields must agree. Evidence eligibility
remains the existing owner-assertion policy. No production generation is selected
or activated by this change.

The v3 proposal requires `temporal.meaning`: `assertion`, `plan`, or `possibility`.
An assertion retains canonical Typed Literal kind/value, polarity, Predicate
identity/definition and unknown Valid Time bounds. A v3 free-text temporal note
cannot qualify an otherwise unqualified fact. Earlier pinned v1/v2 proposal and
operation encodings remain unchanged.

Plans and possibilities use ordinary Claims with explicit Predicate meaning:

| Meaning | Token | Exact label | Object | Cardinality |
| --- | --- | --- | --- | --- |
| plan | `intends` | `Intends (uncompleted plan)` | text | many |
| possibility | `considers` | `Considers (unrealized possibility)` | text | many |

The literal describes the intended/considered action, including any target date
or uncertainty. The Claim asserts the intention/consideration; it does not assert
that the action happened. If the Predicate does not exist, the extractor proposes
its exact definition and the owner uses the #141 alternatives/choose path to
create it as an explicit dependent effect. A pre-existing token with a different
definition cannot be redefined. Existing definitions and Claims are never
silently migrated. Plan/possibility Claims cannot supersede an actual circumstance.
A future Valid Time start or effective change instant in an actual assertion is
rejected relative to its latest supporting Observed Time, rather than the review
clock. An adopted future decision therefore keeps its modal meaning.

A correction proposal enumerates `error`, `changed`, or the canonical ordered
pair `error,changed`, plus an optional supported effective instant. Preparation
returns `needs_choice` until the owner selects one exact active Claim and mode
through `memory-review temporal-options` and `temporal-choose`. Alternatives are
bounded to 32 Claims in the same destination, subject and Predicate; confidence
never chooses a winner. Each choice creates an immutable revision with parent,
owner authorization, audited choice, original alternatives and exact semantic
revision vector. `temporal-revision` inspects that retained choice. Choices do
not change accepted memory or overwrite original extraction bytes.

The exact preview includes the previous Claim and active lifecycle state, mode,
prior/next intervals, replacement Claim create/reuse, exact new support/context,
conflicts, and the one old-Claim supersession. It uses the Stage 3 interval rules:
error corrections retain the old historical interval; changed corrections close
it at an explicit effective instant and begin the replacement there. Unknown
effective time cannot produce a changed correction. Such a candidate remains
unresolved, or a separately compiled supported assertion can retain unknown
bounds and visible conflicting evidence without superseding history. General
editing and dependent batches remain #144 work.

Acceptance rechecks source eligibility, current exact scope/candidate revisions,
identities, previous Claim/lifecycle and conflict disclosure in the same SQLite
transaction that writes the operation, review outcome, Claims, Source Links and
correction history. The source conversation may be closed. Observed Time stays
with each original event; Transaction Time is the accepted commit time; Valid
Time is the proposed world interval. No event time is invented from acceptance.
An equal active Claim is reused. An exact existing Source Link is reused; a
new supporting projection adds only its Source Link. Retracted support is never
restored implicitly. Even a fully reused group retains its own exact reviewed
operation under the existing idempotence/revision contract.

Canonical review preview/effect domain v3 freezes the typed interpretation and
all correction effects in schema-v6 accepted operations. Replay uses that
recorded payload and Stage 3 correction projections without model calls or
current owner-session leases. Old v1/v2 golden bytes are unchanged; the v3 golden
fixture is `internal/eviedb/testdata/candidate_review_encoding_v3.json`.

Verification uses real SQLite compilation-to-CLI-to-accepted-inspection fixtures,
including changed/error historical queries, plans, possibilities, negation,
unknown dates, duplicates, additional support, stale choices, source/policy
changes, rollback, rejection and a two-Store choice race. Structural checks do
not prove textual entailment: an extractor can still select a wrong supported
interpretation, and the exact evidence plus owner review and later quality panels
remain necessary.
