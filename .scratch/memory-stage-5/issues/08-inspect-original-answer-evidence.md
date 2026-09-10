## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Opening an old answer's sources shows what Evie received then and distinguishes later changes.

## Acceptance criteria

- [ ] Build on the receipts recorded by accepted-memory and conversation search; inspect the exact evidence supplied to each request associated with an answer rather than re-running current relevance search.
- [ ] Expose original Claim versions and Conversation Excerpt locators/hashes through the existing HTTP and compact chat source UI, preserving evidence kind, source authority, and supplied-versus-cited distinctions.
- [ ] Show later corrections or retirement separately from original state. Reapply current source access and display an unavailable-source state without leaking restricted text.
- [ ] Finalize and verify receipt persistence ordering, interrupted-request association, process-reopen behavior, and source hash/locator validation. Do not copy source content into diagnostics or claim access to hidden model reasoning.
- [ ] Demonstrate an answer followed by correction, retirement, and source restriction, then inspect before and after restart. Retain original attribution while preventing current access bypass.
- [ ] Use the real-turn acceptance seam for receipt provenance and focused HTTP/UI tests for observable presentation, stale/unavailable sources, and authorization. Run required verification.

## Blocked by

- https://github.com/davidadel66/evie/issues/157
