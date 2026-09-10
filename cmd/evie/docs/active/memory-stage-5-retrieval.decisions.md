# Memory Stage 5 retrieval — decisions

- **2026-09-10 — Ticket breakdown accepted and published.** David accepted the
  proposed breakdown and requested a new-session implement prompt with one PR
  and one commit per GitHub issue. The 13 approved child issues are #156–#168,
  each labeled ready-for-agent and linked to #154 with native blocking edges.
  The implementation handoff uses the implement skill and treats a verified
  predecessor commit on the shared branch as satisfying a local dependency;
  issue closure is not required between commits in the same PR.

- **2026-09-10 — Specification published.** The confirmed specification is
  [issue #154](https://github.com/davidadel66/evie/issues/154), labeled
  ready-for-agent. Its published body and label were verified. David next
  requested the to-tickets workflow; ticket publication follows review of the
  proposed vertical slices and their blocking edges.

- **2026-09-10 — Synthesize the accepted retrieval design.** David requested
  the to-spec workflow after accepting retrieval interview Q1–Q19. The adjacent
  specification consolidates those choices and existing umbrella/ADR
  requirements; its engineering contracts and measured defaults are identified
  as deliverables rather than previously approved values. This task authorizes
  specification publication, not feature implementation.

- **2026-09-10 — Owner confirmed the testing seams.** David accepted the draft
  with "looks good" and requested the to-tickets workflow. Use a
  complete agent turn with real SQLite and a scripted provider as the primary
  acceptance seam, following existing durable-compaction and semantic-scope
  tests. Add focused HTTP/UI checks for source inspection and the memory card.
  This proposal does not introduce separate mocked implementations of each
  retrieval generator. The testing-seam check required before specification
  publication is satisfied.

- **Authority.** The September 10 entries in memory.decisions.md remain the
  binding interview record; ADR 0066 records the scoped conversation-recall
  exception. The adjacent specification does not supersede unrelated memory,
  provider, Task, or compiler decisions.
