## Parent

https://github.com/davidadel66/evie/issues/154

## What to build

Evie answers a memory lookup using relevant accepted Claims and shows their sources.

## Acceptance criteria

- [ ] Provide a bounded read-only relevance search through the existing Memory Plugin and agent turn, combining lexical matches with exact identifiers and aliases. Reuse exact semantic reads; do not replace them or create a general memory-provider framework.
- [ ] Create the first eligible Claim/entity/alias FTS generation with durable coverage, bounded backfill, continuous updates, and restart-safe activation. Queries never silently treat an incomplete generation as complete.
- [ ] Enforce the existing Global/Workspace/project/current-session scope matrix, accepted lifecycle, source authority and eligibility, temporal defaults, remote-memory opt-in, and secret/source fences before provider delivery. Reads do not accept or promote Claims.
- [ ] Define the shared evidence result and minimal request-to-evidence receipt using existing immutable identities, locators, hashes, and Claim versions. Persist sufficient references to inspect the exact supplied evidence after restart; diagnostics remain content-free.
- [ ] Show a minimal Accepted memory activity entry with an inspectable source through existing chat and HTTP/UI patterns. Mark supplied evidence separately from any explicit answer citation.
- [ ] Define bounded query/result/context accounting with documented conservative values measured on the slice fixture. Distinguish empty, unavailable, failed, cancelled, and exhausted searches; repeated calls cannot escape the turn's enforced resource limits.
- [ ] Prove the complete path with real SQLite and a scripted provider, including distractors, wrong-scope matches, stale/retired hits, malformed query, opt-in off, source inspection, restart, and no semantic mutation. Add focused HTTP/UI checks and run required repository verification.

## Blocked by

None (can start immediately).
