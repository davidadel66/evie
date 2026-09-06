# Memory browsing and review UI

David requested this presentation update on 2026-09-05 after approving two response-style memories in General. This supersedes the graph-first presentation described in ui-chat.spec.md; semantic scope, evidence, approval, and replay contracts remain binding.

- Default Memory to a readable list in the active workspace (or Global when no workspace/project is active). Graph remains an alternate view. Every literal graph node opens its owning Claim; every entity opens using its own authorized scope.
- Memories and Review are the primary views. Put Background activity under More. Put record kinds, historical times and diagnostic identifiers in secondary controls.
- Label Global and Workspaces by name. Keep existing conversation/project scopes accessible through an Other scopes group; do not merge, promote, or silently widen stored memory scopes.
- Open a memory in the main content area with a Back action, readable value, subject, scope, exact cited evidence, and collapsed technical history. Keep conflicts and source-unavailability states visible. Preserve denied polarity in every memory rendering.
- Remove redundant descriptive captions; retain meaningful empty/error states and information needed to understand an approval.
- The local `EVIE_OWNER_NAME` configuration supplies the display label for the canonical owner, defaulting to You. David requests David. This is a presentation label: do not rename or recreate the canonical owner anchor, alter claims, or rewrite source evidence. Do not label arbitrary people named owner as the local user.
- Review retains exact effect previews, source/context distinctions, stale/recovery protections, and explicit final approval. Batch setup is secondary.

Verification: focused graph and presentation regressions, HTTP owner-name/default contract checks, complete UI tests and repository verification, then actual desktop/mobile browser checks for list/graph/detail/back, global versus workspace selection, review, background activity, and absence of console errors. Live accepted memories must remain intact.
