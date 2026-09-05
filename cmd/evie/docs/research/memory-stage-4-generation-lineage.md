# Stage 4 generation changes and owner review lineage

Ticket #147 implements the generation/equivalence contract from tickets #133
and #134 on top of explicit activation/backfill (#138/#139) and edits/batches
(#144). It installs no extractor configuration and makes no model-quality claim.

## Comparison and publication

`compiler-recurrence-v2` is an immutable side projection. The old generation,
request, stage, candidate and accepted-operation encodings remain unchanged,
including the `compiler-equivalence-v1` golden. The new explicit generation
policy is `full-effect-equivalence-v2`; existing supported policy manifests keep
their exact identities. Model, prompt, schema, tokenizer/template, decoding and
semantic-policy changes still create separate generations. Endpoint addresses,
worker names, retry timing and queue settings remain operational configuration.

The exact key contains destination, source session/root, all semantic policy
versions, the typed proposal, exact bound Entity and Predicate definitions,
source-bound unresolved Entity mentions, and complete supporting/context
projections including original authority, hashes and any clock observation
contract. Support/context reference order, correction-mode order, timestamp
zone spelling, model confidence and uncertainty prose do not affect equality.
Canonical JSON has a checked golden. A source-bound unresolved mention remains
unresolved even if the current graph now has a matching name; name matching
never binds a known Entity. Current name-search results are review choices,
not an immutable extraction binding.

An indexed related key omits support/context and root identity while keeping
the exact destination and complete typed meaning. It can explain new evidence, including another authorized source session, for a
known proposition; it cannot collapse different source-bound people by name.
A hash match also requires byte equality of the canonical encoding. Different
semantic policies are conservatively fresh. No model similarity score is used.

Publication reads the primary's current review revision and the latest edit,
identity and temporal interpretation revisions in the same immediate transaction
as the complete candidate group and recurrence rows. A duplicate retains its
own original output and `unresolved` state, links the checked primary, and has
no independent review preview. The primary's later resolution remains visible
through that direct link. An edited primary is never overwritten or reopened.
An original-output repeat of an accepted edit links the actual edited result;
it does not assert that the original output was accepted.

For an accepted primary, the system checks its actual reviewed Claims,
Entities, Source Links, aliases and correction lifecycle/valid-time effects,
including dependent group members, against current accepted state. Corrected
intervals come from the exact correction-ledger projection; the original Claim
row still retains its immutable pre-correction interval. A changed
effect creates a new actionable primary with the earlier decision linked.
A monotonically increasing presentation epoch separates that explicit changed
effect from its predecessor; within an epoch, the earliest durable candidate
remains primary. The order comes from candidate insertion, never source-event
chronology or model-generated IDs. Existing Claims, Predicate definitions,
accepted operations and coverage are not changed by classification.

## Bounded work and upgrade

Publication handles at most 16 outputs. A full group uses 53 row mutations,
including all 16 existing inbox-revision trigger writes: 16 candidates, 16 side
projections, 16 inbox revisions, and five group/coverage/stage/resource/job
writes. A real SQLite `total_changes()` test enforces the contract's 64-mutation
ceiling. Indexed exact/related lookups each read at most one representative;
current interpretation lookups use three indexed latest-revision probes. Review
and graph reads are limited by the existing bounded candidate/effect envelopes.

Installation creates the side table and processes at most 31 legacy candidates
plus one migration cursor per open. It never rewrites their original bytes or
copies terminal state. A failed page rolls back completely. A bounded lookup
through the old hash index can prove equality before a legacy row is projected;
if that lookup cannot prove equality, the new output stays reviewable. There is
no unbounded scan, automatic history compilation, deletion or purge. Tests
check the query plans with 2,000 retained rows and reopen a 64-candidate legacy
fixture in pages of 31, 31 and 2.

## Owner inspection

Use the existing exact-scope CLI:

```text
memory-review lineage --scope global --id CANDIDATE_ID
memory-candidates inspect --session SESSION_ID --id JOB_OR_SELECTION_ID
memory-backfill status --selection EXACT_SELECTION_JSON_FILE --range 0
```

`lineage` returns the candidate's producing pinned configuration and selected
unit, the relationship and publication's checked revisions, the primary's
current interpretation, and its actual decision, reason and resolution/operation receipt. The
response follows one link, so related history remains inspectable without
loading an unbounded chain. It rechecks current destination authorization and
source policy for both candidates and every disclosed dependent resolution
member. Revoked sources redact the lineage; retention does not grant disclosure.
The inspection type is separate from review previews and canonical operations.

Activation of a new generation captures a fresh frontier. Historical work
requires a separate bounded selection. The integration fixture activates an
upgrade, proves that it queued no historical work, selects the old interval,
cancels/resumes it, and verifies separate completion and the preserved rejection.
Review state never supplies coverage. Existing worker fences, shared capacity
and staged adoption remain in force across generation changes.

Deterministic checks cover original/edited unresolved, accepted and rejected
primaries; concurrent edit/reject before late publication; later decisions;
changed support, policy, identity, temporal and accepted lifecycle effects;
original identity proposals after creating their reviewed Entity; publication
rollback/adoption; CLI review after session closure/reopen; canonical replay;
and bounds. Actual model adequacy, David's pilot observations and the untouched
holdout gates remain separate pending evaluation work.
