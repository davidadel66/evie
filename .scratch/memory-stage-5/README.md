# Memory Stage 5 tickets

Parent specification: [#154](https://github.com/davidadel66/evie/issues/154).
Status: approved and published as #156–#168, with ready-for-agent labels and native dependencies.

1. **[Search accepted memory with inspectable sources](issues/01-search-accepted-memory.md)** — [#156](https://github.com/davidadel66/evie/issues/156)
   Blocked by: none.
   Delivers: Evie answers a memory lookup using relevant accepted Claims and shows their sources.

2. **[Search prior conversations without reviving retired memory](issues/02-search-conversation-evidence.md)** — [#157](https://github.com/davidadel66/evie/issues/157)
   Blocked by: #156.
   Delivers: Evie finds original statements from earlier conversations, even when no Claim was accepted.

3. **[Expand a conversation excerpt without loading the whole chat](issues/03-expand-conversation-excerpts.md)** — [#158](https://github.com/davidadel66/evie/issues/158)
   Blocked by: #157.
   Delivers: Evie reads neighboring messages when a short search hit needs context.

4. **[Answer historical questions and expose conflicting evidence](issues/04-answer-history-and-conflicts.md)** — [#159](https://github.com/davidadel66/evie/issues/159)
   Blocked by: #157.
   Delivers: Evie distinguishes what was said then from what is accepted now, including newer conflicting statements.

5. **[Recall relevant evidence when a new message arrives](issues/05-automatic-recall.md)** — [#160](https://github.com/davidadel66/evie/issues/160)
   Blocked by: #157.
   Delivers: Ordinary requests receive a small relevant selection of accepted memory and conversation evidence automatically.

6. **[Resolve references across topic changes and clarify ambiguity](issues/06-resolve-cross-topic-references.md)** — [#161](https://github.com/davidadel66/evie/issues/161)
   Blocked by: #160.
   Delivers: Evie can return to 'her' after debugging and asks when two identities remain plausible.

7. **[Investigate further and reuse valid evidence within a turn](issues/07-bounded-investigation-and-reuse.md)** — [#162](https://github.com/davidadel66/evie/issues/162)
   Blocked by: #158, #160.
   Delivers: Evie follows useful searches, refreshes changed evidence, and stops honestly within its resource limits.

8. **[Inspect an answer's original evidence after memory changes](issues/08-inspect-original-answer-evidence.md)** — [#163](https://github.com/davidadel66/evie/issues/163)
   Blocked by: #157.
   Delivers: Opening an old answer's sources shows what Evie received then and distinguishes later changes.

9. **[Find evidence through one- and two-hop relationships](issues/09-retrieve-through-relationships.md)** — [#164](https://github.com/davidadel66/evie/issues/164)
   Blocked by: #156.
   Delivers: Evie finds relevant accepted facts connected through known people and relationships.

10. **[Measure a local semantic-retrieval configuration](issues/10-measure-local-semantic-retrieval.md)** — [#165](https://github.com/davidadel66/evie/issues/165)
   Blocked by: #157.
   Delivers: A reproducible experiment identifies how local embeddings recover paraphrased evidence and what they cost.

11. **[Find paraphrased evidence through hybrid semantic search](issues/11-ship-hybrid-semantic-search.md)** — [#166](https://github.com/davidadel66/evie/issues/166)
   Blocked by: #164, #165.
   Delivers: Evie finds relevant evidence even when the wording differs, alongside lexical and relationship matches.

12. **[Run an integrated pilot and freeze retrieval release gates](issues/12-freeze-stage-5-release-gates.md)** — [#167](https://github.com/davidadel66/evie/issues/167)
   Blocked by: #159, #161, #162, #163, #166.
   Delivers: The complete recall experience has measured default budgets and explicit pass/fail release thresholds.

13. **[Evaluate the frozen configuration and declare retrieval readiness](issues/13-verify-stage-5-readiness.md)** — [#168](https://github.com/davidadel66/evie/issues/168)
   Blocked by: #167.
   Delivers: A reproducible assessment establishes whether Stage 5 meets its agreed behavior and measured release gates.

Each draft includes its own acceptance criteria and verification. Existing
exact-memory tools, source locators, turn composition, and UI patterns are
reused; no independent prefactoring ticket is justified by the inspected code.
Basic bounds, access, source receipts, and failure semantics ship with the first
exposed path. Later slices extend capability or evidence inspection, rather
than deferring those baseline guarantees until the end.

Only direct blockers are listed. Relationship retrieval can proceed after 1;
the local semantic experiment can proceed after 2. The integrated pilot freezes
settings on development cases before the final held-out assessment. Stage 4
compiler completion is not a blocker: explicit accepted operations and retained
episodes provide the baseline. Published issue bodies use GitHub references and native blocking edges.


[Implementation session prompt](implementation-prompt.md).
