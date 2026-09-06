## Parent

[Memory Stage 4: Compile sourced memory candidates for owner review](https://github.com/davidadel66/evie/issues/131)

## What to build

Extend extraction and owner review to supported Entity-valued relationships and project/Workspace facts, using exact identities and lexical alternatives. Let the owner resolve ambiguity and review any new Predicate definition as part of the exact effect. Reuse the existing accepted graph operations rather than adding learned resolution.

## Acceptance criteria

- [ ] Reuse supported existing Entities and Aliases with exact/lexical candidates; preserve same-name alternatives, unresolved references, and uncertainty instead of automatically merging people.
- [ ] Show identity alternatives and their supporting context through the CLI review flow. The owner’s chosen resolution becomes part of the preview-bound effect, and graph changes can stale that preview.
- [ ] Start from the reviewed Predicate vocabulary. New definitions and dependent Entity/relationship effects are explicit and atomically reviewed; do not globally redefine existing Predicate meaning.
- [ ] Keep scope and explicit Promotion rules across identity lookup, alternatives, new definitions, source visibility, and acceptance. Model-selected identity or scope never overrides Kernel authority.
- [ ] Create or reuse Entities, Claims and supporting links according to established Stage 3 canonical identities, equality and source semantics, without duplicate propositions or partial dependent effects.
- [ ] Support useful people/relationship and project-decision fixtures with same-name, alias, unknown identity and uncertain Predicate cases. Keep unsupported interpretations and model confidence visible as uncertainty rather than authorization.
- [ ] Test the complete extraction-to-CLI-review-to-inspection/replay path plus stale resolution and scope boundaries. Run focused checks and repository-required full change verification.
- [ ] Exclude embeddings, dense matching, automatic merging and graph-learning changes.

## Blocked by

- Draft 09: Accept or reject a candidate after its conversation closes

