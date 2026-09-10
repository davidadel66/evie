## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie finds original statements from earlier conversations, even when no Claim was accepted.

## Acceptance criteria

- [ ] Add bounded read-only conversation search to the shared retrieval path, returning attributed short Conversation Excerpts with timestamps, source identity, exact locators, and hashes; do not invent Claim IDs.
- [ ] Permit same-Workspace, same-project, and Global-to-Global history only. Accepted Global-memory access must not expose raw Global history to Workspace/project sessions; General remains a Workspace and another session's session-scoped Claims remain excluded.
- [ ] Define and record the durable association between retired Claims and their corresponding source evidence before enabling excerpt search. Exclude corresponding evidence from ordinary recall; preserve independently eligible unrelated passages in the same conversation.
- [ ] Exercise whole-content sources, precise ranges, overlapping ranges, and passages supporting multiple Claims. If an association cannot satisfy the approved narrow suppression contract, report that contract gap before enabling the affected retrieval path rather than silently hiding an entire conversation or leaking the retired fact.
- [ ] Backfill and continuously maintain an allowlisted event FTS generation with durable coverage and restart behavior, including eligible history predating this feature. Revalidate source access and eligibility after index matches; never index arbitrary raw payloads or opaque reasoning.
- [ ] Extend the activity entry and original-evidence receipt with Conversation excerpt labeling and speaker/authority attribution. A tentative or quoted statement is not accepted current knowledge, and search does not write Semantic Memory.
- [ ] Verify all scope actors, original-text-only evidence, retirement/restoration, unrelated same-conversation evidence, secret exclusion, empty/failure cases, historical backfill with concurrent new events, restart, and exact provider-bound evidence through the confirmed turn seam and focused UI checks.

## Blocked by

- https://github.com/davidadel66/evie/issues/156
