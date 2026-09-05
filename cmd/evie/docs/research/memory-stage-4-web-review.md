# Basic owner review in the web interface

Implementation record for [ticket #145](https://github.com/davidadel66/evie/issues/145),
under the [Stage 4 owner-review contract](../active/memory-stage-4-review-contract.decisions.md).
This record does not select an extractor or replace human evaluation.

The Memory workspace now separates **Accepted memory** from **Review candidates**.
The inbox selects one exact destination independently of the selected conversation.
Its navigation uses registered Workspace/project names and durable conversation
titles, including closed conversations. It lists only retained candidate
destinations and global memory, in bounded pages. It does not enumerate evidence
across scopes. Every candidate read, preview, resolution and accepted-operation
inspection separately obtains current local owner authority for its exact scope.

The HTTP adapter uses the existing loopback Host/Origin and exact JSON protections.
All review routes require POST. Complete request bodies cap at 8 KiB, allowing the
contract's 4 KiB reason and exact preview envelope. Duplicate/unknown keys, null
and non-object bodies, trailing values, and unauthorized scope are rejected.
Responses containing review data use `Cache-Control: no-store`. Browser fields
never supply an owner capability or replacement effect payload.

Acceptance displays the exact Claim, concrete Entity and Predicate identities,
typed object, polarity, known/unknown Valid Time, create/reuse effects, conflicts,
supporting Source Links and separate interpretation context. Exact source
locators, authority, observed time, generation and preview/effect digests remain
inspectable. The confirmation sends only the immutable preview identity, digest,
explicit action, owner reason and unique delivery key. Rejection creates no
accepted-memory operation. Advanced identity/Predicate and compound effects
require the later advanced-review UI and cannot be accepted through a partial
basic rendering.

After resolution the inbox refreshes and reads accepted provenance from the
recorded Kernel operation. Current source-policy failures remain failures; the
browser never restores protected quotes from old local data. A stale preview
requires a fresh inspection and explicit approval. A competing terminal decision
shows the recorded result. A transport interruption retains the exact decision
for retry, including across tab unmount/reload in the same browser session.
Session storage retains at most one bounded delivery request (scope, preview
identity/digest, key, action and original reason), with no candidate, preview,
evidence or graph payload. Recovery is an explicit action and obtains current
Kernel authority. A different decision cannot replace an unresolved delivery.

Navigation uses the existing per-scope inbox ledger and its primary key, with
bounded keyset pages; it does not recompute distinct destinations across job
history per request. Installation freezes the last retained candidate ID. Each
navigation call reconciles at most 63 legacy candidates and one progress update
in a transaction; new publication triggers already populate the ledger. Existing
inbox revisions and candidate bytes are preserved. The UI discloses incomplete
indexing and offers another bounded page. Cancellation/failure rolls back the
page and restart resumes its saved position.

Verification covers real SQLite compilation/review after source closure,
acceptance and reopened idempotent delivery, exact CLI/Kernel operation equality,
protected current-source failures, stale revisions, foreign scopes, request
protections, 4 KiB reasons, rejection and conflicting delivery keys. Navigation
tests compile 65 real legacy candidates and check the 63-row bound, transactional
rollback, restart, new publication and retained bytes/revisions. Frontend tests
exercise the production controller and render the actual review components,
including stale responses after scope change/unmount, exact explicit approval,
pagination, unsupported effects, transport retry and reload recovery.

Manual demonstration: open Memory → Review candidates, choose a scope, inspect a
candidate's support/context, preview acceptance, and confirm the exact memory.
The candidate leaves the unresolved inbox and its recorded provenance appears.
Preview and reject another candidate to observe the separate rejection receipt.
Selecting a closed conversation scope does not reopen its conversation. The
orchestrator's temporary browser fixture uses only scripted extraction and a
temporary SQLite database; no live inference or production configuration is
needed for this demonstration.
